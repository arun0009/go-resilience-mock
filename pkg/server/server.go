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
	"github.com/arun0009/go-resilience-mock/pkg/observability"
	"github.com/gorilla/mux"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type contextKey string

const requestIDKey contextKey = "requestID"

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

// requestIDMiddleware adds a simple request ID to the context.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := atomic.AddUint64(&config.RequestCounter, 1)
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
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
		var bodyReader io.Reader = r.Body

		if r.Body != nil {
			// http.MaxBytesReader requires io.ReadCloser
			bodyReader = http.MaxBytesReader(rwl, r.Body, cfg.MaxBodySize)
			// io.TeeReader returns io.Reader, so we wrap it in NopCloser to satisfy io.ReadCloser if needed later,
			// but here we just need to read from it.
			// However, r.Body assignment requires io.ReadCloser.
			tee := io.TeeReader(bodyReader, &bodyBuf)
			r.Body = io.NopCloser(tee)
		}

		next.ServeHTTP(rwl, r)

		duration := time.Since(startTime)

		observability.ResponseDuration.WithLabelValues(
			r.URL.Path, r.Method, strconv.Itoa(rwl.statusCode),
		).Observe(duration.Seconds())

		history, historyMutex := config.GetRequestHistory()
		historyMutex.Lock()
		if len(history) >= cfg.HistorySize {
			history = history[1:]
		}

		bodySnippet := bodyBuf.String()
		if len(bodySnippet) > 256 {
			bodySnippet = bodySnippet[:256] + "..."
		}

		// Retrieve Request ID from context
		reqID := r.Context().Value(requestIDKey)
		reqIDStr := "unknown"
		if reqID != nil {
			reqIDStr = strconv.FormatUint(reqID.(uint64), 10)
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
		}
		config.RequestHistory = append(history, record)
		historyMutex.Unlock()

		if cfg.LogRequests {
			log.Printf("[%s] %s | Status: %d | Time: %s", r.Method, r.URL.Path, rwl.statusCode, duration)
		}
	})
}

// NewRouter sets up all routes for the server.
func NewRouter(cfg config.Config) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	router.Use(loggingMiddleware)
	router.Use(corsMiddleware)
	router.Use(requestIDMiddleware)
	if config.GetRateLimiter() != nil {
		router.Use(rateLimitMiddleware)
	}

	// Stress
	router.HandleFunc("/api/stress/cpu/{duration}", faults.HandleCPUStress).Methods("GET")
	router.HandleFunc("/api/stress/mem/{size}", faults.HandleMemoryStress).Methods("GET")

	// Control / Reset
	router.HandleFunc("/api/control/reset-history", handleResetHistory).Methods("POST")
	router.HandleFunc("/api/control/reset-metrics", handleResetMetrics).Methods("POST")

	// Core
	router.HandleFunc("/echo", faults.HandleEcho).Methods("GET", "POST", "PUT", "DELETE", "PATCH")
	router.HandleFunc("/history", handleHistory).Methods("GET")
	router.HandleFunc("/dump", handleDump).Methods("GET")
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
	router.HandleFunc("/info", handleInfo).Methods("GET")

	// Documentation
	// Serve the docs directory at /docs/

	router.PathPrefix("/docs/").Handler(http.StripPrefix("/docs/", http.FileServer(http.Dir("docs"))))

	// Catch-all: Check if it matches a dynamic scenario, otherwise Echo
	router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if a scenario exists for this path/method
		key := r.URL.Path + "_" + r.Method
		if _, ok := config.GetScenarios().Load(key); ok {
			faults.HandleScenario(w, r)
			return
		}
		faults.HandleEcho(w, r)
	})

	return router
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
	history, historyMutex := config.GetRequestHistory()
	historyMutex.Lock()
	defer historyMutex.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(history)
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetConfig()
	history, historyMutex := config.GetRequestHistory()
	historyMutex.Lock()
	historyCount := len(history)
	historyMutex.Unlock()

	info := map[string]interface{}{
		"hostname":       cfg.Hostname,
		"port":           cfg.Port,
		"tls_enabled":    cfg.EnableTLS,
		"cors_enabled":   cfg.EnableCORS,
		"history_count":  historyCount,
		"total_requests": atomic.LoadUint64(&config.RequestCounter),
		"uptime":         "Not tracked (stateless)", // Could add start time if needed
		"version":        "1.0.0",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

func handleDump(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleResetHistory clears the recorded request history.
func handleResetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	history, historyMutex := config.GetRequestHistory()
	historyMutex.Lock()
	defer historyMutex.Unlock()
	config.RequestHistory = history[:0]
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

	history, historyMutex := config.GetRequestHistory()
	historyMutex.Lock()
	var record config.RequestRecord
	found := false
	for _, rec := range history {
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
	// Try parsing as list first
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if err := json.Unmarshal(bodyBytes, &scenarios); err != nil {
		// Try single object
		var s config.Scenario
		if err2 := json.Unmarshal(bodyBytes, &s); err2 != nil {
			http.Error(w, "Invalid scenario JSON", http.StatusBadRequest)
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
