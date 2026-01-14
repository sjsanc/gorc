package manager

import (
	"fmt"
	"strings"
	"time"

	replica "github.com/sjsanc/gorc/replica"
	"github.com/sjsanc/gorc/service"
)

const DefaultReconcileInterval = 10 * time.Second

// startReconciliationLoop runs in the background to reconcile desired vs actual state.
func (m *Manager) startReconciliationLoop() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(DefaultReconcileInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.reconcileServices()
			}
		}
	}()
}

// reconcileServices iterates all services and reconciles each one.
func (m *Manager) reconcileServices() {
	services, err := m.listServices()
	if err != nil {
		m.logger.Errorf("reconciliation: failed to list services: %v", err)
		return
	}

	for _, svc := range services {
		if err := m.reconcileService(svc); err != nil {
			m.logger.Errorf("reconciliation: service %s failed: %v", svc.Name, err)
		}
	}
}

// reconcileService reconciles a single service by comparing desired vs actual replicas.
func (m *Manager) reconcileService(svc *service.Service) error {
	replicas, err := m.getReplicasForService(svc.ID)
	if err != nil {
		return fmt.Errorf("failed to get replicas: %v", err)
	}

	// Categorize replicas by state
	var running, pending, failed []*replica.Replica
	for _, r := range replicas {
		switch r.State {
		case replica.ReplicaRunning:
			running = append(running, r)
		case replica.ReplicaPending:
			pending = append(pending, r)
		case replica.ReplicaFailed:
			failed = append(failed, r)
			// ReplicaCompleted is ignored (not a running replica)
		}
	}

	desiredReplicas := svc.Replicas
	actualReplicas := len(running) + len(pending)

	// Handle failed replicas based on restart policy
	for _, failedReplica := range failed {
		if m.shouldRestart(svc, failedReplica) {
			m.logger.Infof("reconciliation: restarting failed replica %s (service: %s)", failedReplica.Name, svc.Name)
			newReplica := m.createReplicaForService(svc, failedReplica.ReplicaID)
			if err := m.scheduleReplica(newReplica); err != nil {
				m.logger.Errorf("failed to restart replica: %v", err)
			}
		}
	}

	// Scale up: create missing replicas
	if actualReplicas < desiredReplicas {
		deficit := desiredReplicas - actualReplicas
		m.logger.Infof("reconciliation: scaling up service %s (%d -> %d replicas)", svc.Name, actualReplicas, desiredReplicas)

		for i := 0; i < deficit; i++ {
			replicaID := m.nextReplicaID(replicas)
			newReplica := m.createReplicaForService(svc, replicaID)
			if err := m.scheduleReplica(newReplica); err != nil {
				m.logger.Errorf("failed to scale up: %v", err)
				break
			}
		}
	}

	// Scale down: stop excess replicas
	if actualReplicas > desiredReplicas {
		excess := actualReplicas - desiredReplicas
		m.logger.Infof("reconciliation: scaling down service %s (%d -> %d replicas)", svc.Name, actualReplicas, desiredReplicas)

		replicasToStop := running
		if len(replicasToStop) > excess {
			replicasToStop = replicasToStop[:excess]
		}

		for _, r := range replicasToStop {
			if err := m.stopReplica(r.ID); err != nil {
				m.logger.Errorf("failed to stop replica: %v", err)
			}
		}
	}

	// Handle config changes (image update, args update)
	if m.serviceConfigChanged(svc, running) {
		m.logger.Infof("reconciliation: service %s config changed, rolling restart", svc.Name)
		for _, r := range running {
			// Stop old replica
			if err := m.stopReplica(r.ID); err != nil {
				m.logger.Errorf("failed to stop outdated replica: %v", err)
				continue
			}
			// Create new replica with updated config
			newReplica := m.createReplicaForService(svc, r.ReplicaID)
			if err := m.scheduleReplica(newReplica); err != nil {
				m.logger.Errorf("failed to start updated replica: %v", err)
			}
		}
	}

	return nil
}

// shouldRestart evaluates the restart policy for a failed replica.
func (m *Manager) shouldRestart(svc *service.Service, failedReplica *replica.Replica) bool {
	switch svc.RestartPolicy {
	case service.RestartAlways:
		return true
	case service.RestartOnFailure:
		// Only restart if it was a genuine failure, not a manual stop
		return failedReplica.Error != "" && !strings.Contains(failedReplica.Error, "stopped")
	case service.RestartNever:
		return false
	default:
		return false
	}
}

// createReplicaForService creates a new replica for a service.
func (m *Manager) createReplicaForService(svc *service.Service, replicaID int) *replica.Replica {
	var replicaName string
	if svc.AppName != "" {
		replicaName = fmt.Sprintf("%s-%s-%d", svc.AppName, svc.Name, replicaID)
	} else {
		replicaName = fmt.Sprintf("%s-%d", svc.Name, replicaID)
	}
	r := replica.NewReplica(replicaName, svc.Image, svc.Cmd)
	r.ServiceID = svc.ID
	r.ServiceName = svc.Name
	r.ReplicaID = replicaID

	// Store replica
	m.replicas.Put(r.ID.String(), r)
	return r
}

// nextReplicaID allocates the next replica ID (monotonically increasing).
func (m *Manager) nextReplicaID(existingReplicas []*replica.Replica) int {
	maxID := -1
	for _, r := range existingReplicas {
		if r.ReplicaID > maxID {
			maxID = r.ReplicaID
		}
	}
	return maxID + 1
}

// serviceConfigChanged detects if running replicas have diverged from the service spec.
func (m *Manager) serviceConfigChanged(svc *service.Service, runningReplicas []*replica.Replica) bool {
	for _, r := range runningReplicas {
		if r.Image != svc.Image {
			return true
		}
		if !slicesEqual(r.Cmd, svc.Cmd) {
			return true
		}
	}
	return false
}

// slicesEqual compares two string slices for equality.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
