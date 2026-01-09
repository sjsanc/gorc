package task

import "github.com/google/uuid"

type EventAction int

const (
	DeployEvent EventAction = iota
	StopEvent
)

type Event struct {
	TaskID uuid.UUID
	Action EventAction
}
