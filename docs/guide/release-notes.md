# Release notes

## v0.6.1 — Skill docs: archfit routing, language guidance, and CLI accuracy

Documentation-only patch release for the built-in `archfit` agent skill and its
portable references.

- **Language-aware skill guidance:** the skill now explicitly covers Go,
  TypeScript/JavaScript, Python, and Rust, with a new `references/languages.md`
  for per-language tool setup, path semantics, and common coverage gaps.
- **Clearer routing:** the skill now says when to stay in `archfit`, when to use
  deeper architecture review, and when to hand off to a language-specific coding
  skill.
- **Safer defaults:** guidance now prefers read-only commands first, temp/stdout
  report paths during review, and explicit treatment of `.archfit-*` drafts,
  SARIF, Markdown reports, and `.archfit-cache/` as generated artifacts.
- **CLI reference refresh:** portable command docs now match the current binary,
  including `score`, `review`, `enrich --subdomains`, Rust-aware tool coverage,
  and current flag semantics.
- **Review grounding:** the skill now requires exact evidence anchors
  (`path:line`, JSON field, or quoted output) and clearer confidence downgrades
  when command execution or analyzer coverage is missing.
- **Install snippets synced:** README and install-guide examples now point at the
  current release tag.

No gate, scoring, or runtime behavior changed.

## v0.7.0 — Balanced Coupling self-alignment

Correctness and honesty fixes across distance, coverage reporting, scope, and
self-config. No breaking changes to gate verdicts, exit codes, or metric names.

### Distance composite model (Task 1)

- **Code-structure baseline always applies.** Distance is computed from a composite
  of code structure, ownership, and deploy unit — not a single-winner precedence.
  Code structure (package-tree position) is the always-available baseline; a
  single-maintainer repo with one owner everywhere does **not** collapse every edge
  to "same owner = low risk". Far-apart modules remain far.
- **Degenerate-owner suppression.** In repos where all modules share the same
  owner, ownership becomes neutral — it does not lower a far code-structure
  distance. Only when multiple distinct owners exist does ownership override the
  structural signal.
- **Deploy unit is the absolute boundary.** Different `deploy_unit` values produce
  `cross_deploy_unit` even when ownership is the same.
- **`distance_basis` field.** Each advisory edge now carries a `distance_basis`
  field (`code_structure`, `ownership`, or `deploy_unit`) recording which signal
  drove the composite. Auditable without re-reading the config.
- **Runtime async bridge (report-only).** When the runtime detector identifies an
  async bridge (message queue, event bus, async task), evidence is recorded per
  module. Never annotates graph edges, never affects distance or score.
  Evidence surfaces in `Diagnostic.RuntimeAsync` and the `runtime_async` JSON field.

### Self-config reclassification (Task 2)

- `internal/extract` is split into 13 explicit adapter submodule stanzas in
  `.archfit.yaml` (`golang`, `ts`, `py`, `rust`, `scip`, `astgrep`, `deployunit`,
  `runtime`, `dynimports`, `clones`, `complexity`, `loc`, `gitnexus`) so adapter
  boundaries are visible, not hidden behind a broad glob.
- `internal/history` reclassified as adapter, not support.
- `internal/score` has an explicit core stanza.
- `cmd/archfit` marked `role: composition_root` — fan-out from the wiring root is
  cohesion, not high-distance coupling.

### Runtime async detection wired end-to-end (Task 3)

`runtime.Detect` is called in the pipeline after dynimports. Detected sites flow
into `RunSignals.RuntimeAsync`, are assembled into `Diagnostic.RuntimeAsync`, and
render in the `runtime_async` JSON field. Evidence-only — never annotates graph
edges, never affects distance, score, or gate verdict.

### `testdata/**` excluded by default (Task 4)

`**/testdata/**` is now in the built-in default exclusion set. Test fixture repos
inside `testdata/` are not production architecture: analysing them created false
coverage gaps and phantom language detections. Re-include intentionally with
`!testdata` or `!**/testdata/**` in your config `exclusions`.

### Clone detection enabled; disabled-vs-missing coverage (Task 5)

- Clone detection (`tools.clones`) is now enabled in archfit's self-config.
  `functional_candidates` reports real cross-module clone pairs when `jscpd` is
  installed.
- Coverage gap reporting now distinguishes **absent** (tool not installed → gap
  with install hint) from **disabled by config** (`enabled: off` → no gap, no
  prompt). A tool you deliberately disabled should not appear as a gap to resolve.

---

## v0.3.0 — Balanced Coupling engine v2 (BREAKING in SCORING)

### Breaking changes

#### Scoring: continuous multiplicative scorer replaces `BalanceResult` bands

The per-edge coupling scorer is now `MultiplicativeScorer` (Balanced Coupling pure
implementation) instead of the `LegacyShim` that wrapped `BalanceResult`.

**What changes:**

- Per-edge `score` and `band` in advisory output shift to continuous values (0–10,
  computed from strength × distance × volatility ordinals).
- `metric_version` bumped to `*.v2` for the two metrics whose input semantics changed:
  `unbalanced_edge.v2` and `change_locality.v2`.
- Distance is now a composite of code structure (always-available baseline),
  ownership (contributes only when genuinely informative — suppressed in
  single-owner repos so one maintainer does not flatten distance to "same owner =
  low risk"), and deploy unit (absolute boundary). Each advisory edge reports a
  `distance_basis` field (`code_structure`, `ownership`, or `deploy_unit`) so the
  result is auditable. Deploy-unit distance (`cross_deploy_unit`) is now reachable
  via auto-detection.
- Gate volatility no longer reads git churn — only explicit `volatility:` config keys
  and the path-pattern heuristic (vendor/lib/util → low; infra/platform/db → medium).

**What does NOT change:**

- Gate verdict behavior is unchanged. No existing gate rule changes its pass/fail
  condition. The same rules produce the same gate findings on the same code.
- Metric _names_ are unchanged.
- Exit codes are unchanged.

**Migrating pinned baselines:**

If you have a `.archfit-baseline.json` from v0.2.x or earlier that contains
`unbalanced_edge` or `change_locality`, the `metric_version` fields will mismatch.
Re-run `archfit baseline` to regenerate:

```sh
archfit baseline --update
```

### New features

- **Continuous per-edge coupling scores:** each advisory now includes an integer score
  in [0, 10], a band (none/low/medium/high/critical), a human-readable reason string,
  and a `cheapest_move` hint (the single dimension change that drops the band).
- **Code-structure distance:** always-available tree-distance baseline; no external
  tool needed.
- **Deploy-unit detection:** Go `main` packages, TS workspaces, Python `pyproject.toml`,
  Dockerfiles, and Kubernetes Deployments/StatefulSets are auto-detected as separate
  deploy units. Config-authored values always win.
- **Domain-volatility heuristic:** path-pattern table fills in volatility when no
  explicit config key is set (vendor/lib/util → low; infra/platform/db → medium).
- **Beyond-BC standard metrics (report-only, never gate):** `instability`,
  `abstractness`, `martin_distance`, `propagation_cost`, `change_coupling`.
- **Connascence tags (report-only):** edges tagged `connascence: type` (SCIP struct/
  interface) or `connascence: algorithm` (clone pair crossing a module boundary).
- **Runtime async detection (report-only):** deterministic FS scan plus optional
  ast-grep patterns detect message-queue, event-bus, and async-task integration
  patterns per language. Evidence flows into `Diagnostic.RuntimeAsync` and renders
  as the `runtime_async` JSON field (`omitempty` when empty). When a bridge is
  detected, evidence is recorded per module in the `runtime_async` JSON field —
  it never annotates graph edges, never affects distance or score, never gates.
- **`enrich --subdomains`:** LLM-assisted subdomain drafting with `--pin` workflow
  and `reviewed_at`/`reviewed_by` provenance.
- **BC-aligned report format:** lint-message format `ARCHFIT[BC-UNBALANCED <SEV>]`,
  strength/distance/volatility line, score, why, cheapest-move; `config_hash` for
  reproducibility; "beyond Balanced Coupling" metrics section; distance-confidence
  section.

### Fixes

Correctness and robustness fixes folded into this release. No gate-verdict
changes; output stays byte-for-byte deterministic.

- **Distance composite model.** Distance is a composite of code structure (always-
  available baseline), ownership (informative only when multiple distinct owners
  exist — suppressed in single-owner repos), and deploy unit (absolute boundary).
  A `distance_basis` field on each advisory edge records which signal drove the
  result. Two unrelated flat (single-segment) module names now read as
  `cross_module_different_owner` via code structure instead of collapsing to
  same-owner. A single-maintainer repo does not get a flat distance map — code
  structure still differentiates close and far modules.
- **Deploy-unit detection robustness.** `package.json` `workspaces` accepts both
  the array form and the `{ "packages": [...] }` object form; detected units are
  remapped to their owning module before filling, so auto-detection works for
  configs whose module keys differ from repo paths.
- **`--no-config` reproducibility.** With `--no-config`, the reported
  `config_hash` is now empty — the ignored on-disk file is no longer hashed, so
  the hash always reflects the config that actually governed the run.
- **Deterministic finding order.** Gate and advisory findings are sorted by a
  total order (severity, rule id, status, endpoints, kind, id), independent of
  upstream map-iteration order.
- **Scorer / metric corrections.** Same-module edges score 0 with no spurious
  band; `change_coupling` is clamped to ≤100%; `abstractness` no longer
  saturates; low-volatility composition roots no longer raise false BC warnings.
- **Internal boundary.** Label-file I/O moved to `internal/labels/labelsio`; the
  engine's import closure is now free of label-loading I/O (enforced by an arch
  test and the `engine_no_labelsio` gate rule).

### Calibration

MultiplicativeScorer was selected over AdditiveScorer by running `make calibrate`
on archfit (Go) + redwoodjs/redwood (TS) + saleor/saleor (Py): 150 edges scored,
105 band-agreement with hand-judged reference, 70% agreement rate.

---

## v0.3.1 — Gap-closure correctness fixes

Correctness fixes with no breaking changes to gate verdicts, exit codes, or
metric names.

### Correctness fixes

- **`review` post-verify: band vocabulary enforcement.** The post-verify pass now
  validates `overall_band` and each dimension's `band` against the five-value
  rubric (`critical / poor / mixed / serviceable / strong`). A fabricated label
  (e.g. "excellent") is blanked on `overall_band` and causes the dimension entry
  to be dropped. Previously only module and dimension names were validated.

- **`review` post-verify: subdomain vocabulary enforcement.** `subdomain_suggestions`
  entries are now dropped when `suggested_subdomain` is not in the fixed DDD set
  (`core`, `supporting`, `generic`). Previously any string was accepted.

- **`review` post-verify: dynamic-import modules accepted as valid evidence.**
  Modules appearing only as dynamic/lazy imports (no static finding or file fact)
  were incorrectly rejected by the post-verify module filter. They are now
  accepted, so the LLM can cite lazy-import coupling risks without the reference
  being dropped.

- **`review` prompt: dynamic/lazy imports surfaced.** Dynamic imports detected
  by the TypeScript and Python extractors are now included in the review prompt
  as a hidden-coupling risk section. The static dependency graph is blind to
  these edges; surfacing them lets the narrative flag hidden coupling the
  metrics miss.

- **`coupling_balance` score: only active findings counted.** `bcEdges` (the
  coupling scorer's input) now filters to `new` and `expired_exception` findings
  only, matching the filter the gate verdict and `activeGateFindings` use.
  Baseline-accepted and excepted edges are operator-suppressed debt; counting them
  deflated `coupling_balance` for repos that had triaged their coupling.

- **`dependency_graph_health` score: `instability` and `propagation_cost` mark
  the dimension as measured.** Both metrics were applied as penalties but did not
  set the `measured` flag, so the dimension could be scored as `n/a` even when
  evidence was present.

- **Coverage confidence propagation to scorecard.** `coverageConfidence` now takes
  the lower of the coverage-value confidence and the metric's own confidence
  (derived from the unresolved-import ratio). A run with 100% file coverage but
  many unresolved imports no longer receives `high` confidence on the scorecard.

- **`change_locality` src-strip: canonical root only.** The path alias that strips
  a leading source-root segment for Python src-layout repos now only strips `src/`
  (the conventional source root), not any leading directory. Stripping any first
  segment would attribute changes in `tests/app/handlers.py` to the source module
  `app.handlers`, inflating the metric.

- **Clone coverage: files-scanned, not clone-pair count.** `FilesSeen` and
  `FilesApplicable` in the jscpd extractor now report `statistics.total.sources`
  (the number of source files jscpd scanned), not the number of clone pairs found.
  A repo with 200 files and 4 clone pairs previously reported coverage of 4.

- **`map_review` gate: `gate: off` disables staleness review.** Setting
  `map_review.gate: off` now correctly disables the staleness advisory pass,
  consistent with `gate: off` semantics everywhere else in the config. Previously
  `off` was treated as a non-empty gate string and enabled the pass.

---

## v0.4.0 — Gap-closure release

Closes the tool-vs-expert gap with new commands, metrics, and config UX. No
breaking changes to gate verdicts, exit codes, or metric names.

- **`score`:** per-dimension banded scorecard (`archfit score`).
- **`review`:** off-gate LLM architecture review of the collected evidence.
- **Cohesion metric** added to the modularity dimension.
- **TypeScript dynamic-imports extractor:** report-only hidden-coupling signal the
  static graph misses.
- **Config-quality lint** for under-specified modules (missing owner/subdomain/etc.).
- **gitnexus auto-detection** of a present `.gitnexus/` / `.codegraph` index.
- **Starter example configs:** go-monolith, microservices, ddd, python-package,
  ts-monorepo.
- _Fixes:_ measurement and scoring correctness fixes folded in from multiple review
  passes.

---

## v0.4.1 — Documentation

Documentation-only release: AI-agent-oriented README rewrite, v0.4.0 references,
and gap-closure notes. No code changes.

---

## v0.5.0 — Reliability hardening + off-gate LLM self-driving

The `check` gate is now trustworthy in CI and agent loops: no false greens, honest
coverage, and an LLM layer that drafts config without ever touching the gate.

- **Honest `n/a`:** `coverage` and `encapsulation` report `n/a` (not a false `1.0`)
  when no extractor ran or the graph is empty; `analysis_confidence` caps every
  dependent metric's band.
- **Loud coverage gaps:** coverage gaps and config warnings surface in stderr, JSON,
  and Markdown; opt-in hard gate for missing tools (`--require-tools` /
  `tools.<x>.gate: fail`).
- **Delta bucketing:** findings grouped as new / existing / resolved /
  severity_changed / touched.
- **`--root`** decouples the scan root from `--config` for external CI (check / scan
  / score / enrich).
- **`autopilot`** drafts a full `.archfit.yaml` in review-only mode (never overwrites).
- **`enrich --owner` / `--volatility`** drafts; `.env` autoload for LLM keys.
- _Fixes:_ dotenv no longer overwrites an explicitly-empty env var (a CI opt-out is
  preserved).

---

## v0.5.1 — Homebrew release plumbing

- Dispatch Homebrew tap updates after stable releases; skip prerelease tags.
- Restore clean local lint/test gates; ignore an inherited hook git env during
  repository detection.

---

## v0.5.2 — Homebrew formula generation

- Generate the Homebrew formula locally and push it directly to the tap (a Ruby
  script fetches `SHA256SUMS` from the published release), removing the dependency on
  the tap's auto-update workflow.

---

## v0.6.0 — Rust language support

archfit now analyzes Rust alongside Go, TypeScript, and Python — the same gates,
Balanced-Coupling advisories, and scorecard apply.

- **Rust crate analysis** via `cargo metadata`: one graph node per workspace member,
  `external:` dependency nodes, and `depends_on` edges. Enable with
  `tools.rust.enabled: auto` (needs `cargo` on `PATH`).
- **Opt-in intra-crate module graph** (`tools.cargo-modules.enabled`, the
  `<crate>::<mod>` graph) and **rust-analyzer SCIP integration strength**
  (`tools.scip.enabled`), so single-crate repos get real cycle, blast-radius,
  cohesion, and god-file signal instead of a single crate-level node.
- **Module-level size & history metrics:** per-file LOC and git churn now resolve to
  `<crate>::<mod>` keys, so `structural_weight`, `change_coupling`,
  `change_amplification`, `hidden_coupling`, and `functional_candidates` measure at
  module granularity. `lizard` complexity is extended to `.rs`.
- _Fix — honest scorecard on sparse graphs:_ a degenerate (<2-node) graph never scores
  `strong`, and `analysis_confidence` reflects the share of measured structural
  dimensions rather than reading high on tool coverage alone.
- _Fix — polyglot cycles:_ a Go/TypeScript import cycle is no longer softened when Rust
  edges dominate the graph by count; Rust module cycles band below crate cycles
  (cargo permits module cycles, forbids crate cycles).
- _Fix — partial coverage honesty:_ a partial `cargo-modules` run caps the affected
  dimensions' confidence and names the crates that failed, instead of presenting a
  confident verdict.
- _Upgrade note:_ the Docker image does not bundle the Rust toolchain — Rust analysis
  reports `n/a` (never fails) there; run on a host with `cargo`, or extend the image.
  Optional depth: `cargo install cargo-modules` and `rustup component add rust-analyzer`.
