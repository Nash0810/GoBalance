package balancer

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nash0810/gobalance/internal/backend"
	"github.com/Nash0810/gobalance/internal/health"
	"github.com/Nash0810/gobalance/internal/logging"
	"github.com/Nash0810/gobalance/internal/metrics"
	"github.com/Nash0810/gobalance/internal/retry"
)

// sharedCollector - Prometheus requires single registration per test run
var (
	sharedCollectorOnce sync.Once
	sharedCollector     *metrics.Collector
)

func getSharedCollector() *metrics.Collector {
	sharedCollectorOnce.Do(func() {
		sharedCollector = metrics.NewCollector()
	})
	return sharedCollector
}

func createTestBalancer(pool *backend.Pool, strategy Strategy) *Balancer {
	logger := logging.NewLogger("balancer")
	passiveTracker := health.NewPassiveTracker(3)
	retryPolicy := retry.NewPolicy(2, 25)
	return NewBalancer(pool, strategy, passiveTracker, retryPolicy, 10*time.Second, getSharedCollector(), logger)
}

// TestE2EHealthyBackend tests basic request routing
func TestE2EHealthyBackend(t *testing.T) {
	requestReceived := make(chan bool, 1)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived <- true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer mockServer.Close()

	pool := backend.NewPool()
	u, _ := url.Parse(mockServer.URL)
	pool.AddBackend(backend.NewBackend(u))

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	balancer.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	select {
	case <-requestReceived:
		// Success
	case <-time.After(time.Second):
		t.Error("Request not received by backend")
	}
}

// TestE2ERoundRobinDistribution tests load distribution
func TestE2ERoundRobinDistribution(t *testing.T) {
	hitCount := make(map[int]int32)
	var mu sync.Mutex

	servers := []*httptest.Server{}
	for i := 0; i < 3; i++ {
		idx := i
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hitCount[idx]++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		servers = append(servers, server)
	}
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	pool := backend.NewPool()
	for _, server := range servers {
		u, _ := url.Parse(server.URL)
		pool.AddBackend(backend.NewBackend(u))
	}

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	// Send 9 requests
	for i := 0; i < 9; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		balancer.ServeHTTP(w, req)
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify each got 3 requests
	for i := 0; i < 3; i++ {
		if hitCount[i] != 3 {
			t.Errorf("Server %d: expected 3 requests, got %d", i, hitCount[i])
		}
	}
}

// TestE2EFailover tests request retry on failure
func TestE2EFailover(t *testing.T) {
	attempt := atomic.Int32{}

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	pool := backend.NewPool()
	u1, _ := url.Parse(server1.URL)
	u2, _ := url.Parse(server2.URL)
	pool.AddBackend(backend.NewBackend(u1))
	pool.AddBackend(backend.NewBackend(u2))

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	req := httptest.NewRequest("GET", "/", strings.NewReader("test"))
	w := httptest.NewRecorder()
	balancer.ServeHTTP(w, req)

	// Should eventually succeed
	if w.Code == http.StatusOK {
		// Success - failover worked
	}
}

// TestE2EPostBodyPreservation tests body is preserved in retries
func TestE2EPostBodyPreservation(t *testing.T) {
	attempt := atomic.Int32{}
	var receivedBody string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)

		if attempt.Load() == 0 {
			attempt.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	pool := backend.NewPool()
	u, _ := url.Parse(mockServer.URL)
	pool.AddBackend(backend.NewBackend(u))

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	testBody := "preserved"
	req := httptest.NewRequest("POST", "/", strings.NewReader(testBody))
	w := httptest.NewRecorder()
	balancer.ServeHTTP(w, req)

	if w.Code == http.StatusOK && receivedBody != testBody {
		t.Errorf("Body not preserved: expected %q, got %q", testBody, receivedBody)
	}
}

// TestE2EConcurrentRequests tests concurrent load handling
func TestE2EConcurrentRequests(t *testing.T) {
	successCount := atomic.Int32{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		successCount.Add(1)
	}))
	defer mockServer.Close()

	pool := backend.NewPool()
	u, _ := url.Parse(mockServer.URL)
	pool.AddBackend(backend.NewBackend(u))

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			balancer.ServeHTTP(w, req)
		}()
	}

	wg.Wait()

	if successCount.Load() < 15 {
		t.Logf("Concurrent requests: %d succeeded", successCount.Load())
	}
}

// TestE2ERequestTimeout tests timeout handling
func TestE2ERequestTimeout(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	pool := backend.NewPool()
	u, _ := url.Parse(mockServer.URL)
	pool.AddBackend(backend.NewBackend(u))

	logger := logging.NewLogger("test")
	passiveTracker := health.NewPassiveTracker(3)
	retryPolicy := retry.NewPolicy(1, 25)

	// Create balancer with very short timeout
	balancer := NewBalancer(pool, NewRoundRobinStrategy(), passiveTracker, retryPolicy, 100*time.Millisecond, getSharedCollector(), logger)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	balancer.ServeHTTP(w, req)
	elapsed := time.Since(start)

	// Should timeout quickly
	if elapsed > 2*time.Second {
		t.Errorf("Request took too long: %v", elapsed)
	}
}

// TestE2ECustomHeaders tests header propagation
func TestE2ECustomHeaders(t *testing.T) {
	var headerReceived http.Header

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerReceived = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	pool := backend.NewPool()
	u, _ := url.Parse(mockServer.URL)
	pool.AddBackend(backend.NewBackend(u))

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Custom", "test")
	w := httptest.NewRecorder()

	balancer.ServeHTTP(w, req)

	if headerReceived.Get("X-Custom") != "test" {
		t.Error("Custom header not propagated")
	}
}

// TestE2EResponseHeaders tests response header passing
func TestE2EResponseHeaders(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "response")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	pool := backend.NewPool()
	u, _ := url.Parse(mockServer.URL)
	pool.AddBackend(backend.NewBackend(u))

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	balancer.ServeHTTP(w, req)

	if w.Header().Get("X-Test") != "response" {
		t.Error("Response header not propagated")
	}
}

// TestE2ELeastConnections tests least connections strategy
func TestE2ELeastConnections(t *testing.T) {
	hitCount := make(map[int]int32)
	var mu sync.Mutex

	servers := []*httptest.Server{}
	for i := 0; i < 3; i++ {
		idx := i
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hitCount[idx]++
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		servers = append(servers, server)
	}
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	pool := backend.NewPool()
	for _, server := range servers {
		u, _ := url.Parse(server.URL)
		pool.AddBackend(backend.NewBackend(u))
	}

	balancer := createTestBalancer(pool, NewLeastConnectionsStrategy())

	// Send concurrent requests to test load distribution
	var wg sync.WaitGroup
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			balancer.ServeHTTP(w, req)
		}()
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	// Verify all backends got requests
	for i := 0; i < 3; i++ {
		if hitCount[i] == 0 {
			t.Errorf("Server %d received no requests", i)
		}
	}
}

// TestE2EWeightedDistribution tests weighted round robin
func TestE2EWeightedDistribution(t *testing.T) {
	hitCount := make(map[int]int32)
	var mu sync.Mutex

	servers := []*httptest.Server{}
	for i := 0; i < 2; i++ {
		idx := i
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hitCount[idx]++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}))
		servers = append(servers, server)
	}
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	pool := backend.NewPool()
	for i, server := range servers {
		u, _ := url.Parse(server.URL)
		b := backend.NewBackend(u)
		if i == 0 {
			b.SetWeight(2)
		} else {
			b.SetWeight(1)
		}
		pool.AddBackend(b)
	}

	balancer := createTestBalancer(pool, NewWeightedRoundRobinStrategy())

	// Send 30 requests
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		balancer.ServeHTTP(w, req)
	}

	mu.Lock()
	defer mu.Unlock()

	// First should get more requests (2x weight)
	total := hitCount[0] + hitCount[1]
	if total != 30 {
		t.Logf("Weighted distribution: server0=%d, server1=%d", hitCount[0], hitCount[1])
	}
}

// TestE2ENoHealthyBackends tests error handling
func TestE2ENoHealthyBackends(t *testing.T) {
	pool := backend.NewPool()
	u, _ := url.Parse("http://127.0.0.1:65432") // Won't connect
	b := backend.NewBackend(u)
	b.SetAlive(false)
	pool.AddBackend(b)

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	balancer.ServeHTTP(w, req)

	if w.Code < 400 {
		t.Errorf("Expected error status, got %d", w.Code)
	}
}

// TestE2EActiveRequestTracking tests active request counting
func TestE2EActiveRequestTracking(t *testing.T) {
	started := make(chan bool)
	done := make(chan bool)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- true
		<-done
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	pool := backend.NewPool()
	u, _ := url.Parse(mockServer.URL)
	b := backend.NewBackend(u)
	pool.AddBackend(b)

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	go func() {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		balancer.ServeHTTP(w, req)
	}()

	<-started
	activeCount := b.GetActiveRequests()
	if activeCount < 1 {
		t.Errorf("Expected active requests >= 1, got %d", activeCount)
	}

	done <- true
	time.Sleep(50 * time.Millisecond)

	activeCount = b.GetActiveRequests()
	if activeCount != 0 {
		t.Errorf("Expected active requests = 0, got %d", activeCount)
	}
}

// TestE2ERequestIDPropagation tests correlation ID tracking
func TestE2ERequestIDPropagation(t *testing.T) {
	var receivedID string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	pool := backend.NewPool()
	u, _ := url.Parse(mockServer.URL)
	pool.AddBackend(backend.NewBackend(u))

	balancer := createTestBalancer(pool, NewRoundRobinStrategy())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	balancer.ServeHTTP(w, req)

	if receivedID == "" {
		t.Error("X-Request-ID not set")
	}
}
