package cmd

import (
	"fmt"

	"github.com/sjsanc/gorc/runtime"
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
			Usage:   "Port for the Worker server to listen on (if not specified, auto-select from 6000-7000)",
			Value:   0,
		},
		&cli.StringFlag{
			Name:    "manager",
			Aliases: []string{"m"},
			Usage:   "Address of the Manager server",
			Value:   "0.0.0.0:5555",
		},
		&cli.StringFlag{
			Name:    "runtime",
			Aliases: []string{"r"},
			Usage:   "Container runtime to use (docker)",
			Value:   "docker",
		},
	},
	Action: func(c *cli.Context) error {
		logger, _ := zap.NewProduction()
		defer logger.Sync()
		sugar := logger.Sugar()

		addr := c.String("address")
		port := c.Int("port")
		managerAddr := c.String("manager")
		runtimeStr := c.String("runtime")

		runtimeType, err := runtime.ParseRuntimeType(runtimeStr)
		if err != nil {
			fmt.Println("Error parsing runtime type:", err)
			return err
		}

		// If port is 0, auto-select from available range
		if port == 0 {
			port, err = worker.FindAvailablePort()
			if err != nil {
				fmt.Println("Error finding available port:", err)
				return err
			}
		}

		fmt.Printf("Starting Gorc Worker on %s:%d\n", addr, port)

		w, err := worker.NewWorker(sugar, addr, port, managerAddr, runtimeType)
		if err != nil {
			fmt.Println("Error starting worker:", err)
			return err
		}

		return w.Run()
	},
}
