# archfit v0.10.0 — assessment from the omni monorepo exercise

Evidence base: ran archfit against omni (DoiT Cloud Intelligence) — a polyglot
monorepo: 184 `go.mod` under `go.work` (10,088 Go files), client TS app (6,826
TS/TSX), ~42 Python services, ~60 yarn workspaces. Findings come from (a) reading
archfit source, (b) probing runs, and (c) a 127-target batch (Go + TS + Python),
in progress at time of writing. Source refs are archfit `main` paths.

---

## TL;DR

archfit's **measurement core is strong** — deterministic, fail-loud, book-aligned
Balanced Coupling, honest `n/a`, good per-module performance. Its **repo model is
the weak point**: it assumes _one git repo = one project = one Go module_. That
assumption breaks on every monorepo dimension at once (multi-module `go.work`,
sub-project scoping, multi-language). The single highest-leverage fix is to
**decouple the scan root from `git rev-parse --show-toplevel`** and honor `--root`
as the real analysis boundary; everything else (workspace mode, multi-language
projects, cross-project edges) builds on that.

---

## 1. Capabilities — what worked well

- **Balanced Coupling scoring** is the standout. Book equation
  (`balance = max(|S−D|, 10−V)+1`), per-edge bands, `classified_edges` rollups,
  `cheapest_move` hints, `distance_basis`. On the monolith: 4,067 scored edges,
  53 genuine critical-band edges with file-level provenance. This is real,
  defensible architecture signal.
- **7-dimension scorecard** with per-dimension band + confidence + evidence refs,
  and an explicit `analysis_confidence` meta-dimension. The refusal to blend one
  number, and confidence caps on thin coverage, are the right calls.
- **Honest `n/a`** — when a tool is absent the dependent metric reports `n/a` with
  the enable step, not a fabricated pass. Rare and valuable.
- **Multi-language extractors exist** — Go (`go/packages`+scip-go), TS
  (dependency-cruiser+scip-typescript), Python (grimp+scip-python), Rust, plus
  lizard/jscpd/gitnexus enrichment. `doctor` reports presence cleanly.
- **Agent/CI ergonomics** — stable exit codes, `agent_tasks[]`, SARIF, JSON,
  scorecard, baseline/exception model.

## 2. Quality

- **Init scaffolding is too granular at scale.** `init` emits one module per Go
  package subtree: 354–380 modules for the monolith, flat `layers: [core, cmd]`,
  overlapping nested globs (`algolia/**` swallows `algolia/dal/**`). Usable as raw
  input but far from a meaningful boundary map; heavy manual curation needed.
- **Analyzers are opt-in and off by default.** `init` leaves `scip`, `complexity`,
  `clones`, `gitnexus` disabled, so the default `check` yields a thin, low-
  confidence scorecard even when `doctor` shows every tool installed. Easy to
  mistake for "archfit can't measure this repo."
- **`encapsulation` needs curated `public:`/`internal:` globs** to classify edge
  kinds; the auto globs don't, so `boundary_integrity` stays `n/a` until a human
  defines real API surfaces. Reasonable by design, but the default experience
  under-delivers this dimension.

## 3. Stability

- **No crashes, no hard failures.** Across 127 targets, 0 produced no scorecard;
  malformed/absent tools degrade to `n/a`, not exit-3. Deterministic config-hash
  reproducibility held.
- **LLM path is the unstable surface.** `autopilot`/`enrich` abort the _entire_
  draft when one sub-step returns non-JSON: observed
  `draft owners failed: enrich --owner: model response is not the required JSON
array: invalid character 'C'` on ~20% of targets (8/40 early in the batch).
  A single flaky `--owner` response discards the (successful) subdomain/volatility/
  layer/role work for that target → analyzer-only fallback. For batch/CI use this
  abort-all behavior is the main reliability gap.

## 4. Reliability

- **Deterministic gate is reliable**; LLM is correctly kept off-gate, so the
  flaky LLM never affects pass/fail — good separation.
- **Measurement honesty is high** but **two dimensions silently mislead in the
  isolated-checkout workaround** this repo forces:
  - `change_locality` reported `96/strong` on a 1-commit checkout (no history) —
    vacuous, yet not flagged as such (it is `n/a`-by-design in `--full`, but the
    scorecard still printed a number here).
  - `architecture_fitness=0` is partly real (no module-local arch checks) and
    partly an artifact (repo-root `.golangci.yaml` lives outside the scanned
    subtree). The tool can't tell these apart without root context.
- **`gitnexus` enrichment depends on a present index** and is silently thinned
  when history is absent. Fine, but compounds the isolated-checkout blind spots.

## 5. Scalability

- **Per-module performance is good.** Monolith (2,509 Go files, 354 modules, scip
  on) ≈ 65–70s; small modules ≈ 20–30s; autopilot scales with module count
  (small modules ≈ 30s, monolith ≈ 8–10 min). 0 import cycles computed fine on the
  largest module.
- **No native multi-target/monorepo orchestration.** Each invocation is one root.
  Analyzing 127 components required an external isolated-checkout runner + batch
  driver. archfit has no concept of "many projects in one repo."
- **Cannot scope into a subdirectory of a git repo** (see §6) — the binding
  scalability constraint for monorepos.
- **Cross-module/cross-service coupling is unmeasurable** under per-component
  isolation — exactly the distributed-monolith question a monorepo most needs.

## 6. The two architectural blockers (root cause)

**(a) Scan root = git toplevel.** `scope.Resolve` takes the root from
`Resolver.RepoRoot` = `git rev-parse --show-toplevel` (`internal/scope/scope.go:121`,
`internal/history/git/git.go:81`). `--root` only sets the git command's workdir
(`cmd/archfit/pipeline.go`), which still resolves _up_ to the toplevel. Every
extractor then uses `s.Root` (`pipeline.go`: `loc.Run(s.Root)`,
`fitness.Detect(s.Root)`, …). Net: for any subdir of a git repo, archfit analyzes
the **whole repo**, never the subdir. Confirmed: `--root <subdir>` still made
`loc` see 18,990 repo-wide files.

**(b) Go = one module per repo.** `internal/extract/golang/golang.go:48-90` runs
`packages.Load({Dir: s.Root}, "./...")` and derives a single `modPath` prefix from
the first package. At omni's root (`go.work`, no root `go.mod`) this returns a
synthetic error package → `go/packages: absent` → empty graph → all structural
dimensions unmeasurable. No `go.work`/workspace handling; no `GOFLAGS`/`GOWORK`
awareness.

These two together mean a multi-module monorepo is **unanalyzable as-is**; we had
to (1) extract the tracked tree to an isolated dir, (2) `git init` _at_ each module
so it became the toplevel, (3) set `GOWORK=off` so relative `replace => ../../`
directives resolved. Workable, but it loses git history and can't see cross-module
edges.

---

## 7. Improvement roadmap

### Tier 0 — the unlock (do first)

1. **Decouple scan root from git toplevel.** Make `--root` (or config `root:`) the
   authoritative analysis boundary: file walk, `packages.Load` Dir, history, and
   fitness detection all scoped to it; use git only for _history within_ that
   subtree (`git log -- <root>`). Add an explicit `--repo-root` for the VCS base.
   This single change makes in-place per-component analysis work with **no**
   isolated checkout, and restores `change_locality`/`gitnexus` for subtrees.

2. **Go `go.work` / multi-module support.** Detect `go.work`; when present, load
   member modules (or those under `--root`) and build a unified graph keyed by
   repo-relative paths, stripping each module's own prefix. Alternatively accept a
   `--go-module <dir>` / module glob. Today multi-module Go is a silent dead end.

<!-- markdownlint-disable MD029 -->

### Tier 1 — monorepo as a first-class concept

3. **Workspace/projects mode.** A config that declares N sub-projects (sub-roots),
   each with its language(s), module map, and rules; one invocation produces
   per-project scorecards **plus an aggregate**. Replaces the external batch
   runner this exercise needed.

4. **Cross-project edges.** With projects declared, classify edges _between_
   projects (the distributed-monolith-across-deploy-units risk) — currently the
   biggest blind spot for monorepos. `deploy_unit` distance already exists; extend
   it to a true inter-project graph.

5. **Multi-language single root.** Define how Go+TS+Python graphs compose under one
   root (per-language node-ID namespaces already differ: `file:` vs `module:`).
   Ensure the module map and rules can be language-scoped.

### Tier 2 — robustness & ergonomics

6. **LLM resilience (high value, low effort).** Isolate each `enrich`/`autopilot`
   sub-step so an `--owner` JSON failure doesn't discard subdomain/volatility/
   layer/role; enforce structured output (tool-use/JSON mode); retry-with-repair on
   malformed JSON; emit partial results. Targets the observed ~20% abort rate.

7. **Smarter `init` defaults for large modules.** Collapse package subtrees into
   domain modules by directory heuristics (`--group-depth`/`--max-modules`),
   instead of one-module-per-package (354 modules). Infer layers from common
   conventions (`*/dal`, `*/service`, `*/handlers`).

8. **Enable installed analyzers by default** (or `init --full-analyzers`): `doctor`
   already knows scip/lizard/jscpd are present; the default scorecard shouldn't be
   thin when they are.

9. **Flag vacuous dimensions.** When history is absent (1 commit / shallow), mark
   `change_locality` and `gitnexus`-fed metrics as `n/a (no history)` rather than
   printing a strong number.

### Tier 3 — scale

10. **Native multi-target parallelism + scip index caching/incremental**, so a
127-component repo is one `archfit` run, not an external driver.
<!-- markdownlint-enable MD029 -->

---

## 8. Quick wins vs larger efforts

- **Quick wins:** #6 (LLM sub-step isolation), #8 (default analyzers), #9 (vacuous-
  dimension flagging), #7 (init grouping knob).
- **Structural:** #1 (root decoupling) is the keystone and unblocks #2–#5.
- **Largest:** #3–#4 (workspace mode + cross-project graph) — but they convert
  archfit from "single-project tool run N times" into "monorepo architecture
  tool", which is what large orgs actually need.

---

## 9. Batch results (127 components — final)

- **Coverage:** 127 components (84 Go + 42 Python + 1 TS), mean overall **54.3/100**.
  Bands: 104 `mixed`, 20 `serviceable`, **3 rendered no verdict** (degenerate/
  errored: `server/cloudrun/api`, `services/data-exports/notifications`,
  `services/bigquery-lens/pricebooks`). No `strong`, no `poor`/`critical` overall —
  the rubric clusters everything in the middle, which limits its discriminating
  power for ranking — partly because the contaminated dimensions (§4: vacuous
  `change_locality` inflating, `architecture_fitness=0` deflating, `encapsulation`
  `n/a`) flatten the composite toward the mean. Rank by individual dimensions
  (coupling, dep-graph, duplication), not the overall.
- **Reliability:** **0 silent wrong results**; 1 hard hang (focus-analytics, §below)
  needed manual intervention. autopilot succeeded on **~105/127 (~83%)**; ~16–20%
  hit the `enrich --owner` JSON-abort and fell back to analyzer-only.
- **`scip` coverage:** 107/127 (Go ✓; Python 27/42; TS 0 — needs node_modules).

### Per-language behavior (important for the multi-language goal)

- **Go (84):** best-supported. Full `go/packages` graph, BC edge classification,
  scip-go symbols. **Every Go component is acyclic** (0 import cycles across all 84)
  — a real, positive structural result. coupling_balance and dep-graph dimensions
  are high-confidence here.
- **TypeScript (client, 1):** dependency-cruiser graph works well and is the most
  _actionable_ result of the run — **28 import cycles**, **904 cross-module
  duplication pairs**, dependency_graph_health 4/100 (critical). But **scip-
  typescript needs `node_modules`** (absent in the tracked-only iso extraction) so
  risk_hub/cohesion are `n/a`. TS type-only edges correctly score as contract
  strength (coupling_balance 68, high confidence).
- **Python (42):** grimp builds the module graph and dependency_graph_health is
  reliable (e.g. azure-etl 94/100, 0 cycles), but **BC coupling classification
  abstains** — "0 scored internal cross-boundary edges": the Python extractor does
  not tag edge kinds, so coupling_balance is `unconfirmed` for most Python
  services. Cohesion is mostly `n/a` (scip-python emits per-file modules). Net:
  Python gets graph-shape signal but little Balanced-Coupling signal.

### New stability finding (from this run)

- **No per-tool watchdog.** `services/cloudflow/focus-analytics` hung
  **>55 min at 0% CPU** in `check`; root cause is scip-go (and likely jscpd)
  choking on a **122,561-LOC generated `sql_parser.go`**. archfit has no timeout/
  progress guard around child analyzers, and config `exclusions` do **not**
  propagate to scip-go (it indexes the whole Go module regardless). Only disabling
  `scip`/`clones`/`complexity` for that target let it finish. **Recommendation
  (add to Tier 2):** per-analyzer timeout with `n/a (timed out)` degradation, and
  make `exclusions` skip files before scip/jscpd see them.

### Headline cross-component hotspots

- **Critical-band coupling (distributed-monolith risk):** scheduled-tasks (53),
  flexsave/aws/flexapi (33), customers (22), cloudflow/orchestrator-go (20).
- **Cross-module duplication:** client (904), scheduled-tasks (766),
  ps4compute/aws (186), flexsave/gcp (158).
- **Import cycles:** only the **client** (28) — all Go/Python components acyclic.

Full ranked table: `tools/archfit/INDEX.md`.
