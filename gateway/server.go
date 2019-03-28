package gateway

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli"

	. "github.com/poddworks/go-gateway/gateway-api/constant"
	. "github.com/poddworks/go-gateway/gateway-api/message"
	. "github.com/poddworks/go-gateway/gateway-api/types"

	AmqpClient "github.com/poddworks/go-gateway/gateway-api/rpc/go"
)

var (
	server *http.Server

	router *mux.Router

	client *AmqpClient.AmqpClient

	publishCh chan<- *CommitRequest
)

func EnableWebhook(c *cli.Context) error {
	var (
		ExchangeForPublish = fmt.Sprintf("%v-forward-exchange", c.App.Name)
		ExchangeForReply   = fmt.Sprintf("%v-reply-exchange", c.App.Name)

		CorrelationId = fmt.Sprintf("%v", c.String("task-id"))
	)

	sub := router.PathPrefix("/webhook/").Subrouter()

	sub.Use(buildCommitRequest, buildPublishAction(
		func(ctx context.Context) error {
			message := ctx.Value(MessageCtxString).(*Message)
			request := ctx.Value(CommitCtxString).(*CommitRequest)

			// request destination
			request.SetRoutingKey(fmt.Sprintf("webhook.%v", message.PathParameters["endpoint"])).
				SetExchange(ExchangeForPublish)

			// request reply address
			request.ReplyTo(ExchangeForReply).CorrelationId(CorrelationId)

			return nil
		},
	))

	sub.Methods("POST").Path("/{endpoint}").HandlerFunc(buildResponse)

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

		WriteTimeout: 120 * time.Second,

		IdleTimeout: 120 ^ time.Second,

		MaxHeaderBytes: 1 << 20,
	}

	// Setup AMQP client
	client = AmqpClient.New(c.Int("amqp-qos-prefetch"), 0, c.Bool("amqp-qos-global"))

	return nil
}

func ConnectSetup(root context.Context, c *cli.Context) <-chan error {
	var logger = log.WithFields(log.Fields{"stack": "service-root.amqp-conn"})

	_, cancel := AmqpClient.Connect(context.WithValue(root, LoggingCtxString, logger), client, c.String("amqp-endpoint"))

	// Prep publishing channel
	publishCh = client.Commit(root)

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		defer cancel()

		// Wait for disconnect signal
		<-root.Done()

		logger.Debug("AmqpClient-termination")
	}()

	return errc
}

func Start(root context.Context, c *cli.Context) <-chan error {
	var logger = log.WithFields(log.Fields{"stack": "service-root.api"})

	// Attach Catch All handler
	router.PathPrefix("/").HandlerFunc(notSupported)

	errc := make(chan error, 1)
	go func() {
		defer close(errc)

		errListenAndServe := make(chan error, 1)
		go func() {
			errListenAndServe <- server.ListenAndServe()
		}()

		logger.WithFields(log.Fields{"name": c.App.Name, "addr": server.Addr}).Info("begin")
		for _, flag := range c.App.VisibleFlags() {
			var field string = flag.GetName()
			if field != "help, h" && field != "version, v" {
				logger.WithFields(log.Fields{"value": c.String(field)}).Debug(field)
			}
		}

		select {
		case <-root.Done():
			errc <- server.Close()

		case err := <-errListenAndServe:
			if err != nil {
				errc <- err
			}
		}

		logger.Debug("Start-termination")
	}()

	return errc
}

func StartWorker(root context.Context, c *cli.Context) <-chan error {
	var (
		logger = log.WithFields(log.Fields{"stack": "service-root.amqp-sub"})

		wait sync.WaitGroup
	)

	var (
		ExchangeForReply = fmt.Sprintf("%v-reply-exchange", c.App.Name)
		RoutingKey       = fmt.Sprintf("%v", c.String("task-id"))
	)

	pending := client.PendingRequests()

	subscription := client.Subscribe(root, &SubscriptionRequest{
		Queue: &QueueRequest{
			AutoDelete: true,
			Exclusive:  true,
		},
		Exchange: &ExchangeRequest{
			Name: ExchangeForReply,
			Kind: "topic",
		},
		Binding: []*BindRequest{
			&BindRequest{
				RoutingKey: RoutingKey,
			},
		},
	})

	errc := make(chan error, 1)
	go func() {
		defer close(errc)

		streamC := subscription.Stream()
		if streamC == nil {
			errc <- subscription.Error()
			return
		}

		wait.Add(runtime.NumCPU())
		for idx := 0; idx < runtime.NumCPU(); idx++ {
			go func() {
				defer wait.Done()
				for message := range streamC {
					logger.WithFields(log.Fields{"message": message.Payload}).Debug("StartWorker-subscription")
					message.Ack(false)
					if request := pending.Once(message.RequestId()); request != nil {
						request.Incoming() <- message.Payload
					}
				}
			}()
		}
		wait.Wait()
		if err := subscription.Error(); err != nil {
			errc <- err
		} else {
			errc <- nil
		}

		logger.Debug("StartWorker-termination")
	}()

	return errc
}
