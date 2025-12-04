package balancer

import (
	"github.com/Nash0810/gobalance/internal/backend"
)

// Strategy defines the interface for load balancing algorithms
type Strategy interface {
	// SelectBackend chooses a backend from the given pool
	// Returns nil if no healthy backends available
	SelectBackend(pool *backend.Pool) *backend.Backend

	// Name returns the strategy name
	Name() string
}
