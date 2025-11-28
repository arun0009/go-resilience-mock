package faults

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/arun0009/go-resilience-mock/pkg/config"
)

// matchesRequest checks if the request matches the scenario's rules
func matchesRequest(s *config.Scenario, r *http.Request) bool {
	// 1. Headers
	for k, v := range s.Matches.Headers {
		if r.Header.Get(k) != v {
			return false
		}
	}

	// 2. Query Params
	query := r.URL.Query()
	for k, v := range s.Matches.Query {
		if query.Get(k) != v {
			return false
		}
	}

	// 3. Body (Regex) - Only if body matching is configured
	if s.Matches.Body != "" {
		if r.Body == nil {
			return false
		}
		// Read body (and restore it)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return false
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// TODO: Compile regex once during config load for performance
		// For now, simple string contains or regex
		// If it starts with regex:, treat as regex
		if strings.HasPrefix(s.Matches.Body, "regex:") {
			pattern := strings.TrimPrefix(s.Matches.Body, "regex:")
			matched, _ := regexp.MatchString(pattern, string(bodyBytes))
			if !matched {
				return false
			}
		} else {
			// Exact match or contains? Let's do exact match for simplicity, or contains?
			// Let's do contains for flexibility
			if !strings.Contains(string(bodyBytes), s.Matches.Body) {
				return false
			}
		}
	}

	return true
}
