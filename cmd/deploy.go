package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sjsanc/gorc/api"
	"github.com/urfave/cli/v2"
)

// parseArgs parses a space-separated string of arguments, respecting quoted sections.
// Supports both single and double quotes.
// Returns an error for unclosed quotes or returns nil for empty input.
func parseArgs(argsStr string) ([]string, error) {
	if argsStr == "" {
		return nil, nil
	}

	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)
	wasQuoted := false

	for _, ch := range argsStr {
		switch {
		case ch == '"' || ch == '\'':
			if !inQuote {
				inQuote = true
				quoteChar = ch
				wasQuoted = true
			} else if ch == quoteChar {
				inQuote = false
				quoteChar = 0
			} else {
				current.WriteRune(ch)
			}
		case ch == ' ' && !inQuote:
			if current.Len() > 0 || wasQuoted {
				args = append(args, current.String())
				current.Reset()
				wasQuoted = false
			}
		default:
			current.WriteRune(ch)
		}
	}

	if inQuote {
		return nil, fmt.Errorf("unclosed quote in args string")
	}

	if current.Len() > 0 || wasQuoted {
		args = append(args, current.String())
	}

	return args, nil
}

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
			Name:  "args",
			Usage: "Command arguments for the container (space-separated, supports quotes)",
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
		argsStr := c.String("args")
		managerAddr := c.String("manager")

		// Generate default name if not provided
		if name == "" {
			name = fmt.Sprintf("task-%s", image)
		}

		// Parse args if provided
		var args []string
		if argsStr != "" {
			var err error
			args, err = parseArgs(argsStr)
			if err != nil {
				return fmt.Errorf("error parsing args: %v", err)
			}
		}

		// Create request payload using DeployRequest struct
		payload := api.DeployRequest{
			Name:  name,
			Image: image,
			Args:  args,
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
