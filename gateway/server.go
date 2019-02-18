package gateway

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	. "github.com/poddworks/go-gateway/gateway-api/message"
	log "github.com/sirupsen/logrus"
)

var (
	server *http.Server

	router *mux.Router
	logger = log.WithFields(log.Fields{"stack": "service-root"})
)

func EnableWebhook() error {
	route := router.PathPrefix("/webhook/")
	route.Methods("POST").HandlerFunc(notYetImpl)
	return nil
}

func EnableStore() error {
	sub := router.PathPrefix("/functions/").Subrouter()
	sub.Methods("POST").Path("/{functionName}").HandlerFunc(notYetImpl)
	return nil
}

func EnableFunction() error {
	sub := router.PathPrefix("/classes/").Subrouter()
	sub.Methods("GET", "POST").Path("/{className}").HandlerFunc(notYetImpl)
	sub.Methods("GET", "PUT", "DELETE").Path("/{className}/{objectId}").HandlerFunc(notYetImpl)
	return nil
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.WithFields(log.Fields{"request": r.Context().Value(MessageCtxString)}).Info("request-log")
		next.ServeHTTP(w, r)
	})
}

func bodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
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
				http.Error(w, fmt.Sprint("error/invalid-request/", err.Error()), 400)
				return
			}

			message.Body = content
		}
		ctx := context.WithValue(r.Context(), MessageCtxString, message)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Init() error {
	router = mux.NewRouter()

	// Setup middleware
	router.Use(bodyMiddleware, loggingMiddleware)

	// TODO: should allow setup to more settings
	// https://golang.org/pkg/net/http/#Server
	server = &http.Server{
		Addr:    ":8080",
		Handler: router,

		ReadHeaderTimeout: 100 * time.Millisecond,
		ReadTimeout:       1 * time.Second,

		WriteTimeout: 1 * time.Second,

		IdleTimeout: 120 ^ time.Second,

		MaxHeaderBytes: 1 << 20,
	}

	return nil
}

func Start(name string) <-chan error {
	// Attach Catch All handler
	router.PathPrefix("/").HandlerFunc(notSupported)

	errc := make(chan error)
	go func() {
		errc <- server.ListenAndServe()
	}()
	logger.WithFields(log.Fields{"name": name, "addr": server.Addr}).Info("begin")
	return errc
}
