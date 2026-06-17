# archfit

[![CI](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml/badge.svg)](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml)
[![Release](https://github.com/alexei-led/archfit/actions/workflows/release.yaml/badge.svg)](https://github.com/alexei-led/archfit/actions/workflows/release.yaml)
[![Version](https://img.shields.io/github/v/tag/alexei-led/archfit?label=version&sort=semver)](https://github.com/alexei-led/archfit/tags)
[![GHCR](https://ghcr-badge.egpl.dev/alexei-led/archfit/latest_tag?label=ghcr.io&color=%230db7ed)](https://github.com/alexei-led/archfit/pkgs/container/archfit)
[![Image Size](https://ghcr-badge.egpl.dev/alexei-led/archfit/size?tag=latest&label=image%20size&color=%230db7ed)](https://github.com/alexei-led/archfit/pkgs/container/archfit)
[![Go Reference](https://pkg.go.dev/badge/github.com/alexei-led/archfit.svg)](https://pkg.go.dev/github.com/alexei-led/archfit)
[![Go Report Card](https://goreportcard.com/badge/github.com/alexei-led/archfit)](https://goreportcard.com/report/github.com/alexei-led/archfit)
[![License](https://img.shields.io/github/license/alexei-led/archfit)](LICENSE)

Architecture fitness checker for Go, TypeScript, and Python repositories.

`archfit` makes selected architecture rules executable. It reads dependency facts
from a repository, compares them with `.archfit.yaml`, and reports findings that
humans, CI, or AI coding agents can act on.

Use it when you want fast feedback on architecture drift: boundary leaks, layer
violations, import cycles, new cross-module dependencies, or coupling risk.

## What you get

- Deterministic gates for dependencies, public APIs, layers, cycles, and
  configured thresholds.
- Architecture metrics for modularity, coupling risk, coverage, change locality,
  and other drift signals.
- Baselines so accepted current debt does not hide new findings.
- SARIF, Markdown, JSON, and text output for local work, CI, and code scanning.
- Structured `agent_tasks` repair blocks for AI-agent feedback loops.
- Optional LLM enrichment for coupling-label drafts, kept off the CI gate.

## Quick start

Install the CLI:

```sh
go install github.com/alexei-led/archfit/cmd/archfit@v0.1.0
```

Or run the Docker image:

```sh
docker run --rm -v "$(pwd):/repo" ghcr.io/alexei-led/archfit:latest \
  check --config /repo/.archfit.yaml --full
```

Run the first check in a repository:

```sh
archfit doctor
archfit init --root . --output .archfit.yaml
$EDITOR .archfit.yaml
archfit check --config .archfit.yaml --full
```

Accept current findings as a baseline when they represent known debt:

```sh
archfit baseline --full --config .archfit.yaml
```

Need Docker, CI, optional analyzers, or language-specific setup? Start with the
[guide](docs/guide/README.md).

## Core commands

| Command                | Use                                            |
| ---------------------- | ---------------------------------------------- |
| `archfit doctor`       | Check local analyzer/tool availability.        |
| `archfit init`         | Generate a starter `.archfit.yaml`.            |
| `archfit update`       | Sync `.archfit.yaml` with current structure.   |
| `archfit check`        | Run architecture gates and metrics.            |
| `archfit scan`         | Produce a full Markdown audit report.          |
| `archfit baseline`     | Save accepted current findings.                |
| `archfit enrich`       | Draft off-gate LLM coupling-label refinements. |
| `archfit explain <id>` | Explain one finding by fingerprint prefix.     |
| `archfit install`      | Check or install optional language tools.      |

See the [commands guide](docs/guide/commands.md) for formats, exit codes, and
examples.

## Model

`archfit` operationalizes the parts of Vlad Khononov's Balanced Coupling model
that can be checked from code, configuration, and git history. It uses the
model's strength, distance, and volatility vocabulary, plus explicitness, to
explain why a relationship is balanced or risky.

It does not replace architecture review. It makes repeatable evidence cheap to
collect and safe to run in CI. For the books, posts, and mapping from theory to
signals, see [Concepts](docs/guide/concepts.md) and
[Metrics](docs/guide/metrics.md).

## Documentation

- [Guide](docs/guide/README.md) — documentation map.
- [Install](docs/guide/install.md) — Go install, Docker, and optional tools.
- [Quick start](docs/guide/quick-start.md) — first useful run.
- [Configuration](docs/guide/configuration.md) — `.archfit.yaml` basics.
- [CI](docs/guide/ci.md) — pull-request and pipeline setup.
- [Agent feedback](docs/guide/agent-feedback.md) — `agent_tasks`, SARIF, and
  `change_locality`.
- [LLM enrichment](docs/guide/llm-enrich.md) — human-reviewed label drafts.
- [Contributing](CONTRIBUTING.md) — local development and release notes.

## License

Apache-2.0. See [LICENSE](LICENSE).
