package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/node"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/task"
	"github.com/sjsanc/gorc/utils"
	"go.uber.org/zap"
)

type Worker struct {
	logger *zap.SugaredLogger
	// `uuid` uniquely identifies the Worker within the Cluster.
	id uuid.UUID
	// `server` is the Worker API server.
	server *server
	// `name` is the Worker name. Defaults to the node hostname.
	name string
	// `node` is the Worker node context.
	node *node.Node
	// `managerAddr` is the address of the Manager server.
	managerAddr string
	// `events` is a queue of events to be processed by the Worker.
	events *utils.Queue[task.Event]
	// `runtime` is the container runtime for executing tasks.
	runtime runtime.Runtime
}

func NewWorker(logger *zap.SugaredLogger, addr string, port int, managerAddr string, runtimeType runtime.RuntimeType) (*Worker, error) {
	hn, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("error getting hostname: %v", err)
	}

	n := node.NewNode(hn, addr, port, 0)

	rt, err := runtime.NewRuntime(runtimeType)
	if err != nil {
		return nil, fmt.Errorf("error creating runtime: %v", err)
	}

	w := &Worker{
		id:          uuid.New(),
		logger:      logger,
		server:      nil,
		name:        hn,
		node:        n,
		managerAddr: managerAddr,
		events:      utils.NewQueue[task.Event](),
		runtime:     rt,
	}

	w.server = newServer(w, addr, port)

	w.server.initRouter()

	return w, nil
}

func (w *Worker) Run() error {
	err := w.registerWithManager()
	if err != nil {
		return fmt.Errorf("error registering Worker with Manager: %v", err)
	}

	// Start event processing loop in background
	go w.processEvents()

	// Start heartbeat loop in background
	go w.sendHeartbeats()

	w.server.start()

	return nil
}

// Stop closes the Worker API server.
func (w *Worker) Stop() error {
	return w.server.stop()
}

// registerSelfWithManager registers the Worker with the Manager via an HTTP POST request.
func (w *Worker) registerWithManager() error {
	req := api.RegisterWorkerRequest{
		WorkerID:      w.id.String(),
		WorkerName:    w.name,
		WorkerAddress: w.node.Address,
		WorkerPort:    w.node.WorkerPort,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshaling JSON: %v", err)
	}

	println(jsonData)

	endpoint := fmt.Sprintf("http://%s/worker", w.managerAddr)
	b := bytes.NewBuffer(jsonData)
	resp, err := http.Post(endpoint, "application/json", b)
	if err != nil {
		return fmt.Errorf("error registering Worker with Manager: %v", err)
	}
	defer resp.Body.Close()

	fmt.Println(resp)

	return nil
}

// processEvents continuously processes events from the event queue.
func (w *Worker) processEvents() {
	w.logger.Info("Starting event processing loop")
	for {
		event, ok := w.events.Dequeue()
		if !ok {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		w.logger.Infof("Processing event: %v for task %s", event.Action, event.Task.Name)

		switch event.Action {
		case task.DeployEvent:
			w.handleDeploy(event.Task)
		case task.StopEvent:
			w.handleStop(event.Task)
		}
	}
}

// handleDeploy starts a container for the given task.
func (w *Worker) handleDeploy(t *task.Task) {
	w.logger.Infof("Deploying task: %s (image: %s)", t.Name, t.Image)

	containerID, err := w.runtime.Start(t.Image, t.ID.String())
	if err != nil {
		w.logger.Errorf("Failed to start task %s: %v", t.Name, err)
		t.State = task.TaskFailed
		t.Error = err.Error()
		t.FinishedAt = time.Now()
		// Report failure to manager
		w.reportTaskStatus(t.ID, "failed", "", err.Error())
		return
	}

	w.logger.Infof("Task %s started successfully (container: %s)", t.Name, containerID)
	t.ContainerID = containerID
	t.State = task.TaskRunning
	t.StartedAt = time.Now()

	// Report running status to manager
	err = w.reportTaskStatus(t.ID, "running", containerID, "")
	if err != nil {
		w.logger.Warnf("Failed to report running status for task %s: %v", t.Name, err)
	}

	// Monitor container completion in background
	go w.monitorTaskCompletion(t)
}

// monitorTaskCompletion waits for a container to finish and reports the final status.
func (w *Worker) monitorTaskCompletion(t *task.Task) {
	w.logger.Infof("Monitoring completion for task: %s (container: %s)", t.Name, t.ContainerID)

	exitCode, err := w.runtime.Wait(t.ContainerID)
	if err != nil {
		w.logger.Errorf("Error waiting for task %s: %v", t.Name, err)
		t.State = task.TaskFailed
		t.Error = err.Error()
		t.FinishedAt = time.Now()
		w.reportTaskStatus(t.ID, "failed", t.ContainerID, err.Error())
		return
	}

	// Check exit code to determine if task succeeded or failed
	if exitCode == 0 {
		w.logger.Infof("Task %s completed successfully (exit code: %d)", t.Name, exitCode)
		t.State = task.TaskCompleted
		t.FinishedAt = time.Now()
		w.reportTaskStatus(t.ID, "completed", t.ContainerID, "")
	} else {
		errMsg := fmt.Sprintf("container exited with code %d", exitCode)
		w.logger.Errorf("Task %s failed: %s", t.Name, errMsg)
		t.State = task.TaskFailed
		t.Error = errMsg
		t.FinishedAt = time.Now()
		w.reportTaskStatus(t.ID, "failed", t.ContainerID, errMsg)
	}
}

// handleStop stops a running container for the given task.
func (w *Worker) handleStop(t *task.Task) {
	w.logger.Infof("Stopping task: %s (container: %s)", t.Name, t.ContainerID)

	if t.ContainerID == "" {
		w.logger.Warnf("Task %s has no container ID, skipping stop", t.Name)
		return
	}

	err := w.runtime.Stop(t.ContainerID)
	if err != nil {
		w.logger.Errorf("Failed to stop task %s: %v", t.Name, err)
		t.State = task.TaskFailed
		t.Error = err.Error()
		return
	}

	w.logger.Infof("Task %s stopped successfully", t.Name)
	t.State = task.TaskCompleted
	t.FinishedAt = time.Now()
}

// reportTaskStatus sends a task status update to the Manager via HTTP PUT request.
func (w *Worker) reportTaskStatus(taskID uuid.UUID, state, containerID, errMsg string) error {
	req := api.TaskStatusUpdateRequest{
		State:       state,
		ContainerID: containerID,
		Error:       errMsg,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshaling status update: %v", err)
	}

	endpoint := fmt.Sprintf("http://%s/tasks/%s/status", w.managerAddr, taskID.String())
	httpReq, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error sending status update: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("manager rejected status update, status code: %d", resp.StatusCode)
	}

	w.logger.Infof("Reported task %s status to manager: %s", taskID.String(), state)
	return nil
}

// sendHeartbeats sends periodic heartbeat messages to the Manager.
// Heartbeats are sent every 5 seconds to keep the worker alive and reachable.
func (w *Worker) sendHeartbeats() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		endpoint := fmt.Sprintf("http://%s/worker/%s/heartbeat", w.managerAddr, w.id.String())
		httpReq, err := http.NewRequest("POST", endpoint, nil)
		if err != nil {
			w.logger.Errorf("error creating heartbeat request: %v", err)
			continue
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			w.logger.Warnf("failed to send heartbeat to manager: %v", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			w.logger.Warnf("manager rejected heartbeat, status code: %d", resp.StatusCode)
		}
	}
}
