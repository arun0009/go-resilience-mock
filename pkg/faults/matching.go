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
	if len(s.Matches.Body) > 0 {
		if r.Body == nil {
			return false
		}
		// Read body (and restore it)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return false
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Convert the expected body to string for comparison
		expectedBody := string(s.Matches.Body)

		// Remove surrounding quotes if present (from JSON)
		expectedBody = strings.Trim(expectedBody, `"`)

		// Check if the expected body is a regex pattern (starts and ends with /)
		if len(expectedBody) > 2 && expectedBody[0] == '/' && expectedBody[len(expectedBody)-1] == '/' {
			pattern := expectedBody[1 : len(expectedBody)-1]
			matched, _ := regexp.MatchString(pattern, string(bodyBytes))
			if !matched {
				return false
			}
		} else {
			// For exact match, check if the expected body is contained in the request body
			if !strings.Contains(string(bodyBytes), expectedBody) {
				return false
			}
		}
	}

	return true
}
