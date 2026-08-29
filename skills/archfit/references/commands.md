# archfit command reference

Portable, self-contained subset of the current `archfit` CLI surface for normal
agent work. Run `archfit --help` or `archfit <cmd> --help` to confirm flags
against the built binary. Maintainer-only `calibrate` is intentionally omitted
here.

## Scope

This reference covers the current released command names, flags, output modes,
and exit codes used by day-to-day agent work. If the binary and docs diverge,
trust the binary and update this file.

## Grounding

Confirm the live surface with tool output: `archfit --help`, `archfit <cmd>
--help`, and the actual command output on the current binary. Do not infer flags
from stale docs or memory.

## Commands

- `archfit doctor` — check local analyzer/tool availability (Go, TS/JS, Python,
  Rust, optional analyzers, AI key/cache). `--fix` installs what archfit can;
  `--fix --dry-run` previews install commands; `--lang <x>` scopes installs.
- `archfit config init` — generate a starter `.archfit.yaml` from discovered
  structure. `--ai-classify` suggests `subdomain`/`volatility`/`layer`/`role` as
  commented-inert YAML by default. `--ai-classify --apply` writes AI judgments
  live into the generated file; review before using as a gate. Use `-o` to
  redirect output to a draft file. See `llm-modes.md`.
- `archfit config update` — sync `.archfit.yaml` with current structure.
  `--ai-classify` adds review-only semantic proposals for unclassified modules;
  even `--ai-classify --apply` applies structural drift only.
- `archfit config update --migration-only` — migrate a config to the current
  schema version and nothing else. `--json` previews, `--apply` writes; not
  combinable with `--ai-classify` or `--refresh`. **Run this first on any config
  written before schema v2** — `version: 1` and the retired
  `coupling.gate.min_band` / `max_drop` keys now exit `3`.
- `archfit config compare <candidate>` — measure one source tree under two
  configurations and report the difference. Report-only: exit `0` on success,
  `3` on an input or runtime error; findings never move the exit code.
- `archfit analyze` — local/report command. Always report-only on success; use it
  for text/JSON/Markdown/SARIF/scorecard output and `--ai-summary` narratives.
- `archfit check` — CI/agent-loop gate. Its exit code is the architecture-state
  verdict (see Exit codes below): gate on `1`, not on any non-zero.
- `archfit baseline` — record accepted current findings as the baseline.
- `archfit explain <id>` — explain one finding by fingerprint prefix
  (`--ai-summary` appends an off-gate narrative; see `llm-modes.md`).
- `archfit config enrich labels` — draft off-gate human-reviewed edge labels for
  cross-module coupling strength.
- `archfit config enrich abstained` — draft labels for abstained
  unknown-strength cross-module edges, judged from snippets.
- `archfit config enrich subdomain / owner / volatility` — draft module metadata;
  `--apply --reviewed-by <name>` writes only approved entries into
  `modules.<name>` and never overwrites live fields. See `llm-modes.md`.

## Common invocations

```sh
archfit doctor
archfit doctor --fix --lang go --lang ts --lang py --lang rust --dry-run
archfit config init --root . --output .archfit.yaml
archfit config update --config .archfit.yaml
archfit analyze --config .archfit.yaml                         # report-only, default text
archfit analyze --format scorecard --config .archfit.yaml
archfit analyze --markdown --config .archfit.yaml > /tmp/archfit-report.md
archfit analyze --ai-summary --config .archfit.yaml
archfit check --config .archfit.yaml                           # CI gate
archfit check --config .archfit.yaml --base origin/main --json
archfit check --config .archfit.yaml --require-tools
archfit check --sarif --config .archfit.yaml > /tmp/archfit.sarif
archfit baseline --config .archfit.yaml
archfit explain <finding-id-prefix> --config .archfit.yaml
archfit explain <finding-id-prefix> --config .archfit.yaml --ai-summary
archfit config enrich labels --config .archfit.yaml
archfit config enrich abstained --config .archfit.yaml
archfit config enrich subdomain --config .archfit.yaml
archfit config enrich owner --config .archfit.yaml
archfit config enrich volatility --config .archfit.yaml
archfit config init --ai-classify --root . -o .archfit-draft.yaml
```

Use `check` for CI gates and agent validation. Use `analyze --markdown` for a
human-readable audit report. Use `analyze --format scorecard` for the banded
summary. Bare `archfit` runs `analyze` in report-only mode.

Prefer stdout or a temp path while reviewing. SARIF, Markdown reports,
`.archfit-cache/`, `.archfit-*.yaml` drafts, and `.archfit-*s.yaml` enrichment
files are generated artifacts.

## Key flags

Shared analysis/check flags:

- `--config` / `-c` — config path (default `.archfit.yaml`).
- `--root` / `-r` where supported — repository root to analyze (default: config
  directory). Decouples the scanned repo from where the config lives.
- `--base <ref>` — score a git ref in addition to HEAD; text/markdown show a
  "CHANGE VS BASE" section. JSON/SARIF are the normal HEAD diagnostic.
- `--format` — `text` (default), `json`, `markdown`/`md`, `sarif`, or
  `scorecard`. Repeatable / comma-separated.
- `--json` / `--markdown` / `--sarif` — shorthands for
  `--format json/markdown/sarif`.
- `--no-advisories` — hide informational Balanced Coupling advisories from
  output.
- `--min-severity <level>` — minimum advisory severity to show (`low`, `medium`,
  `high`, `critical`).
- `--lang <name>` — force an analyzer on. Repeatable.
- `--refresh` — re-run extractors/AI calls and refresh cache entries.
- `--require-tools` — opt-in hard gate: mark missing required analyzer tools as
  failures (use with `check` for CI exit codes).
- `--progress auto|plain|none` — progress output mode.
- `--quiet` / `-q` — suppress progress and non-essential output.

AI/config flags:

- `--ai-summary` — append an off-gate AI advisory narrative to `analyze` or
  `explain` output (needs `ai:` configured or provider/model overrides where
  supported).
- `--ai-classify` — run the off-gate module classification pass for
  `config init` / `config update`.
- `--ai-provider anthropic|openai|ollama` — AI provider override.
- `--ai-model <name>` — AI model override.
- `--apply` — command-specific write mode. For `config init --ai-classify`, writes
  model judgments live into the generated file. For `config update`, applies
  structural drift only. For metadata enrich commands, applies approved draft
  entries.
- `--reviewed-by <name>` — required audit stamp when applying approved metadata
  enrich drafts.

## Coverage gaps and the hard gate

A missing analyzer is **not** scored as healthy: dependent metrics drop to `n/a`
(no evidence — never `strong`) and a coverage gap lists the tool, the metrics its
absence leaves unmeasured, and an install hint. Gaps surface everywhere:
`## Coverage gaps` (md), `## Required tools missing` (scorecard),
`coverage_gaps[]` + `config_warnings[]` (`--format legacy-json` only; the
primary JSON carries `coverage.tools[]`), and stderr.

Default is warn-loud. `archfit check --require-tools` or
`languages.<x>.gate: fail` / `analyzers.<x>.gate: fail` makes missing required
tools block (exit 1, a policy decision distinct from exit 3). Tell users to
install the tool to close the gap, not to disable the gate.

## Output formats

- `text` — human architecture-state report (verdict: HEALTHY / NEEDS ATTENTION /
  BLOCKED), the nine dimension envelopes, the coverage split, and the seam ledger.
- `json` — `archfit.architecture-state.v1` at the document root: `verdict`,
  `decision`, `comparison`, `measurement`, `dimensions`, `coverage`, `findings`,
  `agent_tasks`, `seams`. **No repository score.** `coverage_gaps[]` and
  `config_warnings[]` are NOT in this contract (see `legacy-json`).
- `legacy-json` — the pre-cutover `archfit.diagnostic.v2` envelope, kept for one
  release and selected explicitly. This is where `tool_coverage` detail,
  `owner_source`, `config_warnings`, `git_finding_delta`, and `advisory_tasks`
  still live.
- `markdown` / `md` — the same state, plus the detailed findings audit.
- `sarif` — SARIF 2.1.0 for code-scanning annotations; the state rides in
  `runs[0].properties`.
- `scorecard` — the nine-dimensional state summary. It is not an architecture
  score.

## Finding status lifecycle

- `new` — active finding, not in the baseline.
- `baseline` — accepted finding already recorded; not an error.
- `fixed` — previously baselined finding no longer detected.
- `waived` — active finding covered by an unexpired waiver.
- `expired_waiver` — active finding whose waiver lapsed (gates again).

## Exit codes

`archfit analyze` is report-only and exits `0` on successful analysis, `3` on
usage/config/runtime errors.

`archfit check` uses CI/agent-loop exit codes:

The exit code IS the architecture-state verdict; nothing else participates.

- `0` — `healthy`.
- `1` — `blocked`: active gate finding, or missing required tool under
  `--require-tools` / `analyzers.<x>.gate: fail`. **Gate on this one.**
- `2` — `needs_attention`: no blocker, but an active diagnostic or incomplete
  dimension evidence remains. Read the named missing fact; do not treat it as a
  failure or fabricate evidence to force exit 0. Exit 0 is reachable when every
  required fact is genuinely supplied.
- `3` — usage, config, or runtime error (includes a v1 config under schema v2,
  malformed labels, or missing required AI config for off-gate AI commands).

Exit `1` (policy/finding) is deliberately distinct from exit `3` (error). A
missing required tool you opted to gate on is a gate result, not a crash.

Balanced Coupling advisories are informational by default — use them to
prioritize review, not as automatic pass/fail.

## Full documentation

This file is a distilled, portable subset. The complete guide (configuration
reference, language support, CI) lives at
<https://github.com/alexei-led/archfit/blob/main/docs/guide/README.md>.
