#!/usr/bin/env python3
"""Stdlib-only hostctl API wrapper for agents.

Commands cover deploy/version operations, marketplace browsing, admin session,
token management, site administration, and production readiness checks.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import pathlib
import platform
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

UA = "hostctl-deploy-skill/1.1"
DEFAULT_SERVER = os.environ.get("HOSTCTL_SERVER", "http://localhost:8787")
SESSION_FILE = pathlib.Path(os.environ.get("HOSTCTL_SESSION_FILE", pathlib.Path.home() / ".hostctl" / "session.json"))
CONFIG_FILE = pathlib.Path(os.environ.get("HOSTCTL_CONFIG_FILE", pathlib.Path.home() / ".hostctl" / "config.json"))
PROJECTS_FILE = pathlib.Path(os.environ.get("HOSTCTL_PROJECTS_FILE", pathlib.Path.home() / ".hostctl" / "projects.json"))
AGENT_FILE = pathlib.Path(os.environ.get("HOSTCTL_AGENT_FILE", pathlib.Path.home() / ".hostctl" / "agent.json"))

ALLOWED_BINARY_EXT = {
    "png", "jpg", "jpeg", "gif", "webp", "svg", "ico",
    "woff", "woff2", "ttf", "otf", "eot", "mp3", "mp4", "webm", "pdf",
}
MAX_SINGLE_FILE_BYTES = 1024 * 1024
MAX_SITE_TOTAL_BYTES = 10 * 1024 * 1024
MAX_FILES_PER_SITE = 100


def server_url(args) -> str:
    return (args.server or DEFAULT_SERVER).rstrip("/")


def default_agent_label() -> str:
    host = platform.node() or "agent"
    user = os.environ.get("USERNAME") or os.environ.get("USER") or ""
    return f"{user}@{host}" if user else host


def load_agent_identity(label_hint: str = "") -> dict:
    env_agent_id = os.environ.get("HOSTCTL_AGENT_ID", "").strip()
    env_label = os.environ.get("HOSTCTL_AGENT_LABEL", "").strip()
    try:
        data = json.loads(AGENT_FILE.read_text(encoding="utf-8"))
        if not isinstance(data, dict):
            data = {}
    except Exception:
        data = {}
    changed = False
    if env_agent_id:
        data["agentId"] = env_agent_id
    if not data.get("agentId"):
        data["agentId"] = "agent_" + uuid.uuid4().hex
        changed = True
    label = label_hint.strip() or env_label or str(data.get("agentLabel") or "").strip() or default_agent_label()
    if data.get("agentLabel") != label:
        data["agentLabel"] = label
        changed = True
    if changed or not AGENT_FILE.exists():
        AGENT_FILE.parent.mkdir(parents=True, exist_ok=True)
        AGENT_FILE.write_text(json.dumps(data, indent=2, ensure_ascii=False), encoding="utf-8")
    return {"agentId": str(data["agentId"]), "agentLabel": str(data["agentLabel"])}


def auth_token(args) -> str:
    if args.token:
        return args.token
    if os.environ.get("HOSTCTL_TOKEN"):
        return os.environ["HOSTCTL_TOKEN"]
    try:
        data = json.loads(CONFIG_FILE.read_text(encoding="utf-8"))
        if data.get("server") == server_url(args):
            return str(data.get("token") or "")
    except Exception:
        pass
    return ""


def save_bound_token(base: str, token: str, username: str, token_id: str, agent: dict | None = None) -> None:
    CONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "server": base,
        "token": token,
        "username": username,
        "tokenId": token_id,
    }
    if agent:
        payload.update(agent)
    CONFIG_FILE.write_text(json.dumps(payload, indent=2, ensure_ascii=False), encoding="utf-8")


def project_key(source_arg: str) -> str:
    try:
        return str(pathlib.Path(source_arg).resolve())
    except Exception:
        return source_arg


def load_projects() -> dict:
    try:
        data = json.loads(PROJECTS_FILE.read_text(encoding="utf-8"))
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def remember_project(base: str, source_arg: str, code: str) -> None:
    if not code:
        return
    data = load_projects()
    data[base + "|" + project_key(source_arg)] = {"server": base, "source": project_key(source_arg), "code": code}
    PROJECTS_FILE.parent.mkdir(parents=True, exist_ok=True)
    PROJECTS_FILE.write_text(json.dumps(data, indent=2, ensure_ascii=False), encoding="utf-8")


def remembered_code(base: str, source_arg: str) -> str:
    data = load_projects()
    item = data.get(base + "|" + project_key(source_arg))
    if isinstance(item, dict):
        return str(item.get("code") or "")
    return ""


def load_session_id(base: str) -> str:
    env = os.environ.get("HOSTCTL_SESSION", "").strip()
    if env:
        return env
    try:
        data = json.loads(SESSION_FILE.read_text(encoding="utf-8"))
    except Exception:
        return ""
    if data.get("server") == base:
        return str(data.get("sessionId") or "")
    return ""


def save_session_id(base: str, session_id: str) -> None:
    SESSION_FILE.parent.mkdir(parents=True, exist_ok=True)
    SESSION_FILE.write_text(json.dumps({"server": base, "sessionId": session_id}, indent=2), encoding="utf-8")


def ensure_session(base: str) -> str:
    sid = load_session_id(base)
    if sid:
        return sid
    status, data = request_json(base, "", "/api/session")
    if 200 <= status < 300 and data.get("sessionId"):
        sid = data["sessionId"]
        save_session_id(base, sid)
        return sid
    die("Could not create anonymous session: " + json.dumps({"httpStatus": status, **data}, ensure_ascii=False))


def request_json(base: str, token: str, path: str, method: str = "GET",
                 payload: dict | None = None, session_id: str = "", agent: dict | None = None) -> tuple[int, dict]:
    data = None
    headers = {"User-Agent": UA, "Accept": "application/json"}
    agent = agent or load_agent_identity()
    if agent.get("agentId"):
        headers["X-Hostctl-Agent-Id"] = str(agent["agentId"])
    if agent.get("agentLabel"):
        headers["X-Hostctl-Agent-Label"] = str(agent["agentLabel"])
    if token:
        headers["Authorization"] = "Bearer " + token
    elif session_id:
        headers["X-Hostctl-Session"] = session_id
    if payload is not None:
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            body = resp.read().decode("utf-8")
            return resp.status, json.loads(body) if body else {}
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", "replace")
        try:
            parsed = json.loads(body)
        except Exception:
            parsed = {"success": False, "error": body}
        return e.code, parsed
    except urllib.error.URLError as e:
        return 0, {"success": False, "errorCode": "NETWORK_ERROR", "detail": str(e)}


def print_result(status: int, data: dict) -> int:
    print(json.dumps({"httpStatus": status, **data}, ensure_ascii=False, indent=2))
    if data.get("stage") == "anonymous_quota":
        print("Anonymous free quota is used up. Register or sign in, create/use a user token, then run:", file=sys.stderr)
        print("  hostctl_deploy.py claim-session", file=sys.stderr)
    if status == 429 and data.get("retryAfterSeconds"):
        print(f"Retry after {data['retryAfterSeconds']} seconds.", file=sys.stderr)
    if data.get("preserveHint"):
        print(f"Preserve hint: {data['preserveHint']}", file=sys.stderr)
    return 0 if 200 <= status < 300 and data.get("success", True) is not False else 1


def request_write(args, path: str, method: str, payload: dict | None = None) -> tuple[int, dict]:
    base = server_url(args)
    token = auth_token(args)
    sid = "" if token else ensure_session(base)
    return request_json(base, token, path, method, payload, sid)


def die(msg: str) -> None:
    raise SystemExit(msg)


def looks_binary(data: bytes) -> bool:
    if not data:
        return False
    sample = data[:512]
    if b"\x00" in sample:
        return True
    nonprint = sum(1 for c in sample if c < 0x09 or (0x0d < c < 0x20))
    return nonprint * 8 > len(sample)


def rel_path(root: pathlib.Path, p: pathlib.Path) -> str:
    rel = p.relative_to(root).as_posix()
    if rel.startswith("/") or ".." in rel.split("/"):
        die(f"Refusing unsafe path: {rel}")
    return rel


def read_source(source_arg: str) -> tuple[list[dict], str]:
    root = pathlib.Path(source_arg)
    if not root.exists():
        die(f"Source not found: {source_arg}")
    if root.is_file():
        data = root.read_bytes()
        if len(data) > MAX_SINGLE_FILE_BYTES:
            die(f"File too large ({len(data)} bytes); limit is {MAX_SINGLE_FILE_BYTES}.")
        entry = root.name
        if looks_binary(data):
            return [{"path": entry, "contentBase64": base64.b64encode(data).decode("ascii")}], entry
        return [{"path": entry, "content": data.decode("utf-8", "replace")}], entry

    files_payload: list[dict] = []
    total_size = 0
    main_entry = ""
    walked = sorted(p for p in root.rglob("*") if p.is_file())
    if len(walked) > MAX_FILES_PER_SITE:
        die(f"Too many files ({len(walked)}); limit is {MAX_FILES_PER_SITE}.")
    for p in walked:
        rel = rel_path(root, p)
        data = p.read_bytes()
        if len(data) > MAX_SINGLE_FILE_BYTES:
            die(f"File too large: {rel} ({len(data)} bytes); limit is {MAX_SINGLE_FILE_BYTES}.")
        total_size += len(data)
        if total_size > MAX_SITE_TOTAL_BYTES:
            die(f"Site total exceeds {MAX_SITE_TOTAL_BYTES} bytes; aborting at {rel}.")
        if looks_binary(data) or p.suffix.lstrip(".").lower() in ALLOWED_BINARY_EXT:
            files_payload.append({"path": rel, "contentBase64": base64.b64encode(data).decode("ascii")})
        else:
            files_payload.append({"path": rel, "content": data.decode("utf-8", "replace")})
        if not main_entry:
            main_entry = rel
        if rel == "index.html":
            main_entry = rel
    return files_payload, main_entry or "index.html"


def ensure_description(args) -> None:
    if not getattr(args, "description", None):
        die("--description is required (one concise sentence, max 240 chars).")
    if len(args.description) > 240:
        die("--description must be at most 240 characters.")


def cmd_doctor(args) -> int:
    base = server_url(args)
    token = auth_token(args)
    agent = load_agent_identity()
    report = {"success": True, "server": base, "agent": agent, "checks": []}

    def check(name: str, path: str, required: bool = True, use_token: bool = False):
        status, data = request_json(base, token if use_token else "", path)
        ok = 200 <= status < 300 and data.get("success", True) is not False
        report["checks"].append({"name": name, "ok": ok, "httpStatus": status, "data": data})
        if required and not ok:
            report["success"] = False
        return status, data, ok

    check("health", "/api/health")
    _, config_data, config_ok = check("config", "/api/config")
    mode = config_data.get("mode") if config_ok else "unknown"
    report["mode"] = mode
    if mode == "prod" or args.require_admin:
        _, _, ok = check("admin_session", "/api/admin/session", required=True, use_token=True)
        if not token:
            report["success"] = False
            report["hint"] = "Set HOSTCTL_TOKEN or pass --token with an admin token."
        elif not ok:
            report["hint"] = "The token is missing, invalid, revoked, or not an admin token."
    else:
        check("admin_session", "/api/admin/session", required=False, use_token=bool(token))
        status, data = request_json(base, "", "/api/session", session_id=load_session_id(base))
        report["checks"].append({"name": "anonymous_session", "ok": 200 <= status < 300, "httpStatus": status, "data": data})
    status, data = request_json(base, "", "/openapi.json")
    report["checks"].append({"name": "openapi", "ok": status == 200 and data.get("openapi") != "", "httpStatus": status})
    if status != 200:
        report["success"] = False
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if report["success"] else 1


def cmd_session(args) -> int:
    status, data = request_json(server_url(args), auth_token(args), "/api/admin/session")
    return print_result(status, data)


def cmd_claim_session(args) -> int:
    base = server_url(args)
    sid = args.session_id or load_session_id(base)
    if not sid:
        die("No anonymous session found. Pass --session-id or deploy anonymously once first.")
    payload = {"sessionId": sid}
    status, data = request_json(base, auth_token(args), "/api/session/claim", "POST", payload)
    return print_result(status, data)


def cmd_deploy(args) -> int:
    ensure_description(args)
    base = server_url(args)
    code = args.code or remembered_code(base, args.source)
    files, main_entry = read_source(args.source)
    payload = {"description": args.description, "filename": args.filename or main_entry, "files": files}
    if args.title:
        payload["title"] = args.title
    if getattr(args, "access_password", ""):
        payload["accessPassword"] = args.access_password
    if code:
        payload["enableCustomCode"] = True
        payload["customCode"] = code
        payload["createVersion"] = True
        if not args.code:
            print(f"Using remembered project code {code}; appending a new version.", file=sys.stderr)
    elif getattr(args, "update", False):
        die("This looks like an update but no project code is known. Ask the user for the original code or URL, then pass --code.")
    status, data = request_write(args, "/api/deploy", "POST", payload)
    if 200 <= status < 300 and data.get("code"):
        remember_project(base, args.source, data["code"])
    return print_result(status, data)


def cmd_append(args) -> int:
    ensure_description(args)
    files, main_entry = read_source(args.source)
    payload = {
        "description": args.description,
        "filename": args.filename or main_entry,
        "files": files,
        "enableCustomCode": True,
        "customCode": args.code,
        "createVersion": True,
    }
    if args.title:
        payload["title"] = args.title
    status, data = request_write(args, "/api/deploy", "POST", payload)
    if 200 <= status < 300 and data.get("code"):
        remember_project(server_url(args), args.source, data["code"])
    return print_result(status, data)


def cmd_versions(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    status, data = request_json(server_url(args), auth_token(args), f"/api/deploys/{code}/versions")
    return print_result(status, data)


def cmd_get(args) -> int:
    base = server_url(args)
    query = {"code": args.code}
    if args.version:
        query["version"] = args.version
    if args.download:
        query["download"] = "1"
    qs = urllib.parse.urlencode(query)
    if not args.download:
        status, data = request_json(base, auth_token(args), f"/api/deploy/content?{qs}")
        if args.output and 200 <= status < 300:
            pathlib.Path(args.output).write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
            return 0
        return print_result(status, data)

    url = f"{base}/api/deploy/content?{qs}"
    headers = {"User-Agent": UA, "Accept": "application/json,text/html,application/zip,*/*"}
    token = auth_token(args)
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read()
    except urllib.error.HTTPError as e:
        sys.stderr.write(e.read().decode("utf-8", "replace"))
        return 1
    if args.output:
        pathlib.Path(args.output).write_bytes(body)
        print(f"Saved {len(body)} bytes to {args.output}")
    else:
        try:
            sys.stdout.write(body.decode("utf-8"))
        except UnicodeDecodeError:
            sys.stdout.buffer.write(body)
    return 0


def _ensure_unlocked(args, action: str) -> None:
    base = server_url(args)
    code = urllib.parse.quote(args.code, safe="")
    status, data = request_json(base, auth_token(args), f"/api/deploys/{code}/versions")
    if not (200 <= status < 300) or not data.get("success"):
        die("Could not inspect versions before " + action + ": " +
            json.dumps({"httpStatus": status, **data}, ensure_ascii=False))
    for item in data.get("versions", []):
        if str(item.get("versionNumber")) == str(args.version) or str(item.get("id")) == str(args.version):
            if item.get("isLocked") or (item.get("likeCount") or 0) > 0:
                die(f"Refusing to {action} {args.code} v{item.get('versionNumber')}: locked/liked. Append a new version instead.")
            return
    die(f"Version {args.version!r} not found for code {args.code!r}.")


def cmd_overwrite(args) -> int:
    ensure_description(args)
    _ensure_unlocked(args, "overwrite")
    files, main_entry = read_source(args.source)
    payload = {"description": args.description, "filename": args.filename or main_entry, "files": files}
    if args.title:
        payload["title"] = args.title
    code = urllib.parse.quote(args.code, safe="")
    version = urllib.parse.quote(str(args.version), safe="")
    status, data = request_write(args, f"/api/deploys/{code}/versions/{version}", "PATCH", payload)
    return print_result(status, data)


def cmd_status(args) -> int:
    _ensure_unlocked(args, f"set status={args.status} for")
    code = urllib.parse.quote(args.code, safe="")
    version = urllib.parse.quote(str(args.version), safe="")
    status, data = request_write(args, f"/api/deploys/{code}/versions/{version}", "PATCH", {"status": args.status})
    return print_result(status, data)


def cmd_current(args) -> int:
    payload: dict
    try:
        payload = {"versionNumber": int(args.version)}
    except ValueError:
        payload = {"versionId": args.version}
    code = urllib.parse.quote(args.code, safe="")
    status, data = request_write(args, f"/api/deploys/{code}/current", "PATCH", payload)
    return print_result(status, data)


def cmd_delete_version(args) -> int:
    _ensure_unlocked(args, "delete")
    code = urllib.parse.quote(args.code, safe="")
    version = urllib.parse.quote(str(args.version), safe="")
    status, data = request_write(args, f"/api/deploys/{code}/versions/{version}", "DELETE")
    return print_result(status, data)


def cmd_lock(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    version = urllib.parse.quote(str(args.version), safe="")
    status, data = request_write(args, f"/api/deploys/{code}/versions/{version}/lock", "POST", {"locked": not args.unlock})
    return print_result(status, data)


def cmd_market_search(args) -> int:
    qs_map = {}
    if args.query:
        qs_map["q"] = args.query
    if args.sort:
        qs_map["sort"] = args.sort
    if args.page:
        qs_map["page"] = str(args.page)
    if args.page_size:
        qs_map["pageSize"] = str(args.page_size)
    qs = urllib.parse.urlencode(qs_map)
    status, data = request_json(server_url(args), "", "/api/deploys" + (("?" + qs) if qs else ""))
    return print_result(status, data)


def cmd_market_show(args) -> int:
    pid = urllib.parse.quote(args.public_id, safe="")
    status, data = request_json(server_url(args), "", f"/api/deploys/{pid}")
    return print_result(status, data)


def cmd_like(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    status, data = request_json(server_url(args), "", f"/api/deploys/{code}/like", "POST", {})
    return print_result(status, data)


def cmd_strategy(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    payload = {"primaryVersionStrategy": args.strategy}
    status, data = request_write(args, f"/api/deploys/{code}/primary-strategy", "PATCH", payload)
    return print_result(status, data)


def cmd_access(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    password = "" if args.clear else (args.password or "")
    if not args.clear and len(password.strip()) < 4:
        die("--password must be at least 4 characters, or pass --clear to remove protection.")
    status, data = request_write(args, f"/api/deploys/{code}/access", "PATCH", {"password": password})
    return print_result(status, data)


def cmd_token_create(args) -> int:
    payload = {"label": args.label or "", "isAdmin": args.admin}
    if args.expires_at:
        payload["expiresAt"] = args.expires_at
    if args.ttl_seconds is not None:
        payload["ttlSeconds"] = args.ttl_seconds
    status, data = request_json(server_url(args), auth_token(args), "/api/token", "POST", payload)
    return print_result(status, data)


def cmd_token_list(args) -> int:
    status, data = request_json(server_url(args), auth_token(args), "/api/tokens")
    return print_result(status, data)


def cmd_token_revoke(args) -> int:
    tid = urllib.parse.quote(args.id, safe="")
    status, data = request_json(server_url(args), auth_token(args), f"/api/tokens/{tid}", "DELETE")
    return print_result(status, data)


def cmd_admin_sites(args) -> int:
    status, data = request_json(server_url(args), auth_token(args), "/api/admin/sites")
    return print_result(status, data)


def cmd_admin_delete_site(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    status, data = request_json(server_url(args), auth_token(args), f"/api/admin/sites/{code}", "DELETE")
    return print_result(status, data)


def cmd_admin_pin_site(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    payload = {"pinned": not args.unpin}
    status, data = request_json(
        server_url(args),
        auth_token(args),
        f"/api/admin/sites/{code}/pin",
        "PATCH",
        payload,
    )
    return print_result(status, data)


def cmd_config_get(args) -> int:
    status, data = request_json(server_url(args), auth_token(args), "/api/config")
    return print_result(status, data)


def cmd_config_set_base(args) -> int:
    status, data = request_json(server_url(args), auth_token(args), "/api/config", "PUT", {"publicBaseURL": args.public_base_url})
    return print_result(status, data)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Deploy and manage static sites on a hostctl server")
    parser.add_argument("--server", help="hostctl server URL (default: $HOSTCTL_SERVER or http://localhost:8787)")
    parser.add_argument("--token", help="bearer token (default: $HOSTCTL_TOKEN)")
    sub = parser.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("doctor", help="Check health, config, OpenAPI, and admin auth readiness")
    p.add_argument("--require-admin", action="store_true", help="Fail unless an admin session validates")
    p.set_defaults(func=cmd_doctor)

    p = sub.add_parser("session", help="Validate current token against /api/admin/session")
    p.set_defaults(func=cmd_session)

    p = sub.add_parser("claim-session", help="Claim anonymous-session deployments for the current token/user")
    p.add_argument("--session-id", default="", help="Anonymous session id. Defaults to ~/.hostctl/session.json")
    p.set_defaults(func=cmd_claim_session)

    def add_common_deploy_flags(p, *, with_code: bool, with_create_version: bool):
        p.add_argument("source", help="Path to an HTML file or a site directory")
        p.add_argument("--description", "-d", required=True, help="Required concise description, max 240 chars")
        p.add_argument("--title", "-t", help="Optional site/version title")
        p.add_argument("--filename", "-f", help="Main entry filename (default: source or index.html)")
        if with_code:
            p.add_argument("--code", "-c", help="Stable custom short code. If it exists, deploy appends a new version.")
            p.add_argument("--update", action="store_true", help="Require updating an existing remembered/explicit code; refuse to create a new link.")
        if with_create_version:
            p.add_argument("--create-version", action="store_true", help="Deprecated: deploy now appends automatically when --code is present")
        p.add_argument("--access-password", help="Optional visit password for a new site.")

    p = sub.add_parser("deploy", help="Deploy a new site from a file or directory")
    add_common_deploy_flags(p, with_code=True, with_create_version=True)
    p.set_defaults(func=cmd_deploy)

    p = sub.add_parser("append", help="Append a new version to an existing stable code")
    p.add_argument("code")
    add_common_deploy_flags(p, with_code=False, with_create_version=False)
    p.set_defaults(func=cmd_append)

    p = sub.add_parser("versions", help="List version history for a code")
    p.add_argument("code")
    p.set_defaults(func=cmd_versions)

    p = sub.add_parser("get", help="Fetch metadata or download a version")
    p.add_argument("code")
    p.add_argument("--version")
    p.add_argument("--download", action="store_true")
    p.add_argument("--output", "-o")
    p.set_defaults(func=cmd_get)

    p = sub.add_parser("overwrite", help="Overwrite one unlocked version")
    p.add_argument("code")
    p.add_argument("version")
    add_common_deploy_flags(p, with_code=False, with_create_version=False)
    p.set_defaults(func=cmd_overwrite)

    p = sub.add_parser("status", help="Publish or unpublish one unlocked version")
    p.add_argument("code")
    p.add_argument("version")
    p.add_argument("status", choices=["active", "inactive"])
    p.set_defaults(func=cmd_status)

    p = sub.add_parser("current", help="Switch the public current version")
    p.add_argument("code")
    p.add_argument("version")
    p.set_defaults(func=cmd_current)

    p = sub.add_parser("delete-version", help="Delete one unlocked version")
    p.add_argument("code")
    p.add_argument("version")
    p.set_defaults(func=cmd_delete_version)

    p = sub.add_parser("lock", help="Lock a version; pass --unlock to reverse")
    p.add_argument("code")
    p.add_argument("version")
    p.add_argument("--unlock", action="store_true")
    p.set_defaults(func=cmd_lock)

    p_market = sub.add_parser("market", help="Browse public marketplace")
    market_sub = p_market.add_subparsers(dest="market_cmd", required=True)
    pm = market_sub.add_parser("search", help="Search/browse deploys")
    pm.add_argument("query", nargs="?")
    pm.add_argument("--sort", default="newest", help="newest, oldest, likes_desc, views_desc")
    pm.add_argument("--page", type=int, default=1)
    pm.add_argument("--page-size", type=int, default=24)
    pm.set_defaults(func=cmd_market_search)
    pm = market_sub.add_parser("show", help="Show one deploy")
    pm.add_argument("public_id")
    pm.set_defaults(func=cmd_market_show)

    p = sub.add_parser("like", help="Like a deploy for marketplace ranking")
    p.add_argument("code")
    p.set_defaults(func=cmd_like)

    p = sub.add_parser("strategy", help="Set primary version strategy")
    p.add_argument("code")
    p.add_argument("strategy", choices=["likes", "latest"])
    p.set_defaults(func=cmd_strategy)

    p = sub.add_parser("access", help="Set or clear a site's visit password")
    p.add_argument("code")
    p.add_argument("--password", default="", help="Visit password, at least 4 characters")
    p.add_argument("--clear", action="store_true", help="Clear the visit password and make the site public")
    p.set_defaults(func=cmd_access)

    p_token = sub.add_parser("token", help="Manage bearer tokens")
    token_sub = p_token.add_subparsers(dest="token_cmd", required=True)
    pt = token_sub.add_parser("create", help="Create a token")
    pt.add_argument("--label", default="")
    pt.add_argument("--admin", action="store_true")
    pt.add_argument("--expires-at", default="", help="RFC3339 expiry timestamp. Omit for permanent.")
    pt.add_argument("--ttl-seconds", type=int, help="Temporary token lifetime in seconds. Omit for permanent.")
    pt.set_defaults(func=cmd_token_create)
    pt = token_sub.add_parser("list", help="List tokens")
    pt.set_defaults(func=cmd_token_list)
    pt = token_sub.add_parser("revoke", help="Revoke a token")
    pt.add_argument("id")
    pt.set_defaults(func=cmd_token_revoke)

    p_admin = sub.add_parser("admin", help="Admin site operations")
    admin_sub = p_admin.add_subparsers(dest="admin_cmd", required=True)
    pa = admin_sub.add_parser("sites", help="List all sites")
    pa.set_defaults(func=cmd_admin_sites)
    pa = admin_sub.add_parser("delete-site", help="Delete a whole site")
    pa.add_argument("code")
    pa.set_defaults(func=cmd_admin_delete_site)
    pa = admin_sub.add_parser("pin-site", help="Pin or unpin a marketplace site")
    pa.add_argument("code")
    pa.add_argument("--unpin", action="store_true", help="Clear the marketplace pin")
    pa.set_defaults(func=cmd_admin_pin_site)

    p_config = sub.add_parser("config", help="Read or update runtime config")
    config_sub = p_config.add_subparsers(dest="config_cmd", required=True)
    pc = config_sub.add_parser("get", help="Read runtime config")
    pc.set_defaults(func=cmd_config_get)
    pc = config_sub.add_parser("set-base-url", help="Update publicBaseURL")
    pc.add_argument("public_base_url")
    pc.set_defaults(func=cmd_config_set_base)

    return parser


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()
    if not args.server:
        args.server = DEFAULT_SERVER
    raise SystemExit(args.func(args))


if __name__ == "__main__":
    main()
