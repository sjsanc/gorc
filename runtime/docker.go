package runtime

import (
	"context"

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

func (r *dockerRuntime) Start() error {
	return nil
}

func (r *dockerRuntime) Stop() error {
	return nil
}

func (r *dockerRuntime) Status() (string, error) {
	return "", nil
}
