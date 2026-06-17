---
name: archfit
description: Use archfit for architecture fitness work. Use when installing archfit, creating or reviewing .archfit.yaml, configuring Go, Python, or TypeScript checks, adding CI usage, interpreting findings, fixing architecture drift, or deciding on baselines and exceptions.
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

Use this skill for `archfit` setup, configuration, review, and finding repair.

This skill is intentionally thin. Canonical behavior lives in the project docs,
not here.

**Do not use when** the question is about generic software-architecture advice
unrelated to the `archfit` tool, or when deciding whether to adopt `archfit` in
the first place — those are web research questions, not archfit skill questions.

## Source of truth

Read `references/archfit-docs.md` only when you need links to canonical docs.
Then read the smallest relevant doc page.

Prefer local docs when working inside the `archfit` repository. Use public links
when working elsewhere. When working **outside** the archfit source repository,
use the public links in `references/archfit-docs.md` exclusively; do not assume
local paths exist.

If docs and implementation conflict, verify with the local binary or source. Do
not copy full guide or config reference content into this skill.

## Modes

### Write or fix

Use when the user asks to install, configure, add CI, create a baseline, add an
exception, or fix findings.

1. Inspect existing files first: `.archfit.yaml`, `.archfit-baseline.json`, CI
   workflows, and language package files.
2. Check the local command surface when practical:

   ```sh
   archfit --help
   archfit doctor
   ```

3. If no config exists, generate a draft and review it before editing:

   ```sh
   archfit init --root . --output .archfit.yaml
   ```

4. Keep early config narrow: modules, layers, public APIs, and high-value rules.
5. Prefer code fixes over exceptions. Use expiring exceptions only for intentional
   temporary drift.
6. Baseline only accepted existing debt. Do not baseline new findings just to
   make a check green.
7. Validate with the closest check:

   ```sh
   archfit check --config .archfit.yaml --full
   ```

8. Use JSON for scripts or agent repair loops:

   ```sh
   archfit check --config .archfit.yaml --full --format json
   ```

### Init and update LLM modes

`archfit init` and `archfit update` support optional LLM classification and a
two-step plan→apply safety model. Default modes are advisory (inert or
report-only); `--apply` is the only path that writes LLM judgment into
gate-feeding fields.

**Mode matrix:**

| Command                | Effect                                                            |
| ---------------------- | ----------------------------------------------------------------- |
| `init`                 | structural scaffold (unchanged)                                   |
| `init --llm`           | + commented-inert subdomain/volatility/layer suggestions          |
| `init --llm --apply`   | + subdomain/volatility/layer written live (name stays a comment)  |
| `update`               | drift report, writes nothing                                      |
| `update --apply`       | structural drift written live (add/path/comment-removed modules)  |
| `update --llm`         | drift report including LLM classification of unclassified modules |
| `update --llm --apply` | structural + LLM classification written live                      |

`--apply` without `--llm` is valid only for `update`; `init --apply` requires
`--llm`. Coupling rules always stay with `enrich` — `init`/`update` handle
classification only (`subdomain`, `volatility`, `layer`).

**Guardrails:**

- A live `layer` is written only when it already appears in the config's
  `layers:` list; an out-of-set suggestion is rendered as a comment instead.
- Module keys are never auto-renamed; a name suggestion is always a comment.
- `update --apply` replaces module paths with discovered paths and writes a
  timestamped backup before overwriting.
- Existing fields (`subdomain`, `volatility`, `layer`) are never overwritten;
  `SetModuleFields` is skipped for any field already present.
- Plan mode (`update` without `--apply`) leaves `.archfit.yaml` untouched.
- `--apply` completes the full LLM pass before any file write, validates the
  result with `config.Load`, and uses an atomic temp+rename.

See `docs/guide/commands.md` for flag reference and `docs/guide/llm-enrich.md`
for the `enrich` workflow that governs coupling rules.

### Agent repair loop

Use when fixing findings autonomously. The JSON output carries an
`agent_tasks[]` block — one entry per ACTIVE gate finding with `goal`,
`constraints`, `files`, and the exact `validation` command:

1. Run `archfit check --format json`; exit 0 means done.
2. For each `agent_tasks[]` entry: fix within the stated constraints,
   touching only the listed files where possible.
3. Re-run the entry's `validation` command verbatim.
4. Never "fix" findings with status `baseline` or `excepted` unprompted —
   they are accepted state, not errors.
5. For CI annotation surfaces, use `--format sarif` (SARIF 2.1.0).
6. In delta mode (`--base <ref>`), watch the `change_locality` metric for
   the change's blast surface.

See `docs/guide/agent-feedback.md` for the full loop contract.

### Enrich (off-gate LLM labels)

Use when the user asks to refine coupling-strength labels or run enrichment.
Requires `tools.llm` (provider + model) and `tools.scip.enabled: "on"`.

1. `archfit enrich` drafts refinements into `.archfit-labels.yaml`
   (`status: draft` — inert).
2. A HUMAN reviews each draft: flip `status: approved` to pin, delete to
   reject. Never auto-approve drafts.
3. `check` consumes approved labels only and stays LLM-free; a
   `labels/stale` advisory means the code changed since approval —
   re-run enrich and re-review.
4. `archfit explain <id> --llm` appends an off-gate narrative to one finding.

See `docs/guide/llm-enrich.md`.

### Review

Use when the user asks to audit config, output, CI readiness, PR drift,
baselines, exceptions, or language coverage.

1. Inspect local evidence before judging: config, baseline, CI, package files, and
   supplied `archfit` output.
2. Compare the config with the repo shape and the relevant canonical docs.
3. Separate true drift, accepted debt, tool gaps, stale config, and false
   positives.
4. Do not recommend baselining as the default fix.
5. If reviewing CI readiness, check whether tools, adapter modes, and output
   format are deterministic.
6. Give a verdict and ordered next actions.

## Finding repair order

For each finding, capture the ID, rule ID, status, from path, to path, `why`, and
`constraint`. Then prefer fixes in this order:

1. Remove the unnecessary dependency.
2. Use or add a public API.
3. Move code to the owning module.
4. Invert dependency through an interface or port.
5. Add an expiring exception for intentional temporary drift.
6. Baseline reviewed existing debt.

Do not add broad exclusions to hide findings.

## Output for reviews

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

Omit empty sections. Say `No confirmed findings.` when nothing actionable is
found.

## Failure handling

- Missing `archfit`: use install docs and state command verification was skipped.
- Missing language tools: report coverage risk and expected install path.
- No architecture intent: ask for intended modules or layers before encoding
  rules.
- Noisy generated config: narrow modules and rules before adding exceptions.
- Conflicting output and config: quote the exact finding or config line and lower
  confidence until reproduced.
