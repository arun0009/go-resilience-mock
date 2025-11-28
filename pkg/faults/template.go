package faults

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// TemplateData provides contextual data to the response body template.
type TemplateData struct {
	Request struct {
		ID      string
		Method  string
		Path    string
		Query   map[string]string
		Headers map[string]string
		Body    interface{} // Parsed JSON or raw string
	}
	Server struct {
		Hostname  string
		Timestamp string
	}
}

// executeTemplate renders the response body as a Go template.
func executeTemplate(body string, r *http.Request) (string, error) {
	// 1. Prepare Data
	data := TemplateData{}

	// Request ID from context
	if reqID := r.Context().Value("requestID"); reqID != nil {
		data.Request.ID = fmt.Sprintf("%d", reqID.(uint64))
	}

	data.Request.Method = r.Method
	data.Request.Path = r.URL.Path

	// Query Params (flattened)
	data.Request.Query = make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			data.Request.Query[k] = v[0]
		}
	}

	// Headers (flattened)
	data.Request.Headers = make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			data.Request.Headers[k] = v[0]
		}
	}

	// Request Body
	if r.Body != nil {
		var bodyBuf bytes.Buffer
		_, _ = bodyBuf.ReadFrom(r.Body)
		bodyStr := bodyBuf.String()

		// Try to parse as JSON if Content-Type is application/json
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/json") && bodyStr != "" {
			var jsonBody interface{}
			if err := json.Unmarshal([]byte(bodyStr), &jsonBody); err == nil {
				data.Request.Body = jsonBody
			} else {
				// If JSON parsing fails, store as string
				data.Request.Body = bodyStr
			}
		} else {
			// Non-JSON or empty body
			data.Request.Body = bodyStr
		}

		// Restore body for subsequent readers
		r.Body = io.NopCloser(&bodyBuf)
	}

	// Server Info
	data.Server.Timestamp = time.Now().Format(time.RFC3339)
	if hostname, err := os.Hostname(); err == nil {
		data.Server.Hostname = hostname
	} else {
		data.Server.Hostname = "unknown"
	}

	// 2. Parse and Execute with custom functions
	funcMap := template.FuncMap{
		"uuid": func() string {
			return fmt.Sprintf("%d-%d-%d-%d-%d",
				time.Now().UnixNano()%10000,
				time.Now().UnixNano()%10000,
				time.Now().UnixNano()%10000,
				time.Now().UnixNano()%10000,
				time.Now().UnixNano()%10000)
		},
		"randomInt": func(min, max int) int {
			if max <= min {
				return min
			}
			return min + int(time.Now().UnixNano()%(int64(max-min)))
		},
		"add": func(a, b int) int {
			return a + b
		},
		"subtract": func(a, b int) int {
			return a - b
		},
	}

	tmpl, err := template.New("response").Funcs(funcMap).Parse(body)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
