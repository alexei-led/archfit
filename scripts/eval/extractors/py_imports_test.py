import shutil
import tempfile
import unittest
from pathlib import Path

from . import py_imports
from .py_imports import Import


def write_tree(root: Path, files: dict[str, str]) -> None:
    for name, body in files.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)


class ResolveTest(unittest.TestCase):
    modules = {"app", "app.a", "app.sub", "app.sub.b", "app.sub.c"}

    def test_rules(self):
        cases = [
            ("absolute import", "app.a", False, Import("app.sub.b"), ["app.sub.b"]),
            (
                "from module import name",
                "app.a",
                False,
                Import("app.sub.b", ("f",), True),
                ["app.sub.b"],
            ),
            (
                "from package import submodule",
                "app.a",
                False,
                Import("app.sub", ("c",), True),
                ["app.sub.c"],
            ),
            (
                "from package import name and submodule",
                "app.a",
                False,
                Import("app.sub", ("c", "f"), True),
                ["app.sub.c", "app.sub"],
            ),
            (
                "relative depth 1 in a module",
                "app.sub.b",
                False,
                Import(".", ("c",), True),
                ["app.sub.c"],
            ),
            (
                "relative depth 1 in a package",
                "app.sub",
                True,
                Import(".", ("b",), True),
                ["app.sub.b"],
            ),
            (
                "relative depth 2",
                "app.sub.b",
                False,
                Import("..a", ("x",), True),
                ["app.a"],
            ),
            ("wildcard", "app.a", False, Import("app.sub", (), True), ["app.sub"]),
            (
                "missing module stays named",
                "app.a",
                False,
                Import("app.gone", ("x",), True),
                ["app.gone"],
            ),
        ]
        for name, importer, is_package, imp, want in cases:
            with self.subTest(name):
                self.assertEqual(
                    py_imports.resolve(importer, is_package, imp, self.modules), want
                )

    def test_relative_import_above_the_package_raises(self):
        with self.assertRaises(ImportError):
            py_imports.resolve("app", True, Import("...x", (), True), self.modules)


class LayoutTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)

    def test_src_layout_wins_over_top_level_packages(self):
        write_tree(self.root, {"src/app/__init__.py": "", "tests/__init__.py": ""})
        self.assertEqual(py_imports.discover_packages(self.root), ["app"])
        self.assertEqual(
            py_imports.package_dir(self.root, "app"), self.root / "src" / "app"
        )

    def test_flat_layout(self):
        write_tree(self.root, {"app/__init__.py": "", "docs/x.py": ""})
        self.assertEqual(py_imports.discover_packages(self.root), ["app"])
        self.assertEqual(py_imports.package_dir(self.root, "app"), self.root / "app")

    def test_namespace_directory_inside_a_package_holds_modules(self):
        write_tree(
            self.root,
            {
                "app/__init__.py": "",
                "app/ns/inner/__init__.py": "",
                "app/ns/inner/m.py": "",
                "app/not-id/x.py": "",
            },
        )
        self.assertEqual(
            py_imports.source_files(self.root, "app"),
            {
                "app": "app/__init__.py",
                "app.ns.inner": "app/ns/inner/__init__.py",
                "app.ns.inner.m": "app/ns/inner/m.py",
            },
        )


@unittest.skipUnless(shutil.which("ast-grep"), "ast-grep is not installed")
class ExtractTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        write_tree(
            self.root,
            {
                "pyproject.toml": "",
                "src/app/__init__.py": "from . import a\n",
                "src/app/a.py": (
                    "import os\n"
                    "import requests\n"
                    "from typing import TYPE_CHECKING\n"
                    "from app.sub import (b,  # comment\n    c)\n"
                    "from .gone import x\n"
                    "if TYPE_CHECKING:\n"
                    "    from app import typed_only\n"
                    "elif typing.TYPE_CHECKING:\n"
                    "    import app.typed_too\n"
                    "else:\n"
                    "    import app.sub.c\n"
                    "def f():\n"
                    "    from app.ns.inner import m\n"
                ),
                "src/app/sub/__init__.py": "",
                "src/app/sub/b.py": "from ..a import f\n",
                "src/app/sub/c.py": "",
                "src/app/ns/inner/__init__.py": "",
                "src/app/ns/inner/m.py": "",
                "src/app/legacy/__init__.py": "import app.a\n",
            },
        )

    def extract(self, exclusions=()):
        return py_imports.extract(
            self.root, {"Exclusions": list(exclusions), "PyPackage": "", "Paths": []}
        )

    def test_edges_unknown_and_external(self):
        got = self.extract(["src/app/legacy/**"])
        edges = {(e["from"], e["to"]) for e in got["edges"]}
        self.assertEqual(
            edges,
            {
                ("app", "app.a"),
                ("app.a", "app.sub.b"),
                ("app.a", "app.sub.c"),
                ("app.a", "app.ns.inner.m"),
                ("app.sub.b", "app.a"),
            },
        )
        self.assertEqual(got["unknown"], [{"from": "app.a", "specifier": "app.gone"}])
        self.assertEqual(got["external"], 1, "requests is external; os is stdlib")
        self.assertNotIn(
            "app.legacy", got["sources"], "an excluded file imports nothing"
        )


if __name__ == "__main__":
    unittest.main()
