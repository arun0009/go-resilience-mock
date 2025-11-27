<div align="center">
  <img src="go-resilience-mock.png" alt="Go Resilience Mock Mascot" width="120"/>
</div>



# Chaos Engineering

`go-resilience-mock` allows you to inject system-level faults to test how your application behaves under stress.

## CPU Stress

Simulate high CPU usage to test your application's behavior when the server is under heavy load.

**Endpoint:** `GET /api/stress/cpu/{duration}`

**Example:** Stress CPU for 5 seconds
```bash
curl http://localhost:8080/api/stress/cpu/5s
```

## Memory Stress

Simulate memory pressure to test how your application handles low memory situations or garbage collection pauses.

**Endpoint:** `GET /api/stress/mem/{size}`

**Example:** Allocate 100MB of memory
```bash
curl http://localhost:8080/api/stress/mem/100MB
```

## Network Faults (Scenarios)

For network-level faults like latency, timeouts, and HTTP errors, use the **Scenarios** feature. See [Scenarios Configuration](scenarios.md) for details.

## Header-Based Chaos (Client-Driven)

You can also control faults on a per-request basis using HTTP headers. This is useful for test runners that need to simulate specific conditions without changing server config.

| Header | Value | Description |
| :--- | :--- | :--- |
| `X-Echo-Delay` | `100ms`, `2s` | Delays the response by the specified duration. |
| `X-Echo-Status` | `500`, `404` | Forces the response status code. |
| `X-Echo-Body` | `{"error": "fail"}` | Overrides the response body. |
| `X-Echo-Headers` | `{"X-Custom": "foo"}` | JSON map of headers to include in the response. |

**Example:**
```bash
curl -v -H "X-Echo-Delay: 500ms" -H "X-Echo-Status: 503" http://localhost:8080/echo
```
