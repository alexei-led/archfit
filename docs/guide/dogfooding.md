# Dogfooding: archfit on archfit

`archfit` runs on its own codebase, locally and in CI, using the `.archfit.yaml`
at the repository root. Reading that config is the fastest way to see a realistic,
non-trivial setup — it is the same file the project ships, not a sanitized
example.

The key distinction when reading any archfit run is **violations vs. signals**.

## Violations and regressions gate; signals inform

A **violation** is a gate finding. It has a `gate` of `fail` (or `warn`) and it
sets the exit code. Violations are the deterministic contract: the same code and
config always produce the same verdict, and CI fails when a new one appears.

A **regression** is a metric delta that worsens past its threshold against the
committed baseline. Regressions gate by default (`metrics.<name>.gate` unset =
`fail`) — a new cycle or an encapsulation drop fails the dogfood run even though
the metric's absolute value never does. Downgrade a metric with `gate: warn`/`off`
if you decide its regressions are not enforceable.

A **signal** is report-only. Metric absolute values, `blast_radius`, and Balanced
Coupling advisories describe the shape of the architecture — coupling risk,
blast radius, cycle count — but they never fail the build on their own. They are
there to inform a human or an AI agent, and to feed `archfit analyze --llm`. One
signal is promotable: the synthesised `coupling_balance` score gates when a
`coupling.gate` block is configured (archfit's own config has none, so the score
stays report-only here).

| Aspect       | Violation                             | Regression                                   | Signal                                                    |
| ------------ | ------------------------------------- | -------------------------------------------- | --------------------------------------------------------- |
| Source       | `rules` with `gate: fail`/`warn`      | `metrics` delta vs baseline, `coupling.gate` | metric absolute values, BC advisories                     |
| Effect       | sets exit code; fails CI              | fails CI unless downgraded per metric        | report-only; never gates                                  |
| Determinism  | byte-identical, gate-grade            | byte-identical, gate-grade                   | deterministic, but advisory                               |
| Examples     | forbidden dependency, cycle, API leak | new cycle, encapsulation drop, coverage drop | `blast_radius`, `coupling_balance` advisories, BC rollups |
| Acting on it | must fix or baseline/except           | fix, or re-baseline to accept the new level  | judgement call; prioritize, don't block                   |

A rising signal is **not** a build break — treat it as a prompt to look. A
regression **is** a build break by default: either fix it or deliberately accept
the new level with `archfit baseline`.

## What archfit enforces on itself (violations)

From the project `.archfit.yaml` and the structural gates in `CLAUDE.md`:

- **Core-ring import invariants** (`internal/arch_test.go`, run as
  `go test ./internal/ -run TestArchImports`) — the decision core
  (`classify`, `rules`, `metrics`, `status`, `staleness`, `facts`, `scope`,
  `score`) must not import `os`, `os/exec`, a YAML library, or adapter packages;
  LLM SDKs are reachable only from off-gate command code (`config init --llm`,
  `config update --llm`, `config enrich`, `analyze --llm`, and `explain --llm`),
  never the deterministic gate path.
- **Forbidden dependencies and layer direction** declared as `rules` in the
  config (e.g. the historical engine→scope inversion guard, gated `warn`).
- **Golden output** (`go test ./internal/engine/ -run TestGolden`) — emitted
  output is byte-stable; a change is regenerated deliberately after inspecting
  the diff, never automatically.

These are the executable form of the architecture intent. If one breaks, the
build breaks.

## What archfit only reports on itself (signals)

The self-config enables several analyzers so the project dogfoods its own
capabilities:

- **`coupling_balance` advisories** — the balance formula runs over all
  cross-module edges. With every module declaring `owner`, `subdomain`, and
  `volatility`, advisories are well-classified and display real strength/distance/
  volatility breakdowns (needs `analyzers.scip` for strength hints).
- **`blast_radius`** — fan-in reach for each module, highlighting the high-fan-in
  shared domain types (`Diagnostic`, graph nodes, coupling constants).
- **`cycle`** and **`encapsulation`** — reported per module; `encapsulation`
  becomes measurable once `public:`/`internal:` globs are declared.
- **Clone detection** (`analyzers.clones`, `jscpd`) — enabled in the self-config.
  When `jscpd` is installed, cross-module clone pairs are found and their edges are
  upgraded to `symmetric` strength in the coupling-balance scorer. When `jscpd` is
  absent, the metric reports `n/a` with an install hint. A disabled-by-config tool
  produces no coverage gap at all.

Their absolute values do not fail the build. Baseline deltas can still gate for
metrics such as `cycle` and `encapsulation`, and `coupling_balance` can gate when
`coupling.gate` is configured. The signals show up in `archfit analyze --markdown`
and the JSON bundle that `archfit analyze --llm` narrates.

## See it yourself

```sh
archfit analyze --gate --config .archfit.yaml --full     # gates only: the verdict
archfit analyze --markdown --config .archfit.yaml --full # gates + signals, as Markdown
```

The expected result on a clean checkout is **pass with signals** — no violations
or metric regressions, plus architecture signals for review. That is the normal
steady state: signals are information, not debt to chase to zero.
