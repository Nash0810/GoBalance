package balancer

import (
	"net/url"
	"sync"
	"testing"

	"github.com/Nash0810/gobalance/internal/backend"
)

// TestRoundRobin tests the round-robin strategy
func TestRoundRobin(t *testing.T) {
	pool := backend.NewPool()

	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")
	u3, _ := url.Parse("http://localhost:8083")

	b1 := backend.NewBackend(u1)
	b2 := backend.NewBackend(u2)
	b3 := backend.NewBackend(u3)

	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	strategy := NewRoundRobinStrategy()

	// Track selections
	selections := make(map[string]int)
	mu := sync.Mutex{}

	// Run 300 selections - expect roughly equal distribution (100 each)
	for i := 0; i < 300; i++ {
		backend := strategy.SelectBackend(pool)
		if backend == nil {
			t.Fatal("Strategy returned nil backend")
		}

		mu.Lock()
		selections[backend.URL.Host]++
		mu.Unlock()
	}

	// Verify roughly equal distribution
	for host, count := range selections {
		if count < 80 || count > 120 {
			t.Errorf("Uneven distribution for %s: got %d, expected ~100", host, count)
		}
	}
}

// TestRoundRobinWithUnhealthyBackend tests round-robin only selects healthy backends
func TestRoundRobinWithUnhealthyBackend(t *testing.T) {
	pool := backend.NewPool()

	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")

	b1 := backend.NewBackend(u1)
	b2 := backend.NewBackend(u2)

	pool.AddBackend(b1)
	pool.AddBackend(b2)

	// Mark b1 as unhealthy
	b1.SetAlive(false)

	strategy := NewRoundRobinStrategy()

	// All 50 selections should go to b2
	for i := 0; i < 50; i++ {
		selected := strategy.SelectBackend(pool)
		if selected == nil {
			t.Fatal("Strategy returned nil backend")
		}
		if selected.URL.Host != "localhost:8082" {
			t.Errorf("Selected unhealthy backend: %s", selected.URL.Host)
		}
	}
}

// TestRoundRobinNoHealthyBackends tests behavior when all backends are down
func TestRoundRobinNoHealthyBackends(t *testing.T) {
	pool := backend.NewPool()

	u1, _ := url.Parse("http://localhost:8081")
	b1 := backend.NewBackend(u1)
	b1.SetAlive(false)

	pool.AddBackend(b1)

	strategy := NewRoundRobinStrategy()
	selected := strategy.SelectBackend(pool)

	if selected != nil {
		t.Error("Strategy should return nil when no healthy backends")
	}
}

// TestRoundRobinConcurrency tests thread-safety
func TestRoundRobinConcurrency(t *testing.T) {
	pool := backend.NewPool()

	for i := 1; i <= 5; i++ {
		u, _ := url.Parse("http://localhost:" + string(rune(8080+i)))
		b := backend.NewBackend(u)
		pool.AddBackend(b)
	}

	strategy := NewRoundRobinStrategy()

	// Concurrent selections should not panic
	errors := make(chan error, 10)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				backend := strategy.SelectBackend(pool)
				if backend == nil {
					errors <- nil // No error, just no backends
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
}

// TestLeastConnections tests the least connections strategy
func TestLeastConnections(t *testing.T) {
	pool := backend.NewPool()

	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")
	u3, _ := url.Parse("http://localhost:8083")

	b1 := backend.NewBackend(u1)
	b2 := backend.NewBackend(u2)
	b3 := backend.NewBackend(u3)

	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	// Add active connections: b1=5, b2=3, b3=10
	for i := 0; i < 5; i++ {
		b1.IncrementActiveRequests()
	}
	for i := 0; i < 3; i++ {
		b2.IncrementActiveRequests()
	}
	for i := 0; i < 10; i++ {
		b3.IncrementActiveRequests()
	}

	strategy := NewLeastConnectionsStrategy()

	// Should select b2 (3 connections)
	selected := strategy.SelectBackend(pool)
	if selected == nil {
		t.Fatal("Strategy returned nil backend")
	}
	if selected.URL.Host != "localhost:8082" {
		t.Errorf("Expected b2 (3 connections), got %s", selected.URL.Host)
	}
}

// TestWeightedRoundRobin tests smooth weighted round-robin
func TestWeightedRoundRobin(t *testing.T) {
	pool := backend.NewPool()

	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")
	u3, _ := url.Parse("http://localhost:8083")

	b1 := backend.NewBackend(u1)
	b2 := backend.NewBackend(u2)
	b3 := backend.NewBackend(u3)

	// Set weights: b1=3 (30%), b2=2 (20%), b3=1 (10%) - but they're not normalized
	// Total weight = 6, so: b1=50%, b2=33%, b3=17%
	b1.SetWeight(3)
	b2.SetWeight(2)
	b3.SetWeight(1)

	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	strategy := NewWeightedRoundRobinStrategy()

	// Track selections over 600 requests
	selections := make(map[string]int)
	mu := sync.Mutex{}

	for i := 0; i < 600; i++ {
		backend := strategy.SelectBackend(pool)
		if backend == nil {
			t.Fatal("Strategy returned nil backend")
		}

		mu.Lock()
		selections[backend.URL.Host]++
		mu.Unlock()
	}

	// Expected ratios: b1:b2:b3 = 3:2:1 = 50%:33%:17%
	b1_count := selections["localhost:8081"]
	b2_count := selections["localhost:8082"]
	b3_count := selections["localhost:8083"]

	// Allow 10% variance
	if !(b1_count > 240 && b1_count < 360) { // ~50%
		t.Errorf("b1: expected ~300, got %d", b1_count)
	}
	if !(b2_count > 138 && b2_count < 228) { // ~33%
		t.Errorf("b2: expected ~200, got %d", b2_count)
	}
	if !(b3_count > 42 && b3_count < 132) { // ~17%
		t.Errorf("b3: expected ~100, got %d", b3_count)
	}
}
