package cmd

import (
	"fmt"
	"io"
	"net/http"

	"github.com/urfave/cli/v2"
)

var stopCmd = &cli.Command{
	Name:  "stop",
	Usage: "Stop a running task",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "task-id",
			Usage:    "ID of the task to stop",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "manager",
			Usage: "Manager address (default: localhost:5555)",
			Value: "localhost:5555",
		},
	},
	Action: func(c *cli.Context) error {
		taskID := c.String("task-id")
		managerAddr := c.String("manager")

		endpoint := fmt.Sprintf("http://%s/tasks/%s", managerAddr, taskID)

		req, err := http.NewRequest("DELETE", endpoint, nil)
		if err != nil {
			return fmt.Errorf("error creating request: %v", err)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error contacting manager: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to stop task (status %d): %s", resp.StatusCode, string(body))
		}

		fmt.Printf("Task stopped successfully!\n")
		fmt.Printf("  Task ID: %s\n", taskID)
		fmt.Printf("  Status: stopping\n")

		return nil
	},
}
