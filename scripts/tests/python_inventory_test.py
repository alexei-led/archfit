"""The final image keeps SCIP's locked reader, but never grimp."""

import importlib.metadata
import json
import shlex
import sys
import tempfile
import types
import unittest
from pathlib import Path
from unittest.mock import mock_open, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import image_contracts as image


class PythonInventoryTest(unittest.TestCase):
    def test_python_inventory_requires_reader_pins_and_refuses_grimp(self):
        with tempfile.TemporaryDirectory() as temporary:
            contract = image.ImageContract("local:contract", "colima", Path(temporary))
            with patch.object(contract, "run") as run:
                image.inventory(contract)
            tokens = shlex.split(run.call_args.args[2])
            script = tokens[tokens.index("-c") + 1]
        pins = image.lc.TOOL_MANIFEST
        modules = {
            "google": types.ModuleType("google"),
            "google.protobuf": types.ModuleType("google.protobuf"),
            "grpc_tools": types.ModuleType("grpc_tools"),
            "grimp": types.ModuleType("grimp"),
        }
        cases = {
            "reader only": (False, pins["python"], True),
            "grimp installed": (True, pins["python"], False),
            "wrong protobuf": (False, {**pins["python"], "protobuf": "0.0"}, False),
            "wrong grpcio-tools": (
                False,
                {**pins["python"], "grpcio-tools": "0.0"},
                False,
            ),
            "missing protobuf": (
                False,
                {k: v for k, v in pins["python"].items() if k != "protobuf"},
                False,
            ),
            "missing grpcio-tools": (
                False,
                {k: v for k, v in pins["python"].items() if k != "grpcio-tools"},
                False,
            ),
        }
        for name, (grimp, versions, passes) in cases.items():

            def version(package):
                if package not in versions:
                    raise importlib.metadata.PackageNotFoundError(package)
                return versions[package]

            with (
                self.subTest(name=name),
                patch.dict(sys.modules, modules),
                patch("builtins.open", mock_open(read_data=json.dumps(pins))),
                patch(
                    "platform.python_version", return_value=pins["versions"]["python"]
                ),
                patch("importlib.metadata.version", side_effect=version),
                patch(
                    "importlib.util.find_spec", return_value=object() if grimp else None
                ),
            ):
                if passes:
                    exec(script, {})
                else:
                    with self.assertRaises(
                        (AssertionError, importlib.metadata.PackageNotFoundError)
                    ):
                        exec(script, {})


if __name__ == "__main__":
    unittest.main()
