package faults

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
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
			Responses: []config.Response{{Status: 201, Body: config.JSONBody(`"Matched Header"`)}},
		},
		{
			Path:   "/match",
			Method: "POST",
			Matches: config.MatchConfig{
				Query: map[string]string{"type": "B"},
			},
			Responses: []config.Response{{Status: 202, Body: config.JSONBody(`"Matched Query"`)}},
		},
		{
			Path:   "/match",
			Method: "POST",
			Matches: config.MatchConfig{
				Body: config.JSONBody(`"/^START.*END$/"`), // Use forward slashes to indicate regex pattern
			},
			Responses: []config.Response{{Status: 203, Body: config.JSONBody(`"Matched Body Regex"`)}},
		},
		{
			Path:   "/match",
			Method: "POST",
			// No matches = default fallback for this path/method if others fail?
			// Current logic finds FIRST match. If we add this last, it acts as fallback.
			Responses: []config.Response{{Status: 200, Body: config.JSONBody(`"Fallback"`)}},
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
	assert.Equal(t, 201, w1.Code, "Expected 201 (Header Match)")

	// Test 2: Query Match
	req2, _ := http.NewRequest("POST", "/match?type=B", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, 202, w2.Code, "Expected 202 (Query Match)")

	// Test 3: Body Regex Match
	req3, _ := http.NewRequest("POST", "/match", strings.NewReader("START-content-END"))
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	assert.Equal(t, 203, w3.Code, "Expected 203 (Body Match)")

	// Test 4: Fallback
	req4, _ := http.NewRequest("POST", "/match", nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	assert.Equal(t, 200, w4.Code, "Expected 200 (Fallback)")
}
