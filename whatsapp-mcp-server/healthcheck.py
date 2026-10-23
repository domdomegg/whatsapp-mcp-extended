"""Liveness check for the combined container: exit 1 when WhatsApp is not really up.

Why this exists: the Go bridge is a child of run_server.py (one per user). When
its connection to WhatsApp dies for good, the bridge's watchdog exits — but
nothing respawns it, and from outside everything still looks healthy: the MCP
server answers, reads come from sqlite, only sends fail and new messages stop
arriving. That went unnoticed for hours on 2026-09-13, 09-29 and 10-22. The
only supervisor that can act on it is Kubernetes, via this script as an exec
livenessProbe: fail here and the pod is restarted.

A failure is any launcher whose bridge child is gone, cannot be reached on its
loopback port, or reports that a paired account is not connected. An unpaired
account (needs_pairing) is not a failure: there is nothing to reconnect, and
failing on it would restart the pod forever. No launchers at all (idle) is
healthy. Grace for a bridge that is still connecting comes from the probe's
failureThreshold, not from here.

Stdlib only: the runtime image has no curl, ps or pgrep.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.request
from dataclasses import dataclass, field


@dataclass
class Proc:
    pid: int
    ppid: int
    comm: str
    cmdline: list[str]
    env: dict[str, str] = field(default_factory=dict)


LAUNCHER_SCRIPT = "run_server.py"
BRIDGE_COMM = "whatsapp-bridge"


def is_launcher(proc: Proc) -> bool:
    """True for `python .../run_server.py` itself, not for things that merely mention it.

    Matching the argv loosely (any argument containing the name) made the probe
    count its own `python -c "...run_server.py..."` test invocation, and would
    count an `sh -c` wrapper, as launchers with no bridge.
    """
    return len(proc.cmdline) >= 2 and os.path.basename(proc.cmdline[1]) == LAUNCHER_SCRIPT


def list_procs(proc_root: str = "/proc") -> list[Proc]:
    """Every process visible in /proc, with ppid, comm, cmdline and environ.

    Environ is readable because all of the container's processes run as the
    same user. Processes that vanish mid-read are skipped.
    """
    procs: list[Proc] = []
    for name in os.listdir(proc_root):
        if not name.isdigit():
            continue
        base = os.path.join(proc_root, name)
        try:
            with open(os.path.join(base, "stat"), encoding="utf-8", errors="replace") as f:
                stat = f.read()
            with open(os.path.join(base, "cmdline"), "rb") as f:
                cmdline = [a.decode("utf-8", "replace") for a in f.read().split(b"\0") if a]
            with open(os.path.join(base, "environ"), "rb") as f:
                env_pairs = [a.decode("utf-8", "replace") for a in f.read().split(b"\0") if a]
        except OSError:
            continue
        # comm is in parentheses and may contain spaces; ppid is the 4th field after it.
        comm = stat[stat.index("(") + 1 : stat.rindex(")")]
        ppid = int(stat[stat.rindex(")") + 2 :].split()[1])
        env = dict(p.split("=", 1) for p in env_pairs if "=" in p)
        procs.append(Proc(pid=int(name), ppid=ppid, comm=comm, cmdline=cmdline, env=env))
    return procs


def fetch_health(port: int, timeout: float = 3.0) -> dict:
    """GET the bridge's unauthenticated /api/health; a 503 still carries JSON."""
    url = f"http://127.0.0.1:{port}/api/health"
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        try:
            return json.loads(e.read().decode("utf-8"))
        except (ValueError, OSError):
            raise OSError(f"HTTP {e.code} with no JSON body") from e


def check(procs: list[Proc], fetch=fetch_health) -> list[str]:
    """Return the reasons the container is unhealthy; empty means healthy."""
    problems: list[str] = []
    launchers = [p for p in procs if is_launcher(p)]
    for launcher in launchers:
        bridges = [p for p in procs if p.ppid == launcher.pid and p.comm == BRIDGE_COMM]
        if not bridges:
            problems.append(f"launcher {launcher.pid} has no {BRIDGE_COMM} child (it died and was not respawned)")
            continue
        for bridge in bridges:
            port = bridge.env.get("API_PORT")
            if not port:
                problems.append(f"bridge {bridge.pid} has no API_PORT in its environment")
                continue
            try:
                health = fetch(int(port))
            except (OSError, ValueError) as e:
                problems.append(f"bridge {bridge.pid} on :{port} unreachable: {e}")
                continue
            if health.get("needs_pairing"):
                continue  # not linked to an account: nothing to keep alive
            if not health.get("connected"):
                for_how_long = health.get("disconnected_for", "unknown")
                problems.append(
                    f"bridge {bridge.pid} on :{port} not connected (for {for_how_long}, reconnect errors {health.get('reconnect_errs', '?')})"
                )
    return problems


def main() -> int:
    problems = check(list_procs())
    for p in problems:
        print(f"[healthcheck] {p}", file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
