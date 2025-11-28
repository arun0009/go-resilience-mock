package faults

import (
	"testing"
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
			if err == nil {
				t.Errorf("Expected error for input %s, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input %s: %v", tt.input, err)
			}
			if val != tt.expected {
				t.Errorf("Expected %d for input %s, got %d", tt.expected, tt.input, val)
			}
		}
	}
}
