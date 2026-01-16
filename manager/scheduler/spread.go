package scheduler

import (
	"fmt"
	"math"
	"time"

	"github.com/sjsanc/gorc/config"
	"github.com/sjsanc/gorc/node"
	"github.com/sjsanc/gorc/replica"
)

// Spread distributes replicas across nodes with HA and resource awareness.
type Spread struct {
	// Thresholds for considering a node overloaded
	CPULoadThreshold    float64
	MemoryThresholdPct  float64
	DiskThresholdPct    float64
	roundRobinScheduler *RoundRobin
}

// NewSpread creates a new spread scheduler with conservative thresholds.
func NewSpread() *Spread {
	return &Spread{
		CPULoadThreshold:    8.0,
		MemoryThresholdPct:  90.0,
		DiskThresholdPct:    95.0,
		roundRobinScheduler: NewRoundRobin(),
	}
}

// Schedule selects a worker using multi-factor scoring optimized for HA.
// This implements the Scheduler interface.
func (s *Spread) Schedule(
	r *replica.Replica,
	workers []*ManagedWorker,
	nodes []*node.Node,
	replicaDistribution map[string]int,
) (*ManagedWorker, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("no workers available for scheduling")
	}

	// Filter healthy workers
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

	// Filter non-overloaded workers
	candidates := make([]*ManagedWorker, 0, len(healthy))
	for _, w := range healthy {
		if w.Node == nil {
			// Node metrics not yet available, treat as healthy
			candidates = append(candidates, w)
			continue
		}

		if !s.isOverloaded(w.Node) {
			candidates = append(candidates, w)
		}
	}

	// If all nodes are overloaded, fall back to round-robin among healthy workers
	if len(candidates) == 0 {
		return s.roundRobinScheduler.Schedule(r, healthy, nodes, replicaDistribution)
	}

	// Calculate total replicas for the service from distribution map
	totalReplicas := 0
	for _, count := range replicaDistribution {
		totalReplicas += count
	}
	// Add 1 for the replica being scheduled
	totalReplicas++

	// Score and select best candidate
	bestWorker := candidates[0]
	bestScore := -1.0

	for _, w := range candidates {
		score := s.scoreWorker(w, replicaDistribution, totalReplicas)
		if score > bestScore {
			bestScore = score
			bestWorker = w
		}
	}

	return bestWorker, nil
}

// isOverloaded checks if a node exceeds any resource threshold.
func (s *Spread) isOverloaded(n *node.Node) bool {
	if n.Metrics == nil {
		return false
	}

	if n.Metrics.CPULoad.Load1 > s.CPULoadThreshold {
		return true
	}

	if n.Metrics.Memory.UsedPercent > s.MemoryThresholdPct {
		return true
	}

	if n.Metrics.Disk.UsedPercent > s.DiskThresholdPct {
		return true
	}

	return false
}

// scoreWorker calculates a composite score for a worker using HA and resource metrics.
func (s *Spread) scoreWorker(
	w *ManagedWorker,
	replicaDistribution map[string]int,
	totalReplicas int,
) float64 {
	// Calculate spread score: prefer nodes with fewer replicas
	spreadScore := s.calculateSpreadScore(w, replicaDistribution, totalReplicas)

	// Calculate resource score: prefer nodes with better utilization
	resourceScore := s.calculateResourceScore(w)

	// Weighted combination: 60% spread, 40% resource
	return (0.6 * spreadScore) + (0.4 * resourceScore)
}

// calculateSpreadScore gives higher scores to nodes with fewer replicas of this service.
func (s *Spread) calculateSpreadScore(
	w *ManagedWorker,
	replicaDistribution map[string]int,
	totalReplicas int,
) float64 {
	if totalReplicas == 0 {
		return 1.0
	}

	replicasOnNode := replicaDistribution[w.Node.Hostname]
	return 1.0 - (float64(replicasOnNode) / float64(totalReplicas))
}

// calculateResourceScore gives higher scores to nodes with better resource availability.
func (s *Spread) calculateResourceScore(w *ManagedWorker) float64 {
	if w.Node == nil || w.Node.Metrics == nil {
		// Treat missing metrics as healthy (score = 1.0)
		return 1.0
	}

	// Normalize CPU (threshold 8.0): values from 0 to 1
	cpuNorm := math.Min(w.Node.Metrics.CPULoad.Load1/s.CPULoadThreshold, 1.0)

	// Memory: already in percentage (0-100)
	memoryNorm := math.Min(w.Node.Metrics.Memory.UsedPercent/100.0, 1.0)

	// Disk: already in percentage (0-100)
	diskNorm := math.Min(w.Node.Metrics.Disk.UsedPercent/100.0, 1.0)

	// Weighted utilization
	utilization := (0.5 * cpuNorm) + (0.3 * memoryNorm) + (0.2 * diskNorm)

	// Return inverse: higher score for lower utilization
	return 1.0 - utilization
}
