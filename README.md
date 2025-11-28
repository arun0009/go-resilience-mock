<div align="center">
  <img src="docs/go-resilience-mock.png" alt="Go Resilience Mock Mascot" width="200"/>
</div>

<h1 align="center">Go Resilience Mock Server</h1>

<p align="center">
  A lightweight, high-performance <strong>Fault Injection Server</strong> for Go.
  Test client-side resilience by simulating real-world service failures like
  <strong>delays, errors, resource stress, and sequential failures</strong> (Chaos Engineering).
</p>

<div align="center">

[![Go Report Card](https://goreportcard.com/badge/github.com/arun0009/go-resilience-mock)](https://goreportcard.com/report/github.com/arun0009/go-resilience-mock)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/arun0009/go-resilience-mock)](https://golang.org/)

</div>

---

## Overview

**Go Resilience Mock** is the perfect tool for validating your application's **retry logic**, **circuit breakers**, and **timeout settings** in CI/CD pipelines or local development. It provides a robust suite of features to simulate network instability and server failures without the need for complex infrastructure.

## Key Features

*   **Circuit Breaker Simulation**: **[NEW]** Simulate stateful circuit breakers (Closed -> Open -> Half-Open) with configurable failure thresholds and timeouts.
*   **Advanced Matching Rules**: **[NEW]** Trigger scenarios based on specific Headers, Query Parameters, or Body patterns (Regex).
*   **Health Check Endpoint**: **[NEW]** Standard `/health` endpoint with uptime tracking, system info, and extensible health checks.
*   **CI/CD Ready**: **[NEW]** Includes a GitHub Action (`uses: arun0009/go-resilience-mock@main`) for easy integration into your pipelines.
*   **Scenario-Based Fault Injection**: Define custom sequences of HTTP responses (e.g., `200 -> 500 -> 200`) using a simple `scenarios.yaml` file.
*   **Interactive Web UI**: Built-in **WebSocket** and **SSE** tester pages served directly from the binary. No external tools needed.
*   **Advanced Client-Side Control**: Inject jitter (`100ms-500ms`), custom headers, or random body sizes purely via request headers (`X-Echo-*`).
*   **Chaos Endpoints**: Dedicated, simple API endpoints to inject **system-level stress** (CPU, Memory) directly from your resilience tests.
*   **First-Class Observability**: Native integration with **Prometheus** tracks every injected fault, including type (`delay`, `http_error`, `cpu_stress`), path, and duration.
*   **Mock & Echo**: Provides both a powerful request echoing utility and the ability to serve custom JSON payloads for mocking external dependencies.
*   **Production-Ready**: Includes built-in support for CORS, Rate Limiting, HTTP/2, TLS, and a **built-in documentation viewer**.

## Installation

### Using Go

```bash
go get github.com/arun0009/go-resilience-mock
```

### From Source

```bash
git clone https://github.com/arun0009/go-resilience-mock.git
cd go-resilience-mock
go run main.go
```

### Using Docker

```bash
docker run -p 8080:8080 arun0009/go-resilience-mock
```

### Using Docker Compose

```bash
docker-compose up
```

## Getting Started

1.  **Start the server:**
    ```bash
    go run main.go
    ```

2.  **Verify it's running:**
    Visit `http://localhost:8080/echo` in your browser or use `curl`.

3.  **Explore the Documentation:**
    Visit `http://localhost:8080/docs/` for the built-in documentation viewer.

## Core Endpoints

These handlers allow external systems (like your test runner or a dedicated chaos tool) to trigger faults instantly.

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/echo` | `ANY` | Returns the request body and headers as a JSON response. Supports `X-Echo-*` headers for dynamic faults. |
| `/api/stress/cpu/{duration}` | `GET` | Consumes 100% of available CPU cores for the specified duration (e.g., `10s`). |
| `/api/stress/mem/{size}` | `GET` | Allocates and holds a large amount of memory (e.g., `100MB`). |
| `/history` | `GET` | Returns a JSON array of recent requests processed. |
| `/replay` | `POST` | Replays a past request by ID to the same or different target. |
| `/scenario` | `POST` | Dynamically add new scenarios at runtime without restart. |
| `/info` | `GET` | Returns server status, uptime, and configuration details. |
| `/metrics` | `GET` | Prometheus metrics for response duration and faults injected. |

## Scenario Configuration

Use `scenarios.yaml` to define multi-step response sequences for specific paths to test recovery logic.

```yaml
# scenarios.yaml
- path: /api/payment/v1
  method: POST
  responses:
    # 1. First call works, but is slow (testing client timeout/retry)
    - status: 202 
      delay: 5s 
      body: '{"status": "pending"}'
      
    # 2. Second call fails (testing circuit breaker trip)
    - status: 503 
      delay: 100ms
      body: '{"error": "service unavailable"}'

    # 3. Third call succeeds (sequence repeats)
    - status: 200 
      delay: 50ms 
      body: '{"status": "success"}'
```