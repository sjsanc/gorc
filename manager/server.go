package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/config"
	"github.com/sjsanc/gorc/replica"
	"github.com/sjsanc/gorc/service"
)

type server struct {
	manager    *Manager
	address    string
	port       int
	router     *chi.Mux
	httpServer *http.Server
	ctx        context.Context
	cancel     context.CancelFunc
}

func newServer(manager *Manager, address string, port int) *server {
	if address == "" {
		address = config.DefaultListenAddress
	}

	if port == 0 {
		port = config.DefaultManagerPort
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &server{
		manager: manager,
		address: address,
		port:    port,
		router:  chi.NewRouter(),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (s *server) start() error {
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.address, s.port),
		Handler: s.router,
	}
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.manager.logger.Errorf("server error: %v", err)
		}
	}()
	return nil
}

func (s *server) stop() error {
	s.cancel()
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), config.DefaultServerShutdownTimeout)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *server) initRouter() {
	s.router.Get("/health", s.handleHealth)
	s.router.Route("/node", func(r chi.Router) {
		r.Get("/", s.handleListNodes)
		r.Put("/{hostname}/metrics", s.handleUpdateNodeMetrics)
	})
	s.router.Route("/worker", func(r chi.Router) {
		r.Get("/", s.handleListWorkers)
		r.Post("/", s.handleRegisterWorker)
		r.Post("/{workerID}/heartbeat", s.handleWorkerHeartbeat)
	})
	s.router.Route("/replicas", func(r chi.Router) {
		r.Post("/", s.handleCreateReplica)
		r.Get("/", s.handleListReplicas)
		r.Put("/{replicaID}/status", s.handleUpdateReplicaStatus)
		r.Delete("/{replicaID}", s.handleStopReplica)
	})
	s.router.Route("/services", func(r chi.Router) {
		r.Post("/", s.handleCreateOrUpdateService)
		r.Get("/", s.handleListServices)
		r.Put("/{serviceID}", s.handleUpdateService)
		r.Delete("/{serviceName}", s.handleDeleteService)
	})
	s.router.Route("/apps", func(r chi.Router) {
		r.Get("/", s.handleListApps)
		r.Get("/{appName}/services", s.handleListAppServices)
		r.Delete("/{appName}", s.handleDeleteApp)
	})
}

// GET /health
//
// Health check endpoint
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// GET /node
//
// List all nodes in the Cluster
func (s *server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.manager.listNodes()
	if err != nil {
		http.Error(w, fmt.Sprintf("error listing nodes: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(nodes)
}

// PUT /node/{hostname}/metrics
//
// Update node metrics (called by nodes to push metrics)
func (s *server) handleUpdateNodeMetrics(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	if hostname == "" {
		http.Error(w, "hostname is required", http.StatusBadRequest)
		return
	}

	var req api.NodeMetricsUpdateRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	err = s.manager.updateNodeMetrics(hostname, req.Metrics)
	if err != nil {
		http.Error(w, fmt.Sprintf("error updating metrics: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "metrics_updated"})
}

// GET /worker
//
// List all Workers registered with the Manager
func (s *server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := s.manager.listWorkers()
	if err != nil {
		http.Error(w, fmt.Sprintf("error listing workers: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(workers)
}

// POST /worker
//
// Register a Worker with the Manager
func (s *server) handleRegisterWorker(w http.ResponseWriter, r *http.Request) {
	d := json.NewDecoder(r.Body)
	req := api.RegisterWorkerRequest{}
	err := d.Decode(&req)
	if err != nil {
		http.Error(w, fmt.Sprintf("error decoding request: %v", err), http.StatusBadRequest)
		return
	}

	mw, err := s.manager.registerWorker(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("error registering worker: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "registered",
		"workerId": mw.ID.String(),
	})
}

// POST /worker/{workerID}/heartbeat
//
// Record a heartbeat from a Worker
func (s *server) handleWorkerHeartbeat(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "workerID")
	if workerID == "" {
		http.Error(w, "worker ID is required", http.StatusBadRequest)
		return
	}

	err := s.manager.recordHeartbeat(workerID)
	if err != nil {
		http.Error(w, fmt.Sprintf("error recording heartbeat: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "heartbeat_recorded"})
}

// POST /replicas
//
// Create and schedule a new replica
func (s *server) handleCreateReplica(w http.ResponseWriter, r *http.Request) {
	var req api.DeployRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Image == "" {
		http.Error(w, "name and image are required", http.StatusBadRequest)
		return
	}

	// Create the replica
	replica, err := s.manager.createReplica(req.Name, req.Image, req.Cmd)
	if err != nil {
		http.Error(w, fmt.Sprintf("error creating replica: %v", err), http.StatusInternalServerError)
		return
	}

	// Schedule the replica
	err = s.manager.scheduleReplica(replica)
	if err != nil {
		http.Error(w, fmt.Sprintf("error scheduling replica: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"replica_id": replica.ID.String(),
		"name":       replica.Name,
		"status":     "scheduled",
	})
}

// GET /replicas
//
// List all replicas in the cluster
func (s *server) handleListReplicas(w http.ResponseWriter, r *http.Request) {
	replicas, err := s.manager.listReplicas()
	if err != nil {
		http.Error(w, "error listing replicas", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(replicas)
}

// PUT /replicas/{replicaID}/status
//
// Update replica status (called by workers to report replica state changes)
func (s *server) handleUpdateReplicaStatus(w http.ResponseWriter, r *http.Request) {
	replicaIDStr := chi.URLParam(r, "replicaID")
	if replicaIDStr == "" {
		http.Error(w, "replica ID is required", http.StatusBadRequest)
		return
	}

	var req api.ReplicaStatusUpdateRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	err = s.manager.updateReplicaStatus(replicaIDStr, req)
	if err != nil {
		http.Error(w, fmt.Sprintf("error updating replica status: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

// PUT /replicas/{replicaID}/status (legacy endpoint)
//
// Backward compatibility handler for replica-based status updates
func (s *server) handleUpdateReplicaStatusLegacy(w http.ResponseWriter, r *http.Request) {
	replicaIDStr := chi.URLParam(r, "replicaID")
	if replicaIDStr == "" {
		http.Error(w, "replica ID is required", http.StatusBadRequest)
		return
	}
	// Redirect to new handler with legacy parameter name
	s.handleUpdateReplicaStatus(w, r)
}

// DELETE /replicas/{replicaID}
//
// Stop a running replica
func (s *server) handleStopReplica(w http.ResponseWriter, r *http.Request) {
	replicaIDStr := chi.URLParam(r, "replicaID")
	if replicaIDStr == "" {
		http.Error(w, "replica ID is required", http.StatusBadRequest)
		return
	}

	replicaID, err := parseUUID(replicaIDStr)
	if err != nil {
		http.Error(w, "invalid replica ID", http.StatusBadRequest)
		return
	}

	err = s.manager.stopReplica(replicaID)
	if err != nil {
		if isNotFoundError(err) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if isNotRunningError(err) || isWorkerNotFoundError(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if isContactWorkerError(err) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		} else {
			http.Error(w, fmt.Sprintf("error stopping replica: %v", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "stop initiated",
		"replica_id": replicaIDStr,
	})
}

// DELETE /replicas/{replicaID} (legacy endpoint)
//
// Stop a running replica (backward compatibility)
func (s *server) handleStopReplicaLegacy(w http.ResponseWriter, r *http.Request) {
	replicaIDStr := chi.URLParam(r, "replicaID")
	if replicaIDStr == "" {
		http.Error(w, "replica ID is required", http.StatusBadRequest)
		return
	}

	replicaID, err := parseUUID(replicaIDStr)
	if err != nil {
		http.Error(w, "invalid replica ID", http.StatusBadRequest)
		return
	}

	err = s.manager.stopReplica(replicaID)
	if err != nil {
		if isNotFoundError(err) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else if isNotRunningError(err) || isWorkerNotFoundError(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if isContactWorkerError(err) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		} else {
			http.Error(w, fmt.Sprintf("error stopping replica: %v", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "stop initiated",
		"replica_id": replicaIDStr,
	})
}

// Service handlers

// POST /services - Create or update a service
func (s *server) handleCreateOrUpdateService(w http.ResponseWriter, r *http.Request) {
	var req api.CreateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Image == "" || req.Replicas < 1 {
		http.Error(w, "name, image, and replicas (>=1) are required", http.StatusBadRequest)
		return
	}

	// Parse restart policy
	svc := &service.Service{
		ID:            uuid.New(),
		Name:          req.Name,
		Image:         req.Image,
		Replicas:      req.Replicas,
		Cmd:           req.Cmd,
		RestartPolicy: service.RestartPolicy(req.RestartPolicy),
		AppName:       req.AppName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Validate restart policy
	switch svc.RestartPolicy {
	case service.RestartAlways, service.RestartOnFailure, service.RestartNever, "":
		// Valid
	default:
		http.Error(w, fmt.Sprintf("invalid restart_policy: %s", req.RestartPolicy), http.StatusBadRequest)
		return
	}

	// Default to on-failure
	if svc.RestartPolicy == "" {
		svc.RestartPolicy = service.RestartOnFailure
	}

	created, err := s.manager.createOrUpdateService(svc)
	if err != nil {
		http.Error(w, fmt.Sprintf("error creating service: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service_id": created.ID.String(),
		"name":       created.Name,
		"status":     "reconciling",
	})
}

// GET /services - List all services
func (s *server) handleListServices(w http.ResponseWriter, r *http.Request) {
	services, err := s.manager.listServices()
	if err != nil {
		http.Error(w, "error listing services", http.StatusInternalServerError)
		return
	}

	if services == nil {
		services = []*service.Service{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(services)
}

// DELETE /services/{serviceName} - Delete a service and its replicas
func (s *server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	serviceName := chi.URLParam(r, "serviceName")

	svc, err := s.manager.getServiceByName(serviceName)
	if err != nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	// Stop all replicas for this service
	replicas, _ := s.manager.getReplicasForService(svc.ID)
	for _, r := range replicas {
		if r.State == replica.ReplicaRunning {
			s.manager.stopReplica(r.ID)
		}
	}

	// Delete service
	s.manager.deleteService(svc.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// PUT /services/{serviceID} - Update service replica count
func (s *server) handleUpdateService(w http.ResponseWriter, r *http.Request) {
	serviceIDStr := chi.URLParam(r, "serviceID")
	if serviceIDStr == "" {
		http.Error(w, "service ID is required", http.StatusBadRequest)
		return
	}

	serviceID, err := parseUUID(serviceIDStr)
	if err != nil {
		http.Error(w, "invalid service ID", http.StatusBadRequest)
		return
	}

	var req api.UpdateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Replicas < 0 {
		http.Error(w, "replicas cannot be negative", http.StatusBadRequest)
		return
	}

	// Get the existing service
	svc, err := s.manager.getService(serviceID)
	if err != nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	// Update replicas
	svc.Replicas = req.Replicas
	svc.UpdatedAt = time.Now()

	// Store the updated service
	err = s.manager.services.Put(svc.ID.String(), svc)
	if err != nil {
		http.Error(w, fmt.Sprintf("error updating service: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service_id": svc.ID.String(),
		"name":       svc.Name,
		"replicas":   svc.Replicas,
		"status":     "updated",
	})
}

// GET /apps - List all apps
func (s *server) handleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.manager.listApps()
	if err != nil {
		http.Error(w, "error listing apps", http.StatusInternalServerError)
		return
	}

	if apps == nil {
		apps = []*api.AppInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apps)
}

// GET /apps/{appName}/services - List all services for an app
func (s *server) handleListAppServices(w http.ResponseWriter, r *http.Request) {
	appName := chi.URLParam(r, "appName")
	if appName == "" {
		http.Error(w, "app name is required", http.StatusBadRequest)
		return
	}

	allServices, err := s.manager.listServices()
	if err != nil {
		http.Error(w, "error listing services", http.StatusInternalServerError)
		return
	}

	// Filter services by app name
	var appServices []*service.Service
	for _, svc := range allServices {
		if svc.AppName == appName {
			appServices = append(appServices, svc)
		}
	}

	if appServices == nil {
		appServices = []*service.Service{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(appServices)
}

// DELETE /apps/{appName} - Delete an app and all its services and replicas
func (s *server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	appName := chi.URLParam(r, "appName")
	if appName == "" {
		http.Error(w, "app name is required", http.StatusBadRequest)
		return
	}

	err := s.manager.deleteServicesForApp(appName)
	if err != nil {
		http.Error(w, fmt.Sprintf("error deleting app: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
		"app":    appName,
	})
}

// Helper functions for error parsing
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func isNotFoundError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not exist"))
}

func isNotRunningError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not running")
}

func isWorkerNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "worker not found")
}

func isContactWorkerError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "failed to contact worker")
}
