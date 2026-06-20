# architect-skill quick sweep — codegraph

## 1. Scope, refs, dirty-state risk

- Target: `/Users/alexei/Workspace/codegraph`.
- Mode: read-only source quick sweep. I did not edit source. I only wrote this report under `/Users/alexei/Workspace/archfit/reports/.../codegraph/architect-sweep.md`.
- HEAD: `0e2789ab712b` on `main` (`origin/main`).
- Latest tag: `v0.9.9`.
- Delta baseline: `v0.9.8` exists and was used.
- Dirty state before/after sweep was unchanged:
  - deleted tracked generated/cache file: `assets/__pycache__/generate-waitlist.cpython-313.pyc`.
  - untracked local analysis/config: `.archfit-cache/`, `.archfit.full.yaml`, `.archfit.yaml`, `.claude/skills/gitnexus/`, `bun.lock`.
- Dirty-state risk: low for source architecture, because no tracked source files were dirty. Medium for rerunning archfit, because untracked `.archfit*.yaml` and `bun.lock` can change tool/config interpretation.

## 2. Intent evidence with file refs

Reconstructed intent; no user interview. Confidence is capped below full review because the working model was not user-validated.

- Product purpose: local-first code intelligence library + CLI + MCP server. It parses supported codebases with tree-sitter, stores graph data in SQLite FTS5, and exposes the graph over MCP (`CLAUDE.md:5-7`).
- Intended pipeline: `files → ExtractionOrchestrator → DB → ReferenceResolver → GraphQueryManager/GraphTraverser → ContextBuilder` (`CLAUDE.md:36-48`).
- Public API: `src/index.ts` / `CodeGraph` is the library facade; CLI and MCP also drive it (`CLAUDE.md:48-52`, `src/index.ts:129-136`).
- Module intent: db, extraction, resolution/frameworks, graph, context, search, sync, MCP, installer, upgrade, UI (`CLAUDE.md:50-82`).
- Agent-facing guidance intent: `src/mcp/server-instructions.ts` is the single source of truth for MCP initialize guidance (`CLAUDE.md:88-90`, `CLAUDE.md:260`).
- Product success metric: answer structural/flow questions with a few codegraph calls and minimal Read/Grep; dynamic-dispatch coverage must be end-to-end (`CLAUDE.md:92-135`).
- README confirms public claims: benchmarked `codegraph_explore` primary tool (`README.md:105-121`, `README.md:214`), impact analysis and always-fresh watcher (`README.md:225-227`), 20+ languages and 14 frameworks (`README.md:228-229`), local-only SQLite (`README.md:231`), MCP tool guidance (`README.md:409`, `README.md:509`).
- Package/runtime intent: npm package `@colbymchenry/codegraph` version `0.9.9`; build is `tsc + copy assets`; tests via Vitest; Node `>=20 <25` (`package.json:3`, `package.json:16`, `package.json:21`, `package.json:53-54`).
- Type-safety intent: strict TypeScript with unused and return checks enabled; tests excluded from `tsconfig` (`tsconfig.json:11-33`).
- Archfit intent/config: TypeScript, SCIP, GitNexus, complexity, and clone tools enabled in `.archfit.full.yaml`; one broad `core` layer; `no-forbidden-deps` is `gate: warn`, not fail (`.archfit.full.yaml:8-16`, `.archfit.full.yaml:23`, `.archfit.full.yaml:96-99`).

## 3. System map

- Language/package units:
  - Root Node/TypeScript package with `package-lock.json` tracked. `bun.lock` is present but untracked.
  - Site subpackage under `site/` with Astro and its own `package.json`.
- Tracked scope from git: 295 files total; `src/` 130, `__tests__/` 69, `docs/` 10, `scripts/` 33, `.github/` 2, `site/` 32.
- Source modules:
  - `src/index.ts`: `CodeGraph` facade/lifecycle/index/sync/watch/query API (`src/index.ts:183`, `src/index.ts:239`, `src/index.ts:327`, `src/index.ts:431`, `src/index.ts:854-916`, `src/index.ts:1057-1061`).
  - `src/db/`: SQLite schema and prepared query layer (`src/db/schema.sql`, `src/db/queries.ts:174`, `src/db/queries.ts:1727-1755`).
  - `src/extraction/`: file scan, grammar loading, worker parse, `TreeSitterExtractor`, non-tree-sitter extractors (`src/extraction/index.ts:507`, `src/extraction/index.ts:595`, `src/extraction/index.ts:1329`, `src/extraction/tree-sitter.ts:213`).
  - `src/resolution/`: import/name/framework resolution and synthesized edges (`src/resolution/index.ts:183`, `src/resolution/index.ts:237`, `src/resolution/index.ts:649-655`, `src/resolution/index.ts:730`, `src/resolution/index.ts:763`).
  - `src/graph/`: traversal/query operations.
  - `src/context/`: context building and formatting.
  - `src/mcp/`: MCP tools, daemon/proxy/session/engine (`src/mcp/tools.ts:372-494`, `src/mcp/daemon.ts:2-35`).
  - `src/sync/`: watcher, git hooks, worktree detection (`src/sync/watcher.ts:4-31`, `src/sync/watcher.ts:147-160`).
  - `src/installer/`: agent target registry and config writers.
  - `src/upgrade/`: self-update flow.
- Public interfaces:
  - npm package entry: `dist/index.js` / `dist/index.d.ts`.
  - CLI binary: `codegraph`.
  - MCP tools: `codegraph_explore`, `codegraph_node`, search/callers/callees/impact/status/files.
  - Config/runtime env vars: e.g. `CODEGRAPH_MCP_TOOLS`, `CODEGRAPH_ADAPTIVE_EXPLORE`, `CODEGRAPH_DAEMON_IDLE_TIMEOUT_MS`, `CODEGRAPH_WATCH_DEBOUNCE_MS`, `CODEGRAPH_MAX_DIR_WATCHES` (`src/mcp/tools.ts:563-657`, `src/mcp/daemon.ts:22-23`, `src/mcp/engine.ts:201-204`, `src/sync/watcher.ts:56-62`).
- Deploy units:
  - npm package and platform bundles via manual Release workflow (`.github/workflows/release.yml:18`, `.github/workflows/release.yml:86`, `.github/workflows/release.yml:116-119`, `scripts/build-bundle.sh:58-60`).
  - GitHub Pages site via push to `site/**` (`.github/workflows/deploy-site.yml:4-8`, `.github/workflows/deploy-site.yml:29-41`).
- Generated/local state:
  - `.codegraph/codegraph.db` exists. Read-only SQLite snapshot: 221 indexed files, 3,487 nodes, 13,983 edges, 0 unresolved refs. Metadata says built with codegraph `1.0.1` / extraction version `24`, while source package is `0.9.9` and `src/extraction/extraction-version.ts` exports `1`; use as a coverage hint, not a clean source-version proof.
  - `.gitnexus/meta.json` is fresh to HEAD `0e2789a`, indexed at `2026-06-20T08:35:01Z`, 200 files / 3,574 nodes / 10,667 edges.

## 4. Tool coverage and commands run

### Used

- Skill files read and followed:
  - `architecture-review/SKILL.md`
  - `methodology-balanced-coupling/SKILL.md`
  - `architecture-design/SKILL.md`
  - `tools-code-search/SKILL.md`
  - `tools-typescript/SKILL.md`
  - `tools-codegraph/SKILL.md`
  - `tools-gitnexus/SKILL.md`
  - `methodology-architecture-fitness/SKILL.md`
  - `tools-report-markdown/SKILL.md`
- Local code search/read:
  - targeted reads of `CLAUDE.md`, `README.md`, `package.json`, `tsconfig.json`, `.archfit*.yaml`, source entrypoints, workflows, archfit reports.
  - `find`/`grep` tool calls for source/test/docs/workflow layout and line refs.
- TypeScript semantic check:
  - `cd /Users/alexei/Workspace/codegraph && npm exec -- tsc --noEmit --pretty false`
  - Result: passed with no output.
- Dependency graph checks:
  - `madge --circular --extensions ts src`
  - Result: 124 files processed; 5 file-level circular dependencies.
  - `depcruise src --include-only "^src" --no-config --output-type json --ts-config tsconfig.json`
  - Result: 124 modules, 240 deps, 21 circular edges, 4 orphans.
- Git/delta:
  - `git rev-parse --short=12 HEAD`
  - `git branch --show-current`
  - `git describe --tags --abbrev=0`
  - `git diff --stat v0.9.8..HEAD -- src __tests__ docs package.json package-lock.json .github scripts README.md CHANGELOG.md`
  - `git diff --name-status v0.9.8..HEAD -- ...`
  - `git log --oneline --decorate --no-merges v0.9.8..HEAD`
  - `git ls-files | python3 ...` for tracked counts.
- Read-only generated graph/index inspection:
  - Python `sqlite3.connect('file:/Users/alexei/Workspace/codegraph/.codegraph/codegraph.db?mode=ro', uri=True)` for metadata/counts/fan-in/fan-out/mtime checks.
  - GitNexus `list_repos`, `query`, `context(CodeGraph)`, `context(TreeSitterExtractor)`, `context(synthesizeCallbackEdges)`, `impact(TreeSitterExtractor)`, `impact(CodeGraph)`, `impact(synthesizeCallbackEdges)` with `repo=codegraph`.
- Existing archfit artifacts read:
  - `scan.md`, `scorecard.md`, `delta-scorecard.md`, `full.json`, `delta.json`, `llm-review*.md`.

### Missing/skipped/failed

- No final architecture scores: full review gates were not met and the user requested a quick sweep.
- No full `npm test`: quick-sweep budget; tests can spawn daemons and write temp `.codegraph` state.
- No `npm run build` or release bundle run: those write `dist/` / release artifacts.
- No fresh CodeGraph re-index: would mutate `.codegraph/`; used read-only DB and GitNexus freshness instead.
- `depcruise` without `--no-config` failed because no `.dependency-cruiser.*` config exists; reran with `--no-config`.
- GitNexus `impact` by file path failed (`Target 'src/extraction/tree-sitter.ts' not found`); reran by symbols.
- No LSP/tree-sitter external semantic pass beyond TypeScript and GitNexus/codegraph snapshots.

## 5. Full-current architecture observations

### Confirmed observations

1. **The intended layered pipeline exists, but the facade is broad.**
   - `CodeGraph` owns DB, queries, orchestrator, resolver, graph manager, traverser, context builder, locks, and watcher (`src/index.ts:129-136`).
   - GitNexus context shows `CodeGraph` imports from `src/mcp/tools.ts`, `src/mcp/engine.ts`, and `src/installer/index.ts`, and exposes many lifecycle/query methods.
   - This is a clear public facade, but it is also a high-blast-radius seam. GitNexus rates `CodeGraph` upstream impact `HIGH`: 11 impacted symbols, 4 affected processes.

2. **Extraction is cohesive but structurally heavy.**
   - `TreeSitterExtractor` spans `src/extraction/tree-sitter.ts:212-4127` per GitNexus.
   - It handles generic traversal plus many language-specific concerns, e.g. class/function extraction, inheritance, decorators, type alias members, PHP namespace refs, Objective-C class messages, instantiation, and framework hooks (`src/extraction/tree-sitter.ts:213`, `src/extraction/tree-sitter.ts:378-442`, `src/extraction/tree-sitter.ts:846-868`, `src/extraction/tree-sitter.ts:1674-1678`, `src/extraction/tree-sitter.ts:2130-2134`).
   - Archfit agrees: `src/extraction/tree-sitter.ts` is a god module at 4,214 LOC and has top complexity hotspots.
   - GitNexus rates `TreeSitterExtractor` upstream impact `HIGH`: direct imports/calls from `svelte-extractor.ts`, `vue-extractor.ts`, `razor-extractor.ts`, `parse-worker.ts`, and `extraction/index.ts`.

3. **Import cycles are real and user-visible to dependency tools.**
   - `madge` found 5 circular dependencies:
     - `extraction/tree-sitter.ts > extraction/razor-extractor.ts`
     - `extraction/tree-sitter.ts > extraction/svelte-extractor.ts`
     - `extraction/tree-sitter.ts > extraction/vue-extractor.ts`
     - `index.ts > mcp/index.ts > mcp/daemon.ts > mcp/engine.ts`
     - `index.ts > mcp/index.ts > mcp/daemon.ts > mcp/engine.ts > mcp/tools.ts`
   - `depcruise --no-config` found 21 circular edges.
   - Archfit compresses this to 2 import cycles. Architect sees the file-level shape and can name the cycles.

4. **Dynamic-dispatch synthesis is a strong, central product capability with growing internal breadth.**
   - `callback-synthesizer.ts` explicitly closes callback/observer and EventEmitter holes (`src/resolution/callback-synthesizer.ts:4-22`), React render (`src/resolution/callback-synthesizer.ts:329-337`), JSX child render (`src/resolution/callback-synthesizer.ts:835-839`), RN event channels (`src/resolution/callback-synthesizer.ts:990-1004`), and many more (`src/resolution/callback-synthesizer.ts:1650-1655`).
   - GitNexus context for `synthesizeCallbackEdges` shows fan-out to 20+ channel-specific functions and one caller, `ReferenceResolver.resolveAndPersistBatched`.
   - This is high cohesion at the package level. The risk is not "too coupled"; it is a deep hot file with many volatile framework rules.

5. **MCP agent behavior is product-critical and implemented in multiple text surfaces.**
   - Server instructions tell agents to use codegraph before Read/Grep and describe `codegraph_explore` / `codegraph_node` as primary/secondary paths (`src/mcp/server-instructions.ts:33-68`).
   - `tools.ts` repeats similar guidance in tool descriptions and output strings (`src/mcp/tools.ts:372-494`, `src/mcp/tools.ts:2478-2516`).
   - There is at least one drift-prone output string: `renderNodeSection` still says "Read `<file>` or call codegraph_node" for structural outlines (`src/mcp/tools.ts:3352`), while the surrounding product guidance tries to avoid Read fallback (`src/mcp/tools.ts:2151-2159`, `src/mcp/server-instructions.ts:64-68`).
   - This is a good quick-sweep finding candidate, but needs a focused behavior test before calling it a final finding.

6. **Runtime freshness and daemon design are explicit but platform-sensitive.**
   - Watcher design is bounded by platform: O(1) recursive watcher on macOS/Windows, per-directory on Linux (`src/sync/watcher.ts:4-31`, `src/sync/watcher.ts:283-307`).
   - Pending-file staleness is first-class (`src/sync/watcher.ts:127-141`, `src/sync/watcher.ts:171-181`).
   - Daemon design is shared per project with sockets, lockfiles, refcount, idle timeout, and dead-client sweeps (`src/mcp/daemon.ts:2-35`, `src/mcp/daemon.ts:62-71`, `src/mcp/daemon.ts:116-126`).
   - Current tests cover much of this, but local quick sweep did not validate Linux/Windows runtime behavior.

7. **Architecture fitness exists as behavior tests, but boundary fitness is weak.**
   - Strict `tsc --noEmit` passed.
   - Tests include specific architecture-sensitive behavior around daemon, watcher, no-Read file-view, budgets, security, worktree mismatch, and symbol lookup.
   - There is no repo CI workflow that runs `npm test` on PR/push. The Release workflow is manual and runs `npm ci` plus bundle build; `build-bundle.sh` runs `npm run build`, not tests (`.github/workflows/release.yml:18`, `.github/workflows/release.yml:86`, `.github/workflows/release.yml:116-119`, `scripts/build-bundle.sh:58-60`). The site workflow only builds/deploys `site/` (`.github/workflows/deploy-site.yml:4-8`, `.github/workflows/deploy-site.yml:29-41`).
   - No dependency-cruiser config exists; current archfit rule is warn-only.

## 6. Delta observations since `v0.9.8`

- Delta size: 89 files changed, 10,795 insertions, 1,991 deletions.
- Commit window: 27 non-merge commits from `v0.9.8` to `0e2789a`.
- Main changed areas by file count:
  - `__tests__`: 30 files.
  - `src/extraction`: 9 files plus `src/extraction/languages`: 9 files.
  - `src/mcp`: 7 files.
  - `src/resolution`: 6 files plus `src/resolution/frameworks`: 4 files.
  - docs/scripts/package metadata.
- Notable added files in the delta:
  - `src/extraction/razor-extractor.ts`
  - `src/extraction/wasm/tree-sitter-c_sharp.wasm`
  - `src/resolution/workspace-packages.ts`
  - `src/upgrade/index.ts`
  - `src/mcp/ppid-watchdog.ts`
  - new tests for security, daemon liveness, file-view node mode, object-literal methods, status JSON, upgrade, context ranking.
- Delta themes from changelog and commits:
  - `codegraph_explore` primary-tool shift and file-view `codegraph_node` mode (`CHANGELOG.md:90-99`, `CHANGELOG.md:19`).
  - daemon/process liveness and Windows orphan reaping.
  - watcher resource bounding.
  - cross-language impact/blast-radius expansion across 22 languages and 14 frameworks (`CHANGELOG.md:22`).
  - security fixes for symlink path traversal and config-value redaction (`CHANGELOG.md:14-15`).
  - upgrade/status freshness (`CHANGELOG.md:20-26`).
- Delta risk: high concentration in extraction/resolution/MCP agent behavior. These are core volatile domains, not peripheral changes.
- Delta observation vs baseline: architecture moved toward broader coverage and better runtime behavior, but also expanded the existing hot spots (`tree-sitter.ts`, `callback-synthesizer.ts`, `tools.ts`) rather than reducing them.

## 7. Balanced Coupling relationship records

These are quick-sweep relationship records, not final score inputs.

### BC-1: `TreeSitterExtractor` ↔ Svelte/Vue/Razor extractors and language specs

- Level: extraction package / file-level import graph.
- Strength: functional/model. The wrappers and language specs share the same AST traversal, node model, unresolved refs, and extraction semantics (`src/extraction/tree-sitter.ts:213`, `src/extraction/tree-sitter-types.ts`, `src/types.ts`).
- Distance: low-to-medium. Same package and deploy unit; separate files but same ownership. File cycles raise maintenance distance.
- Volatility: high. Language/framework coverage is core product capability and changed heavily since `v0.9.8`.
- Evidence: madge cycles with `tree-sitter.ts` ↔ `razor/svelte/vue`; GitNexus impact on `TreeSitterExtractor` is `HIGH`; archfit reports 4,214 LOC and co-change with language files.
- Severity: medium. High strength + relatively low distance is cohesive, but cycles and file size increase change cost.
- Balancing move: keep extraction knowledge close, but split a lower-level `tree-sitter-core` / `script-block-adapter` so wrappers depend inward and cycles disappear. Add a cycle gate.

### BC-2: `synthesizeCallbackEdges` ↔ per-framework dynamic-dispatch rules

- Level: resolution package / synthesized-edge subsystem.
- Strength: functional. All rules share the requirement "make static graph answer dynamic flow questions".
- Distance: low in package, but high internal breadth in one file.
- Volatility: high. New frameworks/languages add new channels.
- Evidence: `synthesizeCallbackEdges` calls callback, closure collection, EventEmitter, React, Flutter, C++, Go, Kotlin, interface override, gRPC, JSX, Vue, RN, Expo, Fabric, MyBatis, Gin, Pascal, SvelteKit channels per GitNexus; docs warn partial coverage is worse than none (`CLAUDE.md:127-131`).
- Severity: medium. Cohesive but structurally overloaded.
- Balancing move: extract channel modules behind a small `EdgeSynthesizer` contract in the same package; keep registry close to resolver; require end-to-end probe tests per channel.

### BC-3: root `CodeGraph` facade ↔ MCP/installer consumers

- Level: package public API vs adapters.
- Strength: contract for public API, but model/implementation for internal re-exports and lifecycle.
- Distance: medium. Same deployable, but public library, MCP runtime, and installer have different change vectors.
- Volatility: high. CLI/MCP behavior and embedded API changed in recent releases.
- Evidence: `CodeGraph` owns all core collaborators (`src/index.ts:129-136`); GitNexus shows imports from MCP tools/engine and installer; madge shows `index.ts > mcp/index.ts > mcp/daemon.ts > mcp/engine.ts` cycles.
- Severity: medium-high for the import cycle, low for the facade contract itself.
- Balancing move: separate core API from MCP adapter. Example target: `src/core/index.ts` for `CodeGraph`; root package index re-exports core and optionally `MCPServer` without creating a cycle; MCP imports core only.

### BC-4: agent guidance/docs ↔ MCP tool descriptions/output strings/tests

- Level: product behavior / agent protocol text.
- Strength: functional. The same behavioral rule must be consistent everywhere: agents should not fall back to Read/Grep when codegraph already returned source.
- Distance: high. README, CLAUDE.md, server instructions, MCP tool descriptions, output strings, and tests are separate files and audiences.
- Volatility: high. Recent commits retuned guidance and tool surface.
- Evidence: single-source intent (`CLAUDE.md:88-90`, `CLAUDE.md:260`), server instructions (`src/mcp/server-instructions.ts:33-68`), tool descriptions/output (`src/mcp/tools.ts:372-494`, `src/mcp/tools.ts:2478-2516`), drift candidate (`src/mcp/tools.ts:3352`).
- Severity: high as a product-behavior seam; a text drift can directly cause agent fallback and erase the product's main benefit.
- Balancing move: make guidance generated or constant-backed, and add a text invariant test over rendered tool outputs: no raw `Read` steering except the explicit staleness/unsupported-content path.

### BC-5: daemon/watcher runtime ↔ host OS/process model

- Level: runtime operations.
- Strength: intrusive/contract mix. The code depends on OS filesystem events, sockets/named pipes, process liveness, and lockfiles.
- Distance: high. Behavior spans macOS, Linux, Windows, MCP hosts, local FS, and release bundles.
- Volatility: medium-high. Several delta commits changed daemon and watcher behavior.
- Evidence: watcher platform strategy (`src/sync/watcher.ts:4-31`), daemon lifecycle (`src/mcp/daemon.ts:2-35`, `src/mcp/daemon.ts:62-71`), platform validation guidance in `CLAUDE.md`.
- Severity: medium. There is substantial test coverage, but quick sweep did not validate non-macOS behavior.
- Balancing move: keep code close, but enforce platform smoke tests or documented release gates for watcher/daemon changes.

## 8. Architect-skill blind spots vs archfit

- Architect quick sweep did not rerun SCIP, Lizard, jscpd, or full GitNexus history mining. Archfit did and produced quantitative metrics: 55 complex functions, 86 clone-duplicated cross-module pairs, 23 change-coupled pairs, propagation cost 0.106, and 120 risk hubs.
- Architect findings are narrative and relation-focused. Archfit is better at repeatable counts, trend/delta comparison, clone detection, and module-wide fan-in/fan-out summaries.
- Architect used `madge`/`depcruise` and named file-level cycles. Archfit compressed cycles into fewer structural components and can keep that number stable across runs.
- Architect did not assign final scores by design. Archfit produced scorecards: full overall 50/100, delta 42/100. Treat those as archfit outputs, not architect-skill scores.
- Architect's source reads are sampled. Archfit covered 404 SCIP files, 324 clone-scan files, and 131 LOC files per `scan.md`.

## 9. Archfit blind spots vs architect skill

- Archfit reported `verdict: pass` and no findings because gate rules are weak (`gate: warn`, one broad `core` layer). It did not turn structural risk into narrative findings.
- Archfit scored coupling balance 90/100 because no classified unbalanced edges were detected. It missed functional coupling where no import edge is enough: agent instructions ↔ tool output strings ↔ tests.
- Archfit did not surface intent drift: `server-instructions.ts` is intended as single source, while guidance also lives in README, CLAUDE, MCP descriptions, and rendered output.
- Archfit's LLM review failed (`llm-review*.md`: `unexpected end of JSON input`), so semantic/narrative review did not complete there.
- Archfit reports 2 cycles but not the high-signal cycle paths; `madge` showed 5 concrete file-level cycles.
- Archfit does not judge whether CI actually enforces architecture. It reports one arch-test signal, but the repo has no normal PR/push workflow running tests or cycle checks.
- Archfit's broad generated config makes every module `core`, so boundary integrity is mostly unmeasured rather than healthy.

## 10. Reliability/repeatability notes

- Source read-only was preserved. `git status --short` after commands matched the initial dirty state.
- `tsc --noEmit` passed; no build/test command that writes `dist/` was run.
- `.codegraph` read-only SQLite snapshot had zero mtime mismatches and no missing files, but its metadata was built by codegraph `1.0.1`, not this source package `0.9.9`. Treat it as a graph hint, not authoritative current-version evidence.
- GitNexus index is fresh to HEAD per `gitnexus_list_repos` and `.gitnexus/meta.json`.
- `depcruise` required `--no-config`; no project dependency-cruiser config exists.
- The first shell attempt with a here-doc was parsed by the harness shell and failed; commands were rerun under `/bin/bash -lc`.
- No web research was needed. The question was repo-specific and local evidence was enough.

## 11. Target architecture/design suggestions and executable fitness checks

### Design suggestions

1. **Core API / adapter split.** Break the root `index.ts` ↔ `mcp/*` cycle by splitting core API from MCP adapter exports. Keep package public exports compatible, but ensure MCP imports core, not root.
2. **Extraction core split without scattering language knowledge.** Keep language extraction cohesive under `src/extraction/`, but move reusable traversal/script-block logic out of `tree-sitter.ts` so Svelte/Vue/Razor wrappers do not import back into the god module.
3. **Dynamic-dispatch synthesizer registry.** Keep all synthesizers in `src/resolution/`, but split channel implementations into small files with one `EdgeSynthesizer` contract and a central registry.
4. **Single source for agent guidance.** Generate or constant-share the key "do not Read/Grep" guidance across server instructions, tool descriptions, and output snippets.
5. **Make `.archfit` boundaries real.** Replace one broad `core` layer with intended directions: CLI/MCP/installer may depend on core API; core should not depend on MCP/installer; extraction/resolution/db boundaries should have explicit allowed edges. Promote selected rules from warn to fail after baseline cleanup.

### Executable fitness checks

- Add CI workflow on PR/push:
  - `npm ci`
  - `npm exec -- tsc --noEmit --pretty false`
  - `npm test`
  - `madge --circular --extensions ts src` with zero cycles or explicit short allowlist.
- Add dependency-cruiser config and gate:
  - forbid `src/index.ts -> src/mcp/** -> src/index.ts` cycles.
  - forbid extraction wrapper cycles back into `tree-sitter.ts` after split.
  - enforce intended adapter directions.
- Add text invariant tests:
  - render static/dynamic tool descriptions and representative `codegraph_explore` / `codegraph_node` outputs.
  - fail on `Read` steering except staleness banners or unsupported file formats.
- Add size/complexity budget checks:
  - fail on new/modified functions with CCN > 15 unless explicitly justified.
  - track `src/extraction/tree-sitter.ts`, `src/mcp/tools.ts`, `src/resolution/callback-synthesizer.ts`; after splits, fail if they grow beyond the new baseline.
- Add dynamic-dispatch coverage gate:
  - require `scripts/agent-eval/probe-explore.mjs` / `probe-node.mjs` evidence for each new language/framework channel.
  - require the documented small/medium/large, 3-flow-prompt matrix before marking a channel complete.

## 12. Next checks

1. Run full local validation: `npm test` after confirming daemon-spawning tests are acceptable in this environment.
2. Run platform-sensitive checks for daemon/watcher changes: Linux Docker and Windows VM per `CLAUDE.md` guidance.
3. Convert `madge` cycle output into a dependency-cruiser config and decide whether current cycles are tolerated or fixed.
4. Focused review of `src/mcp/tools.ts` rendered outputs for Read/Grep steering drift, especially container-symbol `codegraph_node` paths.
5. Focused review of extraction split options: `tree-sitter.ts` ↔ `svelte/vue/razor` cycles first; no broad rewrite.
6. Rerun archfit after stabilizing `.archfit.full.yaml` as tracked or explicitly supplied config so warning/pass semantics are repeatable.
7. If a full architecture review is needed, validate working model with the maintainer before scoring.
