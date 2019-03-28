package gateway

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	. "github.com/poddworks/go-gateway/gateway-api/constant"
	. "github.com/poddworks/go-gateway/gateway-api/message"
	. "github.com/poddworks/go-gateway/gateway-api/types"
)

func bodyMiddleware(next http.Handler) http.Handler {
	var logger = log.WithFields(log.Fields{"stack": "http-parser"})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			logger.WithFields(log.Fields{"error": err}).Error("ParseForm")
			http.Error(w, fmt.Sprint("error/invalid-request/", err.Error()), 400)
			return
		}
		var query = url.Values{}
		if r.Form != nil {
			for key, vals := range r.Form {
				for _, val := range vals {
					query.Add(key, val)
				}
			}
		}
		if r.PostForm != nil {
			for key, vals := range r.PostForm {
				for _, val := range vals {
					query.Add(key, val)
				}
			}
		}
		var oldCtx context.Context = r.Context()
		message := &Message{
			RequestId:       requestId(),
			Method:          r.Method,
			Host:            r.Host,
			RemoteAddr:      r.RemoteAddr,
			Header:          r.Header,
			QueryParameters: query,
			PathParameters:  mux.Vars(r),
		}
		if r.Body != nil {
			content, err := ioutil.ReadAll(r.Body)
			if err != nil {
				logger.WithFields(log.Fields{"error": err}).Error("Body-ReadAll")
				http.Error(w, fmt.Sprint("error/invalid-request/", err.Error()), 400)
				return
			}

			message.Body = content
		}
		var newCtx context.Context
		newCtx = context.WithValue(oldCtx, MessageCtxString, message)
		newCtx = context.WithValue(newCtx, RequestCtxString, message.RequestId)
		next.ServeHTTP(w, r.WithContext(newCtx))
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	var logger = log.WithFields(log.Fields{"stack": "request-log"})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		message := r.Context().Value(MessageCtxString).(*Message)
		requestId := r.Context().Value(RequestCtxString).(string)

		_logger := logger.WithFields(log.Fields{"requestId": requestId})

		var newCtx context.Context
		newCtx = context.WithValue(r.Context(), LoggingCtxString, _logger)

		_logger.WithFields(log.Fields{"message": message}).Debug("incoming")
		next.ServeHTTP(w, r.WithContext(newCtx))
	})
}

func buildCommitRequest(next http.Handler) http.Handler {
	var (
		timeout, _ = time.ParseDuration("30s")
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		message := r.Context().Value(MessageCtxString).(*Message)
		requestId := r.Context().Value(RequestCtxString).(string)

		requestCtx, _ := context.WithTimeout(context.Background(), timeout)
		request := &CommitRequest{
			Exchange:   &ExchangeRequest{},
			RoutingKey: "",
			Payload: &Payload{
				ContentType: "application/json",
				Content:     message,
			},
			ReplyAddr: &ReplyAddr{
				RequestId: requestId,
				Context:   requestCtx,
				Reply:     make(chan *Payload, 1),
			},
		}

		ctx := context.WithValue(r.Context(), CommitCtxString, request)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func buildPublishAction(customize func(ctx context.Context) error) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var current = r.Context()

			// run customize on request
			if err := customize(current); err != nil {
				unexpectedError(w, r)
				return
			}

			// Submit and Register reply feedback
			publishCh <- current.Value(CommitCtxString).(*CommitRequest)

			// next handler
			next.ServeHTTP(w, r)
		})
	}
}

func buildResponseHeader(w http.ResponseWriter, payload *Payload) {
	for key, val := range payload.Headers {
		w.Header().Add(key, val.(string))
	}
	if payload.ContentType != "" {
		w.Header().Add("Content-Type", payload.ContentType)
	}
	if payload.ContentEncoding != "" {
		w.Header().Add("Content-Encoding", payload.ContentEncoding)
	}
}

func buildResponse(w http.ResponseWriter, r *http.Request) {
	var pending = client.PendingRequests()

	request := r.Context().Value(CommitCtxString).(*CommitRequest)
	logger := r.Context().Value(LoggingCtxString).(*log.Entry)

	defer pending.Once(request.RequestId())
	select {
	case <-request.Timeout():
		logger.Debug("incoming-timeout")
		requestTimeout(w, r)
	case payload, ok := <-request.Reply():
		logger.Debug("incoming-reply")
		if !ok {
			unexpectedError(w, r)
		}
		if payload != nil {
			// prep response header
			buildResponseHeader(w, payload)

			// stream response as raw data
			_, err := io.Copy(w, payload.NewReader())
			if err != nil {
				logger.WithFields(log.Fields{"error": err}).Error("incoming-reply-error")
				unexpectedError(w, r)
			}
		}
	}
}
