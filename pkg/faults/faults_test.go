package faults

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMemorySize(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		hasError bool
	}{
		{"1KB", 1024, false},
		{"1MB", 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"100MB", 100 * 1024 * 1024, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		val, err := parseMemorySize(tt.input)
		if tt.hasError {
			assert.Error(t, err, "Expected error for input %s", tt.input)
		} else {
			assert.NoError(t, err, "Unexpected error for input %s", tt.input)
			assert.Equal(t, tt.expected, val, "Expected %d for input %s", tt.expected, tt.input)
		}
	}
}
