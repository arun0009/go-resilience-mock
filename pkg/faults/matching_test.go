package faults

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/gorilla/mux"
)

func TestMatchingRules(t *testing.T) {
	// Setup Scenarios
	scenarios := []*config.Scenario{
		{
			Path:   "/match",
			Method: "POST",
			Matches: config.MatchConfig{
				Headers: map[string]string{"X-Test": "A"},
			},
			Responses: []config.Response{{Status: 201, Body: "Matched Header"}},
		},
		{
			Path:   "/match",
			Method: "POST",
			Matches: config.MatchConfig{
				Query: map[string]string{"type": "B"},
			},
			Responses: []config.Response{{Status: 202, Body: "Matched Query"}},
		},
		{
			Path:   "/match",
			Method: "POST",
			Matches: config.MatchConfig{
				Body: "regex:^START.*END$",
			},
			Responses: []config.Response{{Status: 203, Body: "Matched Body Regex"}},
		},
		{
			Path:   "/match",
			Method: "POST",
			// No matches = default fallback for this path/method if others fail?
			// Current logic finds FIRST match. If we add this last, it acts as fallback.
			Responses: []config.Response{{Status: 200, Body: "Fallback"}},
		},
	}

	for _, s := range scenarios {
		config.AddScenario(s)
	}

	r := mux.NewRouter()
	r.HandleFunc("/match", HandleScenario).Methods("POST")

	// Test 1: Header Match
	req1, _ := http.NewRequest("POST", "/match", nil)
	req1.Header.Set("X-Test", "A")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != 201 {
		t.Errorf("Expected 201 (Header Match), got %d", w1.Code)
	}

	// Test 2: Query Match
	req2, _ := http.NewRequest("POST", "/match?type=B", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 202 {
		t.Errorf("Expected 202 (Query Match), got %d", w2.Code)
	}

	// Test 3: Body Regex Match
	req3, _ := http.NewRequest("POST", "/match", strings.NewReader("START-content-END"))
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != 203 {
		t.Errorf("Expected 203 (Body Match), got %d", w3.Code)
	}

	// Test 4: Fallback
	req4, _ := http.NewRequest("POST", "/match", nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	if w4.Code != 200 {
		t.Errorf("Expected 200 (Fallback), got %d", w4.Code)
	}
}
