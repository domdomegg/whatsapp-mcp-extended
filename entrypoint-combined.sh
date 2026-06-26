#!/bin/bash
# Combined per-user entrypoint for running the WhatsApp MCP server under
# mcp-auth-wrapper (stdio). The wrapper spawns one copy of this per authenticated
# user and injects MCP_USER_ID. This script:
#   1. derives a per-user store dir + a deterministic per-user bridge TCP port,
#   2. starts the Go bridge (background, bound to localhost),
#   3. execs the Python MCP server on stdio (the wrapper talks to it),
#   4. tears the bridge down when the server exits (reaper-safe: the wrapper
#      kills this process group on idle; the bridge must not outlive it).
set -euo pipefail

USER_ID="${MCP_USER_ID:-default}"
# Sanitize for use as a path segment (alnum, dash, underscore only).
SAFE_USER="$(printf '%s' "$USER_ID" | tr -c 'A-Za-z0-9_-' '_')"

DATA_ROOT="${DATA_ROOT:-/app/data}"
STORE_DIR="${DATA_ROOT}/store/${SAFE_USER}"
mkdir -p "$STORE_DIR"

# Deterministic per-user port in 20000-29999 from a hash of the user id, so
# concurrent per-user bridges in the shared pod network namespace don't collide.
PORT_HASH="$(printf '%s' "$USER_ID" | cksum | cut -d' ' -f1)"
BRIDGE_PORT="${BRIDGE_PORT:-$((20000 + PORT_HASH % 10000))}"

echo "[entrypoint] user=${USER_ID} store=${STORE_DIR} bridge_port=${BRIDGE_PORT}" >&2

# Shared per-process API key for the loopback bridge<->server channel. The
# bridge requires API_KEY in production; we mint an ephemeral one rather than
# disabling auth, and hand the same value to the Python client below.
export API_KEY="${API_KEY:-$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')}"

# --- start the Go bridge (background) ---------------------------------------
export STORE_DIR
export API_PORT="$BRIDGE_PORT"
export API_BIND_HOST="127.0.0.1"
# The bridge resolves its data under STORE_DIR; run it from a stable cwd.
(
  cd /app/whatsapp-bridge
  exec ./whatsapp-bridge
) &
BRIDGE_PID=$!

# Ensure the bridge dies with us (idle-reap, crash, or normal exit).
cleanup() {
  kill "$BRIDGE_PID" 2>/dev/null || true
  wait "$BRIDGE_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# --- wait for the bridge to be ready ----------------------------------------
# Poll the health endpoint so the server doesn't race the bridge on startup.
for _ in $(seq 1 50); do
  if wget -q -O /dev/null "http://127.0.0.1:${BRIDGE_PORT}/api/health" 2>/dev/null; then
    break
  fi
  sleep 0.2
done

# --- exec the Python MCP server on stdio ------------------------------------
# Points the Python bridge client at this user's bridge port. Same store dir so
# it reads the right messages.db.
export BRIDGE_HOST="localhost:${BRIDGE_PORT}"
export STORE_DIR
# Skip the import-time messages.db existence guard: during first-run onboarding
# the user calls get_setup_qr before any messages exist, so the DB may be absent.
export WA_SKIP_DB_CHECK=1
cd /app/whatsapp-mcp-server
# `python main.py` defaults to stdio transport (MCP_TRANSPORT unset).
exec python main.py
