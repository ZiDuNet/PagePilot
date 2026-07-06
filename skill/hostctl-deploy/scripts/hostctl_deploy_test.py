import importlib.util
import json
import pathlib
import tempfile
import unittest
from unittest import mock
import zipfile


SCRIPT = pathlib.Path(__file__).with_name("hostctl_deploy.py")
SPEC = importlib.util.spec_from_file_location("hostctl_deploy", SCRIPT)
hostctl_deploy = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
SPEC.loader.exec_module(hostctl_deploy)


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

    def test_source_entry_hint_reads_zip_entries(self):
        with tempfile.TemporaryDirectory() as tmp:
            zip_path = pathlib.Path(tmp) / "site.zip"
            with zipfile.ZipFile(zip_path, "w") as zf:
                zf.writestr("docs/readme.md", "# demo")
                zf.writestr("index.html", "<!doctype html>")

            self.assertEqual(hostctl_deploy.source_entry_hint(str(zip_path)), "index.html")


if __name__ == "__main__":
    unittest.main()
