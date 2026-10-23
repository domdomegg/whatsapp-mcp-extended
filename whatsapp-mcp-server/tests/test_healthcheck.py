"""Tests for healthcheck.py, the exec liveness probe."""

import healthcheck
from healthcheck import Proc, check


def launcher(pid: int) -> Proc:
    return Proc(pid=pid, ppid=1, comm="python", cmdline=["python", "/app/whatsapp-mcp-server/run_server.py"])


def bridge(pid: int, ppid: int, port: int = 20452) -> Proc:
    return Proc(
        pid=pid,
        ppid=ppid,
        comm="whatsapp-bridge",
        cmdline=["/app/whatsapp-bridge/whatsapp-bridge"],
        env={"API_PORT": str(port)},
    )


def fetcher(responses: dict):
    """Map port -> JSON dict, or an exception instance to raise."""

    def fetch(port: int) -> dict:
        r = responses[port]
        if isinstance(r, Exception):
            raise r
        return r

    return fetch


def test_idle_container_with_no_launchers_is_healthy():
    assert check([Proc(pid=1, ppid=0, comm="node", cmdline=["mcp-auth-wrapper"])], fetcher({})) == []


def test_connected_bridge_is_healthy():
    procs = [launcher(10), bridge(11, 10)]
    assert check(procs, fetcher({20452: {"connected": True, "needs_pairing": False}})) == []


def test_launcher_without_bridge_child_fails():
    # The silent-death case: the watchdog exited the bridge, nothing respawned it.
    problems = check([launcher(10)], fetcher({}))
    assert len(problems) == 1
    assert "no whatsapp-bridge child" in problems[0]


def test_unreachable_bridge_fails():
    procs = [launcher(10), bridge(11, 10)]
    problems = check(procs, fetcher({20452: OSError("connection refused")}))
    assert problems == ["bridge 11 on :20452 unreachable: connection refused"]


def test_disconnected_paired_bridge_fails_with_duration():
    procs = [launcher(10), bridge(11, 10)]
    health = {"connected": False, "needs_pairing": False, "disconnected_for": "4m10s", "reconnect_errs": 3}
    problems = check(procs, fetcher({20452: health}))
    assert len(problems) == 1
    assert "not connected (for 4m10s, reconnect errors 3)" in problems[0]


def test_unpaired_bridge_is_not_a_failure():
    # Restarting the pod cannot pair an account; failing here would loop forever.
    procs = [launcher(10), bridge(11, 10)]
    assert check(procs, fetcher({20452: {"connected": False, "needs_pairing": True}})) == []


def test_each_launcher_checked_independently():
    procs = [launcher(10), bridge(11, 10, 20001), launcher(20), bridge(21, 20, 20002), launcher(30)]
    responses = {20001: {"connected": True}, 20002: {"connected": False, "disconnected_for": "1m"}}
    problems = check(procs, fetcher(responses))
    assert len(problems) == 2
    assert problems[0].startswith("bridge 21 on :20002 not connected")
    assert problems[1].startswith("launcher 30 has no whatsapp-bridge child")


def test_processes_that_only_mention_the_launcher_are_not_launchers():
    # `sh -c "... run_server.py ..."` and `python -c "..."` are not launchers.
    procs = [
        Proc(pid=5, ppid=1, comm="sh", cmdline=["sh", "-c", "python /app/whatsapp-mcp-server/run_server.py"]),
        Proc(pid=6, ppid=1, comm="python", cmdline=["python", "-c", "print('run_server.py')"]),
        Proc(pid=7, ppid=1, comm="node", cmdline=["mcp-auth-wrapper"]),
    ]
    assert check(procs, fetcher({})) == []


def test_bridge_without_port_fails():
    b = bridge(11, 10)
    b.env = {}
    problems = check([launcher(10), b], fetcher({}))
    assert problems == ["bridge 11 has no API_PORT in its environment"]


def test_list_procs_parses_stat_cmdline_and_environ(tmp_path):
    p = tmp_path / "123"
    p.mkdir()
    (p / "stat").write_text("123 (whatsapp-bridge) S 45 123 123 0 -1 4194560 0 0 0 0")
    (p / "cmdline").write_bytes(b"/app/whatsapp-bridge/whatsapp-bridge\0")
    (p / "environ").write_bytes(b"API_PORT=20452\0API_BIND_HOST=127.0.0.1\0")
    (tmp_path / "self").mkdir()  # non-numeric entries are skipped
    [proc] = healthcheck.list_procs(str(tmp_path))
    assert (proc.pid, proc.ppid, proc.comm) == (123, 45, "whatsapp-bridge")
    assert proc.cmdline == ["/app/whatsapp-bridge/whatsapp-bridge"]
    assert proc.env["API_PORT"] == "20452"


def test_main_exit_code(monkeypatch, capsys):
    monkeypatch.setattr(healthcheck, "list_procs", lambda: [launcher(10)])
    assert healthcheck.main() == 1
    assert "no whatsapp-bridge child" in capsys.readouterr().err
    monkeypatch.setattr(healthcheck, "list_procs", lambda: [])
    assert healthcheck.main() == 0
