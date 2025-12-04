package backend

import (
	"time"
)

// HealthState represents the health status of a backend
type HealthState int

const (
	// Healthy means backend is operational
	Healthy HealthState = iota

	// Unhealthy means backend has failed health checks
	Unhealthy

	// Draining means backend is being removed (finishing existing requests)
	Draining

	// Down means backend is completely offline
	Down
)

// String returns human-readable state name
func (hs HealthState) String() string {
	switch hs {
	case Healthy:
		return "HEALTHY"
	case Unhealthy:
		return "UNHEALTHY"
	case Draining:
		return "DRAINING"
	case Down:
		return "DOWN"
	default:
		return "UNKNOWN"
	}
}

// HealthMetrics tracks health check statistics
type HealthMetrics struct {
	ConsecutiveSuccesses int       // Consecutive successful checks
	ConsecutiveFailures  int       // Consecutive failed checks
	LastCheck            time.Time // Time of last health check
	LastSuccess          time.Time // Time of last successful check
	LastFailure          time.Time // Time of last failed check
}
