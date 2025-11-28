package faults

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecuteTemplate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		method   string
		path     string
		query    url.Values
		headers  http.Header
		reqBody  string
		expected string
		wantErr  bool
	}{
		{
			name:     "Simple Substitution",
			body:     "Hello {{.Request.Query.name}}",
			method:   "GET",
			path:     "/api/hello",
			query:    url.Values{"name": []string{"World"}},
			headers:  nil,
			expected: "Hello World",
			wantErr:  false,
		},
		{
			name:     "Method and Path",
			body:     "{{.Request.Method}} request to {{.Request.Path}}",
			method:   "POST",
			path:     "/api/data",
			query:    nil,
			headers:  nil,
			expected: "POST request to /api/data",
			wantErr:  false,
		},
		{
			name:     "Header Substitution",
			body:     "User-Agent: {{index .Request.Headers \"User-Agent\"}}",
			method:   "GET",
			path:     "/",
			query:    nil,
			headers:  http.Header{"User-Agent": []string{"Go-Test"}},
			expected: "User-Agent: Go-Test",
			wantErr:  false,
		},
		{
			name:     "Missing Variable",
			body:     "Hello {{.Request.Query.missing}}",
			method:   "GET",
			path:     "/",
			query:    nil,
			headers:  nil,
			expected: "Hello ",
			wantErr:  false,
		},
		{
			name:     "Invalid Template",
			body:     "Hello {{.Request.Query.name",
			method:   "GET",
			path:     "/",
			query:    nil,
			headers:  nil,
			expected: "",
			wantErr:  true,
		},
		{
			name:     "JSON Body - Nested Field",
			body:     "Hello {{.Request.Body.name.firstName}} {{.Request.Body.name.lastName}}",
			method:   "POST",
			path:     "/api/greet",
			query:    nil,
			headers:  http.Header{"Content-Type": []string{"application/json"}},
			reqBody:  `{"name":{"firstName":"Arun","lastName":"Gopalpuri"}}`,
			expected: "Hello Arun Gopalpuri",
			wantErr:  false,
		},
		{
			name:     "JSON Body - Array Access",
			body:     "First item: {{index .Request.Body.items 0}}",
			method:   "POST",
			path:     "/api/array",
			query:    nil,
			headers:  http.Header{"Content-Type": []string{"application/json"}},
			reqBody:  `{"items":["apple","banana","orange"]}`,
			expected: "First item: apple",
			wantErr:  false,
		},
		{
			name:     "Non-JSON Body - Raw String",
			body:     "Received: {{.Request.Body}}",
			method:   "POST",
			path:     "/api/text",
			query:    nil,
			headers:  http.Header{"Content-Type": []string{"text/plain"}},
			reqBody:  "Hello World",
			expected: "Received: Hello World",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("http://localhost" + tt.path)
			u.RawQuery = tt.query.Encode()

			var body io.Reader
			if tt.reqBody != "" {
				body = strings.NewReader(tt.reqBody)
			}

			req, _ := http.NewRequest(tt.method, u.String(), body)
			if tt.headers != nil {
				req.Header = tt.headers
			}

			got, err := executeTemplate(tt.body, req)
			if tt.wantErr {
				assert.Error(t, err, "executeTemplate() error = %v, wantErr %v", err, tt.wantErr)
			} else {
				assert.NoError(t, err, "executeTemplate() unexpected error")
				assert.Equal(t, tt.expected, got, "executeTemplate() result mismatch")
			}
		})
	}
}
