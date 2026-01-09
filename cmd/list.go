package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/urfave/cli/v2"
)

var listCmd = &cli.Command{
	Name:  "list",
	Usage: "List various entities in the system (workers, nodes, tasks)",
	Subcommands: []*cli.Command{
		{
			Name:  "nodes",
			Usage: "List all registered nodes",
			Action: func(c *cli.Context) error {
				resp, err := http.Get("http://localhost:5555/node")
				if err != nil {
					fmt.Println("Error listing nodes:", err)
					return err
				}
				defer resp.Body.Close()

				resp.Write(os.Stdout)

				return nil
			},
		},
		{
			Name:  "workers",
			Usage: "List all registered workers",
			Action: func(c *cli.Context) error {
				resp, err := http.Get("http://localhost:5555/worker")
				if err != nil {
					fmt.Println("Error listing nodes:", err)
					return err
				}
				defer resp.Body.Close()

				resp.Write(os.Stdout)

				return nil
			},
		},
		{
			Name:  "tasks",
			Usage: "List all tasks",
			Action: func(c *cli.Context) error {
				fmt.Println("Listing tasks...")
				// Add logic to list tasks
				return nil
			},
		},
	},
}
