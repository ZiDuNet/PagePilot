#!/usr/bin/env python3
"""Cross-platform smoke test for the hostctl-deploy skill script."""
from __future__ import annotations

import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timezone


ROOT = pathlib.Path(__file__).resolve().parent
SCRIPT = ROOT / "skill" / "hostctl-deploy" / "scripts" / "hostctl_deploy.py"
SERVER = os.environ.get("HOSTCTL_SERVER", "http://127.0.0.1:8787")


def cooldown_delay(data: dict | None) -> float:
    """Return the server-provided cooldown delay, including absolute timestamps."""
    if not data:
        return 0.0
    delay = float(data.get("retryAfterSeconds") or 0)
    raw = str(data.get("nextAvailableAt") or "").strip()
    if raw:
        try:
            available = datetime.fromisoformat(raw.replace("Z", "+00:00"))
            if available.tzinfo is None:
                available = available.replace(tzinfo=timezone.utc)
            delay = max(delay, (available - datetime.now(timezone.utc)).total_seconds())
        except ValueError:
            pass
    return max(0.0, delay)


def run(*args: str, output: pathlib.Path | None = None, env: dict[str, str] | None = None) -> dict | None:
    cmd = [sys.executable, str(SCRIPT), "--server", SERVER, *args]
    proc = None
    for attempt in range(6):
        proc = subprocess.run(cmd, cwd=ROOT, text=True, capture_output=True, check=False, env=env)
        if proc.returncode == 0:
            break
        try:
            error_data = json.loads(proc.stdout)
        except json.JSONDecodeError:
            error_data = None
        if not error_data or error_data.get("httpStatus") != 429 or attempt >= 5:
            sys.stderr.write(proc.stdout)
            sys.stderr.write(proc.stderr)
            raise SystemExit(proc.returncode)
        time.sleep(cooldown_delay(error_data) + 0.2)
    assert proc is not None
    if output is not None:
        output.write_text(proc.stdout, encoding="utf-8")
    try:
        return json.loads(proc.stdout)
    except json.JSONDecodeError:
        return None


def assert_ok(name: str, data: dict) -> None:
    assert data.get("httpStatus") == 200, (name, data)
    assert data.get("success", True) is not False, (name, data)


def wait_for_next_available(data: dict | None) -> None:
    """Honor the server's configured deploy cooldown without hard-coded sleeps."""
    delay = cooldown_delay(data)
    if delay > 0:
        time.sleep(delay + 0.2)


def main() -> None:
    # Keep concurrent smoke processes from publishing to the same site code.
    code = f"skill-smoke-{int(time.time())}-{os.getpid()}"
    with tempfile.TemporaryDirectory() as tmp:
        root = pathlib.Path(tmp)
        env = os.environ.copy()
        env["HOSTCTL_SESSION_FILE"] = str(root / "session.json")
        env["HOSTCTL_PROJECTS_FILE"] = str(root / "projects.json")
        env["HOSTCTL_AGENT_FILE"] = str(root / "agent.json")
        env["HOSTCTL_CONFIG_FILE"] = str(root / "config.json")
        site = root / "site"
        site_v2 = root / "site-v2"
        site.mkdir()
        site_v2.mkdir()
        (site / "index.html").write_text(
            '<!doctype html><html><head><meta charset="utf-8"><title>Skill Smoke</title></head><body><h1>v1</h1></body></html>',
            encoding="utf-8",
        )
        (site_v2 / "index.html").write_text(
            '<!doctype html><html><head><meta charset="utf-8"><title>Skill Smoke</title></head><body><h1>v2</h1></body></html>',
            encoding="utf-8",
        )

        doctor = run("doctor", env=env)
        assert doctor and doctor["success"], doctor

        deploy = run("deploy", str(site), "--code", code, "--title", "技能冒烟页面", "--description", "Skill smoke test version one.", env=env)
        assert_ok("deploy", deploy or {})

        wait_for_next_available(deploy)

        append = run("append", code, str(site_v2), "--title", "技能冒烟页面新版", "--description", "Skill smoke test version two.", env=env)
        assert_ok("append", append or {})

        versions = run("versions", code, env=env)
        assert_ok("versions", versions or {})
        assert versions and versions["code"] == code
        assert len(versions["versions"]) >= 2

        current = run("current", code, "1", env=env)
        assert_ok("current", current or {})

        locked = run("lock", code, "1", env=env)
        assert_ok("lock", locked or {})

        show = run("market", "show", code, env=env)
        assert_ok("market show", show or {})

        if env.get("PAGEPILOT_TOKEN") or env.get("HOSTCTL_TOKEN"):
            content_path = root / "content.json"
            content = run("get", code, "--output", str(content_path), env=env)
            assert content is None
            content_data = json.loads(content_path.read_text(encoding="utf-8"))
            assert content_data["code"] == code
        else:
            proc = subprocess.run(
                [sys.executable, str(SCRIPT), "--server", SERVER, "get", code],
                cwd=ROOT,
                text=True,
                capture_output=True,
                check=False,
                env=env,
            )
            assert proc.returncode != 0, proc.stdout
            denied = json.loads(proc.stdout)
            assert denied.get("httpStatus") == 401, denied
            assert denied.get("errorCode") == "UNAUTHORIZED", denied

        print("skill smoke ok:", code)


if __name__ == "__main__":
    main()
