package gateway

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	. "github.com/poddworks/go-gateway/gateway-api/constant"
	. "github.com/poddworks/go-gateway/gateway-api/message"
	log "github.com/sirupsen/logrus"
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
		ctx := context.WithValue(r.Context(), MessageCtxString, message)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	var logger = log.WithFields(log.Fields{"stack": "request-log"})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.WithFields(log.Fields{"request": r.Context().Value(MessageCtxString)}).Debug("request-log")
		next.ServeHTTP(w, r)
	})
}
