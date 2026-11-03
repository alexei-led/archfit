from __future__ import annotations

import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class PrePushEnvironmentTest(unittest.TestCase):
    def test_fixture_git_cannot_change_the_calling_repository(self) -> None:
        names = subprocess.check_output(
            ["git", "rev-parse", "--local-env-vars"], text=True, cwd=ROOT
        ).splitlines()
        clean = {key: value for key, value in os.environ.items() if key not in names}
        clean.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_SYSTEM=os.devnull)

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            caller = root / "caller"
            fixture = root / "fixture"
            bare = root / "bare"
            binaries = root / "bin"
            for path in (fixture, bare, binaries):
                path.mkdir()
            subprocess.run(
                ["git", "init", "-q", str(caller)],
                env=clean, check=True, capture_output=True,
            )
            (caller / "sentinel.txt").write_text("keep\n", encoding="utf-8")
            subprocess.run(
                ["git", "-C", str(caller), "add", "sentinel.txt"],
                env=clean, check=True, capture_output=True,
            )
            config = caller / ".git" / "config"
            index = caller / ".git" / "index"
            before = (config.read_bytes(), index.read_bytes())
            marker = root / "make-arguments"
            make = binaries / "make"
            make.write_text(
                "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > \"$MARKER\"\n"
                "git -C \"$FIXTURE\" init -q\n"
                "printf 'fixture\\n' > \"$FIXTURE/fixture.txt\"\n"
                "git -C \"$FIXTURE\" add --all\n"
                "git -C \"$BARE\" init -q --bare\n",
                encoding="utf-8",
            )
            make.chmod(0o700)
            env = dict(
                clean,
                PATH=f"{binaries}{os.pathsep}{clean['PATH']}",
                GIT_COMMON_DIR=str(caller / ".git"),
                GIT_INDEX_FILE=str(index),
                FIXTURE=str(fixture),
                BARE=str(bare),
                MARKER=str(marker),
            )
            result = subprocess.run(
                self.make_test_command(), cwd=ROOT, env=env,
                capture_output=True, text=True, timeout=30,
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(marker.read_text(encoding="utf-8"), "test\n")
            self.assertEqual(config.read_bytes(), before[0])
            self.assertEqual(index.read_bytes(), before[1])

    def make_test_command(self) -> list[str]:
        in_hook = False
        for line in (ROOT / ".pre-commit-config.yaml").read_text(
            encoding="utf-8"
        ).splitlines():
            field = line.strip()
            if field.startswith("- id:"):
                in_hook = field == "- id: make-test"
            elif in_hook and field.startswith("entry:"):
                return shlex.split(field.removeprefix("entry:").strip())
        self.fail("make-test hook has no command")


if __name__ == "__main__":
    unittest.main()
