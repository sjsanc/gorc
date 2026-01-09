package cmd

import (
	"fmt"

	"github.com/sjsanc/gorc/worker"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

var workerCmd = &cli.Command{
	Name:  "worker",
	Usage: "Start a worker process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "address",
			Aliases: []string{"a"},
			Usage:   "Address for the Worker server to listen on",
			Value:   "0.0.0.0",
		},
		&cli.IntFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Usage:   "Port for the Worker server to listen on",
			Value:   5556,
		},
		&cli.StringFlag{
			Name:    "manager",
			Aliases: []string{"m"},
			Usage:   "Address of the Manager server",
			Value:   "0.0.0.0:5555",
		},
	},
	Action: func(c *cli.Context) error {
		logger, _ := zap.NewProduction()
		defer logger.Sync()
		sugar := logger.Sugar()

		addr := c.String("address")
		port := c.Int("port")
		managerAddr := c.String("manager")

		fmt.Printf("Starting Gorc Worker on %s:%d\n", addr, port)

		w, err := worker.NewWorker(sugar, addr, port, managerAddr)
		if err != nil {
			fmt.Println("Error starting worker:", err)
			return err
		}

		w.Run()

		return nil
	},
}
