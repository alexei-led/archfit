#!/usr/bin/env python3
"""Table tests for scip_reader's pure classification logic (run: python3 scip_reader_test.py).

Covers the per-language reconciliation that silently rots: container/path extraction
and private/interface detection for scip-python, scip-go, scip-typescript symbols.
Does not need protobuf (only the pure helpers are exercised).
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import scip_reader as r  # noqa: E402

# Representative real symbol strings (one per language family).
PY_MOD = "scip-python python ccgram 0.1.0 `src.ccgram.llm.base`/TextCompleter#"
PY_PRIV = "scip-python python ccgram 0.1.0 `src.ccgram.main`/_restart_requested."
PY_FUNC = "scip-python python ccgram 0.1.0 `src.ccgram.handlers.shell.shell_capture`/register_approval_callback()."
GO_TYPE = "scip-go gomod spotinfo v2.3.1 `spotinfo/internal/spot`/Client#"
GO_FUNC = "scip-go gomod spotinfo v2.3.1 `spotinfo/internal/mcp`/handle()."
TS_TYPE = "scip-typescript npm @colbymchenry/codegraph 0.9.9 src/db/`sqlite-adapter.ts`/SqliteStatement#"
EXT_PY = "scip-python python python-stdlib 3.12 typing/Protocol#"

failures = []


def check(name, got, want):
    if got != want:
        failures.append(f"{name}: got {got!r}, want {want!r}")


# _to_path: target path must match archfit node-path form per language.
check("py to (strip src.)", r._to_path(PY_MOD, "python"), "ccgram.llm.base")
check("go to (strip gomod)", r._to_path(GO_TYPE, "go"), "internal/spot")
check("ts to (ns+backtick file)", r._to_path(TS_TYPE, "typescript"), "src/db/sqlite-adapter.ts")

# _doc_from: source path per language.
check("py doc (dotted, strip src)", r._doc_from("src/ccgram/handlers/x.py", "python"), "ccgram.handlers.x")
check("py doc __init__", r._doc_from("src/ccgram/handlers/__init__.py", "python"), "ccgram.handlers")
check("go doc (file)", r._doc_from("internal/spot/client.go", "go"), "internal/spot/client.go")
check("ts doc (file)", r._doc_from("src/db/sqlite-adapter.ts", "typescript"), "src/db/sqlite-adapter.ts")

# _is_private: underscore symbol/module → private; public → not.
check("py private symbol", r._is_private(PY_PRIV), True)
check("py public func", r._is_private(PY_FUNC), False)
check("py public type", r._is_private(PY_MOD), False)

# _is_internal: only the analysed project's symbols count.
check("py internal", r._is_internal(PY_MOD, "ccgram"), True)
check("py external (stdlib)", r._is_internal(EXT_PY, "ccgram"), False)
check("go internal", r._is_internal(GO_TYPE, "spotinfo"), True)
check("ts internal", r._is_internal(TS_TYPE, "@colbymchenry/codegraph"), True)

# _suffix: descriptor kind drives strength.
check("suffix type", r._suffix(GO_TYPE), "type")
check("suffix method", r._suffix(GO_FUNC), "method")

if failures:
    print("FAIL:")
    for f in failures:
        print("  -", f)
    sys.exit(1)
print("ok: all scip_reader table tests passed")
