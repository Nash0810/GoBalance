package balancer

import (
	"net/url"
	"testing"
	"time"

	"github.com/Nash0810/gobalance/internal/backend"
)

// TestRoundRobinStrategy tests the round robin load balancing strategy
func TestRoundRobinStrategy(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	// Create test backends
	backends := make([]*backend.Backend, 3)
	for i := 0; i < 3; i++ {
		u, _ := url.Parse("http://localhost:800" + string(rune(i+1)))
		backends[i] = backend.NewBackend(u)
	}

	// Create pool with backends
	pool := backend.NewPool()
	for _, b := range backends {
		pool.AddBackend(b)
	}

	// Test round robin distribution
	selected := make([]string, 0)
	for i := 0; i < 9; i++ {
		backend := strategy.SelectBackend(pool)
		if backend == nil {
			t.Errorf("SelectBackend returned nil at iteration %d", i)
			continue
		}
		selected = append(selected, backend.URL.Host)
	}

	// Verify even distribution (should cycle through all backends)
	expected := []string{
		"localhost:8001", "localhost:8002", "localhost:8003",
		"localhost:8001", "localhost:8002", "localhost:8003",
		"localhost:8001", "localhost:8002", "localhost:8003",
	}

	for i, exp := range expected {
		if i < len(selected) && selected[i] != exp {
			t.Errorf("Round %d: expected %s, got %s", i, exp, selected[i])
		}
	}
}

// TestWeightedRoundRobinStrategy tests weighted distribution
func TestWeightedRoundRobinStrategy(t *testing.T) {
	strategy := NewWeightedRoundRobinStrategy()

	// Create backends with weights: 3, 2, 1 (6 total)
	u1, _ := url.Parse("http://localhost:8001")
	u2, _ := url.Parse("http://localhost:8002")
	u3, _ := url.Parse("http://localhost:8003")

	b1 := backend.NewBackend(u1)
	b1.SetWeight(3)
	b2 := backend.NewBackend(u2)
	b2.SetWeight(2)
	b3 := backend.NewBackend(u3)
	b3.SetWeight(1)

	pool := backend.NewPool()
	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	// Request 60 backends - should get approximately 30, 20, 10 distribution
	counts := make(map[string]int)
	for i := 0; i < 60; i++ {
		selected := strategy.SelectBackend(pool)
		if selected != nil {
			counts[selected.URL.Host]++
		}
	}

	// Verify distribution (allow 20% variance)
	expectedB1 := 30 // 3/6 of 60
	expectedB2 := 20 // 2/6 of 60
	expectedB3 := 10 // 1/6 of 60

	tolerance := 5

	if count := counts["localhost:8001"]; count < expectedB1-tolerance || count > expectedB1+tolerance {
		t.Errorf("Backend 1: expected ~%d, got %d", expectedB1, count)
	}
	if count := counts["localhost:8002"]; count < expectedB2-tolerance || count > expectedB2+tolerance {
		t.Errorf("Backend 2: expected ~%d, got %d", expectedB2, count)
	}
	if count := counts["localhost:8003"]; count < expectedB3-tolerance || count > expectedB3+tolerance {
		t.Errorf("Backend 3: expected ~%d, got %d", expectedB3, count)
	}
}

// TestLeastConnectionsStrategy tests least connections strategy
func TestLeastConnectionsStrategy(t *testing.T) {
	strategy := NewLeastConnectionsStrategy()

	// Create backends
	u1, _ := url.Parse("http://localhost:8001")
	u2, _ := url.Parse("http://localhost:8002")

	b1 := backend.NewBackend(u1)
	b2 := backend.NewBackend(u2)

	pool := backend.NewPool()
	pool.AddBackend(b1)
	pool.AddBackend(b2)

	// First request should go to either (both have 0)
	backend1 := strategy.SelectBackend(pool)
	if backend1 == nil {
		t.Fatal("SelectBackend returned nil")
	}

	// Simulate active requests on b1
	b1.IncrementActiveRequests()
	b1.IncrementActiveRequests()
	// b2 has 0, b1 has 2

	// Next request should go to b2 (fewer connections)
	backend2 := strategy.SelectBackend(pool)
	if backend2 == nil {
		t.Fatal("SelectBackend returned nil")
	}
	if backend2.URL.Host != "localhost:8002" {
		t.Errorf("Expected localhost:8002 (fewer connections), got %s", backend2.URL.Host)
	}

	// Add more connections to b2
	b2.IncrementActiveRequests()
	b2.IncrementActiveRequests()
	b2.IncrementActiveRequests()
	// Now b1 has 2, b2 has 3

	// Next request should go to b1
	backend3 := strategy.SelectBackend(pool)
	if backend3 == nil {
		t.Fatal("SelectBackend returned nil")
	}
	if backend3.URL.Host != "localhost:8001" {
		t.Errorf("Expected localhost:8001 (fewer connections), got %s", backend3.URL.Host)
	}
}

// TestStrategyWithUnhealthyBackends tests that strategies skip unhealthy backends
func TestStrategyWithUnhealthyBackends(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	// Create backends
	u1, _ := url.Parse("http://localhost:8001")
	u2, _ := url.Parse("http://localhost:8002")

	b1 := backend.NewBackend(u1)
	b2 := backend.NewBackend(u2)

	pool := backend.NewPool()
	pool.AddBackend(b1)
	pool.AddBackend(b2)

	// Mark b1 as unhealthy
	b1.SetState(backend.Unhealthy)
	b1.SetAlive(false)

	// Strategy should only select b2
	for i := 0; i < 5; i++ {
		selected := strategy.SelectBackend(pool)
		if selected == nil {
			t.Fatal("SelectBackend returned nil")
		}
		if selected.URL.Host != "localhost:8002" {
			t.Errorf("Expected only localhost:8002, got %s", selected.URL.Host)
		}
	}
}

// TestStrategyWithNoHealthyBackends tests behavior when all backends are down
func TestStrategyWithNoHealthyBackends(t *testing.T) {
	strategy := NewRoundRobinStrategy()

	u1, _ := url.Parse("http://localhost:8001")
	b1 := backend.NewBackend(u1)
	b1.SetAlive(false)

	pool := backend.NewPool()
	pool.AddBackend(b1)

	// Should return nil when no healthy backends
	selected := strategy.SelectBackend(pool)
	if selected != nil {
		t.Errorf("Expected nil for no healthy backends, got %v", selected.URL.Host)
	}
}

// BenchmarkRoundRobin benchmarks round robin selection performance
func BenchmarkRoundRobin(b *testing.B) {
	strategy := NewRoundRobinStrategy()

	// Create many backends
	pool := backend.NewPool()
	for i := 0; i < 100; i++ {
		u, _ := url.Parse("http://localhost:8000")
		backend := backend.NewBackend(u)
		pool.AddBackend(backend)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.SelectBackend(pool)
	}
}

// BenchmarkWeightedRoundRobin benchmarks weighted round robin performance
func BenchmarkWeightedRoundRobin(b *testing.B) {
	strategy := NewWeightedRoundRobinStrategy()

	// Create backends with weights
	pool := backend.NewPool()
	for i := 0; i < 10; i++ {
		u, _ := url.Parse("http://localhost:8000")
		backend := backend.NewBackend(u)
		backend.SetWeight((i % 10) + 1) // Weights 1-10
		pool.AddBackend(backend)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.SelectBackend(pool)
	}
}

// BenchmarkLeastConnections benchmarks least connections performance
func BenchmarkLeastConnections(b *testing.B) {
	strategy := NewLeastConnectionsStrategy()

	// Create backends with varying connection counts
	pool := backend.NewPool()
	backends := make([]*backend.Backend, 50)
	for i := 0; i < 50; i++ {
		u, _ := url.Parse("http://localhost:8000")
		backend := backend.NewBackend(u)
		// Simulate various connection counts
		for j := 0; j < (i % 20); j++ {
			backend.IncrementActiveRequests()
		}
		backends[i] = backend
		pool.AddBackend(backend)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.SelectBackend(pool)
	}
}
