# Archfit Guide

Short, task-focused docs for using `archfit`.

Start with quick start if you want the first useful run. Use the other pages as
small references while configuring a repo.

## Pages

- [Overview](overview.md) — what `archfit` is and when to use it.
- [Why architecture fitness matters](why-architecture-fitness.md) — architecture
  erosion, AI-agent risk, and why this is more than dependency lint.
- [Concepts](concepts.md) — Balanced Coupling, modularity, and the theory
  archfit makes executable.
- [Metrics reference](metrics.md) — every metric, what it represents, and how it
  is scored.
- [Install](install.md) — binary install, Docker, and first analyzer checks.
- [Tooling reference](tooling.md) — platform setup, package managers, tool
  versions, home pages, and PATH checks.
- [Quick start](quick-start.md) — first run workflow.
- [Language support](languages.md) — supported languages and setup.
- [Configuration basics](configuration.md) — `.archfit.yaml` shape.
- [Configuration reference](configuration-reference.md) — fields and examples.
- [Dogfooding](dogfooding.md) — how archfit runs on itself; signals vs.
  violations.
- [Commands](commands.md) — common commands, formats, and exit codes.
- [Caching](caching.md) — the extractor fact cache: what invalidates it,
  `--no-cache`, eviction, reset.
- [CI](ci.md) — basic CI and pull-request usage.
- [Agent feedback loop](agent-feedback.md) — the AI-agent loop: `agent_tasks`,
  SARIF, `--base` delta mode.
- [LLM enrichment](llm-enrich.md) — off-gate LLM enrichment: `enrich`, pinned
  labels, `explain --llm`.
- [Troubleshooting](troubleshooting.md) — common setup and config issues.

## Related docs

- [Contributing](../../CONTRIBUTING.md) — local development and release process.

Add future long-form reference pages only when the project needs more detail.
Keep this guide focused on the first successful run.
