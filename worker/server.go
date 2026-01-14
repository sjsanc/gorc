package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/config"
	"github.com/sjsanc/gorc/replica"
)

type server struct {
	worker     *Worker
	address    string
	port       int
	router     *chi.Mux
	httpServer *http.Server
	ctx        context.Context
	cancel     context.CancelFunc
}

func newServer(worker *Worker, address string, port int) *server {
	if address == "" {
		address = config.DefaultListenAddress
	}

	if port == 0 {
		port = config.DefaultWorkerPort
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &server{
		worker:  worker,
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
			s.worker.logger.Errorf("server error: %v", err)
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
	s.router.Route("/replicas", func(r chi.Router) {
		r.Post("/", s.handleDeployReplica)
		r.Post("/{replicaID}/stop", s.handleStopReplica)
	})
}

func (s *server) handleDeployReplica(w http.ResponseWriter, r *http.Request) {
	var req api.DeployReplicaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	replicaID, err := uuid.Parse(req.ReplicaID)
	if err != nil {
		http.Error(w, "invalid replica ID", http.StatusBadRequest)
		return
	}

	rep := &replica.Replica{
		ID:    replicaID,
		Name:  req.Name,
		Image: req.Image,
		Cmd:   req.Cmd,
		State: replica.ReplicaPending,
	}

	event := replica.Event{Replica: rep, Action: replica.DeployEvent}
	s.worker.events.Enqueue(event)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func (s *server) handleStopReplica(w http.ResponseWriter, r *http.Request) {
	replicaIDStr := chi.URLParam(r, "replicaID")
	replicaID, err := uuid.Parse(replicaIDStr)
	if err != nil {
		http.Error(w, "invalid replica ID", http.StatusBadRequest)
		return
	}

	var req api.StopReplicaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	rep := &replica.Replica{
		ID: replicaID,
	}
	rep.SetContainerID(req.ContainerID)

	event := replica.Event{Replica: rep, Action: replica.StopEvent}
	s.worker.events.Enqueue(event)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func (s *server) handleStopReplicaLegacy(w http.ResponseWriter, r *http.Request) {
	replicaIDStr := chi.URLParam(r, "replicaID")
	replicaID, err := uuid.Parse(replicaIDStr)
	if err != nil {
		http.Error(w, "invalid replica ID", http.StatusBadRequest)
		return
	}

	var req api.StopReplicaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	rep := &replica.Replica{
		ID: replicaID,
	}
	rep.SetContainerID(req.ContainerID)

	event := replica.Event{Replica: rep, Action: replica.StopEvent}
	s.worker.events.Enqueue(event)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
