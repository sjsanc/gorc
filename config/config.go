package config

import "time"

// Default port numbers
const (
	DefaultManagerPort = 5555
	DefaultWorkerPort  = 5556
)

// Default network configuration
const (
	DefaultListenAddress = "0.0.0.0"
)

// Default timeouts
const (
	DefaultServerShutdownTimeout = 5 * time.Second
	DefaultHTTPClientTimeout     = 10 * time.Second
	DefaultHeartbeatInterval     = 5 * time.Second
	DefaultEventProcessingDelay  = 100 * time.Millisecond
)

// Worker heartbeat detection (used by Manager)
const (
	DefaultHeartbeatTimeout = 30 * time.Second
	DefaultHeartbeatCheck   = 10 * time.Second
)

// Scheduler configuration
const (
	DefaultSchedulerType = "roundrobin"
)
