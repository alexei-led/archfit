#!/usr/bin/env python3
"""Build-only Go SDK and final-image regression checks without a container."""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import image_contracts as image
import provision_tools as provision


class GoRuntimeTest(unittest.TestCase):
    def test_runtime_preparation_removes_only_the_build_sdk(self):
        manifest = json.loads((provision.ROOT / "tools/manifest.json").read_text())
        self.assertEqual(manifest["build_only"], ["go"])
        for arch in ("amd64", "arm64"):
            self.assertIn(
                manifest["versions"]["go"], manifest["artifacts"][arch]["go"]["url"]
            )
        with tempfile.TemporaryDirectory() as temporary:
            prefix = Path(temporary)
            for name in (
                "go/bin/go",
                "go/src/runtime/runtime.go",
                "node/bin/node",
                "bin/cargo-modules",
            ):
                path = prefix / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(name)
            provision.prepare_runtime(provision.ROOT, prefix)
            self.assertFalse((prefix / "go").exists())
            self.assertEqual((prefix / "node/bin/node").read_text(), "node/bin/node")
            self.assertEqual((prefix / "bin/cargo-modules").read_text(), "bin/cargo-modules")

    def test_image_guard_rejects_either_go_executable_or_sdk(self):
        for executable, sdk in (
            (False, False),
            (True, False),
            (False, True),
            (True, True),
        ):
            with (
                self.subTest(executable=executable, sdk=sdk),
                tempfile.TemporaryDirectory() as temporary,
            ):
                root = Path(temporary)
                binary = root / "bin"
                binary.mkdir()
                if executable:
                    go = binary / "go"
                    go.write_text("#!/bin/sh\nexit 0\n")
                    go.chmod(0o755)
                if sdk:
                    (root / "go/src").mkdir(parents=True)
                script = (
                    image.NO_GO.replace("/opt/archfit/go", str(root / "go"))
                    + "printf passed"
                )
                result = subprocess.run(
                    ["/bin/sh", "-ec", script],
                    env={**os.environ, "PATH": str(binary)},
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertEqual(
                    result.returncode, int(executable or sdk), result.stderr
                )
                self.assertEqual(result.stdout, "" if executable or sdk else "passed")

    def test_docker_build_prunes_go_before_copying_the_runtime(self):
        dockerfile = (provision.ROOT / "Dockerfile").read_text()
        builder, runtime = dockerfile.rsplit("\nFROM ", 1)
        self.assertIn("go build -trimpath", builder)
        self.assertIn("&& python scripts/provision_tools.py runtime", builder)
        self.assertIn("COPY --from=tools-builder /opt/archfit /opt/archfit", runtime)
        for value in ("/opt/archfit/go", "GOTOOLCHAIN=", "GOCACHE=", "GOPATH="):
            self.assertNotIn(value, runtime)


if __name__ == "__main__":
    unittest.main()
