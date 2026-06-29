# Syntax facts via ast-grep — design v1.0 (complete)

Date: 2026-06-24. Status: SHIPPED. This is the complete implementation — all
four languages and gate control, no phases (it replaces an earlier phased draft
that scoped only Go+TS and deferred Python/Rust + gate rules).
Research basis: `docs/archived/research/tree-sitter-for-archfit.md`.
Plan: `docs/plans/20260624-syntax-facts-via-astgrep.md`.

---

## 1. What this adds and why

archfit's extractors answer _who imports whom_. They do not answer _what shapes
and annotations exist_, nor enforce rules in those terms. This adds a complete
**syntax-facts layer** for **Go, TypeScript, Python, Rust**:

- **Facts (neutral, off-gate):** declarations (name/kind/exported/location),
  public API surface, decorators/attributes, framework routes → architectural
  roles (handler/service/repository/domain), surfaced in `scan`/`review` output
  and `agent_tasks` evidence.
- **Gate control (opt-in):** new rule types that gate on those facts —
  `forbidden_role_dependency`, `public_api_max`, `public_api_change` — plus
  proper wiring of the `gate:` field (off/warn/fail) for **all** rules.

Delivered through **ast-grep** (`sg`), already shipped and wrapped
(`internal/extract/astgrep`); tree-sitter under the hood, run via
`toolrun.Runner`. In-process tree-sitter Go bindings were rejected (CGO; breaks
the static-binary build — see research doc).

No new Go dependency, no CGO, binary unchanged.

---

## 2. De-duplication: ast-grep's lane vs existing tools

| Producer                                      | Lane (keep)                                                                                |
| --------------------------------------------- | ------------------------------------------------------------------------------------------ |
| go/packages, dependency-cruiser, grimp, cargo | dependency **edges** — who imports whom                                                    |
| SCIP (scip-go/-ts/-python, rust-analyzer)     | symbol **references + fan-in** (feeds `FileFact`), per-edge strength                       |
| **ast-grep (this design)**                    | **syntactic surface** — declarations, exported-ness, decorators/attributes, routes → roles |

**Reuse question (SCIP):** SCIP descriptors encode name/kind/visibility, but
archfit's reader emits opaque keys and SCIP is indexer-gated/partial. This layer
must work universally and cheaply across all four languages with no indexer.
ast-grep is therefore the declaration source; SCIP-descriptor decoding is a
possible precision upgrade, not required. SCIP keeps its lane (references,
fan-in, strength) untouched — no duplication.

---

## 3. Data model

`SyntaxFact` lives in `internal/model/diagnostic`, beside `FileFact` (same
precedent: neutral, score-free, auto-serialized).

```go
type SyntaxFact struct {
    Language  string `json:"language"`            // go|typescript|python|rust
    File      string `json:"file"`                // repo-relative, slash
    Kind      string `json:"kind"`                // function|method|class|struct|interface|trait|enum|type_alias|annotation|route
    Name      string `json:"name"`
    Exported  bool   `json:"exported,omitempty"`
    StartLine int    `json:"start_line"`
    EndLine   int    `json:"end_line,omitempty"`
    Role      string `json:"role,omitempty"`            // handler|service|repository|domain (derived)
    RoleConf  string `json:"role_confidence,omitempty"` // high|medium|low
    Evidence  string `json:"role_evidence,omitempty"`   // e.g. "decorator @Controller", "path contains repository"
    Framework string `json:"framework,omitempty"`       // for routes: gin|fastapi|express|axum|…
}
```

`Diagnostic` gains a `SyntaxFacts []SyntaxFact` field (json `syntax_facts`,
omitempty). Sorted `(File, StartLine, Kind, Name)` before emission for golden
stability.

These roles are **file/declaration-level** and are distinct from archfit's
existing module-level `config.ModuleRole`
(composition_root/adapter/core/shared_model/generated/test). Do not conflate.

---

## 4. Producer (all four languages)

### 4.1 Port

`ports.SyntaxProvider` (mirrors `ports.PatternProvider`); the existing
`astgrep.Adapter` satisfies it.

```go
type SyntaxProvider interface {
    Name() string
    Syntax(ctx context.Context, s scope.Scope, langs []string) ([]diagnostic.SyntaxFact, diagnostic.Coverage, error)
}
```

### 4.2 Mechanism

- **Embedded rules** (`go:embed`) — one curated YAML per language:
  `internal/extract/astgrep/rules/{go,typescript,python,rust}.yml`. Replaces the
  brief's `.scm` files; versioned in-repo.
- **One `sg scan --inline-rules "<yaml>" --json=compact .` per language** —
  rules separated by `---`, each match carries `ruleId` → `Kind`. No temp files,
  pure `Runner`. `--inline-rules` exists since ast-grep 0.33.x (pinned 0.43.0 is
  fine; validated on 0.44.0).
- **Absent `sg`** → `Coverage{Status:"absent"}`, empty facts, never an error.
- **JSON-shape guard** test asserts `ruleId`/`range`/`metaVariables` (shape not
  contractually stable across 0.4x).

### 4.3 Node kinds & detection per language (validated empirically)

| Lang       | Declaration kinds                                                                                                                       | Exported / visibility                                           | Annotations                       | Route detection (capped frameworks)                                                                                                                                                  |
| ---------- | --------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Go**     | `function_declaration`, `method_declaration`, `type_declaration`/`type_spec` (struct/interface via child)                               | name `has: {field: name, regex: '^[A-Z]'}`                      | none (Go has none)                | `call_expression` selector verb (`GET/POST/Get/Post/HandleFunc/Handle`) + string path: net/http, gin, echo, chi, fiber, gorilla/mux; signature `http.ResponseWriter`/`*http.Request` |
| **TS**     | `function_declaration`, `class_declaration`, `interface_declaration`, `enum_declaration`, `type_alias_declaration`, `method_definition` | `inside: {kind: export_statement}`                              | `decorator`                       | `app.get(...)` call_expression (express/koa/fastify); `@Controller`/`@Get` decorators (nest)                                                                                         |
| **Python** | `function_definition`, `class_definition` (`function_definition` matches decorated fns too — no `decorated_definition` trap)            | name `has: {field: name, regex: '^[^_]'}`; `__all__` (optional) | `decorator` (`has: {kind: call}`) | `@app.get`/`@router.post`/`@app.route` (fastapi/flask/starlette/aiohttp); `urlpatterns` `path()`/`re_path()` (django)                                                                |
| **Rust**   | `function_item`, `struct_item`, `enum_item`, `trait_item`, `impl_item`, `mod_item`                                                      | `has: {kind: visibility_modifier}` (pub/pub(crate))             | `attribute_item` (`#[...]`)       | `attribute_item` `#[get(...)]`/`#[post(...)]` (actix/rocket); `Router::new().route(...)` builder (axum/warp)                                                                         |

All rows confirmed by running `sg scan` against fixtures (Go/TS/Python/Rust).

---

## 5. Role derivation (pure, core ring)

New core package `internal/syntax` (added to the arch_test core-ring allow-list).
Pure function over `[]SyntaxFact` + module path config → role/confidence/evidence.
No `os`, no subprocess.

Heuristics (per brief, language-agnostic):

- `handler` — route annotation/registration; or path `handler|controller|routes`;
  or Go signature `http.ResponseWriter`/`*http.Request`.
- `repository` — name `*Repository`; or path `repository|repo|storage|persistence`.
- `service` — name `*Service`; or path `service|usecase|application`.
- `domain` — path `domain|model|entity`.

Confidence: annotation/signature evidence → `high`; name → `medium`; path-only →
`low`. Evidence string records the reason.

---

## 6. The role → graph-node join (the load-bearing mechanism)

`SyntaxFact` is per-**file**; rules iterate `g.Edges()`. Edge node granularity
**differs by language** (verified by reading each extractor):

| Lang       | Graph node granularity                                                 | File→node resolution                                                                                                          |
| ---------- | ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| **TS**     | file → file                                                            | identity (node path == file path)                                                                                             |
| **Go**     | file → **package** (import path); `belongs_to` edges link file→package | aggregate file roles to the package via `EdgeKindBelongsTo` (`EdgesTo(pkg)`)                                                  |
| **Python** | **dotted module** (`billing.service`)                                  | normalize dotted↔slash (`billing.service` ↔ `billing/service.py`) — this is archfit's known dotted-vs-slash seam; fix it here |
| **Rust**   | `crate` / `crate::mod`                                                 | reuse `modgraph.ModuleKeyResolver` (`crate/src/a/b.rs → crate::a::b`) in reverse to map file roles to nodes                   |

Design: a **`NodeRoleIndex`** built once per run from `[]SyntaxFact` + the graph,
mapping each graph node ID → set of `(role, confidence)`. It encapsulates the
per-language resolution above so rules stay language-agnostic:

```go
idx := syntax.BuildNodeRoleIndex(g, syntaxFacts, moduleViews)
// idx.RolesFor(nodeID) -> []RoleHit{Role, Confidence}
```

`forbidden_role_dependency` then iterates edges and asks
`idx.RolesFor(e.From)` / `idx.RolesFor(e.To)`. Verification during
implementation: run `archfit analyze --json` on one repo per language and assert the
node IDs resolve. This join — not extraction — is the real risk; it gets explicit
tests per language.

---

## 7. Gate control

### 7.1 Wire the `gate:` field for all rules (fixes a latent bug)

Today `RuleDef.Gate` (`off|warn|fail`) is stored but **ignored**: every rule
finding is `Kind:"gate"`, so any `StatusNew` finding fails the verdict regardless
of `gate:`; severity is also ignored. Proper gate control wires it for **all**
rule types (consistency — otherwise it would work only on new rules):

- `gate: off` → rule skipped (not instantiated / no findings).
- `gate: warn` → findings emitted as `Kind:"advisory"` → surface in output,
  verdict `warn`, **non-blocking**.
- `gate: fail` (and empty/unset, the default) → `Kind:"gate"` → **blocking** on
  `StatusNew`/`StatusExpiredExcept`.

Implemented centrally: the engine sets each finding's `Kind` from its rule's
`gate:` (or rules read `def.Gate` in `finding.New`). **Behavior change, scoped:**
all self-config rules are `gate: fail` (unaffected); the only `warn` users are
config-parsing tests and `archfit init`-generated rules, whose `warn` is _meant_
to be non-blocking (they carry a "promote to fail" TODO). Golden/dogfood
verified after wiring.

### 7.2 New rule types (consume syntax facts via Evidence)

`Evidence` gains `SyntaxFacts []diagnostic.SyntaxFact` (and the engine builds the
`NodeRoleIndex` from it). Role derivation runs **between extraction and the rules
stage** so roles are in `Evidence`. New types added to the `rules.New` switch:

1. **`forbidden_role_dependency` `{from_role, to_role}`** — for each edge A→B,
   if `NodeRoleIndex` gives A the `from_role` and B the `to_role`, emit a
   finding. Covers "handlers must not call repositories." **Fires only on
   `high`-confidence roles by default** (config `min_confidence` to relax to
   `medium`) — avoids failing builds on a `ServiceAccount` name guess.
2. **`public_api_max` `{module|path, max}`** — count exported declarations per
   module from `SyntaxFacts`; `> max` → finding. Covers "domain API must stay
   small." Static; no baseline.
3. **`public_api_change`** — emit one finding per exported declaration
   (fingerprint = `ruleID + module + name`). The existing **baseline/status
   stage** suppresses known ones; newly-added public API surfaces as
   `StatusNew`. Mirrors `new_cross_module_dependency` — reuses existing plumbing,
   **no new baseline storage**. Defaults to `gate: warn` (advisory drift signal).

New `RuleDef` fields: `FromRole`, `ToRole`, `Max`, `MinConfidence`
(`FromLayer`/`ToLayer` are already-unused precedent for adding fields).

### 7.3 Scorecard

`gate`-kind findings feed `boundary_integrity` automatically (existing
`activeGateFindings` path). Advisory findings do not affect the verdict.

### 7.4 Unknown rule type = error

`rules.New` currently silently skips unknown `type:`. Adding three types is the
moment to make unknown types a config error (fail fast), so a typo'd rule never
silently disables a gate.

---

## 8. Consumers (off-gate facts)

1. **`scan` "Syntax surface" section** — per-module declaration counts, public
   API list, detected roles/routes.
2. **`agent_tasks` evidence** — relevant declarations + role + file:line for the
   files a task references (compact agent context).
3. **Public API surface summary** — per-module exported-declaration list/count.

When `sg` is absent, all three behave as today (no regression, no false green).

---

## 9. Self-config / dogfood

- Enable `analyzers.syntax` (facts) in `.archfit.yaml` for dogfooding — **facts only,
  off-gate**.
- Do **not** enable `public_api_max`/`forbidden_role_dependency` as `gate: fail`
  on archfit itself (or set generous `gate: warn`) — otherwise `make archfit`
  could fail on archfit's own public surface. Keep the dogfood gate green.

---

## 10. Accepted ceilings (deliberate)

- **Heuristic roles, capped frameworks.** Route detection covers a named set per
  language (above); confidence-tagged; config-extensible. Upgrade trigger: a
  needed framework is absent → add a rule line.
- **Role gating defaults to high confidence** — name/path-only roles inform
  reports but don't fail builds unless `min_confidence` is lowered.
- **Syntactic, not semantic.** No type/import resolution or call graph (SCIP's
  lane). Roles can misfire; that's why low confidence is report-only.

---

## 11. Constraints honored

- **Build:** no Go deps, no CGO; `sg` subprocess; binary unchanged.
- **arch_test:** `SyntaxFact` in `internal/model/diagnostic` (stdlib); producer
  in `internal/extract/astgrep` (adapter, uses `Runner`); roles + `NodeRoleIndex`
  in new core pkg `internal/syntax` (**add to core-ring allow-list**); rules in
  `internal/rules` (core) may import `model/diagnostic` (already imports model
  pkgs). No ring violations.
- **Config:** `analyzers.syntax` mirrors `analyzers.scip`; new `RuleDef` fields validated.
- **No false green:** absent `sg` → `n/a`, never invented facts.

---

## 12. Rejected alternatives

See `docs/archived/research/tree-sitter-for-archfit.md` §"Rejected alternatives"
(in-process bindings, zig-cc CGO cross toolchain, purego + per-arch shared libs,
wazero, raw tree-sitter CLI). All cost more than reusing the bundled ast-grep
subprocess for the same facts.

---

## 13. Deviations from design (as shipped)

Minor implementation details that differ from the design text above:

- **`DeriveRoles` signature** — the design showed a `moduleViews` parameter for
  module-context refinement; the shipped function takes only `[]SyntaxFact` and
  the language string. Module context was not needed for the current heuristics.
- **Go file → package resolution in `NodeRoleIndex`** — the design referenced
  `EdgeKindBelongsTo` edges to map files to packages. The Go extractor emits no
  `belongs_to` edges, so the implementation uses a directory-mapping approach
  instead: the file's directory path is matched against graph node keys.
- **`forbidden_role_dependency` lookup** — the design mentioned consulting raw
  `SyntaxFacts` in addition to `NodeRoleIndex`. The shipped rule uses
  `NodeRoleIndex` only; raw facts are not needed once the index is built.
- **`public_api_max` / `public_api_change` module scoping** — the design did not
  specify the scoping mechanism. Both rules use `config.ModuleMap.ModuleFor` to
  resolve a file's owning module, consistent with how other rules scope findings.
- **`public_api_max` in self-config** — set to `max: 1500` (advisory `gate: warn`)
  to stay green while archfit's own catch-all `internal` module resolves ~1271
  declarations. Tighten per module once the module map is split.
- **`public_api_change` warn-by-default** — `defaultGateForType("public_api_change")` returns `"warn"` at construction time in `rules.New`. When `gate:` is absent in the rule definition, `public_api_change` gets `warn` (advisory) instead of the global default `""` (= `fail`). This is deliberate: the rule surfaces newly-added public API as a drift signal, not a hard gate.
