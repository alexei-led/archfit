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
shape of the architecture — coupling risk, blast radius, complexity hot spots,
change locality — but they never fail the build on their own. They are there to
inform a human or an AI agent, and to feed `archfit analyze --format scorecard`
and `archfit analyze --llm`.

| Aspect       | Violation                             | Signal                                  |
| ------------ | ------------------------------------- | --------------------------------------- |
| Source       | `rules` with `gate: fail`/`warn`      | `metrics`, BC advisories                |
| Effect       | sets exit code; fails CI              | report-only; never gates                |
| Determinism  | byte-identical, gate-grade            | deterministic, but advisory             |
| Examples     | forbidden dependency, cycle, API leak | `risk_hub`, `blast_radius`, BC rollups  |
| Acting on it | must fix or baseline/except           | judgement call; prioritize, don't block |

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

The self-config turns several report-only metrics on so the project dogfoods its
own newest capabilities:

- `risk_hub` — cross-module symbol surface-breadth × explicit volatility (needs
  `tools.scip`).
- `architecture_fitness` — archfit enforces its own architecture
  (`internal/arch_test.go` plus an arch-linter in CI), so this scores a real,
  high enforcement signal.
- `functional_candidates` — surfaced on; clone detection (`tools.clones`) is
  enabled in archfit's self-config. When `jscpd` is installed, this metric reports
  real cross-module clone pairs. When `jscpd` is absent, it reports `n/a` with an
  install hint. A disabled-by-config tool produces no coverage gap at all.

None of these can fail the build. They show up in `archfit analyze --markdown`,
`archfit analyze --format scorecard`, and the JSON bundle that
`archfit analyze --llm` narrates.

## See it yourself

```sh
archfit analyze --gate --config .archfit.yaml --full             # gates only: the verdict
archfit analyze --markdown --config .archfit.yaml --full         # gates + signals, as Markdown
archfit analyze --format scorecard --config .archfit.yaml --full # the banded 7-dimension scorecard
```

The expected result on a clean checkout is **pass with signals** — no violations,
plus a set of report-only metrics describing the current architecture. That is
the normal steady state: signals are information, not debt to chase to zero.
