package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/arun0009/go-resilience-mock/pkg/faults"
	"github.com/arun0009/go-resilience-mock/pkg/health"
	"github.com/arun0009/go-resilience-mock/pkg/observability"
	"github.com/gorilla/mux"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type contextKey string

// RequestIDKey is the context key for request IDs (exported for use in templates)
const RequestIDKey contextKey = "requestID"

// responseWriterLogger wraps the http.ResponseWriter to capture status code.
type responseWriterLogger struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriterLogger(w http.ResponseWriter) *responseWriterLogger {
	return &responseWriterLogger{w, http.StatusOK}
}

func (rwl *responseWriterLogger) WriteHeader(code int) {
	rwl.statusCode = code
	rwl.ResponseWriter.WriteHeader(code)
}

func (rwl *responseWriterLogger) Flush() {
	if flusher, ok := rwl.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rwl *responseWriterLogger) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rwl.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
}

// --- Middlewares ---

// requestIDMiddleware adds a request ID to the context.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for incoming Request ID
		reqIDStr := r.Header.Get("X-Request-ID")
		if reqIDStr == "" {
			// Generate new ID if not present
			id := atomic.AddUint64(&config.RequestCounter, 1)
			reqIDStr = strconv.FormatUint(id, 10)
		}

		// Set it back in response header for tracing
		w.Header().Set("X-Request-ID", reqIDStr)

		ctx := context.WithValue(r.Context(), RequestIDKey, reqIDStr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// corsMiddleware handles Cross-Origin Resource Sharing.
func corsMiddleware(next http.Handler) http.Handler {
	cfg := config.GetConfig()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.EnableCORS {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Echo-Delay, X-Echo-Status, X-Echo-Headers, X-Echo-Body")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware applies rate limiting if configured.
func rateLimitMiddleware(next http.Handler) http.Handler {
	limiter := config.GetRateLimiter()
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware handles request logging and request history recording.
func loggingMiddleware(next http.Handler) http.Handler {
	cfg := config.GetConfig()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		observability.InflightRequests.Inc()
		defer observability.InflightRequests.Dec()

		rwl := newResponseWriterLogger(w)

		var bodyBuf bytes.Buffer

		if r.Body != nil {
			// Limit body size to prevent DoS
			maxReader := http.MaxBytesReader(rwl, r.Body, cfg.MaxBodySize)

			// Read the body fully
			bodyBytes, err := io.ReadAll(maxReader)
			if err != nil {
				// If the body is too large, MaxBytesReader returns an error.
				// We log it and proceed with what we have (or empty).
				// In a real server we might want to return 413 here, but this is middleware.
				// For now, we just log the error if it's not EOF
				if err != io.EOF {
					log.Printf("Error reading request body: %v", err)
				}
			}

			// Restore r.Body so the handler can read it
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// Write to bodyBuf for logging
			bodyBuf.Write(bodyBytes)
		}

		next.ServeHTTP(rwl, r)

		duration := time.Since(startTime)

		observability.ResponseDuration.WithLabelValues(
			r.URL.Path, r.Method, strconv.Itoa(rwl.statusCode),
		).Observe(duration.Seconds())

		historyMutex := config.GetHistoryMutex()
		historyMutex.Lock()
		if len(config.RequestHistory) >= cfg.HistorySize {
			config.RequestHistory = config.RequestHistory[1:]
		}

		bodySnippet := ""
		if cfg.LogBody {
			// Store the raw JSON body as-is when LogBody is true
			bodySnippet = bodyBuf.String()
		} else {
			// Otherwise, store a truncated version
			bodySnippet = bodyBuf.String()
			if len(bodySnippet) > 256 {
				bodySnippet = bodySnippet[:256] + "..."
			}
		}

		// Retrieve Request ID from context
		reqID := r.Context().Value(RequestIDKey)
		reqIDStr := "unknown"
		if val, ok := reqID.(string); ok {
			reqIDStr = val
		} else if val, ok := reqID.(uint64); ok {
			reqIDStr = strconv.FormatUint(val, 10)
		}

		record := config.RequestRecord{
			ID:          reqIDStr,
			Timestamp:   startTime,
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.RawQuery,
			RemoteAddr:  r.RemoteAddr,
			Headers:     r.Header,
			BodySnippet: bodySnippet,
			StatusCode:  rwl.statusCode, // Capture the status code
		}
		config.RequestHistory = append(config.RequestHistory, record)
		historyMutex.Unlock()

		if cfg.LogRequests {
			log.Printf("[%s] %s | Status: %d | Time: %s", r.Method, r.URL.Path, rwl.statusCode, duration)
		}
	})
}

// NewRouter sets up all routes for the server.
func NewRouter(cfg config.Config) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	router.Use(requestIDMiddleware) // Must be first to set request ID
	router.Use(loggingMiddleware)   // Reads request ID from context
	router.Use(corsMiddleware)
	if config.GetRateLimiter() != nil {
		router.Use(rateLimitMiddleware)
	}

	// Health Check
	h := health.NewHealth()
	h.AddCheck("ping", func() (string, error) {
		return "pong", nil
	})
	router.HandleFunc("/health", h.Handler().ServeHTTP).Methods("GET")

	// Stress
	router.HandleFunc("/api/stress/cpu/{duration}", faults.HandleCPUStress).Methods("GET")
	router.HandleFunc("/api/stress/mem/{size}", faults.HandleMemoryStress).Methods("GET")

	// Control / Reset
	router.HandleFunc("/api/control/reset-history", handleResetHistory).Methods("POST")
	router.HandleFunc("/api/control/reset-metrics", handleResetMetrics).Methods("POST")

	// Core
	router.HandleFunc("/echo", faults.HandleEcho).Methods("GET", "POST", "PUT", "DELETE", "PATCH")
	router.HandleFunc("/history", handleHistory).Methods("GET")
	router.HandleFunc("/replay", handleReplay).Methods("POST")
	router.HandleFunc("/scenario", handleAddScenario).Methods("POST")

	// Streaming
	router.HandleFunc("/ws", faults.HandleWebsocket)
	router.HandleFunc("/sse", faults.HandleSSE)

	// Scenarios
	config.GetScenarios().Range(func(key, value interface{}) bool {
		// key is path_method.
		// parts[0] is the path template (e.g. /users/{id})
		// parts[1] is the method
		parts := strings.Split(key.(string), "_")
		router.HandleFunc(parts[0], faults.HandleScenario).Methods(parts[1])
		return true
	})

	router.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "docs/favicon.ico")
	})

	// Web Interface
	router.HandleFunc("/web-ws", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "docs/web-ws.html")
	})
	router.HandleFunc("/web-sse", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "docs/web-sse.html")
	})

	// Documentation
	// Serve the docs directory at /docs/

	router.PathPrefix("/docs/").Handler(http.StripPrefix("/docs/", http.FileServer(http.Dir("docs"))))

	// Catch-all: Check if it matches a dynamic scenario, otherwise 404
	router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Try exact match first (fast path)
		key := r.URL.Path + "_" + r.Method
		if _, ok := config.GetScenarios().Load(key); ok {
			faults.HandleScenario(w, r)
			return
		}

		// 2. Try matching path templates (slow path)
		// We iterate through all scenarios to see if any path template matches the current request
		var matched bool
		config.GetScenarios().Range(func(k, v interface{}) bool {
			scenarioKey := k.(string)
			parts := strings.Split(scenarioKey, "_")
			if len(parts) != 2 {
				return true
			}
			tmplPath := parts[0]
			method := parts[1]

			if method != r.Method {
				return true
			}

			// Check if template matches
			// We use a temporary router to check for match and extract vars
			// This is inefficient for high throughput but functional for a mock server
			// A better approach would be to dynamically update the main router, but gorilla/mux doesn't support that easily.

			// Simple check: does it look like a template?
			if strings.Contains(tmplPath, "{") {
				// Manual matching logic or use a helper
				// For simplicity, let's try to match using a throwaway route match
				// This is tricky without access to the route infrastructure.

				// Alternative: We can't easily use gorilla/mux matching here without registering a route.
				// But we can try to register it on a temporary router? No, too slow.

				// Let's implement a basic path matcher for {id} style vars
				vars, matches := matchPath(tmplPath, r.URL.Path)
				if matches {
					// Inject vars into context so HandleScenario can use them
					r = mux.SetURLVars(r, vars)
					// We found a match, but we need to pass the *Scenario* or let HandleScenario find it.
					// HandleScenario looks up by r.URL.Path which won't work.
					// We need to modify HandleScenario or pass the matched template path.

					// Hack: We can temporarily set the URL path to the template path?
					// No, that breaks other things.
					// Better: Put the template path in context.
					ctx := context.WithValue(r.Context(), faults.MatchedTemplateKey, tmplPath)
					faults.HandleScenario(w, r.WithContext(ctx))
					matched = true
					return false // Stop iteration
				}
			}
			return true
		})

		if !matched {
			http.NotFound(w, r)
		}
	})

	return router
}

// matchPath checks if a request path matches a template path (e.g. /users/{id})
// and returns the extracted variables.
func matchPath(template, path string) (map[string]string, bool) {
	tmplParts := strings.Split(strings.Trim(template, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(tmplParts) != len(pathParts) {
		return nil, false
	}

	vars := make(map[string]string)
	for i, part := range tmplParts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			key := part[1 : len(part)-1]
			vars[key] = pathParts[i]
		} else if part != pathParts[i] {
			return nil, false
		}
	}
	return vars, true
}

// Run starts the HTTP server.
func Run(cfg config.Config) error {
	printBanner(cfg)
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      h2c.NewHandler(NewRouter(cfg), &http2.Server{}),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  0,
	}
	log.Printf("Starting HTTP server on port %s", cfg.Port)
	return server.ListenAndServe()
}

// RunTLS starts the HTTPS server.
func RunTLS(cfg config.Config) error {
	printBanner(cfg)
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      h2c.NewHandler(NewRouter(cfg), &http2.Server{}),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  0,
	}

	log.Printf("Starting HTTPS server on port %s", cfg.Port)
	return server.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
}

func printBanner(cfg config.Config) {
	banner := `
   ___       ___        _ _ _                   __  __         _   
  / __|___  | _ \___ __(_) (_)___ _ _  __ ___  |  \/  |___  __| |__
 | (_ / _ \ |   / -_|_-< | | / -_) ' \/ _/ -_) | |\/| / _ \/ _| / /
  \___\___/ |_|_\___/__/_|_|_\___|_||_\__\___| |_|  |_\___/\__|_\_\
                                                                   
`
	fmt.Println(banner)
	fmt.Printf("Go Resilience Mock v1.0.0\n")
	fmt.Printf("Port: %s | TLS: %v | CORS: %v\n", cfg.Port, cfg.EnableTLS, cfg.EnableCORS)
	fmt.Println(strings.Repeat("-", 60))
}

// --- Helper Handlers (Removed front-end HTML, kept API endpoints) ---

func handleHistory(w http.ResponseWriter, r *http.Request) {
	historyMutex := config.GetHistoryMutex()
	historyMutex.Lock()
	defer historyMutex.Unlock()

	history := config.RequestHistory

	cfg := config.GetConfig()

	// Transform to simplified format
	simplified := make([]map[string]interface{}, 0, len(history))
	for _, record := range history {
		userAgent := ""
		if ua, ok := record.Headers["User-Agent"]; ok && len(ua) > 0 {
			userAgent = ua[0]
		}

		entry := map[string]interface{}{
			"id":        record.ID,
			"time":      record.Timestamp.Format("15:04:05"),
			"method":    record.Method,
			"path":      record.Path,
			"query":     record.Query,
			"status":    record.StatusCode,
			"userAgent": userAgent,
		}

		// If LogBody is enabled, include the raw body in the response
		if cfg.LogBody && record.BodySnippet != "" {
			var bodyData interface{}
			if err := json.Unmarshal([]byte(record.BodySnippet), &bodyData); err == nil {
				// If it's valid JSON, include it as parsed JSON
				entry["body"] = bodyData
			} else {
				// If it's not valid JSON, include it as a string
				entry["body"] = record.BodySnippet
			}
		}

		simplified = append(simplified, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(simplified)
}

// handleResetHistory clears the recorded request history.
func handleResetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	historyMutex := config.GetHistoryMutex()
	historyMutex.Lock()
	defer historyMutex.Unlock()
	config.RequestHistory = config.RequestHistory[:0]
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Request history cleared."))
}

// handleResetMetrics clears custom Prometheus metrics.
func handleResetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	observability.FaultsInjected.Reset()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Injected fault metrics reset."))
}

// handleReplay replays a request from history.
func handleReplay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     string `json:"id"`
		Target string `json:"target"` // Optional target URL
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	historyMutex := config.GetHistoryMutex()
	historyMutex.Lock()
	var record config.RequestRecord
	found := false
	for _, rec := range config.RequestHistory {
		if rec.ID == req.ID {
			record = rec
			found = true
			break
		}
	}
	historyMutex.Unlock()

	if !found {
		http.Error(w, "Request ID not found", http.StatusNotFound)
		return
	}

	target := req.Target
	if target == "" {
		// Default to self if no target provided (best effort)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		target = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	// Construct new request
	newReq, err := http.NewRequest(record.Method, target+record.Path, strings.NewReader(record.BodySnippet))
	if err != nil {
		http.Error(w, "Failed to create replay request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, v := range record.Headers {
		for _, val := range v {
			newReq.Header.Add(k, val)
		}
	}

	// Execute
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(newReq)
	if err != nil {
		http.Error(w, "Replay failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Return result
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleAddScenario adds a new scenario dynamically.
func handleAddScenario(w http.ResponseWriter, r *http.Request) {
	var scenarios []config.Scenario
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Try parsing as a list of scenarios first
	if err := json.Unmarshal(bodyBytes, &scenarios); err != nil {
		// Try parsing as a single scenario
		var s config.Scenario
		if err2 := json.Unmarshal(bodyBytes, &s); err2 != nil {
			http.Error(w, "Invalid scenario JSON: "+err2.Error(), http.StatusBadRequest)
			return
		}
		scenarios = []config.Scenario{s}
	}

	for i := range scenarios {
		config.AddScenario(&scenarios[i])
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Scenarios added."))
}
