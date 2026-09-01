#!/usr/bin/env python3
"""PagePilot pagep Skill CLI wrapper.

Commands cover deploy/version operations, PagePilot creation market browsing, admin session,
token management, site administration, and production readiness checks.
"""
from __future__ import annotations

import argparse
import io
import json
import os
import pathlib
import platform
import re
import stat
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile
from contextlib import redirect_stderr

SKILL_VERSION = "0.3.1"
UA = f"pagep-skill/{SKILL_VERSION}"
JSON_OUTPUT = False


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


FALLBACK_SERVER = "https://pagepilot.dell.4dbim.cc:1143/"
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

PREFERRED_PAGE_ENTRIES = (
    "index.html", "index.htm", "README.md", "readme.md",
    "README.markdown", "readme.markdown",
)
TEXT_FILE_EXTENSIONS = (
    ".html", ".htm", ".css", ".js", ".mjs", ".json", ".txt",
    ".md", ".markdown", ".svg", ".xml", ".csv", ".webmanifest", ".map",
)


def normalize_server(value: str) -> str:
    return (value or "").strip().rstrip("/")


def load_local_config() -> dict:
    try:
        data = json.loads(CONFIG_FILE.read_text(encoding="utf-8"))
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def save_local_config(payload: dict) -> None:
    CONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(
        str(CONFIG_FILE),
        os.O_WRONLY | os.O_CREAT | os.O_TRUNC,
        0o600,
    )
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(payload, f, indent=2, ensure_ascii=False)
    except Exception:
        try:
            os.close(fd)
        except OSError:
            pass
        raise
    if hasattr(os, "chmod"):
        try:
            os.chmod(CONFIG_FILE, 0o600)
        except OSError:
            pass


def default_server_url() -> str:
    configured = normalize_server(env_first("PAGEPILOT_SERVER", "HOSTCTL_SERVER"))
    if configured:
        return configured
    saved = normalize_server(str(load_local_config().get("server") or ""))
    if saved:
        return saved
    return FALLBACK_SERVER


def server_url(args) -> str:
    explicit = normalize_server(str(getattr(args, "server", "") or ""))
    if explicit:
        return explicit
    return default_server_url()


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
    if getattr(args, "token", ""):
        return args.token
    token = env_first("PAGEPILOT_TOKEN", "HOSTCTL_TOKEN")
    if token:
        return token
    data = load_local_config()
    saved_token = str(data.get("token") or "")
    saved_server = normalize_server(str(data.get("server") or ""))
    if saved_token and (not saved_server or saved_server == server_url(args)):
        return saved_token
    return ""


def save_bound_token(base: str, token: str, username: str, token_id: str, agent: dict | None = None) -> None:
    payload = {
        "server": base,
        "token": token,
        "username": username,
        "tokenId": token_id,
    }
    if agent:
        payload.update(agent)
    # P1：统一通过 0o600 写入，避免 Bearer Token 被宽权限文件泄露。
    save_local_config(payload)


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
    fd, temp_name = tempfile.mkstemp(prefix=".session-", dir=str(SESSION_FILE.parent))
    try:
        if hasattr(os, "fchmod"):
            os.fchmod(fd, 0o600)
        else:
            os.chmod(temp_name, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump({"server": base, "sessionId": session_id}, handle, indent=2)
            handle.write("\n")
        os.replace(temp_name, SESSION_FILE)
    except Exception:
        try:
            os.close(fd)
        except OSError:
            pass
        try:
            os.unlink(temp_name)
        except FileNotFoundError:
            pass
        raise


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


def structured_http_error(status: int, body: str, fallback: str = "") -> dict:
    """Keep CLI/API errors machine-readable when an upstream returns non-JSON."""
    raw_detail = str(body or "").strip()
    try:
        parsed = json.loads(body) if body else {}
    except (TypeError, json.JSONDecodeError):
        parsed = {}
    if not isinstance(parsed, dict):
        parsed = {}

    data = dict(parsed)
    data["success"] = False
    if not str(data.get("errorCode") or "").strip():
        data["errorCode"] = "HTTP_ERROR"
    detail = str(data.get("detail") or data.get("error") or raw_detail or fallback or f"HTTP {status} request failed").strip()
    data["detail"] = detail
    if not str(data.get("hint") or "").strip():
        data["hint"] = "Check the PagePilot server URL and reverse-proxy response, then retry."
    return data


def structured_invalid_response(status: int, body: str) -> dict:
    detail = str(body or "").strip() or f"HTTP {status} returned an empty response"
    return {
        "success": False,
        "errorCode": "INVALID_RESPONSE",
        "detail": detail,
        "hint": "Check the PagePilot server URL and reverse-proxy response, then retry.",
    }


def network_error_payload(detail: str) -> dict:
    return {
        "success": False,
        "errorCode": "NETWORK_ERROR",
        "detail": str(detail),
        "hint": "Check network connectivity and the PagePilot server URL, then retry.",
    }


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
            body = resp.read().decode("utf-8", "replace")
            if not body.strip():
                return resp.status, structured_invalid_response(resp.status, body)
            try:
                parsed = json.loads(body)
            except json.JSONDecodeError:
                return resp.status, structured_invalid_response(resp.status, body)
            if not isinstance(parsed, dict):
                return resp.status, structured_invalid_response(resp.status, body)
            return resp.status, parsed
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", "replace")
        return e.code, structured_http_error(e.code, body, str(e))
    except TimeoutError as e:
        return 0, network_error_payload(str(e) or "request timed out")
    except urllib.error.URLError as e:
        return 0, network_error_payload(str(e))


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
    method: str = "POST",
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

    req = urllib.request.Request(base + path, data=bytes(body), headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            resp_body = resp.read().decode("utf-8", "replace")
            if not resp_body.strip():
                return resp.status, structured_invalid_response(resp.status, resp_body)
            try:
                parsed = json.loads(resp_body)
            except json.JSONDecodeError:
                return resp.status, structured_invalid_response(resp.status, resp_body)
            if not isinstance(parsed, dict):
                return resp.status, structured_invalid_response(resp.status, resp_body)
            return resp.status, parsed
    except urllib.error.HTTPError as e:
        resp_body = e.read().decode("utf-8", "replace")
        return e.code, structured_http_error(e.code, resp_body, str(e))
    except TimeoutError as e:
        return 0, network_error_payload(str(e) or "request timed out")
    except urllib.error.URLError as e:
        return 0, network_error_payload(str(e))


def safe_multipart_filename(name: str) -> str:
    cleaned = pathlib.Path(str(name).replace("\\", "/")).name
    cleaned = cleaned.replace("\r", "-").replace("\n", "-").replace('"', "-").strip()
    suffix = pathlib.Path(cleaned).suffix
    if suffix and not re.fullmatch(r"\.[A-Za-z0-9]{1,16}", suffix):
        suffix = ""
    stem = cleaned[: -len(suffix)] if suffix else cleaned
    safe_stem = re.sub(r"[^\w.\-\u4e00-\u9fff]+", "-", stem, flags=re.UNICODE)
    safe_stem = re.sub(r"-+", "-", safe_stem).strip(".-")
    if not safe_stem or re.fullmatch(r"(con|prn|aux|nul|com[1-9]|lpt[1-9])", safe_stem, re.IGNORECASE):
        safe_stem = "upload"
    return safe_stem + suffix.lower()


def registered_token(args, action: str = "screen command") -> str:
    token = auth_token(args)
    if not token:
        die(f"{action} requires a registered user token. Set PAGEPILOT_TOKEN, pass --token, or save one in {CONFIG_FILE}.")
    return token


def die(msg: str) -> None:
    if JSON_OUTPUT:
        print(json.dumps({
            "success": False,
            "errorCode": "CLI_ERROR",
            "detail": str(msg),
            "hint": "Fix the reported input and rerun the command.",
        }, ensure_ascii=False))
        raise SystemExit(1)
    raise SystemExit(msg)


def rel_path(root: pathlib.Path, p: pathlib.Path) -> str:
    rel = str(p.relative_to(root)).replace("\\", "/")
    if not _preflight_is_safe_path(rel):
        die(f"Refusing unsafe path: {rel}")
    return rel


def _preflight_issue(code: str, detail: str, hint: str = "") -> dict:
    issue = {"code": code, "detail": detail}
    if hint:
        issue["hint"] = hint
    return issue


def _preflight_report(source: str) -> dict:
    return {
        "success": False,
        "source": source,
        "sourceType": "unknown",
        "kind": "unknown",
        "files": [],
        "count": 0,
        "bytes": 0,
        "mainEntry": "",
        "root": "",
        "warnings": [],
        "errors": [],
        "limits": {
            "maxSingleFileBytes": MAX_SINGLE_FILE_BYTES,
            "maxSiteTotalBytes": MAX_SITE_TOTAL_BYTES,
            "maxFiles": MAX_FILES_PER_SITE,
        },
    }


def _preflight_is_html_path(path: str) -> bool:
    return path.lower().endswith((".html", ".htm"))


def _preflight_is_markdown_path(path: str) -> bool:
    return path.lower().endswith((".md", ".markdown"))


def _preflight_is_page_entry(path: str) -> bool:
    return _preflight_is_html_path(path) or _preflight_is_markdown_path(path)


def _preflight_is_binary_path(path: str) -> bool:
    return not path.lower().endswith(TEXT_FILE_EXTENSIONS)


def _preflight_is_safe_path(path: str) -> bool:
    value = str(path).replace("\\", "/")
    if not value or "\x00" in value or value.startswith("/") or value.startswith("//"):
        return False
    if len(value.encode("utf-8")) > 255 or "//" in value:
        return False
    if len(value) >= 2 and value[1] == ":":
        return False
    segments = value.split("/")
    if len(segments) > 16:
        return False
    for segment in segments:
        if segment in {"", ".", ".."}:
            return False
        if len(segment) >= 2 and segment[1] == ":":
            return False
        if segment.endswith((".", " ")):
            return False
        if any(not char.isprintable() or char in '<>:"\\|?*' for char in segment):
            return False
        # Windows reserves the device-name prefix even when multiple
        # extensions follow it (for example CON.foo.bar), so split at the
        # first dot.
        dot = segment.find(".")
        stem = segment[:dot] if dot > 0 else segment
        if stem.casefold() in {"con", "prn", "aux", "nul", *(f"com{i}" for i in range(1, 10)), *(f"lpt{i}" for i in range(1, 10))}:
            return False
    return True


def _preflight_normalize_path(path: str) -> str:
    return str(path).replace("\\", "/")


def _zip_datetime(path: pathlib.Path) -> tuple[int, int, int, int, int, int]:
    """Return a ZIP-compatible timestamp, clamping files outside DOS limits."""
    try:
        value = time.localtime(path.stat().st_mtime)
    except (OSError, OverflowError, ValueError):
        return (1980, 1, 1, 0, 0, 0)
    if value.tm_year < 1980:
        return (1980, 1, 1, 0, 0, 0)
    if value.tm_year > 2107:
        return (2107, 12, 31, 23, 59, 58)
    return (value.tm_year, value.tm_mon, value.tm_mday, value.tm_hour, value.tm_min, value.tm_sec - (value.tm_sec % 2))


def _zip_write_file(archive: zipfile.ZipFile, source: pathlib.Path, arcname: str) -> None:
    """Write a regular file without letting an invalid mtime abort ZIP creation."""
    info = zipfile.ZipInfo(arcname, date_time=_zip_datetime(source))
    info.compress_type = zipfile.ZIP_DEFLATED
    info.create_system = 3
    info.external_attr = (stat.S_IFREG | 0o644) << 16
    with source.open("rb") as src, archive.open(info, "w") as dst:
        while True:
            chunk = src.read(1024 * 1024)
            if not chunk:
                break
            dst.write(chunk)


def _preflight_skip_archive_path(path: str) -> bool:
    if not path or path.endswith("/"):
        return True
    base = path.rsplit("/", 1)[-1]
    return path.startswith("__MACOSX/") or base in {".DS_Store", "Thumbs.db"}


def _preflight_path_dir(path: str) -> str:
    if "/" not in path:
        return ""
    return path.rsplit("/", 1)[0].strip("/")


def _preflight_common_root(roots: list[str]) -> tuple[str, bool]:
    if not roots:
        return "", False
    root = roots[0].strip("/")
    return root, all(candidate.strip("/") == root for candidate in roots[1:])


def _preflight_choose_root(records: list[dict], entry_hint: str) -> tuple[str, dict | None]:
    if entry_hint:
        for record in records:
            if record["path"].casefold() == entry_hint.casefold() and _preflight_is_page_entry(record["path"]):
                return _preflight_path_dir(record["path"]), None

    for preferred in PREFERRED_PAGE_ENTRIES:
        candidates = [
            _preflight_path_dir(record["path"])
            for record in records
            if record["path"].rsplit("/", 1)[-1].casefold() == preferred.casefold()
        ]
        if len(candidates) == 1:
            return candidates[0], None
        if len(candidates) > 1:
            root, ok = _preflight_common_root(candidates)
            if ok:
                return root, None
            return "", _preflight_issue(
                "ZIP_AMBIGUOUS_ENTRY",
                f"bundle contains multiple possible {preferred} entries",
                "Package one deployable website root per ZIP, or pass --filename with the intended entry.",
            )

    candidates = [_preflight_path_dir(record["path"]) for record in records if _preflight_is_page_entry(record["path"])]
    if not candidates:
        return "", _preflight_issue(
            "ZIP_ENTRY_MISSING",
            "bundle did not contain an HTML or Markdown entry",
            "Put index.html or README.md in the site folder, or pass --filename with an entry path.",
        )
    if len(candidates) == 1:
        return candidates[0], None
    root, ok = _preflight_common_root(candidates)
    if ok:
        return root, None
    return "", _preflight_issue(
        "ZIP_AMBIGUOUS_ENTRY",
        "bundle contains multiple possible page entries",
        "Package one deployable website root per ZIP, or pass --filename with the intended entry.",
    )


def _preflight_entry_after_root(entry_hint: str, root: str) -> str:
    if not entry_hint or not root:
        return entry_hint
    prefix = root.strip("/") + "/"
    if len(entry_hint) > len(prefix) and entry_hint[:len(prefix)].casefold() == prefix.casefold():
        return entry_hint[len(prefix):]
    return entry_hint


def _preflight_choose_main_entry(records: list[dict], entry_hint: str) -> str:
    if entry_hint and _preflight_is_page_entry(entry_hint):
        for record in records:
            if record["path"] == entry_hint:
                return entry_hint
    for preferred in PREFERRED_PAGE_ENTRIES:
        for record in records:
            if record["path"].casefold() == preferred.casefold():
                return record["path"]
    for record in records:
        if _preflight_is_html_path(record["path"]):
            return record["path"]
    for record in records:
        if _preflight_is_markdown_path(record["path"]):
            return record["path"]
    return ""


def _preflight_read_limited(stream, limit: int = MAX_SINGLE_FILE_BYTES) -> bytes:
    data = bytearray()
    while len(data) <= limit:
        chunk = stream.read(min(64 * 1024, limit + 1 - len(data)))
        if not chunk:
            break
        data.extend(chunk)
    if len(data) > limit:
        raise ValueError("file exceeds configured single-file limit")
    return bytes(data)


def _preflight_content_is_binary(data: bytes) -> bool:
    """Mirror the server's lightweight multipart binary detector for entries."""
    if not data:
        return False
    sample = data[:512]
    non_printable = 0
    for value in sample:
        if value == 0:
            return True
        if value < 0x09 or (value > 0x0D and value < 0x20):
            non_printable += 1
    return non_printable * 8 > len(sample)


def _preflight_entry_text(data: bytes) -> str:
    return data.decode("utf-8", errors="replace").strip()


def _preflight_validate_entry(record: dict, load_bytes, report: dict) -> None:
    try:
        data = load_bytes(record)
    except ValueError:
        report["errors"].append(_preflight_issue(
            "ZIP_FILE_TOO_LARGE",
            f"file {record['path']} exceeds max single-file size ({MAX_SINGLE_FILE_BYTES} bytes)",
            "Split large assets or raise the single-file upload limit in admin settings.",
        ))
        return
    except (OSError, RuntimeError, NotImplementedError, EOFError, zipfile.BadZipFile) as exc:
        report["errors"].append(_preflight_issue(
            "ZIP_ENTRY_READ_FAILED",
            f"could not read main entry {record['path']}: {exc}",
            "Rebuild the source archive and run preflight again.",
        ))
        return

    if _preflight_content_is_binary(data):
        report["errors"].append(_preflight_issue(
            "ZIP_ENTRY_MISSING",
            f"main entry {record['path']} is binary and cannot be rendered as a page",
            "Make the HTML or Markdown entry a UTF-8 text file.",
        ))
        return

    text = _preflight_entry_text(data)
    if _preflight_is_markdown_path(record["path"]):
        if len(text.encode("utf-8")) < 3:
            report["errors"].append(_preflight_issue(
                "INVALID_INPUT",
                f"main Markdown entry {record['path']} is too short",
                "Upload a Markdown document with at least one heading or paragraph.",
            ))
        return

    lower = text.lower()
    if len(text.encode("utf-8")) < 32:
        report["errors"].append(_preflight_issue(
            "INVALID_INPUT",
            f"main HTML entry {record['path']} is too short to be a page",
            "Upload a real HTML file with tags such as <html>, <body>, <main>, <script>, or <style>.",
        ))
        return
    html_tags = (
        "main", "section", "article", "nav", "header", "footer", "div", "p",
        "h1", "h2", "h3", "ul", "ol", "table", "form", "button", "canvas",
        "svg", "script", "style",
    )
    if "<" not in lower or ">" not in lower or not any(f"<{tag}" in lower for tag in html_tags):
        report["errors"].append(_preflight_issue(
            "INVALID_INPUT",
            f"main HTML entry {record['path']} does not look like an HTML page",
            "Plain text is not deployable here. Provide a valid HTML document or generated static site.",
        ))


def _preflight_set_files(report: dict, records: list[dict]) -> None:
    report["files"] = [
        {"path": record["path"], "bytes": record["bytes"], "isBinary": record["isBinary"]}
        for record in sorted(records, key=lambda item: item["path"])
    ]
    report["count"] = len(records)
    report["bytes"] = sum(record["bytes"] for record in records)


def _preflight_measure_directory_archive(source_path: pathlib.Path, report: dict) -> None:
    """Measure the ZIP produced by the multipart directory packer.

    The server validates the uploaded ZIP as one multipart file before expanding
    it, so the compressed archive itself must fit the single-file limit too.
    """
    fd, temp_name = tempfile.mkstemp(prefix="pagepilot-preflight-", suffix=".zip")
    os.close(fd)
    temp_path = pathlib.Path(temp_name)
    try:
        with zipfile.ZipFile(temp_path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            for path in sorted(source_path.rglob("*"), key=lambda item: item.as_posix()):
                if path.is_symlink() or not path.is_file():
                    continue
                rel = path.relative_to(source_path).as_posix()
                if _preflight_skip_archive_path(rel):
                    continue
                _zip_write_file(archive, path, rel)
        archive_size = temp_path.stat().st_size
        report["sourceBytes"] = archive_size
        if archive_size > MAX_SINGLE_FILE_BYTES:
            report["errors"].append(_preflight_issue(
                "ZIP_FILE_TOO_LARGE",
                f"generated site ZIP is {archive_size} bytes; single-file upload limit is {MAX_SINGLE_FILE_BYTES} bytes",
                "Reduce the compressed archive size or raise the single-file upload limit in admin settings.",
            ))
        elif archive_size > MAX_SITE_TOTAL_BYTES:
            report["errors"].append(_preflight_issue(
                "ZIP_TOTAL_TOO_LARGE",
                f"generated site ZIP is {archive_size} bytes; site upload limit is {MAX_SITE_TOTAL_BYTES} bytes",
                "Reduce the source directory before uploading.",
            ))
    except (OSError, ValueError, OverflowError, RuntimeError, NotImplementedError, EOFError, zipfile.BadZipFile) as exc:
        report["errors"].append(_preflight_issue(
            "SOURCE_PREPARE_FAILED",
            f"could not create the upload ZIP: {exc}",
            "Check that every source file is readable, then run preflight again.",
        ))
    finally:
        try:
            temp_path.unlink()
        except FileNotFoundError:
            pass


def _preflight_analyze_records(records: list[dict], source_type: str, entry_hint: str,
                               load_bytes, report: dict) -> None:
    root, root_error = _preflight_choose_root(records, entry_hint)
    if root_error:
        report["errors"].append(root_error)
        _preflight_set_files(report, records)
        return

    selected: list[dict] = []
    seen: set[str] = set()
    total_size = 0
    for record in records:
        path = record["path"]
        if root:
            if not path.startswith(root + "/"):
                continue
            path = path[len(root) + 1:]
        if not path or _preflight_skip_archive_path(path):
            continue
        if not _preflight_is_safe_path(path):
            report["errors"].append(_preflight_issue(
                "ZIP_UNSAFE_PATH",
                f"bundle entry {path!r} is not a safe relative path",
                "Paths must not contain '..', absolute paths, drive letters, or empty path segments.",
            ))
            continue
        copied = dict(record)
        copied["path"] = path
        selected.append(copied)
        if path in seen:
            report["errors"].append(_preflight_issue(
                "ZIP_DUPLICATE_PATH",
                f"duplicate file path after bundle root detection: {path}",
                "Rename duplicate files that collapse to the same relative path after root detection.",
            ))
        seen.add(path)
        total_size += copied["bytes"]

    _preflight_set_files(report, selected)
    report["root"] = root
    if len(selected) > MAX_FILES_PER_SITE:
        report["errors"].append(_preflight_issue(
            "ZIP_TOO_MANY_FILES",
            f"too many files in bundle ({len(selected)}); max {MAX_FILES_PER_SITE} per site",
            "Reduce generated artifacts or raise the file-count limit in admin settings.",
        ))
    if total_size > MAX_SITE_TOTAL_BYTES:
        report["errors"].append(_preflight_issue(
            "ZIP_TOTAL_TOO_LARGE",
            f"total size exceeds site limit ({MAX_SITE_TOTAL_BYTES} bytes)",
            "Remove unused assets or raise the whole-site upload limit in admin settings.",
        ))

    main_hint = _preflight_entry_after_root(entry_hint, root)
    if entry_hint and _preflight_is_page_entry(entry_hint) and not any(
        record["path"] == main_hint for record in selected
    ):
        report["warnings"].append(_preflight_issue(
            "ENTRY_HINT_NOT_FOUND",
            f"--filename {entry_hint!r} did not match a deployable page entry; automatic detection was used.",
        ))
    main_entry = _preflight_choose_main_entry(selected, main_hint)
    if not main_entry:
        report["errors"].append(_preflight_issue(
            "ZIP_ENTRY_MISSING",
            "bundle did not contain an HTML or Markdown entry",
            "Put index.html or README.md in the site folder, or pass --filename with an entry path.",
        ))
        return

    report["mainEntry"] = main_entry
    if _preflight_is_markdown_path(main_entry):
        report["kind"] = "markdown"
    elif source_type in {"directory", "zip"}:
        report["kind"] = "zip_site"
    elif len(selected) <= 1:
        report["kind"] = "single_html"
    else:
        report["kind"] = "static_site"

    for record in selected:
        if record["path"] == main_entry:
            if record["bytes"] <= MAX_SINGLE_FILE_BYTES:
                _preflight_validate_entry(record, load_bytes, report)
            break


def _preflight_directory(source_path: pathlib.Path, entry_hint: str, report: dict) -> None:
    records: list[dict] = []
    try:
        paths = sorted(source_path.rglob("*"), key=lambda item: item.as_posix())
    except OSError as exc:
        report["errors"].append(_preflight_issue("SOURCE_READ_FAILED", f"could not inspect directory: {exc}"))
        return

    for path in paths:
        try:
            if path.is_symlink():
                rel = path.relative_to(source_path).as_posix()
                report["errors"].append(_preflight_issue(
                    "UNSAFE_SYMLINK",
                    f"directory contains symbolic link: {rel}",
                    "Replace symbolic links with files contained inside the source directory.",
                ))
                continue
            if not path.is_file():
                continue
            rel = _preflight_normalize_path(path.relative_to(source_path).as_posix())
            if not _preflight_is_safe_path(rel):
                report["errors"].append(_preflight_issue(
                    "ZIP_UNSAFE_PATH",
                    f"directory entry {rel!r} is not a safe relative path",
                    "Paths must not contain '..', absolute paths, drive letters, or empty path segments.",
                ))
                continue
            if _preflight_skip_archive_path(rel):
                report["warnings"].append(_preflight_issue(
                    "IGNORED_ARCHIVE_METADATA", f"ignored archive metadata: {rel}",
                ))
                continue
            size = path.stat().st_size
        except OSError as exc:
            report["errors"].append(_preflight_issue("SOURCE_READ_FAILED", f"could not inspect {path}: {exc}"))
            continue

        records.append({
            "path": rel,
            "bytes": size,
            "isBinary": _preflight_is_binary_path(rel),
            "_source": path,
        })
        if size > MAX_SINGLE_FILE_BYTES:
            report["errors"].append(_preflight_issue(
                "ZIP_FILE_TOO_LARGE",
                f"file {rel} exceeds max single-file size ({MAX_SINGLE_FILE_BYTES} bytes)",
                "Split large assets or raise the single-file upload limit in admin settings.",
            ))

    if not records and not report["errors"]:
        report["errors"].append(_preflight_issue(
            "ZIP_EMPTY",
            "directory did not contain deployable files",
            "Add index.html, README.md, or a static site folder.",
        ))
        return

    # Directory upload first creates a ZIP locally, so enforce its input limits too.
    # Bundle-root trimming below can legitimately reduce the hosted file set, but it
    # cannot bypass the local packer's limits.
    raw_total = sum(record["bytes"] for record in records)
    if len(records) > MAX_FILES_PER_SITE:
        report["errors"].append(_preflight_issue(
            "SOURCE_TOO_MANY_FILES",
            f"directory contains too many files ({len(records)}); max {MAX_FILES_PER_SITE} for local upload",
            "Remove generated artifacts before publishing this directory.",
        ))
    if raw_total > MAX_SITE_TOTAL_BYTES:
        report["errors"].append(_preflight_issue(
            "SOURCE_TOTAL_TOO_LARGE",
            f"directory totals {raw_total} bytes; local upload limit is {MAX_SITE_TOTAL_BYTES} bytes",
            "Remove unused assets or raise the whole-site upload limit in admin settings.",
        ))
    _preflight_measure_directory_archive(source_path, report)

    def load_bytes(record: dict) -> bytes:
        with pathlib.Path(record["_source"]).open("rb") as stream:
            return _preflight_read_limited(stream)

    _preflight_analyze_records(records, "directory", entry_hint, load_bytes, report)


def _preflight_zip(source_path: pathlib.Path, entry_hint: str, report: dict) -> None:
    try:
        source_size = source_path.stat().st_size
    except OSError as exc:
        report["errors"].append(_preflight_issue("SOURCE_READ_FAILED", f"could not inspect ZIP: {exc}"))
        return
    report["sourceBytes"] = source_size
    if source_size > MAX_SINGLE_FILE_BYTES:
        report["errors"].append(_preflight_issue(
            "ZIP_FILE_TOO_LARGE",
            f"source ZIP is {source_size} bytes; single-file upload limit is {MAX_SINGLE_FILE_BYTES} bytes",
            "Reduce the compressed archive size or raise the single-file upload limit in admin settings.",
        ))
        return
    if source_size > MAX_SITE_TOTAL_BYTES:
        report["errors"].append(_preflight_issue(
            "SOURCE_TOO_LARGE",
            f"source ZIP is {source_size} bytes; upload limit is {MAX_SITE_TOTAL_BYTES} bytes",
            "Reduce the archive size before uploading.",
        ))
        return

    records: list[dict] = []
    try:
        with zipfile.ZipFile(source_path) as archive:
            for index, info in enumerate(archive.infolist()):
                if info.is_dir():
                    continue
                raw_path = info.filename
                if stat.S_ISLNK(info.external_attr >> 16):
                    report["warnings"].append(_preflight_issue(
                        "ZIP_SYMLINK_TREATED_AS_FILE",
                        f"ZIP entry {raw_path!r} is a symbolic link and will be stored as file content",
                        "Replace it with a regular file when the application expects a linked asset.",
                    ))
                if not _preflight_is_safe_path(raw_path):
                    report["errors"].append(_preflight_issue(
                        "ZIP_UNSAFE_PATH",
                        f"ZIP entry {raw_path!r} is not a safe relative path",
                        "ZIP entries must not contain '..', absolute paths, drive letters, UNC paths, or empty path segments.",
                    ))
                    continue
                path = _preflight_normalize_path(raw_path)
                if _preflight_skip_archive_path(path):
                    report["warnings"].append(_preflight_issue(
                        "IGNORED_ARCHIVE_METADATA", f"ignored archive metadata: {path}",
                    ))
                    continue
                record = {
                    "path": path,
                    "bytes": int(info.file_size),
                    "isBinary": _preflight_is_binary_path(path),
                    "_zipIndex": index,
                }
                records.append(record)
                if info.file_size > MAX_SINGLE_FILE_BYTES:
                    report["errors"].append(_preflight_issue(
                        "ZIP_FILE_TOO_LARGE",
                        f"file {path} exceeds max single-file size ({MAX_SINGLE_FILE_BYTES} bytes)",
                        "Split large assets or raise the single-file upload limit in admin settings.",
                    ))
                    continue
                try:
                    with archive.open(info) as stream:
                        actual = _preflight_read_limited(stream)
                    if len(actual) != info.file_size:
                        raise zipfile.BadZipFile("ZIP entry size did not match its header")
                except ValueError:
                    report["errors"].append(_preflight_issue(
                        "ZIP_FILE_TOO_LARGE",
                        f"file {path} exceeds max single-file size ({MAX_SINGLE_FILE_BYTES} bytes)",
                        "Split large assets or raise the single-file upload limit in admin settings.",
                    ))
                except (OSError, RuntimeError, NotImplementedError, EOFError, zipfile.BadZipFile) as exc:
                    report["errors"].append(_preflight_issue(
                        "ZIP_ENTRY_READ_FAILED",
                        f"ZIP entry {path!r} cannot be read: {exc}",
                        "Rebuild the archive and run preflight again.",
                    ))
    except (OSError, RuntimeError, NotImplementedError, EOFError, zipfile.BadZipFile) as exc:
        report["errors"].append(_preflight_issue(
            "ZIP_OPEN_FAILED",
            f"ZIP file {source_path.name!r} cannot be opened: {exc}",
            "Upload a valid .zip archive.",
        ))
        return

    if not records and not report["errors"]:
        report["errors"].append(_preflight_issue(
            "ZIP_EMPTY",
            "ZIP did not contain deployable files",
            "Upload a ZIP containing index.html, README.md, or a static site folder.",
        ))
        return

    def load_bytes(record: dict) -> bytes:
        with zipfile.ZipFile(source_path) as archive:
            info = archive.infolist()[record["_zipIndex"]]
            with archive.open(info) as stream:
                return _preflight_read_limited(stream)

    _preflight_analyze_records(records, "zip", entry_hint, load_bytes, report)


def _preflight_file(source_path: pathlib.Path, entry_hint: str, report: dict) -> None:
    try:
        size = source_path.stat().st_size
    except OSError as exc:
        report["errors"].append(_preflight_issue("SOURCE_READ_FAILED", f"could not inspect file: {exc}"))
        return
    report["sourceBytes"] = size
    path = safe_multipart_filename(source_path.name)
    record = {
        "path": path,
        "bytes": size,
        "isBinary": _preflight_is_binary_path(path),
        "_source": source_path,
    }
    if size > MAX_SITE_TOTAL_BYTES:
        report["errors"].append(_preflight_issue(
            "SOURCE_TOO_LARGE",
            f"source file is {size} bytes; upload limit is {MAX_SITE_TOTAL_BYTES} bytes",
            "Reduce the source file size before uploading.",
        ))
    if size > MAX_SINGLE_FILE_BYTES:
        report["errors"].append(_preflight_issue(
            "ZIP_FILE_TOO_LARGE",
            f"file {path} exceeds max single-file size ({MAX_SINGLE_FILE_BYTES} bytes)",
            "Split large assets or raise the single-file upload limit in admin settings.",
        ))

    def load_bytes(item: dict) -> bytes:
        with pathlib.Path(item["_source"]).open("rb") as stream:
            return _preflight_read_limited(stream)

    _preflight_analyze_records([record], "file", entry_hint, load_bytes, report)


def preflight_source(source_arg: str, filename_hint: str = "") -> dict:
    """Inspect a local deploy source without creating a session or uploading it."""
    source_text = str(source_arg)
    report = _preflight_report(source_text)
    hint = str(filename_hint or "").strip()
    if hint:
        hint = _preflight_normalize_path(hint)
        if not _preflight_is_safe_path(hint):
            report["errors"].append(_preflight_issue(
                "UNSAFE_ENTRY_PATH",
                f"--filename {hint!r} is not a safe relative path",
                "Use a clean relative HTML or Markdown path without '..', absolute paths, or drive letters.",
            ))
            return report
        if not _preflight_is_page_entry(hint):
            report["warnings"].append(_preflight_issue(
                "ENTRY_HINT_IGNORED",
                f"--filename {hint!r} is not an HTML or Markdown entry and will be ignored.",
            ))

    source_path = pathlib.Path(source_text)
    try:
        if source_path.is_symlink():
            report["errors"].append(_preflight_issue(
                "UNSAFE_SYMLINK",
                f"source path is a symbolic link: {source_text}",
                "Use the real source file or directory instead of a symbolic link.",
            ))
        elif not source_path.exists():
            report["errors"].append(_preflight_issue("SOURCE_NOT_FOUND", f"source not found: {source_text}"))
        elif source_path.is_dir():
            report["sourceType"] = "directory"
            _preflight_directory(source_path, hint, report)
        elif source_path.is_file():
            if source_path.suffix.lower() == ".zip":
                report["sourceType"] = "zip"
                _preflight_zip(source_path, hint, report)
            else:
                report["sourceType"] = "file"
                _preflight_file(source_path, hint, report)
        else:
            report["errors"].append(_preflight_issue(
                "UNSUPPORTED_SOURCE",
                f"source is neither a regular file nor a directory: {source_text}",
            ))
    except (OSError, ValueError, OverflowError, RuntimeError, zipfile.BadZipFile) as exc:
        report["errors"].append(_preflight_issue("SOURCE_READ_FAILED", f"could not inspect source: {exc}"))

    report["success"] = not report["errors"]
    return report


def cmd_preflight(args) -> int:
    report = preflight_source(args.source, getattr(args, "filename", ""))
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if report["success"] else 1


def prepare_multipart_source(source_arg: str) -> tuple[pathlib.Path, str, callable]:
    root = pathlib.Path(source_arg)
    if not root.exists():
        die(f"Source not found: {source_arg}")
    if root.is_symlink():
        die(f"Refusing symbolic-link source: {source_arg}")
    if root.is_file():
        size = root.stat().st_size
        if size > MAX_SINGLE_FILE_BYTES:
            die(f"Source too large ({size} bytes); single-file limit is {MAX_SINGLE_FILE_BYTES}.")
        if size > MAX_SITE_TOTAL_BYTES:
            die(f"Source too large ({size} bytes); limit is {MAX_SITE_TOTAL_BYTES}.")
        return root, root.name, lambda: None

    fd, temp_name = tempfile.mkstemp(prefix="pagepilot-", suffix=".zip")
    os.close(fd)
    temp_path = pathlib.Path(temp_name)

    def cleanup() -> None:
        try:
            temp_path.unlink()
        except OSError:
            pass

    total_size = 0
    walked = []
    try:
        for path in sorted(root.rglob("*"), key=lambda item: item.as_posix()):
            if path.is_symlink():
                die(f"Refusing symbolic link in source directory: {path.relative_to(root).as_posix()}")
            if path.is_file():
                rel = rel_path(root, path)
                if not _preflight_skip_archive_path(rel):
                    walked.append(path)
        if len(walked) > MAX_FILES_PER_SITE:
            die(f"Too many files ({len(walked)}); limit is {MAX_FILES_PER_SITE}.")
    except BaseException:
        # rel_path()/rglob() can fail before the archive-writing block below;
        # always remove the mkstemp file on those early exits as well.
        cleanup()
        raise
    try:
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
                    _zip_write_file(zf, p, rel)
            archive_size = temp_path.stat().st_size
            if archive_size > MAX_SINGLE_FILE_BYTES:
                cleanup()
                die(f"Generated site ZIP is too large ({archive_size} bytes); single-file upload limit is {MAX_SINGLE_FILE_BYTES}.")
            if archive_size > MAX_SITE_TOTAL_BYTES:
                cleanup()
                die(f"Generated site ZIP exceeds {MAX_SITE_TOTAL_BYTES} bytes; reduce the source directory before uploading.")
        except (OSError, ValueError, OverflowError, RuntimeError, NotImplementedError, EOFError, zipfile.BadZipFile) as exc:
            cleanup()
            die(f"Could not package source directory: {exc}")
    except BaseException:
        # Covers rel_path(), archive I/O, and interruption paths not handled by
        # the typed packaging errors above.
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


def overwrite_multipart(args, fields: dict, source_arg: str) -> tuple[int, dict]:
    base = server_url(args)
    token = auth_token(args)
    sid = "" if token else ensure_session(base)
    source_path, upload_name, cleanup = prepare_multipart_source(source_arg)
    code = urllib.parse.quote(args.code, safe="")
    version = urllib.parse.quote(str(args.version), safe="")
    try:
        return request_multipart(
            base,
            token,
            f"/api/deploys/{code}/versions/{version}",
            fields,
            source_path,
            upload_name,
            sid,
            method="PATCH",
        )
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


def normalized_filename_arg(args) -> str:
    """Use the same slash-normalized entry hint for preflight and upload."""
    raw = str(getattr(args, "filename", "") or "").strip()
    if not raw:
        return ""
    value = _preflight_normalize_path(raw)
    if not _preflight_is_safe_path(value):
        die(f"--filename {value!r} is not a safe relative path")
    return value


def add_deploy_options(payload: dict, args) -> None:
    filename = normalized_filename_arg(args)
    if filename:
        payload["filename"] = filename
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
    if config_ok and isinstance(config_data.get("limits"), dict):
        report["uploadLimits"] = config_data["limits"]
    if token:
        check("credential", "/api/tokens", required=True, use_token=True)
    else:
        report["checks"].append({
            "name": "credential",
            "ok": True,
            "skipped": True,
            "detail": "no bearer token configured; anonymous deployment quota may apply",
        })

        # /api/session intentionally creates a persistent session. Doctor only
        # probes read-only endpoints; anonymous deployment creates it on demand.
        report["checks"].append({
            "name": "anonymous_session",
            "ok": True,
            "skipped": True,
            "detail": "anonymous session is created on first deployment; doctor does not create persistent sessions",
        })

    if args.require_admin:
        _, admin_data, session_ok = check("admin_session", "/api/admin/session", required=False, use_token=True)
        is_admin = isinstance(admin_data, dict) and admin_data.get("isAdmin") is True
        admin_check = report["checks"][-1]
        if not token:
            admin_check["ok"] = False
            report["success"] = False
            report["hint"] = "Set PAGEPILOT_TOKEN or pass --token with an admin token."
        elif not session_ok or not is_admin:
            admin_check["ok"] = False
            admin_check["detail"] = "configured token is not an administrator token"
            report["success"] = False
            report["hint"] = "The token is missing, invalid, revoked, or not an admin token."
    elif token:
        check("admin_session", "/api/admin/session", required=False, use_token=bool(token))
    status, data = request_json(base, "", "/openapi.json")
    report["checks"].append({"name": "openapi", "ok": status == 200 and data.get("openapi") != "", "httpStatus": status})
    if status != 200:
        report["success"] = False
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if report["success"] else 1


def cmd_version(args) -> int:
    return print_result(200, {
        "success": True,
        "name": "pagep",
        "version": SKILL_VERSION,
        "userAgent": UA,
        "configFile": str(CONFIG_FILE),
    })


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
    payload = {"description": args.description}
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
        # Keep stdout machine-readable for Agent callers; human context belongs
        # on stderr so json.loads(stdout) remains reliable after a deploy.
        print_deploy_summary(data, stream=sys.stderr)
    return print_result(status, data)


def cmd_append(args) -> int:
    ensure_description(args)
    ensure_title(args)
    base = server_url(args)
    token = auth_token(args)
    payload = {
        "description": args.description,
        "enableCustomCode": True,
        "customCode": args.code,
        "createVersion": True,
    }
    add_deploy_options(payload, args)
    status, data = deploy_multipart(args, payload, args.source)
    if 200 <= status < 300 and data.get("code"):
        remember_project(base, args.source, data["code"])
        apply_access_password_after_deploy(args, base, token, str(data["code"]))
        print_deploy_summary(data, stream=sys.stderr)
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
        if args.output and 200 <= status < 300 and data.get("success", True) is not False:
            try:
                pathlib.Path(args.output).write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
            except OSError as exc:
                if JSON_OUTPUT:
                    print(json.dumps({
                        "success": False,
                        "errorCode": "OUTPUT_WRITE_FAILED",
                        "detail": f"could not write metadata to {args.output}: {exc}",
                        "hint": "Choose a writable output path and retry.",
                    }, ensure_ascii=False))
                    return 1
                die(f"Could not write metadata to {args.output}: {exc}")
            if JSON_OUTPUT:
                return print_result(status, {
                    "success": True,
                    "output": str(args.output),
                    "metadata": data,
                })
            return 0
        return print_result(status, data)

    if JSON_OUTPUT and not args.output:
        print(json.dumps({
            "success": False,
            "errorCode": "OUTPUT_REQUIRED",
            "detail": "--json with --download requires --output so stdout remains JSON.",
            "hint": "Pass --output <directory> (or an explicit .zip file path).",
        }, ensure_ascii=False))
        return 1

    url = f"{base}/api/deploy/content?{qs}"
    headers = {"User-Agent": UA, "Accept": "application/json,text/html,application/zip,*/*"}
    token = auth_token(args)
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = resp.read()
            status = getattr(resp, "status", 200)
            content_type = resp.headers.get("Content-Type", "")
    except urllib.error.HTTPError as e:
        error_body = e.read().decode("utf-8", "replace")
        data = structured_http_error(e.code, error_body, str(e))
        return print_result(e.code, data)
    except TimeoutError as exc:
        return print_result(0, network_error_payload(str(exc) or "request timed out"))
    except urllib.error.URLError as exc:
        return print_result(0, network_error_payload(str(exc)))
    if args.output:
        output = pathlib.Path(args.output)
        try:
            if output.suffix.lower() == ".zip":
                output.parent.mkdir(parents=True, exist_ok=True)
            else:
                output.mkdir(parents=True, exist_ok=True)
                suffix = ".zip" if "zip" in content_type.lower() else ".html"
                name = f"{args.code}-v{args.version}{suffix}" if args.version else f"{args.code}{suffix}"
                output = output / safe_multipart_filename(name)
            output.write_bytes(body)
        except OSError as exc:
            if JSON_OUTPUT:
                print(json.dumps({
                    "success": False,
                    "errorCode": "OUTPUT_WRITE_FAILED",
                    "detail": f"could not write download to {output}: {exc}",
                    "hint": "Choose a writable output path and retry.",
                }, ensure_ascii=False))
                return 1
            die(f"Could not write download to {output}: {exc}")
        if JSON_OUTPUT:
            return print_result(status, {
                "success": True,
                "output": str(output),
                "bytes": len(body),
                "contentType": content_type,
            })
        print(f"Saved {len(body)} bytes to {output}")
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
    payload = {"description": args.description}
    add_deploy_options(payload, args)
    status, data = overwrite_multipart(args, payload, args.source)
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
    label = (getattr(args, "label", "") or "").strip()
    label_arg = (getattr(args, "label_arg", "") or "").strip()
    if label and label_arg and label != label_arg:
        die("--label and positional label cannot be different.")
    if not label:
        label = label_arg
    expires_at = (getattr(args, "expires_at", "") or "").strip()
    ttl_seconds = getattr(args, "ttl_seconds", None)
    if expires_at and ttl_seconds is not None:
        die("Use either --expires-at or --ttl-seconds, not both.")
    payload = {"label": label, "isAdmin": bool(args.admin)}
    if expires_at:
        payload["expiresAt"] = expires_at
    if ttl_seconds is not None:
        if ttl_seconds <= 0:
            die("--ttl-seconds must be positive. Omit it for a permanent token.")
        payload["ttlSeconds"] = ttl_seconds
    base = server_url(args)
    status, data = request_json(base, auth_token(args), "/api/token", "POST", payload)
    if (
        getattr(args, "save", False)
        and 200 <= status < 300
        and data.get("success", True) is not False
    ):
        created_token = str(data.get("token") or "")
        if not created_token:
            die("Token was created but the response did not include plaintext token.")
        saved = dict(data)
        save_bound_token(
            base,
            created_token,
            str(saved.get("username") or saved.get("ownerUsername") or saved.get("ownerUserId") or ""),
            str(saved.get("tokenId") or saved.get("id") or ""),
        )
        data = {**data, "savedTo": str(CONFIG_FILE)}
    return print_result(status, data)


def cmd_token_list(args) -> int:
    status, data = request_json(server_url(args), auth_token(args), "/api/tokens")
    return print_result(status, data)


def cmd_token_revoke(args) -> int:
    tid = urllib.parse.quote(args.id, safe="")
    status, data = request_json(server_url(args), auth_token(args), f"/api/tokens/{tid}", "DELETE")
    return print_result(status, data)


def cmd_token_save(args) -> int:
    token = (args.value or "").strip()
    if not token:
        die("token value is required.")
    base = server_url(args)
    save_bound_token(base, token, "", "")
    return print_result(200, {
        "success": True,
        "server": base,
        "tokenSaved": True,
        "savedTo": str(CONFIG_FILE),
    })


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
    key = (getattr(args, "key", "") or "").strip()
    if key:
        data = load_local_config()
        value = str(data.get(key) or "")
        if key == "token" and len(value) > 8:
            value = value[:4] + "..." + value[-4:]
        return print_result(200, {
            "success": True,
            "key": key,
            "value": value,
            "configured": key in data,
            "configFile": str(CONFIG_FILE),
        })
    status, data = request_json(server_url(args), auth_token(args), "/api/config")
    return print_result(status, data)


def cmd_config_set_local(args) -> int:
    key = (args.key or "").strip()
    value = (args.value or "").strip()
    if key not in {"server", "token"}:
        die("config set only supports server or token.")
    if not value:
        die("config value is required.")
    data = load_local_config()
    if key == "server":
        old_server = normalize_server(str(data.get("server") or ""))
        new_server = normalize_server(value)
        data["server"] = new_server
        if old_server and old_server != new_server:
            for stale_key in ("token", "tokenId", "username"):
                data.pop(stale_key, None)
    else:
        data["server"] = server_url(args)
        data["token"] = value
        data.pop("tokenId", None)
        data.pop("username", None)
    save_local_config(data)
    return print_result(200, {
        "success": True,
        "key": key,
        "server": data.get("server", ""),
        "tokenSaved": bool(data.get("token")),
        "configFile": str(CONFIG_FILE),
    })


def cmd_config_show_local(args) -> int:
    data = dict(load_local_config())
    if data.get("token"):
        token = str(data["token"])
        data["token"] = token[:4] + "..." + token[-4:] if len(token) > 8 else "***"
    return print_result(200, {
        "success": True,
        "configFile": str(CONFIG_FILE),
        "config": data,
    })


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


def cmd_screen_assign(args) -> int:
    token = registered_token(args, "screen assign")
    screen_id = urllib.parse.quote(args.screen, safe="")
    payload = {"ownerUserId": args.owner_user_id}
    if args.name:
        payload["name"] = args.name
    status, data = request_json(
        server_url(args),
        token,
        f"/api/admin/screens/{screen_id}/assign",
        "POST",
        payload,
    )
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
        payload = {"description": args.description}
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
    parser.add_argument("--server", help="PagePilot server URL (default: saved config, $PAGEPILOT_SERVER, or https://pagepilot.dell.4dbim.cc:1143/)")
    parser.add_argument("--token", help="bearer token (default: saved config or $PAGEPILOT_TOKEN)")
    parser.add_argument("--json", action="store_true", help="emit machine-readable JSON (the Skill already uses JSON by default)")
    sub = parser.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("version", help="Print pagep Skill version")
    p.set_defaults(func=cmd_version)

    p = sub.add_parser("doctor", help="Check health, config, OpenAPI, and admin auth readiness")
    p.add_argument("--require-admin", action="store_true", help="Fail unless an admin session validates")
    p.set_defaults(func=cmd_doctor)

    p = sub.add_parser("preflight", help="Inspect a local file, directory, or ZIP before upload")
    p.add_argument("source", help="Path to an HTML/Markdown file, website ZIP, or site directory")
    p.add_argument("--filename", "-f", default="", help="Optional explicit entry path used for ZIP root detection")
    p.set_defaults(func=cmd_preflight)

    p = sub.add_parser("session", help="Validate current token against /api/admin/session")
    p.set_defaults(func=cmd_session)

    p = sub.add_parser("claim-session", help="Claim anonymous-session deployments for the current token/user")
    p.add_argument("--session-id", default="", help="Anonymous session id. Defaults to ~/.pagep/session.json")
    p.set_defaults(func=cmd_claim_session)

    def add_common_deploy_flags(p, *, with_code: bool, with_create_version: bool):
        p.add_argument("source", help="Path to an HTML file, Markdown file, website ZIP, or site directory")
        p.add_argument("--description", "-d", required=True, help="Required concise description, max 240 chars")
        p.add_argument("--title", "-t", required=True, help="Required meaningful Chinese site/version title")
        p.add_argument("--filename", "-f", help="Optional explicit entry path; omit for automatic server detection")
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
    pt.add_argument("label_arg", nargs="?", default="", help="Token label; same as --label")
    pt.add_argument("--label", default="")
    pt.add_argument("--admin", action="store_true")
    pt.add_argument("--expires-at", default="", help="RFC3339 expiry timestamp. Omit for permanent.")
    pt.add_argument("--ttl-seconds", type=int, help="Temporary token lifetime in seconds. Omit for permanent.")
    pt.add_argument("--save", action="store_true", help="Save the returned plaintext token into ~/.pagep/config.json")
    pt.set_defaults(func=cmd_token_create)
    pt = token_sub.add_parser("save", help="Save an existing plaintext token into local config")
    pt.add_argument("value")
    pt.set_defaults(func=cmd_token_save)
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
    pc.add_argument("key", nargs="?", default="", help="Optional local config key: server or token")
    pc.set_defaults(func=cmd_config_get)
    pc = config_sub.add_parser("set", help="Save local config key: server or token")
    pc.add_argument("key", choices=["server", "token"])
    pc.add_argument("value")
    pc.set_defaults(func=cmd_config_set_local)
    pc = config_sub.add_parser("show", help="Show local config with masked token")
    pc.set_defaults(func=cmd_config_show_local)
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
    ps = screen_sub.add_parser("assign", help="Admin: assign an unpaired connected screen to a user")
    ps.add_argument("screen", help="Unpaired screen id shown in admin /api/screens")
    ps.add_argument("--owner-user-id", required=True, help="Target registered user id")
    ps.add_argument("--name", default="", help="Optional display name for the screen")
    ps.set_defaults(func=cmd_screen_assign)
    ps = screen_sub.add_parser("publish", help="Publish an app or local HTML project to a screen")
    ps.add_argument("--screen", required=True, help="Target screen id")
    ps.add_argument("--app", default="", help="Existing PagePilot app code")
    ps.add_argument("--source", default="", help="Optional local HTML file or site directory to deploy before publishing")
    ps.add_argument("--description", "-d", default="", help="Required when --source is used")
    ps.add_argument("--title", "-t", default="", help="Required meaningful Chinese title when --source is used")
    ps.add_argument("--filename", "-f", default="", help="Optional explicit entry path when --source is used")
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


def normalize_global_options(argv: list[str]) -> list[str]:
    """Accept global options before or after a subcommand.

    argparse only accepts root options before a subcommand by default, while the
    published Skill examples intentionally put the target server and JSON mode
    next to doctor or deploy. Keep both spellings working without duplicating
    every flag.
    """
    global_args: list[str] = []
    remaining: list[str] = []
    index = 0
    while index < len(argv):
        value = argv[index]
        if value in {"--server", "--token"}:
            global_args.append(value)
            if index + 1 < len(argv):
                global_args.append(argv[index + 1])
                index += 2
                continue
        elif value == "--json":
            global_args.append(value)
            index += 1
            continue
        elif value.startswith("--server=") or value.startswith("--token="):
            global_args.append(value)
            index += 1
            continue
        remaining.append(value)
        index += 1
    return global_args + remaining


def parse_cli_args(parser: argparse.ArgumentParser, argv: list[str] | None = None):
    global JSON_OUTPUT
    raw = list(sys.argv[1:] if argv is None else argv)
    JSON_OUTPUT = any(value == "--json" or value.startswith("--json=") for value in raw)
    normalized = normalize_global_options(raw)
    if not JSON_OUTPUT:
        return parser.parse_args(normalized)

    # argparse writes usage errors to stderr and exits before command handlers
    # can format them. Capture that text so a documented --json invocation still
    # produces one parseable object on stdout.
    captured = io.StringIO()
    try:
        with redirect_stderr(captured):
            return parser.parse_args(normalized)
    except SystemExit as exc:
        if exc.code:
            detail = captured.getvalue().strip().splitlines()
            detail = detail[-1] if detail else "invalid command-line arguments"
            print(json.dumps({
                "success": False,
                "errorCode": "CLI_USAGE",
                "detail": detail,
                "hint": "Run pagep --help to see valid commands and options.",
            }, ensure_ascii=False))
        raise


def main() -> None:
    parser = build_parser()
    args = parse_cli_args(parser)
    raise SystemExit(args.func(args))


if __name__ == "__main__":
    main()
