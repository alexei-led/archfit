# archfit

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
