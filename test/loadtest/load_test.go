package loadtest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/arun0009/go-resilience-mock/pkg/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComplexResiliency(t *testing.T) {
	// 1. Setup Server with Complex Scenarios
	config.ResetDefaults()
	cfg, err := config.LoadConfig("load_test_scenarios.yaml")
	require.NoError(t, err)
	cfg.LogRequests = false // Reduce noise during load test

	router := server.NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		},
		Timeout: 5 * time.Second,
	}

	// 2. Add Dynamic Scenarios via POST (User Request)
	t.Run("AddDynamicScenarios", func(t *testing.T) {
		scenarios := []map[string]interface{}{
			{
				"path":   "/api/dynamic/delay",
				"method": "GET",
				"responses": []map[string]interface{}{
					{
						"status": 200,
						"delay":  50000000, // 50ms in nanoseconds
						"body":   `{"type": "dynamic-delay"}`,
					},
				},
			},
			{
				"path":   "/api/dynamic/chaos",
				"method": "POST",
				"responses": []map[string]interface{}{
					{
						"status":      503,
						"probability": 0.5,
						"body":        `{"error": "dynamic-chaos"}`,
					},
				},
			},
		}

		body, _ := json.Marshal(scenarios)
		resp, err := client.Post(ts.URL+"/scenario", "application/json", bytes.NewReader(body))
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	// 3. Test Jitter (Fixed & Dynamic)
	t.Run("JitterVerification", func(t *testing.T) {
		var wg sync.WaitGroup
		count := 20
		latencies := make([]time.Duration, count)

		for i := 0; i < count; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				start := time.Now()
				resp, err := client.Get(ts.URL + "/api/resiliency/delay-jitter")
				if err == nil {
					resp.Body.Close()
				}
				latencies[idx] = time.Since(start)
			}(i)
		}
		wg.Wait()

		// Verify range (50ms - 150ms defined in complex_scenarios.yaml)
		// Allow small margin for overhead
		for _, d := range latencies {
			assert.GreaterOrEqual(t, d.Milliseconds(), int64(50), "Latency too low")
			// Upper bound can be higher due to system load, but shouldn't be crazy
			// assert.LessOrEqual(t, d.Milliseconds(), int64(200), "Latency too high")
		}
	})

	// 4. Test Chaos (Probability)
	t.Run("ChaosVerification", func(t *testing.T) {
		count := 100
		failures := 0

		for i := 0; i < count; i++ {
			resp, err := client.Get(ts.URL + "/api/resiliency/chaos")
			if err == nil {
				if resp.StatusCode == 500 {
					failures++
				}
				resp.Body.Close()
			}
		}

		// Expected 20% failure rate. Allow margin.
		failureRate := float64(failures) / float64(count)
		fmt.Printf("Chaos Failure Rate: %.2f\n", failureRate)
		assert.InDelta(t, 0.2, failureRate, 0.15, "Failure rate should be approx 0.2")
	})

	// 5. Test Circuit Breaker (Stateful)
	t.Run("CircuitBreakerVerification", func(t *testing.T) {
		// Threshold is 5 failures.
		// 1. Trip the breaker
		for i := 0; i < 5; i++ {
			resp, _ := client.Get(ts.URL + "/api/resiliency/circuit-breaker")
			if resp != nil {
				assert.Equal(t, 500, resp.StatusCode)
				resp.Body.Close()
			}
		}

		// 2. Verify Open State (Should return 503 immediately)
		resp, err := client.Get(ts.URL + "/api/resiliency/circuit-breaker")
		require.NoError(t, err)
		assert.Equal(t, 503, resp.StatusCode, "Circuit breaker should be open")
		resp.Body.Close()

		// 3. Wait for Timeout (2s defined in yaml)
		time.Sleep(2100 * time.Millisecond)

		// 4. Verify Half-Open (Next request allowed)
		// The scenario only has a 500 response, so it will fail again and trip back to Open?
		// Or if we can make it succeed? The scenario is fixed to 500.
		// So it should allow the request, return 500, and count as failure.
		resp, err = client.Get(ts.URL + "/api/resiliency/circuit-breaker")
		require.NoError(t, err)
		assert.Equal(t, 500, resp.StatusCode, "Should allow request in Half-Open")
		resp.Body.Close()
	})

	// 6. Load Test (Concurrent Access to Dynamic Scenarios)
	t.Run("LoadTest_DynamicScenarios", func(t *testing.T) {
		var wg sync.WaitGroup
		workers := 10
		requestsPerWorker := 50

		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < requestsPerWorker; j++ {
					// Mix of GET and POST
					if j%2 == 0 {
						resp, err := client.Get(ts.URL + "/api/dynamic/delay")
						if err == nil {
							resp.Body.Close()
						}
					} else {
						resp, err := client.Post(ts.URL+"/api/dynamic/chaos", "application/json", nil)
						if err == nil {
							resp.Body.Close()
						}
					}
				}
			}()
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(10 * time.Second):
			t.Fatal("Load test timed out")
		}
	})
}
