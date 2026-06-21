# archfit

Architecture-fitness CLI (Go). Reads dependency facts from a target repo, checks
them against `.archfit.yaml`, emits gate violations + metrics for CI and AI agents.
Language facts come from external tools run out-of-process: `go list`,
dependency-cruiser, ast-grep, grimp, `cargo metadata`.

## Commands (Makefile)

- `make build` — static binary, `CGO_ENABLED=0` → `.bin/archfit`
- `make test` — `go test -race -coverprofile=coverage.out ./...`
- `make lint` — `golangci-lint run -c .golangci.yaml ./...` (pinned v2.1.6)
- `make fmt` — `gofmt -s` + `goimports -local github.com/alexei-led/archfit`
- `make mock` — regenerate moq fakes (`go generate ./...`)
- `make all` — fmt → lint → test → build
- One test: `go test ./internal/<pkg>/ -run TestName`

## Structural gates (CI runs these explicitly — keep green)

- Import ring: `go test ./internal/ -run TestArchImports`
- Golden output: `go test ./internal/engine/ -run TestGolden` — regenerate
  deliberately and inspect the diff; output changes are never automatic.

## Invariants

Enforced by `internal/arch_test.go`; extend that test when adding a boundary.

- Core ring (`classify`, `rules`, `metrics` + sub-packages, `status`, `staleness`,
  `facts`, `score`, `scope`) must not import `os`, `os/exec`, any YAML lib, or
  adapter packages — it decides over already-gathered facts. `score` synthesises
  the banded scorecard from an already-computed `Diagnostic`.
- `internal/model/*` imports stdlib only.
- Every subprocess call goes through `toolrun.Runner` (interface in `internal/ports`);
  extractors in `internal/extract/{go,ts,py,rust}` are out-of-process adapters. No
  `exec.Command` in core code — fake the `Runner` in tests.
- Parse config once into typed views; pass a package its view, not the whole config.
- LLM SDKs (`anthropic-sdk-go`, `openai-go`) are off-gate: only `enrich`,
  `autopilot`, `explain`, and `review` touch them, never `check`. Enforced
  structurally — `arch_test.go` forbids any `internal/*` package from importing
  `internal/llm`, so the LLM commands live in `cmd`.

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
(decisions) · `docs/guide` (user docs) · `docs/plans`. Optional GitNexus index in
`.gitnexus/` / `.codegraph/`; refresh with `node .gitnexus/run.cjs analyze
--index-only` (`--index-only` skips gitnexus rewriting CLAUDE.md/AGENTS.md and
installing `.claude/skills/gitnexus/`; archfit only reads the index).
