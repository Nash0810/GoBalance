package retry

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
)

// Policy determines whether a request should be retried
type Policy struct {
	maxAttempts int
	budget      *Budget
}

// NewPolicy creates a new retry policy
func NewPolicy(maxAttempts int, budgetPercent int) *Policy {
	return &Policy{
		maxAttempts: maxAttempts,
		budget:      NewBudget(budgetPercent),
	}
}

// ShouldRetry determines if a request should be retried
// FIX #4: Added context cancellation check
func (p *Policy) ShouldRetry(req *http.Request, err error, attempt int) bool {
	// FIX #4: Check if client canceled (context propagation)
	if req.Context().Err() != nil {
		log.Printf("[RETRY] Request context canceled, skipping retry")
		return false
	}

	// Check attempt limit
	if attempt >= p.maxAttempts {
		log.Printf("[RETRY] Max attempts (%d) reached", p.maxAttempts)
		return false
	}

	// Check if method is idempotent
	if !isIdempotent(req.Method) {
		log.Printf("[RETRY] Method %s is not idempotent, skipping retry", req.Method)
		return false
	}

	// Check if error is retryable
	if err == nil {
		return false
	}

	if !isRetryableError(err) {
		log.Printf("[RETRY] Error is not retryable: %v", err)
		return false
	}

	// Track request for adaptive budget
	p.budget.TrackRequest()

	// Check retry budget
	if !p.budget.TryConsume() {
		log.Printf("[RETRY] Retry budget exhausted (available: %d)", p.budget.GetAvailable())
		return false
	}

	log.Printf("[RETRY] Retry allowed (attempt %d/%d)", attempt+1, p.maxAttempts)
	return true
}

// GetBudget returns the budget for metrics tracking
func (p *Policy) GetBudget() *Budget {
	return p.budget
}

// isIdempotent returns true if HTTP method is safe to retry
func isIdempotent(method string) bool {
	// Safe methods: can be retried without side effects
	switch method {
	case "GET", "HEAD", "OPTIONS", "PUT", "DELETE":
		return true
	case "POST":
		return false // POST is NOT idempotent by default
	default:
		return false
	}
}

// isRetryableError returns true if error indicates connection failure
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Connection errors (retryable)
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no route to host",
		"i/o timeout",
		"EOF",
		"deadline exceeded",
		"status 5", // 5xx errors
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), retryable) {
			return true
		}
	}

	return false
}

// BufferRequestBody reads and buffers the request body for potential retries
// FIX #2: Implemented request body buffering for retries
func BufferRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body.Close()

	return bodyBytes, nil
}

// RestoreRequestBody restores the buffered body to the request
// FIX #2: Restore body for retry attempts
func RestoreRequestBody(req *http.Request, bodyBytes []byte) {
	if bodyBytes != nil {
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}
}
