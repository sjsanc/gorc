package service

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// ServiceConfig is for single service deployments
type ServiceConfig struct {
	Service struct {
		Name          string   `toml:"name"`
		Image         string   `toml:"image"`
		Replicas      int      `toml:"replicas"`
		Cmd           []string `toml:"cmd"`
		RestartPolicy string   `toml:"restart_policy"`
	} `toml:"service"`
}

// AppConfig is for multi-service app deployments
type AppConfig struct {
	App struct {
		Name     string `toml:"name"`
		Services []struct {
			Name          string   `toml:"name"`
			Image         string   `toml:"image"`
			Replicas      int      `toml:"replicas"`
			Cmd           []string `toml:"cmd"`
			RestartPolicy string   `toml:"restart_policy"`
		} `toml:"services"`
	} `toml:"app"`
}

// App represents a collection of services
type App struct {
	Name     string
	Services []*Service
}

// NewApp creates a new App with the given name and services
func NewApp(name string, services []*Service) *App {
	return &App{
		Name:     name,
		Services: services,
	}
}

// ParseConfigFile parses a TOML config file and returns a Service object.
func ParseConfigFile(path string) (*Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	var cfg ServiceConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %v", err)
	}

	// Validation
	if cfg.Service.Name == "" || cfg.Service.Image == "" {
		return nil, fmt.Errorf("name and image are required")
	}
	if cfg.Service.Replicas < 1 {
		return nil, fmt.Errorf("replicas must be >= 1")
	}

	// Parse restart policy
	var restartPolicy RestartPolicy
	switch cfg.Service.RestartPolicy {
	case "always":
		restartPolicy = RestartAlways
	case "on-failure", "":
		restartPolicy = RestartOnFailure
	case "never":
		restartPolicy = RestartNever
	default:
		return nil, fmt.Errorf("invalid restart_policy: %s", cfg.Service.RestartPolicy)
	}

	return NewService(
		cfg.Service.Name,
		cfg.Service.Image,
		cfg.Service.Replicas,
		cfg.Service.Cmd,
		restartPolicy,
	), nil
}

// ParseAppConfigFile parses a TOML config file and returns an App object.
func ParseAppConfigFile(path string) (*App, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	var cfg AppConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %v", err)
	}

	// Validation
	if cfg.App.Name == "" {
		return nil, fmt.Errorf("app name is required")
	}
	if len(cfg.App.Services) == 0 {
		return nil, fmt.Errorf("at least one service is required")
	}

	// Parse services
	var services []*Service
	seenNames := make(map[string]bool)

	for _, svc := range cfg.App.Services {
		// Validate service
		if svc.Name == "" || svc.Image == "" {
			return nil, fmt.Errorf("service name and image are required")
		}
		if svc.Replicas < 1 {
			return nil, fmt.Errorf("service %s: replicas must be >= 1", svc.Name)
		}

		// Check for duplicate service names
		if seenNames[svc.Name] {
			return nil, fmt.Errorf("duplicate service name: %s", svc.Name)
		}
		seenNames[svc.Name] = true

		// Parse restart policy
		var restartPolicy RestartPolicy
		switch svc.RestartPolicy {
		case "always":
			restartPolicy = RestartAlways
		case "on-failure", "":
			restartPolicy = RestartOnFailure
		case "never":
			restartPolicy = RestartNever
		default:
			return nil, fmt.Errorf("invalid restart_policy for service %s: %s", svc.Name, svc.RestartPolicy)
		}

		services = append(services, NewServiceWithApp(
			svc.Name,
			svc.Image,
			svc.Replicas,
			svc.Cmd,
			restartPolicy,
			cfg.App.Name,
		))
	}

	return NewApp(cfg.App.Name, services), nil
}
