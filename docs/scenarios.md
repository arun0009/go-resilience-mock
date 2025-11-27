<div align="center">
  <img src="go-resilience-mock.png" alt="Go Resilience Mock Mascot" width="120"/>
</div>



# Scenario Configuration

Scenarios allow you to define complex, stateful sequences of responses for specific endpoints. This is the core of `go-resilience-mock`'s fault injection capability.

> [!TIP]
> You can also add scenarios dynamically at runtime via the `POST /scenario` endpoint.

## Configuration File

Scenarios are defined in `scenarios.yaml`.

## Structure

```yaml
- path: /api/endpoint
  method: GET
  responses:
    - status: 200
      delay: 0s
      body: '{"status": "ok"}'
```

## Advanced Features

### Sequential Responses
Define a list of responses to simulate a changing state. The server cycles through them in order.

```yaml
- path: /api/retry-test
  method: GET
  responses:
    # 1. Fail first
    - status: 503
      body: '{"error": "unavailable"}'
    # 2. Succeed on retry
    - status: 200
      body: '{"status": "recovered"}'
```

### Probability
Inject faults randomly. If the probability check fails, the server falls back to a default "Echo" behavior (200 OK with request details).

```yaml
- path: /api/flaky
  method: GET
  responses:
    - status: 500
      probability: 0.1 # 10% chance of failure
      body: '{"error": "random failure"}'
```

### Dynamic Templates
Use Go templates in the response body to inject request data or server state.

```yaml
- path: /api/echo-id
  method: GET
  responses:
    - status: 200
      body: '{"id": "{{.Request.Query.id}}", "timestamp": "{{.Server.Timestamp}}"}'
```

**Available Data:**
- `.Request.Method`
- `.Request.Path`
- `.Request.Query` (e.g., `.Request.Query.Get "param"`)
- `.Request.Headers`
- `.Server.Hostname`
- `.Server.Timestamp`
