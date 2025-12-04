package health

import (
	"log"

	"github.com/Nash0810/gobalance/internal/backend"
)

// PassiveTracker monitors real request failures
type PassiveTracker struct {
	failureThreshold int // Failures before marking unhealthy
}

// NewPassiveTracker creates a new passive health tracker
func NewPassiveTracker(threshold int) *PassiveTracker {
	return &PassiveTracker{
		failureThreshold: threshold,
	}
}

// RecordSuccess records a successful request
func (pt *PassiveTracker) RecordSuccess(b *backend.Backend) {
	// Reset failure counter on success
	metrics := b.GetHealthMetrics()
	if metrics.ConsecutiveFailures > 0 {
		b.RecordHealthCheckSuccess()
	}
}

// RecordFailure records a failed request
func (pt *PassiveTracker) RecordFailure(b *backend.Backend, err error) {
	b.RecordHealthCheckFailure()
	metrics := b.GetHealthMetrics()
	currentState := b.GetState()

	log.Printf("[PASSIVE] %s: Request failed: %v (failures: %d)",
		b.URL.Host, err, metrics.ConsecutiveFailures)

	// Mark unhealthy if threshold exceeded
	if currentState == backend.Healthy {
		if metrics.ConsecutiveFailures >= pt.failureThreshold {
			log.Printf("[PASSIVE] %s: Marking UNHEALTHY (after %d request failures)",
				b.URL.Host, metrics.ConsecutiveFailures)
			b.SetState(backend.Unhealthy)
		}
	}
}
