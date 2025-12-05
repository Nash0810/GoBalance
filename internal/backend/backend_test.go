package backend

import (
	"net/url"
	"sync"
	"testing"
)

// TestBackendHealthState tests the health state transitions
func TestBackendHealthState(t *testing.T) {
	u, _ := url.Parse("http://localhost:8081")
	b := NewBackend(u)

	// Initial state should be Healthy
	if b.GetState() != Healthy {
		t.Errorf("Initial state should be Healthy, got %v", b.GetState())
	}

	// Test state transitions
	states := []HealthState{Unhealthy, Healthy, Draining, Down}
	for _, state := range states {
		b.SetState(state)
		if b.GetState() != state {
			t.Errorf("Failed to set state to %v", state)
		}
	}
}

// TestBackendAliveStatus tests IsAlive/SetAlive
func TestBackendAliveStatus(t *testing.T) {
	u, _ := url.Parse("http://localhost:8081")
	b := NewBackend(u)

	if !b.IsAlive() {
		t.Error("Backend should be alive by default")
	}

	b.SetAlive(false)
	if b.IsAlive() {
		t.Error("Backend should be dead after SetAlive(false)")
	}

	b.SetAlive(true)
	if !b.IsAlive() {
		t.Error("Backend should be alive after SetAlive(true)")
	}
}

// TestBackendActiveRequests tests atomic request counting
func TestBackendActiveRequests(t *testing.T) {
	u, _ := url.Parse("http://localhost:8081")
	b := NewBackend(u)

	if b.GetActiveRequests() != 0 {
		t.Error("Initial active requests should be 0")
	}

	// Increment and verify
	for i := 1; i <= 10; i++ {
		b.IncrementActiveRequests()
		if b.GetActiveRequests() != int64(i) {
			t.Errorf("Expected %d active requests, got %d", i, b.GetActiveRequests())
		}
	}

	// Decrement and verify
	for i := 9; i >= 0; i-- {
		b.DecrementActiveRequests()
		if b.GetActiveRequests() != int64(i) {
			t.Errorf("Expected %d active requests, got %d", i, b.GetActiveRequests())
		}
	}
}

// TestBackendActiveRequestsConcurrency tests thread-safe request counting
func TestBackendActiveRequestsConcurrency(t *testing.T) {
	u, _ := url.Parse("http://localhost:8081")
	b := NewBackend(u)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.IncrementActiveRequests()
			}
		}()
	}

	wg.Wait()

	if b.GetActiveRequests() != 10000 {
		t.Errorf("Expected 10000 active requests, got %d", b.GetActiveRequests())
	}
}

// TestBackendHealthMetrics tests health check metrics
func TestBackendHealthMetrics(t *testing.T) {
	u, _ := url.Parse("http://localhost:8081")
	b := NewBackend(u)

	m := b.GetHealthMetrics()
	if m.ConsecutiveSuccesses != 0 || m.ConsecutiveFailures != 0 {
		t.Error("Initial metrics should be zero")
	}

	// Record successes
	for i := 1; i <= 3; i++ {
		b.RecordHealthCheckSuccess()
		m := b.GetHealthMetrics()
		if m.ConsecutiveSuccesses != int(i) {
			t.Errorf("Expected %d successes, got %d", i, m.ConsecutiveSuccesses)
		}
		if m.ConsecutiveFailures != 0 {
			t.Error("Failures should reset to 0 on success")
		}
	}

	// Record failures
	for i := 1; i <= 2; i++ {
		b.RecordHealthCheckFailure()
		m := b.GetHealthMetrics()
		if m.ConsecutiveFailures != int(i) {
			t.Errorf("Expected %d failures, got %d", i, m.ConsecutiveFailures)
		}
		if m.ConsecutiveSuccesses != 0 {
			t.Error("Successes should reset to 0 on failure")
		}
	}
}

// TestBackendWeight tests weight configuration
func TestBackendWeight(t *testing.T) {
	u, _ := url.Parse("http://localhost:8081")
	b := NewBackend(u)

	// Test default weight
	if b.Weight != 1 {
		t.Errorf("Default weight should be 1, got %d", b.Weight)
	}

	// Test valid weight
	b.SetWeight(50)
	if b.Weight != 50 {
		t.Errorf("Weight should be 50, got %d", b.Weight)
	}

	// Test min weight
	b.SetWeight(0)
	if b.Weight != 1 {
		t.Errorf("Weight should be clamped to 1, got %d", b.Weight)
	}

	// Test max weight
	b.SetWeight(200)
	if b.Weight != 100 {
		t.Errorf("Weight should be clamped to 100, got %d", b.Weight)
	}
}

// TestPoolAddBackend tests adding backends to pool
func TestPoolAddBackend(t *testing.T) {
	pool := NewPool()

	if pool.Size() != 0 {
		t.Error("Initial pool size should be 0")
	}

	u1, _ := url.Parse("http://localhost:8081")
	b1 := NewBackend(u1)
	pool.AddBackend(b1)

	if pool.Size() != 1 {
		t.Error("Pool size should be 1 after adding backend")
	}

	u2, _ := url.Parse("http://localhost:8082")
	b2 := NewBackend(u2)
	pool.AddBackend(b2)

	if pool.Size() != 2 {
		t.Error("Pool size should be 2 after adding second backend")
	}
}

// TestPoolGetBackends tests retrieving backends
func TestPoolGetBackends(t *testing.T) {
	pool := NewPool()

	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")

	b1 := NewBackend(u1)
	b2 := NewBackend(u2)

	pool.AddBackend(b1)
	pool.AddBackend(b2)

	backends := pool.GetBackends()
	if len(backends) != 2 {
		t.Errorf("Expected 2 backends, got %d", len(backends))
	}

	// Verify it's a copy (modifying shouldn't affect pool)
	backends = append(backends, NewBackend(u1))
	if pool.Size() != 2 {
		t.Error("Pool size should remain 2 (returned slice is a copy)")
	}
}

// TestPoolGetHealthyBackends tests filtering healthy backends
func TestPoolGetHealthyBackends(t *testing.T) {
	pool := NewPool()

	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")
	u3, _ := url.Parse("http://localhost:8083")

	b1 := NewBackend(u1)
	b2 := NewBackend(u2)
	b3 := NewBackend(u3)

	pool.AddBackend(b1)
	pool.AddBackend(b2)
	pool.AddBackend(b3)

	healthy := pool.GetHealthyBackends()
	if len(healthy) != 3 {
		t.Error("All backends should be healthy initially")
	}

	// Mark b2 as dead
	b2.SetAlive(false)

	healthy = pool.GetHealthyBackends()
	if len(healthy) != 2 {
		t.Error("Should have 2 healthy backends after marking one dead")
	}

	for _, b := range healthy {
		if b.URL.Host == "localhost:8082" {
			t.Error("Dead backend should not be in healthy list")
		}
	}
}

// TestPoolReplaceBackends tests hot reload with state preservation
func TestPoolReplaceBackends(t *testing.T) {
	pool := NewPool()

	u1, _ := url.Parse("http://localhost:8081")
	u2, _ := url.Parse("http://localhost:8082")

	b1 := NewBackend(u1)
	b2 := NewBackend(u2)

	pool.AddBackend(b1)
	pool.AddBackend(b2)

	// Mark b1 as unhealthy with metrics
	b1.SetAlive(false)
	b1.SetState(Unhealthy)
	b1.RecordHealthCheckFailure()
	b1.RecordHealthCheckFailure()
	b1.RecordHealthCheckFailure()

	metrics := b1.GetHealthMetrics()
	if metrics.ConsecutiveFailures != 3 {
		t.Error("Should have 3 consecutive failures")
	}

	// Create new backends and replace pool
	newB1 := NewBackend(u1)
	newB2 := NewBackend(u2)
	u3, _ := url.Parse("http://localhost:8083")
	newB3 := NewBackend(u3)

	pool.ReplaceBackends([]*Backend{newB1, newB2, newB3})

	// Verify new backends are in pool
	if pool.Size() != 3 {
		t.Errorf("Pool size should be 3 after replacement, got %d", pool.Size())
	}

	// Verify old b1's state was preserved in newB1
	if newB1.IsAlive() {
		t.Error("New b1 should inherit dead state from old b1")
	}
	if newB1.GetState() != Unhealthy {
		t.Errorf("New b1 should inherit Unhealthy state, got %v", newB1.GetState())
	}

	newMetrics := newB1.GetHealthMetrics()
	if newMetrics.ConsecutiveFailures != 3 {
		t.Error("New b1 should inherit consecutive failures metric")
	}

	// Verify new b3 (completely new) starts healthy
	if !newB3.IsAlive() {
		t.Error("New backend should start as alive")
	}
	if newB3.GetState() != Healthy {
		t.Error("New backend should start in Healthy state")
	}
}

// TestPoolConcurrency tests thread-safe pool operations
func TestPoolConcurrency(t *testing.T) {
	pool := NewPool()

	for i := 1; i <= 5; i++ {
		u, _ := url.Parse("http://localhost:" + string(rune(8080+i)))
		pool.AddBackend(NewBackend(u))
	}

	errors := make(chan error, 0)
	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backends := pool.GetBackends()
			if len(backends) != 5 {
				errors <- nil // Just track concurrent access
			}
		}()
	}

	// Concurrent increments on first backend
	firstBackend := pool.GetBackends()[0]
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				firstBackend.IncrementActiveRequests()
			}
		}()
	}

	wg.Wait()

	if firstBackend.GetActiveRequests() != 1000 {
		t.Errorf("Expected 1000 active requests, got %d", firstBackend.GetActiveRequests())
	}
}
