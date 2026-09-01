#!/usr/bin/env bash
set -euo pipefail

SERVER="${HOSTCTL_SERVER:-http://127.0.0.1:8787}"
SCRIPT="skill/hostctl-deploy/scripts/hostctl_deploy.py"
# Include the process id so parallel smoke runs cannot reuse the same site code.
CODE="skill-smoke-$(date +%s)-$$"
TMP_DIR="$(mktemp -d)"
PYTHON_BIN="${PYTHON:-python3}"

# Keep the smoke run hermetic: it should not consume or overwrite a developer's
# saved anonymous session, project map, agent identity, or CLI configuration.
export HOSTCTL_SESSION_FILE="$TMP_DIR/session.json"
export HOSTCTL_PROJECTS_FILE="$TMP_DIR/projects.json"
export HOSTCTL_AGENT_FILE="$TMP_DIR/agent.json"
export HOSTCTL_CONFIG_FILE="$TMP_DIR/config.json"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

run_skill_with_cooldown() {
  local output_file="$1"
  shift
  local attempt=0 status retry_delay
  while true; do
    set +e
    "$PYTHON_BIN" "$SCRIPT" --server "$SERVER" "$@" > "$output_file"
    status=$?
    set -e
    if [[ "$status" -eq 0 ]]; then
      return 0
    fi
    retry_delay="$("$PYTHON_BIN" - "$output_file" <<'PY'
import json
import pathlib
import sys
from datetime import datetime, timezone

try:
    data = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
except (OSError, json.JSONDecodeError):
    data = {}
if data.get("httpStatus") != 429:
    raise SystemExit(1)
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
print(max(0.2, delay) + 0.2)
PY
    )" || retry_delay=""
    if [[ -z "$retry_delay" || "$attempt" -ge 5 ]]; then
      cat "$output_file" >&2
      return "$status"
    fi
    sleep "$retry_delay"
    attempt=$((attempt + 1))
  done
}

mkdir -p "$TMP_DIR/site" "$TMP_DIR/site-v2"
cat > "$TMP_DIR/site/index.html" <<'HTML'
<!doctype html><html><head><meta charset="utf-8"><title>Skill Smoke</title></head><body><h1>v1</h1></body></html>
HTML
cat > "$TMP_DIR/site-v2/index.html" <<'HTML'
<!doctype html><html><head><meta charset="utf-8"><title>Skill Smoke</title></head><body><h1>v2</h1></body></html>
HTML

"$PYTHON_BIN" "$SCRIPT" --server "$SERVER" doctor
run_skill_with_cooldown "$TMP_DIR/deploy.json" deploy "$TMP_DIR/site" --code "$CODE" --title "技能冒烟页面" --description "Skill smoke test version one."
"$PYTHON_BIN" - "$TMP_DIR/deploy.json" <<'PY'
import datetime
import json
import pathlib
import sys
import time

data = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
delay = float(data.get("retryAfterSeconds") or 0)
raw = str(data.get("nextAvailableAt") or "").strip()
if raw:
    try:
        available = datetime.datetime.fromisoformat(raw.replace("Z", "+00:00"))
        if available.tzinfo is None:
            available = available.replace(tzinfo=datetime.timezone.utc)
        delay = max(delay, (available - datetime.datetime.now(datetime.timezone.utc)).total_seconds())
    except ValueError:
        pass
if delay > 0:
    time.sleep(delay + 0.2)
PY
run_skill_with_cooldown "$TMP_DIR/append.json" append "$CODE" "$TMP_DIR/site-v2" --title "技能冒烟页面新版" --description "Skill smoke test version two."
"$PYTHON_BIN" "$SCRIPT" --server "$SERVER" versions "$CODE" > "$TMP_DIR/versions.json"
"$PYTHON_BIN" "$SCRIPT" --server "$SERVER" current "$CODE" 1 > "$TMP_DIR/current.json"
"$PYTHON_BIN" "$SCRIPT" --server "$SERVER" lock "$CODE" 1 > "$TMP_DIR/lock.json"
"$PYTHON_BIN" "$SCRIPT" --server "$SERVER" market show "$CODE" > "$TMP_DIR/show.json"
if [[ -n "${PAGEPILOT_TOKEN:-}${HOSTCTL_TOKEN:-}" ]]; then
  "$PYTHON_BIN" "$SCRIPT" --server "$SERVER" get "$CODE" --output "$TMP_DIR/content.json"
else
  # Anonymous owners can manage their site but cannot read source content.
  set +e
  "$PYTHON_BIN" "$SCRIPT" --server "$SERVER" get "$CODE" > "$TMP_DIR/content-denied.json"
  get_status=$?
  set -e
  "$PYTHON_BIN" - "$TMP_DIR/content-denied.json" "$get_status" <<'PY'
import json
import pathlib
import sys

data = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
assert int(sys.argv[2]) != 0, data
assert data.get("httpStatus") == 401, data
assert data.get("errorCode") == "UNAUTHORIZED", data
PY
fi

"$PYTHON_BIN" - "$TMP_DIR" "$CODE" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
code = sys.argv[2]
for name in ["deploy", "append", "versions", "current", "lock", "show"]:
    data = json.loads((root / f"{name}.json").read_text(encoding="utf-8"))
    assert data.get("httpStatus") == 200, (name, data)
    assert data.get("success", True) is not False, (name, data)
versions = json.loads((root / "versions.json").read_text(encoding="utf-8"))
assert versions["code"] == code
assert len(versions["versions"]) >= 2
if (root / "content.json").exists():
    content = json.loads((root / "content.json").read_text(encoding="utf-8"))
    assert content["code"] == code
print("skill smoke ok:", code)
PY
