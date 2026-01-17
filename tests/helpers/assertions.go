package helpers

import (
	"net/http"
	"testing"
	"time"

	"github.com/sjsanc/gorc/replica"
)

type ReplicaInfo struct {
	ID          string               `json:"ID"`
	Name        string               `json:"Name"`
	Image       string               `json:"Image"`
	Cmd         []string             `json:"Cmd"`
	State       replica.ReplicaState `json:"State"`
	ContainerID string               `json:"ContainerID"`
	WorkerID    string               `json:"WorkerID"`
	ServiceID   string               `json:"ServiceID"`
	ServiceName string               `json:"ServiceName"`
	ReplicaID   int                  `json:"ReplicaID"`
	Error       string               `json:"Error"`
}

func WaitFor(t *testing.T, timeout time.Duration, condition func() bool, msg string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for condition: %s", msg)
}

func RequireStatus(t *testing.T, resp *http.Response, expected int) {
	if resp.StatusCode != expected {
		t.Fatalf("Expected status %d, got %d", expected, resp.StatusCode)
	}
}

func CountReplicasByState(replicas []ReplicaInfo, state replica.ReplicaState) int {
	count := 0
	for _, r := range replicas {
		if r.State == state {
			count++
		}
	}
	return count
}

func CountReplicasByStateAndService(replicas []ReplicaInfo, state replica.ReplicaState, serviceName string) int {
	count := 0
	for _, r := range replicas {
		if r.State == state && r.ServiceName == serviceName {
			count++
		}
	}
	return count
}

func CountReplicasByNamePrefix(replicas []ReplicaInfo, prefix string) int {
	count := 0
	for _, r := range replicas {
		if len(r.Name) >= len(prefix) && r.Name[:len(prefix)] == prefix {
			count++
		}
	}
	return count
}

func FilterReplicasByService(replicas []ReplicaInfo, serviceName string) []ReplicaInfo {
	result := make([]ReplicaInfo, 0)
	for _, r := range replicas {
		if r.ServiceName == serviceName {
			result = append(result, r)
		}
	}
	return result
}
