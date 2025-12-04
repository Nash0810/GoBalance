package balancer

import (
	"math"

	"github.com/Nash0810/gobalance/internal/backend"
)

// LeastConnectionsStrategy selects backend with fewest active connections
type LeastConnectionsStrategy struct{}

// NewLeastConnectionsStrategy creates a new least-connections strategy
func NewLeastConnectionsStrategy() *LeastConnectionsStrategy {
	return &LeastConnectionsStrategy{}
}

// SelectBackend picks the backend with minimum active requests
func (lc *LeastConnectionsStrategy) SelectBackend(pool *backend.Pool) *backend.Backend {
	backends := pool.GetHealthyBackends()

	if len(backends) == 0 {
		return nil
	}

	// Find backend with minimum connections
	var selected *backend.Backend
	minConnections := int64(math.MaxInt64)

	for _, b := range backends {
		connections := b.GetActiveRequests()
		if connections < minConnections {
			minConnections = connections
			selected = b
		}
	}

	return selected
}

// Name returns the strategy name
func (lc *LeastConnectionsStrategy) Name() string {
	return "least-connections"
}
