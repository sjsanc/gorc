package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/config"
	"github.com/sjsanc/gorc/metrics"
	"github.com/sjsanc/gorc/node"
	"github.com/sjsanc/gorc/replica"
	"github.com/sjsanc/gorc/runtime"
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
	events *utils.Queue[replica.Event]
	// `runtime` is the container runtime for executing replicas.
	runtime runtime.Runtime
	// `metricsCollector` collects system metrics for this worker node.
	metricsCollector metrics.Collector
	// `ctx` is the context for the Worker and its background goroutines.
	ctx context.Context
	// `cancel` cancels the Worker context.
	cancel context.CancelFunc
	// `wg` tracks all background goroutines for graceful shutdown.
	wg sync.WaitGroup
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

	collector, err := metrics.NewSystemCollector()
	if err != nil {
		return nil, fmt.Errorf("error creating metrics collector: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &Worker{
		id:               uuid.New(),
		logger:           logger,
		server:           nil,
		name:             hn,
		node:             n,
		managerAddr:      managerAddr,
		events:           utils.NewQueue[replica.Event](),
		runtime:          rt,
		metricsCollector: collector,
		ctx:              ctx,
		cancel:           cancel,
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
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.processEvents()
	}()

	// Start heartbeat loop in background
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.sendHeartbeats()
	}()

	if err := w.server.start(); err != nil {
		return fmt.Errorf("error starting server: %v", err)
	}

	// Block until context is cancelled
	<-w.ctx.Done()
	w.wg.Wait()
	return nil
}

// Stop gracefully shuts down the Worker and all background goroutines.
// It cancels the context to signal all goroutines to exit and waits for them to complete.
func (w *Worker) Stop() error {
	w.cancel()
	w.wg.Wait()
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

	w.logger.Debugf("Sending registration for worker %s", req.WorkerName)

	endpoint := fmt.Sprintf("http://%s/worker", w.managerAddr)
	b := bytes.NewBuffer(jsonData)
	resp, err := http.Post(endpoint, "application/json", b)
	if err != nil {
		return fmt.Errorf("error registering Worker with Manager: %v", err)
	}
	defer resp.Body.Close()

	w.logger.Debugf("Worker registration response: %s", resp.Status)

	return nil
}

// processEvents continuously processes events from the event queue.
func (w *Worker) processEvents() {
	w.logger.Info("Starting event processing loop")
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			event, ok := w.events.Dequeue()
			if !ok {
				time.Sleep(config.DefaultEventProcessingDelay)
				continue
			}

			w.logger.Infof("Processing event: %v for replica %s", event.Action, event.Replica.Name)

			switch event.Action {
			case replica.DeployEvent:
				w.handleDeploy(event.Replica)
			case replica.StopEvent:
				w.handleStop(event.Replica)
			}
		}
	}
}

// handleDeploy starts a container for the given replica.
func (w *Worker) handleDeploy(t *replica.Replica) {
	w.logger.Infof("Deploying replica: %s (image: %s)", t.Name, t.Image)

	containerID, err := w.runtime.Start(t.Image, t.ID.String(), t.Cmd)
	if err != nil {
		w.logger.Errorf("Failed to start replica %s: %v", t.Name, err)
		t.SetError(err.Error())
		// Report failure to manager
		w.reportReplicaStatus(t.ID, "failed", "", err.Error())
		return
	}

	w.logger.Infof("Replica %s started successfully (container: %s)", t.Name, containerID)
	t.MarkRunning(containerID)

	// Report running status to manager
	err = w.reportReplicaStatus(t.ID, "running", containerID, "")
	if err != nil {
		w.logger.Warnf("Failed to report running status for replica %s: %v", t.Name, err)
	}

	// Monitor container completion in background
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.monitorReplicaCompletion(t)
	}()
}

// monitorReplicaCompletion waits for a container to finish and reports the final status.
// It respects context cancellation to ensure graceful shutdown.
func (w *Worker) monitorReplicaCompletion(t *replica.Replica) {
	w.logger.Infof("Monitoring completion for replica: %s (container: %s)", t.Name, t.GetContainerID())

	// Create a channel to receive the exit code
	exitCodeChan := make(chan int64, 1)
	errChan := make(chan error, 1)

	// Run the blocking Wait call in a separate goroutine
	go func() {
		exitCode, err := w.runtime.Wait(t.ContainerID)
		if err != nil {
			errChan <- err
			return
		}
		exitCodeChan <- exitCode
	}()

	// Wait for either the container to finish or context cancellation
	select {
	case <-w.ctx.Done():
		w.logger.Infof("Replica monitoring cancelled for replica: %s", t.Name)
		return
	case err := <-errChan:
		w.logger.Errorf("Error waiting for replica %s: %v", t.Name, err)
		t.SetError(err.Error())
		w.reportReplicaStatus(t.ID, "failed", t.GetContainerID(), err.Error())
		return
	case exitCode := <-exitCodeChan:
		// Check exit code to determine if replica succeeded or failed
		if exitCode == 0 {
			w.logger.Infof("Replica %s completed successfully (exit code: %d)", t.Name, exitCode)
			t.MarkCompleted()
			w.reportReplicaStatus(t.ID, "completed", t.GetContainerID(), "")
		} else {
			errMsg := fmt.Sprintf("container exited with code %d", exitCode)
			w.logger.Errorf("Replica %s failed: %s", t.Name, errMsg)
			t.SetError(errMsg)
			w.reportReplicaStatus(t.ID, "failed", t.GetContainerID(), errMsg)
		}
	}
}

// handleStop stops a running container for the given replica.
func (w *Worker) handleStop(t *replica.Replica) {
	containerID := t.GetContainerID()
	w.logger.Infof("Stopping replica: %s (container: %s)", t.Name, containerID)

	if containerID == "" {
		w.logger.Warnf("Replica %s has no container ID, marking completed", t.Name)
		t.MarkCompleted()
		w.reportReplicaStatus(t.ID, "completed", "", "")
		return
	}

	err := w.runtime.Stop(containerID)
	if err != nil {
		// Check if container already gone (idempotent)
		if isContainerNotFoundError(err) {
			w.logger.Warnf("Container already removed for replica %s", t.Name)
			t.MarkCompleted()
			w.reportReplicaStatus(t.ID, "completed", containerID, "")
			return
		}

		w.logger.Errorf("Failed to stop replica %s: %v", t.Name, err)
		t.SetError(err.Error())
		w.reportReplicaStatus(t.ID, "failed", containerID, err.Error())
		return
	}

	w.logger.Infof("Replica %s stopped successfully", t.Name)
	t.MarkCompleted()
	w.reportReplicaStatus(t.ID, "completed", containerID, "")
}

// reportReplicaStatus sends a replica status update to the Manager via HTTP PUT request.
func (w *Worker) reportReplicaStatus(replicaID uuid.UUID, state, containerID, errMsg string) error {
	req := api.ReplicaStatusUpdateRequest{
		State:       state,
		ContainerID: containerID,
		Error:       errMsg,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshaling status update: %v", err)
	}

	endpoint := fmt.Sprintf("http://%s/replicas/%s/status", w.managerAddr, replicaID.String())
	httpReq, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: config.DefaultHTTPClientTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("error sending status update: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("manager rejected status update, status code: %d", resp.StatusCode)
	}

	w.logger.Debugf("Reported replica %s status to manager: %s", replicaID.String(), state)
	return nil
}

// sendHeartbeats sends periodic heartbeat messages to the Manager.
// Heartbeats are sent every 5 seconds to keep the worker alive and reachable.
func (w *Worker) sendHeartbeats() {
	ticker := time.NewTicker(config.DefaultHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			// Collect metrics
			m, err := w.metricsCollector.Collect()
			if err != nil {
				w.logger.Warnf("failed to collect metrics: %v", err)
			}

			// Send heartbeat
			endpoint := fmt.Sprintf("http://%s/worker/%s/heartbeat", w.managerAddr, w.id.String())
			httpReq, err := http.NewRequest("POST", endpoint, nil)
			if err != nil {
				w.logger.Errorf("error creating heartbeat request: %v", err)
				continue
			}

			client := &http.Client{Timeout: config.DefaultHeartbeatInterval}
			resp, err := client.Do(httpReq)
			if err != nil {
				w.logger.Warnf("failed to send heartbeat to manager: %v", err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				w.logger.Warnf("manager rejected heartbeat, status code: %d", resp.StatusCode)
			}

			// Push metrics if collection succeeded
			if m != nil {
				w.pushMetrics(m)
			}
		}
	}
}

func (w *Worker) pushMetrics(m *metrics.Metrics) {
	req := api.NodeMetricsUpdateRequest{
		Hostname: w.node.Hostname,
		Metrics:  m,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		w.logger.Errorf("error marshaling metrics: %v", err)
		return
	}

	endpoint := fmt.Sprintf("http://%s/node/%s/metrics", w.managerAddr, w.node.Hostname)
	httpReq, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		w.logger.Errorf("error creating metrics request: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: config.DefaultHTTPClientTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		w.logger.Warnf("failed to send metrics to manager: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.logger.Warnf("manager rejected metrics, status code: %d", resp.StatusCode)
	}
}

// isContainerNotFoundError checks if the error is due to container not being found.
func isContainerNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "No such container") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "no container with")
}
