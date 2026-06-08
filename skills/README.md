# Archfit Agent Skills

Portable Agent Skills for using `archfit` from coding agents.

These skills follow the `SKILL.md` directory format: each skill is a directory
with a `SKILL.md` entrypoint. They avoid harness-specific tool names so they can
be used from Claude Code, Codex, Pi, or other agents that support Agent Skills.

The skills are intentionally thin. Shared reference links live in
[`references/archfit-docs.md`](references/archfit-docs.md), which points to the
canonical public docs and local fallback paths. Do not copy full docs into the
skills.

## Skills

- [`archfit-writing`](archfit-writing/SKILL.md) — install, configure, run, and
  fix with `archfit`.
- [`archfit-reviewing`](archfit-reviewing/SKILL.md) — review `.archfit.yaml`,
  results, CI use, baselines, exceptions, and language coverage.

## Use

Point your agent harness at this directory, or copy or symlink individual skill
folders into the harness-specific skills directory.

Examples:

```sh
# Pi explicit loading
pi --skill ./skills/archfit-writing --skill ./skills/archfit-reviewing

# Generic copy pattern for harnesses with project skill folders
mkdir -p .agent/skills
cp -R skills/archfit-writing skills/archfit-reviewing skills/references .agent/skills/
```

Adjust the target path for the specific harness. Keep this source copy as the
maintained version. If a harness copies one skill folder without `references/`,
use the public links in `skills/references/archfit-docs.md` from this repo.
