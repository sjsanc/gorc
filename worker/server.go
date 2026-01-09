package worker

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type server struct {
	worker  *Worker
	address string
	port    int
	router  *chi.Mux
}

func newServer(worker *Worker, address string, port int) *server {
	if address == "" {
		address = ""
	}

	if port == 0 {
		port = 5556
	}

	return &server{
		worker:  worker,
		address: address,
		port:    port,
		router:  chi.NewRouter(),
	}
}

func (s *server) start() {
	http.ListenAndServe(fmt.Sprintf("%s:%d", s.address, s.port), s.router)
}

func (s *server) initRouter() {
	s.router.Route("/tasks", func(r chi.Router) {
		r.Post("/", s.handleDeployTask)
	})
}

func (s *server) handleDeployTask(w http.ResponseWriter, r *http.Request) {
}
