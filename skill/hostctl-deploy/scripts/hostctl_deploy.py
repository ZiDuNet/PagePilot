#!/usr/bin/env python3
"""PagePilot pagep Skill CLI wrapper.

Commands cover deploy/version operations, PagePilot creation market browsing, admin session,
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
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile

UA = "pagep-skill/1.2"


def env_first(*names: str) -> str:
    for name in names:
        value = os.environ.get(name, "").strip()
        if value:
            return value
    return ""


def state_file(new_env: str, old_env: str, filename: str) -> pathlib.Path:
    configured = env_first(new_env, old_env)
    if configured:
        return pathlib.Path(configured)
    new_path = pathlib.Path.home() / ".pagep" / filename
    old_path = pathlib.Path.home() / ".hostctl" / filename
    if old_path.exists() and not new_path.exists():
        return old_path
    return new_path


DEFAULT_SERVER = env_first("PAGEPILOT_SERVER", "HOSTCTL_SERVER") or "http://localhost:8787"
SESSION_FILE = state_file("PAGEPILOT_SESSION_FILE", "HOSTCTL_SESSION_FILE", "session.json")
CONFIG_FILE = state_file("PAGEPILOT_CONFIG_FILE", "HOSTCTL_CONFIG_FILE", "config.json")
PROJECTS_FILE = state_file("PAGEPILOT_PROJECTS_FILE", "HOSTCTL_PROJECTS_FILE", "projects.json")
AGENT_FILE = state_file("PAGEPILOT_AGENT_FILE", "HOSTCTL_AGENT_FILE", "agent.json")

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
    env_agent_id = env_first("PAGEPILOT_AGENT_ID", "HOSTCTL_AGENT_ID")
    env_label = env_first("PAGEPILOT_AGENT_LABEL", "HOSTCTL_AGENT_LABEL")
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
    token = env_first("PAGEPILOT_TOKEN", "HOSTCTL_TOKEN")
    if token:
        return token
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
    env = env_first("PAGEPILOT_SESSION", "HOSTCTL_SESSION")
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
    headers["X-Hostctl-Current-Origin"] = base
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
        print("  pagep claim-session", file=sys.stderr)
    if status == 429 and data.get("retryAfterSeconds"):
        print(f"Retry after {data['retryAfterSeconds']} seconds.", file=sys.stderr)
    if data.get("preserveHint"):
        print(f"Preserve hint: {data['preserveHint']}", file=sys.stderr)
    return 0 if 200 <= status < 300 and data.get("success", True) is not False else 1


def print_deploy_summary(data: dict, stream=sys.stdout) -> None:
    if not isinstance(data, dict):
        return
    if data.get("success", True) is False:
        return
    has_url = any(data.get(key) for key in ("url", "detailUrl", "versionUrl"))
    if not has_url and not data.get("code"):
        return

    print("PagePilot 发布成功", file=stream)
    for label, key in (
        ("访问 URL", "url"),
        ("详情 URL", "detailUrl"),
        ("版本 URL", "versionUrl"),
    ):
        value = str(data.get(key) or "").strip()
        if value:
            print(f"  {label}: {value}", file=stream)
    code = str(data.get("code") or "").strip()
    if code:
        print(f"  code: {code}", file=stream)
    version = data.get("versionNumber")
    if version:
        print(f"  版本: v{version}", file=stream)

    source_code = str(data.get("templateSourceCode") or "").strip()
    source_version = data.get("templateSourceVersion")
    if source_code:
        suffix = f" v{source_version}" if source_version else ""
        print(f"  复用来源: {source_code}{suffix}", file=stream)
    if data.get("reuseCount") is not None:
        print(f"  复用计数: {data['reuseCount']}", file=stream)
    if data.get("preserveHint"):
        print(f"  提示: {data['preserveHint']}", file=stream)
    if has_url:
        print("  请直接使用服务端返回的链接，不要按本机地址或域名规则自行拼接。", file=stream)
    print("", file=stream)


def request_write(args, path: str, method: str, payload: dict | None = None) -> tuple[int, dict]:
    base = server_url(args)
    token = auth_token(args)
    sid = "" if token else ensure_session(base)
    return request_json(base, token, path, method, payload, sid)


def request_multipart(
    base: str,
    token: str,
    path: str,
    fields: dict,
    source_path: pathlib.Path,
    upload_name: str,
    session_id: str = "",
    agent: dict | None = None,
) -> tuple[int, dict]:
    boundary = "----PagePilotSkill" + uuid.uuid4().hex
    body = bytearray()
    upload_name = safe_multipart_filename(upload_name or source_path.name)

    def add_field(name: str, value) -> None:
        if value is None:
            return
        text = str(value)
        if text.strip() == "":
            return
        body.extend(f"--{boundary}\r\n".encode("utf-8"))
        body.extend(f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode("utf-8"))
        body.extend(text.encode("utf-8"))
        body.extend(b"\r\n")

    for key, value in fields.items():
        add_field(key, value)

    body.extend(f"--{boundary}\r\n".encode("utf-8"))
    body.extend(
        (
            'Content-Disposition: form-data; name="file"; '
            f'filename="{upload_name}"\r\n'
            "Content-Type: application/octet-stream\r\n\r\n"
        ).encode("utf-8")
    )
    body.extend(source_path.read_bytes())
    body.extend(b"\r\n")
    body.extend(f"--{boundary}--\r\n".encode("utf-8"))

    headers = {
        "User-Agent": UA,
        "Accept": "application/json",
        "Content-Type": f"multipart/form-data; boundary={boundary}",
        "X-Hostctl-Current-Origin": base,
    }
    agent = agent or load_agent_identity()
    if agent.get("agentId"):
        headers["X-Hostctl-Agent-Id"] = str(agent["agentId"])
    if agent.get("agentLabel"):
        headers["X-Hostctl-Agent-Label"] = str(agent["agentLabel"])
    if token:
        headers["Authorization"] = "Bearer " + token
    elif session_id:
        headers["X-Hostctl-Session"] = session_id

    req = urllib.request.Request(base + path, data=bytes(body), headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            resp_body = resp.read().decode("utf-8")
            return resp.status, json.loads(resp_body) if resp_body else {}
    except urllib.error.HTTPError as e:
        resp_body = e.read().decode("utf-8", "replace")
        try:
            parsed = json.loads(resp_body)
        except Exception:
            parsed = {"success": False, "error": resp_body}
        return e.code, parsed
    except urllib.error.URLError as e:
        return 0, {"success": False, "errorCode": "NETWORK_ERROR", "detail": str(e)}


def safe_multipart_filename(name: str) -> str:
    cleaned = pathlib.Path(str(name).replace("\\", "/")).name
    cleaned = cleaned.replace("\r", "_").replace("\n", "_").replace('"', "_")
    return cleaned or "site.zip"


def registered_token(args, action: str = "screen command") -> str:
    token = auth_token(args)
    if not token:
        die(f"{action} requires a registered user token. Set PAGEPILOT_TOKEN, pass --token, or save one in {CONFIG_FILE}.")
    return token


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
    readme_entry = ""
    page_entry = ""
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
        lower_rel = rel.lower()
        if lower_rel in ("index.html", "index.htm"):
            main_entry = rel
        elif lower_rel in ("readme.md", "readme.markdown") and not readme_entry:
            readme_entry = rel
        elif (lower_rel.endswith((".html", ".htm", ".md", ".markdown"))) and not page_entry:
            page_entry = rel
    return files_payload, main_entry or readme_entry or page_entry or "index.html"


def choose_main_entry(paths: list[str]) -> str:
    lowered = {p.lower(): p for p in paths}
    for preferred in ("index.html", "index.htm", "readme.md", "readme.markdown"):
        if preferred in lowered:
            return lowered[preferred]
    for p in paths:
        if p.lower().endswith((".html", ".htm", ".md", ".markdown")):
            return p
    return "index.html"


def zip_entry_names(zip_path: pathlib.Path) -> list[str]:
    try:
        with zipfile.ZipFile(zip_path) as zf:
            return sorted(name for name in zf.namelist() if not name.endswith("/"))
    except zipfile.BadZipFile:
        return []


def source_entry_hint(source_arg: str) -> str:
    root = pathlib.Path(source_arg)
    if not root.exists():
        die(f"Source not found: {source_arg}")
    if root.is_file():
        if root.suffix.lower() == ".zip":
            return choose_main_entry(zip_entry_names(root))
        return root.name
    paths = sorted(rel_path(root, p) for p in root.rglob("*") if p.is_file())
    if len(paths) > MAX_FILES_PER_SITE:
        die(f"Too many files ({len(paths)}); limit is {MAX_FILES_PER_SITE}.")
    return choose_main_entry(paths)


def prepare_multipart_source(source_arg: str) -> tuple[pathlib.Path, str, callable]:
    root = pathlib.Path(source_arg)
    if not root.exists():
        die(f"Source not found: {source_arg}")
    if root.is_file():
        if root.stat().st_size > MAX_SITE_TOTAL_BYTES:
            die(f"Source too large ({root.stat().st_size} bytes); limit is {MAX_SITE_TOTAL_BYTES}.")
        return root, root.name, lambda: None

    fd, temp_name = tempfile.mkstemp(prefix="pagepilot-", suffix=".zip")
    os.close(fd)
    temp_path = pathlib.Path(temp_name)

    def cleanup() -> None:
        try:
            temp_path.unlink()
        except FileNotFoundError:
            pass

    total_size = 0
    walked = sorted(p for p in root.rglob("*") if p.is_file())
    if len(walked) > MAX_FILES_PER_SITE:
        cleanup()
        die(f"Too many files ({len(walked)}); limit is {MAX_FILES_PER_SITE}.")
    try:
        with zipfile.ZipFile(temp_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            for p in walked:
                rel = rel_path(root, p)
                size = p.stat().st_size
                if size > MAX_SINGLE_FILE_BYTES:
                    cleanup()
                    die(f"File too large: {rel} ({size} bytes); limit is {MAX_SINGLE_FILE_BYTES}.")
                total_size += size
                if total_size > MAX_SITE_TOTAL_BYTES:
                    cleanup()
                    die(f"Site total exceeds {MAX_SITE_TOTAL_BYTES} bytes; aborting at {rel}.")
                zf.write(p, rel)
    except Exception:
        cleanup()
        raise
    return temp_path, pathlib.Path(source_arg).name + ".zip", cleanup


def deploy_multipart(args, fields: dict, source_arg: str) -> tuple[int, dict]:
    base = server_url(args)
    token = auth_token(args)
    sid = "" if token else ensure_session(base)
    source_path, upload_name, cleanup = prepare_multipart_source(source_arg)
    try:
        return request_multipart(base, token, "/api/deploy", fields, source_path, upload_name, sid)
    finally:
        cleanup()


def ensure_description(args) -> None:
    if not getattr(args, "description", None):
        die("--description is required (one concise sentence, max 240 chars).")
    if len(args.description) > 240:
        die("--description must be at most 240 characters.")


def ensure_title(args) -> None:
    title = str(getattr(args, "title", "") or "").strip()
    if not title:
        die("--title is required. Use a meaningful Chinese display name, not a filename.")
    lowered = title.lower()
    generic = {"index", "index.html", "index.htm", "untitled", "demo", "test", "page", "app"}
    if lowered in generic or lowered.endswith(".html") or lowered.endswith(".htm"):
        die("--title must be a meaningful display name, not index.html, demo, test, or a filename.")
    if not any("\u4e00" <= ch <= "\u9fff" for ch in title):
        die("--title must contain a meaningful Chinese name for the PagePilot listing.")


def add_deploy_options(payload: dict, args) -> None:
    if getattr(args, "title", ""):
        payload["title"] = args.title
    if getattr(args, "visibility", ""):
        payload["visibility"] = args.visibility
    if getattr(args, "category", "") and not getattr(args, "create_version", False):
        payload["category"] = args.category
    if getattr(args, "access_password", ""):
        payload["accessPassword"] = args.access_password
    if getattr(args, "template_source_code", ""):
        payload["templateSourceCode"] = args.template_source_code
    if getattr(args, "template_source_version", None):
        payload["templateSourceVersion"] = int(args.template_source_version)


def apply_access_password_after_deploy(args, base: str, token: str, code: str) -> None:
    password = getattr(args, "access_password", "") or ""
    if not password:
        return
    status, data = request_json(
        base,
        token or auth_token(args),
        f"/api/deploys/{urllib.parse.quote(code, safe='')}/access",
        "PATCH",
        {"password": password},
        "" if token or auth_token(args) else load_session_id(base),
    )
    if not (200 <= status < 300 and data.get("success", True) is not False):
        die("Deploy succeeded but setting access password failed: " +
            json.dumps({"httpStatus": status, **data}, ensure_ascii=False))


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
            report["hint"] = "Set PAGEPILOT_TOKEN or pass --token with an admin token."
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
    ensure_title(args)
    base = server_url(args)
    token = auth_token(args)
    code = args.code or remembered_code(base, args.source)
    main_entry = source_entry_hint(args.source)
    payload = {"description": args.description, "filename": args.filename or main_entry}
    add_deploy_options(payload, args)
    if code:
        payload["enableCustomCode"] = True
        payload["customCode"] = code
        payload["createVersion"] = True
        if not args.code:
            print(f"Using remembered project code {code}; appending a new version.", file=sys.stderr)
    elif getattr(args, "update", False):
        die("This looks like an update but no project code is known. Ask the user for the original code or URL, then pass --code.")
    status, data = deploy_multipart(args, payload, args.source)
    if 200 <= status < 300 and data.get("code"):
        remember_project(base, args.source, data["code"])
        apply_access_password_after_deploy(args, base, token, str(data["code"]))
        print_deploy_summary(data)
    return print_result(status, data)


def cmd_append(args) -> int:
    ensure_description(args)
    ensure_title(args)
    base = server_url(args)
    token = auth_token(args)
    main_entry = source_entry_hint(args.source)
    payload = {
        "description": args.description,
        "filename": args.filename or main_entry,
        "enableCustomCode": True,
        "customCode": args.code,
        "createVersion": True,
    }
    add_deploy_options(payload, args)
    status, data = deploy_multipart(args, payload, args.source)
    if 200 <= status < 300 and data.get("code"):
        remember_project(base, args.source, data["code"])
        apply_access_password_after_deploy(args, base, token, str(data["code"]))
        print_deploy_summary(data)
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
    ensure_title(args)
    _ensure_unlocked(args, "overwrite")
    files, main_entry = read_source(args.source)
    payload = {"description": args.description, "filename": args.filename or main_entry, "files": files}
    add_deploy_options(payload, args)
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
    if getattr(args, "category", ""):
        qs_map["category"] = args.category
    if getattr(args, "kind", ""):
        qs_map["kind"] = args.kind
    if args.page:
        qs_map["page"] = str(args.page)
    if args.page_size:
        qs_map["pageSize"] = str(args.page_size)
    qs = urllib.parse.urlencode(qs_map)
    status, data = request_json(server_url(args), "", "/api/deploys" + (("?" + qs) if qs else ""))
    return print_result(status, data)


def cmd_market_categories(args) -> int:
    status, data = request_json(server_url(args), "", "/api/market/categories")
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


def cmd_admin_site_detail(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    status, data = request_json(server_url(args), auth_token(args), f"/api/admin/sites/{code}")
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


def cmd_admin_audit_logs(args) -> int:
    params = {
        "actorType": args.actor_type,
        "actorId": args.actor_id,
        "actorRole": args.actor_role,
        "action": args.action,
        "result": args.result,
        "siteCode": args.site_code,
        "targetType": args.target_type,
        "targetId": args.target_id,
        "q": args.query,
        "since": args.since,
        "until": args.until,
        "page": args.page,
        "pageSize": args.page_size,
    }
    query = urllib.parse.urlencode({k: v for k, v in params.items() if v not in (None, "")})
    path = "/api/admin/audit-logs" + (f"?{query}" if query else "")
    status, data = request_json(server_url(args), auth_token(args), path)
    return print_result(status, data)


def cmd_admin_reuse_policy(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    payload = {
        "reusePolicy": args.reuse,
        "sourceDownloadPolicy": args.source_download,
    }
    status, data = request_json(
        server_url(args),
        auth_token(args),
        f"/api/admin/sites/{code}/reuse-policy",
        "PATCH",
        payload,
    )
    return print_result(status, data)


def cmd_admin_security_mode(args) -> int:
    code = urllib.parse.quote(args.code, safe="")
    payload = {"securityMode": args.mode}
    status, data = request_json(
        server_url(args),
        auth_token(args),
        f"/api/admin/sites/{code}/security-mode",
        "PATCH",
        payload,
    )
    return print_result(status, data)


def cmd_config_get(args) -> int:
    status, data = request_json(server_url(args), auth_token(args), "/api/config")
    return print_result(status, data)


def cmd_config_set_app_url(args) -> int:
    payload = {
        "appURLMode": args.mode,
        "appDomainSuffix": args.domain_suffix,
        "appURLScheme": args.scheme,
        "appURLPort": args.port,
    }
    status, data = request_json(server_url(args), auth_token(args), "/api/config", "PUT", payload)
    return print_result(status, data)


def cmd_screen_list(args) -> int:
    token = registered_token(args, "screen list")
    status, data = request_json(server_url(args), token, "/api/screens")
    return print_result(status, data)


def screen_orientation(screen: dict) -> str:
    info = screen.get("deviceInfo") if isinstance(screen, dict) else {}
    if not isinstance(info, dict):
        info = {}
    raw = str(info.get("orientation") or screen.get("orientation") or "").strip().lower()
    if raw in {"landscape", "horizontal", "横屏"}:
        return "landscape"
    if raw in {"portrait", "vertical", "竖屏"}:
        return "portrait"
    width = info.get("screenWidthPx", info.get("width"))
    height = info.get("screenHeightPx", info.get("height"))
    try:
        w = float(width)
        h = float(height)
    except (TypeError, ValueError):
        return ""
    if w > h:
        return "landscape"
    if h > w:
        return "portrait"
    return ""


def screen_resolution(screen: dict) -> str:
    info = screen.get("deviceInfo") if isinstance(screen, dict) else {}
    if not isinstance(info, dict):
        info = {}
    width = info.get("screenWidthPx", info.get("width"))
    height = info.get("screenHeightPx", info.get("height"))
    if width and height:
        return f"{width}x{height}"
    return str(info.get("resolution") or "").strip()


def orientation_check_result(screen: dict, expected: str) -> tuple[bool, str]:
    expected = (expected or "any").strip().lower()
    if expected in {"", "any"}:
        return True, ""
    actual = screen_orientation(screen)
    if not actual:
        return True, "Target screen did not report orientation; cannot validate expected orientation."
    if actual == expected:
        return True, ""
    name = screen.get("name") or screen.get("id") or "target screen"
    resolution = screen_resolution(screen)
    suffix = f" ({resolution})" if resolution else ""
    return False, (
        f"Orientation mismatch: app is expected to be {expected}, but screen "
        f"{name} is {actual}{suffix}. Confirm the layout or pass --force-orientation."
    )


def load_target_screen(base: str, token: str, screen_id: str) -> dict:
    status, data = request_json(base, token, "/api/screens")
    if not (200 <= status < 300):
        die("Could not inspect target screen before publishing: " +
            json.dumps({"httpStatus": status, **data}, ensure_ascii=False))
    for item in data.get("screens", []):
        if str(item.get("id") or "") == screen_id:
            return item
    die(f"Target screen not found or not owned by current user: {screen_id}")
    return {}


def ensure_screen_orientation(args, base: str, token: str) -> None:
    expected = str(getattr(args, "expected_orientation", "any") or "any").strip().lower()
    if expected in {"", "any"}:
        return
    screen = load_target_screen(base, token, args.screen)
    ok, message = orientation_check_result(screen, expected)
    if ok:
        if message:
            print(message, file=sys.stderr)
        return
    if getattr(args, "force_orientation", False):
        print(message + " Continuing because --force-orientation was provided.", file=sys.stderr)
        return
    die(message)


def cmd_screen_bind(args) -> int:
    token = registered_token(args, "screen bind")
    payload = {"pairingCode": args.pairing_code}
    if args.name:
        payload["name"] = args.name
    status, data = request_json(server_url(args), token, "/api/screens/bind", "POST", payload)
    return print_result(status, data)


def cmd_screen_publish(args) -> int:
    token = registered_token(args, "screen publish")
    base = server_url(args)
    ensure_screen_orientation(args, base, token)
    code = args.app or ""
    if args.source:
        if not args.description:
            die("--description is required when publishing a local path to a screen.")
        ensure_title(args)
        main_entry = source_entry_hint(args.source)
        payload = {"description": args.description, "filename": args.filename or main_entry}
        add_deploy_options(payload, args)
        deploy_status, deploy_data = deploy_multipart(args, payload, args.source)
        if not (200 <= deploy_status < 300 and deploy_data.get("code")):
            return print_result(deploy_status, deploy_data)
        code = str(deploy_data["code"])
        remember_project(base, args.source, code)
        apply_access_password_after_deploy(args, base, token, code)
    if not code:
        die("Pass --app <code> to publish an existing app, or --source <path> to deploy and publish.")
    payload = {"code": code}
    if args.version_number is not None:
        payload["versionNumber"] = args.version_number
    screen_id = urllib.parse.quote(args.screen, safe="")
    status, data = request_json(base, token, f"/api/screens/{screen_id}/publish", "POST", payload)
    return print_result(status, data)


def cmd_screen_status(args) -> int:
    token = registered_token(args, "screen status")
    status, data = request_json(server_url(args), token, "/api/screens")
    if args.screen and 200 <= status < 300:
        screens = [item for item in data.get("screens", []) if item.get("id") == args.screen]
        data = {"success": True, "screens": screens}
    return print_result(status, data)


def cmd_screen_unbind(args) -> int:
    token = registered_token(args, "screen unbind")
    screen_id = urllib.parse.quote(args.screen, safe="")
    status, data = request_json(server_url(args), token, f"/api/screens/{screen_id}", "DELETE")
    return print_result(status, data)


def cmd_screen_command(args) -> int:
    token = registered_token(args, f"screen {args.command}")
    screen_id = urllib.parse.quote(args.screen, safe="")
    payload = {"type": args.command}
    status, data = request_json(server_url(args), token, f"/api/screens/{screen_id}/command", "POST", payload)
    return print_result(status, data)


def fetch_screen_screenshot(base: str, token: str, screen: str, output: str) -> bool:
    screen_id = urllib.parse.quote(screen, safe="")
    headers = {"User-Agent": UA, "Accept": "image/png,image/jpeg,image/webp,*/*", "Authorization": "Bearer " + token}
    req = urllib.request.Request(f"{base}/api/screens/{screen_id}/screenshot?ts={int(time.time() * 1000)}", headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            body = resp.read()
            pathlib.Path(output).write_bytes(body)
            print(json.dumps({
                "success": True,
                "screen": screen,
                "output": output,
                "bytes": len(body),
                "contentType": resp.headers.get("Content-Type", ""),
            }, ensure_ascii=False, indent=2))
            return True
    except urllib.error.HTTPError:
        return False


def cmd_screen_screenshot(args) -> int:
    token = registered_token(args, "screen screenshot")
    base = server_url(args)
    screen_id = urllib.parse.quote(args.screen, safe="")
    status, data = request_json(base, token, f"/api/screens/{screen_id}/screenshot", "POST")
    if not (200 <= status < 300 and data.get("success", True) is not False):
        return print_result(status, data)
    if not args.output:
        return print_result(status, data)
    deadline = time.time() + max(0, args.timeout)
    while time.time() <= deadline:
        if fetch_screen_screenshot(base, token, args.screen, args.output):
            return 0
        time.sleep(2)
    print(json.dumps({"httpStatus": status, **data}, ensure_ascii=False, indent=2))
    print(f"Screenshot command was sent, but no image was available within {args.timeout} seconds.", file=sys.stderr)
    return 1


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Deploy and manage PagePilot static apps")
    parser.add_argument("--server", help="PagePilot server URL (default: $PAGEPILOT_SERVER or http://localhost:8787)")
    parser.add_argument("--token", help="bearer token (default: $PAGEPILOT_TOKEN)")
    sub = parser.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("doctor", help="Check health, config, OpenAPI, and admin auth readiness")
    p.add_argument("--require-admin", action="store_true", help="Fail unless an admin session validates")
    p.set_defaults(func=cmd_doctor)

    p = sub.add_parser("session", help="Validate current token against /api/admin/session")
    p.set_defaults(func=cmd_session)

    p = sub.add_parser("claim-session", help="Claim anonymous-session deployments for the current token/user")
    p.add_argument("--session-id", default="", help="Anonymous session id. Defaults to ~/.pagep/session.json")
    p.set_defaults(func=cmd_claim_session)

    def add_common_deploy_flags(p, *, with_code: bool, with_create_version: bool):
        p.add_argument("source", help="Path to an HTML file, Markdown file, website ZIP, or site directory")
        p.add_argument("--description", "-d", required=True, help="Required concise description, max 240 chars")
        p.add_argument("--title", "-t", required=True, help="Required meaningful Chinese site/version title")
        p.add_argument("--filename", "-f", help="Main entry filename (default: source or index.html)")
        if with_code:
            p.add_argument("--code", "-c", help="Stable custom short code. If it exists, deploy appends a new version.")
            p.add_argument("--update", action="store_true", help="Require updating an existing remembered/explicit code; refuse to create a new link.")
        if with_create_version:
            p.add_argument("--create-version", action="store_true", help="Deprecated: deploy now appends automatically when --code is present")
        p.add_argument("--visibility", choices=["public", "unlisted"], default="", help="public 进入 PagePilot 创作市场；unlisted 仅链接访问")
        p.add_argument("--category", default="", help="新站点的创作市场分类 slug，例如 landing/dashboard/docs/tool/game/screen")
        p.add_argument("--access-password", help="Optional visit password. Existing codes are updated after deploy.")
        p.add_argument("--template-source-code", default="", help="复用创作市场作品时传入原作品 code，用于记录来源和复用计数")
        p.add_argument("--template-source-version", type=int, help="复用创作市场作品时传入原作品版本号")

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

    p_market = sub.add_parser("market", help="浏览 PagePilot 创作市场")
    market_sub = p_market.add_subparsers(dest="market_cmd", required=True)
    pm = market_sub.add_parser("search", help="Search/browse deploys")
    pm.add_argument("query", nargs="?")
    pm.add_argument("--sort", default="newest", help="hot, newest, featured, oldest, likes_desc, views_desc")
    pm.add_argument("--category", default="", help="market category slug, e.g. landing/dashboard/docs/tool/game/screen")
    pm.add_argument("--kind", default="", help="derived filter: html, md, protected, featured, mine")
    pm.add_argument("--page", type=int, default=1)
    pm.add_argument("--page-size", type=int, default=24)
    pm.set_defaults(func=cmd_market_search)
    pm = market_sub.add_parser("categories", help="List market categories")
    pm.set_defaults(func=cmd_market_categories)
    pm = market_sub.add_parser("show", help="Show one deploy")
    pm.add_argument("public_id")
    pm.set_defaults(func=cmd_market_show)

    p = sub.add_parser("like", help="为创作市场作品点赞并影响排序")
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
    pa = admin_sub.add_parser("site-detail", help="查看站点详情、Bundle、文件树和复用参数")
    pa.add_argument("code")
    pa.set_defaults(func=cmd_admin_site_detail)
    pa = admin_sub.add_parser("audit-logs", help="查询管理员审计日志")
    pa.add_argument("--actor-type", default="")
    pa.add_argument("--actor-id", default="")
    pa.add_argument("--actor-role", default="")
    pa.add_argument("--action", default="")
    pa.add_argument("--result", default="")
    pa.add_argument("--site-code", default="")
    pa.add_argument("--target-type", default="")
    pa.add_argument("--target-id", default="")
    pa.add_argument("--query", "-q", default="")
    pa.add_argument("--since", default="", help="RFC3339 起始时间")
    pa.add_argument("--until", default="", help="RFC3339 截止时间")
    pa.add_argument("--page", type=int, default=1)
    pa.add_argument("--page-size", type=int, default=50)
    pa.set_defaults(func=cmd_admin_audit_logs)
    pa = admin_sub.add_parser("delete-site", help="Delete a whole site")
    pa.add_argument("code")
    pa.set_defaults(func=cmd_admin_delete_site)
    pa = admin_sub.add_parser("pin-site", help="置顶或取消置顶创作市场作品")
    pa.add_argument("code")
    pa.add_argument("--unpin", action="store_true", help="取消创作市场置顶")
    pa.set_defaults(func=cmd_admin_pin_site)
    pa = admin_sub.add_parser("reuse-policy", help="设置源码下载和模板复用策略")
    pa.add_argument("code")
    pa.add_argument("--reuse", choices=["auto", "allow", "deny"], default="auto", help="模板复用策略")
    pa.add_argument("--source-download", choices=["auto", "allow", "deny"], default="auto", help="源码下载策略")
    pa.set_defaults(func=cmd_admin_reuse_policy)
    pa = admin_sub.add_parser("security-mode", help="设置站点运行安全模式")
    pa.add_argument("code")
    pa.add_argument("--mode", choices=["auto", "strict", "compatible", "trusted"], default="auto", help="运行安全模式")
    pa.set_defaults(func=cmd_admin_security_mode)

    p_config = sub.add_parser("config", help="Read or update runtime config")
    config_sub = p_config.add_subparsers(dest="config_cmd", required=True)
    pc = config_sub.add_parser("get", help="Read runtime config")
    pc.set_defaults(func=cmd_config_get)
    pc = config_sub.add_parser("set-app-url", help="Update hosted app URL mode and wildcard domain settings")
    pc.add_argument("--mode", choices=["path", "domain", "dual"], required=True, help="path keeps /agent/{code}; domain uses {code}.suffix; dual enables both")
    pc.add_argument("--domain-suffix", default="", help="Wildcard app host suffix, e.g. apps.pagepilot.example.com")
    pc.add_argument("--scheme", choices=["https", "http"], default="https")
    pc.add_argument("--port", default="", help="Optional external app URL port, e.g. 1143")
    pc.set_defaults(func=cmd_config_set_app_url)

    p_screen = sub.add_parser("screen", help="Manage and publish to hardware screens (registered users only)")
    screen_sub = p_screen.add_subparsers(dest="screen_cmd", required=True)
    ps = screen_sub.add_parser("list", help="List screens bound to the current registered user")
    ps.set_defaults(func=cmd_screen_list)
    ps = screen_sub.add_parser("bind", help="Bind a screen using its short pairing code")
    ps.add_argument("pairing_code")
    ps.add_argument("--name", default="", help="Optional display name for the screen")
    ps.set_defaults(func=cmd_screen_bind)
    ps = screen_sub.add_parser("publish", help="Publish an app or local HTML project to a screen")
    ps.add_argument("--screen", required=True, help="Target screen id")
    ps.add_argument("--app", default="", help="Existing PagePilot app code")
    ps.add_argument("--source", default="", help="Optional local HTML file or site directory to deploy before publishing")
    ps.add_argument("--description", "-d", default="", help="Required when --source is used")
    ps.add_argument("--title", "-t", default="", help="Required meaningful Chinese title when --source is used")
    ps.add_argument("--filename", "-f", default="", help="Main entry filename when --source is used")
    ps.add_argument("--visibility", choices=["public", "unlisted"], default="", help="Visibility for --source deploy")
    ps.add_argument("--access-password", default="", help="Optional visit password for --source deploy")
    ps.add_argument("--version-number", type=int, help="Optional version number for an existing app")
    ps.add_argument(
        "--expected-orientation",
        choices=["any", "portrait", "landscape"],
        default="any",
        help="Expected app layout direction; blocks mismatched target screens unless --force-orientation is set",
    )
    ps.add_argument("--force-orientation", action="store_true", help="Publish even if expected orientation does not match the target screen")
    ps.set_defaults(func=cmd_screen_publish)
    ps = screen_sub.add_parser("screenshot", help="Request a device screenshot and optionally save the returned image")
    ps.add_argument("screen")
    ps.add_argument("--output", "-o", default="", help="Save latest screenshot to this image file after requesting")
    ps.add_argument("--timeout", type=int, default=30, help="Seconds to wait when --output is set")
    ps.set_defaults(func=cmd_screen_screenshot)
    for command, help_text in [
        ("refresh", "Refresh the screen WebView"),
        ("sleep", "Put the screen into black-screen standby"),
        ("wake", "Wake the screen and resume playback"),
        ("shutdown", "Request soft shutdown or black-screen standby"),
    ]:
        ps = screen_sub.add_parser(command, help=help_text)
        ps.add_argument("screen")
        ps.set_defaults(func=cmd_screen_command, command=command)
    ps = screen_sub.add_parser("status", help="Show bound screen status")
    ps.add_argument("screen", nargs="?", default="", help="Optional screen id to filter")
    ps.set_defaults(func=cmd_screen_status)
    ps = screen_sub.add_parser("unbind", help="Unbind a screen")
    ps.add_argument("screen")
    ps.set_defaults(func=cmd_screen_unbind)

    return parser


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()
    if not args.server:
        args.server = DEFAULT_SERVER
    raise SystemExit(args.func(args))


if __name__ == "__main__":
    main()
