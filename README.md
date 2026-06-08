# archfit

[![CI](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml/badge.svg)](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml)

Architecture fitness checker for Go, TypeScript, and Python repositories.

## Installation

```sh
go install github.com/alexei-led/archfit/cmd/archfit@latest
```

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
    check --config /repo/archfit.yaml

# Full audit report (Markdown)
docker run --rm -v $(pwd):/repo ghcr.io/alexei-led/archfit:latest \
    scan --config /repo/archfit.yaml

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
