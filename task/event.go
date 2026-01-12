package task

type EventAction int

const (
	DeployEvent EventAction = iota
	StopEvent
)

type Event struct {
	Task   *Task       // Full task data
	Action EventAction // DeployEvent or StopEvent
}
