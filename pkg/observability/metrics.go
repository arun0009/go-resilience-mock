package observability

import (
	"sync"

	"github.com/arun0009/go-resilience-mock/pkg/config"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// FaultsInjected tracks the number of times a fault (delay, error, stress) was injected
	FaultsInjected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mock_faults_injected_total",
			Help: "Total number of simulated faults injected, labeled by type (delay, http_error, cpu_stress).",
		},
		[]string{"type", "path"},
	)

	// InflightRequests tracks the current number of requests being handled
	InflightRequests = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mock_inflight_requests",
			Help: "Current number of requests being processed by the server.",
		},
	)

	// ResponseDuration is a histogram to track the latency of all requests
	ResponseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mock_response_duration_seconds",
			Help:    "Histogram of response latency (including injected delay) for HTTP requests.",
			Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10, 30}, // Up to 30s delay
		},
		[]string{"path", "method", "status"},
	)

	initOnce sync.Once
)

// InitMetrics registers all custom Prometheus collectors.
// InitMetrics registers all custom Prometheus collectors.
func InitMetrics() {
	initOnce.Do(func() {
		reg := config.GetRegistry()
		reg.MustRegister(FaultsInjected)
		reg.MustRegister(InflightRequests)
		reg.MustRegister(ResponseDuration)
	})
}
