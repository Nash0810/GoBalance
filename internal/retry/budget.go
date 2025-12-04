package retry

import (
	"sync/atomic"
	"time"
)

// Budget limits the number of retries globally using token bucket algorithm
// FIX #9: Implemented adaptive retry budget based on actual request rate
type Budget struct {
	tokens         int64 // Available tokens
	maxTokens      int64 // Maximum tokens
	percent        int   // Percentage for retry budget
	refillRate     int64 // Tokens added per second
	lastRefill     int64 // Unix timestamp of last refill
	requestCounter int64 // Track actual request rate
}

// NewBudget creates a new retry budget
// percent: percentage of requests that can be retries (1-100)
func NewBudget(percent int) *Budget {
	if percent < 1 {
		percent = 1
	}
	if percent > 100 {
		percent = 100
	}

	// Initial assumption: 1000 req/s baseline, allow percent% to be retries
	maxTokens := int64(1000 * percent / 100)

	return &Budget{
		tokens:         maxTokens,
		maxTokens:      maxTokens,
		percent:        percent,
		refillRate:     maxTokens,
		lastRefill:     time.Now().Unix(),
		requestCounter: 0,
	}
}

// TryConsume attempts to consume a token for retry
// Returns true if retry is allowed, false if budget exhausted
func (b *Budget) TryConsume() bool {
	b.refill()

	for {
		current := atomic.LoadInt64(&b.tokens)
		if current <= 0 {
			return false // Budget exhausted
		}

		if atomic.CompareAndSwapInt64(&b.tokens, current, current-1) {
			return true // Token consumed
		}
	}
}

// TrackRequest increments the request counter for adaptive rate calculation
func (b *Budget) TrackRequest() {
	atomic.AddInt64(&b.requestCounter, 1)
}

// refill adds tokens based on elapsed time and adapts to actual request rate
func (b *Budget) refill() {
	now := time.Now().Unix()
	last := atomic.LoadInt64(&b.lastRefill)

	if now <= last {
		return // No time elapsed
	}

	if !atomic.CompareAndSwapInt64(&b.lastRefill, last, now) {
		return // Another goroutine is refilling
	}

	// FIX #9: Calculate actual request rate and adjust refill
	actualRate := atomic.SwapInt64(&b.requestCounter, 0)
	if actualRate > 0 {
		// Adjust refill rate based on actual traffic
		b.refillRate = actualRate * int64(b.percent) / 100
		if b.refillRate < 1 {
			b.refillRate = 1
		}
		// Update max tokens based on actual rate
		newMaxTokens := actualRate * int64(b.percent) / 100
		if newMaxTokens > 0 {
			atomic.StoreInt64(&b.maxTokens, newMaxTokens)
		}
	}

	elapsed := now - last
	tokensToAdd := elapsed * b.refillRate

	for {
		current := atomic.LoadInt64(&b.tokens)
		newTokens := current + tokensToAdd
		maxTokens := atomic.LoadInt64(&b.maxTokens)
		if newTokens > maxTokens {
			newTokens = maxTokens
		}

		if atomic.CompareAndSwapInt64(&b.tokens, current, newTokens) {
			break
		}
	}
}

// GetAvailable returns the number of available tokens
func (b *Budget) GetAvailable() int64 {
	b.refill()
	return atomic.LoadInt64(&b.tokens)
}
