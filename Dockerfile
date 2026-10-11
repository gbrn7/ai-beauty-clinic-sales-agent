# ─────────────────────────────────────────────
# Stage 1: Build
# ─────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies (for pure-Go sqlite, no cgo needed)
RUN apk add --no-cache git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build both binaries (static, no CGO since we use glebarez/sqlite)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/cli    ./cmd/cli

# ─────────────────────────────────────────────
# Stage 2: Runtime
# ─────────────────────────────────────────────
# Install curl for healthcheck
RUN apk add --no-cache curl

# Non-root user for security
RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /out/server /app/server
COPY --from=builder /out/cli    /app/cli

# Assets are embedded via //go:embed — no separate copy needed.
# Create persistent directories for SQLite and local media storage
RUN mkdir -p /app/data /app/media && chown -R app:app /app

USER app

# Server listens on this port (configurable via PORT env)
EXPOSE 8000

# Default entrypoint is the HTTP server
ENTRYPOINT ["/app/server"]
