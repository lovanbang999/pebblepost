# Multi-stage Dockerfile for PebblePost Web Server
# Stage 1: Build Frontend Assets
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Compile Static Go Binary
FROM golang:alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy compiled frontend into embed destination
COPY --from=frontend-builder /app/web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.version=0.2.0 -X main.builtBy=docker" \
    -o /app/bin/pebblepost-server ./cmd/server

# Stage 3: Lightweight, Secure Production Runtime
FROM alpine:3.21 AS runtime

# Install basic runtime utilities for security and healthchecks
RUN apk add --no-cache ca-certificates dumb-init curl

# Create non-root user and group
RUN addgroup -g 10001 -S pebble && \
    adduser -u 10001 -S pebble -G pebble

# Create persistent data directory with permissions
RUN mkdir -p /data && chown -R pebble:pebble /data

COPY --from=backend-builder --chown=pebble:pebble /app/bin/pebblepost-server /usr/local/bin/pebblepost-server

# Default environment variables
ENV PEBBLEPOST_HOST="0.0.0.0" \
    PORT="8080" \
    PEBBLEPOST_DATA_DIR="/data"

# Healthcheck monitoring server readiness
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://127.0.0.1:8080/api/health || exit 1

EXPOSE 8080
VOLUME ["/data"]

USER pebble:pebble

ENTRYPOINT ["dumb-init", "--", "pebblepost-server"]
