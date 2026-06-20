# Release notes

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
- Distance is now a composite: `max(code_structure, ownership, deploy_unit)` +
  `runtime_adjust`. Ownership is suppressed in single-author repos. Deploy-unit
  distance (`cross_deploy_unit`) is now reachable via auto-detection.
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
- **Runtime async detection (report-only):** ast-grep patterns detect message-queue
  and event-bus async bridges; confidence recorded.
- **`enrich --subdomains`:** LLM-assisted subdomain drafting with `--pin` workflow
  and `reviewed_at`/`reviewed_by` provenance.
- **BC-aligned report format:** lint-message format `ARCHFIT[BC-UNBALANCED <SEV>]`,
  strength/distance/volatility line, score, why, cheapest-move; `config_hash` for
  reproducibility; "beyond Balanced Coupling" metrics section; distance-confidence
  section.

### Fixes

Correctness and robustness fixes folded into this release. No gate-verdict
changes; output stays byte-for-byte deterministic.

- **Distance precedence chain.** Distance is resolved by precedence (deploy
  boundary > explicit `owner` > resolved multi-owner signal > code structure)
  instead of a flat max, so an explicit `owner:` is no longer overridden by the
  structural fallback. Two unrelated flat (single-segment) module names now read
  as `cross_module_different_owner` instead of collapsing to same-owner.
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
