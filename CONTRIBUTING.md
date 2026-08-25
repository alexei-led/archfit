# Contributing

Thanks for improving `archfit`.

Keep changes small, deterministic, and documented. `archfit analyze` is a gate:
it must stay reproducible and must not require network access or an LLM. Optional
LLM features belong behind explicit commands or config and stay off the gate.

## Prerequisites

- Go 1.26+
- Git
- `pre-commit`
- Python 3.12+ — `make test` (and CI) runs `python3
internal/extract/scip/scip_reader_test.py` unconditionally, regardless of
  language focus
- Optional: Node.js 24 LTS or Bun for TypeScript analysis
- Optional: `uv` for Python analysis (Python 3.14 recommended)

`archfit doctor` reports which optional tools are available.

## Local setup

```sh
git clone https://github.com/alexei-led/archfit.git
cd archfit
make setup-tools
pre-commit install --install-hooks
make test
```

Use the development binary directly while iterating:

```sh
go run ./cmd/archfit --help
go run ./cmd/archfit doctor
```

## Version pinning policy

Use explicit stable versions in repository docs, workflows, and scripts.

- Do not use floating labels or tags such as `latest`, `ubuntu-latest`, or
  unversioned package installs in repeatable docs, workflows, or scripts.
- Exception: README may use `ghcr.io/alexei-led/archfit:latest` for the simple
  Docker copy-paste path.
- Pin GitHub-hosted runners to explicit labels, for example `ubuntu-24.04` or
  `ubuntu-24.04-arm`.
- Pin GitHub Actions to released versions, or preferably full SHAs with a version
  comment when touching workflow files.
- Pin package-manager examples with an exact release, such as
  `dependency-cruiser@17.4.3` or the grimp package `grimp==3.13`.
- Use release tags for repeatable user commands, such as `@v0.1.0` or
  `:v0.1.0`.

## Development loop

Common checks:

```sh
make fmt
make test
make lint
make build
```

Useful targeted checks:

```sh
go test ./internal/ -run TestArchImports
go test ./internal/application/ -run TestGolden
pre-commit run --all-files
```

Regenerate mocks after changing interfaces under `internal/evidence/ports`:

```sh
make mock
```

## Optional analyzer tools

The CLI can print or install known language tools:

```sh
go run ./cmd/archfit doctor --fix --lang go --lang ts --lang py --dry-run
```

For deterministic CI, prefer explicit setup in the workflow over implicit local
installation. See [docs/guide/install.md](docs/guide/install.md) and
[docs/guide/languages.md](docs/guide/languages.md).

## Tests and docs

- Add behavior tests for non-trivial logic and regressions.
- Update golden tests when output intentionally changes.
- Keep deterministic gate behavior LLM-free.
- Update `docs/guide/` when CLI behavior, config, metrics, or output changes.
- Keep `README.md` compact; link to guide pages instead of duplicating them.

## Pull request checklist

Before opening a pull request:

1. Run `make fmt`.
2. Run `make test`.
3. Run `make lint` when `golangci-lint` is installed.
4. Run `pre-commit run --all-files` when hooks are installed.
5. Update docs and examples affected by the change.

## Releases

Maintainers publish releases by pushing a tag matching `vX.Y.Z` or `v*-rc.*`.
The release workflow builds static binaries for Linux and macOS and publishes a
multi-arch container image to GHCR.
