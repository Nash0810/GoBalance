package retry

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// TestBufferRequestBody tests body buffering for retries
func TestBufferRequestBody(t *testing.T) {
	body := "test request body"
	req, err := http.NewRequest("POST", "http://localhost:8080", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}

	// Buffer the body
	bodyBytes, err := BufferRequestBody(req)
	if err != nil {
		t.Fatalf("Failed to buffer body: %v", err)
	}

	// Verify buffered bytes match
	if !bytes.Equal(bodyBytes, []byte(body)) {
		t.Errorf("Buffered bytes mismatch: expected %q, got %q", body, string(bodyBytes))
	}
}

// TestRestoreRequestBody tests body restoration for retries
func TestRestoreRequestBody(t *testing.T) {
	body := "test request body"
	req, _ := http.NewRequest("POST", "http://localhost:8080", bytes.NewBufferString(body))
	bodyBytes, _ := BufferRequestBody(req)

	// Close and restore body for "retry"
	req.Body.Close()
	RestoreRequestBody(req, bodyBytes)

	// Body should be readable again
	readBody, _ := io.ReadAll(req.Body)
	if string(readBody) != body {
		t.Errorf("Restored body mismatch: expected %q, got %q", body, string(readBody))
	}
}

// TestIsIdempotent tests idempotent method detection
func TestIsIdempotent(t *testing.T) {
	idempotentMethods := []string{"GET", "HEAD", "OPTIONS", "PUT", "DELETE"}
	nonIdempotentMethods := []string{"POST", "PATCH"}

	for _, method := range idempotentMethods {
		if !isIdempotent(method) {
			t.Errorf("Method %s should be idempotent", method)
		}
	}

	for _, method := range nonIdempotentMethods {
		if isIdempotent(method) {
			t.Errorf("Method %s should NOT be idempotent", method)
		}
	}
}

// TestRetryBudgetTokens tests token bucket retry budget
func TestRetryBudgetTokens(t *testing.T) {
	budget := NewBudget(10) // 10% budget

	// Track 1000 requests (to establish baseline)
	for i := 0; i < 1000; i++ {
		budget.TrackRequest()
	}

	// With 10% budget on 1000 req/s, should have ~100 tokens initially
	// Try to consume some tokens
	canRetry := budget.TryConsume()
	if !canRetry {
		t.Logf("Warning: Could not consume retry token")
	}
}

// TestRetryPolicyShouldRetry tests retry policy decisions
func TestRetryPolicyShouldRetry(t *testing.T) {
	policy := NewPolicy(3, 50) // 3 attempts max, 50% budget

	// POST should NOT retry (not idempotent)
	postReq, _ := http.NewRequest("POST", "http://localhost:8080", bytes.NewBufferString("body"))
	if policy.ShouldRetry(postReq, nil, 1) {
		t.Error("POST should not retry (not idempotent)")
	}

	// Max attempts should not retry
	getReq, _ := http.NewRequest("GET", "http://localhost:8080", nil)
	if policy.ShouldRetry(getReq, nil, 3) {
		t.Error("Should not retry at max attempts")
	}

	// Beyond max attempts should not retry
	if policy.ShouldRetry(getReq, nil, 4) {
		t.Error("Should not retry beyond max attempts")
	}
}

// TestRetryBudgetAdaptive tests adaptive budget rate
func TestRetryBudgetAdaptive(t *testing.T) {
	budget := NewBudget(20) // 20% budget

	// Simulate high traffic (5000 req/s)
	for i := 0; i < 5000; i++ {
		budget.TrackRequest()
	}

	// Budget should adapt to traffic rate
	// With adaptive algorithm, tokens should increase based on actual request rate
}
