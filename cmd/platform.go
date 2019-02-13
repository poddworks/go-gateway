package main

import (
	"os"

	. "github.com/poddworks/go-gateway/cli"
	. "github.com/poddworks/go-gateway/gateway"

	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli"
)

var (
	logger = log.WithFields(log.Fields{"stack": "main"})
)

func main() {
	app := cli.NewApp()

	app.Name = AppNameString
	app.HelpName = AppNameString
	app.Usage = UsageString
	app.Version = VersionString
	app.Description = DescriptionString

	app.Before = func(c *cli.Context) error {
		if err := Init(); err != nil {
			return err // setup failure, cannot bootstrap HTTP server and mux
		}

		// based on command line option/flag, turn on storage support
		if err := EnableStore(); err != nil {
			return err // setup failure, cannot enable storage
		}

		// based on command line option/flag, turn on webhook support
		if err := EnableWebhook(); err != nil {
			return err // setup failure, cannot enable webhook
		}

		// based on command line option/flag, turn on function support
		if err := EnableFunction(); err != nil {
			return err // setup failure, cannot enable function
		}

		return nil
	}

	app.Action = func(c *cli.Context) {
		if c.Args().First() == "version" {
			cli.VersionPrinter(c)
			return
		}

		// launch gateway api service
		errc := Start(c.App.Name)

		// report error if there are any
		logger.Errorln(<-errc)
	}

	app.Run(os.Args)
}
