package scheduler

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/sjsanc/gorc/config"
	"github.com/sjsanc/gorc/node"
	"github.com/sjsanc/gorc/replica"
)

// RoundRobin distributes replicas fairly across workers using a rotating counter.
type RoundRobin struct {
	counter atomic.Uint64
}

// NewRoundRobin creates a new round-robin scheduler.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{
		counter: atomic.Uint64{},
	}
}

// Schedule selects a worker using round-robin distribution, skipping unhealthy workers.
// This implements the Scheduler interface.
func (s *RoundRobin) Schedule(
	r *replica.Replica,
	workers []*ManagedWorker,
	nodes []*node.Node,
	replicaDistribution map[string]int,
) (*ManagedWorker, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers available for scheduling")
	}

	// Filter healthy workers (exclude those with stale heartbeats)
	now := time.Now()
	healthy := make([]*ManagedWorker, 0, len(workers))
	for _, w := range workers {
		if now.Sub(w.LastHeartbeat) <= config.DefaultHeartbeatTimeout {
			healthy = append(healthy, w)
		}
	}

	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy workers available for scheduling")
	}

	// Use atomic counter for thread-safe round-robin
	idx := s.counter.Add(1) - 1
	selected := healthy[idx%uint64(len(healthy))]

	return selected, nil
}
