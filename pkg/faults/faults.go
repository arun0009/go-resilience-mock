package faults

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	mrand "math/rand"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/arun0009/go-resilience-mock/pkg/observability"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// --- Request and Scenario Handling ---

// TemplateData provides contextual data to the response body template.
type TemplateData struct {
	Request struct {
		Method  string
		Path    string
		Query   url.Values
		Headers http.Header
		Body    string
	}
	Server struct {
		Hostname  string
		Timestamp time.Time
		FaultType string
	}
}

func HandleScenario(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetConfig()
	scenariosMap := config.GetScenarios()

	// We must look up the scenario using the registered template path, not the raw request path.
	route := mux.CurrentRoute(r)
	pathTemplate, err := route.GetPathTemplate()
	if err != nil || pathTemplate == "/" {
		// Fallback for exact matches if no template logic was used or if matched via catch-all
		pathTemplate = r.URL.Path
	}

	key := pathTemplate + "_" + r.Method
	v, ok := scenariosMap.Load(key)
	if !ok {
		// Safe fallback if lookup fails
		log.Printf("Scenario mismatch: %s not found", key)
		http.Error(w, "Scenario not found for path/method", http.StatusNotFound)
		return
	}

	scenariosList := v.([]*config.Scenario)
	var scenario *config.Scenario

	// Find the first matching scenario
	for _, s := range scenariosList {
		if matchesRequest(s, r) {
			scenario = s
			break
		}
	}

	if scenario == nil {
		// No matching scenario found, fallback to Echo
		HandleEcho(w, r)
		return
	}

	// --- Circuit Breaker Check ---
	if scenario.CircuitBreaker.FailureThreshold > 0 {
		if !checkCircuitBreaker(scenario) {
			http.Error(w, "Service Unavailable (Circuit Breaker Open)", http.StatusServiceUnavailable)
			return
		}
	}

	// Atomically increment and wrap the index
	idx := int(atomic.LoadInt32(&scenario.Index))
	response := scenario.Responses[idx]

	nextIdx := (idx + 1) % len(scenario.Responses)
	atomic.StoreInt32(&scenario.Index, int32(nextIdx))

	// --- 0. Probability Check ---
	// If Probability is set (e.g. 0.25), we only trigger the fault 25% of the time.
	// Otherwise we skip to a default "success" behavior (Echo).
	if response.Probability > 0.0 && response.Probability < 1.0 {
		if mrand.Float64() > response.Probability {
			// Probability check failed; DO NOT inject fault.
			// Fallback to Echo to simulate a "success" pass-through.
			HandleEcho(w, r)
			// Treat as success for CB
			updateCircuitBreaker(scenario, true)
			return
		}
	}

	// --- 1. Fault Injection: Delay ---
	if response.Delay > 0 {
		observability.FaultsInjected.WithLabelValues("delay", pathTemplate).Inc()
		time.Sleep(response.Delay)
	}

	// Track HTTP error faults
	isFailure := false
	if response.Status >= 500 {
		isFailure = true
		observability.FaultsInjected.WithLabelValues("http_error", pathTemplate).Inc()
	} else if response.Status >= 400 {
		observability.FaultsInjected.WithLabelValues("http_error", pathTemplate).Inc()
	}

	// Update Circuit Breaker State
	if scenario.CircuitBreaker.FailureThreshold > 0 {
		updateCircuitBreaker(scenario, !isFailure)
	}

	// --- 2. Headers and Status ---
	if response.Headers != nil {
		for k, v := range response.Headers {
			w.Header().Set(k, v)
		}
	}

	// --- 3. Dynamic Response Templating ---
	// Prepare data for the template
	templateData := TemplateData{
		Request: struct {
			Method  string
			Path    string
			Query   url.Values
			Headers http.Header
			Body    string
		}{
			Method:  r.Method,
			Path:    r.URL.Path,
			Query:   r.URL.Query(),
			Headers: r.Header,
			Body:    "",
		},
		Server: struct {
			Hostname  string
			Timestamp time.Time
			FaultType string
		}{
			Hostname:  cfg.Hostname,
			Timestamp: time.Now(),
			FaultType: func() string {
				if response.Delay > 0 {
					return "delay"
				}
				if response.Status >= 400 {
					return "error"
				}
				return "none"
			}(),
		},
	}

	var finalBody string
	if strings.Contains(response.Body, "{{") {
		var err error
		finalBody, err = executeTemplate(response.Body, templateData)
		if err != nil {
			log.Printf("Error executing template for %s: %v", r.URL.Path, err)
			http.Error(w, "Internal Server Error (Template)", http.StatusInternalServerError)
			return
		}
	} else {
		finalBody = response.Body
	}

	// --- 4. Content Encoding (Gzip) ---
	bodyBytes := []byte(finalBody)
	if response.Gzip && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		if _, err := gz.Write(bodyBytes); err != nil {
			log.Printf("Error gzipping response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		_ = gz.Close()

		w.Header().Set("Content-Encoding", "gzip")
		bodyBytes = b.Bytes()
	}

	// Set Content-Length and Status
	w.Header().Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	w.WriteHeader(response.Status)

	// --- 5. Write Body ---
	if _, err := w.Write(bodyBytes); err != nil {
		log.Printf("Error writing response body: %v", err)
	}
}

// --- NEW Chaos/Stress Handlers ---

// handleCPUStress consumes CPU for a specified duration to simulate a system bottleneck.
func HandleCPUStress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	durationStr := vars["duration"]

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		http.Error(w, "Invalid duration format. Use e.g., /api/stress/cpu/10s", http.StatusBadRequest)
		return
	}

	// Use the number of logical cores for maximum effect
	numWorkers := runtime.NumCPU()
	var wg sync.WaitGroup

	log.Printf("Starting CPU stress for %s using %d workers", duration, numWorkers)

	observability.FaultsInjected.WithLabelValues("cpu_stress", r.URL.Path).Inc()

	// Stop time for the stress test
	stopTime := time.Now().Add(duration)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stopTime) {
				// Simple calculation to burn CPU
				_ = 1000 * 1000 / 1000 * 1000
			}
		}()
	}

	wg.Wait()
	log.Printf("Finished CPU stress after %s", duration)

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "CPU stressed for %s using %d cores. Now back to normal.", duration, numWorkers)
}

// handleMemoryStress allocates a specified amount of memory.
func HandleMemoryStress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sizeStr := vars["size"] // e.g., "100MB"

	size, err := parseMemorySize(sizeStr)
	if err != nil {
		http.Error(w, "Invalid memory size format. Use e.g., /api/stress/mem/100MB", http.StatusBadRequest)
		return
	}

	memoryBuffer := make([]byte, size)
	for i := range memoryBuffer {
		memoryBuffer[i] = byte(i % 256)
	}

	log.Printf("Allocated %s of memory.", sizeStr)
	observability.FaultsInjected.WithLabelValues("memory_stress", r.URL.Path).Inc()

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "Allocated %s of memory. May impact performance until garbage collected.", sizeStr)
}

func parseMemorySize(sizeStr string) (int, error) {
	sizeStr = strings.ToUpper(sizeStr)

	var multiplier int
	if strings.HasSuffix(sizeStr, "GB") {
		sizeStr = strings.TrimSuffix(sizeStr, "GB")
		multiplier = 1024 * 1024 * 1024
	} else if strings.HasSuffix(sizeStr, "MB") {
		sizeStr = strings.TrimSuffix(sizeStr, "MB")
		multiplier = 1024 * 1024
	} else if strings.HasSuffix(sizeStr, "KB") {
		sizeStr = strings.TrimSuffix(sizeStr, "KB")
		multiplier = 1024
	} else {
		return 0, fmt.Errorf("unknown size suffix")
	}

	val, err := strconv.Atoi(sizeStr)
	if err != nil {
		return 0, err
	}

	return val * multiplier, nil
}

// HandleEcho returns the request back to the client.
func HandleEcho(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetConfig()

	// --- Global Faults ---
	// 1. Global Delay
	if cfg.GlobalDelay > 0 {
		time.Sleep(cfg.GlobalDelay)
	}

	// 2. Global Chaos
	if cfg.GlobalChaosProbability > 0 && mrand.Float64() < cfg.GlobalChaosProbability {
		http.Error(w, "Global Chaos Injection", http.StatusInternalServerError)
		return
	}

	// 3. Header-Based Chaos (Client-Driven)
	// Delay: X-Echo-Delay (fixed) or X-Echo-Latency (range min-max)
	if delayStr := r.Header.Get("X-Echo-Delay"); delayStr != "" {
		if d, err := time.ParseDuration(delayStr); err == nil {
			time.Sleep(d)
		}
	} else if latencyStr := r.Header.Get("X-Echo-Latency"); latencyStr != "" {
		// Format: "100ms-500ms" or just "100ms"
		parts := strings.Split(latencyStr, "-")
		if len(parts) == 2 {
			min, err1 := time.ParseDuration(strings.TrimSpace(parts[0]))
			max, err2 := time.ParseDuration(strings.TrimSpace(parts[1]))
			if err1 == nil && err2 == nil && max > min {
				delta := max - min
				randomDelay := min + time.Duration(mrand.Int63n(int64(delta)))
				time.Sleep(randomDelay)
			}
		} else if len(parts) == 1 {
			if d, err := time.ParseDuration(parts[0]); err == nil {
				time.Sleep(d)
			}
		}
	}

	// Status Code
	statusCode := http.StatusOK
	if statusStr := r.Header.Get("X-Echo-Status"); statusStr != "" {
		if code, err := strconv.Atoi(statusStr); err == nil {
			statusCode = code
		}
	}

	// Response Headers
	// X-Echo-Headers: JSON map
	if headersStr := r.Header.Get("X-Echo-Headers"); headersStr != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersStr), &headers); err == nil {
			for k, v := range headers {
				w.Header().Set(k, v)
			}
		}
	}
	// X-Echo-Set-Header-Key: Value
	for k, v := range r.Header {
		if strings.HasPrefix(k, "X-Echo-Set-Header-") {
			headerName := strings.TrimPrefix(k, "X-Echo-Set-Header-")
			if len(v) > 0 {
				w.Header().Set(headerName, v[0])
			}
		}
	}

	// Body Generation
	var body []byte
	if bodyStr := r.Header.Get("X-Echo-Body"); bodyStr != "" {
		body = []byte(bodyStr)
	} else if sizeStr := r.Header.Get("X-Echo-Response-Size"); sizeStr != "" {
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil && size > 0 {
			// Limit max size to avoid OOM
			if size > 10*1024*1024 { // 10MB limit for safety
				size = 10 * 1024 * 1024
			}
			body = make([]byte, size)
			// Fill with 'A'
			for i := range body {
				body[i] = 'A'
			}
		}
	}

	// If custom body is generated, return it raw
	if len(body) > 0 {
		w.WriteHeader(statusCode)
		_, _ = w.Write(body)
		return
	}

	// Default Echo Behavior (JSON Dump)
	var bodyBuf bytes.Buffer
	if r.Body != nil {
		_, _ = io.Copy(&bodyBuf, r.Body)
		// Restore body for other readers if needed
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBuf.Bytes()))
	}

	response := struct {
		Timestamp  time.Time   `json:"timestamp"`
		Method     string      `json:"method"`
		Path       string      `json:"path"`
		Query      url.Values  `json:"query"`
		Headers    http.Header `json:"headers"`
		Body       string      `json:"body"`
		RemoteAddr string      `json:"remoteAddr"`
		Hostname   string      `json:"hostname"`
	}{
		Timestamp:  time.Now(),
		Method:     r.Method,
		Path:       r.URL.Path,
		Query:      r.URL.Query(),
		Headers:    r.Header,
		Body:       bodyBuf.String(),
		RemoteAddr: r.RemoteAddr,
		Hostname:   config.GetConfig().Hostname,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding echo response: %v", err)
	}
}

// handleWebsocket upgrades the connection and echoes messages.
func HandleWebsocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if err := conn.WriteMessage(messageType, p); err != nil {
			return
		}
	}
}

// handleSSE is a simple server-sent events handler
func HandleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			data := fmt.Sprintf("data: The time is %s\n\n", t.Format(time.RFC3339))
			if _, err := fmt.Fprintf(w, "%s", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// executeTemplate runs the response body string through the Go template engine.
func executeTemplate(tmplStr string, data TemplateData) (string, error) {
	tmpl, err := template.New("response").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.String(), nil
}
