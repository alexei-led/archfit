---
name: archfit-writing
description: Create or update archfit setup. Use when installing archfit, generating or editing .archfit.yaml, adding Go, Python, or TypeScript architecture checks, adding CI commands, creating baselines or exceptions, or fixing archfit findings.
license: Apache-2.0
compatibility: Agent Skills compatible. Written for Claude Code, Codex, Pi, and other coding agents with file, shell, and edit capabilities.
---

# Archfit Writing

Use this skill to add, configure, run, or fix `archfit` in a repository.

This skill is intentionally thin. Canonical behavior lives in the project docs,
not here.

## Source of truth

Before editing `.archfit.yaml`, tool setup, language docs, or CI examples, read
only the relevant canonical doc links in `../references/archfit-docs.md`.

If the local repository is available, prefer local files over public links because
they may include unmerged changes.

If docs and implementation conflict, verify with the local binary or source. Do
not expand this skill by copying reference docs into it.

## Workflow

1. Identify the target: install, config, language setup, CI, baseline,
   exception, or code repair.
2. Inspect existing files first:
   - `.archfit.yaml`;
   - `.archfit-baseline.json`;
   - CI workflow files;
   - `go.mod`, `package.json`, `pyproject.toml`, or `setup.py`;
   - existing docs linked from `../references/archfit-docs.md`.
3. Check the local command surface when possible:

   ```sh
   archfit --help
   archfit doctor
   ```

4. If no config exists, generate a starter config, then review it before editing:

   ```sh
   archfit init --root . --output .archfit.yaml
   ```

5. Keep the first config narrow. Define modules, layers, public APIs, and only
   the highest-value rules.
6. Run the closest check after edits:

   ```sh
   archfit check --config .archfit.yaml --full
   ```

7. Use JSON when scripts or agents need structured findings:

   ```sh
   archfit check --config .archfit.yaml --full --format json
   ```

8. Baseline only accepted existing debt. Do not baseline new findings just to
   make a check green.
9. Prefer code fixes over exceptions. Use expiring exceptions only for intentional
   temporary violations.
10. Update canonical docs when the user-facing workflow changes.

## Writing rules

- Make the smallest useful change.
- Treat generated `archfit init` output as a draft, not truth.
- Prefer explicit module globs over broad catch-all globs.
- Keep module names and rule IDs stable.
- Use repo-relative paths unless canonical docs or implementation require a
  language-specific form.
- Do not invent config keys. Verify against canonical docs, the local binary, or
  source.
- Do not duplicate long reference material in skills. Link to canonical docs.

## Fixing findings

For each finding, capture:

- finding ID;
- rule ID;
- status;
- from path and module;
- to path and module;
- why and constraint.

Fix choice order:

1. Remove unnecessary dependency.
2. Use or add a public API.
3. Move code to the owning module.
4. Invert dependency through an interface or port.
5. Add an expiring exception for intentional temporary drift.
6. Baseline reviewed existing debt.

Do not add broad exclusions to hide findings.

## Done criteria

A writing task is done only when:

- changed files are listed;
- the relevant `archfit` command was run, or the skip reason is stated;
- remaining findings are explained as fixed, baselined, excepted, or out of
  scope;
- docs or CI examples are updated when the user-facing workflow changed.

## Failure handling

- Missing `archfit`: show install options from canonical docs and stop before
  editing CI.
- Missing language tools: report the tool gap and set language modes
  intentionally.
- Noisy generated config: narrow modules and rules before adding exceptions.
- Ambiguous architecture intent: ask for intended boundaries before encoding a
  rule.
- A failing check after edits: quote the finding ID, rule, from path, and to
  path; diagnose before changing more files.
