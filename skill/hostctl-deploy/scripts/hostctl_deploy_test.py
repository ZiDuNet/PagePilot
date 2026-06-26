import importlib.util
import pathlib
import unittest
from unittest import mock


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
            captured["origin"] = req.headers.get("X-hostctl-public-origin")
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


if __name__ == "__main__":
    unittest.main()
