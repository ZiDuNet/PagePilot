import importlib.util
import io
import json
import os
import pathlib
import sys
import tempfile
import types
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest import mock
import zipfile


SCRIPT = pathlib.Path(__file__).with_name("hostctl_deploy.py")
SKILL_DOC = SCRIPT.parent.parent / "SKILL.md"
SPEC = importlib.util.spec_from_file_location("hostctl_deploy", SCRIPT)
hostctl_deploy = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(hostctl_deploy)


class SkillDocumentationTests(unittest.TestCase):
    def test_skill_document_declares_package_version(self):
        text = SKILL_DOC.read_text(encoding="utf-8")

        self.assertIn("metadata:", text)
        self.assertIn('version: "0.3.1"', text)
        self.assertIn('updated: "2026-08-31"', text)
        self.assertIn("pagep version", text)
        self.assertIn("pagep preflight", text)

    def test_skill_does_not_upload_without_explicit_publish_request(self):
        text = SKILL_DOC.read_text(encoding="utf-8")
        self.assertIn("不要擅自上传或创建 session", text)
        self.assertIn("如果用户只要求本地产物", text)

    def test_skill_documents_multipart_overwrite_contract(self):
        text = SKILL_DOC.read_text(encoding="utf-8")

        self.assertIn("覆盖版本", text)
        self.assertIn("multipart", text)
        self.assertIn("不要在覆盖版本时把文件塞进 JSON/base64", text)

    def test_skill_focuses_on_static_site_publishing(self):
        text = SKILL_DOC.read_text(encoding="utf-8")

        for required in [
            "HTML",
            "Markdown",
            "ZIP",
            "多文件静态站点",
            "MCP",
            "Token",
            "屏幕投放",
        ]:
            self.assertIn(required, text)

        for forbidden in [
            "Reveal",
            "reveal",
            "PPT",
            "PowerPoint",
            "幻灯",
            "演示文稿",
            "slides",
            "deck",
            "RevealHighlight",
        ]:
            self.assertNotIn(forbidden, text)


class ScreenOrientationTests(unittest.TestCase):
    def test_reads_orientation_from_device_info(self):
        screen = {
            "id": "screen-1",
            "deviceInfo": {
                "screenWidthPx": 1920,
                "screenHeightPx": 1080,
                "orientation": "landscape",
            },
        }

        self.assertEqual(hostctl_deploy.screen_orientation(screen), "landscape")

    def test_infers_orientation_from_resolution(self):
        screen = {
            "id": "screen-1",
            "deviceInfo": {
                "screenWidthPx": 1080,
                "screenHeightPx": 1920,
            },
        }

        self.assertEqual(hostctl_deploy.screen_orientation(screen), "portrait")

    def test_detects_publish_orientation_mismatch(self):
        screen = {
            "id": "screen-1",
            "name": "大厅横屏",
            "deviceInfo": {
                "screenWidthPx": 1920,
                "screenHeightPx": 1080,
                "orientation": "landscape",
            },
        }

        ok, message = hostctl_deploy.orientation_check_result(screen, "portrait")

        self.assertFalse(ok)
        self.assertIn("portrait", message)
        self.assertIn("landscape", message)

    def test_allows_any_or_matching_orientation(self):
        screen = {"deviceInfo": {"orientation": "landscape"}}

        self.assertTrue(hostctl_deploy.orientation_check_result(screen, "any")[0])
        self.assertTrue(hostctl_deploy.orientation_check_result(screen, "landscape")[0])


class RequestHeaderTests(unittest.TestCase):
    def test_request_json_sends_public_origin_header(self):
        captured = {}

        class FakeResponse:
            status = 200

            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb):
                return False

            def read(self):
                return b'{"success": true}'

        def fake_urlopen(req, timeout=0):
            captured["origin"] = req.headers.get("X-hostctl-current-origin")
            return FakeResponse()

        with mock.patch.object(hostctl_deploy.urllib.request, "urlopen", fake_urlopen):
            status, data = hostctl_deploy.request_json(
                "https://pagepilot.chaoxi.live",
                "",
                "/api/config",
            )

        self.assertEqual(status, 200)
        self.assertTrue(data["success"])
        self.assertEqual(captured["origin"], "https://pagepilot.chaoxi.live")

    def test_request_json_normalizes_non_json_http_errors(self):
        error = hostctl_deploy.urllib.error.HTTPError(
            "https://pagepilot.example.com/api/config",
            502,
            "Bad Gateway",
            None,
            io.BytesIO(b"<html>upstream unavailable</html>"),
        )
        try:
            with mock.patch.object(hostctl_deploy.urllib.request, "urlopen", side_effect=error):
                status, data = hostctl_deploy.request_json("https://pagepilot.example.com", "", "/api/config")
        finally:
            error.close()

        self.assertEqual(status, 502)
        self.assertFalse(data["success"])
        self.assertEqual(data["errorCode"], "HTTP_ERROR")
        self.assertIn("upstream unavailable", data["detail"])
        self.assertTrue(data["hint"])

    def test_request_json_normalizes_network_errors_with_hint(self):
        with mock.patch.object(
            hostctl_deploy.urllib.request,
            "urlopen",
            side_effect=hostctl_deploy.urllib.error.URLError("offline"),
        ):
            status, data = hostctl_deploy.request_json("https://pagepilot.example.com", "", "/api/config")

        self.assertEqual(status, 0)
        self.assertFalse(data["success"])
        self.assertEqual(data["errorCode"], "NETWORK_ERROR")
        self.assertIn("offline", data["detail"])
        self.assertTrue(data["hint"])

    def test_request_json_normalizes_timeout_and_empty_success(self):
        with mock.patch.object(
            hostctl_deploy.urllib.request,
            "urlopen",
            side_effect=TimeoutError("read timed out"),
        ):
            status, data = hostctl_deploy.request_json("https://pagepilot.example.com", "", "/api/config")
        self.assertEqual(status, 0)
        self.assertEqual(data["errorCode"], "NETWORK_ERROR")
        self.assertTrue(data["hint"])

        class EmptyResponse:
            status = 200

            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb):
                return False

            def read(self):
                return b""

        with mock.patch.object(hostctl_deploy.urllib.request, "urlopen", return_value=EmptyResponse()):
            status, data = hostctl_deploy.request_json("https://pagepilot.example.com", "", "/api/config")
        self.assertEqual(status, 200)
        self.assertEqual(data["errorCode"], "INVALID_RESPONSE")
        self.assertFalse(data["success"])

    def test_request_multipart_sends_file_and_current_origin_header(self):
        captured = {}

        class FakeResponse:
            status = 200
            headers = {}

            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb):
                return False

            def read(self):
                return b'{"success": true, "code": "demo"}'

        def fake_urlopen(req, timeout=0):
            captured["origin"] = req.headers.get("X-hostctl-current-origin")
            captured["content_type"] = req.headers.get("Content-type")
            captured["body"] = req.data
            return FakeResponse()

        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "index.html"
            source.write_text("<!doctype html><title>demo</title>", encoding="utf-8")
            with mock.patch.object(hostctl_deploy.urllib.request, "urlopen", fake_urlopen):
                status, data = hostctl_deploy.request_multipart(
                    "https://pagepilot.chaoxi.live",
                    "",
                    "/api/deploy",
                    {"description": "demo", "filename": "index.html"},
                    source,
                    "site.zip",
                    "session-1",
                    {"agentId": "agent-1", "agentLabel": "Agent"},
                )

        self.assertEqual(status, 200)
        self.assertEqual(data["code"], "demo")
        self.assertEqual(captured["origin"], "https://pagepilot.chaoxi.live")
        self.assertIn("multipart/form-data", captured["content_type"])
        self.assertIn(b'name="filename"', captured["body"])
        self.assertIn(b'filename="site.zip"', captured["body"])
        self.assertIn(b"<title>demo</title>", captured["body"])

    def test_request_multipart_normalizes_non_json_http_errors(self):
        error = hostctl_deploy.urllib.error.HTTPError(
            "https://pagepilot.example.com/api/deploy",
            503,
            "Service Unavailable",
            None,
            io.BytesIO(b"proxy temporarily unavailable"),
        )
        try:
            with tempfile.TemporaryDirectory() as tmp:
                source = pathlib.Path(tmp) / "index.html"
                source.write_text("<!doctype html><title>demo</title>", encoding="utf-8")
                with mock.patch.object(hostctl_deploy.urllib.request, "urlopen", side_effect=error):
                    status, data = hostctl_deploy.request_multipart(
                        "https://pagepilot.example.com", "", "/api/deploy", {}, source, "index.html"
                    )
        finally:
            error.close()

        self.assertEqual(status, 503)
        self.assertFalse(data["success"])
        self.assertEqual(data["errorCode"], "HTTP_ERROR")
        self.assertEqual(data["detail"], "proxy temporarily unavailable")

    def test_request_multipart_handles_non_utf8_success_response(self):
        class FakeResponse:
            status = 200

            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb):
                return False

            def read(self):
                return b"{\"success\":true,\"detail\":\"\xff\"}"

        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "index.html"
            source.write_text("<!doctype html><title>demo</title>", encoding="utf-8")
            with mock.patch.object(hostctl_deploy.urllib.request, "urlopen", return_value=FakeResponse()):
                status, data = hostctl_deploy.request_multipart(
                    "https://pagepilot.example.com", "", "/api/deploy", {}, source, "index.html"
                )

        self.assertEqual(status, 200)
        self.assertTrue(data["success"])


class LocalConfigAndTokenTests(unittest.TestCase):
    def test_save_session_id_is_atomic_and_private(self):
        with tempfile.TemporaryDirectory() as tmp:
            session_file = pathlib.Path(tmp) / "session.json"
            with mock.patch.object(hostctl_deploy, "SESSION_FILE", session_file):
                hostctl_deploy.save_session_id("https://pagepilot.example.com", "anon-secret")
            self.assertEqual(json.loads(session_file.read_text(encoding="utf-8"))["sessionId"], "anon-secret")
            self.assertEqual(session_file.stat().st_mode & 0o777, 0o600)
            self.assertEqual(list(session_file.parent.glob(".session-*")), [])

    def test_version_command_reports_skill_version(self):
        parser = hostctl_deploy.build_parser()
        args = parser.parse_args(["version"])
        captured = {}

        def fake_print_result(status, data):
            captured["status"] = status
            captured["data"] = data
            return 0

        with mock.patch.object(hostctl_deploy, "print_result", fake_print_result):
            self.assertEqual(args.func(args), 0)

        self.assertEqual(captured["status"], 200)
        self.assertEqual(captured["data"]["version"], hostctl_deploy.SKILL_VERSION)
        self.assertEqual(captured["data"]["userAgent"], f"pagep-skill/{hostctl_deploy.SKILL_VERSION}")

    def test_server_url_uses_saved_server_when_flag_and_env_are_empty(self):
        with tempfile.TemporaryDirectory() as tmp:
            config_file = pathlib.Path(tmp) / "config.json"
            config_file.write_text(
                json.dumps({"server": "https://pagepilot.example.com", "token": "saved-token"}),
                encoding="utf-8",
            )
            args = types.SimpleNamespace(server="", token="")

            with mock.patch.object(hostctl_deploy, "CONFIG_FILE", config_file):
                with mock.patch.dict(os.environ, {"PAGEPILOT_SERVER": "", "HOSTCTL_SERVER": ""}):
                    self.assertEqual(
                        hostctl_deploy.server_url(args),
                        "https://pagepilot.example.com",
                    )

    def test_auth_token_uses_saved_token_for_saved_default_server(self):
        with tempfile.TemporaryDirectory() as tmp:
            config_file = pathlib.Path(tmp) / "config.json"
            config_file.write_text(
                json.dumps({"server": "https://pagepilot.example.com", "token": "saved-token"}),
                encoding="utf-8",
            )
            args = types.SimpleNamespace(server="", token="")

            with mock.patch.object(hostctl_deploy, "CONFIG_FILE", config_file):
                with mock.patch.dict(
                    os.environ,
                    {
                        "PAGEPILOT_SERVER": "",
                        "HOSTCTL_SERVER": "",
                        "PAGEPILOT_TOKEN": "",
                        "HOSTCTL_TOKEN": "",
                    },
                ):
                    self.assertEqual(hostctl_deploy.auth_token(args), "saved-token")

    def test_parser_accepts_go_cli_style_local_config_set(self):
        parser = hostctl_deploy.build_parser()

        args = parser.parse_args(
            ["config", "set", "server", "https://pagepilot.example.com"]
        )

        self.assertEqual(args.config_cmd, "set")
        self.assertEqual(args.key, "server")
        self.assertEqual(args.value, "https://pagepilot.example.com")

    def test_token_create_accepts_positional_label_and_omits_expiry_by_default(self):
        parser = hostctl_deploy.build_parser()
        args = parser.parse_args(
            [
                "--server",
                "https://pagepilot.example.com",
                "--token",
                "owner-token",
                "token",
                "create",
                "ci-bot",
            ]
        )
        captured = {}

        def fake_request_json(base, token, path, method="GET", payload=None, session_id="", agent=None):
            captured["base"] = base
            captured["token"] = token
            captured["path"] = path
            captured["method"] = method
            captured["payload"] = payload
            return 200, {"success": True, "id": "tok-1", "token": "created-token"}

        with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
            with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                code = args.func(args)

        self.assertEqual(code, 0)
        self.assertEqual(captured["base"], "https://pagepilot.example.com")
        self.assertEqual(captured["token"], "owner-token")
        self.assertEqual(captured["path"], "/api/token")
        self.assertEqual(captured["method"], "POST")
        self.assertEqual(captured["payload"], {"label": "ci-bot", "isAdmin": False})

    def test_token_create_save_persists_returned_token(self):
        with tempfile.TemporaryDirectory() as tmp:
            config_file = pathlib.Path(tmp) / "config.json"
            args = types.SimpleNamespace(
                server="https://pagepilot.example.com",
                token="owner-token",
                label="ci-bot",
                label_arg="",
                admin=False,
                expires_at="",
                ttl_seconds=None,
                save=True,
            )

            def fake_request_json(base, token, path, method="GET", payload=None, session_id="", agent=None):
                return 200, {
                    "success": True,
                    "id": "tok-1",
                    "token": "created-token",
                    "ownerUserId": "user-1",
                    "label": "ci-bot",
                }

            with mock.patch.object(hostctl_deploy, "CONFIG_FILE", config_file):
                with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
                    with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                        self.assertEqual(hostctl_deploy.cmd_token_create(args), 0)

                saved = json.loads(config_file.read_text(encoding="utf-8"))

        self.assertEqual(saved["server"], "https://pagepilot.example.com")
        self.assertEqual(saved["token"], "created-token")
        self.assertEqual(saved["tokenId"], "tok-1")
        self.assertEqual(saved["username"], "user-1")


class DeployOptionTests(unittest.TestCase):
    def test_print_deploy_summary_shows_server_returned_urls(self):
        stream = io.StringIO()

        hostctl_deploy.print_deploy_summary(
            {
                "code": "demo-site",
                "url": "https://pagepilot.example.com/agent/demo-site/",
                "detailUrl": "https://pagepilot.example.com/market/demo-site",
                "versionUrl": "https://pagepilot.example.com/agent/demo-site/?v=2",
                "versionNumber": 2,
                "templateSourceCode": "source-demo",
                "templateSourceVersion": 1,
                "reuseCount": 3,
                "preserveHint": "保留原有访问密码。",
            },
            stream=stream,
        )

        text = stream.getvalue()
        self.assertIn("发布成功", text)
        self.assertIn("访问 URL", text)
        self.assertIn("详情 URL", text)
        self.assertIn("版本 URL", text)
        self.assertIn("demo-site", text)
        self.assertIn("source-demo v1", text)
        self.assertIn("复用计数", text)
        self.assertIn("服务端返回", text)

    def test_cmd_deploy_prints_friendly_summary_on_success(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="",
            source="site",
            code="demo-site",
            filename="",
            description="中文描述",
            title="中文标题",
            visibility="public",
            category="",
            create_version=False,
            access_password="",
            template_source_code="",
            template_source_version=None,
            update=False,
        )
        captured = {}
        deploy_result = {
            "success": True,
            "code": "demo-site",
            "url": "https://pagepilot.example.com/agent/demo-site/",
            "detailUrl": "https://pagepilot.example.com/market/demo-site",
            "versionUrl": "https://pagepilot.example.com/agent/demo-site/?v=1",
        }

        def fake_deploy_multipart(args, payload, source):
            captured["payload"] = payload
            return 201, deploy_result

        with mock.patch.object(hostctl_deploy, "deploy_multipart", fake_deploy_multipart):
            with mock.patch.object(hostctl_deploy, "remember_project"):
                with mock.patch.object(hostctl_deploy, "apply_access_password_after_deploy"):
                    with mock.patch.object(hostctl_deploy, "print_deploy_summary") as summary:
                        with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                            code = hostctl_deploy.cmd_deploy(args)

        self.assertEqual(code, 0)
        self.assertNotIn("filename", captured["payload"])
        summary.assert_called_once()
        self.assertEqual(summary.call_args.args[0]["url"], deploy_result["url"])
        self.assertIs(summary.call_args.kwargs["stream"], sys.stderr)

    def test_cmd_deploy_keeps_json_stdout_machine_readable(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="",
            source="site",
            code="demo-site",
            filename="",
            description="中文描述",
            title="中文标题",
            visibility="public",
            category="",
            create_version=False,
            access_password="",
            template_source_code="",
            template_source_version=None,
            update=False,
        )
        deploy_result = {
            "success": True,
            "code": "demo-site",
            "url": "https://pagepilot.example.com/agent/demo-site/",
        }

        with mock.patch.object(hostctl_deploy, "deploy_multipart", return_value=(201, deploy_result)):
            with mock.patch.object(hostctl_deploy, "remember_project"):
                with mock.patch.object(hostctl_deploy, "apply_access_password_after_deploy"):
                    stdout = io.StringIO()
                    stderr = io.StringIO()
                    with redirect_stdout(stdout), redirect_stderr(stderr):
                        self.assertEqual(hostctl_deploy.cmd_deploy(args), 0)

        parsed = json.loads(stdout.getvalue())
        self.assertEqual(parsed["code"], "demo-site")
        self.assertIn("发布成功", stderr.getvalue())

    def test_add_deploy_options_records_template_source(self):
        payload = {}
        args = types.SimpleNamespace(
            title="复用演示",
            visibility="public",
            category="docs",
            create_version=False,
            access_password="",
            template_source_code="source-demo",
            template_source_version=3,
        )

        hostctl_deploy.add_deploy_options(payload, args)

        self.assertEqual(payload["templateSourceCode"], "source-demo")
        self.assertEqual(payload["templateSourceVersion"], 3)

    def test_cmd_overwrite_uses_multipart_patch(self):
        captured = {}
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="user-token",
            code="demo site",
            version=2,
            source="site",
            filename="",
            description="覆盖版本",
            title="覆盖版本标题",
            visibility="",
            category="",
            create_version=False,
            access_password="",
            template_source_code="",
            template_source_version=None,
        )

        def fake_request_multipart(base, token, path, fields, source_path, upload_name, session_id="", agent=None, method="POST"):
            captured["base"] = base
            captured["token"] = token
            captured["path"] = path
            captured["fields"] = fields
            captured["source_path"] = source_path
            captured["upload_name"] = upload_name
            captured["session_id"] = session_id
            captured["method"] = method
            return 200, {"success": True, "code": "demo-site"}

        with mock.patch.object(hostctl_deploy, "_ensure_unlocked"):
            with mock.patch.object(hostctl_deploy, "prepare_multipart_source", return_value=(pathlib.Path("site.zip"), "site.zip", lambda: None)):
                with mock.patch.object(hostctl_deploy, "request_multipart", fake_request_multipart):
                    with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                        code = hostctl_deploy.cmd_overwrite(args)

        self.assertEqual(code, 0)
        self.assertEqual(captured["base"], "https://pagepilot.example.com")
        self.assertEqual(captured["token"], "user-token")
        self.assertEqual(captured["path"], "/api/deploys/demo%20site/versions/2")
        self.assertEqual(captured["method"], "PATCH")
        self.assertEqual(captured["upload_name"], "site.zip")
        self.assertEqual(captured["fields"]["description"], "覆盖版本")
        self.assertEqual(captured["fields"]["title"], "覆盖版本标题")
        self.assertNotIn("filename", captured["fields"])

    def test_cmd_deploy_keeps_explicit_filename(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="",
            source="site",
            code="demo-site",
            filename="docs/README.md",
            description="中文描述",
            title="中文标题",
            visibility="public",
            category="",
            create_version=False,
            access_password="",
            template_source_code="",
            template_source_version=None,
            update=False,
        )
        captured = {}

        def fake_deploy_multipart(args, payload, source):
            captured["payload"] = payload
            return 201, {"success": True, "code": "demo-site"}

        with mock.patch.object(hostctl_deploy, "deploy_multipart", fake_deploy_multipart):
            with mock.patch.object(hostctl_deploy, "remember_project"):
                with mock.patch.object(hostctl_deploy, "apply_access_password_after_deploy"):
                    with mock.patch.object(hostctl_deploy, "print_deploy_summary"):
                        with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                            code = hostctl_deploy.cmd_deploy(args)

        self.assertEqual(code, 0)
        self.assertEqual(captured["payload"]["filename"], "docs/README.md")

    def test_cmd_deploy_normalizes_windows_filename_before_upload(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="",
            source="site",
            code="demo-site",
            filename=r"dist\index.html",
            description="中文描述",
            title="中文标题",
            visibility="public",
            category="",
            create_version=False,
            access_password="",
            template_source_code="",
            template_source_version=None,
            update=False,
        )
        captured = {}

        def fake_deploy_multipart(args, payload, source):
            captured["payload"] = payload
            return 201, {"success": True, "code": "demo-site"}

        with mock.patch.object(hostctl_deploy, "deploy_multipart", fake_deploy_multipart):
            with mock.patch.object(hostctl_deploy, "remember_project"):
                with mock.patch.object(hostctl_deploy, "apply_access_password_after_deploy"):
                    with mock.patch.object(hostctl_deploy, "print_deploy_summary"):
                        with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                            self.assertEqual(hostctl_deploy.cmd_deploy(args), 0)

        self.assertEqual(captured["payload"]["filename"], "dist/index.html")


class AdminCommandTests(unittest.TestCase):
    def test_admin_site_detail_uses_admin_endpoint(self):
        captured = {}
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="admin-token",
            code="demo site",
        )

        def fake_request_json(base, token, path, method="GET", payload=None):
            captured["base"] = base
            captured["token"] = token
            captured["path"] = path
            captured["method"] = method
            captured["payload"] = payload
            return 200, {"success": True}

        with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
            with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                code = hostctl_deploy.cmd_admin_site_detail(args)

        self.assertEqual(code, 0)
        self.assertEqual(captured["path"], "/api/admin/sites/demo%20site")
        self.assertEqual(captured["method"], "GET")
        self.assertIsNone(captured["payload"])

    def test_admin_audit_logs_builds_query(self):
        captured = {}
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="admin-token",
            actor_type="user",
            actor_id="user-1",
            actor_role="admin",
            action="site.pin",
            result="success",
            site_code="demo",
            target_type="site",
            target_id="demo",
            query="pinned",
            since="2026-07-06T00:00:00Z",
            until="2026-07-07T00:00:00Z",
            page=2,
            page_size=25,
        )

        def fake_request_json(base, token, path, method="GET", payload=None):
            captured["path"] = path
            captured["method"] = method
            return 200, {"success": True}

        with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
            with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                code = hostctl_deploy.cmd_admin_audit_logs(args)

        self.assertEqual(code, 0)
        self.assertTrue(captured["path"].startswith("/api/admin/audit-logs?"))
        for part in [
            "actorType=user",
            "actorId=user-1",
            "actorRole=admin",
            "action=site.pin",
            "result=success",
            "siteCode=demo",
            "targetType=site",
            "targetId=demo",
            "q=pinned",
            "since=2026-07-06T00%3A00%3A00Z",
            "until=2026-07-07T00%3A00%3A00Z",
            "page=2",
            "pageSize=25",
        ]:
            self.assertIn(part, captured["path"])
        self.assertEqual(captured["method"], "GET")

    def test_admin_reuse_policy_sends_policy_payload(self):
        captured = {}
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="admin-token",
            code="demo site",
            reuse="deny",
            source_download="allow",
        )

        def fake_request_json(base, token, path, method="GET", payload=None):
            captured["base"] = base
            captured["token"] = token
            captured["path"] = path
            captured["method"] = method
            captured["payload"] = payload
            return 200, {"success": True}

        with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
            with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                code = hostctl_deploy.cmd_admin_reuse_policy(args)

        self.assertEqual(code, 0)
        self.assertEqual(captured["base"], "https://pagepilot.example.com")
        self.assertEqual(captured["token"], "admin-token")
        self.assertEqual(captured["path"], "/api/admin/sites/demo%20site/reuse-policy")
        self.assertEqual(captured["method"], "PATCH")
        self.assertEqual(captured["payload"], {
            "reusePolicy": "deny",
            "sourceDownloadPolicy": "allow",
        })

    def test_admin_security_mode_sends_mode_payload(self):
        captured = {}
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="admin-token",
            code="demo site",
            mode="compatible",
        )

        def fake_request_json(base, token, path, method="GET", payload=None):
            captured["base"] = base
            captured["token"] = token
            captured["path"] = path
            captured["method"] = method
            captured["payload"] = payload
            return 200, {"success": True}

        with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
            with mock.patch.object(hostctl_deploy, "print_result", return_value=0):
                code = hostctl_deploy.cmd_admin_security_mode(args)

        self.assertEqual(code, 0)
        self.assertEqual(captured["base"], "https://pagepilot.example.com")
        self.assertEqual(captured["token"], "admin-token")
        self.assertEqual(captured["path"], "/api/admin/sites/demo%20site/security-mode")
        self.assertEqual(captured["method"], "PATCH")
        self.assertEqual(captured["payload"], {"securityMode": "compatible"})


class PreflightTests(unittest.TestCase):
    def test_preflight_rejects_html_that_server_would_reject(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "index.html"
            source.write_text("<html></html>", encoding="utf-8")

            report = hostctl_deploy.preflight_source(str(source))

        self.assertFalse(report["success"])
        self.assertEqual(report["mainEntry"], "index.html")
        self.assertIn("INVALID_INPUT", [item["code"] for item in report["errors"]])

    def test_preflight_accepts_valid_html_after_trimmed_length_check(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "index.html"
            source.write_text("\n  <!doctype html><html><body><main>ok</main></body></html>  \n", encoding="utf-8")

            report = hostctl_deploy.preflight_source(str(source))

        self.assertTrue(report["success"])
        self.assertEqual(report["mainEntry"], "index.html")

    def test_preflight_directory_reports_detected_bundle(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "site"
            (root / "project" / "dist" / "assets").mkdir(parents=True)
            index = "<!doctype html><html><body><h1>Demo</h1></body></html>"
            css = "body { color: #075985; }"
            (root / "project" / "dist" / "index.html").write_text(index, encoding="utf-8")
            (root / "project" / "dist" / "assets" / "app.css").write_text(css, encoding="utf-8")
            (root / "project" / "README.md").write_text("# Wrapper", encoding="utf-8")

            report = hostctl_deploy.preflight_source(str(root))

        self.assertTrue(report["success"])
        self.assertEqual(report["source"], str(root))
        self.assertEqual(report["sourceType"], "directory")
        self.assertEqual(report["kind"], "zip_site")
        self.assertEqual(report["root"], "project/dist")
        self.assertEqual(report["mainEntry"], "index.html")
        self.assertEqual(report["count"], 2)
        self.assertEqual(report["bytes"], len(index.encode("utf-8")) + len(css.encode("utf-8")))
        self.assertEqual([item["path"] for item in report["files"]], ["assets/app.css", "index.html"])
        self.assertEqual(report["warnings"], [])
        self.assertEqual(report["errors"], [])

    def test_preflight_zip_rejects_unsafe_path(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "unsafe.zip"
            with zipfile.ZipFile(source, "w") as archive:
                archive.writestr("../index.html", "<!doctype html><html><body><h1>Unsafe</h1></body></html>")

            report = hostctl_deploy.preflight_source(str(source))

        self.assertFalse(report["success"])
        self.assertEqual(report["sourceType"], "zip")
        self.assertIn("ZIP_UNSAFE_PATH", [item["code"] for item in report["errors"]])

    def test_preflight_zip_rejects_oversized_outer_archive(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "large.zip"
            with zipfile.ZipFile(source, "w", compression=zipfile.ZIP_STORED) as archive:
                for index in range(99):
                    archive.writestr(f"assets/{index:03d}.bin", bytes([index]) * (11 * 1024))
                archive.writestr("index.html", "<html></html>")

            self.assertGreater(source.stat().st_size, hostctl_deploy.MAX_SINGLE_FILE_BYTES)
            report = hostctl_deploy.preflight_source(str(source))

        self.assertFalse(report["success"])
        self.assertIn("ZIP_FILE_TOO_LARGE", [item["code"] for item in report["errors"]])

    def test_preflight_directory_rejects_oversized_generated_archive(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "site"
            (root / "assets").mkdir(parents=True)
            (root / "index.html").write_text("<html></html>", encoding="utf-8")
            for index in range(99):
                (root / "assets" / f"{index:03d}.bin").write_bytes(os.urandom(11 * 1024))

            report = hostctl_deploy.preflight_source(str(root))

        self.assertFalse(report["success"])
        self.assertIn("ZIP_FILE_TOO_LARGE", [item["code"] for item in report["errors"]])

    def test_preflight_zip_requires_one_entry_root_unless_filename_is_explicit(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "multiple-sites.zip"
            with zipfile.ZipFile(source, "w") as archive:
                archive.writestr("one/index.html", "<!doctype html><html><body><h1>One</h1></body></html>")
                archive.writestr("two/index.html", "<!doctype html><html><body><h1>Two</h1></body></html>")

            ambiguous = hostctl_deploy.preflight_source(str(source))
            explicit = hostctl_deploy.preflight_source(str(source), "one/index.html")

        self.assertFalse(ambiguous["success"])
        self.assertIn("ZIP_AMBIGUOUS_ENTRY", [item["code"] for item in ambiguous["errors"]])
        self.assertTrue(explicit["success"])
        self.assertEqual(explicit["root"], "one")
        self.assertEqual(explicit["mainEntry"], "index.html")

    def test_preflight_normalizes_windows_entry_hint_before_validating(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "index.html"
            source.write_text("<!doctype html><html><body><main>Demo</main></body></html>", encoding="utf-8")

            report = hostctl_deploy.preflight_source(str(source), r"..\index.html")

        self.assertFalse(report["success"])
        self.assertIn("UNSAFE_ENTRY_PATH", [item["code"] for item in report["errors"]])

    def test_preflight_matches_server_path_character_rules(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "index.html"
            source.write_text(
                "<!doctype html><html><body><main>Demo</main></body></html>", encoding="utf-8"
            )

            for hint in ["assets/app?.js", "CON/index.html", "CON.foo.bar/index.html", "/index.html", "a/" * 16 + "index.html"]:
                report = hostctl_deploy.preflight_source(str(source), hint)
                self.assertFalse(report["success"], hint)
                self.assertIn("UNSAFE_ENTRY_PATH", [item["code"] for item in report["errors"]], hint)

            for hint in ["foo bar/index.html", "assets/logo@2x.png", "fonts/Inter (1).woff2", "js/app+polyfills.js"]:
                report = hostctl_deploy.preflight_source(str(source), hint)
                self.assertTrue(report["success"], (hint, report))

    def test_preflight_enforces_existing_single_file_limit(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "site"
            root.mkdir()
            (root / "index.html").write_text(
                "<!doctype html><html><body><h1>Demo</h1></body></html>", encoding="utf-8"
            )
            (root / "assets.bin").write_bytes(b"x" * (hostctl_deploy.MAX_SINGLE_FILE_BYTES + 1))

            report = hostctl_deploy.preflight_source(str(root))

        self.assertFalse(report["success"])
        self.assertIn("ZIP_FILE_TOO_LARGE", [item["code"] for item in report["errors"]])

    def test_preflight_and_directory_packer_ignore_archive_metadata(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "site"
            root.mkdir()
            (root / "index.html").write_text(
                "<!doctype html><html><body><main>Demo</main></body></html>", encoding="utf-8"
            )
            for index in range(hostctl_deploy.MAX_FILES_PER_SITE + 1):
                metadata = root / f"cache-{index:03d}" / ".DS_Store"
                metadata.parent.mkdir()
                metadata.write_bytes(b"")

            report = hostctl_deploy.preflight_source(str(root))
            archive, _, cleanup = hostctl_deploy.prepare_multipart_source(str(root))
            try:
                with zipfile.ZipFile(archive) as packed:
                    self.assertEqual(packed.namelist(), ["index.html"])
            finally:
                cleanup()

        self.assertTrue(report["success"])
        self.assertEqual(report["count"], 1)


class GetCommandTests(unittest.TestCase):
    def test_download_output_directory_matches_go_cli_and_keeps_json_machine_readable(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="",
            code="project-home",
            version="",
            download=True,
            output="",
        )

        class FakeResponse:
            status = 200
            headers = {"Content-Type": "application/zip"}

            def __enter__(self):
                return self

            def __exit__(self, exc_type, exc, tb):
                return False

            def read(self):
                return b"zip-data"

        with tempfile.TemporaryDirectory() as tmp:
            args.output = str(pathlib.Path(tmp) / "downloads")
            output = io.StringIO()
            with mock.patch.object(hostctl_deploy, "auth_token", return_value=""):
                with mock.patch.object(hostctl_deploy.urllib.request, "urlopen", return_value=FakeResponse()):
                    with mock.patch.object(hostctl_deploy, "JSON_OUTPUT", True):
                        with redirect_stdout(output):
                            self.assertEqual(hostctl_deploy.cmd_get(args), 0)

            target = pathlib.Path(args.output) / "project-home.zip"
            self.assertEqual(target.read_bytes(), b"zip-data")

        report = json.loads(output.getvalue())
        self.assertTrue(report["success"])
        self.assertEqual(report["output"], str(target))
        self.assertEqual(report["bytes"], 8)

    def test_download_json_error_is_structured(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="",
            code="missing",
            version="",
            download=True,
            output="/tmp/pagepilot-download.zip",
        )
        error = hostctl_deploy.urllib.error.HTTPError(
            "https://pagepilot.example.com/api/deploy/content",
            404,
            "Not Found",
            None,
            io.BytesIO(b'{"success":false,"errorCode":"NOT_FOUND","detail":"missing"}'),
        )
        output = io.StringIO()

        try:
            with mock.patch.object(hostctl_deploy, "auth_token", return_value=""):
                with mock.patch.object(hostctl_deploy.urllib.request, "urlopen", side_effect=error):
                    with mock.patch.object(hostctl_deploy, "JSON_OUTPUT", True):
                        with redirect_stdout(output):
                            self.assertEqual(hostctl_deploy.cmd_get(args), 1)
        finally:
            error.close()

        report = json.loads(output.getvalue())
        self.assertFalse(report["success"])
        self.assertEqual(report["httpStatus"], 404)
        self.assertEqual(report["errorCode"], "NOT_FOUND")

    def test_download_json_requires_output_path(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="",
            code="project-home",
            version="",
            download=True,
            output="",
        )
        output = io.StringIO()

        with mock.patch.object(hostctl_deploy, "JSON_OUTPUT", True):
            with redirect_stdout(output):
                self.assertEqual(hostctl_deploy.cmd_get(args), 1)

        report = json.loads(output.getvalue())
        self.assertEqual(report["errorCode"], "OUTPUT_REQUIRED")


class PreflightParserTests(unittest.TestCase):
    def test_preflight_parser_prints_json_without_network_calls(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "index.html"
            source.write_text("<!doctype html><html><body><h1>Demo</h1></body></html>", encoding="utf-8")
            args = hostctl_deploy.build_parser().parse_args(["preflight", str(source)])
            output = io.StringIO()

            with redirect_stdout(output):
                result = args.func(args)

        report = json.loads(output.getvalue())
        self.assertEqual(result, 0)
        self.assertTrue(report["success"])
        self.assertEqual(report["kind"], "single_html")
        self.assertEqual(report["mainEntry"], "index.html")

    def test_parser_accepts_global_server_after_subcommand(self):
        parser = hostctl_deploy.build_parser()
        args = hostctl_deploy.parse_cli_args(
            parser,
            ["preflight", "site", "--server", "https://pagepilot.example.com"],
        )

        self.assertEqual(args.server, "https://pagepilot.example.com")
        self.assertEqual(args.source, "site")

    def test_parser_accepts_json_after_subcommand(self):
        parser = hostctl_deploy.build_parser()
        args = hostctl_deploy.parse_cli_args(parser, ["version", "--json"])

        self.assertTrue(args.json)
        hostctl_deploy.JSON_OUTPUT = False

    def test_json_parser_errors_are_machine_readable(self):
        parser = hostctl_deploy.build_parser()
        output = io.StringIO()

        with redirect_stdout(output):
            with self.assertRaises(SystemExit) as raised:
                hostctl_deploy.parse_cli_args(parser, ["version", "--json", "--unknown"])

        self.assertEqual(raised.exception.code, 2)
        report = json.loads(output.getvalue())
        self.assertFalse(report["success"])
        self.assertEqual(report["errorCode"], "CLI_USAGE")
        hostctl_deploy.JSON_OUTPUT = False


class DoctorCommandTests(unittest.TestCase):
    def test_doctor_without_token_does_not_create_anonymous_session(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="",
            require_admin=False,
        )
        paths = []

        def fake_request_json(base, token, path, method="GET", payload=None, session_id="", agent=None):
            paths.append(path)
            return {
                "/api/health": (200, {"success": True, "status": "ok"}),
                "/api/config": (200, {
                    "success": True,
                    "mode": "prod",
                    "limits": {"maxSingleFileBytes": 123, "maxSiteTotalBytes": 456, "maxFilesPerSite": 7},
                }),
                "/openapi.json": (200, {"openapi": "3.1.0"}),
            }[path]

        output = io.StringIO()
        with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
            with redirect_stdout(output):
                result = hostctl_deploy.cmd_doctor(args)

        report = json.loads(output.getvalue())
        self.assertEqual(result, 0)
        self.assertNotIn("/api/session", paths)
        anonymous = next(item for item in report["checks"] if item["name"] == "anonymous_session")
        self.assertTrue(anonymous["ok"])
        self.assertTrue(anonymous["skipped"])
        self.assertEqual(report["uploadLimits"]["maxFilesPerSite"], 7)

    def test_doctor_accepts_a_user_token_without_requiring_admin(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="user-token",
            require_admin=False,
        )

        def fake_request_json(base, token, path, method="GET", payload=None, session_id="", agent=None):
            self.assertEqual(base, "https://pagepilot.example.com")
            responses = {
                "/api/health": (200, {"success": True, "status": "ok"}),
                "/api/config": (200, {"success": True, "mode": "prod"}),
                "/api/tokens": (200, {"success": True, "tokens": []}),
                "/api/admin/session": (403, {"success": False, "errorCode": "FORBIDDEN"}),
                "/openapi.json": (200, {"openapi": "3.1.0"}),
            }
            if path == "/api/tokens":
                self.assertEqual(token, "user-token")
            return responses[path]

        output = io.StringIO()
        with mock.patch.object(hostctl_deploy, "load_agent_identity", return_value={"agentId": "agent-test"}):
            with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
                with redirect_stdout(output):
                    result = hostctl_deploy.cmd_doctor(args)

        report = json.loads(output.getvalue())
        self.assertEqual(result, 0)
        self.assertTrue(report["success"])
        credential = next(item for item in report["checks"] if item["name"] == "credential")
        self.assertTrue(credential["ok"])

    def test_doctor_rejects_a_valid_user_token_when_admin_is_required(self):
        args = types.SimpleNamespace(
            server="https://pagepilot.example.com",
            token="user-token",
            require_admin=True,
        )

        def fake_request_json(base, token, path, method="GET", payload=None, session_id="", agent=None):
            self.assertEqual(base, "https://pagepilot.example.com")
            responses = {
                "/api/health": (200, {"success": True, "status": "ok"}),
                "/api/config": (200, {"success": True, "mode": "prod"}),
                "/api/tokens": (200, {"success": True, "tokens": []}),
                "/api/admin/session": (200, {
                    "success": True,
                    "isAdmin": False,
                    "username": "regular-user",
                }),
                "/openapi.json": (200, {"openapi": "3.1.0"}),
            }
            if path in {"/api/tokens", "/api/admin/session"}:
                self.assertEqual(token, "user-token")
            return responses[path]

        output = io.StringIO()
        with mock.patch.object(hostctl_deploy, "load_agent_identity", return_value={"agentId": "agent-test"}):
            with mock.patch.object(hostctl_deploy, "request_json", fake_request_json):
                with redirect_stdout(output):
                    result = hostctl_deploy.cmd_doctor(args)

        report = json.loads(output.getvalue())
        self.assertEqual(result, 1)
        self.assertFalse(report["success"])
        admin = next(item for item in report["checks"] if item["name"] == "admin_session")
        self.assertFalse(admin["ok"])
        self.assertEqual(admin["detail"], "configured token is not an administrator token")
        self.assertIn("not an admin token", report["hint"])


class MultipartSourceTests(unittest.TestCase):
    def test_prepare_multipart_source_rejects_oversized_single_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = pathlib.Path(tmp) / "large.html"
            source.write_bytes(b"x" * (hostctl_deploy.MAX_SINGLE_FILE_BYTES + 1))

            with self.assertRaises(SystemExit) as raised:
                hostctl_deploy.prepare_multipart_source(str(source))

        self.assertIn("single-file limit", str(raised.exception))

    def test_prepare_multipart_source_zips_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "site"
            (root / "assets").mkdir(parents=True)
            (root / "index.html").write_text("<!doctype html><title>demo</title>", encoding="utf-8")
            (root / "assets" / "app.css").write_text("body{color:red}", encoding="utf-8")

            source_path, upload_name, cleanup = hostctl_deploy.prepare_multipart_source(str(root))
            try:
                self.assertEqual(upload_name, "site.zip")
                self.assertTrue(source_path.exists())
                with zipfile.ZipFile(source_path) as zf:
                    self.assertEqual(sorted(zf.namelist()), ["assets/app.css", "index.html"])
            finally:
                cleanup()
            self.assertFalse(source_path.exists())

    def test_prepare_multipart_source_rejects_backslash_traversal_name(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "site"
            root.mkdir()
            (root / "index.html").write_text(
                "<!doctype html><html><body><main>Demo</main></body></html>", encoding="utf-8"
            )
            (root / r"evil\..\asset.js").write_text("alert(1)", encoding="utf-8")

            # Keep the generated mkstemp archive in this isolated directory so
            # an early rel_path() failure can be checked for cleanup.
            with mock.patch.object(hostctl_deploy.tempfile, "tempdir", tmp):
                with self.assertRaises(SystemExit) as raised:
                    hostctl_deploy.prepare_multipart_source(str(root))
            self.assertEqual(list(pathlib.Path(tmp).glob("pagepilot-*.zip")), [])

        self.assertIn("unsafe path", str(raised.exception).lower())

    def test_preflight_and_packer_handle_pre_1980_mtime(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp) / "site"
            root.mkdir()
            index = root / "index.html"
            index.write_text(
                "<!doctype html><html><body><main>Demo</main></body></html>", encoding="utf-8"
            )
            os.utime(index, (0, 0))

            report = hostctl_deploy.preflight_source(str(root))
            self.assertTrue(report["success"], report)
            source_path, _, cleanup = hostctl_deploy.prepare_multipart_source(str(root))
            try:
                with zipfile.ZipFile(source_path) as archive:
                    self.assertEqual(archive.read("index.html"), index.read_bytes())
            finally:
                cleanup()

if __name__ == "__main__":
    unittest.main()
