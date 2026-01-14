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
	Usage: "List various entities in the system (workers, nodes, services, replicas)",
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
					fmt.Println("Error listing workers:", err)
					return err
				}
				defer resp.Body.Close()

				resp.Write(os.Stdout)

				return nil
			},
		},
		{
			Name:  "services",
			Usage: "List all deployed services",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "manager",
					Usage: "Manager address (default: localhost:5555)",
					Value: "localhost:5555",
				},
			},
			Action: func(c *cli.Context) error {
				managerAddr := c.String("manager")
				endpoint := fmt.Sprintf("http://%s/services", managerAddr)

				resp, err := http.Get(endpoint)
				if err != nil {
					fmt.Println("Error listing services:", err)
					return err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					fmt.Printf("Error: received status code %d\n", resp.StatusCode)
					return fmt.Errorf("failed to list services")
				}

				// Parse response
				var services []map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&services)
				if err != nil {
					fmt.Println("Error decoding response:", err)
					return err
				}

				if len(services) == 0 {
					fmt.Println("No services found")
					return nil
				}

				// Print table header
				fmt.Printf("%-30s %-25s %-10s %-15s\n", "NAME", "IMAGE", "REPLICAS", "RESTART POLICY")
				fmt.Println("--------------------------------------------------------------------------------")

				// Print services
				for _, svc := range services {
					name := getStringField(svc, "Name")
					image := getStringField(svc, "Image")
					replicas := getIntField(svc, "Replicas")
					restartPolicy := getStringField(svc, "RestartPolicy")

					// Truncate long fields for display
					if len(image) > 25 {
						image = image[:22] + "..."
					}

					fmt.Printf("%-30s %-25s %-10d %-15s\n", name, image, replicas, restartPolicy)
				}

				return nil
			},
		},
		{
			Name:  "replicas",
			Usage: "List all running replicas",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "manager",
					Usage: "Manager address (default: localhost:5555)",
					Value: "localhost:5555",
				},
			},
			Action: func(c *cli.Context) error {
				managerAddr := c.String("manager")
				endpoint := fmt.Sprintf("http://%s/replicas", managerAddr)

				resp, err := http.Get(endpoint)
				if err != nil {
					fmt.Println("Error listing replicas:", err)
					return err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					fmt.Printf("Error: received status code %d\n", resp.StatusCode)
					return fmt.Errorf("failed to list replicas")
				}

				// Parse response
				var replicas []map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&replicas)
				if err != nil {
					fmt.Println("Error decoding response:", err)
					return err
				}

				if len(replicas) == 0 {
					fmt.Println("No replicas found")
					return nil
				}

				// Print table header
				fmt.Printf("%-30s %-20s %-3s %-20s %-12s %-10s\n", "NAME", "SERVICE", "REP", "IMAGE", "STATE", "WORKER")
				fmt.Println("-----------------------------------------------------------------------------------------------------")

				// Print replicas
				for _, replica := range replicas {
					name := getStringField(replica, "Name")
					serviceName := getStringField(replica, "ServiceName")
					replicaID := getIntField(replica, "ReplicaID")
					image := getStringField(replica, "Image")
					state := getStateString(replica)
					workerID := getStringField(replica, "WorkerID")

					// Truncate long fields for display
					if len(name) > 30 {
						name = name[:27] + "..."
					}
					if len(serviceName) == 0 {
						serviceName = "(standalone)"
					}
					if len(serviceName) > 20 {
						serviceName = serviceName[:17] + "..."
					}
					if len(image) > 12 {
						image = image[:9] + "..."
					}
					if len(workerID) > 10 {
						workerID = workerID[:7] + "..."
					}

					fmt.Printf("%-30s %-20s %-3d %-20s %-12s %-10s\n", name, serviceName, replicaID, image, state, workerID)
				}

				return nil
			},
		},
	},
}

// Helper function to safely extract string fields from replica map
func getStringField(replica map[string]interface{}, field string) string {
	if val, ok := replica[field]; ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// Helper function to safely extract int fields from map
func getIntField(item map[string]interface{}, field string) int {
	if val, ok := item[field]; ok && val != nil {
		if intVal, ok := val.(float64); ok {
			return int(intVal)
		}
	}
	return 0
}

// Helper function to convert replica state to string
func getStateString(replica map[string]interface{}) string {
	if state, ok := replica["State"]; ok && state != nil {
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
