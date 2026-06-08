# archfit

[![CI](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml/badge.svg)](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml)

Architecture fitness checker for Go, TypeScript, and Python repositories.

## Installation

```sh
go install github.com/alexei-led/archfit/cmd/archfit@latest
```

## Usage

### check — gate violations

```sh
archfit check --config .archfit.yaml
```

Exits 0 (pass), 1 (error), or 2 (gate violations found). Add `--full` to include
all rule types; add `--advisory` to include BC advisory findings in output.

### scan — full audit report

```sh
archfit scan --config .archfit.yaml
```

Equivalent to `check --full --advisory --report --format markdown`. Produces a
Markdown report with Health Summary, Gate Violations, BC Advisories, Metrics,
and Map Staleness sections. Exits 0 on pass, 2 on gate violations.

### init — generate starter config

```sh
# Write to stdout
archfit init --output - --root .

# Write to .archfit.yaml in current directory
archfit init --root .
```

Discovers Go packages (via `go list`), TypeScript (`src/`/`lib/` under `package.json`),
and Python packages (directories with `__init__.py` under `pyproject.toml`/`setup.py`).
Emits a starter `.archfit.yaml` with inferred modules, layers, and a `gate: warn` rule.
Review and promote rules to `gate: fail` once the baseline is established.

### doctor — toolchain check

```sh
archfit doctor
```

Prints a table of all required and optional tools with their status and resolved
paths: `go`, `git`, `node`, `npx`, `bunx`, `uv`, `python3`, `sg` (ast-grep),
`scip-typescript`, `scip-python`, `scip-go`.

### baseline — record known findings

```sh
archfit baseline --full --config .archfit.yaml
```

Snapshots current findings into `.archfit-baseline.json`. Subsequent `check`/`scan`
runs mark matching findings as `status: baseline` instead of `status: new`.

## Toolchain Requirements

| Tool               | Required | Purpose                                          |
| ------------------ | -------- | ------------------------------------------------ |
| Go 1.26+           | yes      | binary + Go analysis (`go list`, `go vet`)       |
| git                | yes      | change history for `new` finding detection       |
| Node.js 22+        | optional | TypeScript analysis via dependency-cruiser       |
| dependency-cruiser | optional | TypeScript/JS dependency graph (`npx depcruise`) |
| Python 3.12+       | optional | Python analysis via grimp                        |
| uv                 | optional | Python environment management                    |
| sg (ast-grep)      | optional | structural pattern evidence (Phase 3)            |
| scip-typescript    | optional | TypeScript barrel-file symbol resolution         |
| scip-python        | optional | Python symbol resolution                         |
| scip-go            | optional | Go symbol resolution                             |

The static Go binary covers Go-only analysis with no extra toolchain.
Use `archfit doctor` to check what is available on your host.
Use the Docker image for TypeScript or Python analysis without installing the toolchain.

## Development

### Prerequisites

- Go 1.26+
- Node.js + npm (for TypeScript analysis)
- Python 3.12+ or uv (for Python analysis)

### Setup

```sh
make setup-tools   # install golangci-lint, goimports, moq
pre-commit install --install-hooks
pre-commit install -t pre-push
```

### Commands

```sh
make build         # compile binary
make test          # run tests with race detector
make lint          # run golangci-lint
make fmt           # format code
make mock          # regenerate mocks
```

## Docker

The fat Docker image bundles Go binary + Node.js 22 + dependency-cruiser + Python 3.12 + uv + grimp.
No toolchain required on the host — useful for TypeScript and Python analysis.

```sh
# Pull from GHCR
docker pull ghcr.io/alexei-led/archfit:latest

# Run check against a repo mounted at /repo
docker run --rm -v $(pwd):/repo ghcr.io/alexei-led/archfit:latest \
    check --config /repo/.archfit.yaml

# Full audit report (Markdown)
docker run --rm -v $(pwd):/repo ghcr.io/alexei-led/archfit:latest \
    scan --config /repo/.archfit.yaml

# Verify bundled toolchain
docker run --rm ghcr.io/alexei-led/archfit:latest doctor
```

The bare static binary (`go install`) is sufficient for Go-only analysis.
Use the Docker image when you need TypeScript (dependency-cruiser) or Python (grimp) analysis.

### Build the image locally

```sh
make docker-build   # linux/amd64 + linux/arm64 (requires docker buildx)
make docker-run     # smoke test: archfit --help
```
