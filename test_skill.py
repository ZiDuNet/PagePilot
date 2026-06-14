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


ROOT = pathlib.Path(__file__).resolve().parent
SCRIPT = ROOT / "skill" / "hostctl-deploy" / "scripts" / "hostctl_deploy.py"
SERVER = os.environ.get("HOSTCTL_SERVER", "http://127.0.0.1:8787")


def run(*args: str, output: pathlib.Path | None = None, env: dict[str, str] | None = None) -> dict | None:
    cmd = [sys.executable, str(SCRIPT), "--server", SERVER, *args]
    proc = subprocess.run(cmd, cwd=ROOT, text=True, capture_output=True, check=False, env=env)
    if proc.returncode != 0:
        sys.stderr.write(proc.stdout)
        sys.stderr.write(proc.stderr)
        raise SystemExit(proc.returncode)
    if output is not None:
        output.write_text(proc.stdout, encoding="utf-8")
    try:
        return json.loads(proc.stdout)
    except json.JSONDecodeError:
        return None


def assert_ok(name: str, data: dict) -> None:
    assert data.get("httpStatus") == 200, (name, data)
    assert data.get("success", True) is not False, (name, data)


def main() -> None:
    code = f"skill-smoke-{int(time.time())}"
    with tempfile.TemporaryDirectory() as tmp:
        root = pathlib.Path(tmp)
        env = os.environ.copy()
        env["HOSTCTL_SESSION_FILE"] = str(root / "session.json")
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

        deploy = run("deploy", str(site), "--code", code, "--description", "Skill smoke test version one.", env=env)
        assert_ok("deploy", deploy or {})

        time.sleep(2)

        append = run("append", code, str(site_v2), "--description", "Skill smoke test version two.", env=env)
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

        content_path = root / "content.json"
        content = run("get", code, "--output", str(content_path), env=env)
        assert content is None
        content_data = json.loads(content_path.read_text(encoding="utf-8"))
        assert content_data["code"] == code

        print("skill smoke ok:", code)


if __name__ == "__main__":
    main()
