"""The final image uses pinned Python and does not contain grimp."""

import json
import shlex
import tempfile
import unittest
from pathlib import Path
from unittest.mock import mock_open, patch
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import image_contracts as image


class PythonInventoryTest(unittest.TestCase):
    def test_python_version_and_absent_grimp(self):
        with tempfile.TemporaryDirectory() as temporary:
            contract = image.ImageContract("local:contract", "colima", Path(temporary))
            with patch.object(contract, "run") as run:
                image.inventory(contract)
            tokens = shlex.split(run.call_args.args[2])
            script = tokens[tokens.index("-c") + 1]
        pins = image.lc.TOOL_MANIFEST
        for version, grimp, valid in (
            (pins["versions"]["python"], False, True),
            ("0.0", False, False),
            (pins["versions"]["python"], True, False),
        ):
            with (
                self.subTest(version=version, grimp=grimp),
                patch("builtins.open", mock_open(read_data=json.dumps(pins))),
                patch("platform.python_version", return_value=version),
                patch("importlib.util.find_spec", return_value=object() if grimp else None),
            ):
                if valid:
                    exec(script, {})
                else:
                    with self.assertRaises(AssertionError):
                        exec(script, {})
