package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/urfave/cli/v2"
)

var deployCmd = &cli.Command{
	Name:  "deploy",
	Usage: "Deploy a container task to the cluster",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Usage:    "Container image to deploy",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "name",
			Usage: "Name for the task (optional, will be auto-generated if not provided)",
		},
		&cli.StringFlag{
			Name:  "manager",
			Usage: "Manager address (default: localhost:5555)",
			Value: "localhost:5555",
		},
	},
	Action: func(c *cli.Context) error {
		image := c.String("image")
		name := c.String("name")
		managerAddr := c.String("manager")

		// Generate default name if not provided
		if name == "" {
			name = fmt.Sprintf("task-%s", image)
		}

		// Create request payload
		payload := map[string]string{
			"name":  name,
			"image": image,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("error marshaling request: %v", err)
		}

		// Send POST request to manager
		endpoint := fmt.Sprintf("http://%s/tasks", managerAddr)
		resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("error deploying task: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("deployment failed, status code: %d", resp.StatusCode)
		}

		// Parse response
		var result map[string]string
		err = json.NewDecoder(resp.Body).Decode(&result)
		if err != nil {
			return fmt.Errorf("error parsing response: %v", err)
		}

		fmt.Printf("Task deployed successfully!\n")
		fmt.Printf("  Task ID: %s\n", result["task_id"])
		fmt.Printf("  Name: %s\n", result["name"])
		fmt.Printf("  Status: %s\n", result["status"])

		return nil
	},
}
