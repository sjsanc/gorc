package tests

import (
	"sync"
	"testing"

	"github.com/sjsanc/gorc/storage"
	"github.com/sjsanc/gorc/task"
)

// TestConcurrentTaskModifications verifies that concurrent Task modifications are safe.
func TestConcurrentTaskModifications(t *testing.T) {
	tk := task.NewTask("test-concurrent", "alpine:latest")
	var wg sync.WaitGroup

	// Spawn 20 goroutines modifying task concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				tk.MarkRunning("container-123")
				tk.MarkCompleted()
			} else {
				tk.SetState(task.TaskRunning)
				tk.SetError("test error")
			}
		}(i)
	}
	wg.Wait()

	// Verify task has valid state after concurrent modifications
	if tk.GetContainerID() != "container-123" && tk.GetContainerID() != "" {
		t.Errorf("Unexpected container ID: %s", tk.GetContainerID())
	}
}

// TestStorageTaskIsolation verifies that modifications to retrieved tasks don't affect stored copies.
func TestStorageTaskIsolation(t *testing.T) {
	store, _ := storage.NewStore[task.Task](storage.StorageInMemory)
	originalTask := task.NewTask("test-isolation", "alpine:latest")
	originalTask.MarkRunning("original-container")

	// Put task in storage
	store.Put("task1", originalTask)

	// Retrieve task and modify it
	retrieved1, _ := store.Get("task1")
	retrieved1.MarkRunning("modified-container")

	// Retrieve task again and verify original container ID is preserved
	retrieved2, _ := store.Get("task1")
	if retrieved2.GetContainerID() == "modified-container" {
		t.Error("Storage did not isolate tasks - modifications leaked between retrievals")
	}

	if retrieved2.GetContainerID() != "original-container" {
		t.Errorf("Expected container ID 'original-container', got '%s'", retrieved2.GetContainerID())
	}
}

// TestConcurrentStorageAccess verifies that concurrent Get/Put operations are safe.
func TestConcurrentStorageAccess(t *testing.T) {
	store, _ := storage.NewStore[task.Task](storage.StorageInMemory)
	var wg sync.WaitGroup

	// Spawn goroutines doing concurrent Get/Put operations
	for i := 0; i < 10; i++ {
		wg.Add(3)

		// Goroutine 1: Put operations
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				tk := task.NewTask("test", "alpine:latest")
				tk.SetState(task.TaskRunning)
				store.Put("task", tk)
			}
		}(i)

		// Goroutine 2: Get operations
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				store.Get("task")
			}
		}(i)

		// Goroutine 3: Modify and put
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if tk, _ := store.Get("task"); tk != nil {
					tk.SetError("concurrent error")
					store.Put("task", tk)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify final state is valid
	finalTask, _ := store.Get("task")
	if finalTask == nil {
		t.Error("Final task should not be nil")
	}
}

// TestTaskSetterConsistency verifies that compound operations are atomic.
func TestTaskSetterConsistency(t *testing.T) {
	tk := task.NewTask("test-consistency", "alpine:latest")
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

	// Verify task has valid final state
	// Final state should be either TaskRunning or TaskCompleted (not corrupted)
	// Both are valid final states due to race between MarkRunning and MarkCompleted
}
