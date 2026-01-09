package manager

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/node"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/storage"
	"go.uber.org/zap"
)

// `managedWorker` represents the Manager's view of a Worker within the Cluster.
type ManagedWorker struct {
	ID   uuid.UUID
	Name string
	Node *node.Node
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
}

func NewManager(logger *zap.SugaredLogger, addr string, port int, storageType storage.StorageType, runtimeType runtime.RuntimeType) (*Manager, error) {
	// Create the Manager Node
	hn, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("error getting hostname: %v", err)
	}

	n := node.NewNode(hn, addr, 0, port)

	// Create a new Manager instance
	m := &Manager{
		logger:  logger,
		server:  nil,
		node:    n,
		nodes:   storage.NewStore[node.Node](storageType),
		workers: storage.NewStore[ManagedWorker](storageType),
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

// Run starts the Manager API server.
func (m *Manager) Run() {
	m.server.start()
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

	newNode := node.NewNode(req.WorkerName, req.WorkerAddress, req.WorkerPort, 0)

	n, err := m.registerNode(newNode)
	if err != nil {
		return nil, fmt.Errorf("error registering Worker node: %v", err)
	}

	mw := &ManagedWorker{
		ID:   id,
		Name: req.WorkerName,
		Node: n,
	}

	err = m.workers.Put(id.String(), mw)
	if err != nil {
		return nil, fmt.Errorf("error registering Worker: %v", err)
	}

	fmt.Println("Worker registered:", req.WorkerName)

	return nil, nil
}

func (m *Manager) listWorkers() ([]*ManagedWorker, error) {
	workers, err := m.workers.List()
	if err != nil {
		return nil, fmt.Errorf("error listing workers: %v", err)
	}
	return workers, nil
}
