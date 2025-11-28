package faults

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/gorilla/mux"
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
			{Status: 500, Body: "fail"},
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
	if w1.Code != 500 {
		t.Errorf("Expected 500, got %d", w1.Code)
	}
	if scenario.CBState.Failures != 1 {
		t.Errorf("Expected 1 failure, got %d", scenario.CBState.Failures)
	}

	// 2. Second Failure (Trip)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != 500 {
		t.Errorf("Expected 500, got %d", w2.Code)
	}
	if scenario.CBState.State != "open" {
		t.Errorf("Expected state 'open', got '%s'", scenario.CBState.State)
	}

	// 3. Third Request (Blocked)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req)
	if w3.Code != 503 {
		t.Errorf("Expected 503 (Open), got %d", w3.Code)
	}

	// 4. Wait for Timeout (Half-Open)
	time.Sleep(150 * time.Millisecond)

	// Change response to success to test recovery
	scenario.Responses[0].Status = 200

	// 5. Fourth Request (Half-Open -> Closed)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req)
	if w4.Code != 200 {
		t.Errorf("Expected 200, got %d", w4.Code)
	}
	if scenario.CBState.State != "closed" {
		t.Errorf("Expected state 'closed', got '%s'", scenario.CBState.State)
	}
}
