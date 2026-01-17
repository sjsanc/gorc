package tests

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sjsanc/gorc/api"
	"github.com/sjsanc/gorc/replica"
	"github.com/sjsanc/gorc/storage"
	"github.com/sjsanc/gorc/tests/helpers"
)

func TestManager(t *testing.T) {
	t.Run("Initialization", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5500), helpers.WithNoWorkers())
		defer cluster.Stop()

		if !cluster.Client().HealthCheck() {
			t.Fatal("Manager not responding to health check")
		}
	})

	t.Run("GracefulShutdown", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5510), helpers.WithNoWorkers())

		if !cluster.Client().HealthCheck() {
			t.Fatal("Manager not responding before shutdown")
		}

		if err := cluster.Manager.Stop(); err != nil {
			t.Fatalf("Failed to stop manager: %v", err)
		}

		cluster.WaitForManagerDone(5 * time.Second)

		time.Sleep(100 * time.Millisecond)
		_, err := http.Get("http://0.0.0.0:5510/health")
		if err == nil {
			t.Fatal("Manager should not respond after shutdown")
		}
	})
}

func TestWorker(t *testing.T) {
	t.Run("Registration", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5555))
		defer cluster.Stop()

		workers := cluster.Client().GetWorkers()
		if len(workers) == 0 {
			t.Fatalf("Expected at least 1 worker, got %d", len(workers))
		}
	})

	t.Run("NodeRegistryCreation", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5556))
		defer cluster.Stop()

		nodes := cluster.Client().GetNodes()
		if len(nodes) < 1 {
			t.Fatalf("Expected at least 1 node, got %d", len(nodes))
		}
	})
}

func TestReplica(t *testing.T) {
	t.Run("Deployment", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5557))
		defer cluster.Stop()

		resp := cluster.Client().Post("/replicas", api.DeployRequest{
			Image: "alpine:latest",
			Name:  "test-replica",
		})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Fatalf("Expected status 200/201, got %d", resp.StatusCode)
		}
	})

	t.Run("EventProcessing", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5558))
		defer cluster.Stop()

		cluster.Client().DeployReplica(api.DeployRequest{
			Image: "alpine:latest",
			Name:  "test-event-replica",
		})

		time.Sleep(2 * time.Second)

		replicas := cluster.Client().GetReplicas()
		if len(replicas) == 0 {
			t.Fatal("Expected replicas after event processing")
		}
	})

	t.Run("CustomArgs", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5520))
		defer cluster.Stop()

		replicaID := cluster.Client().DeployReplica(api.DeployRequest{
			Name:  "test-custom-args",
			Image: "alpine:latest",
			Cmd:   []string{"sh", "-c", "sleep 10"},
		})

		if replicaID == "" {
			t.Fatal("Replica ID not returned")
		}

		time.Sleep(2 * time.Second)

		replicas := cluster.Client().GetReplicasRaw()
		found := false
		expectedCmd := []string{"sh", "-c", "sleep 10"}

		for _, r := range replicas {
			name, ok := r["Name"].(string)
			if !ok || name != "test-custom-args" {
				continue
			}

			found = true
			cmd, ok := r["Cmd"].([]interface{})
			if !ok {
				t.Errorf("Cmd field has unexpected type: %T", r["Cmd"])
				break
			}

			if len(cmd) != len(expectedCmd) {
				t.Errorf("Expected %d cmd items, got %d", len(expectedCmd), len(cmd))
				break
			}

			for i, item := range cmd {
				if s, ok := item.(string); !ok || s != expectedCmd[i] {
					t.Errorf("Cmd[%d]: expected %s, got %v", i, expectedCmd[i], item)
				}
			}
			break
		}

		if !found {
			t.Fatalf("Replica with custom args not found")
		}
	})
}

func TestScheduler(t *testing.T) {
	t.Run("RoundRobin", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5530), helpers.WithWorkerCount(3))
		defer cluster.Stop()

		for i := 0; i < 9; i++ {
			resp := cluster.Client().Post("/replicas", api.DeployRequest{
				Name:  "test-rr-replica-" + helpers.Itoa(i),
				Image: "alpine:latest",
			})
			resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("Expected status 201, got %d for replica %d", resp.StatusCode, i)
			}
		}

		time.Sleep(2 * time.Second)

		expectedPerWorker := 3
		for i := 0; i < 3; i++ {
			replicas := cluster.WorkerClient(i).GetReplicasRaw()
			count := 0
			for _, r := range replicas {
				name, ok := r["Name"].(string)
				if ok && strings.HasPrefix(name, "test-rr-replica-") {
					count++
				}
			}
			if count != expectedPerWorker {
				t.Errorf("Worker %d: expected %d replicas, got %d", i, expectedPerWorker, count)
			}
		}
	})

	t.Run("NoWorkerAvailable", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5600), helpers.WithWorkerCount(1))

		time.Sleep(1 * time.Second)

		cluster.Client().CreateService(api.CreateServiceRequest{
			Name:          "test-service",
			Image:         "alpine:latest",
			Cmd:           []string{"sh", "-c", "sleep infinity"},
			Replicas:      3,
			RestartPolicy: "on-failure",
		})

		time.Sleep(15 * time.Second)

		replicas1 := cluster.Client().GetReplicasRaw()
		running1 := 0
		for _, r := range replicas1 {
			if int(r["State"].(float64)) == 1 {
				running1++
			}
		}
		if running1 != 3 {
			t.Fatalf("Expected 3 running replicas initially, got %d", running1)
		}

		cluster.StopWorker(0)

		time.Sleep(40 * time.Second)

		replicas2 := cluster.Client().GetReplicasRaw()
		count2 := len(replicas2)

		time.Sleep(30 * time.Second)

		replicas3 := cluster.Client().GetReplicasRaw()
		count3 := len(replicas3)

		if count3 > count2 {
			t.Errorf("Replica count increased from %d to %d with no workers available", count2, count3)
		}

		cluster.Manager.Stop()
	})
}

func TestFailure(t *testing.T) {
	t.Run("WorkerRescheduling", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5540), helpers.WithWorkerCount(1))

		time.Sleep(1 * time.Second)

		cluster.Client().CreateService(api.CreateServiceRequest{
			Name:          "test-service",
			Image:         "alpine:latest",
			Cmd:           []string{"sh", "-c", "sleep infinity"},
			Replicas:      3,
			RestartPolicy: "on-failure",
		})

		time.Sleep(15 * time.Second)

		replicas := cluster.Client().GetReplicasRaw()
		runningCount := 0
		for _, r := range replicas {
			if int(r["State"].(float64)) == 1 {
				runningCount++
			}
		}
		if runningCount != 3 {
			t.Fatalf("Expected 3 running replicas, got %d", runningCount)
		}

		cluster.StopWorker(0)

		time.Sleep(35 * time.Second)

		workers := cluster.Client().GetWorkers()
		if len(workers) != 0 {
			t.Fatalf("Expected 0 workers after failure, got %d", len(workers))
		}

		replicas = cluster.Client().GetReplicasRaw()
		failedCount := 0
		for _, r := range replicas {
			if int(r["State"].(float64)) == 3 { // ReplicaFailed
				failedCount++
				errorMsg, ok := r["Error"].(string)
				if !ok || !strings.Contains(errorMsg, "worker died") {
					t.Errorf("Expected error 'worker died', got: %v", r["Error"])
				}
			}
		}
		if failedCount != 3 {
			t.Fatalf("Expected 3 failed replicas, got %d", failedCount)
		}

		cluster.AddWorker()

		time.Sleep(15 * time.Second)

		replicas = cluster.Client().GetReplicasRaw()
		newRunningCount := 0
		for _, r := range replicas {
			if int(r["State"].(float64)) == 1 {
				newRunningCount++
			}
		}
		if newRunningCount < 3 {
			t.Fatalf("Expected at least 3 running replicas after rescheduling, got %d", newRunningCount)
		}

		cluster.Stop()
	})

	t.Run("ExitCode137", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5550), helpers.WithWorkerCount(1))

		time.Sleep(1 * time.Second)

		cluster.Client().CreateService(api.CreateServiceRequest{
			Name:          "test-exit-code-service",
			Image:         "alpine:latest",
			Cmd:           []string{"sh", "-c", "sleep 30"},
			Replicas:      2,
			RestartPolicy: "never",
		})

		time.Sleep(15 * time.Second)

		replicas := cluster.Client().GetReplicasRaw()
		runningCount := 0
		for _, r := range replicas {
			serviceName, ok := r["ServiceName"].(string)
			if ok && serviceName == "test-exit-code-service" && int(r["State"].(float64)) == 1 {
				runningCount++
			}
		}
		if runningCount != 2 {
			t.Fatalf("Expected 2 running replicas, got %d", runningCount)
		}

		cluster.StopWorker(0)

		time.Sleep(3 * time.Second)
		cluster.Manager.Stop()
	})

	t.Run("CrashExitCode", func(t *testing.T) {
		cluster := helpers.NewTestCluster(t, helpers.WithManagerPort(5560))
		defer cluster.Stop()

		cluster.Client().CreateService(api.CreateServiceRequest{
			Name:          "test-crash-service",
			Image:         "alpine:latest",
			Cmd:           []string{"sh", "-c", "exit 1"},
			Replicas:      1,
			RestartPolicy: "never",
		})

		time.Sleep(15 * time.Second)

		replicas := cluster.Client().GetReplicasRaw()
		failedCount := 0
		for _, r := range replicas {
			serviceName, ok := r["ServiceName"].(string)
			if !ok || serviceName != "test-crash-service" {
				continue
			}
			if int(r["State"].(float64)) == 3 { // ReplicaFailed
				failedCount++
				errorMsg, ok := r["Error"].(string)
				if !ok || !strings.Contains(errorMsg, "exited with code 1") {
					t.Errorf("Expected error 'exited with code 1', got: %v", r["Error"])
				}
			}
		}
		if failedCount != 1 {
			t.Fatalf("Expected 1 failed replica, got %d", failedCount)
		}
	})
}

func TestConcurrency(t *testing.T) {
	t.Run("ReplicaModifications", func(t *testing.T) {
		tk := replica.NewReplica("test-concurrent", "alpine:latest", nil)
		var wg sync.WaitGroup

		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				if id%2 == 0 {
					tk.MarkRunning("container-123")
					tk.MarkCompleted()
				} else {
					tk.SetState(replica.ReplicaRunning)
					tk.SetError("test error")
				}
			}(i)
		}
		wg.Wait()

		if tk.GetContainerID() != "container-123" && tk.GetContainerID() != "" {
			t.Errorf("Unexpected container ID: %s", tk.GetContainerID())
		}
	})

	t.Run("StorageIsolation", func(t *testing.T) {
		store, _ := storage.NewStore[replica.Replica](storage.StorageInMemory)
		originalReplica := replica.NewReplica("test-isolation", "alpine:latest", nil)
		originalReplica.MarkRunning("original-container")

		store.Put("replica1", originalReplica)

		retrieved1, _ := store.Get("replica1")
		retrieved1.MarkRunning("modified-container")

		retrieved2, _ := store.Get("replica1")
		if retrieved2.GetContainerID() == "modified-container" {
			t.Error("Storage did not isolate replicas - modifications leaked")
		}
		if retrieved2.GetContainerID() != "original-container" {
			t.Errorf("Expected 'original-container', got '%s'", retrieved2.GetContainerID())
		}
	})

	t.Run("StorageAccess", func(t *testing.T) {
		store, _ := storage.NewStore[replica.Replica](storage.StorageInMemory)
		var wg sync.WaitGroup

		for i := 0; i < 10; i++ {
			wg.Add(3)

			go func() {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					tk := replica.NewReplica("test", "alpine:latest", nil)
					tk.SetState(replica.ReplicaRunning)
					store.Put("replica", tk)
				}
			}()

			go func() {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					store.Get("replica")
				}
			}()

			go func() {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					if tk, _ := store.Get("replica"); tk != nil {
						tk.SetError("concurrent error")
						store.Put("replica", tk)
					}
				}
			}()
		}

		wg.Wait()

		finalReplica, _ := store.Get("replica")
		if finalReplica == nil {
			t.Error("Final replica should not be nil")
		}
	})

	t.Run("SetterConsistency", func(t *testing.T) {
		tk := replica.NewReplica("test-consistency", "alpine:latest", nil)
		var wg sync.WaitGroup

		for i := 0; i < 10; i++ {
			wg.Add(2)

			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					tk.MarkRunning("container-abc")
				}
			}()

			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					tk.MarkCompleted()
				}
			}()
		}

		wg.Wait()
	})
}
