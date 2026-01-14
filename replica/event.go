package replica

type EventAction int

const (
	DeployEvent EventAction = iota
	StopEvent
)

type Event struct {
	Replica *Replica    // Full replica data
	Action  EventAction // DeployEvent or StopEvent
}

// Deprecated: Use Replica field instead
func (e *Event) GetReplica() *Replica {
	return e.Replica
}
