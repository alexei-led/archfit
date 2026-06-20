# spotinfo architect-skill architecture quick sweep

## 1. Scope, refs, dirty-state risk

- Target: `/Users/alexei/Workspace/spotinfo`.
- HEAD: `ed8d6d0` / tag `v2.3.1`.
- Delta baseline: `v2.3.0` (`23007d3`).
- Dirty state: untracked `.archfit-cache/`, `.archfit.yaml`, `.codegraph/`. These can affect tools that scan all files unless constrained to tracked source.
- Review depth: quick sweep. No final architect score assigned.

## 2. Intent evidence with file refs

- `README.md`: CLI + MCP server for AWS EC2 Spot Instance pricing, interruption, and placement-score analysis.
- `CLAUDE.md`: intended units are `cmd/spotinfo`, `internal/spot`, `internal/mcp`; embedded data fallback and AWS API providers are central design constraints.
- `go.mod`: Go 1.24 CLI with AWS SDK, `urfave/cli`, `mcp-go`, `go-pretty`, and test dependencies.
- `.archfit.yaml`: generated, warning-only starter config with `cmd_spotinfo`, `mcp`, and `spot` modules.

## 3. System map

- Language/package manager: Go module `spotinfo`.
- Units:
  - `cmd/spotinfo`: CLI entry point and output formatting.
  - `internal/spot`: core pricing/advisor/placement-score domain and provider interfaces.
  - `internal/mcp`: MCP protocol adapter exposing spot queries to AI assistants.
- Internal import direction from `go list -json ./...`:
  - `cmd/spotinfo` imports `internal/mcp` and `internal/spot`.
  - `internal/mcp` imports `internal/spot`.
  - `internal/spot` imports no internal package.
- Runtime integrations: AWS S3 public feeds, EC2 APIs, MCP stdio/SSE transports, embedded JSON fallback.

## 4. Tool coverage and commands run

- `git status --short` — dirty only due untracked local analysis files.
- `git log --oneline --decorate --max-count=8` — confirmed baseline and tags.
- `git diff --stat v2.3.0..HEAD` / `git diff --name-only v2.3.0..HEAD` — 3 files, 7 insertions, 7 deletions.
- `git diff --unified=3 v2.3.0..HEAD -- README.md cmd/spotinfo/main.go internal/spot/data.go` — delta is README Homebrew command plus gosec suppression comments.
- `go list ./...` — 3 packages.
- `go list -json ./...` — internal dependency direction.
- `go test ./...` — passed; `internal/mcp` took ~92s, so test runtime is a repeatability/CI-cost signal.
- Existing archfit artifacts used: `scorecard.md`, `scan.md`, `llm-review.md`, `full.json`, `delta.json`.

Coverage gaps: no full LSP/call graph, no staticcheck/govulncheck pass in this direct sweep, no runtime AWS integration check.

## 5. Full-current architecture observations

- The architecture is simple and mostly balanced: `internal/spot` owns core domain knowledge; `internal/mcp` adapts it to MCP; `cmd/spotinfo` composes CLI and MCP modes.
- Dependency direction is clean: adapters depend inward on `internal/spot`; the core does not depend on adapters.
- `internal/spot` is a high-fan-in hub by design. That is cohesive while the repo remains one binary/one owner, but it should expose stable provider interfaces and avoid leaking AWS SDK types outward.
- `internal/mcp` is supporting/high-distance relative to core domain. It should depend on contracts from `internal/spot`, not duplicate pricing/filtering rules.
- `go test ./...` passed. The long MCP test runtime should be watched because slow tests reduce repeatable architecture feedback.

## 6. Delta observations since baseline

- Delta is not architectural: README install command corrected; code changes add `//nolint:gosec` comments for false positives in `cmd/spotinfo/main.go` and `internal/spot/data.go`.
- No new package, dependency direction, runtime integration, or module boundary was introduced between `v2.3.0` and `v2.3.1`.
- Delta risk: low. The only architecture-relevant point is that lint suppressions should stay narrowly justified and not become blanket security-noise masking.

## 7. Balanced Coupling relationship records

### BC-1: `cmd/spotinfo` → `internal/spot`

- Level: package boundary inside one binary.
- Strength: functional/model. CLI uses core option types, advice models, and output values.
- Distance: low code/deploy distance; same repo and binary.
- Volatility: `internal/spot` is core/high-to-medium volatility because AWS data, filters, and placement-score behavior evolve; CLI is supporting/generic.
- Severity: low. High strength + low distance is cohesive.
- Balancing move: leave close. Keep CLI orchestration thin and keep business rules in `internal/spot`.

### BC-2: `internal/mcp` → `internal/spot`

- Level: adapter-to-core package boundary.
- Strength: functional. MCP handlers call spot query behavior and translate protocol inputs/outputs.
- Distance: medium code distance, low deploy distance; same binary but different protocol audience.
- Volatility: MCP protocol/tool schemas can evolve independently from spot-pricing rules.
- Severity: medium if MCP duplicates rules; low if it remains an adapter.
- Balancing move: lower strength through stable request/response contracts in `internal/spot`; keep MCP-specific schema and transport in `internal/mcp`.

### BC-3: `internal/spot` → AWS feeds/APIs

- Level: external system integration.
- Strength: contract/model. Uses AWS feed schemas and EC2 API models; embedded fallback lowers runtime coupling.
- Distance: high runtime/ownership distance.
- Volatility: AWS API/feed behavior is generic but provider-controlled; missing/new instance families are known change vectors.
- Severity: medium. High distance is acceptable only while provider details stay behind provider interfaces.
- Balancing move: keep anti-corruption/provider interfaces and tests around embedded fallback, live price fallback, and placement-score behavior.

## 8. Architect-skill blind spots vs archfit

- This quick sweep did not enumerate clone pairs, hidden-coupling pairs, risk hubs, or change-amplification counts. Archfit did: 0 hidden-coupling pairs, 1 duplication pair, 2 blast-radius hubs, propagation cost 0.50.
- This quick sweep did not compute a repeatable scorecard. Archfit produced overall `69/100` with dimension scores.
- Manual review is sampled and can miss low-level complexity; archfit found complex functions including `GetSpotSavings` and `execMainCmd`.

## 9. Archfit blind spots vs architect skill

- Archfit scored `architecture_fitness` 0/100, which is accurate for explicit architecture checks but easy to overread: the repo does have normal CI/tests; it lacks boundary-specific fitness.
- Archfit flagged `.gitnexus/run.cjs` as a complex function in `scan.md`. That is local tool noise, not spotinfo source architecture.
- Archfit boundary confidence is low because the generated config lacks owner/subdomain/volatility detail. Human review can infer `internal/spot` is core, `internal/mcp` is supporting, and `cmd/spotinfo` is composition/generic.
- Archfit does not judge whether long `internal/mcp` tests harm feedback-loop repeatability.

## 10. Reliability/repeatability notes

- Deterministic archfit JSON repeat matched for spotinfo in the main harness.
- `archfit review` succeeded for spotinfo with Anthropic.
- `go test ./...` passed but took over 90s in `internal/mcp`; this is a repeatability cost.
- Untracked local analysis files should be excluded from future source metrics.

## 11. Target architecture/design suggestions and executable fitness checks

- Commit/review `.archfit.yaml` only after replacing generated warning-only defaults with intended module metadata.
- Suggested module metadata:
  - `internal/spot`: core, high/medium volatility, owns pricing/advisor/score rules and provider interfaces.
  - `internal/mcp`: supporting adapter, medium volatility, owns MCP transport/tool schemas.
  - `cmd/spotinfo`: generic composition root, medium/low volatility.
- Fitness checks:
  - Go architecture test: `internal/spot` must not import `internal/mcp` or `cmd/spotinfo`.
  - Archfit gate: `cmd -> adapters/core allowed`, `mcp -> spot allowed`, reverse edges fail.
  - CI check: `go test ./...` plus a faster focused architecture test to avoid 90s feedback just to detect dependency drift.
  - Optional: guard AWS provider interfaces so tests never call real AWS without explicit integration tag/short-skip.

## 12. Next checks

1. Add an architecture import test for package direction.
2. Exclude `.gitnexus`, `.codegraph`, `.archfit-cache`, and report artifacts from structural metrics.
3. Split slow MCP tests or mark true integration tests to keep architecture feedback fast.
4. Re-run archfit after config metadata is reviewed; expect better boundary/coupling confidence.
