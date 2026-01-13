package api

import (
	"github.com/google/uuid"
	"github.com/sjsanc/gorc/metrics"
)

// RegisterWorkerRequest is the request payload for registering a Worker with the Manager.
type RegisterWorkerRequest struct {
	WorkerID      string `json:"worker_id"`
	WorkerName    string `json:"worker_name"`
	WorkerAddress string `json:"worker_address"`
	WorkerPort    int    `json:"worker_port"`
}

// DeployTaskRequest is the request payload for deploying a task to a Worker.
type DeployTaskRequest struct {
	TaskID string   `json:"task_id"`
	Name   string   `json:"name"`
	Image  string   `json:"image"`
	Args   []string `json:"args,omitempty"`
}

// StopTaskRequest is the request payload for stopping a task on a Worker.
type StopTaskRequest struct {
	TaskID      string `json:"task_id"`
	ContainerID string `json:"container_id"`
}

// TaskStatusUpdateRequest is the request payload for updating task status from Worker to Manager.
type TaskStatusUpdateRequest struct {
	State       string `json:"state"`                 // "running", "completed", "failed"
	ContainerID string `json:"containerId,omitempty"` // Docker container ID
	Error       string `json:"error,omitempty"`       // Error message if failed
}

// NodeMetricsUpdateRequest is the request payload for updating node metrics from nodes to Manager.
type NodeMetricsUpdateRequest struct {
	Hostname string           `json:"hostname"`
	Metrics  *metrics.Metrics `json:"metrics"`
}

// DeployRequest is the request payload for deploying a new task via the Manager API.
type DeployRequest struct {
	Name  string   `json:"name"`
	Image string   `json:"image"`
	Args  []string `json:"args,omitempty"`
}

// WorkerInfo represents a Worker's information for API responses.
type WorkerInfo struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Port      int       `json:"port"`
	Heartbeat int64     `json:"last_heartbeat"` // Unix timestamp
}

// NodeInfo represents a Node's information for API responses.
type NodeInfo struct {
	Hostname    string `json:"hostname"`
	Address     string `json:"address"`
	ManagerPort int    `json:"manager_port"`
	WorkerPort  int    `json:"worker_port"`
}
