package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/arun0009/go-resilience-mock/pkg/observability"
	"github.com/arun0009/go-resilience-mock/pkg/server"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test server with a specific config/router
func newTestServer(cfg config.Config) *httptest.Server {
	router := server.NewRouter(cfg)
	return httptest.NewServer(router)
}

func TestEcho(t *testing.T) {
	// Use default config, which is loaded on init()
	cfg := config.GetConfig()
	server := newTestServer(cfg)
	defer server.Close()

	resp, err := http.Get(server.URL + "/echo?q=1")
	require.NoError(t, err, "HTTP request failed")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status OK")
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
	require.NoError(t, err, "Scenario request failed")
	_ = resp.Body.Close()

	duration := time.Since(start)

	// Check delay
	assert.GreaterOrEqual(t, duration, 500*time.Millisecond, "Expected delay of at least 500ms")

	// Check metric
	expectedMetric := `
	# HELP mock_faults_injected_total Total number of simulated faults injected, labeled by type (delay, http_error, cpu_stress).
	# TYPE mock_faults_injected_total counter
	mock_faults_injected_total{path="/api/test",type="delay"} 1
	`
	err = testutil.CollectAndCompare(observability.FaultsInjected, strings.NewReader(expectedMetric))
	assert.NoError(t, err, "Metrics mismatch after scenario run")
}

// Helper to manually inject scenarios since config.LoadConfig reads files
func injectScenario(path, method string, resp config.Response) {
	// Ensure the body is properly formatted as JSON
	if len(resp.Body) == 0 && resp.Status != 0 {
		// If body is empty but status is set, use an empty JSON object
		resp.Body = config.JSONBody("{}")
	}

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
		Body:   config.JSONBody(`{"user": "found"}`),
	})

	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	// Request with a specific ID
	resp, err := http.Get(ts.URL + "/api/users/12345")
	require.NoError(t, err, "Failed to make request")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, 200, resp.StatusCode, "Path variables not matching")

	body, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, `{"user": "found"}`, string(body), "Expected JSON body match")
}

func TestDynamicTemplate(t *testing.T) {
	// Define a scenario that echoes the query param
	injectScenario("/api/search", "GET", config.Response{
		Status: 200,
		Body:   config.JSONBody(`{"query": "{{.Request.Query.q}}"}`),
	})

	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/search?q=golang")
	require.NoError(t, err, "Failed to make request")
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, `{"query": "golang"}`, string(body), "Template did not render query param correctly")
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
	require.GreaterOrEqual(t, len(hist), 2, "Setup failed: expected history")

	// 2. Call Reset
	req, _ := http.NewRequest("POST", ts.URL+"/api/control/reset-history", nil)
	_, _ = http.DefaultClient.Do(req)

	// 3. Verify Empty (should contain 1 item: the reset request itself, as middleware logs it)
	histResp, _ = http.Get(ts.URL + "/history")
	_ = json.NewDecoder(histResp.Body).Decode(&hist)
	assert.Len(t, hist, 1, "History reset failed. Expected 1 item (reset request)")
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
	err := testutil.CollectAndCompare(observability.FaultsInjected, strings.NewReader(expectedMetric))
	assert.NoError(t, err, "Metrics mismatch")
}

func TestWebsocketEcho(t *testing.T) {
	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	// Convert http URL to ws URL
	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	// Send message
	msg := []byte("hello")
	err = ws.WriteMessage(websocket.TextMessage, msg)
	require.NoError(t, err)

	// Receive message
	_, p, err := ws.ReadMessage()
	require.NoError(t, err)

	assert.Equal(t, string(msg), string(p), "Expected echoed message")
}

func TestSSE(t *testing.T) {
	cfg := config.GetConfig()
	ts := newTestServer(cfg)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/sse")
	require.NoError(t, err, "Failed to make request")
	defer func() { _ = resp.Body.Close() }()

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err, "Failed to read SSE stream")

	assert.True(t, strings.HasPrefix(line, "data:"), "Expected SSE data prefix, got: %s", line)
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
	require.NoError(t, err, "Failed to make request")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status OK")

	expectedMetric := `
	# HELP mock_faults_injected_total Total number of simulated faults injected, labeled by type (delay, http_error, cpu_stress).
	# TYPE mock_faults_injected_total counter
	mock_faults_injected_total{path="/api/stress/mem/1MB",type="memory_stress"} 1
	`
	err = testutil.CollectAndCompare(observability.FaultsInjected, strings.NewReader(expectedMetric))
	assert.NoError(t, err, "Metrics mismatch")
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
	require.NoError(t, err, "Failed to send request")
	assert.Equal(t, 418, resp.StatusCode, "Expected status 418")

	// Test Delay (small delay)
	req, _ = http.NewRequest("GET", ts.URL+"/echo", nil)
	req.Header.Set("X-Echo-Delay", "50ms")
	start := time.Now()
	resp, err = http.DefaultClient.Do(req)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	require.NoError(t, err, "Failed to send request")
	duration := time.Since(start)
	assert.GreaterOrEqual(t, duration, 50*time.Millisecond, "Expected delay >= 50ms")
}

func TestDynamicScenarios(t *testing.T) {
	// Setup server
	cfg := config.GetConfig()
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Clear any existing scenarios
	scenarios := config.GetScenarios()
	scenarios.Range(func(key, value interface{}) bool {
		scenarios.Delete(key)
		return true
	})

	// Add Scenario
	scenario := config.Scenario{
		Path:   "/api/dynamic",
		Method: "GET",
		Responses: []config.Response{
			{
				Status: 201,
				Body:   config.JSONBody(`"created"`),
			},
		},
	}

	// Add the scenario directly to ensure it's properly registered
	config.AddScenario(&scenario)

	// Also try adding via the API to test that path
	body, _ := json.Marshal([]config.Scenario{scenario})
	resp, err := http.Post(ts.URL+"/scenario", "application/json", bytes.NewReader(body))
	require.NoError(t, err, "Failed to add scenario")
	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200 OK, got %d. Response: %s", resp.StatusCode, string(bodyBytes))
	}

	// Verify Scenario
	req, err := http.NewRequest("GET", ts.URL+"/api/dynamic", nil)
	require.NoError(t, err, "Failed to create request")

	client := &http.Client{}
	resp, err = client.Do(req)
	require.NoError(t, err, "Failed to make request")
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 201 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 201 Created, got %d. Response: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")

	assert.JSONEq(t, `"created"`, string(bodyBytes), "Expected body 'created'")
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

	assert.Equal(t, 50*time.Millisecond, cfg.GlobalDelay, "Expected GlobalDelay 50ms")
	assert.Equal(t, 100.0, cfg.RateLimitPerS, "Expected RateLimitPerS 100")
	assert.Equal(t, 50, cfg.HistorySize, "Expected HistorySize 50")

	// Verify Global Delay Effect
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	start := time.Now()
	_, _ = http.Get(ts.URL + "/echo")
	duration := time.Since(start)

	assert.GreaterOrEqual(t, duration, 50*time.Millisecond, "Expected global delay >= 50ms")
}

func TestDocsEndpoint(t *testing.T) {
	cfg := config.GetConfig()
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Verify /docs/index.html is served
	resp, err := http.Get(ts.URL + "/docs/index.html")
	require.NoError(t, err, "Failed to request docs")
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, 200, resp.StatusCode, "Expected 200 OK for docs")

	// Verify content type (should be text/html)
	ct := resp.Header.Get("Content-Type")
	assert.Contains(t, ct, "text/html", "Expected Content-Type text/html")

	// Verify streaming.md is accessible
	resp, err = http.Get(ts.URL + "/docs/streaming.md")
	require.NoError(t, err, "Failed to request streaming.md")
	_ = resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode, "Expected 200 OK for streaming.md")
}

func TestAdvancedFeatures(t *testing.T) {
	cfg := config.GetConfig()
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 1. Test /health
	resp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err, "Failed to call /health")
	assert.Equal(t, 200, resp.StatusCode, "Expected 200 OK for /health")
	_ = resp.Body.Close()

	// 2. Test Web UI Routes
	for _, path := range []string{"/web-ws", "/web-sse"} {
		resp, err := http.Get(ts.URL + path)
		require.NoError(t, err, "Failed to call %s", path)
		assert.Equal(t, 200, resp.StatusCode, "Expected 200 OK for %s", path)
		_ = resp.Body.Close()
	}

	// 3. Test Advanced Chaos Headers
	client := &http.Client{}
	req, _ := http.NewRequest("GET", ts.URL+"/echo", nil)
	req.Header.Set("X-Echo-Set-Header-Custom-Key", "CustomValue")
	req.Header.Set("X-Echo-Response-Size", "10") // 10 bytes

	resp, err = client.Do(req)
	require.NoError(t, err, "Failed to call /echo")
	defer func() { _ = resp.Body.Close() }()

	// Check Custom Header
	assert.Equal(t, "CustomValue", resp.Header.Get("Custom-Key"), "Expected Custom-Key header")

	// Check Body Size
	body, _ := io.ReadAll(resp.Body)
	assert.Len(t, body, 10, "Expected body size 10")
}

func TestConcurrency_RaceConditions(t *testing.T) {
	config.ResetDefaults()
	// Setup server
	cfg := config.GetConfig()
	cfg.HistorySize = 100
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Add a scenario
	scenario := config.Scenario{
		Path:   "/api/concurrent",
		Method: "GET",
		Responses: []config.Response{
			{Status: 200, Body: config.JSONBody(`{"status": "ok"}`)},
		},
	}
	config.AddScenario(&scenario)

	var wg sync.WaitGroup
	concurrency := 50
	iterations := 20

	// 1. Concurrent Requests to Scenario
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			client := &http.Client{Timeout: 2 * time.Second}
			for j := 0; j < iterations; j++ {
				resp, err := client.Get(ts.URL + "/api/concurrent")
				if err == nil {
					_ = resp.Body.Close()
				}
			}
		}(i)
	}

	// 2. Concurrent Requests to History (Read)
	wg.Add(concurrency / 2)
	for i := 0; i < concurrency/2; i++ {
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 2 * time.Second}
			for j := 0; j < iterations; j++ {
				resp, err := client.Get(ts.URL + "/history")
				if err == nil {
					_ = resp.Body.Close()
				}
			}
		}()
	}

	// 3. Concurrent Scenario Updates (Write)
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func(id int) {
			defer wg.Done()
			client := &http.Client{Timeout: 2 * time.Second}
			for j := 0; j < iterations; j++ {
				// Add a new scenario dynamically
				s := config.Scenario{
					Path:   fmt.Sprintf("/api/dynamic-%d-%d", id, j),
					Method: "GET",
					Responses: []config.Response{
						{Status: 200, Body: config.JSONBody(`"ok"`)},
					},
				}
				body, _ := json.Marshal([]config.Scenario{s})
				resp, err := client.Post(ts.URL+"/scenario", "application/json", bytes.NewReader(body))
				if err == nil {
					_ = resp.Body.Close()
				}
			}
		}(i)
	}

	wg.Wait()

	// Assert server is still alive
	resp, err := http.Get(ts.URL + "/health")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
}

func TestVerification_EchoAndHistory(t *testing.T) {
	config.ResetDefaults()
	cfg := config.GetConfig()
	cfg.LogBody = true
	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 1. Echo Exactness
	t.Run("EchoExactness", func(t *testing.T) {
		// Case A: JSON Re-formatting
		// json.NewEncoder adds a newline and sorts keys. The user might expect exact byte match.
		reqBody := `{"b": 2, "a": 1}` // No newline, specific order
		req, _ := http.NewRequest("POST", ts.URL+"/echo", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Custom-Header", "custom-value")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(resp.Body)

		// Use JSONEq as requested to ensure semantic equality without being overly strict on bytes
		assert.JSONEq(t, reqBody, string(bodyBytes), "Echo body should match JSON semantically")

		// Case B: Header Echoing
		// Docs say "Echoes back ... headers".
		// Check if X-Custom-Header is present in response
		echoedHeader := resp.Header.Get("X-Custom-Header")
		assert.Equal(t, "custom-value", echoedHeader, "Echo should return request headers")
	})

	// 2. History ID
	t.Run("HistoryID", func(t *testing.T) {
		// Make a request
		_, _ = http.Get(ts.URL + "/echo")

		// Check history
		resp, err := http.Get(ts.URL + "/history")
		require.NoError(t, err)
		defer resp.Body.Close()

		var history []map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&history)
		require.NoError(t, err)

		require.NotEmpty(t, history)
		lastEntry := history[len(history)-1]

		id, ok := lastEntry["id"]
		assert.True(t, ok, "ID field missing")
		assert.NotEqual(t, "unknown", id, "ID should not be unknown")
		assert.NotEqual(t, "", id, "ID should not be empty")
		assert.NotEqual(t, "0", id, "ID should not be 0 (unless it's the very first one, but we expect increment)")
	})

	// 3. JSON Escaping in History
	t.Run("HistoryJSONEscaping", func(t *testing.T) {
		reqBody := `{"key": "value"}`
		req, _ := http.NewRequest("POST", ts.URL+"/echo", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		_, _ = http.DefaultClient.Do(req)

		resp, err := http.Get(ts.URL + "/history")
		require.NoError(t, err)
		defer resp.Body.Close()

		var history []map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&history)
		lastEntry := history[len(history)-1]

		body := lastEntry["body"]
		// Should be a map, not a string
		assert.IsType(t, map[string]interface{}{}, body, "History body should be a JSON object, not string")
	})
}
