package health

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

type Checker func() (string, error)

type Health struct {
	mu        sync.RWMutex
	checkers  map[string]Checker
	startedAt time.Time
}

func NewHealth() *Health {
	return &Health{
		checkers:  make(map[string]Checker),
		startedAt: time.Now(),
	}
}

func (h *Health) AddCheck(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = checker
}

func (h *Health) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checks := make(map[string]string)
		status := http.StatusOK

		h.mu.RLock()
		defer h.mu.RUnlock()

		for name, check := range h.checkers {
			if message, err := check(); err != nil {
				status = http.StatusServiceUnavailable
				checks[name] = "error: " + err.Error()
			} else {
				checks[name] = message
			}
		}

		info := map[string]interface{}{
			"status":    http.StatusText(status),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"uptime":    time.Since(h.startedAt).String(),
			"checks":    checks,
			"system": map[string]interface{}{
				"goroutines": runtime.NumGoroutine(),
				"version":    runtime.Version(),
				"os":         runtime.GOOS,
				"arch":       runtime.GOARCH,
				"hostname":   getHostname(),
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(info)
	})
}

func getHostname() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "unknown"
}
