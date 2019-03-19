package gateway

import (
	"context"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli"

	. "github.com/poddworks/go-gateway/gateway-api/constant"
	AmqpClient "github.com/poddworks/go-gateway/gateway-api/rpc/go"
)

var (
	server *http.Server

	router *mux.Router

	client = AmqpClient.New()
)

func EnableWebhook(c *cli.Context) error {
	route := router.PathPrefix("/webhook/")
	route.Methods("POST").HandlerFunc(notYetImpl)
	return nil
}

func EnableStore(c *cli.Context) error {
	sub := router.PathPrefix("/functions/").Subrouter()
	sub.Methods("POST").Path("/{functionName}").HandlerFunc(notYetImpl)
	return nil
}

func EnableFunction(c *cli.Context) error {
	sub := router.PathPrefix("/classes/").Subrouter()
	sub.Methods("GET", "POST").Path("/{className}").HandlerFunc(notYetImpl)
	sub.Methods("GET", "PUT", "DELETE").Path("/{className}/{objectId}").HandlerFunc(notYetImpl)
	return nil
}

func Init(c *cli.Context) error {
	// Shoudl be configured via command line args
	log.SetLevel(log.DebugLevel)

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

func Start(root context.Context, c *cli.Context) <-chan error {
	var logger = log.WithFields(log.Fields{"stack": "service-root"})

	// Attach Catch All handler
	router.PathPrefix("/").HandlerFunc(notSupported)

	errc := make(chan error)
	go func() {
		errc <- server.ListenAndServe()
	}()

	logger.WithFields(log.Fields{"name": c.App.Name, "addr": server.Addr}).Info("begin")
	for _, flag := range c.App.VisibleFlags() {
		var field string = flag.GetName()
		if field != "help, h" && field != "version, v" {
			logger.WithFields(log.Fields{"value": c.String(field)}).Debug(field)
		}
	}

	return errc
}

func StartWorker(root context.Context, c *cli.Context) <-chan error {
	var logger = log.WithFields(log.Fields{"stack": "service-root.amqp"})

	var (
		subscription = client.Each(root)

		wait sync.WaitGroup
	)

	errc := make(chan error)
	go func() {
		_, cancel := AmqpClient.Connect(context.WithValue(root, LoggingCtxString, logger), client, c.String("endpoint-amqp"))
		defer cancel()

		wait.Add(runtime.NumCPU())
		for idx := 0; idx < runtime.NumCPU(); idx++ {
			go func() {
				defer wait.Done()
				for {
					select {
					case <-root.Done():
						return

					case _, ok := <-subscription:
						if !ok {
							subscription = nil
						}
					}
				}
			}()
		}
		wait.Wait()

		// mark consumer complete
		errc <- nil
	}()

	return errc
}
