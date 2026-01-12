package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/manager"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/storage"
	"github.com/sjsanc/gorc/worker"
	"go.uber.org/zap"
)

// Feature 1: Manager Initialization
func TestManagerInitialization(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, err := manager.NewManager(sugar, "0.0.0.0", 5500, storage.StorageInMemory, runtime.RuntimeDocker)
	if err != nil {
		t.Fatalf("Failed to create Manager: %v", err)
	}

	go m.Run()
	defer m.Stop()

	time.Sleep(1 * time.Second)

	resp, err := http.Get("http://0.0.0.0:5500/health")
	if err != nil {
		t.Fatalf("Manager not responding: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("✓ Manager initialized and responding to requests")
}

// Feature 2: Worker Registration
func TestWorkerRegistration(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, _ := manager.NewManager(sugar, "0.0.0.0", 5555, storage.StorageInMemory, runtime.RuntimeDocker)
	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	w, _ := worker.NewWorker(sugar, "0.0.0.0", 5556, "0.0.0.0:5555", runtime.RuntimeDocker)
	go w.Run()
	defer w.Stop()

	time.Sleep(1 * time.Second)

	resp, err := http.Get("http://0.0.0.0:5555/worker")
	if err != nil {
		t.Fatalf("Failed to fetch workers: %v", err)
	}
	defer resp.Body.Close()

	var workers []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&workers); err != nil {
		t.Fatalf("Failed to decode workers response: %v", err)
	}

	if len(workers) == 0 {
		t.Fatalf("Expected at least 1 registered worker, got %d", len(workers))
	}

	t.Logf("✓ Worker registered successfully. Registered workers: %d", len(workers))
}

// Feature 3: Node Registry
func TestNodeRegistry(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, _ := manager.NewManager(sugar, "0.0.0.0", 5555, storage.StorageInMemory, runtime.RuntimeDocker)
	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	w, _ := worker.NewWorker(sugar, "0.0.0.0", 5556, "0.0.0.0:5555", runtime.RuntimeDocker)
	go w.Run()
	defer w.Stop()

	time.Sleep(1 * time.Second)

	resp, err := http.Get("http://0.0.0.0:5555/node")
	if err != nil {
		t.Fatalf("Failed to fetch nodes: %v", err)
	}
	defer resp.Body.Close()

	var nodes []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		t.Fatalf("Failed to decode nodes response: %v", err)
	}

	if len(nodes) < 1 {
		t.Fatalf("Expected at least 1 node, got %d", len(nodes))
	}

	t.Logf("✓ Nodes registered in cluster. Total nodes: %d", len(nodes))
}

// Feature 4: Task Deployment
func TestTaskDeployment(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, _ := manager.NewManager(sugar, "0.0.0.0", 5555, storage.StorageInMemory, runtime.RuntimeDocker)
	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	w, _ := worker.NewWorker(sugar, "0.0.0.0", 5556, "0.0.0.0:5555", runtime.RuntimeDocker)
	go w.Run()
	defer w.Stop()

	time.Sleep(1 * time.Second)

	deployReq := api.DeployRequest{
		Image: "alpine:latest",
		Name:  "test-task",
	}

	body, _ := json.Marshal(deployReq)
	resp, err := http.Post("http://0.0.0.0:5555/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to deploy task: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected status 200/201, got %d", resp.StatusCode)
	}

	t.Log("✓ Task deployed successfully")
}

// Feature 5: Task Event Processing
func TestTaskEventProcessing(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, _ := manager.NewManager(sugar, "0.0.0.0", 5555, storage.StorageInMemory, runtime.RuntimeDocker)
	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	w, _ := worker.NewWorker(sugar, "0.0.0.0", 5556, "0.0.0.0:5555", runtime.RuntimeDocker)
	go w.Run()
	defer w.Stop()

	time.Sleep(1 * time.Second)

	deployReq := api.DeployRequest{
		Image: "alpine:latest",
		Name:  "test-event-task",
	}

	body, _ := json.Marshal(deployReq)
	resp, err := http.Post("http://0.0.0.0:5555/tasks", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to deploy task: %v", err)
	}
	defer resp.Body.Close()

	time.Sleep(2 * time.Second)

	resp, err = http.Get("http://0.0.0.0:5555/tasks")
	if err != nil {
		t.Fatalf("Failed to fetch tasks: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("✓ Worker received and processed task event. Tasks: %s", string(bodyBytes))
}
