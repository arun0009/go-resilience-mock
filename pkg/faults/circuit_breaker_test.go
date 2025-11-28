package faults

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker(t *testing.T) {
	// Setup
	scenario := &config.Scenario{
		Path:   "/test-cb",
		Method: "GET",
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold: 2,
			SuccessThreshold: 1,
			Timeout:          100 * time.Millisecond,
		},
		CBState: &config.CircuitBreakerState{
			State: "closed",
		},
		Responses: []config.Response{
			{Status: 500, Body: config.JSONBody(`"fail"`)},
		},
	}
	config.AddScenario(scenario)

	// Use Router to ensure mux vars are set
	r := mux.NewRouter()
	r.HandleFunc("/test-cb", HandleScenario).Methods("GET")

	req, _ := http.NewRequest("GET", "/test-cb", nil)

	// 1. First Failure
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req)
	assert.Equal(t, 500, w1.Code, "Expected 500")
	assert.Equal(t, 1, scenario.CBState.Failures, "Expected 1 failure")

	// 2. Second Failure (Trip)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	assert.Equal(t, 500, w2.Code, "Expected 500")
	assert.Equal(t, "open", scenario.CBState.State, "Expected state 'open'")

	// 3. Third Request (Blocked)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req)
	assert.Equal(t, 503, w3.Code, "Expected 503 (Open)")

	// 4. Wait for Timeout (Half-Open)
	time.Sleep(150 * time.Millisecond)

	// Change response to success to test recovery
	scenario.Responses[0].Status = 200

	// 5. Fourth Request (Half-Open -> Closed)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req)
	assert.Equal(t, 200, w4.Code, "Expected 200")
	assert.Equal(t, "closed", scenario.CBState.State, "Expected state 'closed'")
}
