package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/service"
	"github.com/urfave/cli/v2"
)

var appCmd = &cli.Command{
	Name:  "app",
	Usage: "Manage applications (groups of services)",
	Subcommands: []*cli.Command{
		appDeployCmd,
		appStopCmd,
		appRestartCmd,
		appDeleteCmd,
	},
}

var appDeployCmd = &cli.Command{
	Name:      "deploy",
	Usage:     "Deploy an app from a TOML config file",
	ArgsUsage: "<config.toml>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "manager",
			Usage: "Manager address (default: localhost:5555)",
			Value: "localhost:5555",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() < 1 {
			return fmt.Errorf("config file path is required")
		}

		configFile := c.Args().Get(0)
		managerAddr := c.String("manager")

		// Parse app config
		app, err := service.ParseAppConfigFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to parse app config: %v", err)
		}

		// Deploy each service in the app
		fmt.Printf("Deploying app '%s' with %d service(s)...\n", app.Name, len(app.Services))

		for _, svc := range app.Services {
			req := api.CreateServiceRequest{
				Name:          svc.Name,
				Image:         svc.Image,
				Replicas:      svc.Replicas,
				Cmd:           svc.Cmd,
				RestartPolicy: string(svc.RestartPolicy),
				AppName:       app.Name,
			}

			jsonData, err := json.Marshal(req)
			if err != nil {
				return fmt.Errorf("error marshaling request for service %s: %v", svc.Name, err)
			}

			endpoint := fmt.Sprintf("http://%s/services", managerAddr)
			resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				return fmt.Errorf("error deploying service %s: %v", svc.Name, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("deployment of service %s failed (status %d): %s", svc.Name, resp.StatusCode, string(body))
			}

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			if err != nil {
				return fmt.Errorf("error parsing response for service %s: %v", svc.Name, err)
			}

			fmt.Printf("  ✓ Service '%s' deployed (ID: %v, replicas: %d)\n", svc.Name, result["service_id"], svc.Replicas)
		}

		fmt.Printf("\nApp '%s' deployed successfully!\n", app.Name)
		return nil
	},
}

var appStopCmd = &cli.Command{
	Name:      "stop",
	Usage:     "Stop an app by stopping all running replicas (preserves configuration for restart)",
	ArgsUsage: "<app-name>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "manager",
			Usage: "Manager address (default: localhost:5555)",
			Value: "localhost:5555",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() < 1 {
			return fmt.Errorf("app name is required")
		}

		appName := c.Args().Get(0)
		managerAddr := c.String("manager")

		// Get services for this app
		endpoint := fmt.Sprintf("http://%s/apps/%s/services", managerAddr, appName)
		resp, err := http.Get(endpoint)
		if err != nil {
			return fmt.Errorf("error contacting manager: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to get app services (status %d): %s", resp.StatusCode, string(body))
		}

		var services []*service.Service
		err = json.NewDecoder(resp.Body).Decode(&services)
		if err != nil {
			return fmt.Errorf("error parsing services: %v", err)
		}

		if len(services) == 0 {
			return fmt.Errorf("no services found for app '%s'", appName)
		}

		fmt.Printf("Stopping app '%s' (%d service(s))...\n", appName, len(services))

		// Mark all services as stopped to prevent reconciliation from restarting replicas
		stoppedState := "stopped"
		for _, svc := range services {
			updateReq := api.UpdateServiceRequest{
				State: &stoppedState,
			}

			jsonData, err := json.Marshal(updateReq)
			if err != nil {
				fmt.Printf("  ✗ Error marshaling request for service '%s': %v\n", svc.Name, err)
				continue
			}

			updateEndpoint := fmt.Sprintf("http://%s/services/%s", managerAddr, svc.ID.String())
			req, err := http.NewRequest("PUT", updateEndpoint, bytes.NewBuffer(jsonData))
			if err != nil {
				fmt.Printf("  ✗ Error creating request for service '%s': %v\n", svc.Name, err)
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("  ✗ Error stopping service '%s': %v\n", svc.Name, err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("  ✗ Failed to stop service '%s' (status %d): %s\n", svc.Name, resp.StatusCode, string(body))
			}
		}

		// Get all replicas and stop them
		var replicasToStop []string
		for _, svc := range services {
			replicasEndpoint := fmt.Sprintf("http://%s/services/%s/replicas", managerAddr, svc.ID.String())
			replicasResp, err := http.Get(replicasEndpoint)
			if err != nil {
				fmt.Printf("  ✗ Error fetching replicas for service '%s': %v\n", svc.Name, err)
				continue
			}
			defer replicasResp.Body.Close()

			if replicasResp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(replicasResp.Body)
				fmt.Printf("  ✗ Failed to get replicas for service '%s' (status %d): %s\n", svc.Name, replicasResp.StatusCode, string(body))
				continue
			}

			type replicaResponse struct {
				ID    string `json:"ID"`
				State int    `json:"State"`
			}
			var replicas []replicaResponse
			err = json.NewDecoder(replicasResp.Body).Decode(&replicas)
			if err != nil {
				fmt.Printf("  ✗ Error parsing replicas for service '%s': %v\n", svc.Name, err)
				continue
			}

			// Collect running/pending replicas to stop
			for _, r := range replicas {
				if r.State == 0 || r.State == 1 {
					replicasToStop = append(replicasToStop, r.ID)
				}
			}
		}

		if len(replicasToStop) > 0 {
			// Stop replicas in parallel
			var wg sync.WaitGroup
			errChan := make(chan error, len(replicasToStop))

			for _, replicaID := range replicasToStop {
				wg.Add(1)
				go func(id string) {
					defer wg.Done()

					stopEndpoint := fmt.Sprintf("http://%s/replicas/%s", managerAddr, id)
					req, err := http.NewRequest("DELETE", stopEndpoint, nil)
					if err != nil {
						errChan <- fmt.Errorf("error creating request for replica %s: %v", id, err)
						return
					}

					client := &http.Client{}
					resp, err := client.Do(req)
					if err != nil {
						errChan <- fmt.Errorf("error stopping replica %s: %v", id, err)
						return
					}
					defer resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						body, _ := io.ReadAll(resp.Body)
						errChan <- fmt.Errorf("failed to stop replica %s (status %d): %s", id, resp.StatusCode, string(body))
						return
					}
				}(replicaID)
			}

			wg.Wait()
			close(errChan)

			// Collect errors
			var failedCount int
			for err := range errChan {
				fmt.Printf("  ✗ %v\n", err)
				failedCount++
			}

			if failedCount == 0 {
				fmt.Printf("  ✓ Stopped %d replica(s)\n", len(replicasToStop))
			} else {
				fmt.Printf("  ✓ Stopped %d/%d replicas (%d failed)\n", len(replicasToStop)-failedCount, len(replicasToStop), failedCount)
			}
		}

		fmt.Printf("\nApp '%s' stopped successfully (service configurations preserved for restart)\n", appName)
		return nil
	},
}

var appRestartCmd = &cli.Command{
	Name:      "restart",
	Usage:     "Restart an app (restart running replicas or scale up if stopped)",
	ArgsUsage: "<app-name>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "manager",
			Usage: "Manager address (default: localhost:5555)",
			Value: "localhost:5555",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() < 1 {
			return fmt.Errorf("app name is required")
		}

		appName := c.Args().Get(0)
		managerAddr := c.String("manager")

		// Get services for this app
		endpoint := fmt.Sprintf("http://%s/apps/%s/services", managerAddr, appName)
		resp, err := http.Get(endpoint)
		if err != nil {
			return fmt.Errorf("error contacting manager: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to get app services (status %d): %s", resp.StatusCode, string(body))
		}

		var services []*service.Service
		err = json.NewDecoder(resp.Body).Decode(&services)
		if err != nil {
			return fmt.Errorf("error parsing services: %v", err)
		}

		if len(services) == 0 {
			fmt.Printf("No services found for app '%s'\n", appName)
			return nil
		}

		// Collect all running replicas and check which services are stopped
		type replicaInfo struct {
			id          string
			serviceName string
		}
		type serviceScaleInfo struct {
			service *service.Service
			running int
		}

		var allReplicas []replicaInfo
		serviceMap := make(map[string]*serviceScaleInfo)

		for _, svc := range services {
			serviceMap[svc.ID.String()] = &serviceScaleInfo{
				service: svc,
				running: 0,
			}

			replicasEndpoint := fmt.Sprintf("http://%s/services/%s/replicas", managerAddr, svc.ID.String())
			replicasResp, err := http.Get(replicasEndpoint)
			if err != nil {
				fmt.Printf("  ✗ Error fetching replicas for service '%s': %v\n", svc.Name, err)
				continue
			}
			defer replicasResp.Body.Close()

			if replicasResp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(replicasResp.Body)
				fmt.Printf("  ✗ Failed to get replicas for service '%s' (status %d): %s\n", svc.Name, replicasResp.StatusCode, string(body))
				continue
			}

			type replicaResponse struct {
				ID    string `json:"ID"`
				State int    `json:"State"`
			}
			var replicas []replicaResponse
			err = json.NewDecoder(replicasResp.Body).Decode(&replicas)
			if err != nil {
				fmt.Printf("  ✗ Error parsing replicas for service '%s': %v\n", svc.Name, err)
				continue
			}

			// Filter for Running (1) or Pending (0) replicas
			for _, r := range replicas {
				if r.State == 0 || r.State == 1 {
					allReplicas = append(allReplicas, replicaInfo{
						id:          r.ID,
						serviceName: svc.Name,
					})
					serviceMap[svc.ID.String()].running++
				}
			}
		}

		// Determine if app is running or stopped
		appRunning := len(allReplicas) > 0

		if appRunning {
			// App is running: restart existing replicas in parallel
			fmt.Printf("Restarting app '%s' (%d replica(s))...\n", appName, len(allReplicas))

			var wg sync.WaitGroup
			errChan := make(chan error, len(allReplicas))

			for _, r := range allReplicas {
				wg.Add(1)
				go func(replicaID string, serviceName string) {
					defer wg.Done()

					restartEndpoint := fmt.Sprintf("http://%s/replicas/%s/restart", managerAddr, replicaID)
					req, err := http.NewRequest("POST", restartEndpoint, nil)
					if err != nil {
						errChan <- fmt.Errorf("error creating request for replica %s (service %s): %v", replicaID, serviceName, err)
						return
					}

					client := &http.Client{}
					resp, err := client.Do(req)
					if err != nil {
						errChan <- fmt.Errorf("error restarting replica %s (service %s): %v", replicaID, serviceName, err)
						return
					}
					defer resp.Body.Close()

					if resp.StatusCode != http.StatusAccepted {
						body, _ := io.ReadAll(resp.Body)
						errChan <- fmt.Errorf("failed to restart replica %s (service %s, status %d): %s", replicaID, serviceName, resp.StatusCode, string(body))
						return
					}
				}(r.id, r.serviceName)
			}

			wg.Wait()
			close(errChan)

			// Collect errors
			var failedCount int
			for err := range errChan {
				fmt.Printf("  ✗ %v\n", err)
				failedCount++
			}

			if failedCount == 0 {
				fmt.Printf("\nRestarted %d replicas across %d service(s) for app '%s'\n", len(allReplicas), len(services), appName)
			} else if failedCount == len(allReplicas) {
				return fmt.Errorf("failed to restart all replicas: %d errors", failedCount)
			} else {
				fmt.Printf("\nRestarted %d/%d replicas (%d failed) for app '%s'\n", len(allReplicas)-failedCount, len(allReplicas), failedCount, appName)
			}
		} else {
			// App is stopped: scale up services that have desired replicas > 0
			servicesToScale := 0
			for _, svc := range services {
				if svc.Replicas > 0 {
					servicesToScale++
				}
			}

			if servicesToScale == 0 {
				fmt.Printf("App '%s' has no services with replicas to restart\n", appName)
				return nil
			}

			fmt.Printf("Starting app '%s' (%d service(s))...\n", appName, servicesToScale)

			var wg sync.WaitGroup
			errChan := make(chan error, servicesToScale)

			for _, svc := range services {
				if svc.Replicas > 0 {
					wg.Add(1)
					go func(svc *service.Service) {
						defer wg.Done()

						runningState := "running"
						updateReq := api.UpdateServiceRequest{
							Replicas: svc.Replicas,
							State:    &runningState,
						}

						jsonData, err := json.Marshal(updateReq)
						if err != nil {
							errChan <- fmt.Errorf("error marshaling request for service %s: %v", svc.Name, err)
							return
						}

						updateEndpoint := fmt.Sprintf("http://%s/services/%s", managerAddr, svc.ID.String())
						req, err := http.NewRequest("PUT", updateEndpoint, bytes.NewBuffer(jsonData))
						if err != nil {
							errChan <- fmt.Errorf("error creating request for service %s: %v", svc.Name, err)
							return
						}
						req.Header.Set("Content-Type", "application/json")

						client := &http.Client{}
						resp, err := client.Do(req)
						if err != nil {
							errChan <- fmt.Errorf("error updating service %s: %v", svc.Name, err)
							return
						}
						defer resp.Body.Close()

						if resp.StatusCode != http.StatusOK {
							body, _ := io.ReadAll(resp.Body)
							errChan <- fmt.Errorf("failed to scale service %s (status %d): %s", svc.Name, resp.StatusCode, string(body))
							return
						}
					}(svc)
				}
			}

			wg.Wait()
			close(errChan)

			// Collect errors
			var failedCount int
			for err := range errChan {
				fmt.Printf("  ✗ %v\n", err)
				failedCount++
			}

			if failedCount == 0 {
				fmt.Printf("\nStarted app '%s' (%d service(s))\n", appName, servicesToScale)
			} else if failedCount == servicesToScale {
				return fmt.Errorf("failed to start all services: %d errors", failedCount)
			} else {
				fmt.Printf("\nStarted %d/%d services (%d failed) for app '%s'\n", servicesToScale-failedCount, servicesToScale, failedCount, appName)
			}
		}

		return nil
	},
}

var appDeleteCmd = &cli.Command{
	Name:      "delete",
	Usage:     "Delete an app and all its services and replicas",
	ArgsUsage: "<app-name>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "manager",
			Usage: "Manager address (default: localhost:5555)",
			Value: "localhost:5555",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NArg() < 1 {
			return fmt.Errorf("app name is required")
		}

		appName := c.Args().Get(0)
		managerAddr := c.String("manager")

		endpoint := fmt.Sprintf("http://%s/apps/%s", managerAddr, appName)
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
			return fmt.Errorf("failed to delete app (status %d): %s", resp.StatusCode, string(body))
		}

		fmt.Printf("App '%s' deleted successfully!\n", appName)
		return nil
	},
}
