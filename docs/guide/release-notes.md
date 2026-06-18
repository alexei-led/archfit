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
