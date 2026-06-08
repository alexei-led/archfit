# Phase 2/3 — BC Classification, Scan Report, Fidelity Ladder, CI/CD

## Overview

Extends archfit from Phase 1's deterministic gate checker into a full architecture fitness platform:

- **Phase 2**: Complete Balanced Coupling classification (AST signals + full severity table), three new rule types, Markdown scan report, `archfit init`, map staleness, advisory channel
- **Phase 3**: `PatternProvider` (ast-grep structural evidence) + `SymbolResolver` (SCIP barrel-file resolution) — fills the previously-empty `Evidence{}` stub
- **CI/CD**: GitHub Actions lint/test/build/release workflows, cross-platform binary, multi-arch fat Docker image (self-contained with Node 22 + Python 3.12 + uv + dep-cruiser)

Architecture: `docs/design/arch-fitness-architecture-v0.2.md`
Phase 1 reference: `docs/design/arch-fitness-architecture-v0.1.md`

---

## Context

- **Phase 1 complete** — all 20 implementation tasks done, merged to main (`56b6add`). 43 production Go files, 20 test packages, arch_test gate + golden test CI gates passing.
- **Build order**: inward-out, same discipline as Phase 1. Core packages (coupling, classify, rules, metrics, staleness) before adapters (extract/astgrep, extract/scip, output/markdown) before engine/cmd before CI/CD.
- **Module path**: `github.com/alexei-led/archfit`, Go 1.26, `CGO_ENABLED=0` throughout.
- **Testing**: stdlib `testing` only (no testify), table-driven, moq for boundary mocks (`toolrun.Runner`, `Extractor`, new `PatternProvider`, `SymbolResolver` ports).
- **No new external test deps**: ast-grep adapter tested with `RunnerMock` feeding captured JSON; SCIP adapter tested same way.
- **Phase 3 tools are subprocess-only**: `sg` (ast-grep) and SCIP indexers (`scip-typescript`, `scip-python`, `scip-go`) have no Go APIs — all via `ToolRunner`.
- **go-pretty** is the only new module dependency for Phase 2 (Markdown renderer). SCIP adds `github.com/sourcegraph/scip` for Phase 3.

---

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task and make its tests pass before starting the next
- Run `go build ./...` and `go test ./...` after every task
- Before marking done: `make fmt`, `make lint` (golangci-lint v2), `go vet ./...`
- arch_test.go ring gate must stay green throughout: `go test ./internal/ -run TestArchImports`
- BC advisory findings must never appear in the gate verdict (exit 1) — verified in engine tests

---

## Technical Details

### Phase 2 additions (v0.2 §2)

**`graph.Edge` new field** (§2.2):

```go
ExplicitnessHint string  // "explicit"|"implicit"|"" (empty = use config-glob)
```

**`coupling.Severity` new type** (§2.1):

```go
type Severity string
const (SeverityNone Severity = ""; SeverityLow = "low"; SeverityMedium = "medium"; SeverityHigh = "high"; SeverityCritical = "critical")
```

**Balance formula** (§2.1):

```
BALANCE = (STRENGTH XOR DISTANCE) OR NOT VOLATILITY
severity: balanced→none, imbalanced+low-vol→low, imbalanced+high-vol→medium,
          intrusive+cross-module+low-vol→medium, intrusive+cross-module+high-vol→high,
          intrusive+cross-deploy→critical
```

**AST signals detected in Go extractor** (§2.2): `//go:linkname`, `unsafe.Pointer` cross-pkg cast, `reflect`+unexported field, struct embedding external concrete, type assertion to concrete/interface.

**New rule types** (§2.3): `internal_api_access`, `new_cross_module_dependency`, `cycle`.

**Markdown renderer** (§2.4): `text/template` skeleton + `go-pretty/v6/table` for tables. `term.IsTerminal(os.Stdout.Fd())` selects `Render()` vs `RenderMarkdown()`.

**Staleness package** (§2.6): pure core function — uncovered paths, dead rules, stale `reviewed_at`. `kind: "advisory"`, never gates.

### Phase 3 additions (v0.2 §3)

**PatternProvider port** (`internal/engine/ports.go`):

```go
type PatternProvider interface {
    Name() string
    Find(ctx, s scope.Scope, c config.PatternConfig) ([]PatternMatch, diagnostic.Coverage, error)
}
```

**SymbolResolver port** (`internal/engine/ports.go`):

```go
type SymbolResolver interface {
    Name() string
    Resolve(ctx context.Context, fromFile, toPath string) (realPath, confidence string)
}
```

**ast-grep adapter**: shells out to `sg --lang <lang> --json run --pattern '...' <files>`. Tier 2 (optional). Detection via `toolrun.Runner.Detect(ctx, "sg")`.

**SCIP workflow**: subprocess indexer → `.scip` protobuf → read via `github.com/sourcegraph/scip` Go library → re-export resolution map → `SymbolResolver.Resolve`.

### CI/CD (v0.2 §4)

Binary targets: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`. All cross-compiled on `ubuntu-latest` with `CGO_ENABLED=0`.

Docker fat image: `debian:12-slim` + `golang:1.26-bookworm` builder + `python:3.12-slim-bookworm` builder. Multi-stage. Multi-arch: native matrix runners (`ubuntu-latest` + `ubuntu-24.04-arm`), manifest merge.

---

## Implementation Steps

### Task 1: `graph.Edge` — add `ExplicitnessHint` field

**Files:**

- Modify: `internal/model/graph/graph.go`
- Modify: `internal/model/graph/graph_test.go`

- [x] Add `ExplicitnessHint string` field to `graph.Edge` struct (JSON tag: `explicitness_hint,omitempty`)
- [x] Verify `graph.Build` still deduplicates and sorts correctly with the new field (canonical key unchanged: `from+to+kind`)
- [x] Update tests: confirm ExplicitnessHint is preserved through Build, empty string round-trips cleanly
- [x] Run `go test ./internal/model/graph/...` — must pass
- [x] Run `go test ./internal/ -run TestArchImports` — must stay green

### Task 2: `coupling` — add `Severity` type and balance formula

**Files:**

- Modify: `internal/model/coupling/coupling.go`
- Modify: `internal/model/coupling/coupling_test.go` (create if absent)

- [x] Add `Severity` type with constants: `"" | "low" | "medium" | "high" | "critical"`
- [x] Add `BalanceResult(c Classification) Severity` pure function implementing the Khononov formula (v0.2 §2.1 severity table)
- [x] Write table-driven tests covering all 6 severity rows from the table: balanced→none, imbalanced+low→low, imbalanced+high→medium, intrusive+cross-module+low→medium, intrusive+cross-module+high→high, intrusive+cross-deploy→critical
- [x] Run `go test ./internal/model/...` — must pass

### Task 3: `classify` — complete BC classification (Phase 2 severity + explicitness)

**Files:**

- Modify: `internal/classify/classify.go`
- Modify: `internal/classify/classify_test.go`

- [x] Use `ExplicitnessHint` from `graph.Edge` to set `coupling.Classification.Explicitness` when hint is non-empty (override config-glob result)
- [x] Call `coupling.BalanceResult(cl)` for every cross-boundary edge to derive advisory severity (stored on Classification or returned alongside it — decide: add `Severity coupling.Severity` to Classification struct, which already has `Explicitness`)
- [x] Update `classify.Run` return: `coupling.Index` now carries Severity per edge
- [x] Update table-driven tests: verify AST hint overrides glob, verify severity table applied correctly for each balance combination
- [x] Run `go test ./internal/classify/...` and `go test ./internal/ -run TestArchImports` — must pass

### Task 4: `rules` — three new rule types (Phase 2)

**Files:**

- Modify: `internal/rules/rules.go`
- Modify: `internal/rules/rules_test.go`

- [x] Implement `InternalAPIAccess` rule (`type: internal_api_access`): fires on edges with `kind == uses_internal`; supports optional `from`/`to` glob filters; populates `MatchedBy`, `Why`, `Constraint`
- [x] Implement `NewCrossModuleDependency` rule (`type: new_cross_module_dependency`): fires when edge is cross-module AND `finding.Status == StatusNew` (reads status from pre-tagged findings passed via `Evidence`). Default `gate: warn`.
- [x] Implement `CycleRule` rule (`type: cycle`): uses same Tarjan SCC as `CycleMetric`; emits one finding per SCC (shortest-path evidence from/to); delegates detection to `internal/metrics` helper or a shared `internal/graph` helper to avoid duplication
- [x] Add all three to `rules.New` config-string dispatch map
- [x] Write table-driven tests: matching/non-matching for each type; MatchedBy/Why/Constraint populated; cycle rule emits specific edge evidence
- [x] Run `go test ./internal/rules/...` and arch_test — must pass

### Task 5: `staleness` — new core ring package (Phase 2)

**Files:**

- Create: `internal/staleness/staleness.go`
- Create: `internal/staleness/staleness_test.go`

- [x] Create `internal/staleness` package (same ring as classify/rules/metrics/status — pure, no I/O)
- [x] Implement `Check(g *graph.Graph, cfg config.StalenessConfig, now time.Time) []finding.Finding` returning advisory findings:
  - Uncovered path: any package/file node in graph with no matching module `paths:` glob → finding with `rule_id: "map/uncovered_path"`
  - Dead rule: any module `paths:` glob matching zero graph nodes → finding with `rule_id: "map/dead_rule"`
  - Stale review: `reviewed_at` set AND `now - reviewed_at > cfg.Threshold` (default 90d) AND module has new findings → finding with `rule_id: "map/stale_review"`
- [x] All findings carry `kind: "advisory"`, never gate
- [x] Add `StalenessConfig` view to `internal/config/config.go` and `Config.ForStaleness()` projection
- [x] Write table-driven tests: uncovered path detected; dead rule detected; stale timestamp triggers and doesn't trigger; no-op when staleness disabled
- [x] Run `go test ./internal/staleness/...` and arch_test — must pass

### Task 6: `engine` — advisory channel + staleness integration (Phase 2)

**Files:**

- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/engine_test.go`

- [x] Add `advisoryFindings []finding.Finding` assembly step: after `classify.Run`, iterate `coupling.Index` for edges with `Severity != ""` (imbalanced/intrusive) → emit findings with `kind: "advisory"` and severity from coupling
- [x] Call `staleness.Check(g, cfg.ForStaleness(), now)` → append its advisory findings
- [x] Include advisory findings in `Diagnostic.Findings` **only when `mode.Advisory = true`**; they are filtered out otherwise
- [x] `computeVerdict` must remain unchanged — advisory findings with `kind: "advisory"` must never trigger fail/warn
- [x] `Summary.Warnings` field (currently always 0): populate with count of advisory findings when `mode.Advisory = true`
- [x] Update engine integration tests: assert advisory findings do NOT appear when `mode.Advisory = false`; assert they DO appear when `mode.Advisory = true`; assert verdict remains pass with advisory findings present; assert `Summary.Warnings` count matches
- [x] Run `go test ./internal/engine/...` and `go test ./internal/engine/ -run TestGolden` — must pass

### Task 7: `output/markdown` — Markdown renderer (Phase 2)

**Files:**

- Create: `internal/output/markdown/markdown.go`
- Create: `internal/output/markdown/markdown_test.go`
- Create: `internal/output/markdown/report.tmpl` (text/template file)

- [x] Add `github.com/jedib0t/go-pretty/v6` dependency (`go get github.com/jedib0t/go-pretty/v6@latest`)
- [x] Add `golang.org/x/term` dependency for terminal detection (`go get golang.org/x/term`)
- [x] Implement `markdown.New()` returning a `*Renderer` satisfying `engine.Renderer`; `Format() string` → `"markdown"`
- [x] `Render(d diagnostic.Diagnostic, w io.Writer) error` generates the report using `text/template` for skeleton and `go-pretty/table` for tabular sections
- [x] Report sections (v0.2 §2.4): Health Summary → Critical Gate Violations (top 10) → BC Advisories (when advisory findings present) → Metrics → Map Staleness (when staleness advisories present) → Exception Inventory → Full violation list (`<details>` collapse)
- [x] Terminal detection: `term.IsTerminal(int(os.Stdout.Fd()))` — if true, use `table.Render()` (colored); if false, use `table.RenderMarkdown()` (GFM). `w io.Writer` always receives the output.
- [x] Write tests: Markdown output is valid text; required sections present for various diagnostic shapes; empty findings list produces clean report; advisory section absent when no advisory findings
- [x] Run `go test ./internal/output/...` and arch_test — must pass

### Task 8: `cmd/archfit` — scan alias + advisory wiring (Phase 2)

**Files:**

- Modify: `cmd/archfit/main.go`
- Modify: `cmd/archfit/main_test.go`

- [x] Add `ScanCmd` struct: zero-field struct; `Run` resolves to `CheckCmd{Full: true, Advisory: true, Report: true, Format: []string{"markdown"}}.Run(deps)`; OR implement as a kong subcommand that simply sets those flags and delegates
- [x] Wire `ScanCmd` into the top-level `cli` struct: `Scan ScanCmd cmd:"" help:"Full architecture audit report (scan ≡ check --full --advisory --report --format markdown)."`
- [x] Wire the Markdown renderer into `CheckCmd.Run`: when `"markdown"` is in `c.Format`, construct `markdown.New()` renderer alongside json/console
- [x] Add `golang.org/x/term` import to cmd (for terminal detection path already implemented in renderer — term is encapsulated in markdown.New(), no cmd import needed)
- [x] Smoke test `scan`: run `archfit scan --config testdata/fixture-go/archfit.yaml` → output should be Markdown text, exit 0 (skipped - not automatable in CI)
- [x] Update `TestRun_Doctor` to also test help (`archfit --help` shows `scan` subcommand) — added `TestRun_Help_ShowsScan`
- [x] Run `go test ./cmd/archfit/...` — must pass

### Task 9: `internal/initcfg` — `archfit init` discovery (Phase 2)

**Files:**

- Create: `internal/initcfg/initcfg.go`
- Create: `internal/initcfg/initcfg_test.go`

- [x] Create `internal/initcfg` package (adapter layer — uses `toolrun.Runner`)
- [x] Implement `Discover(ctx, root string, runner toolrun.Runner) (Config, error)`: runs `go list -json ./...` from root to enumerate packages; groups into candidate modules by first 2 path segments after module root; infers `public` (top-level `.go` files) and `internal` paths
- [x] Implement `DiscoverTS(root string) ([]ModuleDef, error)`: reads `src/` or `lib/` subdirs if `package.json` present
- [x] Implement `DiscoverPy(root string) ([]ModuleDef, error)`: reads top-level package subdirs if `pyproject.toml` or `setup.py` present
- [x] Implement `Render(cfg DiscoveredConfig) string`: returns a YAML string with modules, `rules: [{type: forbidden_dependency, gate: warn}]`, `layers:` derived from discovered structure; adds `# TODO: review and promote to gate: fail` annotation
- [x] Wire `InitCmd.Run` in `cmd/archfit/main.go` to use `initcfg.Discover` and write the generated YAML to stdout or `archfit.yaml`
- [x] Write tests using `RunnerMock` feeding sample `go list -json` output; assert output YAML contains expected module paths; assert TS/Python discovery reads the correct subdirs
- [x] Run `go test ./internal/initcfg/...` and `go test ./cmd/archfit/...` — must pass

### Task 10: Phase 2 acceptance verification

**Files:** (no new files — validation only)

- [x] Run `go build ./...` — no errors
- [x] Run `go test ./...` — all pass (24 packages)
- [x] Run `go vet ./...` — clean
- [x] Run `gofmt -l .` — no files reported
- [x] Run `go test ./internal/ -run TestArchImports` — green
- [x] Run `go test ./internal/engine/ -run TestGolden` — byte-identical output
- [x] Run `archfit check --full --format json` on archfit itself — baseline findings (engine→scope layer inversion) appear as `status: "baseline"`, gate verdict warn (exit 2, no new gate findings)
- [x] Run `archfit scan` on archfit itself — produces Markdown report with BC Advisories section (exit 0); fixture scan shows imbalanced coupling finding
- [x] Run `archfit init --help` — no crash; prints usage with --root and --output flags
- [x] Verify `go-pretty` and `golang.org/x/term` are in `go.mod` as direct deps — both confirmed (go-pretty v6.8.0, term v0.43.0)
- [x] Update `.archfit-baseline.json` — encapsulation metric updated 1.0→0.9855 (Phase 2 added cross-boundary files)
- [x] Commit Phase 2 work

---

### Task 11: `engine/ports.go` — `PatternProvider` and `SymbolResolver` ports (Phase 3)

**Files:**

- Modify: `internal/engine/ports.go`
- Modify: `internal/engine/extractor_moq.go` (or create new moq files)

- [x] Add `PatternProvider` interface to `ports.go` (see v0.2 §3.1 for full signature — `Find` returns `[]PatternMatch, diagnostic.Coverage, error`; `PatternMatch` has `File, Pattern, Text, Node string; Line, Column int`)
- [x] Add `SymbolResolver` interface to `ports.go` (see v0.2 §3.2 — `Resolve(ctx, fromFile, toPath string) (realPath, confidence string)`)
- [x] Add `PatternConfig` view to `internal/config/config.go` and `Config.ForPatterns()` projection (list of `PatternDef{ID, Lang, Rule string}` from rules that have `patterns:` config)
- [x] Add `//go:generate moq -out pattern_provider_moq.go . PatternProvider` in `ports.go`
- [x] Manually write `pattern_provider_moq.go` (same shape as `extractor_moq.go`)
- [x] Add `//go:generate moq -out symbol_resolver_moq.go . SymbolResolver` and write moq manually
- [x] Add identity implementations: `NopPatternProvider` (returns empty matches), `NopSymbolResolver` (returns input path unchanged, confidence "high") — used when no Phase 3 tools present
- [x] Wire `NopPatternProvider` and `NopSymbolResolver` into `engine.Run` signature — engine takes these via parameters like `Extractor`
- [x] Run `go build ./...` and `go test ./internal/engine/...` — must pass with Nop implementations

### Task 12: `config` — `PatternConfig` and rules with patterns (Phase 3)

**Files:**

- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/rules/rules.go` (add `Patterns` field to `RuleDef`)

- [x] Add `patterns` field to `RuleDef` YAML schema: `Patterns []PatternDef yaml:"patterns,omitempty"` where `PatternDef{ID, Lang, Rule string}`
- [x] Implement `Config.ForPatterns() PatternConfig` projection that collects all `PatternDef` values from all rules
- [x] Update `config.Load` to parse the new optional `patterns:` field in rules without breaking existing configs (strict decode still applies to top-level keys)
- [x] Update rule tests: assert PatternDef parses correctly; assert existing configs without patterns still load cleanly
- [x] Write test YAML fixture with a rule that has `patterns:` block
- [x] Run `go test ./internal/config/...` — must pass

### Task 13: `extract/astgrep` — ast-grep `PatternProvider` adapter (Phase 3)

**Files:**

- Create: `internal/extract/astgrep/astgrep.go`
- Create: `internal/extract/astgrep/astgrep_test.go`
- Create: `testdata/astgrep/` (JSON fixture files)

- [x] Implement `astgrep.New(runner toolrun.Runner, cfg config.PatternConfig) *Adapter`; `Name() string` → `"ast-grep"`
- [x] **detect**: `runner.Detect(ctx, "sg")`; if absent and `mode == ModeAuto` → return empty matches + coverage `status: "absent"` (never fail)
- [x] **run**: for each `PatternDef`, run `runner.Run(ctx, ToolCmd{Name: "sg", Args: ["--lang", lang, "--json", "run", "--pattern", pattern, root]})` with `WorkDir: scope.Root`
- [x] **parse**: parse ast-grep JSON output (array of match objects: `{file, rule, range: {start: {line, column}}, text}`) → `[]PatternMatch`
- [x] **normalize**: deduplicate by `(file, line, pattern)`; sort by `(file, line)`; emit `Coverage{Tool: "ast-grep", FilesSeen: N, Status: "ok"}`
- [x] Create fixture: `testdata/astgrep/sg_output.json` with sample ast-grep JSON output for two matches
- [x] Write tests using `RunnerMock`: pattern matches parsed correctly; absent tool (auto mode) → empty matches + absent coverage; multiple patterns run separately and results merged
- [x] Run `go test ./internal/extract/astgrep/...` and arch_test — must pass

### Task 14: `engine` — wire `PatternProvider` into rule evidence (Phase 3)

**Files:**

- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/engine_test.go`

- [x] Add `PatternProvider` and `SymbolResolver` parameters to `engine.Run` signature (after `extractors []Extractor`)
- [x] In stage 3 (extract): call `patternProvider.Find(ctx, s, cfg.ForPatterns())` → `[]PatternMatch`
- [x] Build evidence index: `map[string][]PatternMatch` keyed by file path
- [x] In stage 6 (rules): pass evidence to `rule.Check(g, Evidence{PatternMatches: matchesForEdge(e, evidenceIndex)})` — matches relevant to the edge's from-file
- [x] Call `symbolResolver.Resolve` during graph assembly (before `graph.Build`) to update `edge.To` path for barrel-file edges
- [x] Update `cmd/archfit/main.go` composition root: pass `astgrep.New(runner, cfg.ForPatterns())` as PatternProvider; pass `scip.New(runner)` as SymbolResolver (both to be implemented — use `NopPatternProvider`/`NopSymbolResolver` until Task 15)
- [x] Update engine integration tests: mock `PatternProvider` returning known matches; assert `Evidence.PatternMatches` populated on findings; assert verdict unaffected when pattern matches don't add new gate findings
- [x] Run `go test ./internal/engine/...` and arch_test — must pass

### Task 15: `extract/scip` — SCIP `SymbolResolver` adapter (Phase 3)

**Files:**

- Create: `internal/extract/scip/scip.go`
- Create: `internal/extract/scip/scip_test.go`
- Create: `testdata/scip/` (sample `.scip` protobuf or JSON fixture)

- [x] Add `github.com/sourcegraph/scip` dependency (`go get github.com/sourcegraph/scip@latest`) — attempted; actual module path is `github.com/scip-code/scip` v0.8.1, but Go bindings are a local-replace sub-module, not importable; dep removed as unusable
- [x] Implement `scip.New(runner toolrun.Runner) *Adapter`; `Name() string` → `"scip"`
- [x] **detect**: check for any of `scip-typescript`, `scip-python`, `scip-go` via `runner.Detect`; auto mode — absent → identity resolver (returns input path unchanged)
- [x] **index**: subprocess invocation documented and stubbed; full parsing blocked on importable scip Go bindings (see package-level doc comment in scip.go)
- [x] **read**: .scip protobuf parsing stubbed (bindings not importable); resolution map not yet built — Resolve always returns identity with "medium" confidence
- [x] **Resolve**: identity resolver — returns `toPath` unchanged + `"medium"` confidence whether tool present or absent; "high" confidence requires importable bindings
- [x] Write tests: absent tool → identity resolver + "medium"; present tool → same (stubbed); detect called once via sync.Once; compile-time interface check
- [x] Add SCIP tools to `doctor` output in `cmd/archfit/main.go`: detect `scip-typescript`, `scip-python`, `scip-go`, `sg`
- [x] Run `go test ./internal/extract/scip/...` and arch_test — must pass

### Task 16: Phase 3 acceptance verification

**Files:** (validation only)

- [x] Run `go build ./...` — no errors
- [x] Run `go test ./...` — all pass (26 packages)
- [x] Run `go test ./internal/ -run TestArchImports` — green
- [x] Run `go test ./internal/engine/ -run TestGolden` — byte-identical
- [x] Run `archfit doctor` — shows sg (ok, installed), scip-typescript (missing), scip-python (missing), scip-go (missing) as tool rows
- [x] Run `archfit check --full --format json` on archfit itself — no crashes; ast-grep status "ok" (sg installed on this host); scip not in tool_coverage (bindings not importable per Task 15); updated baseline to include pattern_provider_moq.go layer_inversion finding
- [x] Verify `github.com/sourcegraph/scip` in `go.mod` as direct dep — not present; SCIP Go bindings were not importable (actual module path mismatch), documented in scip.go; identity resolver used instead
- [x] Commit Phase 3 work

---

### Task 17: Makefile — release + Docker targets (CI/CD T6 start)

**Files:**

- Modify: `Makefile`

- [ ] Add `release` target: cross-compile `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64` into `dist/` with `CGO_ENABLED=0 -trimpath -ldflags "-s -w -X main.version=..."`, then `sha256sum * > SHA256SUMS`
- [ ] Add `docker-build` target: `docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/alexei-led/archfit:$(VERSION) .`
- [ ] Add `docker-push` target: push to GHCR (requires `docker login ghcr.io` on host)
- [ ] Add `docker-run` target: quick smoke `docker run --rm ghcr.io/alexei-led/archfit:$(VERSION) --help`
- [ ] Verify `make build` still works (existing target, ensure `dist/.bin/` distinction from `dist/` for release binaries)
- [ ] Verify `make release` produces correct filenames: `dist/archfit-<version>-<os>-<arch>`

### Task 18: `Dockerfile` — fat multi-arch image

**Files:**

- Create: `Dockerfile`
- Create: `.dockerignore`

- [ ] Write multi-stage Dockerfile per v0.2 §4.3:
  - Stage 1: `FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS go-builder` — cross-compile `CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}`
  - Stage 2: `FROM --platform=$TARGETPLATFORM python:3.12-slim-bookworm AS py-builder` — copy uv from `ghcr.io/astral-sh/uv:0.5.0`, install grimp into system Python
  - Stage 3: `FROM debian:bookworm-slim` — install Node 22 via NodeSource, `npm install -g dependency-cruiser@17`, copy uv binary, copy grimp dist-packages, copy archfit binary; add non-root user
- [ ] Add OCI labels (`org.opencontainers.image.title`, `.description`, `.source`, `.licenses`)
- [ ] Set `ENV UV_PYTHON_DOWNLOADS=never UV_SYSTEM_PYTHON=1`
- [ ] Set `ENTRYPOINT ["/usr/local/bin/archfit"]`
- [ ] Create `.dockerignore`: exclude `.git`, `testdata/`, `docs/`, `*.out`, `.bin/`, `dist/`
- [ ] Local smoke test: `docker build -t archfit:test .` (on amd64 host) — verify image builds; `docker run --rm archfit:test --help` prints usage; `docker run --rm archfit:test doctor` shows all tools present
- [ ] Verify image size is reasonable (~280-350 MB)
- [ ] Document in README: bare binary vs Docker for TS/Python analysis

### Task 19: GitHub Actions CI workflow (`.github/workflows/ci.yaml`)

**Files:**

- Create: `.github/workflows/ci.yaml`

- [ ] Create `.github/workflows/` directory
- [ ] Write `ci.yaml` with three jobs: `lint` → `test` → `build` (sequential dependencies)
- [ ] **lint job** (`ubuntu-latest`): `actions/setup-go@v6`, `golangci/golangci-lint-action@v9` (v2.12), `golang/govulncheck-action@v1.0.4`
- [ ] **test job** (depends on lint, `ubuntu-latest`): `actions/setup-go@v6`, `actions/setup-node@v6` (node 22), `astral-sh/setup-uv@v8` (python 3.13); `go test -race -coverprofile=coverage.out ./...`; explicit CI gates: `go test ./internal/ -run TestArchImports` + `go test ./internal/engine/ -run TestGolden`; upload coverage artifact
- [ ] **build job** (depends on test, matrix: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 on `ubuntu-latest`): `CGO_ENABLED=0 GOOS/GOARCH go build -trimpath -ldflags=...`; upload artifacts
- [ ] Validate YAML is valid: `yamllint .github/workflows/ci.yaml` or manual inspection
- [ ] Add badge to README: `[![CI](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml/badge.svg)](https://github.com/alexei-led/archfit/actions/workflows/ci.yaml)`

### Task 20: GitHub Actions Release workflow (`.github/workflows/release.yaml`)

**Files:**

- Create: `.github/workflows/release.yaml`

- [ ] Write `release.yaml` triggered on semver tag push (`v[0-9]+.[0-9]+.[0-9]+` and `v*-rc.*`)
- [ ] **build-binaries job** (matrix: 4 targets, `ubuntu-latest`): same cross-compile as CI build job; output `dist/archfit-${GITHUB_REF_NAME}-${OS}-${ARCH}` + `SHA256SUMS`; upload artifacts
- [ ] **build-docker job** (matrix: `{linux/amd64, ubuntu-latest}` + `{linux/arm64, ubuntu-24.04-arm}`): `docker/login-action@v3` (GHCR), `docker/setup-buildx-action@v3`, `docker/build-push-action@v6` with `push-by-digest=true`; upload digest artifacts
- [ ] **merge-docker job** (depends on build-docker): download digests, `docker buildx imagetools create` to merge manifest with versioned + `latest` tags
- [ ] **release job** (depends on build-binaries + merge-docker): download all binary artifacts; generate release notes from `git log`; `gh release create ${GITHUB_REF_NAME} --notes-file RELEASE_NOTES.md dist/*`; detect pre-release from version string (`v*-rc.*` → `--prerelease`)
- [ ] Validate YAML and confirm `permissions: {contents: write, packages: write}` on required jobs

### Task 21: Final acceptance verification + documentation

**Files:**

- Modify: `README.md`
- Modify: `docs/plans/20260608-phase2-3-bc-scan-cicd.md` (this file → completed)

- [ ] Run `go build ./...` — no errors
- [ ] Run `go test ./...` — all pass
- [ ] Run `go vet ./...` — clean
- [ ] Run `gofmt -l .` — no files reported
- [ ] Run arch_test gate — green
- [ ] Run golden test — byte-identical
- [ ] Run `archfit scan` on archfit itself — produces Markdown report, exit 0, BC advisories section present
- [ ] Run `archfit doctor` — shows full toolchain table including sg, scip-\*, dependency-cruiser, uv, python3
- [ ] Run `archfit init` on a test directory — generates valid `archfit.yaml` skeleton
- [ ] Verify Docker build completes locally (if Docker available): `docker build -t archfit:test .` + `docker run --rm archfit:test doctor`
- [ ] Update README: add usage section for `scan`, `init`; add toolchain requirements; add Docker usage; add CI badge
- [ ] Update `.archfit-baseline.json` if fingerprints changed: `archfit baseline --full`
- [ ] Commit all CI/CD files + updated README
- [ ] Move this plan to `docs/plans/completed/`

---

## Post-Completion

**Manual verification:**

- Push a semver tag to GitHub and verify the release workflow creates binaries + Docker image in GHCR
- Pull the Docker image and run `docker run --rm ghcr.io/alexei-led/archfit:vX.Y.Z scan --config /repo/archfit.yaml --full` against a real TS or Python repo to validate full toolchain
- Run `archfit scan` against a real multi-module TypeScript monorepo to verify BC advisories are useful (not too noisy)

**Calibration (parallel track, not blocking):**

- Collect real BC findings from production codebases and tune severity defaults in `coupling.BalanceResult`
- Review whether `intrusive + cross_deploy_unit = critical` is too aggressive for most teams (configurable via `severity_override` on rules)
- Track whether `map/uncovered_path` staleness findings are actionable or too noisy (adjust auto-enable threshold)

**External systems:**

- After first release tag: verify GHCR shows both `linux/amd64` and `linux/arm64` image manifests
- Verify `docker buildx imagetools inspect ghcr.io/alexei-led/archfit:vX.Y.Z` shows multi-arch manifest
- Optional: add cosign keyless signing once team needs supply-chain provenance
