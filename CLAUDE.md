# archfit

Architecture-fitness CLI (Go). Reads dependency facts from a target repo, checks
them against `.archfit.yaml`, emits gate violations + metrics for CI and AI agents.
Language facts come from external tools run out-of-process: `go list`,
dependency-cruiser, ast-grep, grimp, `cargo metadata`, gocyclo.

## Commands (Makefile)

- `make build` — static binary, `CGO_ENABLED=0` → `.bin/archfit`
- `make test` — `go test -race -coverprofile=coverage.out ./...`
- `make lint` — `golangci-lint run -c .golangci.yaml ./...` (pinned v2.1.6)
- `make fmt` — `gofmt -s` + `goimports -local github.com/alexei-led/archfit`
- `make archfit` — dogfood architecture-drift gate: `.bin/archfit check --config .archfit.yaml --full`
- `make arch-lint` — architecture drift linter (alias for `make archfit`); wired into the pre-push hook
- `make archfit-report` — write `reports/archfit-report.md` via `archfit scan`
- `make mock` — regenerate moq fakes (`go generate ./...`)
- `make test-fast` — `go test -race -short ./...` (skips slow subprocess/ast-grep integration tests; for inner-loop speed)
- `make all` — fmt → lint → test → archfit
- One test: `go test ./internal/<pkg>/ -run TestName`

## Structural gates (CI runs these explicitly — keep green)

- Import ring: `go test ./internal/ -run TestArchImports`
- Golden output: `go test ./internal/engine/ -run TestGolden` — regenerate
  deliberately and inspect the diff; output changes are never automatic.
- Dogfood gate: `make archfit` — CI runs the same target after tests/goldens. Also
  runs locally pre-push via the `arch-lint` hook in `.pre-commit-config.yaml`. The
  self-config (`.archfit.yaml`) gates its own architecture: forbidden-dependency
  ring + `forbidden_layer_direction` (fail) and a god-struct ceiling
  (`struct_field_max: 90`, fail; current max is `Diagnostic` at ~61 body-lines).
  `public_api_type_leak` runs advisory (warn).

## Invariants

Enforced by `internal/arch_test.go`; extend that test when adding a boundary.

- Core ring (`classify`, `rules`, `metrics` + sub-packages, `status`, `staleness`,
  `facts`, `score`, `scope`, `syntax`) must not import `os`, `os/exec`, any YAML
  lib, or adapter packages — it decides over already-gathered facts. `score`
  synthesises the banded scorecard from an already-computed `Diagnostic`.
  `syntax` derives architectural roles (handler/service/repository/domain) and
  builds the `NodeRoleIndex` used by `forbidden_role_dependency`.
- `internal/model/*` imports stdlib only.
- Every subprocess call goes through `toolrun.Runner` (interface in `internal/ports`);
  extractors in `internal/extract/{go,ts,py,rust}` are out-of-process adapters. No
  `exec.Command` in core code — fake the `Runner` in tests.
- **No gitnexus.** The `risk_hub` dependant-impact factor and the per-module
  `symbol_dependants` facts/JSON field are derived in-process from the SCIP symbol
  graph (`symbol.DependantsFromSymbolGraph`) — both are `n/a` unless SCIP is enabled.
  The `.gitnexus`/`.codegraph` index dirs are still excluded from file walks
  (`scope.go`), but archfit no longer runs the tool.
- **Complexity backend is configurable** (`tools.complexity.backend`): `auto`
  (default) = gocyclo for exact Go CCN + an ast-grep decision-point proxy for
  TS/Py/Rust (no default Python pin); `lizard` opts back into exact per-function CCN.
  Report-only hotspot metric (over-flagging is the safe direction).
- **Go edge strength** comes from `go/packages` type info (`NeedTypesInfo`,
  mirroring the `scip_reader` BC mapping) when SCIP is off; SCIP `enrichEdges`
  overrides it when on. Unclassified edges stay `unknown` (abstain-not-fake).
- Parse config once into typed views; pass a package its view, not the whole config.
- LLM SDKs (`anthropic-sdk-go`, `openai-go`) are off-gate: only `enrich`,
  `autopilot`, `explain`, and `review` touch them, never `check`. Enforced
  structurally — `arch_test.go` forbids any `internal/*` package from importing
  `internal/llm`, so the LLM commands live in `cmd`.
- `gate:` is wired for **all rule types** (`off` skips, `warn` is advisory/non-blocking,
  `fail`/unset is blocking; exception: `public_api_change` defaults to `warn` when unset). An unknown `type` value is a config error.

## Coupling scorer — key design facts

`ScoreVersion = "bc_score.v3"` (`internal/model/coupling/scorer.go`).
Formula: `balance = max(|S−D|, 10−V) + 1` (Khononov Ch10 verbatim).
Ordinals frozen as named constants — changing any is a breaking metric change.

**Abstain-not-fake:** when strength OR distance is `unknown`, the edge is
unscored (`EdgeScore.Scored = false`). No invented ordinals. Genuine internal
edges with unknown strength stay in the `abstained` bucket (lowers
`coupling_balance` confidence); external/library edges (`DistanceUnknown`) are
excluded from the denominator entirely (counted in `classified_edges.external`).

**External edges excluded from `coupling_balance`:** edges whose target is not a
declared module are NOT internal coupling seams. Their count surfaces in
`classified_edges.external` and the `coupling_balance` evidence string.
`dependency_graph_health` is where external-dependency concerns live.

**Symmetric from clones:** when `tools.clones` detects a cross-module clone
pair, the edge strength is upgraded to `StrengthSymmetric` (S=9) unless a
config-authoritative `contract`/`intrusive` or an approved pinned label already
applies.

**Provenance lowers confidence:** approved labels in `.archfit-labels.yaml` with
`provenance: llm` and `confidence` below `high` lower `coupling_balance`
confidence by one band. `provenance: human` and `provenance: tool` do not.

**Opt-in volatility cascade:** `volatility_cascade_enabled: true` in
`.archfit.yaml` enables a single-hop propagation pass (book Ch9) that raises
effective volatility to `high` for modules strongly coupled to a `core` module.
archfit's own self-config has this enabled.

**Runtime async is report-only:** `runtime_async` JSON field records async-bridge
evidence per module. Never annotates graph edges, never affects distance or
balance score, never gates. This is a deliberate design decision — do not wire
async detection into distance.

## Release (tag-triggered — never release manually)

`git tag -a vX.Y.Z -m … && git push origin vX.Y.Z` → `release.yaml` builds binaries +
multi-arch image, pushes `ghcr.io/alexei-led/archfit:<tag>` + `:latest`, and its
`release` job runs `gh release create` itself. A second `gh release create` (or a
release tool) collides on the tag and fails the job.

## Runtime image

`Dockerfile` is `debian:bookworm-slim` (glibc; musl broke ast-grep) — one image with
Go SDK, git, Node 24, dependency-cruiser, ast-grep (`sg`), uv, python3; non-root.
The Rust toolchain (`cargo`, `rust-analyzer`) is **not** bundled — Rust analysis
reports `n/a` (never fails) in the image; run on a host with cargo or extend it.
`archfit doctor` checks tools; `sg` must resolve to ast-grep, not util-linux. Build
amd64 in CI, not local emulation.

## Rust analysis granularity

Rust crate facts are **crate-level**: `cargo metadata` makes one graph node per
workspace member. The scorecard caps a **degenerate graph** (<2 connected modules,
e.g. a single crate) at `mixed` — it never scores `strong` on a one-node graph (see
`internal/score`, `degenerateGraph`). Opt-in `tools.cargo-modules.enabled` adds an
**intra-crate module graph** (`<crate>::<mod>` nodes + aggregated `uses` edges), so
single-crate projects get real cycle/blast-radius/cohesion signal. Opt-in
`tools.scip.enabled` runs rust-analyzer SCIP, which produces a correct
`<crate>::<mod>` strength map and attaches `StrengthHint` to those module edges. The
engine then registers auto-discovered module nodes as modules
(`classify.AugmentModulesFromGraph`, gated on the `::` separator so Go/TS/Python are
untouched) so distance/volatility classify and the strength is consumed — verified on
herdr: `coupling_balance` and `cohesion_lcom` measure (were n/a). `encapsulation`
stays `n/a` for typical Rust by design: it scores only contract/intrusive edges, and
Rust's module privacy makes cross-module _intrusive_ edges rare. With all three on
(`tools.rust` + `tools.cargo-modules` + `tools.scip`) a single-crate Rust project gets
full module-level coupling analysis.

Per-file size/history metrics resolve to **module granularity** too: the extractor carries
crate roots (`graph.CrateRoot`, repo-relative src dir + crate name from cargo metadata) on
the graph, and `modgraph.ModuleKeyResolver` maps a `.rs` file to its module key
(`crate/src/a/b.rs → crate::a::b`) so `structural_weight` (god-file-by-size),
`change_coupling`, `change_amplification`, `hidden_coupling` and `functional_candidates`
measure at module level instead of collapsing every file to the crate. It is filesystem
convention (accepted ceiling: `#[path]`, inline `mod {}`, `include!` diverge from
rust-analyzer's semantic tree; SCIP is the precision upgrade). The meta dimension
`analysis_confidence` is also capped by the share of n/a/low-confidence dimensions, so a
fully-tooled run no longer reads 100 when (by Rust design) encapsulation is n/a. See
`docs/plans/completed/20260621-archfit-rust-depth-and-calibration.md`.

## Layout

`cmd/archfit` (kong CLI) · `internal/` decision core + adapters · `docs/design`
(decisions) · `docs/guide` (user docs) · `docs/plans`.
