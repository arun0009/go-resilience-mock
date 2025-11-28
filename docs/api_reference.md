<div align="center">
  <img src="go-resilience-mock.png" alt="Go Resilience Mock Mascot" width="120"/>
</div>



# API Reference

`go-resilience-mock` provides several built-in endpoints for control, stress testing, and observability.

## Core Endpoints

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/echo` | `ANY` | Echoes the request back as JSON. Useful for debugging client requests. |
| `/history` | `GET` | Returns a JSON array of the last `HISTORY_SIZE` requests. |
| `/dump` | `GET` | Returns 200 OK. Simple health check or dump endpoint. |

## Observability

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/health` | `GET` | Health check endpoint with uptime tracking, system info (goroutines, OS, arch), and extensible health checks. |
| `/metrics` | `GET` | Prometheus metrics endpoint. |

## Stress Testing (Chaos)

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/api/stress/cpu/{duration}` | `GET` | Consumes CPU for the specified duration (e.g., `/api/stress/cpu/5s`). |
| `/api/stress/mem/{size}` | `GET` | Allocates memory of specified size (e.g., `/api/stress/mem/100MB`). |

## Streaming

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/ws` | `GET` | Websocket echo endpoint. Connect and send messages to have them echoed back. |
| `/sse` | `GET` | Server-Sent Events endpoint. Streams the current time every 2 seconds. |

## Control

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/api/control/reset-history` | `POST` | Clears the request history. |
| `/api/control/reset-metrics` | `POST` | Resets the `mock_faults_injected_total` metric. |
| `/replay` | `POST` | Replays a past request. Body: `{"id": "123", "target": "http://..."}`. |
| `/scenario` | `POST` | Adds a dynamic scenario. Body: JSON Scenario object or array. |
