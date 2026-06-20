# Architecture Review: codegraph

**Reviewer:** Independent (blind — no archfit tooling used)
**Date:** 2026-06-20
**Repo HEAD:** `0e2789a`
**Primary language:** TypeScript (~196 .ts files, 46 747 source lines)
**Tool:** code graph indexer / SCIP tool exposed as an MCP server and CLI

---

## 1. System Map

### 1.1 Package structure

Single npm package (`@colbymchenry/codegraph` v0.9.9). No monorepo workspaces. One `tsconfig.json` targeting CommonJS. No ESLint config, no dependency-cruiser config.

### 1.2 Entrypoints

| Entrypoint | Role |
|---|---|
| `src/bin/codegraph.ts` (1 727 L) | CLI — commander-based, all user-facing commands |
| `src/mcp/index.ts` | MCP server — spawns daemon, registers MCP protocol |
| `src/index.ts` (1 103 L) | Package facade / public API — `CodeGraph` class |

### 1.3 Source modules

| Module | Files | Lines | Churn (6 mo) |
|---|---|---|---|
| `extraction/` | ~18 | ~11 000 | 203 commits |
| `resolution/` | ~40 | ~9 000 | 119 commits |
| `mcp/` | ~8 | ~5 000 | 88 commits |
| `db/` | ~4 | ~3 200 | 53 commits |
| `bin/` | ~3 | ~1 800 | 43 commits |
| `graph/` | ~3 | ~1 500 | low |
| `context/` | ~3 | ~1 700 | 24 commits |
| `search/` | ~5 | ~1 000 | 11 commits |
| `sync/` | ~6 | ~1 200 | 14 commits |
| `installer/` | ~6 | ~1 500 | 78 commits |
| `ui/`, `upgrade/` | ~5 | ~500 | low |

### 1.4 Intended layering

The repo ships a `.archfit.yaml` that defines 12 modules, all placed in a single `"core"` layer with one `no-forbidden-deps` rule set to `gate: warn`. In practice this means **no layer order is enforced** — every module is considered a peer of every other.

No dependency-cruiser config, no ESLint import rules. There is no mechanically enforced boundary between layers.

### 1.5 Observed dependency direction

```
bin ──────────────────────────────> index (facade)
mcp/tools.ts ──> index + types + sync + search + extraction/generated-detection
index.ts ──> db + db/queries + extraction + resolution + graph + search + sync + context
extraction ──> db/queries + resolution/frameworks + types
resolution ──> db/queries + extraction/generated-detection + extraction/grammars (2 spots)
db/queries ──> search/query-utils + search/query-parser + extraction/generated-detection
graph ──> db/queries + types
context ──> db/queries + types
search ──> types
sync ──> types + utils
```

Notable: `extraction` and `resolution` mutually import each other (cycle), and `db/queries` imports from `search` and `extraction` (inversion — persistence layer imports domain-logic layer).

---

## 2. Evidence with File:Line Citations

### 2.1 Extraction ↔ Resolution mutual cycle

```
src/extraction/index.ts:24    import { detectFrameworks } from '../resolution/frameworks';
src/extraction/index.ts:25    import type { ResolutionContext } from '../resolution/types';
src/extraction/tree-sitter.ts:31   import { ... } from '../resolution/frameworks';

src/resolution/callback-synthesizer.ts:27  import { isGeneratedFile } from '../extraction/generated-detection';
src/resolution/frameworks/play.ts:20       import { isPlayRoutesFile } from '../../extraction/grammars';
src/resolution/frameworks/drupal.ts:50     import { generateNodeId } from '../../extraction/tree-sitter-helpers';
```

Extraction calls `detectFrameworks` from resolution (to know which extractors to run). Resolution calls back into extraction for `isGeneratedFile`, `isPlayRoutesFile`, `generateNodeId`. These two modules are tightly intertwined — changing either requires understanding both.

### 2.2 DB/queries imports domain logic (layer inversion)

```
src/db/queries.ts:21  import { kindBonus, nameMatchBonus, scorePathRelevance } from '../search/query-utils';
src/db/queries.ts:22  import { parseQuery, boundedEditDistance } from '../search/query-parser';
src/db/queries.ts:23  import { isGeneratedFile } from '../extraction/generated-detection';
```

`db/queries.ts` (1 817 L, 32 commit churn) is the SQLite prepared-statement layer. It reaches up into `search/` for ranking helpers and into `extraction/` for a file-classification predicate. A persistence layer importing a domain-search layer inverts the expected dependency direction. Changing the search ranking model requires touching the database query file.

### 2.3 QueryBuilder injected into domain modules directly

```
src/extraction/index.ts:18        import { QueryBuilder } from '../db/queries';
src/resolution/index.ts:10        import { QueryBuilder } from '../db/queries';
src/resolution/callback-synthesizer.ts:25  import type { QueryBuilder } from '../db/queries';
src/graph/queries.ts:8            import { QueryBuilder } from '../db/queries';
src/graph/traversal.ts:8          import { QueryBuilder } from '../db/queries';
src/context/index.ts:23           import { QueryBuilder } from '../db/queries';
```

`QueryBuilder` is the concrete SQLite prepared-statement container. It is passed directly into `extraction`, `resolution`, `graph`, `context`. There is no repository or port interface — domain logic is coupled to the concrete persistence implementation. Swapping the DB (e.g., to in-memory for testing) requires threading a compatible `QueryBuilder` everywhere.

### 2.4 God files

| File | Lines | 6-mo churn |
|---|---|---|
| `src/extraction/tree-sitter.ts` | 4 214 | 68 commits |
| `src/mcp/tools.ts` | 3 375 | 46 commits |
| `src/resolution/import-resolver.ts` | 1 825 | — |
| `src/db/queries.ts` | 1 817 | 32 commits |
| `src/bin/codegraph.ts` | 1 727 | 39 commits |
| `src/resolution/callback-synthesizer.ts` | 1 721 | — |

`mcp/tools.ts` (3 375 L) imports from `index`, `directory`, `sync/worktree`, `sync`, `types`, `search/query-utils`, `utils`, `extraction/generated-detection` — 8 distinct module sources in one file. It is both the MCP schema layer and the agent-output formatting engine. These responsibilities should be separate.

`extraction/tree-sitter.ts` (4 214 L, highest churn) is the single file doing grammar loading, WASM lifecycle, tree-sitter query dispatch, and per-language node extraction for all 30+ languages.

### 2.5 No import-boundary enforcement

No ESLint config (checked for `.eslintrc*`, `eslint.config*`) — not present.
No `.dependency-cruiser.*` — not present.
`.archfit.yaml` places all modules in one layer with `gate: warn` only — structural violations generate warnings, not CI failures.

---

## 3. Balanced Coupling Analysis

Balanced Coupling model per Khononov: coupling is **balanced** when (STRENGTH × DISTANCE) is inversely proportional to VOLATILITY — high-strength couplings are acceptable at low distance/low volatility, dangerous at high distance/high volatility.

### 3.1 Coupling classification

| Coupling | Strength | Distance | Volatility | Balance |
|---|---|---|---|---|
| `index.ts` → all domain modules | Functional (calls concrete methods) | Low (same package) | Medium (changes often) | Acceptable — facade role, expected fan-out |
| `extraction` ↔ `resolution` (cycle) | Intrusive (direct internal function calls) | Medium (peer modules) | High (both top-2 churn dirs) | **UNBALANCED** — high strength + medium distance + high volatility |
| `db/queries.ts` → `search/query-utils` | Functional (calls ranking functions) | High (crosses conceptual layer: persistence → domain) | High (DB + search both churn) | **UNBALANCED** — high strength + high distance + high volatility |
| `resolution` → `db/queries` (QueryBuilder) | Intrusive (direct use of concrete DB class) | Medium-High (domain → infrastructure) | High (resolution is #2 churn) | **UNBALANCED** — intrusive strength + medium-high distance |
| `mcp/tools.ts` → many modules | Functional (calls through index, search, sync) | Low-Medium | High (mcp #3 churn, tools file #2 file churn) | Borderline — import fan-out is high but most through index facade |
| `resolution/frameworks/drupal.ts` → `extraction/tree-sitter-helpers` | Intrusive (calls `generateNodeId`) | High (sub-module of resolution crosses into extraction internals) | Medium | **UNBALANCED** — intrusive cross-layer |
| `search/query-*` → `types` | Contract (shared type definitions) | Low | Low (types stable) | Balanced — correct shared kernel pattern |
| `context` → `db/queries` (QueryBuilder) | Intrusive | Medium-High | Medium | Borderline unbalanced |

### 3.2 TS-specific observations

- **No barrel-file abuse detected.** Each module's `index.ts` re-exports its own internals cleanly.
- **Deep relative imports across module boundaries** are common: `../../extraction/grammars`, `../../types` from `resolution/frameworks/`. Acceptable for a flat single-package codebase but increases refactoring cost.
- **Type-only vs runtime coupling:** `mcp/tools.ts` uses `import type` for `Node`, `Edge`, etc. from `types` — correct. However `resolution/callback-synthesizer.ts` uses `import type { QueryBuilder }` but still receives `QueryBuilder` at runtime through dependency injection — the type-only import is accurate but the runtime dependency still exists.
- **No import cycles at the module file level beyond `extraction ↔ resolution`** (confirmed by manual grep tracing).

---

## 4. Banded Scorecard

Scoring follows the standard rubric: 1–10 numeric, banded (1–3 Critical, 4–5 Poor, 6–7 Adequate, 8–9 Good, 10 Excellent).

| Dimension | Score | Band | Confidence | Notes |
|---|---|---|---|---|
| Modularity / Cohesion | 5 | Poor | High | God files (4 214 L, 3 375 L) with mixed responsibilities. `extraction/tree-sitter.ts` handles grammar loading, WASM lifecycle, and per-language extraction. `mcp/tools.ts` handles protocol, output formatting, and ranking logic. |
| Coupling Balance | 4 | Poor | High | Three confirmed unbalanced couplings (extraction↔resolution cycle; db→search; resolution→db-concrete). No layer enforcement. |
| Dependency Direction / Cycles | 4 | Poor | High | `extraction ↔ resolution` is a confirmed mutual cycle. `db/queries.ts` inverts the dependency direction by importing `search` and `extraction`. No tooling to detect or prevent regressions. |
| Encapsulation | 5 | Poor | Medium | `QueryBuilder` (concrete DB) exposed as public API through `src/index.ts:60`. Internal helpers (`generateNodeId`, `isPlayRoutesFile`) crossed directly from resolution into extraction internals. No interfaces/ports between layers. |
| Blast Radius / Change Locality | 4 | Poor | High | `src/types.ts` (585 L, 25 commit churn) is imported by nearly every module — a change to any type is a blast across the system. `db/queries.ts` is similarly central (32 commits). God files concentrate risk. |
| Testability | 5 | Poor | High | 65 test files, 17 using mocks. `QueryBuilder` is a concrete class with no interface — domain logic that takes it cannot be tested without a real (or in-memory) SQLite DB. `extraction/tree-sitter.ts` at 4 214 lines has no clear seam for unit testing individual language extractors. Integration tests require spawning `dist/bin/codegraph.js`. |
| Architecture Fitness | 3 | Critical | High | No ESLint import rules. No dependency-cruiser rules. `.archfit.yaml` has `gate: warn` only and puts all modules in one undifferentiated layer — there is no enforced fitness function that would catch the layer inversions or mutual cycles identified above. Violations accumulate silently. |

**Overall band: Poor (4.3 average)**

---

## 5. Top Findings

### Finding 1 — Extraction ↔ Resolution mutual import cycle (Unbalanced: intrusive, medium distance, high volatility)

`src/extraction/index.ts:24` imports `detectFrameworks` from `resolution/frameworks`.
`src/resolution/frameworks/play.ts:20` imports `isPlayRoutesFile` from `extraction/grammars`.
`src/resolution/frameworks/drupal.ts:50` imports `generateNodeId` from `extraction/tree-sitter-helpers`.
`src/resolution/callback-synthesizer.ts:27` imports `isGeneratedFile` from `extraction/generated-detection`.

**BC dimension:** Intrusive strength at medium distance between the two most volatile modules (extraction: 203 commits, resolution: 119 commits). A change to either module's internal API risks the other.

**Root cause:** Framework detection and extraction are genuinely interleaved — extractors need to know which frameworks are active; resolvers need extraction utility functions. The functions that cross the boundary (`generateNodeId`, `isPlayRoutesFile`, `isGeneratedFile`) should live in a shared kernel (e.g., `src/common/` or `src/types/helpers/`) with no module owning them — eliminating the cycle.

### Finding 2 — `db/queries.ts` imports `search` and `extraction` (layer inversion, unbalanced)

`src/db/queries.ts:21–23` imports from `search/query-utils`, `search/query-parser`, and `extraction/generated-detection`.

**BC dimension:** High strength (functional calls to ranking/parsing functions) at high conceptual distance (persistence layer imports domain logic), with high volatility (32 commits on `db/queries.ts`, 11 on `search/`).

**Root cause:** Search-ranking scoring functions (`kindBonus`, `nameMatchBonus`, `scorePathRelevance`, `parseQuery`) were placed in `search/` but are consumed directly inside SQL prepared statements in `db/`. The query layer accumulated domain logic it should not own. Fix: move the scoring functions that are invoked inside SQL to a `db/`-owned or `shared/` module, or make the DB layer accept scoring callbacks injected by the caller.

### Finding 3 — `QueryBuilder` (concrete DB class) injected into all domain modules without an interface

Six modules (`extraction`, `resolution`, `graph`, `context`, `resolution/callback-synthesizer`, plus `index.ts`) directly import `QueryBuilder` from `db/queries`.

**BC dimension:** Intrusive coupling (concrete class, no interface) at medium-high distance (domain → infrastructure). Volatility: `db/queries.ts` has 32 commit churn.

**Root cause:** No repository pattern or port interface. Testing domain logic in isolation requires a real or in-memory SQLite DB. Extracting a `IQueryBuilder` interface (or splitting `QueryBuilder` into read/write ports) would let extraction and resolution be unit-tested with in-memory fakes.

### Finding 4 — `mcp/tools.ts` is a god file (3 375 L) mixing concerns

`src/mcp/tools.ts` imports from 8 distinct module sources and combines: MCP tool schema definitions, agent output formatting, file-cluster ranking heuristics (lines 2222–2357), security validation (`validatePathWithinRoot`), and worktree detection. It is the highest-churn non-extraction file (46 commits).

**BC dimension:** High internal complexity, multiple axes of change. Any change to ranking heuristics, output format, or security policy requires editing the same 3 375-line file, increasing merge conflict probability and cognitive load.

### Finding 5 — No enforced architecture fitness functions

No ESLint import rules, no dependency-cruiser rules. `.archfit.yaml` maps all 12 modules to a single `core` layer with `gate: warn` — meaning the layer inversions and mutual cycles documented above generate at most advisory warnings, not CI failures. The archfit config was scaffolded by `archfit init` and the TODO comment in the file (`# TODO: review and promote gate: warn to gate: fail`) was never acted on.

**BC dimension:** Architecture fitness band = Critical. Without enforced checks, the structural problems identified will worsen as the codebase grows.

---

## 6. Biggest Structural Risk

The **extraction ↔ resolution mutual cycle** combined with **both modules being the highest-churn directories in the repo** (extraction: 203 commits, resolution: 119 commits) is the highest structural risk. These are the two most actively developed modules, and they are tightly coupled with no interface between them. Any refactor of extraction's grammar or AST API propagates immediately into resolution, and vice versa. The absence of any enforced boundary means the cycle will deepen with each new language or framework added. This is the classic distributed-monolith pattern at the intra-package level: two modules that appear separate but cannot be changed independently.

Secondary risk: `db/queries.ts` (1 817 L, 32 commit churn) accumulating both persistence logic and domain search-ranking logic inverts the dependency direction in a file that is already a high-churn hub. Any growth in search sophistication (new ranking signals, query-language features) will be forced into the DB layer.

---

## 7. Coverage Gaps and Low-Confidence Areas

- **No dependency-cruiser or madge run was performed.** A full graph traversal would enumerate all cycles, not just the ones reachable by manual grep. There may be additional cycles among the 40+ `resolution/frameworks/` files.
- **Test coverage percentage not verified.** Vitest is configured with v8 coverage provider, but the coverage report was not run in this review. The 65-test / 17-mocks count is a structural signal, not a coverage percentage.
- **Runtime behavior of daemon/watcher not assessed.** `src/sync/watcher.ts` (639 L) and `src/mcp/daemon.ts` (618 L) were reviewed structurally but not traced for concurrency or IPC coupling issues.
- **`src/extraction/tree-sitter.ts` (4 214 L) was not fully read.** The internal structure of the per-language extraction dispatch was traced only at the import boundary. There may be additional coupling embedded in that file.
- **No tsc type-check or lint run.** TypeScript errors, if any, were not observed. The absence of an ESLint config was verified; `tsc` was not invoked.

---

## Appendix: Module Dependency Map (observed)

```
types.ts  (shared kernel, imported everywhere)
    ↑
    |
db/queries.ts ←── search/query-utils
    ↑             search/query-parser
    |             extraction/generated-detection
    |
extraction/index.ts ──────→ resolution/frameworks (detectFrameworks)
    ↑                              ↓
    └──────── resolution/callback-synthesizer
              resolution/frameworks/play.ts → extraction/grammars
              resolution/frameworks/drupal.ts → extraction/tree-sitter-helpers
    |
graph/ ──→ db/queries
context/ ──→ db/queries
    |
index.ts (facade) ──→ db, extraction, resolution, graph, search, sync, context
    ↑
bin/codegraph.ts
mcp/tools.ts ──→ index + sync + search + extraction/generated-detection
```
