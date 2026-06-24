# Syntax facts via ast-grep — complete implementation

## Overview

Add a complete **syntax-facts layer** to archfit, extracted via the
already-shipped ast-grep (`sg`) subprocess, for **Go, TypeScript, Python, and
Rust**:

- **Facts (neutral, off-gate):** declarations (name/kind/exported/location),
  public API surface, decorators/attributes, framework routes → architectural
  roles (handler/service/repository/domain). Surfaced in `scan`/`review` output
  and `agent_tasks` evidence.
- **Gate control (opt-in):** new rule types `forbidden_role_dependency`,
  `public_api_max`, `public_api_change`, plus wiring the `gate:` field
  (off/warn/fail) for **all** rules (a latent-bug fix).

Zero new build/binary cost: no CGO, no new Go module — `sg` is a subprocess.
Single-phase, complete implementation (no deferred languages or features).

Design: `docs/design/syntax-facts-via-astgrep-v1.0.md`.
Research basis: `docs/research/tree-sitter-for-archfit.md`.

## Context (from discovery)

- **Files/components:**
  - `internal/model/diagnostic/diagnostic.go` — add `SyntaxFact` + `Diagnostic.SyntaxFacts`.
  - `internal/extract/astgrep/{astgrep.go,rules/}` — add `Syntax()` + 4 embedded rule files.
  - `internal/ports/ports.go` — `SyntaxProvider` port.
  - `internal/config/{tools.go,views.go,modules.go}` — `tools.syntax` opt-in; new `RuleDef` fields.
  - `internal/syntax/` (NEW core pkg) — role derivation + `NodeRoleIndex` (the role→node join).
  - `internal/rules/rules.go` — `Evidence.SyntaxFacts`; gate wiring; 3 new rule types; unknown-type error.
  - `internal/arch_test.go` — add `internal/syntax` to core-ring allow-list.
  - `internal/engine/engine.go`, `cmd/archfit/pipeline.go` — wire provider, derive roles before rules, populate block.
  - `cmd/archfit` scan + agent_tasks; `.archfit.yaml`; engine goldens.
- **Patterns to reuse:** astgrep adapter (`sg` via `Runner`, absent→`absent`, `RunnerMock` tests); `FileFact` neutral-facts precedent; SCIP `go:embed` script technique; opt-in tools (`tools.scip`); `new_cross_module_dependency` (baseline-suppressed findings → model for `public_api_change`); `forbidden_layer_direction` (rule with `ModuleMap` → model for role rules); Rust `modgraph.ModuleKeyResolver` (file→`crate::mod`).
- **Validated empirically (sg 0.44, `--inline-rules`):** declaration kinds + exported detection + decorators/attributes + routes for **all four** languages; one scan per language tags kinds by `ruleId`.
- **Edge granularity per language (the join):** TS file→file; Go file→**package** (+`belongs_to`); Python **dotted module**; Rust `crate::mod`.
- **Gate state:** `RuleDef.Gate` stored but ignored (all findings block); all self-config rules `gate: fail`; only config-parse tests + `archfit init` use `warn`.

## Development Approach

- **Testing approach:** Regular (code first, then tests) — matches existing adapter/test style.
- Complete each task fully before the next; small focused changes.
- **Every task includes new/updated tests** (success + error/edge).
- **All tests pass before the next task.** `make test` (race + coverage); keep `make lint` clean.
- **Structural gates stay green:** `go test ./internal/ -run TestArchImports`; `go test ./internal/engine/ -run TestGolden` (regenerate deliberately, inspect diff); `make archfit` (dogfood).
- Update this plan when scope changes (`➕` new task, `⚠️` blocker).

## Testing Strategy

- **Unit:** adapter via `RunnerMock` + canned `sg` JSON; role derivation + `NodeRoleIndex` as pure table-driven tests (one case per language for the join); config + rule tests.
- **Golden:** `TestGolden` updated once, deliberately (new `syntax_facts` block + any gate-wiring change); inspect diff.
- **Integration (skip-if-`sg`-absent):** JSON-shape guard against a fixture.
- **No e2e/UI** — CLI project.

## What Goes Where

- **Implementation Steps (`[ ]`):** code, tests, docs in this repo.
- **Post-Completion (no checkboxes):** manual verification only.

## Implementation Steps

### Task 1: SyntaxFact model + Diagnostic block

- [x] add `SyntaxFact` to `internal/model/diagnostic/diagnostic.go` (fields per design §3, incl. Role/RoleConf/Evidence/Framework)
- [x] add `SyntaxFacts` slice to `Diagnostic` with omitempty json tag
- [x] add `SortSyntaxFacts` (File, StartLine, Kind, Name)
- [x] write tests for the sort (stable order, tie-breaks)
- [x] run tests — must pass before next task

### Task 2: SyntaxProvider port + tools.syntax config

- [x] add `SyntaxProvider` interface to `internal/ports/ports.go`
- [x] add `const ToolSyntax` + mode accessor in `internal/config/tools.go` (mirror `tools.scip`)
- [x] expose mode + target languages via `ForSyntax()` in `internal/config/views.go`
- [x] `make mock` to regenerate the port fake
- [x] write config tests (enabled/auto/off parsing)
- [x] run tests — must pass before next task

### Task 3: Adapter Syntax() + embedded Go rules

- [x] add `internal/extract/astgrep/rules/go.yml` (go:embed): function/method/type declarations, exported via name regex `^[A-Z]`, route calls (net/http, gin, echo, chi, fiber, gorilla) + handler signature
- [x] implement `Syntax()`: one `sg scan --inline-rules <yaml> --json=compact .` per language, map `ruleId`→Kind, capture name + start/end line + framework; absent `sg` → `absent`, no error; sort via Task 1 helper
- [x] write adapter tests with `RunnerMock` canned JSON: kinds, exported, route+framework, absent path, malformed-JSON error
- [x] run tests — must pass before next task

### Task 4: Embedded TypeScript rules

- [x] add `rules/typescript.yml`: function/class/interface/enum/type_alias/method, exported via `inside: export_statement`, `decorator` capture, routes (express/koa/fastify calls; nest `@Controller`/`@Get`)
- [x] wire TS into `Syntax()` dispatch
- [x] write adapter tests (canned JSON): class/interface/exported/decorator/route
- [x] run tests — must pass before next task

### Task 5: Embedded Python rules

- [x] add `rules/python.yml`: function*definition/class_definition, public via name regex `^[^*]`, `decorator`(has call), routes (fastapi/flask/starlette/aiohttp decorators; django`urlpatterns` path/re_path)
- [x] wire Python into `Syntax()` dispatch
- [x] write adapter tests (canned JSON): public detection, decorator, route per framework
- [x] run tests — must pass before next task

### Task 6: Embedded Rust rules

- [x] add `rules/rust.yml`: function/struct/enum/trait/impl/mod items, pub via `has: {kind: visibility_modifier}`, `attribute_item` capture, routes (actix/rocket `#[get]`; axum/warp `.route()` builder)
- [x] wire Rust into `Syntax()` dispatch
- [x] write adapter tests (canned JSON): pub detection, attribute, route per framework
- [x] run tests — must pass before next task

### Task 7: JSON-shape guard + Docker pin

- [ ] add skip-if-`sg`-absent integration test asserting `ruleId`/`range`/`metaVariables` against a fixture
- [ ] align Dockerfile `ASTGREP_VERSION` (0.44.x) or comment-confirm 0.43.0 supports `--inline-rules`
- [ ] confirm the test skips cleanly without `sg`
- [ ] run tests — must pass before next task

### Task 8: Role derivation (new core pkg)

- [ ] create `internal/syntax` core pkg; pure `DeriveRoles([]SyntaxFact, moduleViews) []SyntaxFact` setting Role/RoleConf/Evidence per design §5
- [ ] add `internal/syntax` to the core-ring allow-list in `internal/arch_test.go`
- [ ] write table-driven tests: each role, each confidence tier, no-match
- [ ] run `TestArchImports` + unit tests — must pass before next task

### Task 9: NodeRoleIndex — the role→graph-node join

- [ ] add `BuildNodeRoleIndex(g, []SyntaxFact, moduleViews)` + `RolesFor(nodeID) []RoleHit` in `internal/syntax`
- [ ] implement per-language resolution: TS identity; Go aggregate file roles to package via `EdgeKindBelongsTo`; Python dotted↔slash normalization; Rust reuse `modgraph.ModuleKeyResolver` (file→`crate::mod`)
- [ ] write table-driven tests with one synthetic graph per language asserting `RolesFor` resolves endpoints (esp. Python dotted, Go package, Rust crate::mod)
- [ ] run tests — must pass before next task

### Task 10: Engine wiring — populate SyntaxFacts

- [ ] run `SyntaxProvider` when `tools.syntax` enabled; derive roles (Task 8); build `NodeRoleIndex` (Task 9) — all **before** the rules stage
- [ ] populate `Diagnostic.SyntaxFacts`; append ast-grep `Coverage` to `ToolCoverage`
- [ ] write engine tests (fake provider): block populated, roles applied, coverage recorded, verdict unchanged (off-gate)
- [ ] run tests — must pass before next task

### Task 11: Wire gate: semantics for all rules

- [ ] in the engine/rules layer, set finding `Kind` from each rule's `gate:`: `off`→skip, `warn`→`advisory`, `fail`/unset→`gate`
- [ ] make unknown rule `type:` a config error in `rules.New` (fail fast)
- [ ] write tests: off skips, warn→advisory (verdict warn, non-blocking), fail→blocking; unknown type errors
- [ ] regenerate/verify `TestGolden`; run `make archfit` — dogfood stays green
- [ ] run tests — must pass before next task

### Task 12: forbidden_role_dependency rule

- [ ] add `FromRole`, `ToRole`, `MinConfidence` to `RuleDef`; add `Evidence.SyntaxFacts` + pass `NodeRoleIndex` into the rule
- [ ] add `case "forbidden_role_dependency"` to `rules.New`: edge A→B where `RolesFor(A)` has from_role and `RolesFor(B)` has to_role; default fire on `high` confidence only (relax via `MinConfidence`)
- [ ] write tests: violation fires (high conf), suppressed below threshold, gate off/warn/fail honored, per-language endpoint resolution
- [ ] run tests — must pass before next task

### Task 13: public_api_max rule

- [ ] add `Max` to `RuleDef`; add `case "public_api_max"`: count exported decls per module (from `SyntaxFacts`); `> Max` → finding with the offending count
- [ ] write tests: under/over limit, per-module scoping, gate modes
- [ ] run tests — must pass before next task

### Task 14: public_api_change rule

- [ ] add `case "public_api_change"`: emit one finding per exported decl (fingerprint = ruleID+module+name) so the baseline/status stage suppresses known ones and surfaces new public API as `StatusNew` (mirror `new_cross_module_dependency`); default `gate: warn`
- [ ] write tests: first run baselines all, added decl surfaces new, removed decl handled, advisory by default
- [ ] run tests — must pass before next task

### Task 15: Consumer — scan output + public API summary

- [ ] add a "Syntax surface" section to `scan`: per-module declaration counts, public API (exported) list, detected roles/routes
- [ ] write output tests (table-driven/golden)
- [ ] run tests — must pass before next task

### Task 16: Consumer — agent_tasks evidence enrichment

- [ ] enrich `agent_tasks` evidence with declarations + role + file:line for referenced files; unchanged when `SyntaxFacts` empty
- [ ] write tests: enriched when present, no regression when absent
- [ ] run tests — must pass before next task

### Task 17: Self-config enablement + golden

- [ ] enable `tools.syntax` (facts, off-gate) in `.archfit.yaml`
- [ ] add example gate rules to `.archfit.yaml` as `gate: warn` (or omit) so `make archfit` stays green; document them
- [ ] regenerate `TestGolden`; inspect + commit the `syntax_facts` diff
- [ ] run `make archfit` — must stay green
- [ ] run tests — must pass before next task

### Task 18: Verify acceptance criteria

- [ ] verify facts (declarations + exported + public API + decorators/routes→roles) for Go, TS, Python, Rust
- [ ] verify gate control: `forbidden_role_dependency`, `public_api_max`, `public_api_change` fire correctly; `gate:` off/warn/fail honored for all rules; unknown type errors
- [ ] verify absent-`sg` → empty facts + `n/a` coverage, no verdict change (no false green)
- [ ] run `make test`; `TestArchImports`; `TestGolden`; `make lint`; `make archfit`
- [ ] verify coverage meets project standard

### Task 19: Update documentation

- [ ] update user guide (`docs/guide`): `tools.syntax`, `syntax_facts` block, the 3 new rule types, `gate:` semantics
- [ ] flip design doc status PROPOSED → SHIPPED; note deviations
- [ ] update `CLAUDE.md`: new `internal/syntax` core pkg in the ring; `gate:` now wired for all rules

## Technical Details

- **Producer:** `sg scan --inline-rules "<embedded yaml>" --json=compact .` per language via `Runner`, `WorkDir: scope.Root`; `---`-separated rules, `ruleId`→Kind. No temp files.
- **Join:** `NodeRoleIndex` encapsulates per-language file→node resolution (TS identity, Go `belongs_to` aggregation, Python dotted↔slash, Rust `ModuleKeyResolver`). Rules stay language-agnostic.
- **Gate:** finding `Kind` derived from `gate:` (`off`/`warn`→advisory/`fail`→gate). Gate findings feed `boundary_integrity` automatically.
- **Ordering:** SyntaxProvider → DeriveRoles → BuildNodeRoleIndex → rules stage.
- **Determinism:** sort `(File, StartLine, Kind, Name)`.
- **Confidence:** role gating defaults to `high`; `min_confidence` relaxes.

## Post-Completion

_Manual/external only — no checkboxes._

- Run `archfit scan` on a real repo per language (Go/TS/Python/Rust); eyeball
  roles/routes for false positives; tune the capped framework set and role
  heuristics if needed.
- Confirm `archfit check` with the new gate rules on a downstream repo behaves as
  intended before promoting any `gate: warn` to `gate: fail`.
