# Architecture Review: spotinfo

**Date:** 2026-06-20  
**Reviewer:** Architect (independent, blind to archfit tooling)  
**Target:** `~/Workspace/spotinfo` @ HEAD (`955604a`)  
**Method:** Balanced Coupling (Khononov); evidence gathered via `go list`, `go vet`, `staticcheck`, `git log`, source reads  

---

## 1. System Map

### 1.1 Package Inventory

| Package | Role | Production LoC | Test LoC |
|---------|------|---------------|---------|
| `cmd/spotinfo` | CLI entrypoint + output formatting + MCP dispatch | 712 | 1,650 |
| `internal/mcp` | MCP server, SSE/stdio transport, tool handlers | 518 | 1,747 |
| `internal/spot` | Domain core: advisor data, pricing, scoring, live-price fallback | 1,433 | 2,344 |
| **Total** | | **2,663** | **6,458** |

Test-to-production ratio: **2.43x** — well above average for a CLI tool.

### 1.2 Dependency Direction

```
cmd/spotinfo
├── internal/mcp    (creates MCP server, injects spot.Client)
└── internal/spot   (primary consumer: creates Client, calls GetSpotSavings)

internal/mcp
└── internal/spot   (via spotClient interface defined locally in mcp/server.go:26)

internal/spot
├── aws-sdk-go-v2/config    (score.go, liveprice.go — each independently)
├── aws-sdk-go-v2/service/ec2  (score.go, liveprice.go)
├── bluele/gcache           (score.go — LRU cache)
├── golang.org/x/time/rate  (score.go — rate limiter)
└── net/http                (data.go — fetch from S3)
```

No import cycles. Dependency direction flows correctly: entrypoints → internal → external services.

### 1.3 Layering

```
┌──────────────────────────────────────────────┐
│  cmd/spotinfo  (CLI flags, output, dispatch)  │
└──────────┬───────────────┬────────────────────┘
           │               │
   ┌───────▼───────┐ ┌─────▼──────────────┐
   │ internal/mcp  │ │ internal/spot       │
   │ (MCP server)  │ │ (domain: advisor,   │
   └───────┬───────┘ │  pricing, score,    │
           └────────►│  live-price)        │
                     └──────────┬──────────┘
                                │
                     ┌──────────▼──────────┐
                     │ AWS SDK / S3 / net  │
                     │ (external services)  │
                     └─────────────────────┘
```

### 1.4 Entrypoints

- `cmd/spotinfo/main.go:main()` — single binary, bifurcates at `isMCPMode()` (`main.go:129`)
- MCP path: `runMCPServer()` → `mcp.NewServer(Config{})` → stdio or SSE transport
- CLI path: `execMainCmd()` → `spot.New().GetSpotSavings()` → output formatters

---

## 2. Evidence Catalogue

All citations are in the repo at HEAD (`955604a`).

| # | File:line | Observation |
|---|-----------|-------------|
| E1 | `cmd/spotinfo/main.go:159` | `spotClient` interface defined (duplicate #1) |
| E2 | `internal/mcp/server.go:26` | `spotClient` interface defined (duplicate #2, identical shape) |
| E3 | `internal/mcp/tools.go:28` | `spotClient` used by tool structs, same type from server.go |
| E4 | `internal/spot/score.go:1–9` | Imports `aws-sdk-go-v2/config`, `service/ec2`, `bluele/gcache`, `x/time/rate` |
| E5 | `internal/spot/liveprice.go:1–9` | Imports `aws-sdk-go-v2/config`, `service/ec2` independently |
| E6 | `internal/spot/score.go:75` | `createAPIProvider()` silently falls back to `mockScoreProvider` on any AWS config error |
| E7 | `internal/spot/client.go:72–83` | Four private interfaces: `advisorProvider`, `pricingProvider`, `scoreProvider`, `livePriceProvider` |
| E8 | `internal/spot/client.go:95–100` | `New()` wires: `newDefaultAdvisorProvider`, `newDefaultPricingProvider`, `newScoreCache()`, `createLivePriceProvider()` |
| E9 | `internal/spot/client.go:105` | `NewWithProviders(advisor, pricing)` for testing — does NOT inject scoreProvider or livePriceProvider |
| E10 | `internal/spot/client.go:111` | `SetLivePriceProvider(p)` — setter injection, not constructor injection |
| E11 | `cmd/spotinfo/main.go:166` | `execMainCmd` takes `spotClient` interface — testable |
| E12 | `internal/spot/data.go:11–12` | `//go:embed data/spot-advisor-data.json` + `spot-price-data.json` |
| E13 | `internal/spot/data.go:50–90` | `fetchAdvisorData()` fetches from S3, falls back to embedded JSON |
| E14 | `internal/spot/score.go:100–130` | `scoreCache` with LRU + rate limiter; `getSpotPlacementScores()` caches results |
| E15 | `go vet ./...` | `internal/spot/data_test.go:27,77` — `cancel()` from `context.WithTimeout` discarded (context leak) |
| E16 | `staticcheck ./...` | Clean (no output) |
| E17 | `git log --oneline` | 30 commits; churn leaders: `Makefile` (19), `README.md` (17), `public/spot/info.go` (11 — deleted) |
| E18 | `internal/spot/score.go:155–165` | Score-assignment loop assigns first score to ALL requested types — AWS returns per-type scores but the loop fills gaps with one result |
| E19 | `internal/mcp/tools.go:112` | `parseParameters(arguments interface{})` — untyped `map[string]interface{}` throughout |
| E20 | `cmd/spotinfo/main.go` (nolint comments) | `//nolint:cyclop,gocyclo,funlen` on `execMainCmd` and `mainCmd` |

---

## 3. Balanced Coupling Analysis

### BC Framework Applied

BC classifies coupling on three dimensions:
- **Integration strength**: intrusive > functional > model > contract
- **Distance**: fractal-aware; highest here is inter-package within a monorepo
- **Volatility**: derived from subdomain role (core / supporting / generic), not commit frequency

**Balance rule**: coupling is balanced when `(strength XOR distance) OR (NOT volatility)` — strong coupling is acceptable only across stable nearby seams; distant coupling is acceptable only when weak; high-volatility seams require extra care at any strength.

---

### 3.1 `cmd/spotinfo` → `internal/spot`

| Dimension | Classification | Rationale |
|-----------|---------------|-----------|
| Strength | **Model** | Consumer uses `spot.Advice`, `spot.GetSpotSavingsOption`; does not touch spot internals |
| Distance | **Local** | Same module, sibling package tree |
| Volatility | **Supporting** | spot is an adapter/domain package; stable API surface |

**Verdict: Balanced.** The `spotClient` interface is defined at the consumer (`main.go:159`), not exported from `internal/spot`. This follows the Go "accept interfaces, return structs" pattern correctly. `GetSpotSavings` with variadic options is a stable functional boundary.

**File:line evidence:** `cmd/spotinfo/main.go:159–163`, `cmd/spotinfo/main.go:166`

---

### 3.2 `internal/mcp` → `internal/spot`

| Dimension | Classification | Rationale |
|-----------|---------------|-----------|
| Strength | **Model** | Uses `spot.Advice`, `spot.GetSpotSavingsOption` through interface |
| Distance | **Local** | Sibling internal packages |
| Volatility | **Supporting** | MCP adapter; changes when transport changes, not when spot logic changes |

**Verdict: Balanced.** `spotClient` is defined locally at `mcp/server.go:26`, same shape as cmd's version.

**Finding F1 (Design smell, not BC violation):** `spotClient` is declared identically in two consumers (`cmd/spotinfo/main.go:159` and `internal/mcp/server.go:26`). Both have the single method `GetSpotSavings(ctx, ...option) ([]Advice, error)`. This is intentional Go practice (interface duplication avoids import cycles and keeps consumers independent), but creates a drift risk: if `spot.Advice` gains a required field, both callers must be updated without compiler enforcement across the two sites.

---

### 3.3 `internal/spot` → `aws-sdk-go-v2` (score.go + liveprice.go)

| Dimension | Classification | Rationale |
|-----------|---------------|-----------|
| Strength | **Functional** | Calls `awsconfig.LoadDefaultConfig`, `ec2.NewGetSpotPlacementScoresPaginator`, `ec2.NewDescribeSpotPriceHistoryPaginator` |
| Distance | **Distant** | External vendored dependency; crosses module boundary |
| Volatility | **Generic** | AWS SDK v2 is a stable external library; not subdomain-volatile |

**Balance rule check:** Functional strength + distant distance = tension. But volatility is low (generic/stable). Rule: `(strength XOR distance) OR (NOT volatility)`. Volatility is false (generic = not volatile), so `NOT volatility = true`, making the coupling balanced.

**Verdict: Balanced by volatility (AWS SDK is stable).**

**However — structural smell (Finding F2):** Both `score.go` and `liveprice.go` independently call `awsconfig.LoadDefaultConfig(context.Background(), ...)`. There is no shared AWS config factory. If AWS auth configuration needs to change (e.g., assume-role, custom endpoint, credential source), it must be changed in two places. The private interfaces (`awsAPIProvider` in score.go, `livePriceProvider` in liveprice.go) are parallel but unrelated — no shared adapter port exists despite both being AWS EC2 adapters.

**File:line evidence:** `score.go:70–85` (`newAWSScoreProvider`), `liveprice.go:60–75` (`newAWSLivePriceProvider`), both call `awsconfig.LoadDefaultConfig` independently.

---

### 3.4 `internal/spot` internal cohesion (score.go → liveprice.go boundary)

| Dimension | Classification | Rationale |
|-----------|---------------|-----------|
| Strength | **Intrusive** | Both access AWS SDK config-loading internals without shared factory; both modify `[]Advice` slice in place (enrichMissingPrices, enrichWithScores) |
| Distance | **Local** | Same package |
| Volatility | **Core** | Spot domain logic; changes with new data sources or scoring models |

**Verdict: Technically within one package so distance is minimal — not unbalanced by BC rules. But cohesion concern:** `internal/spot` is a single Go package containing 7 files with distinct responsibilities (advisor data fetching, pricing, scoring, live-price enrichment, types, client). Each sub-concern has its own private interface but they are all co-located in the same package. This inflates the blast radius of any change.

---

### 3.5 Silent Mock Fallback (Finding F3 — Correctness Risk)

`score.go:createAPIProvider()` (`score.go:75`) falls back silently to `mockScoreProvider` when AWS credentials are unavailable:

```go
func createAPIProvider() awsAPIProvider {
    if provider, err := newAWSScoreProvider(context.Background()); err == nil {
        return provider
    }
    return &mockScoreProvider{} // silent fallback — caller cannot detect this
}
```

`mockScoreProvider.fetchScores()` returns deterministic hash-based scores (`(len(instanceType)*7+i*3)%10 + 1`). In a production environment without AWS credentials, users get plausible-looking but entirely fake scores with no warning at the response level. The only signal is a debug log at init time. This is a **correctness risk** — not a coupling imbalance but a design flaw.

**File:line evidence:** `score.go:75–82`, `score.go:108–117`

---

### 3.6 Score Assignment Logic (Finding F4 — Correctness Risk)

`score.go:155–165`:

```go
for _, result := range output.SpotPlacementScores {
    score := int(aws.ToInt32(result.Score))
    for _, instanceType := range instanceTypes {
        if _, exists := scores[instanceType]; !exists {
            scores[instanceType] = score  // assigns FIRST result's score to ALL types
        }
    }
}
```

When AWS returns multi-instance scores, the outer loop iterates over results, but the inner loop assigns the first result's score to all unset instance types. If batch has 5 types and AWS returns 5 scores, type 2–5 all get the score from result[0]. This is a silent correctness bug.

**File:line evidence:** `internal/spot/score.go:155–165`

---

## 4. Banded Scorecard

### Scoring Key
- 9–10: Exemplary | 7–8: Good | 5–6: Adequate | 3–4: Problematic | 1–2: Critical

---

### 4.1 Modularity / Cohesion

**Score: 5 | Band: Adequate | Confidence: High**

Three packages with clear names. `internal/spot` bundles 7 distinct concerns (advisor fetch, pricing fetch, types, score cache, live-price enrichment, client, sort helpers) in one package without sub-packages. For a 2,663-LoC project this is acceptable, but the package is a catch-all for the entire domain. `cmd/spotinfo/main.go` at 712 lines conflates CLI flag parsing, output formatting (table, JSON, CSV, text, number), and MCP dispatch — it has 3+ nolint suppressions for complexity.

**Evidence:** `main.go:712 lines`, `//nolint:cyclop,gocyclo,funlen` at `main.go:167`, `main.go:398`; `internal/spot/` has 7 Go files covering distinct responsibilities with no sub-packaging.

---

### 4.2 Coupling Balance

**Score: 6 | Band: Adequate | Confidence: High**

All inter-package couplings are model-strength at local distance: balanced. The `spotClient` interface duplication (F1) is intentional Go idiom but creates silent drift risk. The AWS config duplication across `score.go` and `liveprice.go` (F2) is the weakest point — two AWS adapter sub-concerns within the same package with no shared factory. No distant high-strength couplings exist.

**Evidence:** E1–E5, F1, F2.

---

### 4.3 Dependency Direction / Cycles

**Score: 9 | Band: Exemplary | Confidence: High**

`go list` shows no import cycles. Direction is strictly: `cmd → internal/mcp → internal/spot → external`. No reverse dependencies. The `//go:embed` data files are contained within `internal/spot/data/`. `internal/spot` does not import `internal/mcp` or `cmd`.

**Evidence:** `go list -f '{{.ImportPath}}: {{join .Imports ","}}' ./...` — clean acyclic graph.

---

### 4.4 Encapsulation

**Score: 6 | Band: Adequate | Confidence: High**

Private interfaces defined close to consumer — good. All four provider interfaces in `client.go` are unexported. The `Advice` struct is exported correctly as the domain model. However:
- `NewWithProviders` (`client.go:105`) does not expose `scoreProvider` or `livePriceProvider` injection — partial constructor.
- `SetLivePriceProvider` is a public setter on a struct (`client.go:111`) — mutable post-construction state.
- `CachedScoreData` and `FreshnessLevel` are exported from `internal/spot/score.go` despite being implementation details of the score cache — these leak internal abstractions.

**Evidence:** `client.go:105–113`, `score.go:30–55` (`CachedScoreData`, `FreshnessLevel` exported).

---

### 4.5 Blast Radius / Change Locality

**Score: 6 | Band: Adequate | Confidence: Medium**

Adding a new output format requires touching only `cmd/spotinfo/main.go`. Adding a new MCP tool requires `internal/mcp/server.go` + a new tools file. Adding a new AWS data enrichment requires `internal/spot/client.go` + a new file. The risk is that `main.go` is the blast point for any CLI change. Git churn confirms `Makefile` (19 commits) and `README.md` (17) lead churn — these are infra/docs changes, not domain changes. Domain churn is moderate and localized.

Historical churn evidence: `public/spot/info.go` has 11 commit touches but is deleted at HEAD — the package was restructured from `public/spot` to `internal/spot` during the project's lifetime, a significant change-locality event that is now complete.

**Evidence:** git churn analysis, `go list` dependency graph.

---

### 4.6 Testability

**Score: 7 | Band: Good | Confidence: High**

Test-to-production ratio of 2.43x is strong. All major integration points are behind interfaces and mocked with mockery. `execMainCmd` is extracted from `main()` to accept `spotClient` interface — correct seam. `internal/mcp` has benchmark tests, race tests, SSE transport tests, stdio transport tests. 

Gaps:
- `NewWithProviders` does not inject `scoreProvider` or `livePriceProvider`, limiting unit test control over those sub-concerns without using `SetLivePriceProvider` setter (E9, E10).
- `go vet` reports context cancel leak in `data_test.go:27,77` — production code is fine, but test hygiene is weak.
- `tools.go:parseParameters` uses `map[string]interface{}` — argument parsing is tested but untyped, so wrong argument types pass at compile time.

**Evidence:** `go vet` output (E15), `client.go:105` (E9), `client.go:111` (E10), loc table (test ratio).

---

### 4.7 Architecture Fitness

**Score: 4 | Band: Problematic | Confidence: Medium**

No executable fitness checks exist. No import boundary rules (e.g., via `go-build-tag`, `depguard`, or similar). The golangci config disables `gochecknoglobals` explicitly (`golangci.yaml` — "Too restrictive for CLI apps"). Global variables `mainCtx` and `log` in `main.go` are unguarded. No CI-enforced contract that `cmd` must not reach AWS SDK directly (currently true but not enforced). No fitness function over the `internal/spot` catch-all growth. The `reassign` linter is enabled (golangci.yaml) which catches one class of global-state mutation, but architectural boundaries are purely conventional.

**Evidence:** `.golangci.yaml` (gochecknoglobals disabled), absence of any `internal/arch_test.go` or equivalent, `go list` graph not encoded as a test.

---

## 5. Coverage Gaps and Low-Confidence Areas

1. **No `go test -race` run during review** — race tests exist (`internal/mcp/race_test.go`, 521 lines) but were not executed. Concurrency in `enrichMissingPrices` (`liveprice.go`) uses `sync.WaitGroup + sync.Mutex` — pattern looks correct but not verified under race detector.

2. **AWS credential environment not available** — could not exercise `awsScoreProvider` or `awsLivePriceProvider` live paths. Score assignment bug (F4) is read from source, not reproduced.

3. **golangci-lint v2 not run** — lint configuration reviewed but linter not executed; `staticcheck` ran clean. May be additional style findings.

4. **Size appropriateness** — this is a small CLI+MCP tool (~2,663 production LoC). Most findings are design smells rather than structural failures. The architecture is appropriate for the size and use case; the issues found are refinements, not rebuilds.

---

## 6. Summary of Findings

| ID | Severity | File:line | Description | BC Dimension |
|----|----------|-----------|-------------|--------------|
| F1 | Low | `main.go:159`, `mcp/server.go:26` | Duplicate `spotClient` interface definition — silent drift risk | Coupling balance |
| F2 | Low | `score.go:70–85`, `liveprice.go:60–75` | Dual AWS config loading — no shared adapter factory | Modularity / blast radius |
| F3 | Medium | `score.go:75–82` | Silent fallback to `mockScoreProvider` in production — fake scores with no visible signal | Correctness (encapsulation of error) |
| F4 | Medium | `score.go:155–165` | Score assignment loop assigns first AWS result to all instance types in batch | Correctness (testability gap) |
| F5 | Low | `main.go:167,398` | `execMainCmd` suppressed for cyclop/funlen — 712-line god file | Modularity / cohesion |
| F6 | Low | `data_test.go:27,77` | `context.WithTimeout` cancel leaked in tests | Testability hygiene |
| F7 | Low | `score.go:30–55` | `CachedScoreData`, `FreshnessLevel` exported — implementation detail leak | Encapsulation |
| F8 | Low | `client.go:105` | `NewWithProviders` skips `scoreProvider`/`livePriceProvider` — incomplete test constructor | Testability |

---

## 7. Overall Assessment

**Overall Band: Adequate (5–6)**

spotinfo is a small, well-structured CLI+MCP tool. Dependency direction is clean, no cycles, and test coverage is generous. The main architectural concerns are:

1. **`internal/spot` is under-partitioned** — 7 files, 1,433 LoC, covering advisor data, pricing data, live-price enrichment, placement scoring, types, and domain client in a single package. For this size it is manageable but grows awkward as features add.

2. **Silent mock fallback** (F3) is a real risk for users running without AWS credentials who expect placement scores — they receive deterministic hash-based fakes with no indication.

3. **Score assignment bug** (F4) means multi-instance-type score queries likely return the same score for all types in a batch, silently.

4. **No architectural fitness enforcement** — boundaries are conventional, not enforced by tests or lint rules.

For a tool of this size and maturity (~30 commits, one developer), these are the right problems to have. The foundation is clean. The two correctness findings (F3, F4) deserve attention before the scoring feature is advertised as production-ready.
