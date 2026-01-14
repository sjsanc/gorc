package runtime

import (
	"context"
	"fmt"
	"os"

	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/specgen"
)

type podmanRuntime struct {
	ctx  context.Context
	conn context.Context
}

func newPodmanRuntime() (*podmanRuntime, error) {
	ctx := context.Background()

	// Determine socket path for Podman connection
	sockPath := getPodmanSocketPath()

	// Create connection - returns context with embedded connection
	conn, err := bindings.NewConnection(ctx, sockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to podman at %s: %v", sockPath, err)
	}

	return &podmanRuntime{
		ctx:  ctx,
		conn: conn,
	}, nil
}

// getPodmanSocketPath determines the Podman socket path.
// Checks PODMAN_SOCKET environment variable first, then probes rootful socket,
// then falls back to rootless socket.
func getPodmanSocketPath() string {
	// Check environment variable first
	if socket := os.Getenv("PODMAN_SOCKET"); socket != "" {
		return socket
	}

	// Try rootful socket
	rootfulSock := "unix:///run/podman/podman.sock"
	if _, err := os.Stat("/run/podman/podman.sock"); err == nil {
		return rootfulSock
	}

	// Fall back to rootless socket
	uid := os.Getuid()
	return fmt.Sprintf("unix:///run/user/%d/podman/podman.sock", uid)
}

func (r *podmanRuntime) Start(imageName string, replicaID string, args []string) (string, error) {
	// Pull the image
	_, err := images.Pull(r.conn, imageName, &images.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to pull image %s: %v", imageName, err)
	}

	// Create container spec
	s := specgen.NewSpecGenerator(imageName, false)
	s.Name = fmt.Sprintf("gorc-replica-%s", replicaID)
	s.Labels = map[string]string{
		"gorc.replica.id": replicaID,
	}

	// Set command args if provided
	if len(args) > 0 {
		s.Command = args
	}

	// Create container
	createResp, err := containers.CreateWithSpec(r.conn, s, &containers.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create container: %v", err)
	}

	// Start container
	err = containers.Start(r.conn, createResp.ID, &containers.StartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to start container: %v", err)
	}

	return createResp.ID, nil
}

func (r *podmanRuntime) Stop(containerID string) error {
	// Stop container with timeout
	timeout := uint(10)
	err := containers.Stop(r.conn, containerID, &containers.StopOptions{Timeout: &timeout})
	if err != nil {
		return fmt.Errorf("failed to stop container %s: %v", containerID, err)
	}

	// Remove container
	force := true
	_, err = containers.Remove(r.conn, containerID, &containers.RemoveOptions{Force: &force})
	if err != nil {
		return fmt.Errorf("failed to remove container %s: %v", containerID, err)
	}

	return nil
}

func (r *podmanRuntime) Status(containerID string) (string, error) {
	inspectData, err := containers.Inspect(r.conn, containerID, &containers.InspectOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to inspect container %s: %v", containerID, err)
	}

	return inspectData.State.Status, nil
}

func (r *podmanRuntime) Wait(containerID string) (int64, error) {
	// Wait returns int32, need to convert to int64
	exitCode, err := containers.Wait(r.conn, containerID, &containers.WaitOptions{})
	if err != nil {
		return -1, fmt.Errorf("failed to wait for container %s: %v", containerID, err)
	}

	return int64(exitCode), nil
}
