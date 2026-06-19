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

# ---------------------------------------------------------------------------
# _compute_symbols tests — fake duck-typed index, no protobuf needed.
# ---------------------------------------------------------------------------

from types import SimpleNamespace  # noqa: E402


def _occ(symbol: str, roles: int) -> SimpleNamespace:
    return SimpleNamespace(symbol=symbol, symbol_roles=roles)


def _doc(relative_path: str, occurrences: list) -> SimpleNamespace:
    return SimpleNamespace(relative_path=relative_path, occurrences=occurrences)


def _idx(documents: list) -> SimpleNamespace:
    return SimpleNamespace(documents=documents)


DEF = 0x1  # SymbolRole.Definition

# Symbols used in fixture (Go, root = "spotinfo")
SYM_CLIENT = "scip-go gomod spotinfo v2.3.1 `spotinfo/internal/spot`/Client#"
SYM_HANDLE = "scip-go gomod spotinfo v2.3.1 `spotinfo/internal/mcp`/handle()."
SYM_EXT    = "scip-go gomod some-dep v1.0.0 `some-dep/pkg`/Thing#"    # external

# --- Case 1: populated fixture with cross-module reference ---
# doc A defines Client; doc B defines handle and references Client.
fixture_idx = _idx([
    _doc("internal/spot/client.go", [
        _occ(SYM_CLIENT, DEF),              # definition of Client
    ]),
    _doc("internal/mcp/server.go", [
        _occ(SYM_HANDLE, DEF),              # definition of handle
        _occ(SYM_CLIENT, 0),                # reference to Client (cross-module)
        _occ(SYM_EXT,    0),                # external ref — must be ignored
    ]),
])

syms, srefs, _ = r._compute_symbols(fixture_idx, "spotinfo", "go")

# symbols: both internal definitions present with correct fields.
sym_map = {s["symbol"]: s for s in syms}
check("symbols: Client present", SYM_CLIENT in sym_map, True)
check("symbols: handle present", SYM_HANDLE in sym_map, True)
check("symbols: external absent", SYM_EXT in sym_map, False)

check("symbols: Client path",   sym_map[SYM_CLIENT]["path"],   "internal/spot/client.go")
check("symbols: Client module", sym_map[SYM_CLIENT]["module"], "internal/spot")
# Client is referenced from mcp/server.go → fan_in = 1
check("symbols: Client fan_in", sym_map[SYM_CLIENT]["fan_in"], 1)
# handle is not referenced → fan_in = 0
check("symbols: handle fan_in", sym_map[SYM_HANDLE]["fan_in"], 0)

# symbol_refs: handle (mcp) → Client (spot) cross-module edge expected.
sref_pairs = {(e["from_symbol"], e["to_symbol"]) for e in srefs}
check("symbol_refs: handle→Client", (SYM_HANDLE, SYM_CLIENT) in sref_pairs, True)
check("symbol_refs: no external",   any(e["to_symbol"] == SYM_EXT for e in srefs), False)

# sorted determinism: verify both arrays are sorted.
if syms:
    check("symbols sorted", syms, sorted(syms, key=lambda s: s["symbol"]))
if srefs:
    check("symbol_refs sorted", srefs,
          sorted(srefs, key=lambda e: (e["from_symbol"], e["to_symbol"])))

# --- Case 2: empty index ---
empty_syms, empty_srefs, empty_irefs = r._compute_symbols(_idx([]), "spotinfo", "go")
check("empty index: symbols", empty_syms, [])
check("empty index: symbol_refs", empty_srefs, [])
check("empty index: intra_refs", empty_irefs, [])

# --- Case 3: no internal symbols (only external references) ---
ext_only_idx = _idx([
    _doc("internal/mcp/server.go", [
        _occ(SYM_EXT, 0),   # reference to external symbol only
    ]),
])
ext_syms, ext_srefs, ext_irefs = r._compute_symbols(ext_only_idx, "spotinfo", "go")
check("no internal: symbols", ext_syms, [])
check("no internal: symbol_refs", ext_srefs, [])
check("no internal: intra_refs", ext_irefs, [])

# --- Case 4: same-module reference does not produce a symbol_ref edge ---
SYM_OTHER = "scip-go gomod spotinfo v2.3.1 `spotinfo/internal/spot`/Fetch()."
same_mod_idx = _idx([
    _doc("internal/spot/client.go", [
        _occ(SYM_CLIENT, DEF),
        _occ(SYM_OTHER,  0),   # ref within same module (spot → spot)
    ]),
])
sm_syms, sm_srefs, sm_irefs = r._compute_symbols(same_mod_idx, "spotinfo", "go")
check("same-module ref: no symbol_refs", sm_srefs, [])
check("same-module ref: Client fan_in 0", sm_syms[0]["fan_in"] if sm_syms else -1, 0)
# intra_refs: Client (def) → Fetch (ref), both in internal/spot, is an intra edge.
sm_ipairs = {(e["from_symbol"], e["to_symbol"]) for e in sm_irefs}
check("same-module ref: Client→Fetch intra", (SYM_CLIENT, SYM_OTHER) in sm_ipairs, True)

# --- Case 5: multi-doc fan_in counts distinct documents ---
fan_idx = _idx([
    _doc("internal/spot/client.go", [
        _occ(SYM_CLIENT, DEF),
    ]),
    _doc("internal/mcp/server.go", [
        _occ(SYM_HANDLE, DEF),
        _occ(SYM_CLIENT, 0),   # ref from doc B
    ]),
    _doc("internal/mcp/proxy.go", [
        _occ(SYM_CLIENT, 0),   # ref from doc C (same symbol, different doc)
    ]),
])
fan_syms, _, _ = r._compute_symbols(fan_idx, "spotinfo", "go")
fan_map = {s["symbol"]: s for s in fan_syms}
check("fan_in multi-doc", fan_map[SYM_CLIENT]["fan_in"], 2)

if failures:
    print("FAIL (compute_symbols):")
    for f in failures:
        print("  -", f)
    sys.exit(1)
print("ok: all _compute_symbols tests passed")
