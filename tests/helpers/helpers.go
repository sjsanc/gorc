package helpers

import (
	"testing"
	"time"

	"github.com/sjsanc/gorc/manager"
	"github.com/sjsanc/gorc/manager/scheduler"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/storage"
	"github.com/sjsanc/gorc/worker"
	"go.uber.org/zap"
)

const (
	DefaultManagerPort = 5555
	DefaultWorkerPort  = 5556
)

type clusterConfig struct {
	managerPort   int
	workerCount   int
	schedulerType scheduler.Type
}

type ClusterOption func(*clusterConfig)

func WithManagerPort(port int) ClusterOption {
	return func(c *clusterConfig) {
		c.managerPort = port
	}
}

func WithWorkerCount(n int) ClusterOption {
	return func(c *clusterConfig) {
		c.workerCount = n
	}
}

func WithNoWorkers() ClusterOption {
	return func(c *clusterConfig) {
		c.workerCount = 0
	}
}

func WithScheduler(s scheduler.Type) ClusterOption {
	return func(c *clusterConfig) {
		c.schedulerType = s
	}
}

type TestCluster struct {
	T           *testing.T
	Manager     *manager.Manager
	Workers     []*worker.Worker
	ManagerAddr string

	sugar      *zap.SugaredLogger
	nextPort   int
	managerErr chan error
}

func NewTestCluster(t *testing.T, opts ...ClusterOption) *TestCluster {
	cfg := &clusterConfig{
		managerPort:   DefaultManagerPort,
		workerCount:   1,
		schedulerType: scheduler.TypeRoundRobin,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, err := manager.NewManager(
		sugar,
		"0.0.0.0",
		cfg.managerPort,
		storage.StorageInMemory,
		runtime.RuntimeDocker,
		cfg.schedulerType,
	)
	if err != nil {
		t.Fatalf("Failed to create Manager: %v", err)
	}

	managerErr := make(chan error, 1)
	go func() {
		managerErr <- m.Run()
	}()

	time.Sleep(500 * time.Millisecond)

	cluster := &TestCluster{
		T:           t,
		Manager:     m,
		Workers:     make([]*worker.Worker, 0),
		ManagerAddr: "0.0.0.0:" + Itoa(cfg.managerPort),
		sugar:       sugar,
		nextPort:    DefaultWorkerPort,
		managerErr:  managerErr,
	}

	for i := 0; i < cfg.workerCount; i++ {
		cluster.AddWorker()
	}

	if cfg.workerCount > 0 {
		time.Sleep(500 * time.Millisecond)
	}

	return cluster
}

func (c *TestCluster) AddWorker() *worker.Worker {
	port := c.nextPort
	c.nextPort++

	w, err := worker.NewWorker(c.sugar, "0.0.0.0", port, c.ManagerAddr, runtime.RuntimeDocker)
	if err != nil {
		c.T.Fatalf("Failed to create worker on port %d: %v", port, err)
	}

	go w.Run()
	c.Workers = append(c.Workers, w)

	return w
}

func (c *TestCluster) StopWorker(index int) {
	if index < 0 || index >= len(c.Workers) {
		c.T.Fatalf("Invalid worker index: %d", index)
	}
	if err := c.Workers[index].Stop(); err != nil {
		c.T.Fatalf("Failed to stop worker %d: %v", index, err)
	}
}

func (c *TestCluster) Stop() {
	for i, w := range c.Workers {
		if err := w.Stop(); err != nil {
			c.T.Logf("Warning: failed to stop worker %d: %v", i, err)
		}
	}
	if err := c.Manager.Stop(); err != nil {
		c.T.Logf("Warning: failed to stop manager: %v", err)
	}
}

func (c *TestCluster) Client() *Client {
	return &Client{
		t:       c.T,
		baseURL: "http://" + c.ManagerAddr,
	}
}

func (c *TestCluster) WorkerClient(index int) *Client {
	if index < 0 || index >= len(c.Workers) {
		c.T.Fatalf("Invalid worker index: %d", index)
		return nil
	}
	return &Client{
		t:       c.T,
		baseURL: "http://0.0.0.0:" + Itoa(DefaultWorkerPort+index),
	}
}

func (c *TestCluster) WaitForManagerDone(timeout time.Duration) error {
	select {
	case err := <-c.managerErr:
		return err
	case <-time.After(timeout):
		c.T.Fatalf("Manager did not shut down within timeout")
		return nil
	}
}

func Itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte(n%10) + '0'
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
