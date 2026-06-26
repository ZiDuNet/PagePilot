import importlib.util
import pathlib
import unittest


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


if __name__ == "__main__":
    unittest.main()
