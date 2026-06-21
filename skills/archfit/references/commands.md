# archfit command reference

Portable, self-contained subset of the `archfit` CLI surface for normal agent
work. Run `archfit --help` or `archfit <cmd> --help` to confirm flags against
the installed version. Maintainer-only `calibrate` is intentionally omitted here.

## Commands

- `archfit doctor` — check the local toolchain (Go, TS/JS, Python, Rust,
  optional analyzers, LLM key, cache).
- `archfit init` — generate a starter `.archfit.yaml` from discovered structure.
- `archfit update` — sync `.archfit.yaml` with current structure (see
  `llm-modes.md`).
- `archfit check` — run architecture gates and metrics (the CI/agent gate).
- `archfit score` — print the banded scorecard (`check --format scorecard`
  packaged as a read-only command).
- `archfit review` — off-gate LLM narrative over deterministic evidence (see
  `llm-modes.md`).
- `archfit scan` — produce a full Markdown audit report.
- `archfit baseline` — record accepted current findings as the baseline.
- `archfit explain <id>` — explain one finding by fingerprint prefix (`--llm`
  appends an off-gate narrative; see `llm-modes.md`).
- `archfit enrich` — draft off-gate human-reviewed labels and metadata;
  `--subdomains`, `--owner`, and `--volatility` draft module metadata,
  `--pin` writes approved entries into the config (see `llm-modes.md`).
- `archfit autopilot` — one-shot off-gate LLM draft of a full `.archfit.yaml`
  (review-only; never applies — writes `.archfit-autopilot.yaml`; see
  `llm-modes.md`).
- `archfit install` — install or print install commands for optional language
  tools.

## Common invocations

```sh
archfit doctor
archfit install --lang go --lang ts --lang py --lang rust --dry-run
archfit init --root . --output .archfit.yaml
archfit update --config .archfit.yaml
archfit check --config .archfit.yaml --full
archfit check --config .archfit.yaml --base origin/main
archfit check --config .archfit.yaml --full --format json
archfit check --config .archfit.yaml --full --format scorecard
archfit check --config .archfit.yaml --full --require-tools
archfit score --config .archfit.yaml --full
archfit review --config .archfit.yaml
archfit scan --config .archfit.yaml > /tmp/archfit-report.md
archfit check --config .archfit.yaml --format sarif > /tmp/archfit.sarif
archfit baseline --full --config .archfit.yaml
archfit explain <finding-id-prefix> --config .archfit.yaml
archfit explain <finding-id-prefix> --config .archfit.yaml --llm
archfit enrich --config .archfit.yaml
archfit enrich --subdomains --config .archfit.yaml
archfit enrich --owner --config .archfit.yaml
archfit enrich --volatility --config .archfit.yaml
archfit autopilot --root . --output .archfit-autopilot.yaml
```

Use `check` for gates. Use `score` for the banded summary. Use `scan` for a
human-readable audit report.

Prefer stdout or a temp path while reviewing. SARIF, Markdown reports,
`.archfit-cache/`, and `.archfit-*.yaml` drafts are generated artifacts.

## Key flags

- `--config` / `-c` — config path (default `.archfit.yaml`).
- `--root` — repository root to analyze (default: the config's directory).
  Decouples the scanned repo from where the config lives. On `init`/`update` it
  is the discovery root (`-r`).
- `--full` — run the complete analysis instead of the fast subset (also on
  `score`, where it is implied when `--base` is absent).
- `--base <ref>` — delta mode: compare against a git ref.
- `--format` — `text` (default), `json`, `markdown`/`md`, `sarif`, or
  `scorecard`. `check` accepts repeatable `--format`.
- `--require-tools` — opt-in hard gate (`check` / `scan`): exit `1` if any
  required analyzer tool is missing.
- `--advisory` — include informational Balanced Coupling advisories in output.
- `--report` — render findings but never fail for architecture violations.
- `--no-config` — skip `.archfit.yaml` and use built-in defaults for an ad-hoc
  scan.
- `--severity <level>` — minimum advisory severity to show (`low`, `medium`,
  `high`, `critical`).
- `--lang <name>` — force an analyzer on (`install` / `check --no-config`).

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
- `json` — machine-actionable; carries `agent_tasks[]`, metrics, coverage gaps,
  config warnings, and structural facts (see `agent-loop.md`).
- `markdown` / `md` — human-readable findings plus coverage/config sections.
- `sarif` — SARIF 2.1.0 for code-scanning annotations.
- `scorecard` — the banded deterministic score summary (same output family as
  `archfit score`).

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
- `3` — usage, config, or runtime error (includes malformed labels or missing
  required LLM config for off-gate LLM commands).

Exit `1` (policy/finding) is deliberately distinct from exit `3` (error). A
missing required tool you opted to gate on is a gate result, not a crash.

Balanced Coupling advisories are informational by default — use them to
prioritize review, not as automatic pass/fail.

## Full documentation

This file is a distilled, portable subset. The complete guide (configuration
reference, language support, CI) lives at
<https://github.com/alexei-led/archfit/blob/main/docs/guide/README.md>.
