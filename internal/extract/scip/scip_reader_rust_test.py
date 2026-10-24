"""Rust target-identity regressions using data-only Cargo metadata and SCIP indexes."""

import json
import os
import subprocess
import tempfile
import unittest
from types import SimpleNamespace

import scip_reader as r

ROOT = "/analysis/workspace"


def symbol(package, descriptor, version="0.1.0"):
    return f"rust-analyzer cargo {package} {version} {descriptor}"


def package(name, targets, directory="", version="0.1.0"):
    return {
        "id": f"path+file://{ROOT}/{directory}#{name}@{version}",
        "name": name,
        "version": version,
        "targets": [
            {"name": target, "kind": [kind], "src_path": f"{ROOT}/{directory}{source}"}
            for target, kind, source in targets
        ],
    }


def metadata(*packages):
    return {
        "packages": list(packages),
        "workspace_members": [p["id"] for p in packages],
    }


def index(*documents):
    return SimpleNamespace(
        metadata=SimpleNamespace(project_root=f"file://{ROOT}"),
        documents=[
            SimpleNamespace(
                relative_path=path,
                occurrences=[
                    SimpleNamespace(symbol=sym, symbol_roles=roles)
                    for sym, roles in occurrences
                ],
            )
            for path, occurrences in documents
        ],
    )


class RustTargetIdentityTests(unittest.TestCase):
    def test_hyphen_and_custom_single_targets_join_cargo_modules_keys(self):
        for name, target, kind in [
            ("lab-demo", "lab_demo", "lib"),
            ("custom-pkg", "actual_lib", "lib"),
            ("bin-pkg", "actual-bin", "bin"),
        ]:
            with self.subTest(package=name, target=target):
                source = symbol(name, "core/run().")
                dest = symbol(name, "adapter/Value#")
                idx = index(
                    ("src/core.rs", [(source, 1), (dest, 0)]),
                    ("src/adapter.rs", [(dest, 1)]),
                )
                targets = r._RustTargetMap(
                    idx, metadata(package(name, [(target, kind, "src/lib.rs")])), name
                )
                syms, refs, intra = r._compute_symbols(idx, name, "rust", targets)
                self.assertEqual(
                    {s["module"] for s in syms},
                    {
                        target.replace("-", "_") + "::core",
                        target.replace("-", "_") + "::adapter",
                    },
                )
                self.assertEqual(refs, [{"from_symbol": source, "to_symbol": dest}])
                self.assertEqual(intra, [])
                self.assertEqual(targets.diagnostics()["status"], "ok")
                self.assertEqual(
                    r._doc_from("src/core.rs", "rust", {source}, targets),
                    target.replace("-", "_") + "::core",
                )

    def test_workspace_versions_and_real_lib_namespace(self):
        a = symbol("first-pkg", "lib/api/Thing#")
        b = symbol("second-pkg", "consumer/run().", "2.0.0")
        external = symbol("first-pkg", "api/Other#", "9.0.0")
        idx = index(
            ("one/src/lib/api.rs", [(a, 1)]),
            ("two/src/consumer.rs", [(b, 1), (a, 0), (external, 0)]),
        )
        meta = metadata(
            package("first-pkg", [("custom_api", "lib", "src/lib.rs")], "one/"),
            package(
                "second-pkg", [("consumer-bin", "bin", "src/main.rs")], "two/", "2.0.0"
            ),
        )
        targets = r._RustTargetMap(idx, meta, {"first-pkg", "second-pkg"})
        self.assertEqual(r._to_path(a, "rust", targets), "custom_api::lib::api")
        self.assertEqual(r._to_path(b, "rust", targets), "consumer_bin::consumer")
        self.assertIsNone(r._to_path(external, "rust", targets))
        self.assertEqual(targets.diagnostics()["unresolved_symbols"], 1)

    def test_multi_target_abstains_non_roots_and_duplicate_root_symbols(self):
        root = symbol("multi-pkg", "crate/")  # RA1.99.1 duplicates this across targets.
        main = symbol("multi-pkg", "main().")
        lib_fn = symbol("multi-pkg", "entry().")
        ambiguous = symbol("multi-pkg", "api/Value#")
        idx = index(
            ("src/lib.rs", [(root, 1), (lib_fn, 1), (ambiguous, 0)]),
            ("src/main.rs", [(root, 1), (main, 1), (ambiguous, 0)]),
            ("src/api.rs", [(ambiguous, 1)]),
        )
        targets = r._RustTargetMap(
            idx,
            metadata(
                package(
                    "multi-pkg",
                    [
                        ("custom_lib", "lib", "src/lib.rs"),
                        ("custom-bin", "bin", "src/main.rs"),
                    ],
                )
            ),
            "multi-pkg",
        )
        self.assertEqual(r._to_path(main, "rust", targets), "custom_bin")
        self.assertEqual(r._to_path(lib_fn, "rust", targets), "custom_lib")
        self.assertIsNone(r._to_path(root, "rust", targets))
        self.assertIsNone(r._to_path(ambiguous, "rust", targets))
        self.assertEqual(
            r._doc_from("src/lib.rs", "rust", {root, lib_fn}, targets), "custom_lib"
        )
        self.assertIsNone(r._doc_from("src/api.rs", "rust", {ambiguous}, targets))
        self.assertIsNone(r._doc_from("src/lib.rs", "rust", None, targets))
        syms, refs, intra = r._compute_symbols(idx, "multi-pkg", "rust", targets)
        self.assertEqual({s["symbol"] for s in syms}, {main, lib_fn})
        self.assertEqual(refs, [])
        self.assertEqual(intra, [])
        self.assertEqual(
            targets.diagnostics(),
            {
                "status": "partial",
                "unresolved_symbols": 2,
                "diagnostics": [{"reason": "ambiguous_target", "count": 2}],
            },
        )

    def test_member_filter_and_missing_target_do_not_guess(self):
        sym = symbol("lab-demo", "api/Thing#")
        idx = index(("src/api.rs", [(sym, 1)]))
        meta = metadata(package("lab-demo", [("lab_demo", "lib", "src/lib.rs")]))
        meta["workspace_members"] = ["some-other-package"]
        targets = r._RustTargetMap(idx, meta, "lab-demo")
        self.assertIsNone(r._to_path(sym, "rust", targets))
        self.assertEqual(
            targets.diagnostics()["diagnostics"],
            [{"reason": "missing_target", "count": 1}],
        )
        # The legacy direct-reader contract remains unchanged without metadata.
        self.assertEqual(r._to_path(sym, "rust"), "lab-demo::api")

    def test_metadata_paths_match_symlinked_scip_project_root(self):
        with tempfile.TemporaryDirectory(prefix="scip-rust-alias-") as tmp:
            actual = os.path.join(tmp, "actual")
            os.mkdir(actual)
            alias = os.path.join(tmp, "alias")
            os.symlink(actual, alias)
            sym = symbol("multi-pkg", "main().")
            idx = index(("src/main.rs", [(sym, 1)]))
            idx.metadata.project_root = "file://" + alias
            meta = metadata(package("multi-pkg", [
                ("custom_lib", "lib", "src/lib.rs"),
                ("custom-bin", "bin", "src/main.rs"),
            ]))
            for target in meta["packages"][0]["targets"]:
                target["src_path"] = target["src_path"].replace(ROOT, actual)
            targets = r._RustTargetMap(idx, meta, "multi-pkg")
            self.assertEqual(r._to_path(sym, "rust", targets), "custom_bin")
            self.assertEqual(targets.diagnostics()["status"], "ok")

    def test_multiple_binaries_and_shared_target_roots_remain_ambiguous(self):
        sym = symbol("multi-pkg", "api/Value#")
        idx = index(("src/api.rs", [(sym, 1)]))
        meta = metadata(package("multi-pkg", [
            ("one-bin", "bin", "src/one.rs"),
            ("two-bin", "bin", "src/two.rs"),
        ]))
        targets = r._RustTargetMap(idx, meta, "multi-pkg")
        self.assertIsNone(r._to_path(sym, "rust", targets))
        self.assertEqual(targets.diagnostics()["status"], "partial")
        # Even an exact target-root path is insufficient if two targets share it.
        meta["packages"][0]["targets"][0]["src_path"] = ROOT + "/src/api.rs"
        meta["packages"][0]["targets"][1]["src_path"] = ROOT + "/src/api.rs"
        targets = r._RustTargetMap(idx, meta, "multi-pkg")
        self.assertIsNone(r._doc_from("src/api.rs", "rust", {sym}, targets))

    def test_cli_real_protobuf_uses_metadata_and_fails_closed_when_missing(self):
        # Run through the same real protobuf and reader CLI seam as the existing suite.
        here = os.path.dirname(os.path.abspath(__file__))
        with tempfile.TemporaryDirectory(prefix="scip-rust-metadata-") as tmp:
            meta = os.path.join(tmp, "metadata.json")
            idx_path = os.path.join(tmp, "index.scip")
            with open(meta, "w", encoding="utf-8") as f:
                json.dump(
                    metadata(
                        package("lab-demo", [("custom_lib", "lib", "src/lib.rs")])
                    ),
                    f,
                )
            helper = """
import sys
sys.path.insert(0, sys.argv[1])
import scip_reader as r
pb = r._load_scip_pb2(sys.argv[2])
idx = pb.Index()
idx.metadata.project_root = 'file:///analysis/workspace'
for path, own, ref in [('src/core.rs', 'core/run().', 'adapter/Value#'), ('src/adapter.rs', 'adapter/Value#', '')]:
    doc = idx.documents.add()
    doc.relative_path = path
    occ = doc.occurrences.add()
    occ.symbol = 'rust-analyzer cargo lab-demo 0.1.0 ' + own
    occ.symbol_roles = 1
    if ref:
        doc.occurrences.add().symbol = 'rust-analyzer cargo lab-demo 0.1.0 ' + ref
with open(sys.argv[3], 'wb') as f:
    f.write(idx.SerializeToString())
"""
            proto = os.path.join(here, "scip.proto")
            gen = subprocess.run(
                [
                    "uv",
                    "run",
                    "--with",
                    "grpcio-tools>=1.60",
                    "--with",
                    "protobuf>=4",
                    "python",
                    "-c",
                    helper,
                    here,
                    proto,
                    idx_path,
                ],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(gen.returncode, 0, gen.stderr)
            args = [
                "uv",
                "run",
                os.path.join(here, "scip_reader.py"),
                "--proto",
                proto,
                "--index",
                idx_path,
                "--package",
                "lab-demo",
                "--lang",
                "rust",
                "--rust-metadata",
                meta,
            ]
            run = subprocess.run(args, capture_output=True, text=True, check=False)
            self.assertEqual(run.returncode, 0, run.stderr)
            result = json.loads(run.stdout)
            self.assertEqual(
                result["edges"],
                [
                    {
                        "from": "custom_lib::core",
                        "to": "custom_lib::adapter",
                        "strength": "model",
                        "data_strength": "model",
                        "connascence": ["type"],
                    }
                ],
            )
            self.assertEqual(result["rust_attribution"]["status"], "ok")
            with open(meta, "w", encoding="utf-8") as f:
                json.dump(metadata(package("lab-demo", [
                    ("custom_lib", "lib", "src/lib.rs"),
                    ("custom-bin", "bin", "src/main.rs"),
                ])), f)
            ambiguous = subprocess.run(args, capture_output=True, text=True, check=False)
            self.assertEqual(ambiguous.returncode, 0, ambiguous.stderr)
            result = json.loads(ambiguous.stdout)
            self.assertEqual(result["edges"], [])
            self.assertEqual(result["symbols"], [])
            self.assertEqual(result["symbol_refs"], [])
            self.assertEqual(result["rust_attribution"], {
                "status": "partial", "unresolved_symbols": 2,
                "diagnostics": [{"reason": "ambiguous_target", "count": 2}],
            })
            os.unlink(meta)
            bad = subprocess.run(args, capture_output=True, text=True, check=False)
            self.assertNotEqual(bad.returncode, 0)
            self.assertEqual(json.loads(bad.stdout)["edges"], [])


if __name__ == "__main__":
    unittest.main()
