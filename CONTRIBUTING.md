# Contributing

Thanks for improving `archfit`.

Keep changes small, deterministic, and documented. `archfit check` is the gate:
it must stay reproducible and must not require network access or an LLM. Use
`archfit analyze` for reports. Optional LLM features belong behind explicit
commands or config and stay off the gate.

## Prerequisites

- Go 1.27.0
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
go test ./internal/ -run TestArchImports        # import ring
go test ./internal/ -run TestSelfModel          # .archfit.yaml describes real source
go test ./internal/ -run TestModelSurfaceNoDrift # published model contract
go test ./internal/application/ -run TestGolden # output fixtures
go test ./internal/ ./cmd/archfit/ -run TestErosion_ # architecture-state erosion gates
make arch-lint                                 # run the configured v2 gate
pre-commit run --all-files
```

`make arch-lint` runs `archfit check` and accepts exit `0` (healthy) or `2`
(needs attention); exit `1` (blocked) and exit `3` (error) fail it. Exit `0` is reachable with
complete evidence. This repo may report `2` while supplied coverage,
operational corroboration, or a comparable persisted baseline is missing; no
evidence gap is reported as healthy.

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

## Architecture boundaries

archfit gates its own architecture. Before moving a package or adding a
capability, read
[docs/design/architecture-baseline.md](docs/design/architecture-baseline.md) —
it records the layer ranks, the measured module dependency map, which check
enforces which invariant, and the change recipes for adding a metric, language,
output format, or CLI command.

Two rules to internalise:

- The layer direction is `model → support → core → application → adapter → cmd`.
  Inner never imports outer; `forbidden_layer_direction` gates it at `fail`.
- Moving a package means updating the owning module's `paths:` in
  `.archfit.yaml` in the same commit. `go test ./internal/ -run TestSelfModel`
  fails on an unowned package and on the glob you left behind.

Do not weaken a gate, add a waiver, relabel volatility, or re-baseline to make a
check pass. Fix the boundary or record the accepted risk with a written
rationale.

### Architecture-state erosion gates

Six named checks keep the state report from decaying back into the averaged
score it replaced. CI runs them as an explicit step; run them locally with
`go test ./internal/ ./cmd/archfit/ -run TestErosion_`.

| Check                       | What it prevents                                                     |
| --------------------------- | -------------------------------------------------------------------- |
| `no_scalar_decision`        | an averaged score re-entering the path from evidence to exit code    |
| `no_dead_archfit_rule`      | a rule reporting "0 violations" for a boundary nobody checks         |
| `dimension_status_required` | an envelope with no status reading as an empty, healthy result       |
| `config_hash_required`      | a delta taken across a config edit blaming the code                  |
| `label_evidence_required`   | an unevidenced approval silencing a seam permanently                 |
| `baseline_idempotent`       | a self-referential capture reporting drift that is not there         |

Each check has a paired fixture proving it fires on a violating input, so none
can pass because it happens to look at nothing. When you extend one, extend its
fixture in the same commit.

Three more rules that follow from the state contract:

- A dimension that measured nothing reports `unmeasured` with a named
  `UnknownFact`. Never report a missing collector as a measured zero — an empty
  list reads as healthy, and "we found no problems" must stay distinguishable
  from "nothing looked".
- The four comparability fingerprints live only in the root `comparison` block.
  A second copy is a second answer to whether two runs may be compared.
- `.archfit-labels.yaml` stays empty unless a pair was reviewed by hand. An
  approved label needs `evidence_hash`, `rationale`, `provenance`, and
  `confidence`; without the hash it can never go stale and silences its pair
  permanently.

See
[docs/design/architecture-state-reporting.md](docs/design/architecture-state-reporting.md)
for the full contract, the nine dimensions, and what v1 does and does not
measure.

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
