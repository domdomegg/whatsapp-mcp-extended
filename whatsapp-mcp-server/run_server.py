#!/usr/bin/env python
"""Per-user launcher: supervise the Go bridge, then run the MCP server (stdio).

This is the single process the mcp-auth-wrapper spawns per authenticated user.
It owns the Go bridge's lifecycle directly in Python — far more robust than a
bash entrypoint, which couldn't both exec the server AND reap the bridge.

Design:
  - Derive a per-user store dir + a deterministic loopback port from MCP_USER_ID.
  - Start the Go bridge as a child, set the env the MCP server reads at import.
  - On exit (idle-reap, crash, signal) the bridge is terminated with us — an
    atexit hook plus a try/finally around mcp.run() (which anyio unwinds on
    SIGTERM/SIGINT) guarantee no orphan keeps holding the port.
  - The bridge's stdout/stderr go to OUR stderr, keeping stdout clean for the MCP
    stdio JSON-RPC channel.
"""

import atexit
import os
import subprocess
import sys
import time
import urllib.request
import zlib

BRIDGE_DIR = os.environ.get("BRIDGE_DIR", "/app/whatsapp-bridge")
BRIDGE_BIN = os.path.join(BRIDGE_DIR, "whatsapp-bridge")
SERVER_DIR = os.path.dirname(os.path.abspath(__file__))

_bridge_proc: subprocess.Popen | None = None


def _per_user_port(user_id: str) -> int:
    """Deterministic loopback port in 20000-29999 from the user id."""
    return 20000 + (zlib.crc32(user_id.encode()) % 10000)


def _stop_bridge() -> None:
    """Terminate the bridge child if running. Idempotent; safe in atexit/signals."""
    global _bridge_proc
    proc = _bridge_proc
    _bridge_proc = None
    if proc and proc.poll() is None:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()


def _wait_healthy(port: int, timeout: float = 10.0) -> None:
    """Poll the bridge health endpoint so the server doesn't race startup."""
    deadline = time.monotonic() + timeout
    url = f"http://127.0.0.1:{port}/api/health"
    while time.monotonic() < deadline:
        try:
            urllib.request.urlopen(url, timeout=1).read()
            return
        except Exception:
            time.sleep(0.2)
    # Not fatal: the server starts anyway and surfaces bridge errors per-call.
    print(f"[launcher] bridge health check timed out on :{port}", file=sys.stderr)


def main() -> None:
    global _bridge_proc

    user_id = os.environ.get("MCP_USER_ID", "default")
    safe_user = "".join(c if c.isalnum() or c in "-_" else "_" for c in user_id)
    data_root = os.environ.get("DATA_ROOT", "/app/data")
    store_dir = os.path.join(data_root, "store", safe_user)
    os.makedirs(store_dir, exist_ok=True)

    port = int(os.environ.get("BRIDGE_PORT") or _per_user_port(user_id))
    # Ephemeral key for the loopback bridge<->server channel (bridge requires one).
    api_key = os.environ.get("API_KEY") or os.urandom(32).hex()

    # Env the MCP server reads AT IMPORT — must be set before importing it below.
    os.environ["STORE_DIR"] = store_dir
    os.environ["BRIDGE_HOST"] = f"localhost:{port}"
    os.environ["API_KEY"] = api_key
    os.environ.setdefault("WA_SKIP_DB_CHECK", "1")

    print(f"[launcher] user={user_id} store={store_dir} port={port}", file=sys.stderr)

    # Start the bridge; its stdout (logs + QR) must not pollute our stdout, which
    # is the MCP stdio protocol channel — send it all to stderr.
    _bridge_proc = subprocess.Popen(
        [BRIDGE_BIN],
        cwd=BRIDGE_DIR,
        stdout=sys.stderr,
        stderr=sys.stderr,
        env={**os.environ, "API_PORT": str(port), "API_BIND_HOST": "127.0.0.1"},
    )
    # atexit covers normal exit and unhandled exceptions. SIGTERM/SIGINT (the
    # wrapper's idle-reap) is handled by anyio inside mcp.run(), which unwinds the
    # call so the `finally` below runs. Belt and braces: both reap the bridge.
    atexit.register(_stop_bridge)

    _wait_healthy(port)

    # Import + run the server only now that env is set. Defaults to stdio transport.
    sys.path.insert(0, SERVER_DIR)
    import main as mcp_main  # noqa: E402

    transport = os.environ.get("MCP_TRANSPORT", "stdio")
    if transport in {"sse", "streamable-http"}:
        mcp_main.mcp.settings.host = os.environ.get("HOST", "0.0.0.0")
        mcp_main.mcp.settings.port = int(os.environ.get("PORT", "8081"))
    try:
        mcp_main.mcp.run(transport=transport)
    finally:
        _stop_bridge()


if __name__ == "__main__":
    main()
