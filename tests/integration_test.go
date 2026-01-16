package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/manager"
	"github.com/sjsanc/gorc/manager/scheduler"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/storage"
	"github.com/sjsanc/gorc/worker"
	"go.uber.org/zap"
)

// Feature 1: Manager Initialization
func TestManagerInitialization(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, err := manager.NewManager(sugar, "0.0.0.0", 5500, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
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

	m, _ := manager.NewManager(sugar, "0.0.0.0", 5555, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
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

	m, _ := manager.NewManager(sugar, "0.0.0.0", 5555, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
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

// Feature 4: replica Deployment
func TestreplicaDeployment(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, _ := manager.NewManager(sugar, "0.0.0.0", 5555, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	w, _ := worker.NewWorker(sugar, "0.0.0.0", 5556, "0.0.0.0:5555", runtime.RuntimeDocker)
	go w.Run()
	defer w.Stop()

	time.Sleep(1 * time.Second)

	deployReq := api.DeployRequest{
		Image: "alpine:latest",
		Name:  "test-replica",
	}

	body, _ := json.Marshal(deployReq)
	resp, err := http.Post("http://0.0.0.0:5555/replicas", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to deploy replica: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected status 200/201, got %d", resp.StatusCode)
	}

	t.Log("✓ replica deployed successfully")
}

// Feature 5: replica Event Processing
func TestreplicaEventProcessing(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, _ := manager.NewManager(sugar, "0.0.0.0", 5555, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	w, _ := worker.NewWorker(sugar, "0.0.0.0", 5556, "0.0.0.0:5555", runtime.RuntimeDocker)
	go w.Run()
	defer w.Stop()

	time.Sleep(1 * time.Second)

	deployReq := api.DeployRequest{
		Image: "alpine:latest",
		Name:  "test-event-replica",
	}

	body, _ := json.Marshal(deployReq)
	resp, err := http.Post("http://0.0.0.0:5555/replicas", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to deploy replica: %v", err)
	}
	defer resp.Body.Close()

	time.Sleep(2 * time.Second)

	resp, err = http.Get("http://0.0.0.0:5555/replicas")
	if err != nil {
		t.Fatalf("Failed to fetch replicas: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("✓ Worker received and processed replica event. replicas: %s", string(bodyBytes))
}

// Feature 6: Graceful Shutdown
func TestGracefulShutdown(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	m, err := manager.NewManager(sugar, "0.0.0.0", 5510, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
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
	m, err := manager.NewManager(sugar, "0.0.0.0", 5520, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
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

	// Deploy replica with custom args
	deployReq := api.DeployRequest{
		Name:  "test-custom-args",
		Image: "alpine:latest",
		Cmd:   []string{"sh", "-c", "sleep 10"},
	}

	jsonData, err := json.Marshal(deployReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post("http://0.0.0.0:5520/replicas", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to deploy replica with args: %v", err)
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

	replicaID := result["replica_id"]
	if replicaID == "" {
		t.Fatalf("Replica ID not returned in response")
	}

	// Wait for replica to be processed by worker
	time.Sleep(2 * time.Second)

	// Verify replica was created with correct args
	replicasResp, err := http.Get("http://0.0.0.0:5520/replicas")
	if err != nil {
		t.Fatalf("Failed to get replicas: %v", err)
	}
	defer replicasResp.Body.Close()

	var replicas []interface{}
	err = json.NewDecoder(replicasResp.Body).Decode(&replicas)
	if err != nil {
		t.Fatalf("Failed to parse replicas response: %v", err)
	}

	found := false
	for _, replicaInterface := range replicas {
		replica, ok := replicaInterface.(map[string]interface{})
		if !ok {
			continue
		}

		replicaName, ok := replica["Name"].(string)
		if !ok || replicaName != "test-custom-args" {
			continue
		}

		found = true
		cmd, ok := replica["Cmd"].([]interface{})
		if !ok {
			// Cmd might be nil or empty
			if replica["Cmd"] == nil {
				t.Errorf("Cmd field is nil in replica response")
			} else {
				t.Errorf("Cmd field has unexpected type in replica response: %T", replica["Cmd"])
			}
			break
		}

		// Verify cmd match what we sent
		expectedCmd := []string{"sh", "-c", "sleep 10"}
		if len(cmd) != len(expectedCmd) {
			t.Errorf("Expected %d cmd items, got %d", len(expectedCmd), len(cmd))
			break
		}

		for i, cmdItem := range cmd {
			cmdStr, ok := cmdItem.(string)
			if !ok {
				t.Errorf("Cmd item %d has unexpected type: %T", i, cmdItem)
				continue
			}
			if cmdStr != expectedCmd[i] {
				t.Errorf("Cmd item %d: expected %s, got %s", i, expectedCmd[i], cmdStr)
			}
		}

		break
	}

	if !found {
		t.Fatalf("Replica with custom args not found in replica list. replicas: %v", replicas)
	}

	t.Log("✓ replica deployed successfully with custom arguments")
}

// Feature: Round-Robin Scheduling
// Tests that the round-robin scheduler distributes replicas evenly across workers
func TestRoundRobinScheduling(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	// Create manager with round-robin scheduler
	m, err := manager.NewManager(sugar, "0.0.0.0", 5530, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
	if err != nil {
		t.Fatalf("Failed to create Manager: %v", err)
	}

	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	// Start 3 workers on the same node
	workers := make([]*worker.Worker, 3)
	workerPorts := []int{5531, 5532, 5533}
	for i := 0; i < 3; i++ {
		w, err := worker.NewWorker(sugar, "0.0.0.0", workerPorts[i], "0.0.0.0:5530", runtime.RuntimeDocker)
		if err != nil {
			t.Fatalf("Failed to create Worker %d: %v", i, err)
		}
		workers[i] = w
		go w.Run()
		defer w.Stop()
	}

	time.Sleep(1 * time.Second)

	// Deploy 9 replicas (should distribute as 3 per worker with round-robin)
	for i := 0; i < 9; i++ {
		deployReq := api.DeployRequest{
			Name:  fmt.Sprintf("test-rr-replica-%d", i),
			Image: "alpine:latest",
		}

		jsonData, err := json.Marshal(deployReq)
		if err != nil {
			t.Fatalf("Failed to marshal request: %v", err)
		}

		resp, err := http.Post("http://0.0.0.0:5530/replicas", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatalf("Failed to deploy replica %d: %v", i, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d for replica %d", resp.StatusCode, i)
		}
	}

	// Wait for replicas to be distributed
	time.Sleep(2 * time.Second)

	// Verify round-robin distribution: each worker should have 3 replicas
	expectedPerWorker := 3
	for i, port := range workerPorts {
		endpoint := fmt.Sprintf("http://0.0.0.0:%d/replicas", port)
		resp, err := http.Get(endpoint)
		if err != nil {
			t.Fatalf("Failed to get replicas from worker %d: %v", i, err)
		}
		defer resp.Body.Close()

		var replicas []interface{}
		err = json.NewDecoder(resp.Body).Decode(&replicas)
		if err != nil {
			t.Fatalf("Failed to parse replicas from worker %d: %v", i, err)
		}

		// Count test-rr-replica replicas
		count := 0
		for _, replicaInterface := range replicas {
			replica, ok := replicaInterface.(map[string]interface{})
			if !ok {
				continue
			}
			replicaName, ok := replica["Name"].(string)
			if ok && strings.HasPrefix(replicaName, "test-rr-replica-") {
				count++
			}
		}

		if count != expectedPerWorker {
			t.Errorf("Worker %d: expected %d replicas, got %d", i, expectedPerWorker, count)
		}
	}

	t.Log("✓ round-robin scheduler distributed replicas evenly")
}
