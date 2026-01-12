package cmd

import (
	"encoding/json"
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
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "manager",
					Usage: "Manager address (default: localhost:5555)",
					Value: "localhost:5555",
				},
			},
			Action: func(c *cli.Context) error {
				managerAddr := c.String("manager")
				endpoint := fmt.Sprintf("http://%s/tasks", managerAddr)

				resp, err := http.Get(endpoint)
				if err != nil {
					fmt.Println("Error listing tasks:", err)
					return err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					fmt.Printf("Error: received status code %d\n", resp.StatusCode)
					return fmt.Errorf("failed to list tasks")
				}

				// Parse response
				var tasks []map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&tasks)
				if err != nil {
					fmt.Println("Error decoding response:", err)
					return err
				}

				if len(tasks) == 0 {
					fmt.Println("No tasks found")
					return nil
				}

				// Print table header
				fmt.Printf("%-36s %-20s %-20s %-10s %-36s\n", "TASK ID", "NAME", "IMAGE", "STATE", "WORKER ID")
				fmt.Println("------------------------------------------------------------------------------------------------------------------------------------------------")

				// Print tasks
				for _, task := range tasks {
					taskID := getStringField(task, "ID")
					name := getStringField(task, "Name")
					image := getStringField(task, "Image")
					state := getStateString(task)
					workerID := getStringField(task, "WorkerID")

					// Truncate long fields for display
					if len(name) > 20 {
						name = name[:17] + "..."
					}
					if len(image) > 20 {
						image = image[:17] + "..."
					}

					fmt.Printf("%-36s %-20s %-20s %-10s %-36s\n", taskID, name, image, state, workerID)
				}

				return nil
			},
		},
	},
}

// Helper function to safely extract string fields from task map
func getStringField(task map[string]interface{}, field string) string {
	if val, ok := task[field]; ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// Helper function to convert task state to string
func getStateString(task map[string]interface{}) string {
	if state, ok := task["State"]; ok && state != nil {
		if stateNum, ok := state.(float64); ok {
			switch int(stateNum) {
			case 0:
				return "pending"
			case 1:
				return "running"
			case 2:
				return "completed"
			case 3:
				return "failed"
			default:
				return "unknown"
			}
		}
	}
	return "unknown"
}
