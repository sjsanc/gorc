package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/node"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/storage"
	"github.com/sjsanc/gorc/task"
	"go.uber.org/zap"
)

// `managedWorker` represents the Manager's view of a Worker within the Cluster.
type ManagedWorker struct {
	ID            uuid.UUID
	Name          string
	Address       string
	Port          int
	Node          *node.Node
	LastHeartbeat time.Time
}

type Manager struct {
	// `logger` is the Manager logger.
	logger *zap.SugaredLogger
	// `server` is the Manager API server.
	server *server
	// `node` is the current node context for the Manager.
	node *node.Node
	// `nodes` is a store of all registered nodes, keyed by `hostname`.
	nodes storage.Store[node.Node]
	// `workers` is a store of all registered workers, keyed by `id`.
	workers storage.Store[ManagedWorker]
	// `tasks` is a store of all tasks, keyed by task `id`.
	tasks storage.Store[task.Task]
	// `ctx` is the context for the Manager and its background goroutines.
	ctx context.Context
	// `cancel` cancels the Manager context.
	cancel context.CancelFunc
}

func NewManager(logger *zap.SugaredLogger, addr string, port int, storageType storage.StorageType, runtimeType runtime.RuntimeType) (*Manager, error) {
	// Create the Manager Node
	hn, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("error getting hostname: %v", err)
	}

	n := node.NewNode(hn, addr, 0, port)

	// Create stores with error handling
	nodesStore, err := storage.NewStore[node.Node](storageType)
	if err != nil {
		return nil, fmt.Errorf("error creating nodes store: %v", err)
	}

	workersStore, err := storage.NewStore[ManagedWorker](storageType)
	if err != nil {
		return nil, fmt.Errorf("error creating workers store: %v", err)
	}

	tasksStore, err := storage.NewStore[task.Task](storageType)
	if err != nil {
		return nil, fmt.Errorf("error creating tasks store: %v", err)
	}

	// Create a new Manager instance
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		logger:  logger,
		server:  nil,
		node:    n,
		nodes:   nodesStore,
		workers: workersStore,
		tasks:   tasksStore,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Register the Manager node
	_, err = m.registerNode(n)
	if err != nil {
		return nil, fmt.Errorf("error registering manager node: %v", err)
	}

	// Create a new Manager server
	m.server = newServer(m, addr, port)

	// Initialise server routes
	m.server.initRouter()

	return m, nil
}

// Run starts the Manager API server and health checking for workers.
// This method blocks until Stop() is called.
func (m *Manager) Run() error {
	// Start detecting dead workers: timeout 30 seconds, check every 10 seconds
	m.detectDeadWorkers(30*time.Second, 10*time.Second)

	if err := m.server.start(); err != nil {
		return fmt.Errorf("error starting server: %v", err)
	}

	// Block until context is cancelled
	<-m.ctx.Done()
	return nil
}

// Stop gracefully shuts down the Manager and all background goroutines.
func (m *Manager) Stop() error {
	m.cancel()
	return m.server.stop()
}

func (m *Manager) registerNode(node *node.Node) (*node.Node, error) {
	existingNode, err := m.nodes.Get(node.Hostname)
	if err != nil {
		return nil, fmt.Errorf("error getting node storage: %v", err)
	}

	// If the node already exists, return it
	if existingNode != nil {
		return existingNode, nil
	}

	err = m.nodes.Put(node.Hostname, node)
	if err != nil {
		return nil, fmt.Errorf("error registering node: %v", err)
	}

	fmt.Println("Node registered:", node.Hostname)

	return node, nil
}

func (m *Manager) listNodes() ([]*node.Node, error) {
	nodes, err := m.nodes.List()
	if err != nil {
		return nil, fmt.Errorf("error listing nodes: %v", err)
	}
	return nodes, nil
}

func (m *Manager) registerWorker(req api.RegisterWorkerRequest) (*ManagedWorker, error) {
	id, err := uuid.Parse(req.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("error parsing Worker ID: %v", err)
	}

	// Use localhost if worker registers with 0.0.0.0 (local worker)
	address := req.WorkerAddress
	if address == "0.0.0.0" || address == "" {
		address = "127.0.0.1"
	}

	mw := &ManagedWorker{
		ID:            id,
		Name:          req.WorkerName,
		Address:       address,
		Port:          req.WorkerPort,
		Node:          nil, // Node registration handled separately if needed
		LastHeartbeat: time.Now(),
	}

	err = m.workers.Put(id.String(), mw)
	if err != nil {
		return nil, fmt.Errorf("error registering Worker: %v", err)
	}

	m.logger.Infof("Worker registered: %s at %s:%d", req.WorkerName, address, req.WorkerPort)

	return mw, nil
}

func (m *Manager) listWorkers() ([]*ManagedWorker, error) {
	workers, err := m.workers.List()
	if err != nil {
		return nil, fmt.Errorf("error listing workers: %v", err)
	}
	return workers, nil
}

func (m *Manager) createTask(name, image string) (*task.Task, error) {
	t := task.NewTask(name, image)
	err := m.tasks.Put(t.ID.String(), t)
	if err != nil {
		return nil, fmt.Errorf("error creating task: %v", err)
	}
	m.logger.Infof("Task created: %s (%s)", t.Name, t.ID.String())
	return t, nil
}

func (m *Manager) getTask(id uuid.UUID) (*task.Task, error) {
	t, err := m.tasks.Get(id.String())
	if err != nil {
		return nil, fmt.Errorf("error getting task: %v", err)
	}
	if t == nil {
		return nil, fmt.Errorf("task not found: %s", id.String())
	}
	return t, nil
}

func (m *Manager) updateTaskState(id uuid.UUID, state task.TaskState) error {
	t, err := m.getTask(id)
	if err != nil {
		return err
	}
	t.SetState(state)
	err = m.tasks.Put(id.String(), t)
	if err != nil {
		return fmt.Errorf("error updating task state: %v", err)
	}
	m.logger.Infof("Task state updated: %s -> %v", t.Name, state)
	return nil
}

func (m *Manager) updateTaskStatus(taskIDStr string, req api.TaskStatusUpdateRequest) error {
	id, err := uuid.Parse(taskIDStr)
	if err != nil {
		return fmt.Errorf("invalid task ID: %v", err)
	}

	t, err := m.getTask(id)
	if err != nil {
		return err
	}

	// Update task state based on request
	switch req.State {
	case "running":
		t.SetState(task.TaskRunning)
	case "completed":
		t.MarkCompleted()
	case "failed":
		t.SetError(req.Error)
	default:
		return fmt.Errorf("invalid task state: %s", req.State)
	}

	// Update container ID if provided
	if req.ContainerID != "" {
		t.SetContainerID(req.ContainerID)
	}

	// Save updated task
	err = m.tasks.Put(id.String(), t)
	if err != nil {
		return fmt.Errorf("error updating task: %v", err)
	}

	m.logger.Infof("Task status updated: %s -> %s", t.Name, req.State)
	return nil
}

func (m *Manager) listTasks() ([]*task.Task, error) {
	tasks, err := m.tasks.List()
	if err != nil {
		return nil, fmt.Errorf("error listing tasks: %v", err)
	}
	return tasks, nil
}

func (m *Manager) scheduleTask(t *task.Task) error {
	// Get list of available workers
	workers, err := m.listWorkers()
	if err != nil {
		return fmt.Errorf("error listing workers: %v", err)
	}

	if len(workers) == 0 {
		return fmt.Errorf("no workers available for scheduling")
	}

	// Simple scheduling: pick the first worker (MVP strategy)
	worker := workers[0]
	t.SetWorkerID(worker.ID)

	// Update task in store with assigned worker
	err = m.tasks.Put(t.ID.String(), t)
	if err != nil {
		return fmt.Errorf("error updating task with worker assignment: %v", err)
	}

	// Create deploy request
	deployReq := api.DeployTaskRequest{
		TaskID: t.ID.String(),
		Name:   t.Name,
		Image:  t.Image,
	}

	jsonData, err := json.Marshal(deployReq)
	if err != nil {
		return fmt.Errorf("error marshaling deploy request: %v", err)
	}

	// Send task to worker
	workerEndpoint := fmt.Sprintf("http://%s:%d/tasks", worker.Address, worker.Port)
	resp, err := http.Post(workerEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending task to worker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("worker rejected task, status: %d", resp.StatusCode)
	}

	// Update task state to Running
	t.SetState(task.TaskRunning)
	err = m.tasks.Put(t.ID.String(), t)
	if err != nil {
		return fmt.Errorf("error updating task state: %v", err)
	}

	m.logger.Infof("Task %s scheduled to worker %s", t.Name, worker.Name)
	return nil
}

// recordHeartbeat updates the LastHeartbeat timestamp for a worker.
func (m *Manager) recordHeartbeat(workerID string) error {
	worker, err := m.workers.Get(workerID)
	if err != nil {
		return fmt.Errorf("error getting worker: %v", err)
	}
	if worker == nil {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	worker.LastHeartbeat = time.Now()
	err = m.workers.Put(workerID, worker)
	if err != nil {
		return fmt.Errorf("error updating worker heartbeat: %v", err)
	}

	return nil
}

// detectDeadWorkers starts a background goroutine that removes workers with stale heartbeats.
// Workers are considered dead if they haven't sent a heartbeat within the timeout period.
func (m *Manager) detectDeadWorkers(heartbeatTimeout time.Duration, checkInterval time.Duration) {
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				workers, err := m.listWorkers()
				if err != nil {
					m.logger.Errorf("error listing workers for dead worker detection: %v", err)
					continue
				}

				now := time.Now()
				for _, worker := range workers {
					if now.Sub(worker.LastHeartbeat) > heartbeatTimeout {
						m.logger.Warnf("Worker %s (%s) is dead (no heartbeat for %v), removing", worker.Name, worker.ID.String(), now.Sub(worker.LastHeartbeat))
						err := m.workers.Delete(worker.ID.String())
						if err != nil {
							m.logger.Errorf("error removing dead worker %s: %v", worker.ID.String(), err)
						}
					}
				}
			}
		}
	}()
}
