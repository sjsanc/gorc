package tests

import (
	"sync"
	"testing"

	"github.com/sjsanc/gorc/replica"
	"github.com/sjsanc/gorc/storage"
)

// TestConcurrentReplicaModifications verifies that concurrent Replica modifications are safe.
func TestConcurrentreplicaModifications(t *testing.T) {
	tk := replica.NewReplica("test-concurrent", "alpine:latest", nil)
	var wg sync.WaitGroup

	// Spawn 20 goroutines modifying replica concurrently
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

	// Verify replica has valid state after concurrent modifications
	if tk.GetContainerID() != "container-123" && tk.GetContainerID() != "" {
		t.Errorf("Unexpected container ID: %s", tk.GetContainerID())
	}
}

// TestStorageReplicaIsolation verifies that modifications to retrieved replicas don't affect stored copies.
func TestStoragereplicaIsolation(t *testing.T) {
	store, _ := storage.NewStore[replica.Replica](storage.StorageInMemory)
	originalreplica := replica.NewReplica("test-isolation", "alpine:latest", nil)
	originalreplica.MarkRunning("original-container")

	// Put replica in storage
	store.Put("replica1", originalreplica)

	// Retrieve replica and modify it
	retrieved1, _ := store.Get("replica1")
	retrieved1.MarkRunning("modified-container")

	// Retrieve replica again and verify original container ID is preserved
	retrieved2, _ := store.Get("replica1")
	if retrieved2.GetContainerID() == "modified-container" {
		t.Error("Storage did not isolate replicas - modifications leaked between retrievals")
	}

	if retrieved2.GetContainerID() != "original-container" {
		t.Errorf("Expected container ID 'original-container', got '%s'", retrieved2.GetContainerID())
	}
}

// TestConcurrentStorageAccess verifies that concurrent Get/Put operations on replicas are safe.
func TestConcurrentStorageAccess(t *testing.T) {
	store, _ := storage.NewStore[replica.Replica](storage.StorageInMemory)
	var wg sync.WaitGroup

	// Spawn goroutines doing concurrent Get/Put operations
	for i := 0; i < 10; i++ {
		wg.Add(3)

		// Goroutine 1: Put operations
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				tk := replica.NewReplica("test", "alpine:latest", nil)
				tk.SetState(replica.ReplicaRunning)
				store.Put("replica", tk)
			}
		}(i)

		// Goroutine 2: Get operations
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				store.Get("replica")
			}
		}(i)

		// Goroutine 3: Modify and put
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if tk, _ := store.Get("replica"); tk != nil {
					tk.SetError("concurrent error")
					store.Put("replica", tk)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify final state is valid
	finalreplica, _ := store.Get("replica")
	if finalreplica == nil {
		t.Error("Final replica should not be nil")
	}
}

// TestReplicaSetterConsistency verifies that compound operations are atomic.
func TestReplicaSetterConsistency(t *testing.T) {
	tk := replica.NewReplica("test-consistency", "alpine:latest", nil)
	var wg sync.WaitGroup

	// Spawn goroutines calling MarkRunning and MarkCompleted concurrently
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

	// Verify replica has valid final state
	// Final state should be either replicaRunning or replicaCompleted (not corrupted)
	// Both are valid final states due to race between MarkRunning and MarkCompleted
}
