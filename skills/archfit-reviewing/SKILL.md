---
name: archfit-reviewing
description: Review archfit configuration, output, CI integration, baselines, exceptions, or architecture findings. Use when auditing .archfit.yaml, deciding whether findings are safe to baseline, reviewing PR architecture drift, or checking Go, Python, and TypeScript adapter coverage.
license: Apache-2.0
compatibility: Agent Skills compatible. Written for Claude Code, Codex, Pi, and other coding agents with file, shell, and edit capabilities.
---

# Archfit Reviewing

Use this skill to review whether `archfit` is configured well, whether findings
are actionable, and whether a repo is ready to use `archfit` as a gate.

This skill is intentionally thin. Canonical behavior lives in the project docs,
not here.

## Source of truth

Before making claims about config fields, language setup, commands, output, or CI,
read only the relevant canonical doc links in `../references/archfit-docs.md`.

If the local repository is available, prefer local files over public links because
they may include unmerged changes.

If docs and implementation conflict, verify with the local binary or source. Do
not expand this skill by copying reference docs into it.

## Workflow

1. Identify the review target: config, output, CI, PR diff, baseline,
   exceptions, or language coverage.
2. Inspect local evidence first:
   - `.archfit.yaml`;
   - `.archfit-baseline.json`;
   - CI workflow files;
   - package files such as `go.mod`, `package.json`, or `pyproject.toml`;
   - existing `archfit` output if supplied.
3. When allowed and practical, run:

   ```sh
   archfit doctor
   archfit check --config .archfit.yaml --full --format json
   ```

4. Compare the config with the repo shape and canonical docs.
5. Review findings by evidence. Separate true drift, accepted debt, tool gaps,
   stale config, and false positives.
6. Do not recommend baselining as a default fix. Baseline only reviewed existing
   debt.
7. If reviewing CI readiness, check whether missing tools, adapter modes, and
   output format make the result deterministic.
8. Give a verdict and ordered actions.

## Review priorities

Flag these first:

- missing or wrong language adapter setup;
- broad module globs that hide ownership problems;
- public or internal API globs that do not match actual import targets;
- layer order that contradicts intended dependency direction;
- exceptions without reason, approver, or expiry;
- baselines that appear to hide current unreviewed findings;
- CI commands that cannot find tools or config;
- generated config used unreviewed as policy;
- docs or skills that duplicate canonical reference content and may drift.

## Output

Use this structure:

```markdown
## Archfit Review

Scope: <files, command output, or PR area>
Verdict: ready | needs changes | blocked
Confidence: high | medium | low
Sources: <canonical docs, local files, commands>

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

Omit empty sections. Use `No confirmed findings.` when nothing actionable is
found.

## Failure handling

- Missing config: review install/init readiness instead of guessing policy.
- Missing `archfit`: review files only and report that command verification was
  skipped.
- Missing language tools: report coverage risk and expected canonical install
  path.
- No architecture intent: do not invent boundaries; ask for intended modules or
  layers.
- Conflicting output and config: quote the exact finding or config line and mark
  confidence medium or low until reproduced.
