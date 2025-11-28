package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/arun0009/go-resilience-mock/pkg/observability"
	"github.com/arun0009/go-resilience-mock/pkg/server"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Helper function to create a test server with a specific config/router
func newTestServer(cfg config.Config) *httptest.Server {
	router := server.NewRouter(cfg)
	return httptest.NewServer(router)
}

func TestRefactoredEcho(t *testing.T) {
	// Use default config, which is loaded on init()
	cfg := config.GetConfig()
	server := newTestServer(cfg)
	defer server.Close()

	resp, err := http.Get(server.URL + "/test/path?q=1")
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}
}

func TestScenarioDelayMetric(t *testing.T) {
	// Use injectScenario to ensure the scenario exists with expected values
	injectScenario("/api/test", "GET", config.Response{
		Status: 200,
		Delay:  500 * time.Millisecond,
	})

	cfg := config.GetConfig()
	server := newTestServer(cfg)
	defer server.Close()

	// Ensure metrics are clean for this test
	reg := config.GetRegistry()
	reg.Unregister(observability.FaultsInjected)
	reg.MustRegister(observability.FaultsInjected)
	observability.FaultsInjected.Reset()

	// The first response for /api/test is 200, 500ms delay
	path := "/api/test"
	req, _ := http.NewRequest("GET", server.URL+path, nil)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Scenario request failed: %v", err)
	}
	_ = resp.Body.Close()

	duration := time.Since(start)

	// Check delay
	if duration < 500*time.Millisecond {
		t.Errorf("Expected delay of 500ms, but request took only %s", duration)
	}

	// Check metric
	expectedMetric := `
	# HELP mock_faults_injected_total Total number of simulated faults injected, labeled by type (delay, http_error, cpu_stress).
	# TYPE mock_faults_injected_total counter
	mock_faults_injected_total{path="/api/test",type="delay"} 1
	`
	if err := testutil.CollectAndCompare(observability.FaultsInjected, strings.NewReader(expectedMetric)); err != nil {
		t.Errorf("Metrics mismatch after scenario run:\n%s", err)
	}
}

// Helper to manually inject scenarios since config.LoadConfig reads files
func injectScenario(path, method string, resp config.Response) {
	s := config.Scenario{
		Path:      path,
		Method:    method,
		Responses: []config.Response{resp},
	}
	// Note: In real app, LoadConfig handles this map. We insert manually for test.
	config.AddScenario(&s)
}

func TestPathVariables(t *testing.T) {
	// Define a scenario with a path variable
	injectScenario("/api/users/{id}", "GET", config.Response{
		Status: 200,
		Body:   `{"user": "found"}`,
	})

	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	// Request with a specific ID
	resp, err := http.Get(ts.URL + "/api/users/12345")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 OK, got %d. Path variables not matching.", resp.StatusCode)
	}
}

func TestDynamicTemplate(t *testing.T) {
	// Define a scenario that echoes the query param
	injectScenario("/api/search", "GET", config.Response{
		Status: 200,
		Body:   `{"query": "{{.Request.Query.q}}"}`,
	})

	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/search?q=golang")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "golang") {
		t.Errorf("Template did not render query param. Got: %s", string(body))
	}
}

func TestResetEndpoints(t *testing.T) {
	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	// 1. Generate some history
	_, _ = http.Get(ts.URL + "/echo")
	_, _ = http.Get(ts.URL + "/echo")

	// Verify history exists
	histResp, _ := http.Get(ts.URL + "/history")
	var hist []config.RequestRecord
	_ = json.NewDecoder(histResp.Body).Decode(&hist)
	if len(hist) < 2 {
		t.Fatalf("Setup failed: expected history, got %d items", len(hist))
	}

	// 2. Call Reset
	req, _ := http.NewRequest("POST", ts.URL+"/api/control/reset-history", nil)
	_, _ = http.DefaultClient.Do(req)

	// 3. Verify Empty (should contain 1 item: the reset request itself, as middleware logs it)
	histResp, _ = http.Get(ts.URL + "/history")
	_ = json.NewDecoder(histResp.Body).Decode(&hist)
	if len(hist) != 1 {
		t.Errorf("History reset failed. Expected 1 item (reset request), got %d items", len(hist))
	}
}

func TestCPUStressMetric(t *testing.T) {
	reg := config.GetRegistry()
	reg.Unregister(observability.FaultsInjected)
	reg.MustRegister(observability.FaultsInjected)
	observability.FaultsInjected.Reset()

	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/stress/cpu/10ms")
	_ = resp.Body.Close()

	expectedMetric := `
	# HELP mock_faults_injected_total Total number of simulated faults injected, labeled by type (delay, http_error, cpu_stress).
	# TYPE mock_faults_injected_total counter
	mock_faults_injected_total{path="/api/stress/cpu/10ms",type="cpu_stress"} 1
	`
	if err := testutil.CollectAndCompare(observability.FaultsInjected, strings.NewReader(expectedMetric)); err != nil {
		t.Errorf("Metrics mismatch:\n%s", err)
	}
}

func TestWebsocketEcho(t *testing.T) {
	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	// Convert http URL to ws URL
	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send message
	msg := []byte("hello")
	if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatalf("%v", err)
	}

	// Receive message
	_, p, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("%v", err)
	}

	if string(p) != string(msg) {
		t.Errorf("Expected %s, got %s", msg, p)
	}
}

func TestSSE(t *testing.T) {
	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read SSE stream: %v", err)
	}

	if !strings.HasPrefix(line, "data:") {
		t.Errorf("Expected SSE data prefix, got: %s", line)
	}
}

func TestMemoryStress(t *testing.T) {
	reg := config.GetRegistry()
	reg.Unregister(observability.FaultsInjected)
	reg.MustRegister(observability.FaultsInjected)
	observability.FaultsInjected.Reset()

	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/stress/mem/1MB")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}

	expectedMetric := `
	# HELP mock_faults_injected_total Total number of simulated faults injected, labeled by type (delay, http_error, cpu_stress).
	# TYPE mock_faults_injected_total counter
	mock_faults_injected_total{path="/api/stress/mem/1MB",type="memory_stress"} 1
	`
	if err := testutil.CollectAndCompare(observability.FaultsInjected, strings.NewReader(expectedMetric)); err != nil {
		t.Errorf("Metrics mismatch:\n%s", err)
	}
}

func TestHeaderChaos(t *testing.T) {
	// Setup server
	cfg := config.GetConfig()
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Test Status
	req, _ := http.NewRequest("GET", ts.URL+"/echo", nil)
	req.Header.Set("X-Echo-Status", "418")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	if resp.StatusCode != 418 {
		t.Errorf("Expected status 418, got %d", resp.StatusCode)
	}

	// Test Delay (small delay)
	req, _ = http.NewRequest("GET", ts.URL+"/echo", nil)
	req.Header.Set("X-Echo-Delay", "50ms")
	start := time.Now()
	resp, err = http.DefaultClient.Do(req)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	duration := time.Since(start)
	if duration < 50*time.Millisecond {
		t.Errorf("Expected delay >= 50ms, got %v", duration)
	}
}

func TestDynamicScenarios(t *testing.T) {
	// Setup server
	cfg := config.GetConfig()
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Add Scenario
	scenario := []config.Scenario{
		{
			Path:   "/api/dynamic",
			Method: "GET",
			Responses: []config.Response{
				{Status: 201, Body: "created"},
			},
		},
	}
	body, _ := json.Marshal(scenario)
	resp, err := http.Post(ts.URL+"/scenario", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to add scenario: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}

	// Verify Scenario
	resp, err = http.Get(ts.URL + "/api/dynamic")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 201 {
		t.Errorf("Expected 201 Created, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if string(bodyBytes) != "created" {
		t.Errorf("Expected body 'created', got '%s'", string(bodyBytes))
	}
}

func TestGlobalConfig(t *testing.T) {
	// Set env vars
	t.Setenv("ECHO_DELAY", "50ms")
	t.Setenv("RATE_LIMIT_RPS", "100")
	t.Setenv("HISTORY_SIZE", "50")

	// Load config (LoadConfig reads env vars)
	// Note: LoadConfig uses a lock and global state, which might interfere with other tests if run in parallel.
	// But our tests are sequential here.
	// We need to point to a dummy file to avoid loading real scenarios
	cfg, _ := config.LoadConfig("non_existent.yaml")

	if cfg.GlobalDelay != 50*time.Millisecond {
		t.Errorf("Expected GlobalDelay 50ms, got %v", cfg.GlobalDelay)
	}
	if cfg.RateLimitPerS != 100.0 {
		t.Errorf("Expected RateLimitPerS 100, got %f", cfg.RateLimitPerS)
	}
	if cfg.HistorySize != 50 {
		t.Errorf("Expected HistorySize 50, got %d", cfg.HistorySize)
	}

	// Verify Global Delay Effect
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	start := time.Now()
	_, _ = http.Get(ts.URL + "/echo")
	duration := time.Since(start)

	if duration < 50*time.Millisecond {
		t.Errorf("Expected global delay >= 50ms, got %v", duration)
	}
}

func TestDocsEndpoint(t *testing.T) {
	cfg := config.GetConfig()
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Verify /docs/index.html is served
	resp, err := http.Get(ts.URL + "/docs/index.html")
	if err != nil {
		t.Fatalf("Failed to request docs: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 OK for docs, got %d", resp.StatusCode)
	}

	// Verify content type (should be text/html)
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Expected Content-Type text/html, got %s", ct)
	}

	// Verify streaming.md is accessible
	resp, err = http.Get(ts.URL + "/docs/streaming.md")
	if err != nil {
		t.Fatalf("Failed to request streaming.md: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 OK for streaming.md, got %d", resp.StatusCode)
	}
}

func TestAdvancedFeatures(t *testing.T) {
	cfg := config.GetConfig()
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 1. Test /info
	resp, err := http.Get(ts.URL + "/info")
	if err != nil {
		t.Fatalf("Failed to call /info: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 OK for /info, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 2. Test Web UI Routes
	for _, path := range []string{"/web-ws", "/web-sse"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("Failed to call %s: %v", path, err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("Expected 200 OK for %s, got %d", path, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}

	// 3. Test Advanced Chaos Headers
	client := &http.Client{}
	req, _ := http.NewRequest("GET", ts.URL+"/echo", nil)
	req.Header.Set("X-Echo-Set-Header-Custom-Key", "CustomValue")
	req.Header.Set("X-Echo-Response-Size", "10") // 10 bytes

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to call /echo: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check Custom Header
	if val := resp.Header.Get("Custom-Key"); val != "CustomValue" {
		t.Errorf("Expected Custom-Key header 'CustomValue', got '%s'", val)
	}

	// Check Body Size
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 10 {
		t.Errorf("Expected body size 10, got %d", len(body))
	}
}
