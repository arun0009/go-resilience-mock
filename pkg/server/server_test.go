package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestIDMiddleware(t *testing.T) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Context().Value(RequestIDKey)
		assert.NotNil(t, id, "Request ID not found in context")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
}

func TestCorsMiddleware(t *testing.T) {
	// Ensure CORS is enabled in config
	cfg := config.GetConfig()
	cfg.EnableCORS = true
	// Note: In a real scenario we might need to inject config, but here we rely on global state which is tricky.
	// For this unit test, we assume default config or current state allows CORS.

	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("OPTIONS", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode, "Expected 200 OK for OPTIONS")
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"), "Expected Access-Control-Allow-Origin: *")
}

func TestDynamicPathParameters_Reproduction(t *testing.T) {
	// Setup server
	cfg := config.GetConfig()
	router := NewRouter(cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 1. Add a dynamic scenario with a path parameter via API
	// The server should register this.
	scenario := config.Scenario{
		Path:   "/api/items/{id}",
		Method: "GET",
		Responses: []config.Response{
			{
				Status: 200,
				Body:   config.JSONBody(`{"item_id": "{{.Request.PathVars.id}}"}`),
			},
		},
	}

	body, _ := json.Marshal([]config.Scenario{scenario})
	resp, err := http.Post(ts.URL+"/scenario", "application/json", bytes.NewReader(body))
	require.NoError(t, err, "Failed to add scenario")
	require.Equal(t, 200, resp.StatusCode, "Failed to add scenario")

	// 2. Request a matching path
	// This relies on the catch-all handler finding the scenario.
	// Suspected bug: The catch-all handler looks up by exact path key, so "/api/items/123" won't match "/api/items/{id}"
	req, err := http.NewRequest("GET", ts.URL+"/api/items/123", nil)
	require.NoError(t, err)

	client := &http.Client{}
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 3. Assert
	assert.Equal(t, 200, resp.StatusCode, "Expected 200 OK for dynamic path match")
}
