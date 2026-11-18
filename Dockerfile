# Build the React/Vite frontend.
FROM node:22-bookworm-slim AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Compile the Rust Axum server.
FROM rust:1-bookworm AS backend-builder
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
COPY crates/ ./crates/
COPY apps/ ./apps/
RUN cargo build --release --package pebblepost-server

# Keep glibc compatibility with the Rust build and run as an unprivileged user.
FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl dumb-init \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 pebble \
    && useradd --system --uid 10001 --gid pebble --home-dir /nonexistent pebble \
    && mkdir -p /data /app/web/dist \
    && chown -R pebble:pebble /data /app

COPY --from=backend-builder --chown=pebble:pebble /app/target/release/pebblepost-server /usr/local/bin/pebblepost-server
COPY --from=frontend-builder --chown=pebble:pebble /app/web/dist/ /app/web/dist/

ENV PEBBLEPOST_HOST="0.0.0.0" \
    PORT="8080" \
    PEBBLEPOST_DATA_DIR="/data" \
    PEBBLEPOST_FRONTEND_DIR="/app/web/dist"

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://127.0.0.1:8080/api/health || exit 1

EXPOSE 8080
VOLUME ["/data"]
USER pebble:pebble
ENTRYPOINT ["dumb-init", "--", "pebblepost-server"]
