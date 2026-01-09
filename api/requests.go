package api

// RegisterWorkerRequest is the request payload for registering a Worker with the Manager.
type RegisterWorkerRequest struct {
	WorkerID      string `json:"worker_id"`
	WorkerName    string `json:"worker_name"`
	WorkerAddress string `json:"worker_address"`
	WorkerPort    int    `json:"worker_port"`
}

// DeployTaskRequest is the request payload for deploying a task to a Worker.
type DeployTaskRequest struct {
	// `task_id` is the unique identifier for the task.
	TaskID string `json:"task_id"`
	Image  string `json:"image"`
}
