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

// DeployReplicaRequest is the request payload for deploying a replica to a Worker.
type DeployReplicaRequest struct {
	ReplicaID string   `json:"replica_id"`
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Cmd       []string `json:"cmd,omitempty"`
}

// StopReplicaRequest is the request payload for stopping a replica on a Worker.
type StopReplicaRequest struct {
	ReplicaID   string `json:"replica_id"`
	ContainerID string `json:"container_id"`
}

// ReplicaStatusUpdateRequest is the request payload for updating replica status from Worker to Manager.
type ReplicaStatusUpdateRequest struct {
	State       string `json:"state"`                 // "running", "completed", "failed"
	ContainerID string `json:"containerId,omitempty"` // Docker container ID
	Error       string `json:"error,omitempty"`       // Error message if failed
}

// NodeMetricsUpdateRequest is the request payload for updating node metrics from nodes to Manager.
type NodeMetricsUpdateRequest struct {
	Hostname string           `json:"hostname"`
	Metrics  *metrics.Metrics `json:"metrics"`
}

// DeployRequest is the request payload for deploying a new replica via the Manager API.
type DeployRequest struct {
	Name  string   `json:"name"`
	Image string   `json:"image"`
	Cmd   []string `json:"cmd,omitempty"`
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

// CreateServiceRequest is the request payload for creating or updating a service.
type CreateServiceRequest struct {
	Name          string   `json:"name"`
	Image         string   `json:"image"`
	Replicas      int      `json:"replicas"`
	Cmd           []string `json:"cmd,omitempty"`
	RestartPolicy string   `json:"restart_policy"`
	AppName       string   `json:"app_name,omitempty"`
}

// UpdateServiceRequest is the request payload for updating an existing service.
type UpdateServiceRequest struct {
	Replicas int `json:"replicas"`
}

// ServiceInfo represents a service for API responses.
type ServiceInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Image         string `json:"image"`
	Replicas      int    `json:"replicas"`
	RestartPolicy string `json:"restart_policy"`
	AppName       string `json:"app_name,omitempty"`
}
