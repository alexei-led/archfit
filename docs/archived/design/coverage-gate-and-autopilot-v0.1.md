<!-- markdownlint-configure-file { "MD013": false, "MD036": false, "MD024": { "siblings_only": true } } -->

# Coverage gate, fail-loud scoring, and autopilot — design v0.1

Status: implemented (2026-06-20). Source: the archfit-vs-architect study under
`reports/archfit-vs-architect-20260620 (claude|openai)/`, which reproduced a set
of false-green defects on both runs.

## 1. Problem

archfit was scoring _absence of evidence as good architecture_. An unanalysed
repo scored "strong": the coverage metric returned 100% when no extractor ran,
`coupling_balance` returned 90 on zero edges, missing analyzers degraded scores
silently, `analysis_confidence` reported 100/100 while three dimensions were
`n/a`, and `architecture_fitness` counted a Go module-cache `_test.go` as an
architecture test. A gate that goes green when it cannot see the code is worse
than no gate — it manufactures false confidence.

The fix has two halves:

- **Fail-loud scoring** (core ring): every metric is either correct or honestly
  `n/a`. Absence is never strong.
- **Warn-loud surfacing + opt-in hard gate** (`cmd/`): the missing tools and the
  metrics they unlock are reported in every format; CI can opt in to block.

The LLM self-driving layer (enrich owners/volatility, autopilot) closes the
root cause — under-specified modules — that leaves the structural metrics `n/a`
in the first place.

## 2. Coverage-gap model

`internal/model/diagnostic.CoverageGap` is the machine-readable record of one
analyzer that did not run:

```go
type CoverageGap struct {
    Tool            string   `json:"tool"`             // coverage name, e.g. "go/packages", "lizard"
    InstallCmd      string   `json:"install_cmd"`      // one-line install hint
    AffectedMetrics []string `json:"affected_metrics"` // metrics that drop to n/a
    Gate            string   `json:"gate"`             // effective posture: off | warn | fail
}
```

It is the warn-loud counterpart of a `Coverage{Status:"absent"}` entry: it turns
"this tool is missing" into "here is what you lose and how to fix it". The
`Diagnostic` carries `CoverageGaps []CoverageGap` and `ConfigWarnings []string`
(both `omitempty`, stdlib only — safe in `internal/model`).

**Where it is built.** The core ring computes bands and confidence from coverage
facts and never sees tool names or config. `cmd/archfit/pipeline.go` owns a
static `toolAffectedMetrics` table (tool → install hint + unlocked metrics) and
builds the gaps from the absent `ToolCoverage` records after the engine returns.
This keeps the ring invariant intact (`internal/arch_test.go`): no config, YAML,
`os`, or tool names cross into the core.

The current table:

| Tool                 | Config key   | Unlocked metrics                                               |
| -------------------- | ------------ | -------------------------------------------------------------- |
| `go/packages`        | `go`         | coverage, coupling_balance, encapsulation, cycle, blast_radius |
| `dependency-cruiser` | `typescript` | coverage, coupling_balance, encapsulation, cycle, blast_radius |
| `grimp`              | `python`     | coverage, coupling_balance, encapsulation, cycle, blast_radius |
| `lizard`             | `complexity` | complexity                                                     |
| `jscpd`              | `clones`     | functional_candidates                                          |
| `gitnexus`           | `gitnexus`   | risk_hub                                                       |

Only tools with an actionable install path appear — an absent coverage entry a
user cannot close is not a gap worth reporting.

## 3. Fail-loud scoring fixes

These are pure changes in the core ring (decide over facts already in the
`Diagnostic`):

- **`coverage`** — when no extractor contributed any coverage record (`len(in.ToolCoverage)==0`)
  return `n/a` (band `n/a`, confidence low). `value=1.0` is kept **only** when
  extractors ran but nothing was applicable.
- **`coupling_balance`** — on empty edges with a low base confidence, return
  `{value 50, confidence low}` with a summary that states the classified-edge
  count, instead of the old `{90, strong}`. A legible 90 now reads "no unbalanced
  coupling among N edges", not a blanket "great".
- **`analysis_confidence`** — when the `coverage` metric band is `n/a`, start at
  60 (not 0) and penalize each absent **primary** extractor (go/packages,
  dependency-cruiser, grimp) −15 (capped at 45) on top of the existing
  semantic-tool penalties, so an all-absent repo lands ≈ 0/critical.
- **`architecture_fitness`** — when the metric is `n/a` (scan never ran) return
  `{value 40, confidence low}` (poor, "scan didn't run") instead of
  `{10, critical}`. `critical` stays only when the metric ran and found 0/3.
  `detectArchTestFiles` also skips `<root>/pkg/mod/**` (Go module cache) and
  `**/testdata/**`, so a vendored module-cache `_test.go` is no longer counted.
- **`boundary_integrity`** — when `encapsulation` is `n/a`, an explicit evidence
  note is added rather than fabricating a value.

The `n/a` vs `critical` distinction is load-bearing and documented in
[metrics.md](../guide/metrics.md): `n/a` is _no evidence_, never _measured and
bad_.

## 4. GateMode — opt-in hard gate

Default posture is **warn-loud, exit 0**: a missing tool drops its metrics to
`n/a` and lists a coverage gap, but never fails the build. CI opts in to blocking
two ways:

- `tools.<x>.gate: fail` in config — per-tool. `GateMode` is `off | warn | fail`
  (default `warn`), validated as an enum on `ToolConfig`.
- `--require-tools` on `check`/`scan` — raises **every** coverage gap to `fail`
  for that run; equivalent to `gate: fail` on every tool.

`cmd/` computes each gap's effective gate and, if any is `fail`, returns
`exitError{code:1}` and stamps the verdict `fail`. **Exit 1 is a policy
violation, distinct from exit 3** (the tool/config/runtime error). This layering
keeps the decision in `cmd/`: the ring computes bands; `cmd/` reads
`tools.<x>.gate` + `--require-tools` and decides the exit code.

`applyToolGate` is idempotent and render-order safe — callers invoke it before
rendering so the output shows the effective (post-override) gate.

## 5. Role-aware modules

In a one-binary Go CLI, a `cmd` package's fan-out into every adapter is
composition-root cohesion, not high-distance coupling. archfit previously scored
those outbound edges as unbalanced, producing false-positive Balanced-Coupling
advisories.

`ModuleDef.role` declares a module's architectural role:
`composition_root | adapter | core | shared_model | generated | test` (optional,
validated enum, rides on the classify config view). When the source module is
`composition_root`, `generated`, or `test`, `capDistanceForRole` downgrades its
outbound `cross_deploy_unit` / different-owner edges to `cross_module_same_owner`,
so the BC advisory severity, the continuous Score, and every distance-reading
metric all read cohesion. A `core → core` unbalanced edge is still flagged;
inbound edges to a wiring module are unaffected.

The role is pure input (arrives via the config view, not I/O). The LLM that
suggests a role per module is wired in `init --llm`/`autopilot` alongside the
owner and volatility drafts.

## 6. Built-in excludes + output-inside-root warning

`internal/scope.DefaultExclusions` is a pure const list of tool-artifact, cache,
and dependency directories archfit never analyses — measuring them yields
non-deterministic or irrelevant facts (a vendored tree's complexity, a generated
index, or a report written back into the scanned repo, which was the study's
self-scan "instability"):

```text
.archfit-cache/  .archfit-baseline.json  .gitnexus/  .codegraph/  reports/
.venv/  node_modules/  vendor/  dist/  build/
```

`MergeExclusions` merges them with — never replaces — the config `exclusions`,
de-duplicated and sorted so a double-run stays byte-identical. A config entry
prefixed `!` re-includes a default (e.g. `!reports` removes the `reports/`
exclusion). The list lives in the core ring as data so `scope` stays free of I/O.

`cmd/` separately warns (a `ConfigWarnings` entry + stderr line) when an
output/report path resolves **inside** the analyzed root — exclude it or write
outside `--root` to keep scans deterministic.

## 7. Delta bucketing

A delta run groups findings into mutually exclusive buckets by lifecycle status
and the changed-file set (`internal/status.DeltaBuckets`), so a reviewer reads
what changed apart from what was already there. Precedence (highest first):

1. **resolved** — a baseline finding no longer detected.
2. **new** — introduced by this change.
3. **severity_changed** — a baseline finding whose severity differs from the
   severity recorded in the baseline (skipped when the baseline predates severity
   tracking).
4. **touched_by_delta** — a pre-existing finding whose edge endpoints sit on a
   changed file.
5. **existing** — any remaining pre-existing finding, untouched by this change.

Rendered as a `## Delta` section in markdown/scorecard and an additive
(`omitempty`) grouping in the JSON delta payload.

## 8. Autopilot — one-shot config drafter (review-only)

`archfit autopilot` discovers project structure, classifies every module
(subdomain / volatility / layer / role), drafts an owner per module from
CODEOWNERS context, and renders the whole `.archfit.yaml` in **plan mode** —
every suggestion is a commented YAML line, nothing is applied. The draft lands in
a separate review file (`.archfit-autopilot.yaml` by default); autopilot
**refuses** to write `.archfit.yaml` directly (exit 3 on `--output .archfit.yaml`).

It reuses `init`'s discover + classify plumbing and `enrich`'s owner-draft pass.
Like every LLM command it lives in `cmd/` — the LLM layer never crosses into the
core ring (enforced by `internal/arch_test.go`). The human reviews the draft and
applies fields deliberately; nothing the model proposes is auto-trusted.

`enrich --owner` and `enrich --volatility` are the targeted counterparts: draft a
single field per module into `.archfit-owners.yaml` / `.archfit-volatility.yaml`,
then `--pin` writes only the approved entries into `modules.<name>` (never
overwriting a live field). Filling `owner`/`subdomain`/`volatility` is the
through-line that makes distance classification work → `encapsulation` becomes
measurable → `boundary_integrity` and `coupling_balance` stop being `n/a`.

## 9. .env autoload

`cmd/archfit/main.go` best-effort loads `.env` (cwd) at startup so the LLM
commands pick up an API key from a local file without polluting the shell. A key
is set **only when `os.Getenv(key)==""`** — real environment variables and CI
secrets always win. `.env` is gitignored. Supports `export KEY=VALUE`, comments,
blank lines, and optional surrounding quotes.

## 10. Determinism

Every output change in this work is additive and `omitempty`, so clean fixtures
are unchanged. The `internal/engine/golden_test.go` double-run must stay
byte-identical; `coverage_gaps`, `config_warnings`, and the delta buckets all
sort deterministically. Any intended schema change bumps the relevant
`metric_version` and regenerates the golden deliberately, inspecting the diff —
never auto-accept.
