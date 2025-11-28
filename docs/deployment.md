<div align="center">
  <img src="go-resilience-mock.png" alt="Go Resilience Mock Mascot" width="120"/>
</div>



# Deployment

## Local Development

Prerequisites: Go 1.16+

1. Clone the repository:
   ```bash
   git clone https://github.com/arun0009/go-resilience-mock.git
   cd go-resilience-mock
   ```

2. Run the server:
   ```bash
   go run main.go
   ```

3. The server will start on port 8080 (default).

## Docker
You can run the server directly using Docker.

```bash
docker run -p 8080:8080 arun0009/go-resilience-mock:latest
```

### Environment Variables

Pass configuration via `-e` flags:

```bash
docker run -p 8080:8080 \
  -e RATE_LIMIT_RPS=10 \
  -e LOG_BODY=false \
  arun0009/go-resilience-mock:latest
```

## Docker Compose

For complex setups or to mount a `scenarios.yaml` file, use the included `docker-compose.yaml`.

```bash
docker-compose up
```

## TLS (HTTPS) Support

To enable HTTPS, set `ENABLE_TLS=true` and provide the path to your certificate and key files.

```bash
# Generate self-signed certs
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes -subj '/CN=localhost'

# Run with TLS
export ENABLE_TLS=true
export CERT_FILE=./cert.pem
export KEY_FILE=./key.pem
./go-resilience-mock
```

The server will listen on the configured `PORT` (default 8080) using HTTPS.

## CI/CD Integration
This project includes a GitHub Action that allows you to easily spin up the mock server in your CI pipelines.

### GitHub Actions
Add the following step to your workflow:

```yaml
- name: Start Resilience Mock
  uses: arun0009/go-resilience-mock@main
  with:
    port: 8080
    scenarios: ./tests/scenarios.yaml
```

The server will start in the background, allowing you to run your integration tests against it.

## Kubernetes

(Coming Soon)
