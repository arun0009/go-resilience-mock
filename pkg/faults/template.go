package faults

import (
	"bytes"
	"html/template"
	"net/http"
	"os"
	"time"
)

// TemplateData provides contextual data to the response body template.
type TemplateData struct {
	Request struct {
		Method  string
		Path    string
		Query   map[string]string
		Headers map[string]string
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

	// Server Info
	data.Server.Timestamp = time.Now().Format(time.RFC3339)
	if hostname, err := os.Hostname(); err == nil {
		data.Server.Hostname = hostname
	} else {
		data.Server.Hostname = "unknown"
	}

	// 2. Parse and Execute
	tmpl, err := template.New("response").Parse(body)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
