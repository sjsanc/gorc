package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/config"
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
	})
	s.router.Route("/worker", func(r chi.Router) {
		r.Get("/", s.handleListWorkers)
		r.Post("/", s.handleRegisterWorker)
		r.Post("/{workerID}/heartbeat", s.handleWorkerHeartbeat)
	})
	s.router.Route("/tasks", func(r chi.Router) {
		r.Post("/", s.handleCreateTask)
		r.Get("/", s.handleListTasks)
		r.Put("/{taskID}/status", s.handleUpdateTaskStatus)
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

// POST /tasks
//
// Create and schedule a new task
func (s *server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Image == "" {
		http.Error(w, "name and image are required", http.StatusBadRequest)
		return
	}

	// Create the task
	t, err := s.manager.createTask(req.Name, req.Image)
	if err != nil {
		http.Error(w, fmt.Sprintf("error creating task: %v", err), http.StatusInternalServerError)
		return
	}

	// Schedule the task
	err = s.manager.scheduleTask(t)
	if err != nil {
		http.Error(w, fmt.Sprintf("error scheduling task: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"task_id": t.ID.String(),
		"name":    t.Name,
		"status":  "scheduled",
	})
}

// GET /tasks
//
// List all tasks in the cluster
func (s *server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.manager.listTasks()
	if err != nil {
		http.Error(w, "error listing tasks", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tasks)
}

// PUT /tasks/{taskID}/status
//
// Update task status (called by workers to report task state changes)
func (s *server) handleUpdateTaskStatus(w http.ResponseWriter, r *http.Request) {
	taskIDStr := chi.URLParam(r, "taskID")
	if taskIDStr == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	var req api.TaskStatusUpdateRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	err = s.manager.updateTaskStatus(taskIDStr, req)
	if err != nil {
		http.Error(w, fmt.Sprintf("error updating task status: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}
