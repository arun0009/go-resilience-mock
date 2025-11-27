<div align="center">
  <img src="go-resilience-mock.png" alt="Go Resilience Mock Mascot" width="120"/>
</div>



# Observability

`go-resilience-mock` provides built-in tools to monitor traffic and faults.

## Prometheus Metrics

Metrics are exposed at `/metrics`.

| Metric Name | Type | Labels | Description |
| :--- | :--- | :--- | :--- |
| `mock_faults_injected_total` | Counter | `type` (delay, http_error, cpu_stress), `path` | Total number of faults injected. |
| `mock_inflight_requests` | Gauge | None | Current number of active requests. |
| `mock_response_duration_seconds` | Histogram | `path`, `method`, `status` | Latency distribution of responses. |

## Request History

The server keeps a circular buffer of recent requests.

**Endpoint:** `GET /history`

Returns a JSON array of request details, including **ID**, timestamp, method, path, headers, and body snippet. Use the **ID** to replay requests via `/replay`.

**Clear History:** `POST /api/control/reset-history`

## Logging

Configure logging via environment variables:
- `LOG_REQUESTS=true`: Log basic request info (Method, Path, Status, Duration).
- `LOG_HEADERS=true`: Log request headers.
- `LOG_BODY=true`: Log request bodies.
