package faults

import (
	"net/http"
	"net/url"
	"testing"
)

func TestExecuteTemplate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		method   string
		path     string
		query    url.Values
		headers  http.Header
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("http://localhost" + tt.path)
			u.RawQuery = tt.query.Encode()
			req := &http.Request{
				Method: tt.method,
				URL:    u,
				Header: tt.headers,
			}

			got, err := executeTemplate(tt.body, req)
			if (err != nil) != tt.wantErr {
				t.Errorf("executeTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("executeTemplate() = %v, want %v", got, tt.expected)
			}
		})
	}
}
