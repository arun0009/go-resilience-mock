<div align="center">
  <img src="go-resilience-mock.png" alt="Go Resilience Mock Mascot" width="120"/>
</div>

# Streaming Features

`go-resilience-mock` supports real-time streaming protocols to help you test how your applications handle long-lived connections.

## WebSockets

The server provides a WebSocket echo endpoint. It accepts connections and echoes back any message it receives.

**Endpoint:** `/ws`

### Example (JavaScript)
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  console.log('Connected');
  ws.send('Hello Server!');
};

ws.onmessage = (event) => {
  console.log('Received:', event.data);
};
```

### Example (wscat)
```bash
wscat -c ws://localhost:8080/ws
> Hello
< Hello
```

## Server-Sent Events (SSE)

The server provides an SSE endpoint that streams a timestamp event every 2 seconds. This is useful for testing event source clients.

**Endpoint:** `/sse`

### Example (JavaScript)
```javascript
const evtSource = new EventSource('http://localhost:8080/sse');

evtSource.onmessage = (event) => {
  console.log('New event:', event.data);
};
```

```bash
curl -N http://localhost:8080/sse
data: The time is 2023-10-27T10:00:00Z

data: The time is 2023-10-27T10:00:02Z
...
```

## Interactive Testers

The server includes built-in HTML pages to test these features without writing code:

*   **WebSocket Tester**: [http://localhost:8080/web-ws](/web-ws)
*   **SSE Tester**: [http://localhost:8080/web-sse](/web-sse)
