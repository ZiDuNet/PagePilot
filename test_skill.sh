#!/usr/bin/env bash
set -euo pipefail

SERVER="${HOSTCTL_SERVER:-http://127.0.0.1:8787}"
SCRIPT="skill/hostctl-deploy/scripts/hostctl_deploy.py"
CODE="skill-smoke-$(date +%s)"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

mkdir -p "$TMP_DIR/site" "$TMP_DIR/site-v2"
cat > "$TMP_DIR/site/index.html" <<'HTML'
<!doctype html><html><head><meta charset="utf-8"><title>Skill Smoke</title></head><body><h1>v1</h1></body></html>
HTML
cat > "$TMP_DIR/site-v2/index.html" <<'HTML'
<!doctype html><html><head><meta charset="utf-8"><title>Skill Smoke</title></head><body><h1>v2</h1></body></html>
HTML

python "$SCRIPT" --server "$SERVER" doctor
python "$SCRIPT" --server "$SERVER" deploy "$TMP_DIR/site" --code "$CODE" --title "技能冒烟页面" --description "Skill smoke test version one." > "$TMP_DIR/deploy.json"
sleep 2
python "$SCRIPT" --server "$SERVER" append "$CODE" "$TMP_DIR/site-v2" --title "技能冒烟页面新版" --description "Skill smoke test version two." > "$TMP_DIR/append.json"
python "$SCRIPT" --server "$SERVER" versions "$CODE" > "$TMP_DIR/versions.json"
python "$SCRIPT" --server "$SERVER" current "$CODE" 1 > "$TMP_DIR/current.json"
python "$SCRIPT" --server "$SERVER" lock "$CODE" 1 > "$TMP_DIR/lock.json"
python "$SCRIPT" --server "$SERVER" market show "$CODE" > "$TMP_DIR/show.json"
python "$SCRIPT" --server "$SERVER" get "$CODE" --output "$TMP_DIR/content.json"

python - "$TMP_DIR" "$CODE" <<'PY'
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
content = json.loads((root / "content.json").read_text(encoding="utf-8"))
assert content["code"] == code
print("skill smoke ok:", code)
PY
