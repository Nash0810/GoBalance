package balancer

import (
	"math"
	"sync"

	"github.com/Nash0810/gobalance/internal/backend"
)

// WeightedBackend tracks current weight for smooth weighted round robin
type WeightedBackend struct {
	backend       *backend.Backend
	weight        int
	currentWeight int
}

// WeightedRoundRobinStrategy distributes requests using smooth weighted round robin (Nginx algorithm)
// FIX #7: Implemented smooth weighted round robin for better distribution
type WeightedRoundRobinStrategy struct {
	weightedBackends map[string]*WeightedBackend
	mux              sync.RWMutex
}

// NewWeightedRoundRobinStrategy creates a new weighted round-robin strategy
func NewWeightedRoundRobinStrategy() *WeightedRoundRobinStrategy {
	return &WeightedRoundRobinStrategy{
		weightedBackends: make(map[string]*WeightedBackend),
	}
}

// SelectBackend picks backend using smooth weighted round-robin (Nginx algorithm)
func (wrr *WeightedRoundRobinStrategy) SelectBackend(pool *backend.Pool) *backend.Backend {
	backends := pool.GetHealthyBackends()

	if len(backends) == 0 {
		return nil
	}

	wrr.mux.Lock()
	defer wrr.mux.Unlock()

	// Initialize or update weighted backends
	for _, b := range backends {
		key := b.URL.String()
		if _, exists := wrr.weightedBackends[key]; !exists {
			wrr.weightedBackends[key] = &WeightedBackend{
				backend:       b,
				weight:        b.Weight,
				currentWeight: 0,
			}
		} else {
			// Update weight in case it changed
			wrr.weightedBackends[key].weight = b.Weight
		}
	}

	// Remove weighted backends that are no longer in the pool
	for key := range wrr.weightedBackends {
		found := false
		for _, b := range backends {
			if b.URL.String() == key {
				found = true
				break
			}
		}
		if !found {
			delete(wrr.weightedBackends, key)
		}
	}

	// Smooth weighted round robin algorithm
	totalWeight := 0
	var selected *WeightedBackend
	maxCurrentWeight := math.MinInt

	for _, wb := range wrr.weightedBackends {
		// Increase current weight by configured weight
		wb.currentWeight += wb.weight
		totalWeight += wb.weight

		// Select backend with highest current weight
		if wb.currentWeight > maxCurrentWeight {
			maxCurrentWeight = wb.currentWeight
			selected = wb
		}
	}

	if selected != nil {
		// Decrease selected backend's current weight by total weight
		selected.currentWeight -= totalWeight
		return selected.backend
	}

	return nil
}

// Name returns the strategy name
func (wrr *WeightedRoundRobinStrategy) Name() string {
	return "weighted-round-robin"
}
