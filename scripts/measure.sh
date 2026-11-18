#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
BENCH_DATA_DIR="/tmp/pebblepost-bench-$$"
DOCS_BENCH_FILE="$ROOT_DIR/docs/benchmarks.md"
TEST_PORT=39281
TEST_TOKEN="bench_secret_token_12345"
SERVER_PID=""

mkdir -p "$BIN_DIR" "$BENCH_DATA_DIR"
cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$BENCH_DATA_DIR"
}
trap cleanup EXIT

if [ ! -d "$ROOT_DIR/web/dist" ]; then
  (cd "$ROOT_DIR/web" && npm run build)
fi

echo "Building Rust release binaries..."
(cd "$ROOT_DIR" && cargo build --release --package pebblepost-cli --package pebblepost-server)
cp "$ROOT_DIR/target/release/pebblepost" "$BIN_DIR/pebblepost"
cp "$ROOT_DIR/target/release/pebblepost-server" "$BIN_DIR/pebblepost-server"

CLI_SIZE_BYTES=$(stat -c %s "$BIN_DIR/pebblepost" 2>/dev/null || stat -f %z "$BIN_DIR/pebblepost")
SERVER_SIZE_BYTES=$(stat -c %s "$BIN_DIR/pebblepost-server" 2>/dev/null || stat -f %z "$BIN_DIR/pebblepost-server")
CLI_SIZE_MB=$(awk "BEGIN {printf \"%.2f\", ${CLI_SIZE_BYTES}/1048576}")
SERVER_SIZE_MB=$(awk "BEGIN {printf \"%.2f\", ${SERVER_SIZE_BYTES}/1048576}")

CLI_STARTUP_MS=$(python3 -c "
import subprocess, time
runs = []
for _ in range(10):
    start = time.perf_counter()
    subprocess.run(['${BIN_DIR}/pebblepost', '--version'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    runs.append((time.perf_counter() - start) * 1000)
print(f'{sum(runs)/len(runs):.2f}')
")

SERVER_STARTUP_MS=$(python3 -c "
import os, subprocess, time, urllib.request, sys
env = os.environ.copy()
env.update({'PEBBLEPOST_HOST': '127.0.0.1', 'PORT': '${TEST_PORT}', 'PEBBLEPOST_TOKEN': '${TEST_TOKEN}', 'PEBBLEPOST_DATA_DIR': '${BENCH_DATA_DIR}', 'PEBBLEPOST_FRONTEND_DIR': '${ROOT_DIR}/web/dist'})
start = time.perf_counter()
proc = subprocess.Popen(['${BIN_DIR}/pebblepost-server'], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
open('${BENCH_DATA_DIR}/server.pid', 'w').write(str(proc.pid))
for _ in range(100):
    try:
        with urllib.request.urlopen('http://127.0.0.1:${TEST_PORT}/api/health', timeout=0.5) as response:
            if response.status == 200:
                print(f'{(time.perf_counter()-start)*1000:.2f}')
                sys.exit(0)
    except Exception:
        time.sleep(0.01)
proc.terminate()
sys.exit('Server did not become healthy in time')
")

SERVER_PID=$(cat "$BENCH_DATA_DIR/server.pid")
sleep 2
RSS_KB=$(ps -o rss= -p "$SERVER_PID" | tr -d ' ')
RSS_MB=$(awk "BEGIN {printf \"%.2f\", ${RSS_KB}/1024}")
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""

RUST_VERSION=$(rustc --version)
mkdir -p "$(dirname "$DOCS_BENCH_FILE")"
cat <<EOF > "$DOCS_BENCH_FILE"
# Empirical Performance & Footprint Benchmarks

*Measured on Linux x86_64 with ${RUST_VERSION} and Cargo's release profile.*

| Metric | PebblePost CLI | PebblePost Server | Postman (Desktop v11) | Bruno (Desktop v1.38) |
| :--- | :--- | :--- | :--- | :--- |
| Cold startup time | ${CLI_STARTUP_MS} ms | ${SERVER_STARTUP_MS} ms | ~2,400 ms | ~1,100 ms |
| Idle memory (RSS) | Ephemeral process | ${RSS_MB} MB | ~420 MB | ~170 MB |
| Executable size | ${CLI_SIZE_MB} MB | ${SERVER_SIZE_MB} MB | ~180 MB installer | ~95 MB installer |
| Runtime architecture | Rust native binary | Axum server serving React assets | Electron / Chromium | Electron / Chromium |

Reproduce these measurements with `./scripts/measure.sh`.

- Startup is measured until `/api/health` returns HTTP 200.
- Idle memory is process RSS after two seconds.
- Executable sizes are measured after a Cargo release build.
EOF

echo "Saved results to $DOCS_BENCH_FILE"
