package runtime

import "fmt"

type RuntimeType string

const (
	RuntimeDocker RuntimeType = "docker"
)

type Runtime interface {
	Start(image string, taskID string) (containerID string, err error)
	Stop(containerID string) error
	Status(containerID string) (string, error)
	Wait(containerID string) (exitCode int64, err error)
}

func NewRuntime(runtimeType RuntimeType) (Runtime, error) {
	switch runtimeType {
	case RuntimeDocker:
		return newDockerRuntime()
	default:
		panic("Unknown runtime type")
	}
}

func ParseRuntimeType(s string) (RuntimeType, error) {
	switch s {
	case "docker":
		return RuntimeDocker, nil
	default:
		return "", fmt.Errorf("unknown runtime type: %s", s)
	}
}
