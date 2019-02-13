package cli

import (
	"fmt"
	"path/filepath"

	cli "github.com/urfave/cli"
)

func init() {
	cli.VersionPrinter = customVersionPrinter
}

func customVersionPrinter(c *cli.Context) {
	fmt.Printf("%v version %v, %v, build (%v)\n", filepath.Base(c.App.Name), VersionString, GoVersionString, RevisionString)
}
