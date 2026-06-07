# Phase 1 — Deterministic Core (Go · TypeScript · Python)

## Overview

Bootstrap the `archfit` binary until it is useful on **Go, TypeScript, and Python** repos:
`archfit check` fails on a real boundary violation **in any of the three languages** and passes after the fix.
Every metric delta is explainable from concrete edge changes.
An AI agent can read a gate finding and apply the repair constraints.

All three import extractors ship together (spec §16, Phase 1) — the tool is not useful single-language. Investment
priority: **TypeScript and Python first** (primary audience); Go last (simplest, native path). Phases 2–4 are sketched
at the end as non-blocking reminders; do not build them until Phase 1 is proven.

Source spec: `docs/spec/arch-fitness-spec-v0.4.md`
Architecture: `docs/design/arch-fitness-architecture-v0.1.md`

---

## Context

- Repo is greenfield — no Go code yet. Task 1 bootstraps the module and skeleton.
- **Module path:** `github.com/alexei-led/archfit` (your GitHub / GHCR org).
- **Go version:** 1.26 (installed toolchain). Use `go 1.26` in `go.mod`.
- **Selected 3rd-party stack (idiomatic Go 1.26, stdlib-first; Perplexity-validated 2026-06-07):** CLI `alecthomas/kong` (typed, no globals — fits design D5) · config `goccy/go-yaml` · globs `bmatcuk/doublestar/v4` · Go analysis `x/tools/go/packages` · tests **stdlib `testing`** (table-driven, no testify) + `matryer/moq` for boundary mocks · logging stdlib `log/slog` (edges only) · errors stdlib `fmt.Errorf %w`. Tooling: Makefile · golangci-lint v2 · gitleaks + pre-commit/pre-push · GitHub Actions CI+Release · GHCR. Rationale in Phase 0 and "Tooling & Automation".
- Architecture style: hexagonal ports-and-adapters pipeline (design §1).
- Build order: import rings inward-out (design §4) — `model/*` first, then `config`/`baseline`, then core stages, then adapters, then `engine` + `cmd`.
- **Package layout follows design §4** (`internal/model/*`, `internal/scope`, `internal/classify`, `internal/status`, `internal/history/git`), which **refines** spec §15's older `internal/graph`/`internal/tools` sketch. Where they differ, design §4 wins.
- **External analyzers (multi-language extraction, design D8):** Go = native `go/packages` (in-proc) · TypeScript = `dependency-cruiser` via `bunx`/`npx` · Python 3.12+ = `grimp` via `uv run` (PEP 723 helper) or `python3.12`. All behind the `Extractor` port + `ToolRunner`; **auto-detected and optional** (missing toolchain → coverage gap + install hint, never a hard fail). Distribution: the pure-Go binary runs the Go path anywhere; full TS/Python analysis needs Node + Python present (see Phase 0, Task T6). Deferred fidelity tiers (ast-grep patterns, tree-sitter native fallback, SCIP/LSP real-target resolution): see Non-Goals.

---

## Phase 0 — Allowed APIs (verified)

Verified against current docs/source on 2026-06-07 (Documentation Discovery). Use these signatures; do **not** invent methods or flags. The anti-patterns listed are confirmed non-existent or wrong — they are the traps to avoid.

### `golang.org/x/tools/go/packages` — native Go extractor (x/tools ≥ v0.45.0)

- **To get import-statement line numbers you MUST load with `NeedName | NeedFiles | NeedImports | NeedSyntax | NeedTypes`.** `pkg.Fset` is populated **only when `NeedTypes` is set**; without it `pkg.Fset.Position(...)` panics on a nil Fset.
- Read positions from the AST, not the `Imports` map:

  ```go
  cfg := &packages.Config{
      Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports |
            packages.NeedSyntax | packages.NeedTypes,
      Dir: rootDir, Context: ctx,
  }
  pkgs, err := packages.Load(cfg, "./...")
  for _, pkg := range pkgs {
      for _, f := range pkg.Syntax {
          for _, imp := range f.Imports {
              pos := pkg.Fset.Position(imp.Pos())
              path := strings.Trim(imp.Path.Value, `"`)
              // pos.Filename, pos.Line, path
          }
      }
  }
  ```

- **Internal-package detection is path-based only** (no API): `strings.Contains(p, "/internal/") || strings.HasSuffix(p, "/internal")`.
- **Anti-patterns (do NOT do):** `NeedImports|NeedFiles|NeedModule` yields no positions (no AST, no Fset); `NeedSyntax` without `NeedTypes` → nil-`Fset` panic; there is **no** `Package.IsInternal` field; the `pkg.Imports` map carries no AST node.
- **Testing:** `packages.Load` shells out to real `go list` — it **cannot be mocked**. Tests use a real `go.mod` module on disk under `testdata/` (or `packages.Config.Overlay` for synthetic files).

### TypeScript extraction — `dependency-cruiser` (adapter, via `bunx`/`npx`)

- **Invoke** (v13+ needs no `--config`), with the target repo as cwd:
  `bunx depcruise <src> --output-type json --ts-config tsconfig.json --include-only "^src" --exclude "^node_modules"`.
  Prefer `bunx` (faster); fall back to `npx`.
- **JSON shape:** top-level `modules[]`; each module has `source` (repo-relative path) and a dependency array
  (`dependencies[]`; some builds name it `deps[]` — handle both).
- **Per dependency edge:** `resolved` (real target path after tsconfig-alias resolution), `module` (raw specifier),
  `dependencyTypes` (`es6`/`cjs`/`type-only`/`npm`…), `dynamic` (bool), `couldNotResolve` (bool), `coreModule` (bool).
- **Edge mapping:** from = `modules[i].source`, to = `dependencies[j].resolved`. `couldNotResolve:true` → unresolved
  (lower confidence, keep the edge); `dynamic:true` → mark; skip `coreModule`.
- **tsconfig & install:** pass `--ts-config <path>` for path aliases/`baseUrl`. Bare/`npm` specifiers need the project's
  `node_modules`; relative + aliased imports (the internal boundaries archfit cares about) resolve without a full install.
- **Detect:** `bunx depcruise --version` (or check `node_modules/.bin/depcruise`); applicability marker = `package.json`.
  **Install hint:** `npm i -D dependency-cruiser` (+ Node).
- **Anti-patterns:** don't require a `.dependency-cruiser.js`; don't assume the array key is `dependencies` (handle
  `deps`); never shell out directly — go through `ToolRunner`.

### Python extraction — `grimp` via `uv` (adapter, bundled PEP 723 helper)

- **Use grimp directly, NOT import-linter** (import-linter is a contract checker built on grimp; archfit has its own
  rules and needs only the graph).
- **Bundled helper** (`go:embed`ed, written to a temp file) with a PEP 723 header so `uv` auto-provisions grimp:

  ```python
  # /// script
  # requires-python = ">=3.12"
  # dependencies = ["grimp"]
  # ///
  import json, grimp
  # g = grimp.build_graph(pkg); for m in g.modules: for d in g.get_import_details(m): ...
  # emit JSON {"edges":[{"importer","imported","line"}], "unresolved": N}
  ```

- **Invoke:** `uv run --directory <project_root> <helper.py> --package <top_pkg>` — uv provisions grimp ephemerally
  (no venv/pre-install). Fallback: `python3.12 <helper.py>` when grimp is already importable.
- **grimp API:** `g = grimp.build_graph(pkg)`; `g.modules` (all module names); `g.get_import_details(importer)` →
  `ImportDetails(importer, imported, line_number)` (statement-level lines). Static AST analysis, no code execution.
- **Minimal env:** internal edges need only the source on `sys.path` (set via `--directory`/cwd) with externals
  excluded (`exclude_external_packages`/`only_internal_packages`) — **no third-party deps installed**.
- **Detect:** prefer `uv --version` (then run via uv); else `python3.12 -c "import sys;print(sys.version_info[:2])"` +
  `python3.12 -c "import grimp"`. Applicability marker = `pyproject.toml`/`setup.py`/a top-level package.
  **Install hints:** `uv` (astral.sh/uv) or `python3.12 -m pip install grimp`.
- **Anti-patterns:** don't use import-linter for the graph; don't require the target's third-party deps for internal
  edges; **ruff/ty are NOT import-graph tools** (Perplexity-confirmed) — don't use them here; go through `ToolRunner`.

### `github.com/alecthomas/kong` — CLI — idiomatic Go 1.26 (Perplexity-validated)

**Decision:** kong over urfave/cli and cobra. For idiomatic Go — explicit data flow, no hidden global state, type safety, no codegen — kong is the strongest fit (Perplexity 2026; Daniel Michaels / Matt Turner write-ups, HN consensus). It is a **typed command struct**, which the design's **D5 (explicit wiring, no global registries)** wants; urfave is stringly-typed (`c.String("flag")`), cobra uses `init()` globals + viper + a generator. (Your fleet uses urfave, but the closest analog — qmd-go — is alpha, so idiomatic wins over fleet-consistency here.)

- Shape: a top-level CLI struct with command structs as fields tagged `cmd:""`; flags/args are **typed fields** (`Base string`, `Full bool`, `Format []string`) — compile-checked, no string keys.
- `kong.Parse(&cli, opts...)` populates the struct and returns a `*kong.Context`; dispatch via `ctx.Run(bindings...)` which calls the selected command's `Run(deps...) error`.
- **Dependency wiring is explicit, not global:** `main` builds the composition root and passes it with `kong.Bind(v)` / `kong.BindTo(impl, (*Iface)(nil))`; bound values inject by type into the matched `Run`. No `init()` registration.
- Repeatable flag: `Format []string` accepts `--format json --format console`; `enum:"json,console"` validates (needs `default`/`required`).
- **Exit codes:** map verdict → exit in **one place** — `main` does `os.Exit(code)` after `ctx.Run()` (commands return a typed error carrying the code; they never call `os.Exit`).

### YAML — `github.com/goccy/go-yaml` (recommended) — config decode

- **Maintenance note (conflicting signals — verify before relying on `yaml.v3`):** `gopkg.in/yaml.v3` was archived, then reportedly un-archived/maintained again. `goccy/go-yaml` is unambiguously maintained, pure-Go YAML 1.2.
- **Why goccy:** strict decode with precise `[line:col] unknown field` errors — important for rejecting typos in `archfit.yaml`. (`yaml.v3` can do strict via `dec.KnownFields(true)` and is the lighter dep if you'd rather — both viable; pick one.)
- Strict decode:

  ```go
  dec := yaml.NewDecoder(r, yaml.DisallowUnknownField())
  err := dec.Decode(&cfg)
  ```

### `github.com/bmatcuk/doublestar/v4` — globs (v4.x)

- Use `doublestar.Match(pattern, name)` (always `/`-separated) for repo-relative paths — **not** `PathMatch` (OS separator). `**` must be a whole path component (`pkg/b/internal/**`).

### Testing / mocking — stdlib `testing` + `matryer/moq` (idiomatic, Perplexity-validated)

- **stdlib `testing`** only: table-driven, `t.Run`, `if got != want { t.Errorf(...) }`; deep compares via `reflect.DeepEqual` / `slices.Equal` / `maps.Equal`. **No testify** — Perplexity 2026: the idiomatic baseline is stdlib testing + small interfaces + minimal fakes; testify is a pragmatic addon, not best practice.
- **`github.com/matryer/moq`** for generated mocks: plain structs with func fields, no expect/assert DSL — stays out of test logic, works directly with `testing`. Generate via `//go:generate moq`.
- **Mock only system boundaries** (your CLAUDE.md rule): `toolrun.Runner` (process boundary) and the `Extractor` port. For trivial interfaces a hand-written fake is equally fine — moq just removes boilerplate. Do **NOT** mock the pure `Rule`, `Metric`, or `Renderer` — table-test them on real sealed values.
- `go/packages` still cannot be mocked (real `go list`) — `extract/golang` is tested against the real `testdata/` module, not a mock.

---

## Development Approach

- **Testing:** stdlib `testing`, table-driven; test externally visible behavior. Mock only boundaries (`toolrun.Runner`, `Extractor`) with `matryer/moq`. No testify.
- Complete each task and make its tests pass before starting the next.
- Run `go build ./...` and `go test ./...` at the end of every task.
- Before marking a task done: `make fmt` (`gofmt -s` + goimports), `make lint` (golangci-lint v2), `go vet ./...`.
- **Tooling sequencing** (Tasks T1–T6, see "Tooling & Automation"): set up the Makefile (T1), golangci-lint v2 config (T2), moq (T3), and pre-commit/pre-push + gitleaks (T4) **with/right after Task 1**, so every later task runs under lint+test from the start. Wire CI (T5) after Task 16; Release + Dockerfile + GHCR (T6) after Task 18.
- Walking-skeleton target: after Task 18 (cmd wiring), `archfit check` on the fixtures produces JSON for all three languages with one rule wired end-to-end. Tasks 1–17 (incl. 11A/11B) build inward-out; nothing is runnable end-to-end until `cmd` is wired.

---

## Non-Goals (Phase 1 — do NOT add)

These are explicitly deferred or cut per design §9 decision records (D6) and the 2026-06-07 simplification pass.
Do not reintroduce without a concrete need:

- Per-consumer graph interfaces (`RuleGraph`, `MetricGraph`) — Go doesn't break callers on additive struct change.
- `Classification`/`ClassificationEvidence` type split — one struct until evidence grows.
- Schema-compatibility gate — a version constant suffices; no external consumers yet.
- JSON round-trip CI gate — guards the deferred plugin protocol.
- `Clock` interface — `cmd` captures `now := time.Now()` once and passes it in.
- `AdvisoryFinding` type — advisory channel deferred past v1.
- Full BC classification (explicitness, connascence, severity-mapping tables) — Phase 2.
- `change_locality` metric (spec §10.4) — Phase 2; it "needs calibration against real tasks."
- `repair_task` finding block + populated `agent_tasks[]` content (spec §13) — Phase 4. Phase 1 findings carry `why` + `constraint` + `alternatives`; the top-level `agent_tasks` field is emitted as an **empty typed slice** to keep `diagnostic.v1` honest.
- Rule types `internal_api_access`, `new_cross_module_dependency`, and a dedicated `cycle` rule (spec §9) — Phase 1 ships only `forbidden_dependency`, `public_api_only`, `forbidden_layer_direction`; **cycle gating rides the cycle-count metric's `gate: fail`, not a rule** (spec §16.1).
- `archfit scan` command (requires markdown renderer + advisory channel) — Phase 2. Phase 1 wires only `check`, `baseline`, `explain`, `doctor`; `init` stubs. Drop `ScanCmd` entirely from Phase 1 cmd wiring.
- **Fidelity tiers above the v1 import adapters** (design D8) — deferred, contracts preserved:
  - `ast-grep` `PatternProvider` (structural rule evidence) — v1 `Evidence` is empty; the three v1 rules are graph-based.
  - `tree-sitter` native fallback (`go-tree-sitter`) — a real later feature, not a freebie; v1 degrades via the coverage channel (missing toolchain → install hint), not a second extractor per language.
  - SCIP indexers (`scip-go/ts/python`) + LSP (gopls/tsgo/pyright) for symbol-level real-target / re-export-shim resolution (spec §17/§18). v1 does best-effort resolution via the adapters themselves.
  - `import-linter` for Python — use `grimp` directly (import-linter is a contract checker on top; we need the graph).
  - `skott`/`madge`/oxc/bun-transpiler for TS — `dependency-cruiser` chosen on fidelity; `bunx` is just the runner.
- SARIF output, Markdown renderer, GitHub Action, MCP server — Phase 4 / Phase 2.
- Plugin protocol (process plugins §8) — deferred.
- `goreleaser` for releases — your fleet (0/14 repos) uses `make release` cross-compile + `gh release create`; match that, not goreleaser (Task T6).
- `depguard` (or any lint rule) for the import-ring boundary — **`arch_test.go` (Task 5) is the authoritative ring gate** (design D6: structure over discipline). golangci-lint stays code-quality only; a second hand-maintained ring definition would drift.
- `stretchr/testify` / assertion libraries — stdlib `testing` only (Perplexity 2026: idiomatic baseline). Add testify only on a concrete, repeated pain point.
- `gofumpt` — `gofmt -s` + `goimports` keep us closer to stdlib; gofumpt is an optional stricter formatter, not needed.

---

## Implementation Steps

### Task 1: Repo bootstrap and skeleton

- [x] `git init /Users/alexei/Workspace/archfit` (if not already a git repo)
- [x] `go mod init github.com/alexei-led/archfit` in repo root; set `go 1.26`
- [x] Add `cmd/archfit/` with a stub `main.go`: a minimal kong CLI struct + `kong.Parse` (or a `--version` flag) that prints version and exits 0
- [x] Add runtime deps (`go get`): `github.com/alecthomas/kong`, `github.com/goccy/go-yaml`, `github.com/bmatcuk/doublestar/v4`, `golang.org/x/tools`. **Tests use stdlib `testing` only** (no testify). Tools (installed via `make setup-tools`, not module deps): `github.com/matryer/moq`, `golang.org/x/tools/cmd/goimports`.
- [x] Create empty package directories for the Phase 1 skeleton:
      `internal/model/graph`, `internal/model/finding`, `internal/model/coupling`, `internal/model/diagnostic`,
      `internal/config`, `internal/baseline`, `internal/scope`, `internal/classify`,
      `internal/rules`, `internal/metrics`, `internal/status`,
      `internal/extract/golang`, `internal/history/git`, `internal/toolrun`,
      `internal/output/jsonout`, `internal/output/console`, `internal/engine`
- [x] Logging: **stdlib `log/slog`** at the edges only (`cmd`, `engine`); core stays pure (no logging). Deliberate divergence from your fleet's `logrus` — slog is the modern, zero-dep choice.
- [x] Set up tooling now — Makefile (Task T1), golangci-lint v2 config (T2), moq (T3), pre-commit/pre-push + gitleaks (T4) — see "Tooling & Automation".
- [x] `go build ./...` passes; `make lint` passes
- [x] Write smoke test in `cmd/archfit/main_test.go`: `app.Run([]string{"archfit","--version"})` exits 0 and prints the version

### Task 2: `model/graph` — Graph, Node, Edge (sealed)

Implements the central contract. Everything else depends on this. Sealed constructor enforces determinism.

- [x] Define `Node` struct: `Kind NodeKind` (repo/module/package/file/external), `Path string` (repo-relative). **Node identity is `kind:path`** (e.g. `file:services/a/x.go`, `module:a`) — matches the spec §7 edge endpoints (`"file:..."`) and the design §7 canonical key `type:path`.
- [x] Define `Edge` struct: `From, To string` (node IDs, `kind:path`), `Kind EdgeKind` (belongs_to/imports/depends_on/exposes/uses_internal), `Language string`, `Confidence string`, `Locations []Location`. JSON tags per spec §7: `from`, `to`, `kind`, `language`, `confidence`, `locations`.
- [x] Define `Location` struct: `File string`, `Line int`
- [x] Define `Facts` struct: raw extractor output (nodes + edges + unresolved counts) before sealing
- [x] Implement `graph.Build(facts []Facts) *Graph` — the sealed constructor: dedup edges by canonical key `(from, to, kind)`, choose highest-priority source per language, sort nodes by `type:path`, sort edges by `(from, to, kind, firstLocation)`, freeze (no exported mutability)
- [x] `Graph` exposes read-only accessors: `Nodes() []Node`, `Edges() []Edge`, `EdgesFrom(path string) []Edge`, `EdgesTo(path string) []Edge`
- [x] Write table-driven tests: dedup by canonical key, sort is deterministic, no mutation after build, multi-source priority

### Task 3: `model/finding` and `model/coupling` — sealed

- [x] Define `Finding` struct (JSON tags per spec §9/§12): `ID string` (`id`), `Kind string` (`kind`: `gate`|`advisory`; **always `gate` in Phase 1**), `RuleID string` (`rule_id`), `Status Status` (`status`: new/baseline/excepted/expired_exception/fixed), `Severity Severity` (`severity`: critical/high/medium/low), `Confidence string` (`confidence`), `Edge EdgeEvidence` (`edge`), `MatchedBy map[string]string` (`matched_by`), `Locations []Location` (`locations`), `Why string` (`why`), `Constraint string` (`constraint`), `Alternatives []string` (`allowed_alternatives,omitempty`)
- [x] Define `EdgeEvidence` struct (the **finding** edge, spec §9 — distinct from the graph `Edge`): `From, To Endpoint`, `Kind string`; `Endpoint{ Module string; Path string }`. Resolved at diagnostic assembly via `ModuleMap` (the graph `Edge` carries `kind:path` IDs only; the module label is joined in). Do **not** reuse `graph.Edge` here — it cannot emit `{module, path}`.
- [x] **No `RepairTask` struct in Phase 1** — the structured `repair_task` block is spec §16.4 (Phase 4). `Why` + `Constraint` + `Alternatives` are the Phase 1 equivalent.
- [x] Implement `finding.New(ruleID string, e graph.Edge, locs []Location) Finding` — computes `ID = hash(rule_id+from+to+kind)` inside the constructor; `Kind` defaults to `gate`; `Status` defaults to `new`; `Severity`/`MatchedBy`/`Edge`-module-labels filled later by the rule and diagnostic assembly
- [x] Define `coupling.Classification` struct: `Strength Strength`, `Distance Distance`, `Volatility Volatility`, `Explicitness Explicitness` (minimal for Phase 1; full field set stubbed, only contract/intrusive used initially)
- [x] Define `coupling.Index` as `map[string]Classification` keyed by edge canonical key
- [x] Write tests: `finding.New` produces stable ID for same inputs, different line numbers produce same ID

### Task 4: `model/diagnostic` — Diagnostic, MetricResult, Verdict

- [x] Define `Verdict` type: `pass | fail | warn`
- [x] Define `MetricResult` struct with **exact spec §10 JSON tags**: `Name string` (`name`), `Value float64` (`value`), `Display string` (`display`), `Band string` (`band`), `Confidence string` (`confidence`), `Version string` (`metric_version` — e.g. `encapsulation.v1`), `Mode string` (`mode`), `Definition string` (`definition`), `Delta *float64` (`delta,omitempty`)
- [x] Define `MetricSnapshot` as `map[string]struct{ Value float64; Version string }`
- [x] Define `Summary` struct (spec §12): `GateFindings int` (`gate_findings`), `Warnings int` (`warnings`), `ExceptionsUsed int` (`exceptions_used`)
- [x] Define `Coverage` struct: `Tool string`, `Version string`, `FilesSeen int`, `FilesApplicable int`, `Unresolved int`, `Status string`
- [x] Define `AgentTask` struct (placeholder shape; **no content emitted in Phase 1**, see Non-Goals) so the field is a typed empty slice, not `[]any`
- [x] Define `Diagnostic` struct with **exact spec §12 JSON tags**: `SchemaVersion string` (`schema_version` = `archfit.diagnostic.v1`), `Verdict Verdict` (`verdict`), `Base string` (`base`), `Head string` (`head`), `Metrics []MetricResult` (`metrics`), `Findings []Finding` (`findings`), `AgentTasks []AgentTask` (`agent_tasks` — emitted `[]`), `ToolCoverage []Coverage` (`tool_coverage`), `Summary Summary` (`summary`)
- [x] Write tests: zero-value Diagnostic has expected schema version; JSON marshals with the exact spec field names; `agent_tasks` serializes as `[]` not `null`; verdict derivation

### Task 5: `arch_test.go` — import gate (CI gate 1)

Add this early — it is the structural enforcement for ring 2 (core must not directly import os/os-exec/YAML/adapters).

- [x] Create `internal/arch_test.go` (package `arch_test`): uses `golang.org/x/tools/go/packages` to load all packages under `internal/`
- [x] Assert that packages in the "core ring" (`classify`, `rules`, `metrics`, `status`) do not directly import `os`, `os/exec`, any YAML library, or any adapter package (`toolrun`, `extract/golang`, `history/git`, `output/*`)
- [x] Assert that `model/*` packages import nothing outside stdlib
- [x] Gate runs as `go test ./internal/ -run TestArchImports`
- [x] Add `golang.org/x/tools` dependency
- [x] Verify gate is red if a deliberate bad import is added temporarily, then revert

### Task 6: `config` — Config struct, views, Load

- [x] Define `Config` struct covering Phase 1 fields: `Version int`, `Modules map[string]ModuleDef`, `Layers []string`, `Rules []RuleDef`, `Exclusions []string`, `Tools ToolsConfig`, `Metrics MetricsConfig`, `Exceptions []ExceptionDef`, `MapReview MapReviewConfig`, `Outputs OutputsConfig`
- [x] Define `ModuleDef`: `Paths []string`, `Public []string`, `Internal []string`, `Layer string`, `Subdomain string`, `Volatility string`, `Owner string`, `DeployUnit string`, `ReviewedAt string`, `ReviewedBy string`
- [x] Define `RuleDef`: `ID string`, `Type string`, `Gate string`, `From string`, `To string`, `FromLayer string`, `ToLayer string`
- [x] Define `ExceptionDef`: `Rule string`, `From string`, `To string`, `Reason string`, `ApprovedBy string`, `Expires string`
- [x] Define view types: `ScopeConfig`, `ExtractConfig`, `ClassifyConfig`, `RuleConfig`, `MetricConfig`, `ExceptionSet`, `MapReviewConfig`, `OutputConfig` (each is a narrow projection of `Config`)
- [x] **`ExtractConfig` is per-language** (built by `ForExtract(lang)`): common `Src`/`Paths`/`Exclusions`/`Internal` globs + `Mode` (`auto`|`on`|`off`, from the `Tools` map) + language settings — **TS:** `TSConfig` (tsconfig path); **Python:** `PyPackage` (top-level package) + project root; **Go:** none. `Tools` maps tool name → `{enabled: true|false|auto}` (spec §6).
- [x] Add `ModuleMap` sub-view with `ModuleFor(path string) (string, bool)` using glob matching (`doublestar/v4`)
- [x] Add projection methods on `Config`: `ForScope()`, `ForExtract(lang)`, `ForClassify()`, `ForRules()`, `ForMetric(name)`, `ForStatus()`, `ForOutput()`
- [x] Implement `config.Load(ctx, path string) (Config, error)` using `github.com/goccy/go-yaml` with `yaml.NewDecoder(r, yaml.DisallowUnknownField())` (strict: reject unknown keys with `[line:col]` errors); validate required fields; return descriptive errors. **Do NOT use `gopkg.in/yaml.v3` — it is archived (Phase 0).**
- [x] Add `github.com/bmatcuk/doublestar/v4` and `github.com/goccy/go-yaml` dependencies
- [x] Write table-driven tests: valid config loads; missing required field errors; `ModuleFor` glob matching; view projections contain only expected fields

### Task 7: `baseline` — Baseline types, Load/Save, status matching

- [x] Define `Baseline` struct: `SchemaVersion string` ("archfit.baseline.v1"), `Accepted []AcceptedFinding`, `Metrics MetricSnapshot`
- [x] Define `AcceptedFinding` struct: `Fingerprint string`, `RuleID string`
- [x] Implement `baseline.Load(ctx, path string) (Baseline, error)`: returns empty `Baseline` (not error) if file absent; returns exit-3-style error on schema version mismatch
- [x] Implement `baseline.Save(ctx, path string, b Baseline) error`
- [x] Write tests: missing file → empty baseline; schema mismatch → error; round-trip Load/Save; fingerprint lookup

### Task 8: `toolrun` — ToolRunner (subprocess choke point)

- [x] Define `ToolInfo` struct: `Name string`, `Path string`, `Version string`
- [x] Define `ToolCmd` struct: `Name string`, `Args []string`, `Env []string`, `Timeout time.Duration`
- [x] Define `Output` struct: `Stdout []byte`, `Stderr []byte`, `ExitCode int`
- [x] Define `Runner` **interface** in `toolrun`: `Detect(ctx, tool string) (ToolInfo, bool)`, `Run(ctx, cmd ToolCmd) (Output, error)` — adapters depend on this interface (the process boundary), so they're testable with a generated mock
- [x] Add `//go:generate moq -out runner_moq.go . Runner` (Task T3); the generated `RunnerMock` (func fields) backs `history/git` and `scope` tests
- [x] Implement `toolrun.New() *ToolRunner` (concrete struct implementing `Runner`) — the only package touching `os/exec`; the arch_test gate must catch any violation
- [x] Write tests using fake binaries in `$TMPDIR`: detect found/not-found, run captures output, env is controlled (locale+TZ pinned), timeout fires

### Task 9: `history/git` — git history functions

Per design §3 note on single-impl interfaces: `HistoryProvider` is one implementation so expose it as plain functions in the `history/git` package. The engine and `scope` import `history/git` directly. Each function takes a `toolrun.Runner` (the interface from Task 8) so tests pass the generated `toolrun` mock.

- [x] Implement `git.Changed(ctx, base, head string, runner toolrun.Runner) (ChangeSet, error)` — runs `git diff --name-only base..head`, returns sorted file list
- [x] Define `ChangeSet` struct: `Files []string`, `Base string`, `Head string`
- [x] Implement `git.HeadRef(ctx, runner toolrun.Runner) (string, error)` — runs `git rev-parse HEAD`
- [x] Implement `git.RepoRoot(ctx, runner toolrun.Runner) (string, error)` — runs `git rev-parse --show-toplevel`
- [x] Implement `git.Churn(ctx, runner toolrun.Runner) (ChurnStats, Coverage, error)` — stub returning empty + coverage record for Phase 1
- [x] Write tests: a real temp git repo (`git init` in `$TMPDIR`) for end-to-end; plus the generated `&toolrun.RunnerMock{RunFunc: ...}` for controlled stdout/exit — Changed returns expected files; HeadRef returns a SHA

### Task 10: `scope` — Scope type + resolution

- [x] Define `Scope` struct: `Base string`, `Head string`, `Changed []string` (sorted), `Root string` (repo root absolute path), `Mode ScopeMode` (delta/full)
- [x] Implement `scope.Resolve(ctx, cfg ScopeConfig, runner toolrun.Runner) (Scope, error)`:
  - full mode (`--full`): empty `Changed`, `Mode=full`
  - delta mode (`--base ref`): call `git.Changed(ctx, base, head, runner)`, populate `Changed`
  - detect repo root via `git.RepoRoot(ctx, runner)` (defined in Task 9)
- [x] Write tests: full scope has empty Changed; delta scope has sorted files; missing git → hard error

### Task 11: `extract/golang` — native Go extractor

Uses `golang.org/x/tools/go/packages`. No subprocess — in-process. Implements the `Extractor` interface declared in `engine` (see Task 16 note).

- [x] Implement `golang.New(cfg config.ExtractConfig) *GoExtractor` — constructor; `*GoExtractor` satisfies `engine.Extractor`
- [x] Implement `(*GoExtractor).Name() string` → `"go"`
- [x] Implement `(*GoExtractor).Extract(ctx, s scope.Scope) (graph.Facts, graph.Coverage, error)`:
  - load packages with the **Phase 0 LoadMode** `NeedName | NeedFiles | NeedImports | NeedSyntax | NeedTypes` (NOT `NeedModule`-only — that yields no positions; `Fset` requires `NeedTypes`)
  - read import positions from the **AST** (`pkg.Syntax` → `f.Imports` → `pkg.Fset.Position(imp.Pos())`), never from the `Imports` map
  - for each package: emit `file` nodes, `package` nodes, `module` node (IDs as `kind:path`)
  - for each import: emit `imports` edges from file/package to import path
  - classify Go `internal/` access via path only (`strings.Contains(p, "/internal/") || strings.HasSuffix(p, "/internal")`): `uses_internal` edge kind
  - **re-export shim resolution (spec §17):** best-effort resolve re-export shims/allowed-path wrappers to their real target so the forbidden coupling is not hidden; spec §18 marks full resolution an open question, so Phase 1 does what `go/packages` gives cheaply (direct import target) and lowers `confidence` when the real target is unresolved — never drops the edge
  - attach `language: "go"`, `confidence: "high"`, locations (file+line for import statement)
  - exclude paths matching `cfg.Exclusions`
  - emit `Coverage` record: files seen, packages loaded, unresolved imports
- [x] Add `golang.org/x/tools` dependency (already added for arch_test; confirm version ≥ v0.45.0)
- [x] Write table-driven tests against a **real `go.mod` module** under `testdata/` (`go/packages` shells out to real `go list` — it cannot be faked; use `Config.Overlay` only for synthetic-file cases):
  - simple A imports B → edge present
  - `internal/` access → `uses_internal` edge
  - excluded path → no edge
  - missing package → unresolved count increases, confidence lowers, no error

### Task 11A: `extract/ts` — TypeScript extractor (dependency-cruiser adapter)

Out-of-process adapter (design §6: detect → run → parse → normalize → coverage). Takes a `toolrun.Runner` (Task 8)
so it's testable with a moq mock; satisfies `engine.Extractor`. See the Phase 0 dependency-cruiser section.

- [x] Implement `ts.New(runner toolrun.Runner, cfg config.ExtractConfig) *TSExtractor`; `Name()` → `"typescript"`
- [x] **detect** (in `Extract`): `bunx depcruise --version` (prefer `bunx`, fall back to `npx`); applicability marker
      `package.json`. If `mode: auto` and absent/not-applicable → no facts, no error, **+ a coverage record** (never fail).
- [x] **run** via `ToolRunner`, repo as cwd, controlled env + timeout + version captured:
      `depcruise <src> --output-type json --ts-config <cfg.TSConfig> --include-only "^<src>" --exclude "^node_modules"`.
- [x] **parse** the JSON (per-tool parser, fixture-tested): `modules[].source` + the dependency array (handle both
      `dependencies[]` and `deps[]`); per edge read `resolved`, `module`, `dependencyTypes`, `dynamic`,
      `couldNotResolve`, `coreModule`.
- [x] **normalize** to `graph.Facts`: from = `file:<source>`, to = `file:<resolved>`, `imports` kind; skip `coreModule`;
      `couldNotResolve` → lower confidence + count unresolved (keep the edge); `language: "typescript"`; classify TS
      `internal` access via the configured module `internal` globs (TS has no `/internal/` convention).
- [x] **coverage** record: tool `dependency-cruiser` + version, files seen, unresolved count, status.
- [x] Tests use a **moq `toolrun.Runner`** feeding captured depcruise JSON fixtures (no real Node in unit tests):
      A imports B → edge; internal-glob target → `uses_internal`; `couldNotResolve` → unresolved + lower confidence;
      tool absent under `auto` → coverage record, no error.

### Task 11B: `extract/py` — Python 3.12+ extractor (grimp adapter via uv)

Out-of-process adapter; bundles a PEP 723 grimp helper (`go:embed`). Takes a `toolrun.Runner`; satisfies
`engine.Extractor`. See the Phase 0 grimp/uv section. **grimp directly, not import-linter.**

- [x] `//go:embed grimp_helper.py` (PEP 723 header, `dependencies = ["grimp"]`); write to a temp file at run time.
- [x] Implement `py.New(runner toolrun.Runner, cfg config.ExtractConfig) *PyExtractor`; `Name()` → `"python"`.
- [x] **detect** (in `Extract`): prefer `uv --version` (run via `uv run`); else `python3.12` + `import grimp`.
      Applicability marker `pyproject.toml`/`setup.py`/top-level package. Absent under `auto` → no facts, no error, + coverage record.
- [x] **run** via `ToolRunner`: `uv run --directory <root> <helper.py> --package <cfg.PyPackage>` (or the `python3.12`
      fallback), controlled env + timeout + version captured. Helper builds the **internal-only** graph (external
      packages excluded — no third-party deps needed).
- [x] **parse** the helper JSON: `{"edges":[{"importer","imported","line"}], "unresolved": N}`.
- [x] **normalize** to `graph.Facts`: nodes `module:<dotted.name>`; `imports` edges importer→imported with line;
      `language: "python"`; classify internal access via configured `internal` globs; `unresolved` lowers confidence.
- [x] **coverage** record: tool `grimp` (+ uv/python version), modules seen, unresolved count, status.
- [x] Tests: a small real Python package fixture under `testdata/`; plus a **moq `toolrun.Runner`** feeding captured
      helper JSON for the parse/normalize path (no real Python in unit tests).

### Task 12: `classify` — minimal BC classification (Phase 1 subset)

Phase 1 needs enough classification to drive the unbalanced-edge metric: `contract` vs `intrusive` strength (high-confidence), and `distance` from module path + owner + deploy_unit. Full explicitness/connascence/severity tables are Phase 2.

- [x] Implement `classify.Run(g *graph.Graph, c config.ClassifyConfig) coupling.Index`:
  - for each cross-boundary edge: determine `Strength` (contract if target path matches a configured `public` glob; intrusive if target matches an `internal` glob; model/functional start as `unknown`)
  - determine `Distance` from module ownership (same_module / cross_module_same_owner / cross_module_different_owner / cross_deploy_unit) using `ModuleMap` + `owner` + `deploy_unit` fields
  - determine `Volatility` from `subdomain` (core→high, supporting→low/medium, generic→low, unknown→unknown)
  - `Explicitness`: explicit if strength=contract; implicit if strength=intrusive; unknown otherwise (Phase 2 fills this out)
  - return index: edge canonical key → `Classification`
- [x] Write table-driven tests: contract edge has correct strength; intrusive edge; same-module edge distance; cross-deploy-unit distance; unknown subdomain → unknown volatility

### Task 13: `rules` — Rule interface + built-in Phase 1 rules

Per design §3: `Rule` is an interface (many impls). Config bound at construction.

- [x] Define `Rule` interface: `ID() string`, `Check(g *graph.Graph, ev Evidence) []finding.Finding`
- [x] Define `Evidence` struct: `PatternMatches []PatternMatch` (empty for Phase 1; no ast-grep yet)
- [x] **Config `type` strings are snake_case and must match the spec §9 exactly** (Go type names differ): `forbidden_dependency`, `public_api_only`, `forbidden_layer_direction`. `rules.New` maps the config `type` string → the concrete rule type.
- [x] Implement `ForbiddenDependency` rule (`type: forbidden_dependency`): from-module glob matches edge source, to-module glob matches edge target → finding
- [x] Implement `PublicAPIOnly` rule (`type: public_api_only`): cross-module edge where target is in an `internal` path → finding
- [x] Implement `ForbiddenLayerDirection` rule (`type: forbidden_layer_direction`): edge from a lower-priority layer to a higher-priority layer (per config `layers:` order) → finding
- [x] **Each rule populates the finding's `MatchedBy`** (e.g. `{"from_module_glob": "...", "to_internal_glob": "..."}`), `Why` (plain-language), and `Constraint` (the rule to preserve) — this is what satisfies the Phase 1 Done-when "an agent can read a gate finding and apply the repair constraints" (spec §16.1).
- [x] **No `cycle` rule in Phase 1** — cycle gating comes from the cycle-count _metric_'s `gate: fail` (spec §16.1), not a rule.
- [x] Implement `rules.New(cfg config.RuleConfig) []Rule` — returns slice of concrete rules from config
- [x] Write table-driven tests per rule type: matching cases produce findings; non-matching don't; edge-key in finding matches the edge; `matched_by`/`why`/`constraint` are populated

### Task 14: `status` — baseline diff + exception matching

`status` is a pure function (no interface needed — one impl, takes `now time.Time`).

- [x] Implement `status.Assign(findings []finding.Finding, base baseline.Baseline, exceptions config.ExceptionSet, now time.Time) []finding.Finding`:
  - for each finding: lookup fingerprint in `base.Accepted` → `baseline`
  - check exception rules (glob match on from/to, rule ID match, expiry vs `now`) → `excepted` or `expired_exception`
  - no match → `new`
  - for each accepted fingerprint not in current findings → produce `fixed` finding (optional for Phase 1; emit if not present)
- [x] Write table-driven tests: new finding; baseline finding; active exception; expired exception; fixed finding; expiry boundary

### Task 15: `metrics` — Metric interface + Phase 1 metrics

Per design §3: `Metric` is an interface (many impls). Config bound at construction via `metrics.New`.

- [x] Define `Metric` interface: `Name() string`, `Version() string`, `Calculate(in MetricInput) MetricResult`. **Names/versions are the spec §10.4 strings** — `Name()` ∈ {`encapsulation`, `unbalanced_edge`, `cycle`, `coverage`}; `Version()` is `<name>.v1` (e.g. `encapsulation.v1`), emitted as the `metric_version` JSON field.
- [x] Define `MetricInput` struct: `Graph *graph.Graph`, `Classifications coupling.Index`, `Findings []finding.Finding` (status-tagged), `Baseline MetricSnapshot`
- [x] Implement the **band model** (spec §10.1) as a shared helper: value→band (`strong` 9.0–10.0, `serviceable` 7.0–8.9, `mixed` 5.0–6.9, `poor` 3.0–4.9, `critical` 0.0–2.9). **Confidence caps the band**: low confidence cannot report `strong`/`serviceable` — clamp the highest reportable band down.
- [x] Implement `EncapsulationMetric` (`encapsulation.v1`): `contract_cross_boundary_edges / all_cross_boundary_edges`; uses `coupling.Index` for strength; emits `value`, `display`, `band`, `confidence`, `mode`, `definition`, `delta` vs baseline
- [x] Implement `UnbalancedEdgeMetric` (`unbalanced_edge.v1`): count edges where `strength=intrusive AND distance>=cross_module_different_owner AND volatility=high`; report counts by status (new_high, baseline_high, excepted_high, expired)
- [x] Implement `CycleMetric` (`cycle.v1`): detect module-level cycles via DFS on the `Graph`; count new vs baseline. (Cycle gating is this metric's `gate: fail` config — there is no cycle rule.)
- [x] Implement `CoverageMetric` (`coverage.v1`): `extracted_files / applicable_files` from `Coverage` records passed in; confidence based on unresolved count
- [x] Implement `metrics.New(cfg config.Config) []Metric` using `cfg.ForMetric(name)` per metric
- [x] Write table-driven tests per metric: known inputs produce expected values; delta computed correctly; zero-denominator handled (encapsulation = 1.0 if no cross-boundary edges); cycle detection correct; **low confidence caps the band**

### Task 16: `engine` — port interfaces + stage orchestrator

The engine declares the port interfaces it consumes (design principle: consumer defines the smallest interface). Adapter packages return concrete types that satisfy these interfaces; the engine imports only `engine`-declared interfaces, not adapter packages — the arch_test gate enforces this.

- [x] Declare port interfaces in `internal/engine/ports.go`

  ```go
  type Extractor interface {
      Name() string
      Extract(ctx context.Context, s scope.Scope) (graph.Facts, graph.Coverage, error)
  }
  type Renderer interface {
      Format() string
      Render(d diagnostic.Diagnostic, w io.Writer) error
  }
  // Rule and Metric interfaces live in rules/ and metrics/ respectively (many impls defined there).
  ```

- [x] Define `Mode` struct: `Base string`, `Head string`, `Full bool`, `Advisory bool`, `ReportOnly bool`, `Formats []string`
- [x] Implement `engine.Run(ctx, mode Mode, s scope.Scope, classifyCfg config.ClassifyConfig, exceptions config.ExceptionSet, extractors []Extractor, rules []rules.Rule, ms []metrics.Metric, renderers []Renderer, base baseline.Baseline, now time.Time) (diagnostic.Diagnostic, error)`:
  1. run the applicable language extractors (sequential for Phase 1), merge their `Facts` **and coverage records**, call `graph.Build`
  2. call `classify.Run(g, classifyCfg)` → `coupling.Index`
  3. run rules → raw findings
  4. call `status.Assign(findings, base, exceptions, now)`
  5. compute `MetricInput`, run metrics
  6. assemble `Diagnostic`: **resolve each finding's `EdgeEvidence` `{module, path}` from the graph node IDs via `ModuleMap`** (the graph `Edge` carries only `kind:path`); join severity onto findings from `coupling.Index`; set `agent_tasks` to an **empty typed slice** (Phase 1 emits no content); fill `Summary`; compute `computeVerdict(gateFindings, metricResults)`
  7. run renderers
  8. return `(Diagnostic, nil)` on success; `(Diagnostic, error)` for hard errors (exit 3 cases)
- [x] Implement `computeVerdict(gateFindings []finding.Finding, ms []diagnostic.MetricResult) diagnostic.Verdict` — unexported; gate findings with status `new` **or `expired_exception`** on a `gate: fail` rule → `fail` (exit 1); metric regressions outside threshold → `warn` (exit 2); clean → `pass` (exit 0). A `--report` run downgrades metric regressions to non-blocking (gate rules still fail).
- [x] Add `//go:generate moq -out extractor_moq.go . Extractor` in `engine` (boundary mock, Task T3)
- [x] Write integration tests: mock the **`Extractor`** boundary (`&ExtractorMock{ExtractFunc: ...}` returning canned `Facts`); use **real** `rules`, `metrics`, and `jsonout` on the canned graph (don't mock pure stages); assert gate finding → verdict=fail (exit 1); clean run → verdict=pass (exit 0)

### Task 17: `output/jsonout` and `output/console` — Renderers

- [x] `jsonout.New()` and `console.New()` return values whose types satisfy `engine.Renderer` (the interface declared in Task 16); no separate `Renderer` interface definition needed here
- [x] Implement `jsonout.New()` — marshals `Diagnostic` as JSON with `schema_version: "archfit.diagnostic.v1"`; no wall-clock in the deterministic body
- [x] Implement `console.New()` — prints a short summary: verdict line + finding count + exit-code hint + pointer to JSON for full detail
- [x] Write tests: JSON output is valid JSON; schema_version field present; console output contains verdict string

### Task 18: `cmd/archfit` — CLI wiring (kong) + integration fixture

- [x] Create per-language fixtures under `testdata/`, each a real project with a deliberate boundary violation:
  - **Go** `testdata/fixture-go/` — real Go module (`go.mod`); `pkg/a/a.go` imports `pkg/b/internal/impl` (violates `public_api_only`); `archfit.yaml` with modules `a`/`b`, a `public_api_only` rule, `gate: fail`
  - **TypeScript** `testdata/fixture-ts/` — `package.json` + `tsconfig.json`; `src/a.ts` imports `src/b/internal/impl.ts`; `archfit.yaml` with the same rule shape
  - **Python** `testdata/fixture-py/` — `pyproject.toml` + a top-level package; `a/x.py` imports `b/_internal/impl.py`; `archfit.yaml` with the same rule shape
  - The **Go** fixture drives the integration + golden tests (no external toolchain in the deterministic CI gate); the TS/Python fixtures drive their extractor integration tests when Node/Python are present (skipped with a coverage note otherwise)
- [x] Build the kong CLI struct: a top-level struct whose fields are the command structs `Check`, `Baseline`, `Explain`, `Doctor`, `Init` (each a `*Cmd` type, tagged `cmd:""`) — **no `Scan` in Phase 1** (scan needs markdown + advisory; Phase 2)
- [x] `CheckCmd` typed fields: `Base string`, `Full bool`, `Format []string` (tag `enum:"json,console" default:"console"`), plus `Advisory bool` and `Report bool` (parsed but no-op in Phase 1)
- [x] `main` builds the composition root **once** and passes it via `kong.Bind(...)`; `ctx := kong.Parse(&cli, ...); err := ctx.Run()`. Construct `config.Load`, `baseline.Load`, `runner := toolrun.New()`, then the **applicable** extractors —
      `golang.New(cfg.ForExtract("go"))`, `ts.New(runner, cfg.ForExtract("typescript"))`, `py.New(runner, cfg.ForExtract("python"))` (each detects/skips itself) — plus `git.*`, `rules.New(cfg.ForRules())`, `metrics.New(cfg)`, `jsonout.New()`, `console.New()`.
- [x] `CheckCmd.Run(deps...) error`: run `engine.Run`, render; return a typed error carrying the exit code
- [x] `BaselineCmd.Run`: run engine stages 1–7 then `baseline.Save`
- [x] `ExplainCmd.Run`: re-run engine in-memory, filter to the single finding by fingerprint prefix (stateless, D7); print `why`/`constraint`/`edge`
- [x] `DoctorCmd.Run`: detect the full toolchain via `toolrun` — `go`, `git`, `node`+`dependency-cruiser` (`bunx depcruise --version`), `uv`/`python3.12`+`grimp` — and print an availability + version + install-hint table (the same `Detect` each extractor uses)
- [x] `InitCmd.Run`: stub (prints "not yet implemented")
- [x] **Exit codes in one place:** `main` maps the result → `os.Exit(0/1/2/3)` after `ctx.Run()` (error→3, verdict→0/1/2). Commands return typed errors; they never call `os.Exit`.
- [x] Write integration test: invoke the parsed app against `testdata/fixture/` with violation → exit 1 + JSON finding present; remove the bad import → exit 0

### Task 19: Double-run golden test (CI gate 2)

- [x] Create `internal/engine/golden_test.go`: run `engine.Run` twice with identical inputs against `testdata/fixture/` → compare JSON body bytes (excluding run metadata envelope); fixture is created in Task 18
- [x] Assert byte-identical output on both runs
- [x] This is a CI-mandatory test; add it to the Makefile `test` target

### Task 20: Final acceptance verification

- [x] Run `go build ./...` — no errors
- [x] Run `go test ./...` — all pass
- [x] Run `go vet ./...` — clean
- [x] Run `gofmt -l .` — no files reported
- [x] Run `arch_test.go` gate explicitly — green
- [x] Run double-run golden test — byte-identical output
- [x] Manual smoke: `archfit check` on `testdata/fixture/` with violation → exit 1 + JSON finding; remove violation → exit 0
- [x] Verify finding JSON contains: `id`, `kind: "gate"`, `rule_id`, `status: "new"`, `severity`, `confidence`, `edge.from.module`, `edge.from.path`, `edge.to.module`, `edge.to.path`, `matched_by`, `why`, `constraint`
- [x] Verify metric JSON contains: `name`, `value`, `display`, `band`, `confidence`, `metric_version`, `mode`, `definition`, `delta`
- [x] Verify top-level JSON: `schema_version: "archfit.diagnostic.v1"`, `agent_tasks` serializes as `[]` (not `null`)
- [x] Verify console output includes verdict + pointer to detail

---

## Technical Details

### Import rings (design §4)

```
model/* ← no deps outside stdlib
config (goccy/go-yaml), baseline (encoding/json) ← model/*
core (classify, rules, metrics, status) ← model/*, config views, baseline types
ports+adapters (extract/golang, history/git, toolrun, output/*) ← model/*, toolrun
engine ← contracts, core — NOT adapters directly
cmd ← everything (composition root only)
```

### Stack & idioms (matches your fleet)

- **CLI:** `alecthomas/kong` — typed command structs, explicit `kong.Bind` wiring, no globals/codegen (fits design D5). Idiomatic Go 1.26 (Perplexity-validated). Chosen over urfave (stringly-typed) and cobra (globals + viper + codegen).
- **Logging:** stdlib `log/slog` at edges only; core is pure.
- **Errors:** stdlib `fmt.Errorf("...: %w", err)` + `errors.Is/As`; no `pkg/errors`.
- **Tests:** stdlib `testing` (table-driven, no testify) + `matryer/moq` for boundary mocks only (`toolrun.Runner`, `Extractor`).
- **Ring enforcement is `arch_test.go` only** (design D6). golangci-lint = code quality, never the ring boundary (no depguard for rings — it would drift from the test).

### Node identity & graph vs finding edges

- Graph node ID = `kind:path` (`file:...`, `package:...`, `module:...`); graph `Edge.From/To` are these IDs (spec §7).
- Finding `EdgeEvidence` is a **separate** shape: `from/to: {module, path}` (spec §9), resolved from graph IDs via `ModuleMap` at diagnostic assembly. One struct cannot emit both — keep them distinct.

### Metric bands (spec §10.1)

```
strong       9.0-10.0
serviceable  7.0-8.9
mixed        5.0-6.9
poor         3.0-4.9
critical     0.0-2.9
```

Low confidence caps the highest reportable band (cannot report `strong`/`serviceable`).

### Sealed constructors (determinism)

- `graph.Build([]Facts)` — sorts, dedups, freezes
- `finding.New(ruleID, edge, locs)` — computes ID fingerprint inside
- Neither returns anything mutable

### Finding identity

`ID = hex(sha256(rule_id + "\x00" + from + "\x00" + to + "\x00" + kind)[:16])`
No line numbers. One finding per (rule, from→to, kind). Sites in `Locations[]`.

### Exit codes

```
0  pass
1  gate failed (new findings with gate: fail)
2  metric regression / warning
3  tool/config error (engine returned non-nil error)
```

### Minimal BC classification for Phase 1

Only `contract` and `intrusive` strength are high-confidence in Phase 1 (spec §18).
`model` and `functional` are set to `unknown` until Phase 2 adds config-assisted signals.
Unbalanced-edge metric uses `intrusive + cross_module_different_owner + high volatility` as the Phase 1 proxy for "high severity."

---

## Tooling & Automation (Tasks T1–T6)

Tooling **structure** follows your mature repos (**pumba**, **bq-ch-sync**) — not qmd-go (alpha). **Library choices are idiomatic Go 1.26, not fleet-matched** (see Phase 0). Sequencing: **T1–T4 with/right after Task 1**; **T5 after Task 16**; **T6 after Task 18**.

### Task T1: Makefile (copy pumba / bq-ch-sync pattern)

- [ ] `help` is the default target (grep-based self-doc, as in pumba)
- [ ] `build`: `CGO_ENABLED=0 go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)" -o .bin/archfit ./cmd/archfit`
- [ ] `test`: `go test -race -coverprofile=coverage.out ./...`; `test-coverage`: HTML view
- [ ] `lint`: `golangci-lint run -c .golangci.yaml ./...`
- [ ] `fmt`: `gofmt -s -w .` + `goimports -w -local github.com/alexei-led/archfit .`
- [ ] `mock`: `go generate ./...` (runs the moq `//go:generate` directives)
- [ ] `setup-tools`: `go install` golangci-lint (pinned), goimports, moq (pinned)
- [ ] `docker-build` / `docker-push`: build + push to GHCR (Task T6)
- [ ] `clean`, `version`, `all: fmt lint test build`

### Task T2: golangci-lint v2 config (copy bq-ch-sync `.golangci.yaml`)

- [ ] Use the **v2 schema** (`version: "2"`) — your newest mature repos are on it
- [ ] `linters.default: standard` + enable list: errcheck, govet, staticcheck, revive, ineffassign, unused, gocritic, gocyclo, goconst, gosec, prealloc, unconvert, unparam, nolintlint, misspell (US), whitespace, errorlint, perfsprint, usestdlibvars (drop `testifylint` — no testify)
- [ ] `formatters.enable: [gofmt, goimports]` with `goimports.local-prefixes: github.com/alexei-led/archfit`
- [ ] `exclusions.paths`: `mocks/`, `testdata/`; relax funlen/dupl/gosec on `_test.go`
- [ ] **No depguard ring rule** — `arch_test.go` owns the rings. (A depguard `deny: logrus` lib-hygiene rule is fine; ring boundaries are not.)

### Task T3: moq (mock generation)

- [ ] `//go:generate moq -out <iface>_moq.go . <Iface>` on **boundary** interfaces only: `toolrun.Runner`, `engine.Extractor`
- [ ] `make mock` runs `go generate ./...`; generated `*_moq.go` files are committed and lint-excluded
- [ ] moq mocks are plain structs with `<Method>Func` fields used directly with stdlib `testing` — no testify dependency
- [ ] Do NOT generate mocks for `Rule`/`Metric`/`Renderer` (pure — table-tested)

### Task T4: pre-commit + pre-push + gitleaks (copy pumba config + `~/Workspace/architect/.gitleaks.toml`)

- [ ] `.pre-commit-config.yaml` (fast hooks only): `pretty-format-golang` (gofmt), `go-mod-tidy`, **`gitleaks`**, `check-added-large-files`, `sign-commit` (your fleet signs commits)
- [ ] **pre-push** hook (slow): `make lint test` — heavy checks here, not per-commit
- [ ] `.gitleaks.toml`: `[extend] useDefault = true` + allowlist (`.gitignore`, `.gitleaks.toml`) + the belt-and-suspenders path rules from `architect/.gitleaks.toml` (env files, credential/token-named files, `*.pem|key|p12`)
- [ ] Document `pre-commit install --install-hooks` + `pre-commit install -t pre-push` in the README

### Task T5: GitHub Actions CI (copy bq-ch-sync / pumba; after Task 16)

- [ ] `.github/workflows/ci.yaml` on push/PR: separate `lint`, `test`, `build` jobs on `ubuntu-24.04`
- [ ] `actions/setup-go@v6` with `cache: true`, Go 1.26
- [ ] `golangci/golangci-lint-action@v9` (v2 config)
- [ ] `govulncheck ./...` step (as in pumba)
- [ ] **Language toolchains for the TS/Python extractor tests:** `actions/setup-node@v4` (+ `dependency-cruiser` via `bunx`/`npm i -g`) and `astral-sh/setup-uv@v5` (provides `uv` + Python 3.12). The deterministic gates (golden, `arch_test`) use the **Go** fixture and need none; the TS/Python extractor integration tests run only when these toolchains are present.
- [ ] run the **double-run golden test** + `arch_test.go` gate in the test job (CI gates 1 & 2)

### Task T6: Release + Dockerfile + GHCR (copy pumba / spotinfo; after Task 18)

- [ ] **Dockerfile — distribution decision (design D8 / spec §17), two targets:**
  - default `scratch` image (`golang:1.26-alpine` builder, `--mount=type=cache`, `CGO_ENABLED=0`, `FROM scratch` + ca-certs, nonroot) — runs the **Go path** anywhere; TS/Python degrade to a coverage gap + install hint.
  - optional `archfit:full` image on a slim base bundling Node + `dependency-cruiser` and `uv` + Python 3.12 — analyses TS/Python in CI/containers out of the box.
- [ ] README: the bare binary needs the target's toolchain on `PATH` for TS/Python (host/CI), or use `archfit:full`.
- [ ] `.github/workflows/release.yaml` on semver tag (`[0-9]+.[0-9]+.[0-9]+`): `make release` cross-compile (darwin/linux × amd64/arm64) + native multi-arch Docker → **GHCR** `ghcr.io/alexei-led/archfit` (both image targets), merge manifest, `softprops/action-gh-release` with binaries
- [ ] **No goreleaser** (fleet convention: make-release + `gh release create`)
- [ ] Optional later: cosign keyless signing (fleet doesn't sign images yet — flagged, not required)

---

## Post-Completion

**Before declaring Phase 1 done:**

- `archfit check` fails on the deliberate violation in **each** fixture (Go, TS, Python) and passes once fixed — the
  same rule model across all three languages.
- A missing optional toolchain (Node/dependency-cruiser, uv/python/grimp) yields a coverage record + install hint and
  lowered confidence — never a panic or a false failure. `doctor` lists all toolchains with install hints.
- Run `archfit check` against the `archfit` repo itself (Go) and verify it doesn't crash (it won't catch all its own
  violations until `archfit.yaml` is written for it, but must not error).
- A missing `git` binary (Tier 0) produces exit 3 with a clear message, not a panic.

**Phase 2 trigger** (Balanced Coupling classification + scan report):
Start when Phase 1 passes its Done-when criteria (spec §16):
a real violation fails in any language, the fix passes, metric deltas are explainable, an agent can use the finding.

**Phase 3 trigger** (semantic fidelity: ast-grep patterns · SCIP/LSP real-target resolution · tree-sitter fallback):
Start when re-export shims / barrel files hide real coupling, or a toolchain-free native fallback is needed (design D8).

**Phase 4 trigger** (SARIF + GitHub Action + MCP):
Start when the JSON contract is stable and an agent loop integration is the bottleneck.
