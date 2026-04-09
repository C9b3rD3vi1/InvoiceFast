# Build Stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o migrate ./cmd/migrate

# Production Stage
FROM alpine:3.19

# Install certificates and ca-certificates
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Create directories
RUN mkdir -p /app/data /app/templates /app/logs

# Copy binaries from builder
COPY --from=builder /app/server /app/server
COPY --from=builder /app/migrate /app/migrate

# Copy configuration
COPY .env.example /app/.env
COPY templates /app/templates

# Set permissions
RUN chmod +x /app/server /app/migrate

# Expose port
EXPOSE 8082

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8082/health || exit 1

# Run as non-root user
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

USER appuser

# Start command
CMD ["./server"]
