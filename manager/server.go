package manager

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sjsanc/gorc/api"
)

type server struct {
	manager *Manager
	address string
	port    int
	router  *chi.Mux
}

func newServer(manager *Manager, address string, port int) *server {
	if address == "" {
		address = "0.0.0.0"
	}

	if port == 0 {
		port = 5555
	}

	return &server{
		manager: manager,
		address: address,
		port:    port,
		router:  chi.NewRouter(),
	}
}

func (s *server) start() {
	http.ListenAndServe(fmt.Sprintf("%s:%d", s.address, s.port), s.router)
}

func (s *server) initRouter() {
	s.router.Route("/node", func(r chi.Router) {
		r.Get("/", s.handleListNodes)
	})
	s.router.Route("/worker", func(r chi.Router) {
		r.Get("/", s.handleListWorkers)
		r.Post("/", s.handleRegisterWorker)
	})
}

// GET /node
//
// List all nodes in the Cluster
func (s *server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.manager.listNodes()
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error listing nodes"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(nodes)
}

// GET /worker
//
// List all Workers registered with the Manager
func (s *server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := s.manager.listWorkers()
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error listing workers"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
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
		w.WriteHeader(500)
		w.Write([]byte("Error decoding request"))
		return
	}

	_, err = s.manager.registerWorker(req)
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte("Error registering worker"))
		return
	}

	w.WriteHeader(200)
	w.Write([]byte("LOL"))
}
