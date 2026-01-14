package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
	Usage:     "Stop an app by scaling all its services to 0 replicas",
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

		// Scale each service to 0 replicas
		for _, svc := range services {
			updateReq := api.UpdateServiceRequest{
				Replicas: 0,
			}

			jsonData, err := json.Marshal(updateReq)
			if err != nil {
				return fmt.Errorf("error marshaling request for service %s: %v", svc.Name, err)
			}

			updateEndpoint := fmt.Sprintf("http://%s/services/%s", managerAddr, svc.ID.String())
			req, err := http.NewRequest("PUT", updateEndpoint, bytes.NewBuffer(jsonData))
			if err != nil {
				return fmt.Errorf("error creating request for service %s: %v", svc.Name, err)
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("error updating service %s: %v", svc.Name, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("failed to stop service %s (status %d): %s", svc.Name, resp.StatusCode, string(body))
			}

			fmt.Printf("  ✓ Service '%s' scaled to 0 replicas\n", svc.Name)
		}

		fmt.Printf("\nApp '%s' stopped successfully!\n", appName)
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
