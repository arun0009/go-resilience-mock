package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arun0009/go-resilience-mock/pkg/config"
)

func TestRequestIDMiddleware(t *testing.T) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Context().Value(requestIDKey)
		if id == nil {
			t.Error("Request ID not found in context")
		}
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

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK for OPTIONS, got %d", w.Result().StatusCode)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Expected Access-Control-Allow-Origin: *")
	}
}
