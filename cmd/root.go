package cmd

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func Exec() {
	app := &cli.App{
		Name:  "gorc",
		Usage: "A lightweight container orchestrator.",
		Commands: []*cli.Command{
			managerCmd,
			workerCmd,
			listCmd,
			deployCmd,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
