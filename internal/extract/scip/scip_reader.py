# /// script
# requires-python = ">=3.10"
# dependencies = ["protobuf>=4", "grpcio-tools>=1.60"]
# ///
"""scip_reader.py — read a SCIP index and emit per-edge integration strength as JSON.

SCIP (https://scip-code.org/) is a language-agnostic code-intelligence format, so
one reader serves scip-python, scip-go, scip-typescript, and rust-analyzer. It maps
each cross-module symbol reference to a Balanced-Coupling integration strength and
emits edges whose from/to paths match archfit's graph node paths for that language:
  - python : module -> module   (dotted, e.g. "ccgram.handlers")
  - go     : file   -> package  (e.g. "internal/x/y.go" -> "internal/z", gomod-stripped)
  - ts     : file   -> file     (e.g. "src/a.ts" -> "src/b.ts")
  - rust   : file   -> file     (relative path as-is, like ts)

Strength (BC, strongest->weakest): intrusive > functional > model > contract.
  private (underscore) -> intrusive; Protocol/ABC/interface -> contract;
  method/function -> functional; concrete type -> model; else -> functional.
Edge strength = strongest among its cross-module references.

Usage: uv run scip_reader.py --proto scip.proto --index index.scip
                             --package <root> --lang <python|go|typescript|rust>
"""
from __future__ import annotations

import argparse
import importlib.util
import json
import os
import re
import sys
import tempfile

RANK = {"contract": 1, "model": 2, "functional": 3, "intrusive": 4}
ABSTRACT_BASES = ("/Protocol#", "/ABC#")  # python protocol/abc markers


def _load_scip_pb2(proto_path: str):
    out_dir = tempfile.mkdtemp(prefix="scip_pb2_")
    from grpc_tools import protoc  # type: ignore[import]

    rc = protoc.main(["protoc", f"-I{os.path.dirname(os.path.abspath(proto_path))}",
                      f"--python_out={out_dir}", os.path.basename(proto_path)])
    if rc != 0:
        raise RuntimeError(f"protoc failed: {rc}")
    mod_path = os.path.join(out_dir, os.path.basename(proto_path).replace(".proto", "_pb2.py"))
    spec = importlib.util.spec_from_file_location("scip_pb2", mod_path)
    assert spec and spec.loader
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def _fields(symbol: str):
    """Split a SCIP symbol into (scheme, manager, package, version, descriptors)."""
    parts = symbol.split(" ", 4)
    if len(parts) < 5:
        return None
    return parts[0], parts[1], parts[2], parts[3], parts[4]


def _container(descriptors: str) -> str:
    """The path/module portion of a descriptor: namespaces before the first backtick
    plus the backtick content (TS files have dir namespaces outside the backtick;
    Go/Python carry the whole container inside it)."""
    m = re.search(r"`([^`]+)`", descriptors)
    if m:
        return descriptors[: m.start()] + m.group(1)
    # No backtick: take leading namespace segments (e.g. "myapp/__init__:").
    segs = []
    for tok in descriptors.split("/"):
        if tok == "" or tok.endswith(("#", "().", ":", ".")):
            break
        segs.append(tok)
    return "/".join(segs)


def _to_path(symbol: str, lang: str) -> str | None:
    """The target module/package/file path for an edge, matching archfit node paths."""
    f = _fields(symbol)
    if f is None:
        return None
    pkg, descriptors = f[2], f[4]
    c = _container(descriptors)
    if not c:
        return None
    if lang == "python":
        return c[4:] if c.startswith("src.") else c        # dotted module, strip src.
    if lang == "go":
        prefix = pkg + "/"                                  # strip gomod module prefix
        return c[len(prefix):] if c.startswith(prefix) else (None if c == pkg else c)
    return c                                                # ts: file path as-is


def _is_internal(symbol: str, root: str) -> bool:
    """A symbol belongs to the analysed project (not stdlib / a dependency)."""
    f = _fields(symbol)
    return f is not None and f[2] == root


def _is_private(symbol: str) -> bool:
    f = _fields(symbol)
    if f is None:
        return False
    for seg in re.split(r"[./]", _container(f[4])):
        if seg.startswith("_") and not (seg.startswith("__") and seg.endswith("__")):
            return True
    leaf = f[4].rsplit("/", 1)[-1].rstrip("#:.").rstrip("()")
    return leaf.startswith("_") and not (leaf.startswith("__") and leaf.endswith("__"))


def _suffix(symbol: str) -> str:
    s = symbol.rstrip()
    if s.endswith("()."):
        return "method"
    if s.endswith("#"):
        return "type"
    return "term"


def _doc_from(relative_path: str, lang: str) -> str | None:
    if lang == "python":
        p = relative_path[:-3] if relative_path.endswith(".py") else relative_path
        p = p.replace("/", ".").replace("\\", ".")
        if p.endswith(".__init__"):
            p = p[: -len(".__init__")]
        if p.startswith("src."):  # src-layout: match grimp's package-relative module
            p = p[4:]
        return None if p in ("", "__init__") else p
    return relative_path or None  # go/ts: file path


_DEFINITION_ROLE = 0x1  # SymbolRole.Definition bit


def _compute_symbols(
    idx: object,
    root: str,
    lang: str,
) -> tuple[list[dict], list[dict], list[dict]]:
    """Return (symbols, symbol_refs, intra_refs) for the given index.

    symbols     — one entry per internal definition symbol:
                  {symbol, path, module, fan_in}
                  fan_in = count of distinct documents that reference the symbol
                  (excluding the defining document itself).

    symbol_refs — cross-module symbol→symbol reference edges:
                  {from_symbol, to_symbol}
                  from_symbol is an internal definition symbol in the referencing
                  document; to_symbol is the referenced internal symbol.

    intra_refs  — same-module symbol→symbol reference edges (from_symbol and
                  to_symbol share a module), self-edges excluded. These feed the
                  report-only cohesion (LCOM edge-density) proxy.

    Attribution is document-scoped because SCIP indexers do not populate
    enclosing_range, so per-call-site attribution is unavailable: within one
    document every definition is treated as the source of every reference in
    that document. Same-document intra-module edges are therefore over-connected;
    only cross-document intra-module structure (multi-document modules) carries a
    trustworthy cohesion signal — see internal/metrics/modularity/cohesion.go.

    All arrays are sorted for determinism (byte-identical output across runs).
    """
    # Pass 1: collect definition occurrences to build
    #   def_docs[symbol]       = relative_path of the defining document (first seen)
    #   doc_defs[relative_path] = set of internal definition symbols in that doc
    def_docs: dict[str, str] = {}
    doc_defs: dict[str, set[str]] = {}
    for doc in idx.documents:  # type: ignore[union-attr]
        defs: set[str] = set()
        for occ in doc.occurrences:
            if not (occ.symbol_roles & _DEFINITION_ROLE):
                continue
            if not _is_internal(occ.symbol, root):
                continue
            defs.add(occ.symbol)
            if occ.symbol not in def_docs:
                def_docs[occ.symbol] = doc.relative_path
        if defs:
            doc_defs[doc.relative_path] = defs

    # Pass 2: collect reference occurrences for fan-in counts and symbol refs.
    # fan_in_docs[symbol] = set of distinct documents that reference (not define) it.
    fan_in_docs: dict[str, set[str]] = {}
    # sym_refs: cross-module definition→referenced-symbol edges (deduped).
    sym_refs: set[tuple[str, str]] = set()
    # intra_refs: same-module definition→referenced-symbol edges (deduped, no self-edge).
    intra_refs: set[tuple[str, str]] = set()

    for doc in idx.documents:  # type: ignore[union-attr]
        frm_defs = doc_defs.get(doc.relative_path, set())
        for occ in doc.occurrences:
            if occ.symbol_roles & _DEFINITION_ROLE:
                continue  # skip definitions in this pass
            if not _is_internal(occ.symbol, root):
                continue
            to_mod = _to_path(occ.symbol, lang)
            if to_mod is None:
                continue

            fan_in_docs.setdefault(occ.symbol, set()).add(doc.relative_path)

            # Per-document edges: from every internal definition in this doc to
            # the referenced symbol. Split by module: differing modules → cross
            # (sym_refs), same module → intra (intra_refs, self-edge excluded).
            for from_sym in frm_defs:
                from_mod = _to_path(from_sym, lang)
                if from_mod is None:
                    continue
                if from_mod != to_mod:
                    sym_refs.add((from_sym, occ.symbol))
                elif from_sym != occ.symbol:
                    intra_refs.add((from_sym, occ.symbol))

    # Assemble output arrays (sorted for determinism).
    symbols_out = [
        {
            "symbol": sym,
            "path": doc_path,
            "module": _to_path(sym, lang),
            "fan_in": len(fan_in_docs.get(sym, set())),
        }
        for sym, doc_path in sorted(def_docs.items())
        if _to_path(sym, lang) is not None
    ]

    symbol_refs_out = [
        {"from_symbol": fs, "to_symbol": ts}
        for fs, ts in sorted(sym_refs)
    ]

    intra_refs_out = [
        {"from_symbol": fs, "to_symbol": ts}
        for fs, ts in sorted(intra_refs)
    ]

    return symbols_out, symbol_refs_out, intra_refs_out


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--proto", required=True)
    ap.add_argument("--index", required=True)
    ap.add_argument("--package", required=True)
    ap.add_argument("--lang", required=True, choices=["python", "go", "typescript", "rust"])
    args = ap.parse_args()
    lang, root = args.lang, args.package

    try:
        scip_pb2 = _load_scip_pb2(args.proto)
    except Exception as exc:  # noqa: BLE001
        print(json.dumps({"error": f"scip bindings: {exc}", "edges": []}))
        sys.exit(1)

    idx = scip_pb2.Index()
    with open(args.index, "rb") as f:
        idx.ParseFromString(f.read())

    # Contract symbols: python Protocol/ABC subclasses (impl source) + internal
    # interfaces (impl targets, for Go/TS).
    contract: set[str] = set()
    for doc in idx.documents:
        for s in doc.symbols:
            for r in s.relationships:
                if not r.is_implementation:
                    continue
                if any(r.symbol.rstrip().endswith(b) for b in ABSTRACT_BASES):
                    contract.add(s.symbol)
                if _is_internal(r.symbol, root):
                    contract.add(r.symbol)

    def classify(symbol: str) -> str:
        if _is_private(symbol):
            return "intrusive"
        if symbol in contract:
            return "contract"
        return "functional" if _suffix(symbol) != "type" else "model"

    edges: dict[tuple[str, str], str] = {}
    refs = {k: 0 for k in RANK}

    symbols_out, symbol_refs_out, intra_refs_out = _compute_symbols(idx, root, lang)

    for doc in idx.documents:
        a = _doc_from(doc.relative_path, lang)
        if a is None:
            continue
        for occ in doc.occurrences:
            if not _is_internal(occ.symbol, root):
                continue
            b = _to_path(occ.symbol, lang)
            if b is None or b == a:
                continue
            st = classify(occ.symbol)
            refs[st] += 1
            key = (a, b)
            if key not in edges or RANK[st] > RANK[edges[key]]:
                edges[key] = st

    print(json.dumps({
        "edges": [{"from": a, "to": b, "strength": st} for (a, b), st in sorted(edges.items())],
        "contract_symbols": len(contract),
        "ref_strength_dist": refs,
        "symbols": symbols_out,
        "symbol_refs": symbol_refs_out,
        "intra_refs": intra_refs_out,
    }))


if __name__ == "__main__":
    main()
