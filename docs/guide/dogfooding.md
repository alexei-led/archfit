# Dogfooding: archfit on archfit

`archfit` runs on its own codebase, locally and in CI, using the `.archfit.yaml`
at the repository root. Reading that config is the fastest way to see a realistic,
non-trivial setup — it is the same file the project ships, not a sanitized
example.

The key distinction when reading any archfit run is **violations vs. signals**.

## Violations gate. Signals inform.

A **violation** is a gate finding. It has a `gate` of `fail` (or `warn`) and it
sets the exit code. Violations are the deterministic contract: the same code and
config always produce the same verdict, and CI fails when a new one appears.

A **signal** is report-only. Metrics and Balanced Coupling advisories describe the
shape of the architecture — coupling risk, blast radius, cycle count — but they
never fail the build on their own. They are there to inform a human or an AI
agent, and to feed `archfit analyze --llm`.

| Aspect       | Violation                             | Signal                                                    |
| ------------ | ------------------------------------- | --------------------------------------------------------- |
| Source       | `rules` with `gate: fail`/`warn`      | `metrics`, BC advisories                                  |
| Effect       | sets exit code; fails CI              | report-only; never gates                                  |
| Determinism  | byte-identical, gate-grade            | deterministic, but advisory                               |
| Examples     | forbidden dependency, cycle, API leak | `blast_radius`, `coupling_balance` advisories, BC rollups |
| Acting on it | must fix or baseline/except           | judgement call; prioritize, don't block                   |

A new signal is **not** a build break. Treat a rising signal as a prompt to look,
not as a failure. Only promote a signal to a gate (give a metric a `gate`, or add
a `rule`) once you have decided it is a real, enforceable boundary.

## What archfit enforces on itself (violations)

From the project `.archfit.yaml` and the structural gates in `CLAUDE.md`:

- **Core-ring import invariants** (`internal/arch_test.go`, run as
  `go test ./internal/ -run TestArchImports`) — the decision core
  (`classify`, `rules`, `metrics`, `status`, `staleness`, `facts`, `scope`,
  `score`) must not import `os`, `os/exec`, a YAML library, or adapter packages;
  LLM SDKs are reachable only from `enrich`/`explain`/`analyze --llm`, never the
  deterministic gate path.
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

None of these can fail the build. They show up in `archfit analyze --markdown` and
the JSON bundle that `archfit analyze --llm` narrates.

## See it yourself

```sh
archfit analyze --gate --config .archfit.yaml --full     # gates only: the verdict
archfit analyze --markdown --config .archfit.yaml --full # gates + signals, as Markdown
```

The expected result on a clean checkout is **pass with signals** — no violations,
plus a set of report-only metrics describing the current architecture. That is
the normal steady state: signals are information, not debt to chase to zero.
