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

## Grounding

Use the built binary and its live help output as the source of truth. Ground
claims in current tool output, not memory. If a reference disagrees with
`archfit --help`, `archfit doctor`, or the current command output, trust the
binary, note the stale reference, and update the docs. When command execution is
unavailable, say so and lower confidence.

**Do not use** for generic software-architecture advice unrelated to `archfit`,
or to decide whether to adopt `archfit` — use web research for that. Do not use
this skill to evaluate archfit itself against the book-alignment workflow or the
test corpus — use `skills/archfit-eval/`.

archfit reports one **architecture state**: a verdict (`healthy` /
`needs_attention` / `blocked`) over nine dimension envelopes — `intent`,
`structure`, `modularity`, `coupling`, `change_locality`, `complexity`,
`testability`, `operations`, `drift` — each stating its own status, gate,
confidence, denominator, and what it could not measure. **There is no repository
score.** Coupling additionally reports a **seam ledger**: one record per ordered
module pair, with a stable ID, a score distribution, and a balancing hypothesis.
The state also carries the measurement profile behind the run. A persisted
baseline is exposed as `gate_reference`; an explicit `--base` comparison is
reported separately and does not replace the gate reference.

Underneath it measures **Balanced Coupling** (`coupling_balance` band, Khononov
S×D×V formula) plus structural architecture rules (forbidden deps, layering,
cycles, public API) and complementary metrics. `unbalanced_edge`, `cycle`,
`encapsulation`, and `coverage` can gate on baseline regressions;
`coupling_balance` and absolute metric values never gate. Code-quality concerns
(complexity, duplication, panic/unsafe/god-struct density) are delegated to
linters by design.

## Routing

- Stay in this skill for deterministic `archfit` setup, config review, CI wiring,
  finding interpretation, baseline/exception decisions, and constrained repair.
- Use `skills/archfit-eval/` when the job is to evaluate archfit itself:
  book-alignment coverage, corpus sweeps, semantic comparison, docs/UX, or skill
  maintenance.
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
- `references/llm-modes.md` — `analyze --ai-summary`, `config init` /
  `config update --ai-classify`, `config enrich` (labels, abstained,
  subdomain, owner, volatility), `.env`, and `explain --ai-summary`.
- `references/agent-loop.md` — autonomous repair contract (`agent_tasks`, SARIF,
  `blast_radius`), and how coverage gaps read in the loop.
- `references/migration.md` — exact manual config-v1-to-v2 transform and the
  review-and-regenerate procedure for baseline v1.

`archfit --help` and `archfit <cmd> --help` confirm flags. When a reference and
the binary disagree, trust the binary, say which reference was stale, and update
it if this repo owns it. Full guide:
<https://github.com/alexei-led/archfit/blob/main/docs/guide/README.md>.

If shell or command execution is unavailable, use local docs and references only,
state that verification was skipped, and lower confidence.

## Safe defaults

- Inspect existing config, baseline, CI, and package files before proposing edits.
- Prefer non-failing commands first: `archfit --help`, `archfit doctor`, and
  report-only `archfit analyze` (always exits 0 on success; add `--format
scorecard` or `--markdown` for those views). Use `archfit check` only when
  you want CI/agent-loop exit codes.
- Prefer stdout or a temp path for reports and SARIF during review. Treat
  `.archfit-cache/`, `.archfit-*.yaml`, `archfit.sarif`, and Markdown reports as
  generated artifacts; do not leave them in the repo unless the task calls for it.
- Keep early config narrow: modules, layers, public APIs, and high-value rules.

## Write or fix

Install, configure, add CI, baseline, add an exception, or fix findings.

1. Inspect first: `.archfit.yaml`, `.archfit-baseline.json`, CI workflows,
   language package files, and existing generated `archfit` artifacts.
   **Check the config schema version before editing.** Only the current schema
   version is accepted. For config v1 or baseline v1, read
   `references/migration.md` before changing the file; do not change only the
   schema-version string.
2. Detect the repo languages and read `references/languages.md` for each one in
   scope before suggesting tool setup or config globs.
3. Check the surface: `archfit --help`, `archfit doctor`.
4. No config? Generate and review before editing:
   `archfit config init --root . --output .archfit.yaml`.
5. Prefer code fixes over exceptions; use expiring exceptions only for
   intentional temporary drift.
6. Baseline only accepted existing debt — never to make a new finding green.
7. Validate: `archfit check --config .archfit.yaml` (add `--json` for agent
   loops, `--require-tools` only when missing-tool coverage should gate).

`analyze --ai-summary`, `config init` / `config update --ai-classify`, `config
enrich` (labels / abstained / `subdomain` / `owner` / `volatility`), and
`explain --ai-summary` are all off-gate and draft-first: detail and guardrails
are in `references/llm-modes.md`. Never approve or apply AI classifications or
drafts without reviewing them first. `config init --ai-classify` without
`--apply` is review-only (writes commented-inert suggestions; use `-o` to
redirect to a draft file instead of `.archfit.yaml`).

## Agent loop: ask, check, hook

**1. Ask before an edit.** Before a new import across modules, run
`archfit policy can-import <file> <target>`. It runs the rule pass `check` runs,
under the same baseline and waivers, on that one import, in about 50 ms.
`denied` (exit 1) names the finding ID, the rule, and the repair: do not add
the import. `not_decided` (exit 2) means a cycle rule, the seam gate, or a
missing analyzer can still block it: add it, then check. `allowed` names the
allowlist or layer order that permits it. `unconstrained` is not permission.
`archfit policy where <path>` names the owning module, its layer, allowlists,
and the rules that select the path.

**2. Check before you finish.** Run `archfit check --format agent`. It prints
`archfit.agent-result.v1`: the verdict, ONE `next_action`, and the repairs
grouped by edge, in at most 8 KB. Follow `next_action`:
`repair` — change the code within each repair's `goal` and `constraints`, edit
only the files in `edit` (never the target of a forbidden edge), then run
`validate`; `ask_owner` — stop and report to the architecture owner;
`restore_evidence` — install or fix the analyzer the result names;
`report_blocked` — report the blockers, they are not from this change;
`none` — done, even with exit 2. With `--base HEAD`, a repair whose findings
all exist at HEAD is outside the scope of your change. Never edit the config,
the baseline, waivers, or labels to pass, and never "fix" `baseline` or
`waived` findings unprompted. The full JSON (`--format json`, `agent_tasks[]`)
is in `references/agent-loop.md`.

**3. Hook it in.** `archfit hook claude` is a Claude Code `Stop` and
`SubagentStop` hook: on a dirty tree it runs the agent result against
`--base HEAD` and exits 2 with the repair on stderr, at most once per stop; it
fails open on an archfit error. `archfit hook git` is the pre-commit hook
(`.pre-commit-hooks.yaml`, id `archfit`). `archfit agents-md --write` keeps a
generated block of these rules in `AGENTS.md`; `--check` fails CI on drift.
`archfit skill install` installs this skill from the binary that runs it.

## Coverage gaps and gate promotion

archfit never scores absence of evidence as healthy. A metric reading `n/a`, a
`## Coverage gaps` (Markdown) or `## Required tools missing` (scorecard)
section, or a `coverage.tools[]` row that is not `ok` in the primary JSON means
an analyzer did not run — not a passing gate. The JSON row carries only `tool`,
`status`, and `reason`; the Markdown and scorecard sections also list the
affected metrics and the install command. Close the gap by installing the tool
(`archfit doctor` lists them) or filling the config, not by ignoring it.

- By default a missing tool is reported without setting `hard_gates` to `fail`.
  Incomplete evidence can still make `check` return exit `2`. To make CI block
  on a missing tool, promote with `archfit check --require-tools` or per-tool
  `languages.<x>.gate: fail` / `analyzers.<x>.gate: fail` (exits 1 — a policy
  decision, distinct from exit 3 errors).
- Promote rules to `gate: fail` only when high-confidence (cycles,
  forbidden-dependency, layer-direction); keep noisy ones at `warn`.
- Separate `tool missing`, `tool failed`, `tool disabled`, and `config
under-specified`. They have different fixes.
- If a gap cannot be closed now, say which metrics are unmeasured, lower
  confidence, and avoid treating the run as clean just because report-only
  `analyze` stayed exit 0.
- If `decision.unevaluated_required_rules` is non-empty, the named fail-gated
  rules lack enough producer evidence. Resolve those structured reasons; do not
  infer a pass from an empty finding list or report prose.
- Under-specified-module warnings usually clear once modules declare `owner` /
  `subdomain` / `volatility`. Draft them with `config enrich subdomain` /
  `config enrich owner` / `config enrich volatility` or `config init
--ai-classify`, **review**, then `--apply` or copy deliberately — never
  auto-apply.
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
2. Use or add a public API only when the active rule permits that alternative.
3. Move code to the owning module.
4. Invert the dependency through an interface or port.
5. Add an expiring exception for intentional temporary drift.
6. Baseline reviewed existing debt only after owner approval; baseline capture
   skips findings covered by temporary waivers, including expired waivers.

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
