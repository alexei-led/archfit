# Commands

Common commands:

```sh
archfit doctor
archfit init --root . --output .archfit.yaml
archfit check --config .archfit.yaml --full
archfit check --config .archfit.yaml --base main
archfit check --config .archfit.yaml --format json
archfit scan --config .archfit.yaml > archfit-report.md
archfit baseline --full --config .archfit.yaml
archfit explain <finding-id-prefix> --config .archfit.yaml
archfit enrich --config .archfit.yaml
archfit explain <finding-id-prefix> --llm
archfit check --format sarif > archfit.sarif
```

Use `check` for gates. Use `scan` for a human-readable audit report.

## Command summary

- `archfit doctor` — check available local toolchain.
- `archfit init` — generate a starter `.archfit.yaml`.
- `archfit check` — run architecture gates and metrics.
- `archfit scan` — produce a full Markdown audit report.
- `archfit baseline` — record accepted current findings.
- `archfit explain <id>` — explain one finding by fingerprint prefix
  (`--llm` appends an off-gate narrative; needs `tools.llm`).
- `archfit enrich` — draft LLM coupling-label refinements for human review
  (off-gate; writes `.archfit-labels.yaml` drafts — see
  [llm-enrich.md](llm-enrich.md)).
- `archfit install` — install or print commands for optional language tools.

Output formats for `check`: `text`, `json`, `markdown`/`md`, `sarif`
(SARIF 2.1.0 for CI code-scanning annotations).

For wiring archfit into an AI coding agent's loop (`agent_tasks`, SARIF,
`change_locality`), see [agent-feedback.md](agent-feedback.md).

## Finding status

Findings have a lifecycle status:

- `new` — active finding not present in the baseline;
- `baseline` — accepted finding already recorded;
- `fixed` — previously baselined finding that is no longer detected;
- `excepted` — active finding covered by an approved exception;
- `expired_exception` — active finding whose exception has expired.

## Exit codes

- `0` — pass;
- `1` — fail;
- `2` — warn;
- `3` — usage, config, or runtime error.

Balanced Coupling advisories are informational by default. Use them to prioritize
architecture review and refactoring, not as automatic pass/fail rules.
