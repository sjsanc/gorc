package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
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

// Test that the reconciliation loop doesn't create infinite replicas when no workers are available
func TestNoWorkerInfiniteLoop(t *testing.T) {
	logger := zap.NewNop()
	sugar := logger.Sugar()

	// Start Manager
	m, err := manager.NewManager(sugar, "0.0.0.0", 5600, storage.StorageInMemory, runtime.RuntimeDocker, scheduler.TypeRoundRobin)
	if err != nil {
		t.Fatalf("Failed to create Manager: %v", err)
	}

	go m.Run()
	defer m.Stop()

	time.Sleep(500 * time.Millisecond)

	// Start Worker
	w, err := worker.NewWorker(sugar, "0.0.0.0", 5601, "0.0.0.0:5600", runtime.RuntimeDocker)
	if err != nil {
		t.Fatalf("Failed to create Worker: %v", err)
	}

	go w.Run()

	time.Sleep(1 * time.Second)

	// Deploy service with 3 replicas
	serviceReq := api.CreateServiceRequest{
		Name:          "test-service",
		Image:         "alpine:latest",
		Cmd:           []string{"sh", "-c", "sleep infinity"},
		Replicas:      3,
		RestartPolicy: "on-failure",
	}

	jsonData, _ := json.Marshal(serviceReq)
	resp, err := http.Post("http://0.0.0.0:5600/services", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to deploy service: %v", err)
	}
	resp.Body.Close()

	// Wait for replicas to be created
	time.Sleep(15 * time.Second)

	// Verify we have 3 replicas running
	resp, _ = http.Get("http://0.0.0.0:5600/replicas")
	var replicas1 []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&replicas1)
	resp.Body.Close()

	running1 := 0
	for _, r := range replicas1 {
		if int(r["State"].(float64)) == 1 {
			running1++
		}
	}

	if running1 != 3 {
		t.Fatalf("Expected 3 running replicas initially, got %d", running1)
	}
	t.Logf("✓ Initial state: %d replicas running", running1)

	// Kill the worker
	w.Stop()
	t.Logf("✓ Worker stopped")

	// Wait for dead worker detection (30s timeout + 10s check)
	time.Sleep(40 * time.Second)

	// Count replicas after worker death
	resp, _ = http.Get("http://0.0.0.0:5600/replicas")
	var replicas2 []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&replicas2)
	resp.Body.Close()

	count2 := len(replicas2)
	t.Logf("Replica count after worker death: %d", count2)

	// Wait through 3 more reconciliation cycles (30 seconds)
	time.Sleep(30 * time.Second)

	// Count replicas again
	resp, _ = http.Get("http://0.0.0.0:5600/replicas")
	var replicas3 []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&replicas3)
	resp.Body.Close()

	count3 := len(replicas3)
	t.Logf("Replica count after 3 more reconciliation cycles: %d", count3)

	// The count should NOT increase - we should not be creating replicas when no workers are available
	if count3 > count2 {
		t.Errorf("BUG CONFIRMED: Replica count increased from %d to %d with no workers available!", count2, count3)

		// Show the replica states
		for i, r := range replicas3 {
			state := int(r["State"].(float64))
			stateNames := []string{"Pending", "Running", "Completed", "Failed"}
			t.Logf("  Replica %d: State=%s, Error=%v", i, stateNames[state], r["Error"])
		}
	} else {
		t.Logf("✓ Replica count stable at %d (not creating infinite replicas)", count3)
	}
}
