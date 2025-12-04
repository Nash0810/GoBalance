package backend

import (
	"sync"
)

// Pool manages a collection of backends
type Pool struct {
	backends []*Backend
	mux      sync.RWMutex
}

// NewPool creates a new backend pool
func NewPool() *Pool {
	return &Pool{
		backends: make([]*Backend, 0),
	}
}

// AddBackend adds a backend to the pool
func (p *Pool) AddBackend(b *Backend) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.backends = append(p.backends, b)
}

// GetBackends returns all backends (copy of slice)
func (p *Pool) GetBackends() []*Backend {
	p.mux.RLock()
	defer p.mux.RUnlock()

	// Return a copy to avoid race conditions
	backends := make([]*Backend, len(p.backends))
	copy(backends, p.backends)
	return backends
}

// GetHealthyBackends returns only healthy backends
func (p *Pool) GetHealthyBackends() []*Backend {
	p.mux.RLock()
	defer p.mux.RUnlock()

	var healthy []*Backend
	for _, b := range p.backends {
		if b.IsAlive() {
			healthy = append(healthy, b)
		}
	}
	return healthy
}

// Size returns the total number of backends
func (p *Pool) Size() int {
	p.mux.RLock()
	defer p.mux.RUnlock()
	return len(p.backends)
}
