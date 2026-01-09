package runtime

import "fmt"

type RuntimeType string

const (
	RuntimeDocker RuntimeType = "docker"
)

type Runtime interface {
	Start() error
	Stop() error
	Status() (string, error)
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
