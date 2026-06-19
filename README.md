# archfit

[![CI](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml/badge.svg)](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml)
[![Release](https://github.com/alexei-led/archfit/actions/workflows/release.yaml/badge.svg)](https://github.com/alexei-led/archfit/actions/workflows/release.yaml)
[![Version](https://img.shields.io/github/v/tag/alexei-led/archfit?label=version&sort=semver)](https://github.com/alexei-led/archfit/tags)
[![GHCR](https://ghcr-badge.egpl.dev/alexei-led/archfit/latest_tag?label=ghcr.io&color=%230db7ed)](https://github.com/alexei-led/archfit/pkgs/container/archfit)
[![Image Size](https://ghcr-badge.egpl.dev/alexei-led/archfit/size?tag=latest&label=image%20size&color=%230db7ed)](https://github.com/alexei-led/archfit/pkgs/container/archfit)
[![Go Reference](https://pkg.go.dev/badge/github.com/alexei-led/archfit.svg)](https://pkg.go.dev/github.com/alexei-led/archfit)
[![Go Report Card](https://goreportcard.com/badge/github.com/alexei-led/archfit)](https://goreportcard.com/report/github.com/alexei-led/archfit)
[![License](https://img.shields.io/github/license/alexei-led/archfit)](LICENSE)

Architecture-fitness CLI that gives AI coding agents and CI a deterministic,
machine-readable view of architecture drift across Go, TypeScript, and Python.

## Why archfit

An AI agent editing your repo cannot see your architecture. It sees files. So it
adds an import from the domain layer into the HTTP adapter, breaks a module
boundary, or grows a cycle — and the diff looks fine. `archfit` makes the
architecture a fact the agent (and your pipeline) can check.

**Without archfit** — the agent's loop has no architecture signal:

```text
agent edits files → tests pass → PR opened → boundary leak ships → reviewer
catches it days later (or doesn't)
```

**With archfit** — the same loop closes on structured, repairable feedback:

```text
agent edits files → archfit check --format sarif → boundary violation +
agent_tasks repair block → agent fixes the import → check is clean → PR opened
```

`archfit` reads dependency facts from the repository, compares them with
`.archfit.yaml`, and emits findings as SARIF, JSON, Markdown, or text — plus
`agent_tasks` blocks an agent can act on directly. Gates are deterministic:
byte-identical output for unchanged input, no LLM on the gate path.

## What you get

- **Deterministic gates** for forbidden dependencies, public-API boundaries,
  layer direction, import cycles, and configured thresholds.
- **`agent_tasks` repair blocks** so an AI agent gets the fix, not just the error.
- **A banded 7-dimension scorecard** (`archfit score`) aligned to an architecture
  rubric: boundary integrity, coupling balance, dependency-graph health, cohesion,
  change locality, architecture fitness, analysis confidence.
- **Balanced-Coupling advisories** — strength × distance × volatility, grouped
  into rollups instead of one line per edge.
- **Honest coverage** — when a tool is missing the dependent metric reports `n/a`
  with the enable step; runs never fail because an optional analyzer is absent.
- **Baselines** so accepted current debt does not hide new findings.
- **Off-gate LLM narration** (`archfit review --llm`, `archfit enrich`) that may
  only narrate and prioritize collected evidence — never invent gate violations.

## How it compares

`archfit` is not a replacement for language-specific boundary linters — it is the
multi-language, agent-facing layer above them.

| Tool               | Languages          | Output for agents/CI                 | Coupling model            | LLM narration |
| ------------------ | ------------------ | ------------------------------------ | ------------------------- | ------------- |
| **archfit**        | Go, TS/JS, Python  | SARIF + JSON + `agent_tasks` + score | Balanced Coupling (S/D/V) | off-gate      |
| dependency-cruiser | TS/JS              | JSON/HTML, rule violations           | rule-based                | no            |
| import-linter      | Python             | text, contract pass/fail             | layered contracts         | no            |
| ArchUnit           | Java/JVM (.NET/TS) | test assertions in the build         | rule-based                | no            |

Use a single-language linter when you live in one language and want fine-grained,
in-build assertions. Use `archfit` when you want one config and one
machine-readable verdict across a polyglot repo, scored and shaped for an agent
loop.

## Quick start

Install the CLI (use a release tag, not `@latest`, in repeatable docs):

```sh
go install github.com/alexei-led/archfit/cmd/archfit@v0.3.0
```

Or run the Docker image with the bundled toolchain:

```sh
docker run --rm -v "$(pwd):/repo" ghcr.io/alexei-led/archfit:v0.3.0 \
  check --config /repo/.archfit.yaml --full
```

Run the first check in a repository:

```sh
archfit doctor                                   # check available analyzers
archfit init --root . --output .archfit.yaml     # generate a starter config
$EDITOR .archfit.yaml
archfit check --config .archfit.yaml --full      # run the gates
archfit score --config .archfit.yaml --full      # banded scorecard
```

Accept current findings as a baseline when they represent known debt:

```sh
archfit baseline --full --config .archfit.yaml
```

Starter configs for common project shapes live in
[`examples/`](examples/README.md). Need Docker, CI, optional analyzers, or
language-specific setup? Start with the [guide](docs/guide/README.md).

## Core commands

| Command                | Use                                            |
| ---------------------- | ---------------------------------------------- |
| `archfit doctor`       | Check local analyzer/tool availability.        |
| `archfit init`         | Generate a starter `.archfit.yaml`.            |
| `archfit update`       | Sync `.archfit.yaml` with current structure.   |
| `archfit check`        | Run architecture gates and metrics.            |
| `archfit score`        | Emit the banded 7-dimension scorecard.         |
| `archfit scan`         | Produce a full Markdown audit report.          |
| `archfit baseline`     | Save accepted current findings.                |
| `archfit review`       | Off-gate LLM narrative review of the evidence. |
| `archfit enrich`       | Draft off-gate LLM coupling-label refinements. |
| `archfit explain <id>` | Explain one finding by fingerprint prefix.     |
| `archfit install`      | Check or install optional language tools.      |

See the [commands guide](docs/guide/commands.md) for formats, exit codes, and
examples.

## Model

`archfit` operationalizes the parts of Vlad Khononov's Balanced Coupling model
that can be checked from code, configuration, and git history — integration
strength, socio-technical distance, and volatility — to explain why a
relationship is balanced (cohesion, the good coupling) or risky (cascading
changes, distributed-monolith risk).

It does not replace architecture review. It makes repeatable evidence cheap to
collect and safe to run in CI. For the mapping from theory to signals, see
[Concepts](docs/guide/concepts.md) and [Metrics](docs/guide/metrics.md).

## Documentation

- [Guide](docs/guide/README.md) — documentation map.
- [Install](docs/guide/install.md) — Go install, Docker, and optional tools.
- [Quick start](docs/guide/quick-start.md) — first useful run.
- [Language support](docs/guide/languages.md) — Go, TS, Python setup and optional
  analyzers.
- [Configuration](docs/guide/configuration.md) — `.archfit.yaml` basics.
- [Dogfooding](docs/guide/dogfooding.md) — how archfit runs on itself; signals
  vs. violations.
- [CI](docs/guide/ci.md) — pull-request and pipeline setup.
- [Agent feedback](docs/guide/agent-feedback.md) — `agent_tasks`, SARIF, and
  `change_locality`.
- [LLM enrichment](docs/guide/llm-enrich.md) — human-reviewed label drafts.
- [Examples](examples/README.md) — starter `.archfit.yaml` templates.
- [Contributing](CONTRIBUTING.md) — local development and release notes.

## License

Apache-2.0. See [LICENSE](LICENSE).
