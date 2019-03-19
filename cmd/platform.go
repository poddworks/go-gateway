package main

import (
	"context"
	"os"

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
			Name:   "endpoint-amqp",
			Usage:  "Specify URI to AMQP (https://www.rabbitmq.com/uri-spec.html)",
			EnvVar: "ENDPOINT_AMQP",
			Value:  "amqp://localhost",
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

		errc, errw := Start(root, c), StartWorker(root, c)
		for errc != nil && errw != nil {
			select {
			case err := <-errc:
				errc = nil
				if err != nil {
					logger.WithFields(log.Fields{"error": err}).Error("Start")
				}

			case err := <-errw:
				errw = nil
				if err != nil {
					logger.WithFields(log.Fields{"error": err}).Error("StartWorker")
				}
			}

			cancel() // if the error channel reported from any party, we halt
		}
	}

	app.Run(os.Args)
}
