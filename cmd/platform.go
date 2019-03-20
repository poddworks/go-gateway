package main

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/poddworks/go-gateway/cli"
	. "github.com/poddworks/go-gateway/gateway"

	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()

	app.Name = AppNameString
	app.HelpName = AppNameString
	app.Usage = UsageString
	app.Version = VersionString
	app.Description = DescriptionString

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "task-id",
			Usage:  "Specify a task id for the API worker, default to Unix timestamp of the time service is launched",
			EnvVar: "GATEWAY_TASK_ID",
			Value:  fmt.Sprintf("%v-%v", AppNameString, time.Now().Unix()),
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

	app.Before = func(c *cli.Context) error {
		if err := Init(c); err != nil {
			return err // setup failure, cannot bootstrap HTTP server and mux
		}

		// based on command line option/flag, turn on storage support
		if err := EnableStore(c); err != nil {
			return err // setup failure, cannot enable storage
		}

		// based on command line option/flag, turn on webhook support
		if err := EnableWebhook(c); err != nil {
			return err // setup failure, cannot enable webhook
		}

		// based on command line option/flag, turn on function support
		if err := EnableFunction(c); err != nil {
			return err // setup failure, cannot enable function
		}

		return nil
	}

	app.Action = func(c *cli.Context) {
		if c.Args().First() == "version" {
			cli.VersionPrinter(c)
			return
		}

		var logger = log.WithFields(log.Fields{"stack": "main"})

		root, cancel := context.WithCancel(context.Background())

		errc, errs, errw := ConnectSetup(root, c), Start(root, c), StartWorker(root, c)
		for errc != nil || errs != nil || errw != nil {
			select {
			case err := <-errc:
				errc = nil
				if err != nil {
					logger.WithFields(log.Fields{"error": err}).Error("Connect")
				}

			case err := <-errs:
				errs = nil
				if err != nil {
					logger.WithFields(log.Fields{"error": err}).Error("Start")
				}

			case err := <-errw:
				errw = nil
				if err != nil {
					logger.WithFields(log.Fields{"error": err}).Error("StartWorker")
				}
			}

			cancel() // Cancel current context to notify halt
		}
	}

	app.Run(os.Args)
}
