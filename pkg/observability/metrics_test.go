package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitMetrics(t *testing.T) {
	// Ensure InitMetrics can be called multiple times without panicking
	assert.NotPanics(t, func() {
		InitMetrics()
		InitMetrics()
	}, "InitMetrics should not panic")
}
