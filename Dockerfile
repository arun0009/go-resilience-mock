# Build Stage
FROM golang:1.25.4-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum to cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o go-resilience-mock main.go

# Final Stage
FROM alpine:3.21

# Update packages for security
RUN apk update && apk upgrade --no-cache

# Create non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /

# Copy the built binary and static assets
COPY --from=builder /app/go-resilience-mock /go-resilience-mock
COPY --from=builder /app/docs /docs

# Set ownership and permissions
RUN chown -R appuser:appgroup /go-resilience-mock /docs

USER appuser

EXPOSE 8080

CMD ["/go-resilience-mock"]
