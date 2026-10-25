#!/usr/bin/env python3
"""Exercise the CI bootstrap without installing SDKs or using sudo."""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class CIProvisionTest(unittest.TestCase):
    def test_bootstrap_uses_pinned_python_and_publishes_it_on_path(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            result, calls = self.run_bootstrap(base)
            self.assertEqual(result.returncode, 0, result.stderr)
            version = json.loads((ROOT / "tools/manifest.json").read_text())[
                "versions"
            ]["python"]
            python = str(base / "owned python/bin/python3")
            self.assertEqual(
                [call[1:] for call in calls if call[0] == "uv"],
                [["python", "install", version], ["python", "find", version]],
            )
            sudo = [call[1:] for call in calls if call[0] == "sudo"]
            self.assertEqual(sudo[0][0], python)
            self.assertEqual(
                sudo[1],
                [
                    "bash",
                    str(ROOT / "scripts/provision-analyzers.sh"),
                    "/opt/archfit",
                    python,
                ],
            )
            self.assertIn(
                str(Path(python).parent), (base / "path").read_text().splitlines()
            )
            self.assertIn(
                "ARCHFIT_TOOL_PYTHON=/opt/archfit/python/bin/python",
                (base / "env").read_text().splitlines(),
            )

    def test_python_failure_stops_before_privileged_provisioning(self):
        for command in ("install", "find"):
            with (
                self.subTest(command=command),
                tempfile.TemporaryDirectory() as directory,
            ):
                result, calls = self.run_bootstrap(Path(directory), command)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(call[0] == "sudo" for call in calls))

    def run_bootstrap(self, base, fail=""):
        bin_dir = base / "bin"
        bin_dir.mkdir()
        (bin_dir / "python3").symlink_to(sys.executable)
        boundary = bin_dir / "boundary"
        boundary.write_text(
            f"#!{sys.executable}\n"
            "import json, os, sys\n"
            "from pathlib import Path\n"
            "name = Path(sys.argv[0]).name\n"
            "with open(os.environ['CALLS'], 'a') as out:\n"
            "    out.write(json.dumps([name, *sys.argv[1:]]) + '\\n')\n"
            "if name == 'uname': print('x86_64')\n"
            "if name == 'uv':\n"
            "    if sys.argv[1:3] == ['python', os.environ['FAIL_UV']]: sys.exit(9)\n"
            "    if sys.argv[1:3] == ['python', 'find']: print(os.environ['OWNED_PYTHON'])\n",
            encoding="utf-8",
        )
        boundary.chmod(0o755)
        for name in ("sudo", "uv", "uname"):
            (bin_dir / name).symlink_to(boundary)
        calls = base / "calls"
        env = {
            **os.environ,
            "PATH": f"{bin_dir}{os.pathsep}{os.defpath}",
            "GITHUB_ENV": str(base / "env"),
            "GITHUB_PATH": str(base / "path"),
            "CALLS": str(calls),
            "OWNED_PYTHON": str(base / "owned python/bin/python3"),
            "FAIL_UV": fail,
            "UV_PYTHON": "0.0.0",
        }
        result = subprocess.run(
            ["bash", str(ROOT / "scripts/ci-provision-tools.sh")],
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )
        return result, [json.loads(line) for line in calls.read_text().splitlines()]


if __name__ == "__main__":
    unittest.main()
