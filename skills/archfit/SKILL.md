---
name: archfit
description: Use archfit for architecture fitness work. Use when installing archfit, creating or reviewing .archfit.yaml, configuring Go, Python, TypeScript, or Rust checks, adding CI usage, interpreting findings, fixing architecture drift, or deciding on baselines and exceptions. Not for generic software-architecture advice or deciding whether to adopt archfit.
license: Apache-2.0
compatibility: Requires access to the repository files. Shell access is recommended for archfit, git, and language tool checks; if command execution is unavailable, stay doc-only and lower confidence.
metadata:
  author: alexei-led
  tags:
    - architecture
    - fitness
    - go
    - python
    - typescript
    - rust
    - coupling
---

# Archfit

Thin skill for `archfit` setup, configuration, review, and finding repair.
Detailed behavior lives in `references/`, loaded on demand.

**Do not use** for generic software-architecture advice unrelated to `archfit`,
or to decide whether to adopt `archfit` — use web research for that.

## Routing

- Stay in this skill for deterministic `archfit` setup, config review, CI wiring,
  finding interpretation, baseline/exception decisions, and constrained repair.
- Use a deeper architecture-review skill when the job is broader than `archfit`
  output: module design, intended architecture, tradeoff judgment, or repo-wide
  structural assessment.
- Use a language-specific coding skill for deep ecosystem work beyond archfit:
  package-manager setup, dependency installation strategy, or language-specific
  source fixes.

## References

Read the one the task needs:

- `references/commands.md` — normal CLI surface, flags, output formats, finding
  statuses, exit codes, coverage gaps, and the `--require-tools` hard gate.
- `references/languages.md` — Go, TypeScript/JavaScript, Python, and Rust tool
  setup, config shape, path semantics, and common coverage gaps.
- `references/llm-modes.md` — `analyze --llm`, `init`/`update --llm`, `enrich`
  (labels, `--subdomains`, `--owner`, `--volatility`), `autopilot`, `.env`,
  and `explain --llm`.
- `references/agent-loop.md` — autonomous repair contract (`agent_tasks`, SARIF,
  `change_locality`), and how coverage gaps read in the loop.

`archfit --help` and `archfit <cmd> --help` confirm flags. When a reference and
the binary disagree, trust the binary, say which reference was stale, and update
it if this repo owns it. Full guide:
<https://github.com/alexei-led/archfit/blob/main/docs/guide/README.md>.

If shell or command execution is unavailable, use local docs and references only,
state that verification was skipped, and lower confidence.

## Safe defaults

- Inspect existing config, baseline, CI, and package files before proposing edits.
- Prefer non-failing commands first: `archfit --help`, `archfit doctor`, and
  report-only `archfit analyze` (no `--gate` → always exit 0 on success; add
  `--format scorecard` or `--markdown` for those views).
- Prefer stdout or a temp path for reports and SARIF during review. Treat
  `.archfit-cache/`, `.archfit-*.yaml`, `archfit.sarif`, and Markdown reports as
  generated artifacts; do not leave them in the repo unless the task calls for it.
- Keep early config narrow: modules, layers, public APIs, and high-value rules.

## Write or fix

Install, configure, add CI, baseline, add an exception, or fix findings.

1. Inspect first: `.archfit.yaml`, `.archfit-baseline.json`, CI workflows,
   language package files, and existing generated `archfit` artifacts.
2. Detect the repo languages and read `references/languages.md` for each one in
   scope before suggesting tool setup or config globs.
3. Check the surface: `archfit --help`, `archfit doctor`.
4. No config? Generate and review before editing:
   `archfit init --root . --output .archfit.yaml`.
5. Prefer code fixes over exceptions; use expiring exceptions only for
   intentional temporary drift.
6. Baseline only accepted existing debt — never to make a new finding green.
7. Validate: `archfit analyze --gate --config .archfit.yaml --full` (add `--json`
   for agent loops, `--require-tools` only when missing-tool coverage should gate).

`analyze --llm`, `init`/`update --llm`, `enrich` (labels / `--subdomains` / `--owner` /
`--volatility`), `autopilot`, and `explain --llm` are all off-gate and
draft-first: detail and guardrails are in `references/llm-modes.md`. Never write
LLM classifications (`--apply`/`--pin`) or approve drafts without reviewing them
first. `autopilot` only ever writes a review file — it refuses to touch
`.archfit.yaml`.

## Agent repair loop

Fixing findings autonomously: run `archfit analyze --gate --json`; exit 0 means
done. Each `agent_tasks[]` entry has `goal`, `constraints`, `files`, and a
`validation` command — fix within the constraints, touch only the listed files
where possible, then re-run `validation` verbatim. Never "fix" `baseline` or
`waived` findings unprompted. Full contract: `references/agent-loop.md`.

## Coverage gaps and gate promotion

archfit never scores absence of evidence as healthy. A metric reading `n/a`, or a
`## Coverage gaps` / `## Required tools missing` section (`coverage_gaps[]` +
`config_warnings[]` in JSON), means an analyzer did not run — not a passing gate.
Read the gap's `affected_metrics` and `install_cmd`; close it by installing the
tool (`archfit doctor` lists them) or filling the config, not by ignoring it.

- Default is warn-loud (exit 0). To make CI block on a missing tool, promote with
  `--require-tools` or per-tool `languages.<x>.gate: fail` / `analyzers.<x>.gate: fail`
  (exits 1 — a policy decision, distinct from exit 3 errors).
- Promote rules to `gate: fail` only when high-confidence (cycles,
  forbidden-dependency, layer-direction); keep noisy ones at `warn`.
- Separate `tool missing`, `tool failed`, `tool disabled`, and `config
under-specified`. They have different fixes.
- If a gap cannot be closed now, say which metrics are unmeasured, lower
  confidence, and avoid treating the run as clean just because it stayed exit 0.
- Under-specified-module warnings usually clear once modules declare `owner` /
  `subdomain` / `volatility`. Draft them with `enrich --subdomains` / `--owner` /
  `--volatility` or `autopilot`, **review**, then pin — never auto-apply.
  Filling them also makes `encapsulation` measurable. A wiring/`cmd` package
  flagged for fan-out wants a `role:` (e.g. `composition_root`), not an exception.

## Review

Audit config, output, CI readiness, PR drift, baselines, exceptions, or coverage.

1. Inspect local evidence before judging: config, baseline, CI, package files,
   `doctor` / help output, and supplied findings output.
2. Compare config against the repo shape, active languages, and the references.
3. Separate true drift, accepted debt, tool gaps, stale config, and false
   positives.
4. Don't recommend baselining as the default fix.
5. For CI readiness, check that tools, adapter modes, and output format are
   deterministic.
6. If tools are missing or failed, downgrade confidence and call out the exact
   coverage risk.
7. Give a verdict and ordered next actions.

## Finding repair order

Capture each finding's ID, rule ID, status, from/to path, `why`, and
`constraint`. Then prefer, in order:

1. Remove the unnecessary dependency.
2. Use or add a public API.
3. Move code to the owning module.
4. Invert the dependency through an interface or port.
5. Add an expiring exception for intentional temporary drift.
6. Baseline reviewed existing debt.

Never add broad exclusions to hide findings.

## Review output

```markdown
## Archfit Review

Scope: <files, command output, or PR area>
Verdict: ready | needs changes | blocked
Confidence: high | medium | low
Sources: <references, local files, commands>

### Findings

1. `path` or finding `<id>` — <issue>. Evidence: <path:line | json.field | quoted output>. Fix: <action>.

### Working Well

- <good config or workflow to keep>

### Next Actions

1. <highest-impact action>
2. <next action>

### Verification

- <command run or exact skipped reason>
```

Omit empty sections. Say `No confirmed findings.` when nothing is actionable.

## Failure handling

- Command execution unavailable: stay doc-only, quote the missing command path or
  tool limitation, and lower confidence.
- `archfit` unavailable: use install docs; state command verification was skipped.
- Missing language tools: report coverage risk, expected install path, and the
  affected metrics.
- No architecture intent: ask for intended modules or layers before encoding rules.
- Repo-local generated artifacts already present: treat them as generated state;
  avoid mistaking them for source or letting them contaminate scans.
- Noisy generated config: narrow modules and rules before adding exceptions.
- Conflicting output and config: quote the exact finding, JSON field, or config
  line and lower confidence until reproduced.
- `--apply` fails or `config.Load` rejects the result: the original file is intact;
  report the error and stay in plan mode.
