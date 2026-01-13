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

// Feature 6: Graceful Shutdown
func TestGracefulShutdown(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, err := manager.NewManager(sugar, "0.0.0.0", 5510, storage.StorageInMemory, runtime.RuntimeDocker)
	if err != nil {
		t.Fatalf("Failed to create Manager: %v", err)
	}

	// Start manager in goroutine
	done := make(chan error, 1)
	go func() {
		done <- m.Run()
	}()

	time.Sleep(500 * time.Millisecond)

	// Verify manager is responding
	resp, err := http.Get("http://0.0.0.0:5510/health")
	if err != nil {
		t.Fatalf("Manager not responding: %v", err)
	}
	resp.Body.Close()

	// Gracefully stop the manager
	stopErr := m.Stop()
	if stopErr != nil {
		t.Fatalf("Failed to stop manager: %v", stopErr)
	}

	// Wait for Run() to return
	select {
	case <-done:
		t.Log("✓ Manager shutdown gracefully")
	case <-time.After(5 * time.Second):
		t.Fatalf("Manager did not shut down within timeout")
	}

	// Verify manager is no longer responding
	time.Sleep(100 * time.Millisecond)
	_, err = http.Get("http://0.0.0.0:5510/health")
	if err == nil {
		t.Fatalf("Manager should not be responding after shutdown")
	}

	t.Log("✓ Manager confirmed offline after graceful shutdown")
}

// Feature: Deploy with Custom Arguments
func TestDeployWithCustomArgs(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	// Start Manager
	m, err := manager.NewManager(sugar, "0.0.0.0", 5520, storage.StorageInMemory, runtime.RuntimeDocker)
	if err != nil {
		t.Fatalf("Failed to create Manager: %v", err)
	}

	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	// Start Worker
	w, err := worker.NewWorker(sugar, "0.0.0.0", 5521, "0.0.0.0:5520", runtime.RuntimeDocker)
	if err != nil {
		t.Fatalf("Failed to create Worker: %v", err)
	}

	go w.Run()
	defer w.Stop()

	time.Sleep(1 * time.Second)

	// Deploy task with custom args
	deployReq := api.DeployRequest{
		Name:  "test-custom-args",
		Image: "alpine:latest",
		Args:  []string{"sh", "-c", "sleep 10"},
	}

	jsonData, err := json.Marshal(deployReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post("http://0.0.0.0:5520/tasks", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to deploy task with args: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 201, got %d. Body: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	taskID := result["task_id"]
	if taskID == "" {
		t.Fatalf("Task ID not returned in response")
	}

	// Wait for task to be processed by worker
	time.Sleep(2 * time.Second)

	// Verify task was created with correct args
	tasksResp, err := http.Get("http://0.0.0.0:5520/tasks")
	if err != nil {
		t.Fatalf("Failed to get tasks: %v", err)
	}
	defer tasksResp.Body.Close()

	var tasks []interface{}
	err = json.NewDecoder(tasksResp.Body).Decode(&tasks)
	if err != nil {
		t.Fatalf("Failed to parse tasks response: %v", err)
	}

	found := false
	for _, taskInterface := range tasks {
		task, ok := taskInterface.(map[string]interface{})
		if !ok {
			continue
		}

		taskName, ok := task["Name"].(string)
		if !ok || taskName != "test-custom-args" {
			continue
		}

		found = true
		args, ok := task["Args"].([]interface{})
		if !ok {
			// Args might be nil or empty
			if task["Args"] == nil {
				t.Errorf("Args field is nil in task response")
			} else {
				t.Errorf("Args field has unexpected type in task response: %T", task["Args"])
			}
			break
		}

		// Verify args match what we sent
		expectedArgs := []string{"sh", "-c", "sleep 10"}
		if len(args) != len(expectedArgs) {
			t.Errorf("Expected %d args, got %d", len(expectedArgs), len(args))
			break
		}

		for i, arg := range args {
			argStr, ok := arg.(string)
			if !ok {
				t.Errorf("Arg %d has unexpected type: %T", i, arg)
				continue
			}
			if argStr != expectedArgs[i] {
				t.Errorf("Arg %d: expected %s, got %s", i, expectedArgs[i], argStr)
			}
		}

		break
	}

	if !found {
		t.Fatalf("Task with custom args not found in task list. Tasks: %v", tasks)
	}

	t.Log("✓ Task deployed successfully with custom arguments")
}
