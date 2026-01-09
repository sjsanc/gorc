package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/node"
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
	events utils.Queue[task.Event]
}

func NewWorker(logger *zap.SugaredLogger, addr string, port int, managerAddr string) (*Worker, error) {
	hn, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("error getting hostname: %v", err)
	}

	n := node.NewNode(hn, addr, port, 0)

	w := &Worker{
		id:          uuid.New(),
		logger:      logger,
		server:      nil,
		name:        hn,
		node:        n,
		managerAddr: managerAddr,
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

	w.server.start()

	return nil
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
