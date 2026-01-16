package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sjsanc/gorc/node"
	"github.com/sjsanc/gorc/replica"
)

type Type string

const (
	TypeRoundRobin Type = "roundrobin"
	TypeSpread     Type = "spread"
)

// Scheduler determines which worker should run a replica.
type Scheduler interface {
	// Schedule assigns a replica to a worker based on the scheduling strategy.
	// workers: available workers
	// nodes: nodes in the cluster with metrics
	// replicaDistribution: map of hostname -> count of replicas for this service
	Schedule(
		r *replica.Replica,
		workers []*ManagedWorker,
		nodes []*node.Node,
		replicaDistribution map[string]int,
	) (*ManagedWorker, error)
}

// ManagedWorker represents a managed worker as seen by the scheduler.
type ManagedWorker struct {
	ID            uuid.UUID
	Name          string
	Address       string
	Port          int
	Node          *node.Node
	LastHeartbeat time.Time
}

// Parse parses a scheduler type string.
func Parse(s string) (Type, error) {
	t := Type(strings.ToLower(s))
	switch t {
	case TypeRoundRobin, TypeSpread:
		return t, nil
	default:
		return "", fmt.Errorf("unknown scheduler type: %s", s)
	}
}

// New creates a new scheduler of the specified type.
func New(schedulerType Type) (Scheduler, error) {
	switch schedulerType {
	case TypeRoundRobin:
		return NewRoundRobin(), nil
	case TypeSpread:
		return NewSpread(), nil
	default:
		return nil, fmt.Errorf("unknown scheduler type: %v", schedulerType)
	}
}
