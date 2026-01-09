package node

type Node struct {
	// Hostname of the machine.
	Hostname string
	// IPv4 address of the machine.
	Address string
	// Port that the worker is listening on.
	WorkerPort int
	// Port that the manager is listening on.
	ManagerPort int
}

func NewNode(hostname string, address string, workerPort int, managerPort int) *Node {
	return &Node{
		Hostname:    hostname,
		Address:     address,
		WorkerPort:  workerPort,
		ManagerPort: managerPort,
	}
}
