package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Get port from command line or use default
	port := "8081"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	// Health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"healthy","port":%s}`, port)
	})

	// Main endpoint
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Log the request
		log.Printf("[Port %s] %s %s", port, r.Method, r.RequestURI)

		// Handle different paths
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"backend":"test-server","port":%s,"method":"%s"}`, port, r.Method)

		case "/api/test":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"message":"test response","port":%s}`, port)

		case "/delay":
			// Simulate slow endpoint
			delay := time.Duration(100) * time.Millisecond
			time.Sleep(delay)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"ok","delay_ms":100}`)

		case "/error":
			// Return error
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error":"simulated error"}`)

		default:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"path":"%s","port":%s}`, r.URL.Path, port)
		}
	})

	// Start server
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Test server listening on port %s", port)
	log.Fatal(http.ListenAndServe(addr, nil))
}
