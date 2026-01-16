package manager

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
	"github.com/sjsanc/gorc/service"
	"github.com/sjsanc/gorc/storage"
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
	// `replicas` is a store of all replicas, keyed by replica `id`.
	replicas storage.Store[replica.Replica]
	// `services` is a store of all services, keyed by service `id`.
	services storage.Store[service.Service]
	// `metricsCollector` collects system metrics for this manager node.
	metricsCollector metrics.Collector
	// `ctx` is the context for the Manager and its background goroutines.
	ctx context.Context
	// `cancel` cancels the Manager context.
	cancel context.CancelFunc
	// `wg` waits for all background goroutines to complete.
	wg sync.WaitGroup
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

	replicasStore, err := storage.NewStore[replica.Replica](storageType)
	if err != nil {
		return nil, fmt.Errorf("error creating replicas store: %v", err)
	}

	servicesStore, err := storage.NewStore[service.Service](storageType)
	if err != nil {
		return nil, fmt.Errorf("error creating services store: %v", err)
	}

	collector, err := metrics.NewSystemCollector()
	if err != nil {
		return nil, fmt.Errorf("error creating metrics collector: %v", err)
	}

	// Create a new Manager instance
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		logger:           logger,
		server:           nil,
		node:             n,
		nodes:            nodesStore,
		workers:          workersStore,
		replicas:         replicasStore,
		services:         servicesStore,
		metricsCollector: collector,
		ctx:              ctx,
		cancel:           cancel,
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
	m.detectDeadWorkers(config.DefaultHeartbeatTimeout, config.DefaultHeartbeatCheck)

	// Start collecting metrics for this manager node
	go m.collectAndStoreMetrics()

	// Start reconciliation loop
	m.startReconciliationLoop()

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
	m.wg.Wait()
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

	m.logger.Infof("Node registered: %s", node.Hostname)

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

	// Register/update node for this worker
	workerNode := node.NewNode(req.WorkerName, address, req.WorkerPort, 0)
	registeredNode, err := m.registerNode(workerNode)
	if err != nil {
		m.logger.Warnf("failed to register node for worker %s: %v", req.WorkerName, err)
	}

	mw := &ManagedWorker{
		ID:            id,
		Name:          req.WorkerName,
		Address:       address,
		Port:          req.WorkerPort,
		Node:          registeredNode,
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

func (m *Manager) createReplica(name, image string, args []string) (*replica.Replica, error) {
	r := replica.NewReplica(name, image, args)
	err := m.replicas.Put(r.ID.String(), r)
	if err != nil {
		return nil, fmt.Errorf("error creating replica: %v", err)
	}
	m.logger.Infof("Replica created: %s (%s)", r.Name, r.ID.String())
	return r, nil
}

func (m *Manager) getReplica(id uuid.UUID) (*replica.Replica, error) {
	r, err := m.replicas.Get(id.String())
	if err != nil {
		return nil, fmt.Errorf("error getting replica: %v", err)
	}
	if r == nil {
		return nil, fmt.Errorf("replica not found: %s", id.String())
	}
	return r, nil
}

func (m *Manager) updateReplicaState(id uuid.UUID, state replica.ReplicaState) error {
	r, err := m.getReplica(id)
	if err != nil {
		return err
	}
	r.SetState(state)
	err = m.replicas.Put(id.String(), r)
	if err != nil {
		return fmt.Errorf("error updating replica state: %v", err)
	}
	m.logger.Infof("Replica state updated: %s -> %v", r.Name, state)
	return nil
}

func (m *Manager) updateReplicaStatus(replicaIDStr string, req api.ReplicaStatusUpdateRequest) error {
	id, err := uuid.Parse(replicaIDStr)
	if err != nil {
		return fmt.Errorf("invalid replica ID: %v", err)
	}

	r, err := m.getReplica(id)
	if err != nil {
		return err
	}

	// Update replica state based on request
	switch req.State {
	case "running":
		r.SetState(replica.ReplicaRunning)
	case "completed":
		r.MarkCompleted()
	case "failed":
		r.SetError(req.Error)
	default:
		return fmt.Errorf("invalid replica state: %s", req.State)
	}

	// Update container ID if provided
	if req.ContainerID != "" {
		r.SetContainerID(req.ContainerID)
	}

	// Save updated replica
	err = m.replicas.Put(id.String(), r)
	if err != nil {
		return fmt.Errorf("error updating replica: %v", err)
	}

	m.logger.Infof("Replica status updated: %s -> %s", r.Name, req.State)
	return nil
}

func (m *Manager) listReplicas() ([]*replica.Replica, error) {
	replicas, err := m.replicas.List()
	if err != nil {
		return nil, fmt.Errorf("error listing replicas: %v", err)
	}
	return replicas, nil
}

func (m *Manager) scheduleReplica(r *replica.Replica) error {
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
	r.SetWorkerID(worker.ID)

	// Update replica in store with assigned worker
	err = m.replicas.Put(r.ID.String(), r)
	if err != nil {
		return fmt.Errorf("error updating replica with worker assignment: %v", err)
	}

	// Create deploy request
	deployReq := api.DeployReplicaRequest{
		ReplicaID: r.ID.String(),
		Name:      r.Name,
		Image:     r.Image,
		Cmd:       r.Cmd,
	}

	jsonData, err := json.Marshal(deployReq)
	if err != nil {
		return fmt.Errorf("error marshaling deploy request: %v", err)
	}

	// Send replica to worker
	workerEndpoint := fmt.Sprintf("http://%s:%d/replicas", worker.Address, worker.Port)
	resp, err := http.Post(workerEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error sending replica to worker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("worker rejected replica, status: %d", resp.StatusCode)
	}

	// Update replica state to Running
	r.SetState(replica.ReplicaRunning)
	err = m.replicas.Put(r.ID.String(), r)
	if err != nil {
		return fmt.Errorf("error updating replica state: %v", err)
	}

	m.logger.Infof("Replica %s scheduled to worker %s", r.Name, worker.Name)
	return nil
}

// stopReplica sends a stop request to the worker running the specified replica.
func (m *Manager) stopReplica(replicaID uuid.UUID) error {
	r, err := m.getReplica(replicaID)
	if err != nil {
		return err
	}

	if r.State != replica.ReplicaRunning {
		return fmt.Errorf("replica not running (current state: %v)", r.State)
	}

	worker, err := m.workers.Get(r.WorkerID.String())
	if err != nil {
		return fmt.Errorf("error getting worker: %v", err)
	}
	if worker == nil {
		return fmt.Errorf("worker not found for replica")
	}

	stopReq := api.StopReplicaRequest{
		ReplicaID:   replicaID.String(),
		ContainerID: r.GetContainerID(),
	}

	jsonData, err := json.Marshal(stopReq)
	if err != nil {
		return fmt.Errorf("error marshaling stop request: %v", err)
	}

	workerEndpoint := fmt.Sprintf("http://%s:%d/replicas/%s/stop", worker.Address, worker.Port, replicaID.String())
	resp, err := http.Post(workerEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to contact worker: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("worker rejected stop request: %d", resp.StatusCode)
	}

	m.logger.Infof("Stop request sent to worker for replica %s", r.Name)
	return nil
}

// restartReplica forcefully restarts a replica by stopping the old one and scheduling a new one.
func (m *Manager) restartReplica(id uuid.UUID) error {
	// Fetch the old replica
	oldReplica, err := m.getReplica(id)
	if err != nil {
		return err
	}

	// Log warning if not in expected state, but proceed anyway (idempotent)
	if oldReplica.State != replica.ReplicaRunning && oldReplica.State != replica.ReplicaPending {
		m.logger.Warnf("Restarting replica %s not in Running or Pending state (current: %v), proceeding anyway", oldReplica.Name, oldReplica.State)
	}

	// Fetch parent service for current config (image, cmd)
	var newImage string
	var newCmd []string
	var serviceName string

	if oldReplica.ServiceID != uuid.Nil {
		svc, err := m.getService(oldReplica.ServiceID)
		if err != nil {
			// If service not found, use replica's stored config as fallback
			m.logger.Warnf("Service not found for replica %s, using stored config: %v", oldReplica.Name, err)
			newImage = oldReplica.Image
			newCmd = oldReplica.Cmd
			serviceName = oldReplica.ServiceName
		} else {
			newImage = svc.Image
			newCmd = svc.Cmd
			serviceName = svc.Name
		}
	} else {
		// Orphaned replica, use stored config
		newImage = oldReplica.Image
		newCmd = oldReplica.Cmd
		serviceName = oldReplica.ServiceName
	}

	// Stop the old replica (ignore "already stopped" errors)
	if oldReplica.State == replica.ReplicaRunning {
		err = m.stopReplica(id)
		if err != nil && !strings.Contains(err.Error(), "not running") {
			// Log error but continue - worker may be dead, we'll create new replica anyway
			m.logger.Warnf("Failed to stop old replica %s: %v", oldReplica.Name, err)
		}
	}

	// Delete old replica from store
	err = m.deleteReplica(id)
	if err != nil {
		return fmt.Errorf("error deleting old replica: %v", err)
	}

	// Create new replica with same ReplicaID integer but new UUID
	newReplica := &replica.Replica{
		ID:          uuid.New(),
		Name:        oldReplica.Name,
		Image:       newImage,
		Cmd:         newCmd,
		State:       replica.ReplicaPending,
		ServiceID:   oldReplica.ServiceID,
		ServiceName: serviceName,
		ReplicaID:   oldReplica.ReplicaID,
		CreatedAt:   time.Now(),
	}

	// Store the new replica
	err = m.replicas.Put(newReplica.ID.String(), newReplica)
	if err != nil {
		return fmt.Errorf("error storing new replica: %v", err)
	}

	// Schedule the new replica
	err = m.scheduleReplica(newReplica)
	if err != nil {
		return fmt.Errorf("error scheduling new replica: %v", err)
	}

	m.logger.Infof("Replica restarted: %s (service: %s, old ID: %s, new ID: %s)",
		newReplica.Name, serviceName, oldReplica.ID.String(), newReplica.ID.String())

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

func (m *Manager) collectAndStoreMetrics() {
	ticker := time.NewTicker(config.DefaultHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			metrics, err := m.metricsCollector.Collect()
			if err != nil {
				m.logger.Warnf("failed to collect manager metrics: %v", err)
				continue
			}

			node, err := m.nodes.Get(m.node.Hostname)
			if err != nil || node == nil {
				m.logger.Errorf("failed to get manager node: %v", err)
				continue
			}

			node.Metrics = metrics
			err = m.nodes.Put(m.node.Hostname, node)
			if err != nil {
				m.logger.Errorf("failed to update manager node metrics: %v", err)
			}
		}
	}
}

func (m *Manager) updateNodeMetrics(hostname string, metrics *metrics.Metrics) error {
	node, err := m.nodes.Get(hostname)
	if err != nil {
		return fmt.Errorf("error getting node: %v", err)
	}
	if node == nil {
		return fmt.Errorf("node not found: %s", hostname)
	}

	node.Metrics = metrics
	err = m.nodes.Put(hostname, node)
	if err != nil {
		return fmt.Errorf("error updating node metrics: %v", err)
	}

	return nil
}

// Service management methods

func (m *Manager) createOrUpdateService(svc *service.Service) (*service.Service, error) {
	// Check if service already exists by name
	existing, err := m.getServiceByName(svc.Name)
	if err != nil && err.Error() != "service not found" {
		return nil, err
	}

	if existing != nil {
		// Update existing service (preserve ID, update timestamp)
		svc.ID = existing.ID
		svc.CreatedAt = existing.CreatedAt
		svc.UpdatedAt = time.Now()
	}

	err = m.services.Put(svc.ID.String(), svc)
	if err != nil {
		return nil, fmt.Errorf("error storing service: %v", err)
	}

	verb := "created"
	if existing != nil {
		verb = "updated"
	}
	m.logger.Infof("Service %s: %s", verb, svc.Name)
	return svc, nil
}

func (m *Manager) getServiceByName(name string) (*service.Service, error) {
	services, err := m.services.List()
	if err != nil {
		return nil, err
	}
	for _, svc := range services {
		if svc.Name == name {
			return svc, nil
		}
	}
	return nil, fmt.Errorf("service not found")
}

func (m *Manager) getService(id uuid.UUID) (*service.Service, error) {
	return m.services.Get(id.String())
}

func (m *Manager) listServices() ([]*service.Service, error) {
	return m.services.List()
}

func (m *Manager) deleteService(id uuid.UUID) error {
	return m.services.Delete(id.String())
}

func (m *Manager) getReplicasForService(serviceID uuid.UUID) ([]*replica.Replica, error) {
	allReplicas, err := m.listReplicas()
	if err != nil {
		return nil, err
	}

	var serviceReplicas []*replica.Replica
	for _, r := range allReplicas {
		if r.ServiceID == serviceID {
			serviceReplicas = append(serviceReplicas, r)
		}
	}
	return serviceReplicas, nil
}

func (m *Manager) deleteReplica(id uuid.UUID) error {
	return m.replicas.Delete(id.String())
}

func (m *Manager) deleteReplicasForService(serviceID uuid.UUID) error {
	replicas, err := m.getReplicasForService(serviceID)
	if err != nil {
		return err
	}

	for _, r := range replicas {
		if r.State == replica.ReplicaRunning {
			if err := m.stopReplica(r.ID); err != nil {
				m.logger.Warnf("failed to stop replica %s during delete: %v", r.Name, err)
			}
		}
		if err := m.deleteReplica(r.ID); err != nil {
			m.logger.Warnf("failed to delete replica %s: %v", r.Name, err)
		}
	}
	return nil
}

func (m *Manager) deleteServicesForApp(appName string) error {
	allServices, err := m.listServices()
	if err != nil {
		return err
	}

	for _, svc := range allServices {
		if svc.AppName == appName {
			if err := m.deleteReplicasForService(svc.ID); err != nil {
				m.logger.Warnf("failed to delete replicas for service %s: %v", svc.Name, err)
			}
			if err := m.deleteService(svc.ID); err != nil {
				m.logger.Warnf("failed to delete service %s: %v", svc.Name, err)
			}
		}
	}
	return nil
}

func (m *Manager) listApps() ([]*api.AppInfo, error) {
	allServices, err := m.listServices()
	if err != nil {
		return nil, err
	}

	// Group services by app name
	appMap := make(map[string]*api.AppInfo)
	for _, svc := range allServices {
		appName := svc.AppName
		if appName == "" {
			continue // Skip services not part of an app
		}

		if appMap[appName] == nil {
			appMap[appName] = &api.AppInfo{
				Name:      appName,
				CreatedAt: svc.CreatedAt,
			}
		}

		appMap[appName].ServiceCount++
		appMap[appName].TotalReplicas += svc.Replicas

		// Update CreatedAt to earliest service creation time
		if svc.CreatedAt.Before(appMap[appName].CreatedAt) {
			appMap[appName].CreatedAt = svc.CreatedAt
		}
	}

	// Convert map to slice
	var apps []*api.AppInfo
	for _, app := range appMap {
		apps = append(apps, app)
	}

	return apps, nil
}
