package service

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type RestartPolicy string

const (
	RestartAlways    RestartPolicy = "always"
	RestartOnFailure RestartPolicy = "on-failure"
	RestartNever     RestartPolicy = "never"
)

// Service represents a declarative deployment configuration (desired state).
type Service struct {
	mu            sync.RWMutex
	ID            uuid.UUID      // Unique service identifier
	Name          string         // User-friendly name (unique key)
	Image         string         // Container image
	Replicas      int            // Desired replica count
	Cmd           []string       // Override container CMD
	RestartPolicy RestartPolicy  // Restart behavior
	AppName       string         // Optional: app this service belongs to
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func NewService(name, image string, replicas int, cmd []string, restartPolicy RestartPolicy) *Service {
	return NewServiceWithApp(name, image, replicas, cmd, restartPolicy, "")
}

func NewServiceWithApp(name, image string, replicas int, cmd []string, restartPolicy RestartPolicy, appName string) *Service {
	if cmd == nil {
		cmd = []string{}
	}
	return &Service{
		ID:            uuid.New(),
		Name:          name,
		Image:         image,
		Replicas:      replicas,
		Cmd:           cmd,
		RestartPolicy: restartPolicy,
		AppName:       appName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// Clone creates a deep copy of the service.
func (s *Service) Clone() *Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cmdCopy := make([]string, len(s.Cmd))
	copy(cmdCopy, s.Cmd)
	return &Service{
		ID:            s.ID,
		Name:          s.Name,
		Image:         s.Image,
		Replicas:      s.Replicas,
		Cmd:           cmdCopy,
		RestartPolicy: s.RestartPolicy,
		AppName:       s.AppName,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}
