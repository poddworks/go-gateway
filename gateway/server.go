package gateway

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
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

func Init() error {
	router = mux.NewRouter()

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
