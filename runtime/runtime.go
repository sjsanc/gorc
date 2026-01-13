package runtime

import "fmt"

type RuntimeType string

const (
	RuntimeDocker RuntimeType = "docker"
	RuntimePodman RuntimeType = "podman"
)

type Runtime interface {
	Start(image string, taskID string, args []string) (containerID string, err error)
	Stop(containerID string) error
	Status(containerID string) (string, error)
	Wait(containerID string) (exitCode int64, err error)
}

func NewRuntime(runtimeType RuntimeType) (Runtime, error) {
	switch runtimeType {
	case RuntimeDocker:
		return newDockerRuntime()
	case RuntimePodman:
		return newPodmanRuntime()
	default:
		return nil, fmt.Errorf("unknown runtime type: %s", runtimeType)
	}
}

func ParseRuntimeType(s string) (RuntimeType, error) {
	switch s {
	case "docker":
		return RuntimeDocker, nil
	case "podman":
		return RuntimePodman, nil
	default:
		return "", fmt.Errorf("unknown runtime type: %s", s)
	}
}
