# archfit

[![CI](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml/badge.svg)](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml)

Architecture fitness checker for Go, TypeScript, and Python repositories.

`archfit` turns architecture intent into executable checks. It extracts dependency
facts from a repo, compares them with `.archfit.yaml`, and reports gate findings,
Balanced Coupling advisories, metrics, and baseline status.

Use it in local development, CI, pre-push hooks, or AI-agent repair loops when you
want deterministic feedback that code still fits the intended architecture.

## Quick start

Install the CLI:

```sh
go install github.com/alexei-led/archfit/cmd/archfit@latest
```

Run the first check in a repository:

```sh
archfit doctor
archfit init --root .
$EDITOR .archfit.yaml
archfit check --config .archfit.yaml --full
```

Create a Markdown audit report:

```sh
archfit scan --config .archfit.yaml > archfit-report.md
```

If the current findings are accepted technical debt, save a baseline:

```sh
archfit baseline --full --config .archfit.yaml
```

After that, new findings are marked as `new`; known findings are marked as
`baseline` until fixed or re-baselined.

## What it checks

`archfit` focuses on architecture drift, not general code quality.

It can check:

- forbidden dependencies between paths or modules;
- public API boundaries and internal API access;
- layer direction rules;
- import cycles;
- new cross-module dependencies;
- coupling advisories based on strength, distance, volatility, and explicitness;
- metric deltas such as encapsulation, unbalanced edges, cycles, and coverage.

Three additional report-only (info-band) metrics surface structural risk without
gating CI:

- `risk_hub` — cross-module surface-breadth: counts the distinct symbols in a
  module that are referenced by at least one symbol in a _different_ module.
  SCIP-based. Surfaces broad-interface hubs that module-level `blast_radius`
  misses (e.g. a 42-dep store visible only at symbol granularity).
  Requires `tools.scip.enabled: on`; reports `n/a` otherwise.
- `architecture_fitness` — detects whether architecture intent is actually
  enforced: presence of arch tests, import-linter config, or archfit/deptry/
  dependency-cruiser in CI workflows. Score 0–10, report-only.
- `functional_candidates` — clone-detection + co-change candidate pairs:
  module pairs that share duplicated logic, surfacing implicit functional
  coupling with no import edge. Distinct from `hidden_coupling` (co-change
  without an edge). Requires `tools.clones.enabled: on`; reports `n/a` otherwise.

### Structural-facts block

With `tools.scip.enabled: on`, every run also emits a neutral per-module
**structural-facts block** (`file_facts` in JSON; a compact
"Structural facts" section in Markdown). One entry per symbol-graph module:

```json
{
  "module": "ccgram.handlers.polling.polling_state",
  "files": ["src/ccgram/handlers/polling/polling_state.py"],
  "inbound_module_fanin": 13,
  "outbound_destinations": 9,
  "loc": 1018,
  "cochange_partners": ["src/.../observe.py", "..."],
  "gitnexus_impact": 41
}
```

The block is evidence, not judgment: no band, no score, no risk labels, and it
never affects the `check` verdict. It exists so a downstream reviewer (human or
LLM) can rank intra-module risk — e.g. separate a benign read-only config hub
from a mutable shared-state hub with the same fan-in — by reading the named
files. `gitnexus_impact` appears only when the optional gitnexus provider ran;
the block is empty when SCIP is off.

## Configuration

Configuration lives in `.archfit.yaml`.

Generate a starter file:

```sh
archfit init --root . --output .archfit.yaml
```

Then review the generated modules, layers, and rules before using it as a gate.
Start with narrow rules and baseline accepted current findings while calibrating.
Keep `gate` values aligned with the intended CI policy.

Minimal shape:

```yaml
version: 1
layers: [domain, application, adapter]
modules:
  domain:
    paths: [internal/domain/**]
    public: [internal/domain]
    layer: domain
    subdomain: core
rules:
  - id: no_adapter_to_domain_internal
    type: public_api_only
    gate: fail
```

See [`docs/guide/`](docs/guide/README.md) for the user guide.

## Commands

- `archfit doctor` — check available local toolchain.
- `archfit init` — generate a starter `.archfit.yaml`.
- `archfit check` — run architecture gates and metrics.
- `archfit scan` — produce a full Markdown audit report.
- `archfit baseline` — record accepted current findings.
- `archfit explain <id>` — explain one finding by fingerprint prefix.
- `archfit install` — install or print commands for optional language tools.

Output formats for `check`: `text`, `json`, `markdown`/`md`.

Exit codes:

- `0` — pass;
- `1` — fail;
- `2` — warn;
- `3` — usage, config, or runtime error.

## Toolchain

Go analysis works from the Go binary and Go package loader. TypeScript and Python
analysis use optional external tools.

```sh
archfit doctor
archfit install --lang py --lang ts --dry-run
```

Two additional tools are opt-in and off by default:

- `tools.gitnexus.enabled: on` — enriches `risk_hub` symbol-impact with
  gitnexus historical graph data. Requires gitnexus on PATH. Never runs
  automatically (network access, reproducibility).
- `tools.clones.enabled: on` — runs a clone detector (jscpd for JS/TS,
  PMD-CPD for Go/Py) to populate `functional_candidates`. Off by default
  (expensive). Absent tool → metric reports `n/a`.

Tool-coverage sources reported by `archfit check`/`scan`:

- `scip-symbols` — SCIP-based symbol fan-in used by `risk_hub`.
- `clones` — clone-detection pass used by `functional_candidates`.
- `gitnexus` — optional symbol-impact enrichment provider.

Use Docker when you want the bundled toolchain instead of installing language
analysis tools on the host:

```sh
docker run --rm -v "$(pwd):/repo" ghcr.io/alexei-led/archfit:latest \
  check --config /repo/.archfit.yaml --full
```

## Documentation

- [`docs/guide/`](docs/guide/README.md) — user guide and documentation map.
- [`docs/spec/arch-fitness-spec-v0.4.md`](docs/spec/arch-fitness-spec-v0.4.md)
  — product/build spec.
- [`docs/design/arch-fitness-architecture-v0.2.md`](docs/design/arch-fitness-architecture-v0.2.md)
  — internal architecture design.

## Development

```sh
make setup-tools
pre-commit install --install-hooks
pre-commit install -t pre-push
make test
make lint
make build
```

The release workflow builds static binaries and a multi-arch Docker image.
