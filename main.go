package main

import (
	"log"
	"net/http"

	"github.com/arun0009/go-resilience-mock/pkg/config"
	"github.com/arun0009/go-resilience-mock/pkg/observability"
	"github.com/arun0009/go-resilience-mock/pkg/server"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	log.SetFlags(0)
	log.Println("Initializing Go Resilience Mock Server...")

	// 1. Load Configuration
	cfg, err := config.LoadConfig("scenarios.yaml")
	if err != nil {
		log.Fatalf("Fatal: Failed to load config: %v", err)
	}

	// 2. Initialize Observability
	observability.InitMetrics()

	// 3. Register Prometheus Handler
	http.Handle("/metrics", promhttp.Handler())

	// 4. Create and Start Server
	router := server.NewRouter(cfg)

	// Use the router built by the server package
	http.Handle("/", router)

	// Handle HTTP/2 Cleartext (h2c) for compatibility
	// Note: The mixed router logic from the old main.go is now handled inside pkg/server/server.go's Run function

	log.Printf("Server starting on port %s (TLS: %t, CORS: %t)", cfg.Port, cfg.EnableTLS, cfg.EnableCORS)

	if cfg.EnableTLS {
		// Use server.RunTLS or os.Exit(1) if certs are missing
		log.Fatalf("Server failed to run with TLS: %v", server.RunTLS(cfg))
	} else {
		log.Fatalf("Server failed to run: %v", server.Run(cfg))
	}
}
