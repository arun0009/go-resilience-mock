package observability

import (
	"testing"
)

func TestInitMetrics(t *testing.T) {
	// Ensure InitMetrics can be called multiple times without panicking
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InitMetrics panicked: %v", r)
		}
	}()

	InitMetrics()
	InitMetrics()
}
