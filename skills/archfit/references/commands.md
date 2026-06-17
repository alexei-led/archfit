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
- `archfit enrich` — draft off-gate coupling-label refinements for human review
  (see `llm-modes.md`).
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
- `--root` / `-r` — project root for discovery (default: the config's directory).
- `--full` — run the complete analysis instead of the fast subset.
- `--base <ref>` — delta mode: compare against a git ref.
- `--format` — `text` (default), `json`, `markdown`/`md`, or `sarif` (2.1.0).

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
- `1` — fail (active gate findings).
- `2` — warn.
- `3` — usage, config, or runtime error (includes a malformed labels file).

Balanced Coupling advisories are informational by default — use them to
prioritize review, not as automatic pass/fail.

## Full documentation

This file is a distilled, portable subset. The complete guide (configuration
reference, language support, CI) lives at
<https://github.com/alexei-led/archfit/blob/main/docs/guide/README.md>.
