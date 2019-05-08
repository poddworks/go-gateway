package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"encoding/json"

	. "github.com/poddworks/go-gateway/cli"
	. "github.com/poddworks/go-gateway/gateway-api/constant"
	. "github.com/poddworks/go-gateway/gateway-api/message"
	. "github.com/poddworks/go-gateway/gateway-api/types"

	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli"

	AmqpClient "github.com/poddworks/go-gateway/gateway-api/rpc/go"
)

func main() {
	app := cli.NewApp()

	app.Name = UtilAppNameString
	app.HelpName = UtilAppNameString
	app.Usage = UtilUsageString
	app.Version = VersionString
	app.Description = UtilDescriptionString

	app.Flags = []cli.Flag{
		cli.StringSliceFlag{
			Name:  "amqp-routing-key",
			Usage: "Specify routing key(s) to bind for the API worker",
		},
		cli.StringFlag{
			Name:  "amqp-queue-name",
			Usage: "Specify a queue for the API worker",
			Value: "",
		},
		cli.StringFlag{
			Name:   "amqp-endpoint",
			Usage:  "Specify URI to AMQP (https://www.rabbitmq.com/uri-spec.html)",
			EnvVar: "GATEWAY_ENDPOINT_AMQP",
			Value:  "amqp://localhost",
		},
		cli.UintFlag{
			Name:   "amqp-qos-prefetch",
			Usage:  "Specify AMQP client prefetch count",
			EnvVar: "GATEWAY_ENDPOINT_AMQP_QOS_PREFETCH",
			Value:  25,
		},
		cli.BoolFlag{
			Name:   "amqp-qos-global",
			Usage:  "Specify AMQP client QoS settings to all existing and future consumers on all channels on the same connection",
			EnvVar: "GATEWAY_ENDPOINT_AMQP_QOS_GLOBAL",
		},
	}

	app.Action = func(c *cli.Context) {
		if c.Args().First() == "version" {
			cli.VersionPrinter(c)
			return
		}

		var logger = log.WithFields(log.Fields{"stack": "main"})

		// TODO: Should be configured via command line args
		log.SetLevel(log.DebugLevel)

		logger.WithFields(log.Fields{"name": c.App.Name}).Info("begin")
		for _, flag := range c.App.VisibleFlags() {
			var field string = flag.GetName()
			if field != "help, h" && field != "version, v" {
				logger.WithFields(log.Fields{"value": c.String(field)}).Debug(field)
			}
		}

		root, cancel := context.WithCancel(context.Background())
		defer cancel()

		var (
			ExchangeForPublish = fmt.Sprintf("%v-forward-exchange", AppNameString)
			RoutingKeys        = c.StringSlice("amqp-routing-key")
		)

		// Setup AMQP client
		client := AmqpClient.New(c.Int("amqp-qos-prefetch"), 0, c.Bool("amqp-qos-global"))

		// Build reuqest subscription
		requestSubOpts := &SubscriptionRequest{
			Queue: &QueueRequest{
				AutoDelete: true,
				Exclusive:  true,
			},
			Exchange: &ExchangeRequest{
				Name: ExchangeForPublish,
				Kind: "topic",
			},
			Binding: make([]*BindRequest, 0),
		}
		if c.String("amqp-queue-name") != "" {
			requestSubOpts.Queue.Name = c.String("amqp-queue-name")
			requestSubOpts.Queue.Durable = true
			requestSubOpts.Queue.AutoDelete = false
			requestSubOpts.Queue.Exclusive = false
		}
		for _, RoutingKey := range RoutingKeys {
			requestSubOpts.Binding = append(requestSubOpts.Binding, &BindRequest{
				RoutingKey: RoutingKey,
			})
		}

		// Execute SubscriptionRequest
		subscription := client.Subscribe(root, requestSubOpts)

		streamCh := subscription.Stream()
		if streamCh == nil {
			logger.WithFields(log.Fields{"error": subscription.Error()}).Error("init-failed")
			return
		}

		// Prepare commit channel to echo back response
		publishCh := client.Commit(root)

		// Connect AMQP client
		AmqpClient.Connect(context.WithValue(root, LoggingCtxString, logger), client, c.String("amqp-endpoint"))

		for message := range streamCh {
			logger.WithFields(log.Fields{"message": message.Payload}).Debug("request-log")
			func() {
				defer message.Ack(false)

				if message.Payload.Content.Header.Get("Content-Type") == "application/json" {
					body := make(map[string]interface{})
					json.Unmarshal(message.Payload.Content.Body, &body)
					logger.WithFields(log.Fields{"body": body}).Debug("request-log.body")
				}

				request := &CommitRequest{
					Exchange:   &ExchangeRequest{},
					RoutingKey: "",
					Payload: &Payload{
						ContentType: "application/json",
						Content: &Message{
							RequestId: message.Payload.Content.RequestId,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
						},
					},
				}

				body, err := json.Marshal(message.Payload.Content)
				if err != nil {
					logger.WithFields(log.Fields{"payload": message.Payload, "error": err}).Error("request-log")
					return
				}

				request.Payload.Content.Body = body
				request.SetRoutingKey(message.CorrelationId()).SetExchange(message.ReplyTo())

				// reply to commit publishing stream
				publishCh <- request
			}()
		}
	}

	app.Run(os.Args)
}
