---
name: archfit
description: Use archfit for architecture fitness work. Use when installing archfit, creating or reviewing .archfit.yaml, configuring Go, Python, or TypeScript checks, adding CI usage, interpreting findings, fixing architecture drift, or deciding on baselines and exceptions. Not for generic software-architecture advice or deciding whether to adopt archfit.
license: Apache-2.0
compatibility: Requires access to the repository files. Shell access is recommended for archfit, git, and language tool checks.
metadata:
  author: alexei-led
  tags:
    - architecture
    - fitness
    - go
    - python
    - typescript
    - coupling
---

# Archfit

Thin skill for `archfit` setup, configuration, review, and finding repair.
Detailed behavior lives in `references/`, loaded on demand.

**Do not use** for generic software-architecture advice unrelated to `archfit`,
or to decide whether to adopt `archfit` — those are web research questions.

## References

Read the one the task needs:

- `references/commands.md` — commands, flags, output formats, finding statuses,
  exit codes.
- `references/llm-modes.md` — `init`/`update --llm` plan→apply, `enrich`,
  `explain --llm`.
- `references/agent-loop.md` — autonomous repair contract (`agent_tasks`, SARIF,
  `change_locality`).

`archfit --help` confirms flags. When a reference and the binary disagree, verify
against the binary and say so. Full guide:
<https://github.com/alexei-led/archfit/blob/main/docs/guide/README.md>.

## Write or fix

Install, configure, add CI, baseline, add an exception, or fix findings.

1. Inspect first: `.archfit.yaml`, `.archfit-baseline.json`, CI workflows,
   language package files.
2. Check the surface: `archfit --help`, `archfit doctor`.
3. No config? Generate and review before editing:
   `archfit init --root . --output .archfit.yaml`.
4. Keep early config narrow: modules, layers, public APIs, high-value rules.
5. Prefer code fixes over exceptions; use expiring exceptions only for
   intentional temporary drift.
6. Baseline only accepted existing debt — never to make a new finding green.
7. Validate: `archfit check --config .archfit.yaml --full` (add `--format json`
   for agent loops).

`init`/`update --llm` use a plan→apply safety model: detail and guardrails are in
`references/llm-modes.md`. Never write LLM classifications (`--apply`) or approve
`enrich` drafts without reviewing them first.

## Agent repair loop

Fixing findings autonomously: run `archfit check --format json`; exit 0 means
done. Each `agent_tasks[]` entry has `goal`, `constraints`, `files`, and a
`validation` command — fix within the constraints, touch only the listed files,
then re-run `validation` verbatim. Never "fix" `baseline` or `excepted` findings
unprompted. Full contract: `references/agent-loop.md`.

## Review

Audit config, output, CI readiness, PR drift, baselines, exceptions, or coverage.

1. Inspect local evidence before judging: config, baseline, CI, package files,
   supplied output.
2. Compare config against the repo shape and the references.
3. Separate true drift, accepted debt, tool gaps, stale config, and false
   positives.
4. Don't recommend baselining as the default fix.
5. For CI readiness, check that tools, adapter modes, and output format are
   deterministic.
6. Give a verdict and ordered next actions.

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

1. `path` or finding `<id>` — <issue>. Evidence: <fact>. Fix: <action>.

### Working Well

- <good config or workflow to keep>

### Next Actions

1. <highest-impact action>
2. <next action>

### Verification

- <command run or skipped reason>
```

Omit empty sections. Say `No confirmed findings.` when nothing is actionable.

## Failure handling

- `archfit` unavailable: use install docs; state command verification was skipped.
- Missing language tools: report coverage risk and the expected install path.
- No architecture intent: ask for intended modules or layers before encoding rules.
- Noisy generated config: narrow modules and rules before adding exceptions.
- Conflicting output and config: quote the exact finding or config line and lower
  confidence until reproduced.
- `--apply` fails or `config.Load` rejects the result: the original file is intact;
  report the error and stay in plan mode.
