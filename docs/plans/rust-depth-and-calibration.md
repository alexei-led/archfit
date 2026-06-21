# Plan: Rust analysis depth + scorecard calibration

Derived from a two-repo study (yazi: 32-crate workspace; herdr: single-crate
binary) comparing archfit deterministic output against independent architect
reviews. Evidence and rationale: see the session comparison + synthesis.

Status legend: [done] shipped · [todo] not started.

Already shipped this effort:

- [done] LOC counts `.rs` + skips `target/`; lizard analyses `rust`; `rustFileToModuleKey`
  handles root-level workspace layouts (commit bd74ef6).
- [done] P0a: scorecard caps degenerate-graph and zero-edge dimensions at mixed —
  no more false-green that rises as archfit sees less (commit b42e1d1, +ba25172 to
  keep the module graph from reintroducing the false-green).
- [done] P2: analysis_confidence degenerate cap + disabled-language tool noise (b42e1d1).
- [done] T5 docs (1501379). T3 init topo-layers + read-only fix (5f73359, b6fb70b).
- [done] T1 module graph (1fc0faa) + parseDOT uses-edge aggregation so it is a real
  dependency graph, not an ownership tree (dbe3a36).
- [done] T2 SCIP: rust-analyzer scip RUNS and maps modules correctly and
  attaches StrengthHint to module edges (commit 5cb95ef + dbe3a36 fixed two
  showstoppers: the reader required a `crate/` prefix that rust-analyzer does not emit
  for top-level modules, and `rust-analyzer scip` was invoked with no positional path
  so it wrote no index).
- [done] Last mile (fcfd812): `classify.AugmentModulesFromGraph` registers
  auto-discovered `<crate>::<mod>` nodes as modules (gated on `::` so Go/TS/Python are
  untouched); Rust NodeConvention separator `/`→`::` so siblings classify same-owner.
  Verified on herdr: coupling_balance n/a→50/mixed/high-confidence with 2 advisories;
  cohesion_lcom n/a→measured; scip seen=2375. encapsulation stays n/a by design (Rust
  privacy ⇒ no intrusive cross-module edges; it scores only contract/intrusive).

Optional polish (not blocking): per-module owner/subdomain/volatility for synthesised
module nodes (today they inherit defaults) would sharpen distance/volatility; layer
coalescing for init (T3) to fewer than the raw topo-depth count.

Guiding evidence:

- herdr (1 crate) scored 73 "serviceable" with four strong dimensions on a 1-node
  graph; the architect scored it ~4.7 with two High-severity god-file findings.
- archfit's Rust model is crate-level; a single crate is one node, so the real
  architecture (the `src/` module tree — 38 modules / 2,542 edges per cargo-modules)
  was invisible.
- rust-analyzer SCIP descriptors are crate-relative MODULE paths (`crate/mod/Type#`),
  not file paths; the reader treats Rust like TypeScript, so edges collapse to
  `file -> "crate"` and never match crate-level graph nodes.

---

## Tasks

### Task 1: Intra-crate module graph for Rust via cargo-modules (P0b)

The highest-value Rust improvement. archfit must model Rust at MODULE granularity,
not just crate granularity, so single-crate projects and large crates are no longer
opaque.

- Add `tools.rust.module_graph` (or reuse `tools.scip`) opt-in. When on, after
  `cargo metadata` builds crate nodes, run `cargo modules dependencies --lib`
  (and `--bin <name>` for binary-only crates; detect from cargo metadata targets)
  per workspace member, parse the DOT output, and emit module-level nodes
  (`<crate>::<mod>::<sub>`) + `owns`/`uses` edges into `graph.Facts`.
- Node-key convention: extend `internal/model/graph/convention.go` so Rust module
  nodes have a stable path and `FileToModuleKey` can map a `.rs` file to its module
  node (not just its crate). Keep stdlib-only (core-ring invariant).
- Gate on cost: cargo-modules requires a compile (minutes); document it as opt-in,
  reuse the existing tool-coverage/partial machinery, and set a generous timeout.
- Coverage: report a `cargo-modules` tool row (ok/partial/absent) like scip.

Verification:

- New extractor test with a fixture DOT (no live compile in unit tests; feed canned
  cargo-modules output through a faked `toolrun.Runner`).
- On herdr: module nodes > 1, cycle/blast_radius/cohesion now measured (not n/a);
  scorecard no longer hits the degenerate guard.
- `make test`, `make lint`, golden inspected.

### Task 2: Map rust-analyzer SCIP to module/crate nodes (P1a)

Fix the SCIP reader so Rust strength actually attaches. Prefer module granularity
(ties to Task 1); fall back to crate granularity when the module graph is off.

- `internal/extract/scip/scip_reader.py`: for `lang == "rust"`, derive `_to_path`
  and `_doc_from` from the symbol's crate field (`f[2]`) + the namespace descriptors
  (`crate/mod/sub`), NOT `_container` as a file path. Emit module-node keys that
  match Task 1's convention (or crate keys when module graph is off).
- `_is_internal`: accept a SET of internal package names (workspace members), not a
  single root. `--package` becomes comma-separated.
- `internal/extract/scip/scip_strength.go`: `detectIndexer` must handle a virtual
  workspace (`[workspace]` with no `[package]`) — enumerate member crate names via
  `cargo metadata --no-deps` and pass them all. Today `cargoPackageName` returns ""
  and SCIP never runs.
- Persist/cache the `.scip` index (rust-analyzer scip is ~55s/crate) keyed by repo
  HEAD, like gitnexus/codegraph; reuse if unchanged. Generous timeout, opt-in.

Verification:

- Reader unit test against a checked-in rust-analyzer `.scip` fixture: edges resolve
  to module/crate node keys, not `"crate"`/`""`.
- End-to-end on a 2-crate fixture workspace: `tools.scip` engages, strength attaches
  to graph edges (StrengthHint set), coupling_balance leaves `n/a`.

### Task 3: `init` scaffolds layers + volatility (P1b)

Flat auto-config (all modules in one `core` layer) produced 0 gate findings on both
repos; archfit could have gated yazi's real `config -> widgets` inversion with
authored layers.

- `archfit init`: infer layer tiers from the dependency DAG (topological levels) and
  seed `rules: forbidden_dependency` for back-edges; seed `volatility` from gitnexus
  churn when an index is present.
- When layers/owners cannot be inferred, emit a prominent comment that only metrics
  (no gates) will be produced until they are filled in.

Verification: `init` on yazi yields >1 layer and a forbidden_dependency rule that
flags `yazi-config -> yazi-widgets`; unit test the layering inference on a fixture graph.

### Task 4: P2 calibration + signal additions

- `analysis_confidence` must reflect the n/a-dimension ratio, not just file-extraction
  coverage (both repos reported 95 while half the dimensions were n/a).
  File: `internal/score/score.go` `analysisConfidence`.
- Suppress "required tools missing" warnings for DISABLED languages (both repos
  warned about dependency-cruiser/grimp/go-packages on Rust-only repos).
- Add a testability signal (test-file / test-dir ratio) as a dimension or sub-metric;
  the architect scored testability on both repos, archfit has no axis.
- Add `cargo-machete` (fast, no compile) as a Rust dependency-health signal (unused
  deps). File: new `internal/extract/rust` sub-step or a `deps` extractor.

Verification: per-item unit tests; re-run scorecards on yazi/herdr and confirm
analysis_confidence drops when dimensions are n/a.

### Task 5: Skills + docs

- Architect plugin: add a `tools-rust` skill (mirror `tools-go`/`tools-typescript`):
  cargo metadata, cargo-modules (intra-crate graph), clippy, cargo-machete,
  rust-analyzer/scip, with the compile-cost caveats. NOTE: architect skills live in
  the plugin source repo (not archfit); confirm path before editing — do not edit the
  plugin cache.
- Architect `tools-archfit` skill: add a calibration note that archfit over-scores
  low-evidence / single-crate Rust repos (pre-P0a) and that crate-level metrics are
  blind to intra-crate structure — calibrate accordingly.
- archfit `CLAUDE.md` runtime note + `--help` tagline: mention Rust (tagline still
  says "Go, TypeScript, and Python"); document the crate-vs-module granularity and
  that module depth needs Task 1.

Verification: skills lint clean; docs match behaviour.

## Calibration findings (2026-06-21 all-tools two-repo re-run)

Re-ran archfit with ALL tools (rust + cargo-modules + scip + complexity + clones +
gitnexus), full and delta, on yazi (32-crate workspace) and herdr (single crate), and
re-compared against the architect reviews. Two virtual-workspace showstoppers were
fixed first (commit b148996): cargo 1.96's `workspace_members` ID format
(`path+file:///…/<name>#<version>`) broke SCIP member enumeration, and cargo-modules
needs `--package` per member in a workspace — both made "all tools" silently no-op on
workspaces.

Result: archfit now ORDERS the same as the architect (yazi 43 > herdr 37 ↔ architect
5.4 > 4.7); the pre-fix false-green inversion (herdr 73 > yazi 59) is gone. archfit and
the architect converge on the verdict and core issues. archfit even caught **module
cycles the architect missed** (yazi 26, herdr 5) — the human review only checked
crate/top-level granularity. SCIP coverage was deep (yazi seen=12929, herdr seen=2375).

Status (commit 30ac2c7, branch rebased onto main): **Cal-1, Cal-2, Cal-3, Cal-5 done**
(+ real cargo-modules/coverage bugs fixed: ModeOff zero-coverage, `--bin` target-name,
partial-coverage confidence cap). **Cal-4 and Cal-6 remain** — Cal-4 is a real feature
(file→module LOC plumbing under the filesystem-free core ring); Cal-6 is meta-only and
was reverted once for test friction, so it is deferred as low-value/low-risk-to-defer.

The following calibration items remain (evidence: the two-repo run). None is a broken
tool; they are scoring/labelling refinements.

### Cal-1: dependency_graph_health vs dependency_health name collision

archfit's `dependency_graph_health` measures INTRA-crate module-graph health
(cycles/instability → poor); the architect's same-named dimension measured EXTERNAL
dependency hygiene (clean → good). They are different axes; archfit's number tracks the
architect's `complexity`, not its `dependency_health`. Rename/clarify the dimension (or
split intra-graph health from dependency hygiene) so the two are not conflated.
Files: `internal/score/score.go` (dimension name/summary), docs.

### Cal-2: Rust module-cycle severity should be softer than crate cycles

archfit flags module cycles as `critical` (yazi cycle=26, herdr cycle=5). But Rust
PERMITS module cycles — only crate cycles are forbidden by cargo. The crate-level
"cycles defeat boundaries outright" severity is too harsh at module granularity. Add a
language/granularity-aware cycle penalty: module-level Rust cycles → advisory/serviceable
band, not critical. Files: `internal/score/score.go` `dependencyGraphHealth` (cycle
penalty), and/or the cycle metric tagging module vs crate scope.

### Cal-3: propagation_cost is granularity-dependent

yazi propagation_cost = 0.15 (crate) vs 0.013 (module) — the [0,1] density shrinks with
node count, so the band is not comparable across granularities. Normalise the band by
node count, or report it only at a fixed granularity. File:
`internal/metrics/.../propagation_cost`.

### Cal-4: structural_weight n/a for Rust MODULES (god-module-by-size miss)

structural_weight works at crate level (caught yazi's 2 god crates) but is n/a at module
level, because per-file LOC maps to the crate, not the module node — so archfit missed
herdr's god FILES (server/headless.rs 7,843 lines; integration/mod.rs 6,365) that the
architect flagged. This is the "file→module LOC/churn mapping" polish (also unblocks
change_coupling/cohesion at module granularity). The hard part is the core-ring's
filesystem-free constraint; plumb the cargo-modules member→module file ranges, or map via
the rust file→module convention. Files: `internal/extract/rust/modules.go` (emit per-module
file/LOC), `internal/metrics/internal/modgraph` (FileToModuleKey for module nodes).

### Cal-5: cargo-modules partial on workspaces — surface which crates failed

yazi cargo-modules came back `partial` (some of 31 crates' invocations failed — likely
proc-macro/codegen crates). Record and `log()` the failed crate names in the coverage
reason so the gap is visible, not silent. File: `internal/extract/rust/modules.go`
`runModuleGraph` (collect failedCrate names into Coverage.Reason).

### Cal-6: analysis_confidence ignores the n/a-dimension ratio (non-degenerate case)

Both repos scored analysis_confidence 100 with several low-confidence dims. The
degenerate-graph cap no longer fires once a real module graph exists, but the meta score
still doesn't reflect how many dimensions came back n/a/low. Reconsider the
measured-dimension-ratio cap (reverted earlier for test friction) now that module graphs
make more dims measurable. File: `internal/score/score.go` `analysisConfidence`. Meta-only.
