package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sjsanc/gorc/logger"
	"github.com/sjsanc/gorc/manager"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/storage"
	"github.com/urfave/cli/v2"
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
			Usage:   "Container runtime to use (docker, podman)",
			Value:   "docker",
		},
	},
	Action: func(c *cli.Context) error {
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

		m, err := manager.NewManager(logger.Log, addr, port, storageType, runtimeType)
		if err != nil {
			fmt.Println("Error creating manager:", err)
			return err
		}

		// Handle graceful shutdown on SIGINT/SIGTERM
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		// Run manager in background
		errChan := make(chan error, 1)
		go func() {
			errChan <- m.Run()
		}()

		// Wait for shutdown signal or error
		select {
		case sig := <-sigChan:
			logger.Log.Infof("Received signal %v, shutting down gracefully...", sig)
			if err := m.Stop(); err != nil {
				logger.Log.Errorf("Error during shutdown: %v", err)
				return err
			}
			fmt.Println("Manager shut down successfully")
			return nil
		case err := <-errChan:
			return err
		}
	},
}
