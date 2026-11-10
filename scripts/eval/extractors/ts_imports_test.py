import json
import shutil
import tempfile
import unittest
from pathlib import Path

from . import ts_imports

TREE = {
    "package.json": json.dumps({"workspaces": {"packages": ["packages/*"]}}),
    "tsconfig.json": (
        "{\n  // JSONC: comments and trailing commas\n"
        '  "compilerOptions": {"baseUrl": ".", "paths": {"@app/*": ["src/*"], "@lib": ["src/lib/index.ts"],},},\n}\n'
    ),
    "src/a.ts": "",
    "src/b.tsx": "",
    "src/c/index.ts": "",
    "src/esm.ts": "",
    "src/legacy.js": "",
    "src/types.d.ts": "",
    "src/style.css": "",
    "src/lib/index.ts": "",
    "packages/core/package.json": json.dumps(
        {
            "name": "@acme/core",
            "exports": {
                ".": {
                    "types": "./dist/index.d.ts",
                    "code": "./src/index.ts",
                    "default": "./dist/index.js",
                },
                "./plain": "./src/plain.ts",
                "./internal/*": {"code": "./src/internal/*.ts"},
                "./private": None,
                "./blocked": {"code": None, "default": "./src/plain.ts"},
                "./*": {"code": "./src/*.ts"},
            },
            "imports": {
                "#store": {
                    "storybook": "./src/store.mock.ts",
                    "default": "./src/store.ts",
                }
            },
        }
    ),
    "packages/core/tsconfig.json": '{"extends": "../../tsconfig.json", "compilerOptions": {"customConditions": ["code"]}}',
    "packages/core/src/index.ts": "",
    "packages/core/src/plain.ts": "",
    "packages/core/src/store.ts": "",
    "packages/core/src/internal/x.ts": "",
    "packages/old/package.json": json.dumps({"name": "old", "main": "./lib/main.js"}),
    "packages/old/lib/main.js": "",
    "packages/old/lib/extra.js": "",
    "packages/node10/tsconfig.json": '{"compilerOptions": {"moduleResolution": "node"}}',
    "packages/node10/package.json": json.dumps({"name": "node10"}),
    "packages/node10/use.js": "",
    "packages/core/preview.js": "",
    "packages/core/src/private.ts": "",
    "packages/loop/package.json": json.dumps({"name": "loop", "main": "."}),
    "packages/loop/index.js": "",
    "apps/web/tsconfig.json": '{"compilerOptions": {"baseUrl": "src"}}',
    "apps/web/src/components/Button.tsx": "",
    "nx/tsconfig.base.json": '{"compilerOptions": {"baseUrl": "."}}',
    "nx/apps/web/tsconfig.json": '{"extends": ["../../tsconfig.base.json", "./tsconfig.paths.json", "./tsconfig.strict.json"]}',
    "nx/apps/web/tsconfig.paths.json": '{"compilerOptions": {"paths": {"@web/*": ["apps/web/src/*"]}}}',
    "nx/apps/web/tsconfig.strict.json": '{"compilerOptions": {"strict": true}}',
    "nx/apps/web/src/x.ts": "",
    "star/tsconfig.json": '{"compilerOptions": {"paths": {"*": ["types/*"]}}}',
}


def write_tree(root: Path, files: dict[str, str]) -> None:
    for name, body in files.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)


class ResolveTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)
        write_tree(self.root, TREE)
        self.resolver = ts_imports.Resolver(self.root)

    def test_rules(self):
        core = "packages/core/src/index.ts"
        cases = [
            ("relative, extension probed", "src/x.ts", "./a", ("file", "src/a.ts")),
            ("relative tsx", "src/x.ts", "./b", ("file", "src/b.tsx")),
            ("directory index", "src/x.ts", "./c", ("file", "src/c/index.ts")),
            (
                ".js specifier names a .ts file",
                "src/x.ts",
                "./esm.js",
                ("file", "src/esm.ts"),
            ),
            (
                ".js file that exists",
                "src/x.ts",
                "./legacy.js",
                ("file", "src/legacy.js"),
            ),
            (
                ".ts specifier names a declaration",
                "src/x.ts",
                "./types.ts",
                ("file", "src/types.d.ts"),
            ),
            (
                "asset with a query",
                "src/x.ts",
                "./style.css?raw",
                ("file", "src/style.css"),
            ),
            (
                "missing relative is unknown",
                "src/x.ts",
                "./gone",
                ("unknown", "./gone"),
            ),
            (
                "relative above the root is unknown",
                "src/x.ts",
                "../../x",
                ("unknown", "../../x"),
            ),
            ("paths wildcard", "src/x.ts", "@app/a", ("file", "src/a.ts")),
            ("paths match without a file is unknown", "src/x.ts", "@app/missing", ("unknown", "@app/missing")),
            ("catch-all paths does not make a package first-party", "star/a.ts", "react", ("external", "react")),
            ("baseUrl alone resolves a bare specifier", "apps/web/src/app.tsx", "components/Button", ("file", "apps/web/src/components/Button.tsx")),
            ("array extends merges; paths use the inherited baseUrl", "nx/apps/web/src/main.ts", "@web/x", ("file", "nx/apps/web/src/x.ts")),
            ("exports null blocks a subpath a pattern would match", core, "@acme/core/private", ("unknown", "@acme/core/private")),
            ("exports null under an active condition blocks", core, "@acme/core/blocked", ("unknown", "@acme/core/blocked")),
            ("main naming its own dir falls back to index", "src/x.ts", "loop", ("file", "packages/loop/index.js")),
            ("paths exact", "src/x.ts", "@lib", ("file", "src/lib/index.ts")),
            ("exports with the tsconfig condition", core, "@acme/core", ("file", core)),
            (
                "exports without the condition reaches only dist",
                "src/x.ts",
                "@acme/core",
                ("unknown", "@acme/core"),
            ),
            (
                "exports string subpath",
                "src/x.ts",
                "@acme/core/plain",
                ("file", "packages/core/src/plain.ts"),
            ),
            (
                "exports pattern subpath",
                core,
                "@acme/core/internal/x",
                ("file", "packages/core/src/internal/x.ts"),
            ),
            (
                "subpath that exports does not list",
                "src/x.ts",
                "@acme/core/preview",
                ("unknown", "@acme/core/preview"),
            ),
            (
                "package imports field",
                core,
                "#store",
                ("file", "packages/core/src/store.ts"),
            ),
            ("main field", "src/x.ts", "old", ("file", "packages/old/lib/main.js")),
            (
                "subpath without exports",
                "src/x.ts",
                "old/lib/extra",
                ("file", "packages/old/lib/extra.js"),
            ),
            (
                "node10 ignores exports",
                "packages/node10/use.js",
                "@acme/core/preview",
                ("file", "packages/core/preview.js"),
            ),
            ("third-party package", "src/x.ts", "react", ("external", "react")),
            ("node builtin", "src/x.ts", "node:fs", ("external", "node:fs")),
        ]
        for name, importer, spec, want in cases:
            with self.subTest(name):
                self.assertEqual(self.resolver.resolve(importer, spec), want)


class WorkspaceTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)

    def test_pnpm_workspace_with_an_exclusion(self):
        write_tree(
            self.root,
            {
                "package.json": "{}",
                "pnpm-workspace.yaml": "# workspace\npackages:\n  - 'libs/*'\n  - \"!libs/skip\"\ncatalog:\n  - x\n",
                "libs/a/package.json": json.dumps({"name": "@org/a"}),
                "libs/skip/package.json": json.dumps({"name": "@org/skip"}),
            },
        )
        self.assertEqual(ts_imports.workspace_packages(self.root), {"@org/a": "libs/a"})


@unittest.skipUnless(shutil.which("ast-grep"), "ast-grep is not installed")
class ParseTest(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.root)

    def test_import_forms(self):
        source = (
            '/// <reference path="../types.d.ts" />\n'
            "import a from './a';\n"
            "import type { T } from './t';\n"
            "export * from './re';\n"
            "export type { Y } from './rey';\n"
            "import eq = require('./eq');\n"
            "const d = import('./dyn');\n"
            "const r = require('./req');\n"
            "// import './commented';\n"
            "const s = \"import './in-string'\";\n"
        )
        write_tree(
            self.root,
            {
                "src/x.ts": source,
                "src/y.tsx": source,
                "src/z.js": "import a from './a';\nrequire('./req');\n",
            },
        )
        got = ts_imports.parse(self.root, ["src/x.ts", "src/y.tsx", "src/z.js"])
        want = [
            "../types.d.ts",
            "./a",
            "./t",
            "./re",
            "./rey",
            "./eq",
            "./dyn",
            "./req",
        ]
        self.assertEqual(sorted(got["src/x.ts"]), sorted(want))
        self.assertEqual(sorted(got["src/y.tsx"]), sorted(want))
        self.assertEqual(sorted(got["src/z.js"]), ["./a", "./req"])

    def test_extract_skips_node_modules(self):
        write_tree(
            self.root,
            {
                "package.json": "{}",
                "src/x.ts": "import a from './a';\nimport b from './gone';\nimport React from 'react';\n",
                "src/a.ts": "",
                "node_modules/dep/index.js": "import x from './x';\n",
            },
        )
        got = ts_imports.extract(self.root, {})
        self.assertEqual(got["sources"], ["src/a.ts", "src/x.ts"])
        self.assertEqual(got["edges"], [{"from": "src/x.ts", "to": "src/a.ts"}])
        self.assertEqual(got["unknown"], [{"from": "src/x.ts", "specifier": "./gone"}])
        self.assertEqual(got["external"], 1)


if __name__ == "__main__":
    unittest.main()
