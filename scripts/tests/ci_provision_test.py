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
SCRIPT = ROOT / "scripts/ci-provision-tools.sh"
BOUNDARY = r"""#!{python}
import json, os, sys
from pathlib import Path

name = Path(sys.argv[0]).name
with open(os.environ["CALLS"], "a") as out:
    out.write(json.dumps([name, *sys.argv[1:]]) + "\n")
if name == "uname":
    print("x86_64")
if name == "uv" and sys.argv[1:3] == ["python", os.environ["FAIL_UV"]]:
    sys.exit(9)
if name == "uv" and sys.argv[1:3] == ["python", "find"]:
    print(os.environ["OWNED_PYTHON"])
if name == "bash":
    venv = Path(sys.argv[2]) / "python/bin"
    venv.mkdir(parents=True, exist_ok=True)
    (venv / "python").unlink(missing_ok=True)
    (venv / "python").symlink_to(sys.argv[3])
"""


class CIProvisionTest(unittest.TestCase):
    def test_install_builds_the_locked_toolchain_without_sudo(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            prefix = base / "toolchain"
            result, calls, env, path = self.run_bootstrap(base, "install", prefix)
            self.assertEqual(result.returncode, 0, result.stderr)
            version = self.manifest_python()
            python = str(base / "owned python/bin/python3")
            self.assertEqual(
                [call[1:] for call in calls if call[0] == "uv"],
                [["python", "install", version], ["python", "find", version]],
            )
            self.assertEqual(
                [call[1:] for call in calls if call[0] == "python3"],
                [
                    [
                        str(ROOT / "scripts/provision_tools.py"),
                        "sdks",
                        "--arch",
                        "amd64",
                        "--prefix",
                        str(prefix),
                    ]
                ],
            )
            self.assertEqual(
                [call[1:] for call in calls if call[0] == "bash"],
                [[str(ROOT / "scripts/provision-analyzers.sh"), str(prefix), python]],
            )
            self.assertNotIn("sudo", [call[0] for call in calls])
            entries = path.read_text().splitlines()
            self.assertEqual(
                entries[:5],
                [
                    str(prefix / "analyzers/node/node_modules/.bin"),
                    str(prefix / "bin"),
                    str(prefix / "go/bin"),
                    str(prefix / "node/bin"),
                    str(prefix / "rust/bin"),
                ],
            )
            # The base interpreter is published by its resolved directory: the
            # venv entry is a symlink into uv's tree, as in the shipped image.
            self.assertEqual(
                Path(entries[5]).resolve(), (base / "owned python/bin").resolve()
            )
            self.assertEqual(
                env.read_text().splitlines(),
                [
                    "ARCHFIT_TOOL_PYTHON=" + str(prefix / "python/bin/python"),
                    "GOTOOLCHAIN=local",
                    "CARGO_HOME=/tmp/archfit-cargo",
                    "RUST_SRC_PATH="
                    + str(prefix / "rust/lib/rustlib/src/rust/library"),
                    "UV_PYTHON_DOWNLOADS=never",
                ],
            )

    def test_cache_hit_skips_install_and_only_exports_the_restored_tree(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            prefix = self.fake_toolchain(base)
            result, calls, env, path = self.run_bootstrap(
                base, "auto", prefix, cache_hit="true"
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls, [])
            self.assertIn(
                "ARCHFIT_TOOL_PYTHON=" + str(prefix / "python/bin/python"),
                env.read_text().splitlines(),
            )
            self.assertEqual(
                Path(path.read_text().splitlines()[-1]).resolve(),
                (base / "owned python/bin").resolve(),
            )

    def test_unusable_cache_is_rebuilt_instead_of_trusted(self):
        for broken in ("cargo-modules", "python"):
            with (
                self.subTest(broken=broken),
                tempfile.TemporaryDirectory() as directory,
            ):
                base = Path(directory)
                prefix = self.fake_toolchain(base, broken=broken)
                result, calls, _, _ = self.run_bootstrap(
                    base, "auto", prefix, cache_hit="true"
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("uv", [call[0] for call in calls])
                self.assertIn("bash", [call[0] for call in calls])

    def test_export_needs_no_downloads_and_publishes_the_go_defaults(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            prefix = base / "toolchain"
            result, calls, env, _ = self.run_bootstrap(base, "export", prefix)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(calls, [])
            lines = env.read_text().splitlines()
            self.assertFalse([line for line in lines if line.startswith("GOCACHE=")])
            self.assertFalse([line for line in lines if line.startswith("GOPATH=")])

    def test_missing_prefix_fails_before_any_work(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            result, calls, _, _ = self.run_bootstrap(base, "install", None)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("ARCHFIT_PREFIX", result.stderr)
            self.assertEqual(calls, [])

    def test_python_failure_stops_before_analyzer_provisioning(self):
        for command in ("install", "find"):
            with (
                self.subTest(command=command),
                tempfile.TemporaryDirectory() as directory,
            ):
                base = Path(directory)
                result, calls, _, _ = self.run_bootstrap(
                    base, "install", base / "toolchain", fail=command
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("bash", [call[0] for call in calls])

    def manifest_python(self):
        manifest = json.loads((ROOT / "tools/manifest.json").read_text())
        return manifest["versions"]["python"]

    def fake_toolchain(self, base, broken=None):
        prefix = base / "toolchain"
        for name in (
            "bin/cargo-modules",
            "bin/scip-go",
            "rust/bin/rustc",
            "analyzers/node/node_modules/.bin/sg",
        ):
            target = prefix / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
            if name.rsplit("/", 1)[-1] != broken:
                target.chmod(0o755)
        interpreter = base / "owned python/bin/python3.14"
        interpreter.parent.mkdir(parents=True, exist_ok=True)
        interpreter.write_text(
            f"#!{sys.executable}\nimport sys\nsys.exit({1 if broken == 'python' else 0})\n",
            encoding="utf-8",
        )
        interpreter.chmod(0o755)
        venv = prefix / "python/bin"
        venv.mkdir(parents=True, exist_ok=True)
        (venv / "python").symlink_to(interpreter)
        return prefix

    def run_bootstrap(self, base, mode, prefix, cache_hit="", fail=""):
        bin_dir = base / "bin"
        bin_dir.mkdir()
        (bin_dir / "python3").symlink_to(sys.executable)
        boundary = BOUNDARY.format(python=sys.executable)
        (bin_dir / "boundary").write_text(boundary, encoding="utf-8")
        (bin_dir / "boundary").chmod(0o755)
        for name in ("bash", "sudo", "uv", "uname"):
            (bin_dir / name).symlink_to(bin_dir / "boundary")
        owned = base / "owned python/bin/python3"
        owned.parent.mkdir(parents=True, exist_ok=True)
        owned.write_text(boundary, encoding="utf-8")
        owned.chmod(0o755)
        calls = base / "calls"
        calls.write_text("", encoding="utf-8")
        env = {
            **os.environ,
            "PATH": f"{bin_dir}{os.pathsep}{os.defpath}",
            "GITHUB_ENV": str(base / "env"),
            "GITHUB_PATH": str(base / "path"),
            "CALLS": str(calls),
            "OWNED_PYTHON": str(owned),
            "FAIL_UV": fail,
            "UV_PYTHON": "0.0.0",
            "ARCHFIT_TOOLCHAIN_CACHE_HIT": cache_hit,
        }
        env.pop("ARCHFIT_PREFIX", None)
        if prefix is not None:
            env["ARCHFIT_PREFIX"] = str(prefix)
        result = subprocess.run(
            ["/bin/bash", str(SCRIPT), mode],
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )
        return (
            result,
            [json.loads(line) for line in calls.read_text().splitlines()],
            base / "env",
            base / "path",
        )


if __name__ == "__main__":
    unittest.main()

