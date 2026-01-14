package runtime

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type dockerRuntime struct {
	ctx          context.Context
	dockerClient *client.Client
}

func newDockerRuntime() (*dockerRuntime, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &dockerRuntime{
		ctx:          ctx,
		dockerClient: cli,
	}, nil
}

func (r *dockerRuntime) Start(imageName string, replicaID string, args []string) (string, error) {
	// Pull the image
	reader, err := r.dockerClient.ImagePull(r.ctx, imageName, image.PullOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to pull image %s: %v", imageName, err)
	}
	defer reader.Close()
	// Read the pull output to ensure it completes
	io.Copy(io.Discard, reader)

	// Create container with replica ID as name
	containerConfig := &container.Config{
		Image: imageName,
		Labels: map[string]string{
			"gorc.replica.id": replicaID,
		},
	}

	// Set Cmd only if args provided (otherwise use image default)
	if len(args) > 0 {
		containerConfig.Cmd = args
	}

	resp, err := r.dockerClient.ContainerCreate(r.ctx, containerConfig, nil, nil, nil, fmt.Sprintf("gorc-replica-%s", replicaID))
	if err != nil {
		return "", fmt.Errorf("failed to create container: %v", err)
	}

	// Start the container
	err = r.dockerClient.ContainerStart(r.ctx, resp.ID, container.StartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to start container: %v", err)
	}

	return resp.ID, nil
}

func (r *dockerRuntime) Stop(containerID string) error {
	timeout := 10 // seconds
	err := r.dockerClient.ContainerStop(r.ctx, containerID, container.StopOptions{Timeout: &timeout})
	if err != nil {
		return fmt.Errorf("failed to stop container %s: %v", containerID, err)
	}

	// Remove the container after stopping
	err = r.dockerClient.ContainerRemove(r.ctx, containerID, container.RemoveOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove container %s: %v", containerID, err)
	}

	return nil
}

func (r *dockerRuntime) Status(containerID string) (string, error) {
	inspect, err := r.dockerClient.ContainerInspect(r.ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container %s: %v", containerID, err)
	}

	return inspect.State.Status, nil
}

func (r *dockerRuntime) Wait(containerID string) (int64, error) {
	statusCh, errCh := r.dockerClient.ContainerWait(r.ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return -1, fmt.Errorf("failed to wait for container %s: %v", containerID, err)
		}
	case status := <-statusCh:
		return status.StatusCode, nil
	}
	return 0, nil
}
