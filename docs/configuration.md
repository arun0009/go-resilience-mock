<div align="center">
  <img src="go-resilience-mock.png" alt="Go Resilience Mock Mascot" width="120"/>
</div>



# Configuration

`go-resilience-mock` can be configured via `scenarios.yaml` for fault injection and environment variables for server settings.

## Environment Variables

| Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | Server port | `8080` |
| `LOG_REQUESTS` | Log each request to stdout | `true` |
| `LOG_HEADERS` | Log request headers | `false` |
| `LOG_BODY` | Log request body | `true` |
| `ENABLE_CORS` | Enable CORS for all origins | `true` |
| `RATE_LIMIT_RPS` | Requests per second limit | `0` (unlimited) |
| `HISTORY_SIZE` | Number of requests to keep in history | `100` |
| `MAX_BODY_SIZE` | Max request body size in bytes | `1048576` (1MB) |
| `ECHO_DELAY` | Global delay for all echo requests (e.g. `100ms`) | `0` |
| `ECHO_CHAOS_PROBABILITY` | Probability (0.0-1.0) of random 500 errors | `0.0` |
| `ENABLE_TLS` | `false` | Enable HTTPS support. |
| `CERT_FILE` | `cert.pem` | Path to the TLS certificate file. |
| `KEY_FILE` | `key.pem` | Path to the TLS key file. |
| `ENABLE_CORS` | `true` | Enable Cross-Origin Resource Sharing. |
| `LOG_REQUESTS` | `true` | Log incoming requests to stdout. |
| `LOG_BODY` | `true` | Log request bodies. |
| `MAX_BODY_SIZE` | `1048576` | Maximum request body size in bytes (default 1MB). |
| `HOSTNAME` | `localhost` | Hostname to use in responses. |
| `RATE_LIMIT_PER_S` | `0.0` | Global rate limit (requests per second). 0 means disabled. |
| `HISTORY_SIZE` | `100` | Number of recent requests to keep in memory. |

## Scenario Configuration (YAML)

Define multi-step response sequences in `scenarios.yaml`. See [Scenario Configuration](scenarios.md) for full details.
