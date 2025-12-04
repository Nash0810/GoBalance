package health

import (
	"log"
	"sync"
	"time"
)

// CircuitState represents the circuit breaker state
type CircuitState int

const (
	// StateClosed means circuit is healthy, requests pass through
	StateClosed CircuitState = iota

	// StateOpen means circuit is broken, requests fail fast
	StateOpen

	// StateHalfOpen means circuit is testing if backend recovered
	StateHalfOpen
)

func (cs CircuitState) String() string {
	switch cs {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreaker implements the circuit breaker pattern with sliding window
// FIX #6: Implemented sliding window for failure counting
type CircuitBreaker struct {
	name             string
	state            CircuitState
	successes        int64
	lastFailTime     time.Time
	recentFailures   []time.Time // FIX #6: Sliding window of recent failures
	mux              sync.RWMutex

	// Configuration
	failureThreshold int           // Failures before opening circuit
	successThreshold int           // Successes to close circuit from half-open
	timeout          time.Duration // Time before trying half-open
	windowSize       time.Duration // FIX #6: Rolling window duration
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string) *CircuitBreaker {
	return &CircuitBreaker{
		name:             name,
		state:            StateClosed,
		recentFailures:   make([]time.Time, 0),
		failureThreshold: 5,
		successThreshold: 2,
		timeout:          30 * time.Second,
		windowSize:       10 * time.Second, // FIX #6: 10 second sliding window
	}
}

// AllowRequest returns true if request is allowed through circuit
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mux.Lock()
	defer cb.mux.Unlock()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if timeout elapsed, move to half-open
		if time.Since(cb.lastFailTime) >= cb.timeout {
			log.Printf("[CIRCUIT] %s: OPEN → HALF_OPEN (timeout elapsed)", cb.name)
			cb.state = StateHalfOpen
			cb.successes = 0
			return true
		}
		return false // Still open, reject request

	case StateHalfOpen:
		return true // Allow test request

	default:
		return false
	}
}

// RecordSuccess records a successful request
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mux.Lock()
	defer cb.mux.Unlock()

	cb.successes++

	if cb.state == StateHalfOpen {
		if cb.successes >= int64(cb.successThreshold) {
			log.Printf("[CIRCUIT] %s: HALF_OPEN → CLOSED (after %d successes)",
				cb.name, cb.successes)
			cb.state = StateClosed
			cb.recentFailures = make([]time.Time, 0) // Clear failure history
			cb.successes = 0
		}
	} else if cb.state == StateClosed {
		// FIX #6: On success, clean old failures from sliding window
		cb.cleanOldFailures()
	}
}

// RecordFailure records a failed request
// FIX #6: Uses sliding window for failure counting
func (cb *CircuitBreaker) RecordFailure() {
	cb.mux.Lock()
	defer cb.mux.Unlock()

	now := time.Now()
	cb.recentFailures = append(cb.recentFailures, now)
	cb.lastFailTime = now

	// FIX #6: Remove failures outside the sliding window
	cb.cleanOldFailures()

	if cb.state == StateHalfOpen {
		log.Printf("[CIRCUIT] %s: HALF_OPEN → OPEN (test failed)", cb.name)
		cb.state = StateOpen
		cb.successes = 0
	} else if cb.state == StateClosed {
		// FIX #6: Check failures within sliding window
		if len(cb.recentFailures) >= cb.failureThreshold {
			log.Printf("[CIRCUIT] %s: CLOSED → OPEN (after %d failures in %v window)",
				cb.name, len(cb.recentFailures), cb.windowSize)
			cb.state = StateOpen
		}
	}
}

// cleanOldFailures removes failures outside the sliding window
// FIX #6: Sliding window implementation
func (cb *CircuitBreaker) cleanOldFailures() {
	cutoff := time.Now().Add(-cb.windowSize)
	validFailures := make([]time.Time, 0)

	for _, t := range cb.recentFailures {
		if t.After(cutoff) {
			validFailures = append(validFailures, t)
		}
	}

	cb.recentFailures = validFailures
}

// GetState returns the current circuit state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mux.RLock()
	defer cb.mux.RUnlock()
	return cb.state
}
