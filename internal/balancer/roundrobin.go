package balancer

import (
	"sync/atomic"

	"github.com/Nash0810/gobalance/internal/backend"
)

// RoundRobinStrategy distributes requests evenly across backends
type RoundRobinStrategy struct {
	counter uint64 // Atomic counter for round-robin
}

// NewRoundRobinStrategy creates a new round-robin strategy
func NewRoundRobinStrategy() *RoundRobinStrategy {
	return &RoundRobinStrategy{
		counter: 0,
	}
}

// SelectBackend picks the next backend in round-robin order
func (rr *RoundRobinStrategy) SelectBackend(pool *backend.Pool) *backend.Backend {
	// Get healthy backends
	backends := pool.GetHealthyBackends()

	if len(backends) == 0 {
		return nil // No healthy backends
	}

	// Atomically increment counter and get index
	count := atomic.AddUint64(&rr.counter, 1)
	index := int((count - 1) % uint64(len(backends)))

	return backends[index]
}

// Name returns the strategy name
func (rr *RoundRobinStrategy) Name() string {
	return "round-robin"
}
