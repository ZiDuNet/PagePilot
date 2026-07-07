import importlib.util
import io
import json
import pathlib
import tempfile
import types
import unittest
from unittest import mock
import zipfile


SCRIPT = pathlib.Path(__file__).with_name("hostctl_deploy.py")
SKILL_DOC = SCRIPT.parent.parent / "SKILL.md"
SPEC = importlib.util.spec_from_file_location("hostctl_deploy", SCRIPT)
hostctl_deploy = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(hostctl_deploy)


class SkillDocumentationTests(unittest.TestCase):
    def test_skill_documents_multipart_overwrite_contract(self):
        text = SKILL_DOC.read_text(encoding="utf-8")

        self.assertIn("覆盖版本", text)
        self.assertIn("multipart", text)
        self.assertIn("不要在覆盖版本时把文件塞进 JSON/base64", text)


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


class MultipartSourceTests(unittest.TestCase):
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

if __name__ == "__main__":
    unittest.main()
