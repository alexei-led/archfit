#!/usr/bin/env python3
"""The clean image check must retain a real change to compare with main."""

import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import image_contracts as image


class GoImageChangeTest(unittest.TestCase):
    def test_clean_go_case_restores_files_without_switching_to_the_base(self):
        spec = next(spec for spec in image.lc.FIXTURES if spec.name == "go")
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary)
            contract = image.ImageContract("local:contract", "unused", output)
            with (
                patch.object(image.lc, "FIXTURES", (spec,)),
                patch.object(
                    contract, "run", side_effect=RuntimeError("capture script")
                ) as run,
                self.assertRaisesRegex(RuntimeError, "capture script"),
            ):
                image.language_cases(contract)
            _, _, script, fixture = run.call_args.args

            def git(*args):
                return subprocess.run(
                    ["git", "-C", str(fixture), *args],
                    capture_output=True,
                    text=True,
                    check=True,
                ).stdout.strip()

            change_head = git("rev-parse", "HEAD")
            self.assertNotEqual(change_head, git("rev-parse", image.lc.TARGET_BRANCH))
            destination = output / "out"
            destination.mkdir()
            script = script.replace("/out/", str(destination) + "/")
            script = script.replace("/opt/archfit/go", str(output / "absent-sdk"))
            git_binary = subprocess.run(
                ["/bin/sh", "-c", "command -v git"],
                capture_output=True,
                text=True,
                check=True,
            ).stdout.strip()
            binary = output / "bin"
            binary.mkdir()
            (binary / "git").symlink_to(git_binary)
            result = subprocess.run(
                ["/bin/sh", "-ec", "archfit() { :; }; " + script],
                cwd=fixture,
                env={**os.environ, "PATH": str(binary)},
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(git("rev-parse", "HEAD"), change_head)
            self.assertEqual(git("branch", "--show-current"), image.lc.CHANGE_BRANCH)
            for name in spec.forbidden:
                self.assertEqual((fixture / name).read_text(), spec.files[name])
            self.assertEqual(
                git("diff", image.lc.TARGET_BRANCH, "--", *spec.forbidden), ""
            )


if __name__ == "__main__":
    unittest.main()
