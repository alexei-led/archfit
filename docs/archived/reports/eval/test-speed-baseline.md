# Test + Lint Speed Baseline

Measured 2026-06-25 on darwin arm64 (Apple Silicon). All timings wall-clock from `time`.
Warm = test binary cache hot; cold = first run after a clean build or gcflags change.

## Baseline (before Task 13)

Measured before any changes in this task.

| Command | Cold (wall) | Warm (wall) |
| ------- | ----------- | ----------- |
| `go test -race ./... -count=1` | ~23s | ~16s |
| `golangci-lint run -c .golangci.yaml ./...` | ~1.3s | ~1.3s |

### Per-package breakdown (warm, before)

Slowest packages (by wall time):

| Package | Time |
| ------- | ---- |
| `cmd/archfit` | 4.5s |
| `internal/extract/astgrep` | 3.9s |
| `internal/extract/clones` | 3.9s |
| `internal/baseline` | 3.4s |
| `internal/config` | 3.4s |

### Root cause analysis

- 53 packages × ~1.3–1.5s binary startup cost (Go race runtime + dynamic linker) = structural floor.
- Packages run in parallel (observed ~170% CPU), so wall time is ~13–16s not 53×1.5s.
- **cmd/archfit:** 3.27s of actual serial test execution (135 tests, 0.37s slowest single test) + 1.3s startup = 4.5s. Parallelizing within the package was the main lever.
- **config, astgrep, clones, etc.:** <0.1s actual test execution; the 2.5–3.9s is entirely binary startup. No savings from t.Parallel() inside these packages.
- **sg integration tests** (astgrep): 0.16s total for 2 tests; minor. Gated behind -short for subprocess-free loop.
- **time.Sleep in llm_test.go:** 200ms server-handler sleep was blocking srv.Close() ~180ms on cleanup. Converted to context-aware select.
- **No time.Sleep / busy-waits** anywhere else in tests (grep confirmed).

## After Task 13 changes

| Command | Warm (wall) | Delta |
| ------- | ----------- | ----- |
| `go test -race ./... -count=1` | ~14.7s | **−1.3s (−8%)** |
| `go test -race -short ./... -count=1` (make test-fast) | ~14.7s | same as full (sg tests only 0.2s total) |
| `golangci-lint run -c .golangci.yaml ./...` | ~1.3s | no change |

### cmd/archfit package alone

| | Before | After |
| -- | ------ | ----- |
| Wall time (warm) | 4.5s | 2.5s |
| Delta | | **−2.0s (−44%)** |

## Changes made

### 1. `t.Parallel()` in cmd/archfit test files

Added to all top-level test functions in 14 test files. The `Run(args, buf)` function takes output
as a parameter (no `os.Stdout` mutation) and args as a slice (no `os.Args` mutation), making all
tests safe to parallelize.

**Cannot parallelize (documented):**

- `TestLoadDotEnv` — uses `t.Setenv` (env var mutation)
- `TestLoadDotEnv_EmptyEnvVarWins` — uses `t.Setenv`

Also added `t.Parallel()` to table subtests in `TestRun_Check_RequireToolsHardGate`,
`TestRun_Check_RootDecoupledFromConfig`, `TestParseEnrichResponse`, `TestRun_Score_FullFlagParses`,
and other table-driven tests.

**Not parallelized in other packages (documented reason):**

- `internal/config` (0.1s in-test time, no wall benefit)
- `internal/extract/astgrep` (<0.01s unit tests; sg integration is gated by -short)
- All other packages: binary startup dominates; in-test time <50ms — no wall benefit from adding t.Parallel().

### 2. llm_test.go: context-aware handler

`TestAnthropic_Timeout` server handler converted from `time.Sleep(200ms)` to:

```go
select {
case <-time.After(200 * time.Millisecond):
case <-r.Context().Done():
    return
}
```

The client's 20ms timeout fires, the connection closes, the server handler unblocks immediately via
`r.Context().Done()` rather than blocking srv.Close() cleanup for ~180ms.

### 3. sg integration tests gated behind -short

`TestSyntaxIntegration_JSONShape` and `TestSyntaxIntegration_AllRuleFiles` skip when `-short` is
passed. Verified: both show `SKIP` with reason in verbose output.

### 4. make test-fast target

Added to `Makefile`:

```
go test -race -short ./...
```

Skips subprocess integration tests for a subprocess-free inner loop. Full `make test` (without
-short) continues to run everything — CI uses `make test`.

### 5. Lint: no changes

`golangci-lint` runs at 1.3s warm. Every enabled linter finished in that time; no single linter
dominated. Default profile unchanged. No CI-only profile added.

## No sleeps/polling elsewhere

`grep -r "time.Sleep" ./internal/ ./scripts/` confirmed: only the one converted handler in
`llm_test.go`. No busy-waits or polling loops in tests.

## Verification

All gates passed after changes:

- `go test -race ./... -count=1` → 53/53 ok
- `go test ./internal/engine/ -run TestGolden` → ok
- `go test ./internal/ -run TestArchImports` → ok
- `golangci-lint run -c .golangci.yaml ./...` → 0 issues
- `make archfit` → verdict: PASS (exit 0)
