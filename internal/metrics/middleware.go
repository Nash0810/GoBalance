package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// Middleware wraps http.Handler to collect metrics
type Middleware struct {
	collector *Collector
	next      http.Handler
}

// NewMiddleware creates metrics middleware
func NewMiddleware(collector *Collector, next http.Handler) *Middleware {
	return &Middleware{
		collector: collector,
		next:      next,
	}
}

// ServeHTTP implements http.Handler interface
func (m *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Wrap response writer to capture status code
	crw := &CaptureResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// Call next handler
	m.next.ServeHTTP(crw, r)

	// Record metrics
	duration := time.Since(start).Seconds()
	statusStr := strconv.Itoa(crw.statusCode)

	m.collector.RequestsTotal.WithLabelValues("all", r.Method, statusStr).Inc()
	m.collector.RequestDuration.WithLabelValues("all", r.Method).Observe(duration)
}

// CaptureResponseWriter captures HTTP status code
type CaptureResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (crw *CaptureResponseWriter) WriteHeader(code int) {
	crw.statusCode = code
	crw.ResponseWriter.WriteHeader(code)
}
