package worker

import (
	"fmt"
	"net"
)

// Port range for automatic selection
const (
	PortRangeStart = 6000
	PortRangeEnd   = 7000
)

// FindAvailablePort finds an available port in the configured range.
// If no port is available in the range, an error is returned.
func FindAvailablePort() (int, error) {
	for port := PortRangeStart; port <= PortRangeEnd; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", PortRangeStart, PortRangeEnd)
}
