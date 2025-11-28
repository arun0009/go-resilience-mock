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

### Circuit Breaker
Simulate a stateful Circuit Breaker pattern. The server tracks failures and "trips" the breaker, rejecting requests with 503 until the timeout expires.

```yaml
- path: /api/unstable
  method: GET
  circuitBreaker:
    failureThreshold: 3      # Trip after 3 consecutive failures
    successThreshold: 2      # Reset after 2 successes in Half-Open state
    timeout: 10s             # Remain Open for 10 seconds
  responses:
    - status: 500            # These failures count towards the threshold
      body: "Internal Error"
```

### Advanced Matching Rules
Trigger scenarios only when specific conditions are met. If multiple scenarios match the same path, the first one with matching rules is used.

```yaml
- path: /api/search
  method: GET
  matches:
    query:
      q: "golang"            # Only match if ?q=golang
  responses:
    - status: 200
      body: "Search results for Golang"

- path: /api/search
  method: GET
  matches:
    headers:
      X-User-Type: "admin"   # Only match for admins
  responses:
    - status: 200
      body: "Admin Search Results"

- path: /api/data
  method: POST
  matches:
    body: "regex:^START.*"   # Match body starting with START
  responses:
    - status: 201
      body: "Created"
```
