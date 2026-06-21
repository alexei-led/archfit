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

## Layout

`cmd/archfit` (kong CLI) · `internal/` decision core + adapters · `docs/design`
(decisions) · `docs/guide` (user docs) · `docs/plans`. Optional GitNexus index in
`.gitnexus/` / `.codegraph/`; refresh with `node .gitnexus/run.cjs analyze
--index-only` (`--index-only` skips gitnexus rewriting CLAUDE.md/AGENTS.md and
installing `.claude/skills/gitnexus/`; archfit only reads the index).
