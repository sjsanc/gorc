package cmd

import (
	"fmt"

	"github.com/sjsanc/gorc/manager"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/storage"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

var managerCmd = &cli.Command{
	Name:  "manager",
	Usage: "Start a manager process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "address",
			Aliases: []string{"a"},
			Usage:   "Address for the Manager server to listen on",
			Value:   "0.0.0.0",
		},
		&cli.IntFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Usage:   "Port for the Manager server to listen on",
			Value:   5555,
		},
		&cli.StringFlag{
			Name:    "storage",
			Aliases: []string{"s"},
			Usage:   "Storage type to use for the Manager",
			Value:   "memory",
		},
		&cli.StringFlag{
			Name:    "runtime",
			Aliases: []string{"r"},
			Usage:   "Container runtime to use for the Manager",
			Value:   "docker",
		},
	},
	Action: func(c *cli.Context) error {
		logger, _ := zap.NewProduction()
		defer logger.Sync()
		sugar := logger.Sugar()

		addr := c.String("address")
		port := c.Int("port")

		fmt.Printf("Starting Gorc Manager on %s:%d\n", addr, port)

		storageType, err := storage.ParseStorageType(c.String("storage"))
		if err != nil {
			fmt.Println("Unknown storage type:", err)
			return err
		}

		runtimeType, err := runtime.ParseRuntimeType(c.String("runtime"))
		if err != nil {
			fmt.Println("Unknown runtime type:", err)
			return err
		}

		m, err := manager.NewManager(sugar, addr, port, storageType, runtimeType)
		if err != nil {
			fmt.Println("Error creating manager:", err)
			return err
		}

		return m.Run()
	},
}
