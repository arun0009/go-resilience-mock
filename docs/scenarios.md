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

**Example: Simple Query Parameter**
```yaml
- path: /api/echo-id
  method: GET
  responses:
    - status: 200
      body: '{"id": "{{.Request.Query.id}}", "timestamp": "{{.Server.Timestamp}}"}'
```

**Example: Nested JSON Body (requires `Content-Type: application/json`)**
```yaml
- path: /api/greet
  method: POST
  responses:
    - status: 200
      body: 'Hello {{.Request.Body.name.firstName}} {{.Request.Body.name.lastName}}!'
```

**Available Data:**
- `.Request.ID` - Unique request ID
- `.Request.Method`
- `.Request.Path`
- `.Request.Query.paramName` (e.g., `{{.Request.Query.id}}`)
- `.Request.Headers` (use `{{index .Request.Headers "Header-Name"}}` for headers with special characters)
- `.Request.Body` (JSON parsed as nested objects/arrays if `Content-Type: application/json`, otherwise raw string)
- `.Server.Hostname`
- `.Server.Timestamp`

**Template Helper Functions:**
- `{{uuid}}` - Generate pseudo-random UUID
- `{{randomInt 1 100}}` - Random integer between min and max
- `{{add 5 3}}` - Add two integers (returns 8)
- `{{subtract 10 3}}` - Subtract two integers (returns 7)

**Example: Using Helpers**
```yaml
- path: /api/order
  method: POST
  responses:
    - status: 201
      body: '{"orderId": "{{uuid}}", "total": {{add 100 50}}, "requestId": "{{.Request.ID}}"}'
```

### Delay Jitter
Add realistic latency variation with delay ranges instead of fixed delays:

```yaml
- path: /api/slow
  method: GET
  responses:
    - status: 200
      delayRange: "100ms-500ms"  # Random delay between 100ms and 500ms
      body: "Response with variable latency"
```

Use `delay` for fixed latency or `delayRange` for jitter. If both are specified, `delayRange` takes precedence.

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
