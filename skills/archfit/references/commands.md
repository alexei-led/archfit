# archfit command reference

Portable, self-contained subset of the `archfit` CLI surface. Run `archfit --help`
or `archfit <cmd> --help` to confirm flags against the installed version.

## Commands

- `archfit doctor` — check the local toolchain (Go/TS/Python tools, LLM key, cache).
- `archfit init` — generate a starter `.archfit.yaml` from discovered structure.
- `archfit update` — sync `.archfit.yaml` with the current structure (see
  `llm-modes.md`).
- `archfit check` — run architecture gates and metrics (the CI/agent gate).
- `archfit scan` — produce a full Markdown audit report.
- `archfit baseline` — record accepted current findings as the baseline.
- `archfit explain <id>` — explain one finding by fingerprint prefix (`--llm`
  adds an off-gate narrative; see `llm-modes.md`).
- `archfit enrich` — draft off-gate coupling-label refinements for human review;
  `--owner` / `--volatility` draft those module fields, `--pin` writes approved
  entries into the config (see `llm-modes.md`).
- `archfit autopilot` — one-shot off-gate LLM draft of a full `.archfit.yaml`
  (review-only; never applies — writes `.archfit-autopilot.yaml`; see `llm-modes.md`).
- `archfit install` — install or print install commands for optional language tools.

## Common invocations

```sh
archfit doctor
archfit init --root . --output .archfit.yaml
archfit check --config .archfit.yaml --full
archfit check --config .archfit.yaml --base main          # delta vs a git ref
archfit check --config .archfit.yaml --full --format json
archfit check --config .archfit.yaml --format sarif > archfit.sarif
archfit scan --config .archfit.yaml > archfit-report.md
archfit baseline --full --config .archfit.yaml
```

Use `check` for gates; use `scan` for a human-readable audit report.

## Key flags

- `--config` / `-c` — config path (default `.archfit.yaml`).
- `--root` — repository root to analyze (default: the config's directory).
  Decouples the scanned repo from where the config lives (e.g. an external CI
  config). On `init`/`update` it is the discovery root (`-r`).
- `--full` — run the complete analysis instead of the fast subset (also on
  `score`, where it is implied when `--base` is absent).
- `--base <ref>` — delta mode: compare against a git ref.
- `--require-tools` — opt-in hard gate (`check`/`scan`): exit `1` if any required
  analyzer tool is missing (≡ `tools.<x>.gate: fail` for every tool).
- `--format` — `text` (default), `json`, `markdown`/`md`, `sarif` (2.1.0), or
  `scorecard` (banded 7-dimension synthesis).

## Coverage gaps and the hard gate

A missing analyzer is **not** scored as healthy: dependent metrics drop to `n/a`
(no evidence — never `strong`) and a coverage gap lists the tool, the metrics its
absence leaves unmeasured, and an install hint. Gaps surface everywhere:
`## Coverage gaps` (md), `## Required tools missing` (scorecard),
`coverage_gaps[]` + `config_warnings[]` (json), and stderr.

Default is warn-loud (exit 0). `--require-tools` or `tools.<x>.gate: fail` makes
it block (exit 1, a policy decision distinct from exit 3). Tell users to install
the tool to close the gap, not to disable the gate.

## Output formats

- `text` — human summary.
- `json` — machine-actionable; carries `agent_tasks[]`, metrics, and structural
  facts (see `agent-loop.md`).
- `sarif` — SARIF 2.1.0 for GitHub code-scanning annotations.

## Finding status lifecycle

- `new` — active finding, not in the baseline.
- `baseline` — accepted finding already recorded; not an error.
- `fixed` — previously baselined finding no longer detected.
- `excepted` — active finding covered by an unexpired exception.
- `expired_exception` — active finding whose exception lapsed (gates again).

## Exit codes

- `0` — pass.
- `1` — fail: an active gate finding, **or** a missing required tool under
  `--require-tools` / `tools.<x>.gate: fail` (a policy violation).
- `2` — warn.
- `3` — usage, config, or runtime error (includes a malformed labels file).

Exit `1` (policy/finding) is deliberately distinct from exit `3` (error). A
missing required tool you opted to gate on is a gate result, not a crash.

Balanced Coupling advisories are informational by default — use them to
prioritize review, not as automatic pass/fail.

## Full documentation

This file is a distilled, portable subset. The complete guide (configuration
reference, language support, CI) lives at
<https://github.com/alexei-led/archfit/blob/main/docs/guide/README.md>.
