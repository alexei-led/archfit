# archfit external-tooling analysis: capability, performance, overlap, reduction

Date: 2026-06-26. Scope: every external tool archfit shells out to, per supported
language (Go, TypeScript, Python, Rust) plus the cross-language tools. Goal: map
what each tool actually extracts vs. what it _can_ extract, measure operational
quality/performance, locate overlap, and recommend replacements / prerequisite
reductions.

**Operating principle (ordered).** This analysis optimizes for _evidence-based
metric coverage first_, tool count second:

1. **Maximize the metrics we can extract** from the tools we already run — and from
   tools we could add — as long as each metric is evidence-based and trustworthy.
2. **Before concluding a tool "can't" give a metric**, check that we are using it
   correctly: a tool that returns nothing or noise is often _misconfigured_, not
   incapable (see §9). Read the tool's current docs, fix the invocation, re-measure.
3. **Reduce tool count only when it is lossless** — drop or merge a tool only if the
   same metric, at the same quality, is recoverable from another tool or from code
   analysis. Fewer prerequisites is a real win (less install/complexity), but never
   at the cost of a metric. Every cut in §5 is gated on this.
4. **Abstain, don't fake.** A metric that reads `n/a` because the signal genuinely
   isn't there (no intrusive edges, no delta base) is _correct_ behavior, not a gap
   to paper over — distinguish it from a misconfiguration (§9.3).

Method: reproduced archfit's exact invocations (flags read from
`internal/extract/*`) in one uniform benchmark (`scratchpad/bench.sh`) — cold +
warm wall-clock, exit code, output size — across 7 local repos:
Go (archfit 287, pumba 159, spotinfo 22 files), TS (codegraph 196),
Python (ccgram 1835), Rust (herdr 203, yazi 1118). All tools installed and `ok`
per `archfit doctor`. Numbers are single-host (Apple Silicon, warm OS cache
unless noted); treat as relative, not absolute CI budgets.

> **Implementation status (2026-06-26) — executed via the plan
> `docs/plans/20260626-archfit-tooling-reduction.md`; validated lossless on all 7
> repos (0 metric regressions).**
>
> | Recommendation                                           | Status                         | Result                                                                                                                                                                              |
> | -------------------------------------------------------- | ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
> | §9.1 depcruise `--ts-config` auto-detect                 | ✅ done (`dd40bf7`)            | codegraph unresolved edges 1→0; alias edges recovered into coupling                                                                                                                 |
> | §5.1 drop gitnexus → SCIP-derived `symbol_dependants`    | ✅ done (`c631195`)            | Node tool + index build removed; risk_hub preserved, coverage **up** (archfit 53→98, ccgram 104→166, yazi 24→1208 files); schema v1→v2                                              |
> | §5.2 lizard → gocyclo (Go) + ast-grep proxy (TS/Py/Rust) | ✅ done (`60dcb42`)            | default Python pin removed; complexity preserved `info→info` on archfit (gocyclo) + herdr (proxy) **without lizard**; recall vs lizard Py 0.96 / Rust 0.94 / TS 0.88; lizard opt-in |
> | §5.3 cargo-modules vs scip                               | ⚠️ corrected — **not** demoted | code shows they are complementary (cargo-modules = structure, scip = strength); demoting would regress Rust                                                                         |
> | go/packages `NeedTypes` → Go edge strength               | ✅ done (`7c86d7f`)            | scip-off Go gains scored edges (pumba 10 abstained→0, coupling_balance 63/high); scip-on archfit unchanged (78/high)                                                                |
>
> Net dependency reduction (fully-instrumented Go repo): gitnexus's Node runtime +
> index build gone; lizard's Python pin gone by default. Validation method + per-repo
> before/after in §7.4.

---

## 1. The prerequisite unit is a _runtime_, not a binary

archfit's ~16 external binaries collapse into **four language runtimes** plus two
standalone native tools. Dropping a binary that shares a runtime with a required
tool saves nothing; the only real reductions remove a whole runtime from a
project's requirement set.

| Runtime           | Occupants                                                         | Pinned by (irreducible)                                          |
| ----------------- | ----------------------------------------------------------------- | ---------------------------------------------------------------- |
| **Go toolchain**  | go/packages (in-proc), scip-go, deployunit (`go list`)            | the Go language itself                                           |
| **Node**          | dependency-cruiser, jscpd, gitnexus, scip-typescript, scip-python | only dependency-cruiser (TS); the rest are _secondary occupants_ |
| **Python / uv**   | grimp, lizard (`uvx` path), SCIP proto-reader (`uv run`)          | only grimp (Python projects)                                     |
| **Rust / cargo**  | cargo metadata, cargo-modules, rust-analyzer scip                 | the Rust language itself                                         |
| Standalone native | **sg** (ast-grep — single static Rust binary, no runtime), git    | —                                                                |

**Everything except the primary graph tool is opt-in.** There is no default-on
injection: `syntax` (ast-grep), `scip`, `complexity` (lizard), and `clones`
(jscpd) all run only on explicit `tools.<x>.enabled: on`; gitnexus auto-runs
_only_ when a `.gitnexus`/`.codegraph` index is already present
(`internal/config/tools.go`). A minimal config that just declares its language
runs `check` with **only** that language's primary graph tool (in-process for Go)
plus the in-process signals (loc, dynimports, manifest, git history, ownership,
deployunit) — no Node, no Python, no `sg`. Prerequisites pile up only as a user
opts into richer signals. archfit's **own** `.archfit.yaml` opts into nearly
everything (syntax + scip + complexity + gitnexus + clones, Go only) because it is
a dogfooding showcase — it is the _maximal_, not the default, footprint.

Consequence used throughout this report: the addressable targets are the
**secondary occupants** that a fully-instrumented config drags in — for a
**non-TS** project, Node is pinned only by gitnexus + jscpd (+ opt-in
scip-python); for a **non-Python** project, Python/uv is pinned only by lizard
(+ the opt-in SCIP reader).

---

## 2. Tool → signal → metric map (what each tool's output is for)

Every gate and metric decision flows from one of these signals. "Gate" = can fail
`check`; "advisory" = warn-only / report; "opt-in" = off unless enabled.

| Tool                                        | Runtime              | archfit invocation                                        | Signal produced                    | Consuming metrics                                                                                         | Role            |
| ------------------------------------------- | -------------------- | --------------------------------------------------------- | ---------------------------------- | --------------------------------------------------------------------------------------------------------- | --------------- |
| **go/packages**                             | Go (in-proc)         | `packages.Load(NeedImports\|NeedDeps\|NeedFiles)`         | import graph                       | ~13 graph metrics¹                                                                                        | gate            |
| **dependency-cruiser**                      | Node                 | `bunx/npx depcruise <src> --output-type json --no-config` | import graph + StrengthHint        | ~13 graph metrics¹                                                                                        | gate            |
| **grimp**                                   | Py/uv                | `uv run --with grimp … --package <pkg>`                   | import graph + StrengthHint        | ~13 graph metrics¹                                                                                        | gate            |
| **cargo metadata**                          | Rust                 | `cargo metadata --format-version 1 --no-deps`             | crate graph + CrateRoot            | ~13 graph metrics¹                                                                                        | gate            |
| **sg (ast-grep) — syntax**                  | native               | `sg scan -r <rules> --json=compact .`                     | SyntaxFacts: roles + density nodes | unsafe/panic/test/struct_field/global_state density (5), `forbidden_role_dependency`, encapsulation roles | gate            |
| **sg (ast-grep) — patterns**                | native               | `sg run --lang <l> --pattern <p> .`                       | pattern hits                       | `public_api_type_leak`, forbidden-pattern rules                                                           | gate            |
| **lizard**                                  | Py                   | `lizard . --csv -l …`                                     | CCN + NLOC per function            | `complexity`, intramodule complexity                                                                      | gate            |
| **jscpd**                                   | Node                 | `jscpd --reporters json --output <t> .`                   | cross-module clone pairs           | `functional_candidates` + symmetric-strength upgrade (classify)                                           | advisory        |
| **git history**                             | git (in-proc)        | `git log -n<N> --name-only`                               | FileChurn, CoChange                | `change_coupling`, `change_amplification`, `hidden_coupling`, `functional_candidates`                     | gate            |
| **gitnexus**                                | Node + index         | `gitnexus cypher -r . <CodeRelation query>`               | per-file dependants count          | **`risk_hub`** + `gitnexus_impact` facts/JSON field (`facts.Build`) — both SCIP-gated                     | advisory/opt-in |
| **scip-{go,ts,python}, rust-analyzer scip** | per-lang + uv reader | `<indexer> … ; uv run scip_reader.py …`                   | StrengthHint upgrade (symbol refs) | martin / coupling classification (precision)                                                              | opt-in          |
| **cargo-modules**                           | Rust                 | `cargo modules dependencies --no-externs --package <p>`   | intra-crate module graph           | graph metrics at `crate::mod` granularity                                                                 | opt-in          |
| **loc**                                     | Go (in-proc)         | `filepath.WalkDir` + line count                           | LOC/file                           | `structural_weight`, `file_structural_weight`                                                             | gate            |
| **dynimports**                              | Go (in-proc)         | 3 regexps (`require(`, `import(`, `importlib`)            | dynamic-import sites               | report (hidden-coupling context)                                                                          | report          |
| **manifest**                                | Go (in-proc)         | `os.ReadFile` go.mod/package.json                         | deprecated deps                    | `deprecated_dep_count`                                                                                    | gate            |
| **ownership**                               | git (in-proc)        | `git log --format=%ae --name-only`                        | author/file                        | change_locality / ownership                                                                               | gate            |
| **deployunit**                              | Go subprocess        | `go list -f '{{if eq .Name "main"}}…'`                    | deploy units                       | distance (deploy boundary)                                                                                | gate            |
| **runtime**                                 | Go (in-proc)         | ast/regexp async-bridge scan                              | RuntimeAsync sites                 | **none — report-only**                                                                                    | report          |

¹ The import/crate graph is the backbone: `cycle`, `encapsulation`,
`unbalanced_edge`, `change_locality`, `martin_distance`/`abstractness`/`instability`,
`file_mutual_import`, `propagation_cost`, `structural_weight`, `blast_radius`,
`change_amplification`, `change_coupling`, `hidden_coupling`, `functional_candidates`.

Two facts that correct common assumptions:

- **Co-change is in-process, not gitnexus.** `internal/history/git` runs `git log
--name-only` by default and produces FileChurn + CoChange. gitnexus is _not_ the
  change-coupling source — its cypher query is a **static CodeRelation graph**
  (DEFINES/CALLS/ACCESSES/IMPORTS/EXTENDS) yielding a per-file _dependants count_.
- **blast_radius** is computed from the in-process dependency graph
  (`modgraph.BlastRadius`), not from any history or symbol tool.

### 2.1 Capability headroom — what each tool _can_ extract that archfit doesn't

archfit taps a deliberately small slice of each tool. The untapped surface (per the
sub-agents' `--help`/API probes) shows where future signals could come from without
adding a prerequisite:

| Tool                   | archfit uses                          | Capability headroom (unused)                                                                                                                                                                                                       |
| ---------------------- | ------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **go/packages**        | import graph (NeedImports/Deps/Files) | full type info (NeedTypes/TypesInfo) → type-checked call graph, exact symbol refs, SSA                                                                                                                                             |
| **dependency-cruiser** | import edges                          | `--metrics` (Martin instability/abstractness — archfit recomputes these itself), `--affected <rev>` change-impact, `--reaches` reachability, orphan detection, cycle rules, native rule-based fitness gates, dot/mermaid/d2 graphs |
| **grimp**              | direct import edges                   | graph-query API: `find_shortest_chain(s)`, up/downstream closure, `chain_exists`, **`find_illegal_dependencies_for_layers`** (ready-made layer-violation checks), `nominate_cycle_breakers`, `get_import_details`                  |
| **cargo metadata**     | crate graph + roots                   | feature graph, build/dev/target dep kinds, full resolved dep tree (without `--no-deps`), registry/source provenance                                                                                                                |
| **ast-grep**           | roles + density + type-leak           | unbounded structural rules with autofix, relational rules (`inside`/`has`/`follows`), metavar constraints — can absorb dynimports + a CCN proxy (§5.2)                                                                             |
| **lizard**             | CCN + NLOC/function                   | token count, param count, function length, built-in `--duplicate` clone detector (overlaps jscpd at function granularity)                                                                                                          |
| **jscpd**              | cross-module clone pairs              | clone-% per file, intra-file clones, configurable min-tokens, threshold gating, HTML/badge reports                                                                                                                                 |
| **SCIP indexers**      | edge StrengthHint                     | full symbol definitions/references/implementations, hover docs, precise find-all-refs, a complete call/reference graph (a sliver is consumed)                                                                                      |
| **gitnexus**           | one dependants-count query            | a full Cypher-queryable code-knowledge graph — arbitrary CodeRelation traversals (call chains, blast radius) far beyond the single query                                                                                           |
| **git history**        | churn + co-change pairs               | commit-message mining, temporal-coupling windows, bug-fix density (ownership is already used separately)                                                                                                                           |

Two recurring patterns: (1) the per-language graph tools (depcruise, grimp) ship
**layering/fitness primitives archfit reimplements internally** — an integration
opportunity, not a prerequisite cut; (2) the SCIP indexers and gitnexus expose
**orders of magnitude more** than the single fact archfit reads, which is exactly
why their cost/benefit (§3, §5.1) looks poor for what's currently consumed.

---

## 3. Performance & operational quality (measured)

### Cross-language tier — fast, no network, sub-second to ~2s

| tool          | repo (files)       | cold s    | warm s | output               |
| ------------- | ------------------ | --------- | ------ | -------------------- |
| ast-grep scan | spotinfo go (22)   | 0.10      | 0.08   | 1.4 MB               |
| ast-grep scan | pumba go (159)     | 0.19      | 0.18   | 8.4 MB               |
| ast-grep scan | archfit go (287)   | 0.22      | 0.19   | **11.9 MB**          |
| ast-grep scan | codegraph ts (196) | 0.16      | 0.17   | 4.9 MB               |
| ast-grep scan | ccgram py (1835)   | 0.41      | 0.45   | **38 MB**            |
| ast-grep scan | yazi rs (1118)     | 0.32      | 0.31   | 26 MB                |
| ast-grep scan | herdr rs (203)     | 0.99      | 1.02   | **104 MB**           |
| lizard        | range across repos | 0.2–2.1   | ~same  | ≤1.5 MB              |
| jscpd         | range across repos | 0.15–0.88 | ~same  | tiny (report → file) |

ast-grep is the cheapest analyzer by wall-clock (compiled Rust, 0.1–1s) but emits
**huge JSON** (104 MB on herdr, 38 MB on ccgram). The syntax-scan path
(`syntax.go`) now **streams** this output element-by-element through a
`json.Decoder` into a lean struct (commit `a7dc937`), so peak memory no longer
tracks output size — see §5.4. Output size still tracks rule-match count, not file
count (herdr's 203 files out-emit yazi's 1118 because `rust.yml` over-matches
pub/trait/impl nodes). The one residual buffer is the **pattern** path
(`astgrep.go:86`, `sg run --pattern`), which still `json.Unmarshal`s its whole
output — far smaller than scan output (single pattern, not a rule pack), so it is
the last, low-priority `Unmarshal`-the-world site in the fast tier.

### Primary dependency-graph tier — all fast

| tool                        | repo      | cold s   | warm s |
| --------------------------- | --------- | -------- | ------ |
| cargo metadata              | herdr     | **0.07** | 0.07   |
| cargo metadata              | yazi      | 0.15     | 0.08   |
| go list (go/packages proxy) | archfit   | 0.39     | 0.37   |
| grimp (uv run --with)       | ccgram    | 0.47     | 0.14   |
| dependency-cruiser (bunx)   | codegraph | 1.75     | 1.83   |

cargo metadata is the fastest primary tool of any language (resolves from
`Cargo.lock`, no compile). depcruise is the slowest (~1.8s) but needed **no
network** and tolerated `--no-config` with no tsconfig/path-alias errors on
codegraph. grimp's `uv run --with` hit **no network** (grimp already in uv's
cache) — true cold (first-ever) would add a one-time download; thereafter ~0.14s.

### SCIP / opt-in heavy tier — seconds to tens of seconds

| tool                  | repo (files)            | cold s   | note                                                                                |
| --------------------- | ----------------------- | -------- | ----------------------------------------------------------------------------------- |
| scip-go               | archfit (287)           | 0.67     | warm Go build cache                                                                 |
| scip-go               | pumba (159)             | 1.49     | more transitive deps to resolve                                                     |
| scip-go               | spotinfo (22)           | **11.6** | **cold Go build cache** (first run of session)                                      |
| scip-typescript       | codegraph (196)         | 2.06     | no `npm install`/build needed                                                       |
| scip-python           | ccgram (407 indexed)    | 12.68    | 16 MB index                                                                         |
| rust-analyzer scip    | herdr (203)             | 16.74    | 29 MB index; no pre-`cargo build` needed                                            |
| rust-analyzer scip    | yazi (1118)             | 14.82    | whole 31-crate workspace, one pass                                                  |
| cargo-modules         | herdr (1 crate)         | 5.27     | compiles; 2.3 MB DOT                                                                |
| cargo-modules         | yazi (1 of 31 crates)   | 7.39     | **per-crate** — workspace = ×31                                                     |
| gitnexus cypher query | archfit (index present) | 1.20     | + a separate `gitnexus analyze` index build (≈75–100 MB; size is snapshot-specific) |

Operational notes that matter for reliability:

- **scip-go's cost is dominated by `go build` of dependencies**, globally cached.
  First run ≈12s, every subsequent run <1.5s — the variance is _primarily_ cold
  build cache (spotinfo's outlier is partly its own heavy transitive tree), not
  source-file count.
- **rust-analyzer scip did _not_ time out** (≤17s even on yazi) and needs no prior
  `cargo build` — it does one analysis pass for the **whole workspace**.
- **cargo-modules runs per crate** (5–7s each). On yazi (31 members) a full
  workspace pass is ~31 invocations → minutes. It also misses `#[path]`, inline
  `mod {}`, and re-exports that rust-analyzer resolves correctly.
- **gitnexus** query is cheap (1.2s) but presupposes an index from a separate,
  heavy `gitnexus analyze` build — its size is snapshot-specific (≈75–100 MB here,
  and the current branch was unindexed at writing), and that build cost is hidden
  from `check`.
- No tool failed (all exit 0). No network was required by any tool on warm caches.

---

## 4. Overlap analysis

Three overlap zones, ranked by reduction value.

**Zone A — symbol-reference graph computed three ways (highest overlap).**
The same underlying fact — "who references whom at symbol granularity" — is
produced independently by (a) the SCIP indexers (→ StrengthHint), (b) gitnexus's
CodeRelation query (→ dependants count for `risk_hub`), and (c) the user's own
`codegraph` tool. gitnexus's dependants count is a strict _subset_ of what SCIP
references already encode (SCIP is a true superset); the dependency graph's reverse
fan-in is only a coarse echo of it (ρ=0.43, §7.2), not a substitute. gitnexus is
the redundant occupant — a whole Node runtime + an index build feeding **one
advisory metric** — but two facts (both verified in code) qualify "redundant":

- It is a **live, conditional consumer**, not dead code: `risk_hub` multiplies its
  surface-breadth ranking by a bounded `[1.0, 2.0]` gitnexus factor
  (`risk_hub.go:146,221`). Dropping it removes that refinement (§5.1), it does not
  silently no-op.
- gitnexus is **only ever consumed when SCIP is also on.** `risk_hub`'s breadth
  comes from the SCIP symbol graph (`in.Symbol.Graph`); with SCIP off that graph is
  empty and `risk_hub` returns n/a _before_ the gitnexus factor is read
  (`risk_hub.go:198`). So gitnexus's index is wasted work unless SCIP is enabled —
  exactly the case where SCIP references already encode the same fact. The code
  labels the factor "historical impact," but its _derivation_ is static
  (`CALLS/ACCESSES/IMPORTS/EXTENDS`); "historical" names its intended _role_ as a
  change-impact proxy, not how it is computed — and static derivation is precisely
  why SCIP can reproduce the data (with the wiring caveat in §5.1).

**Zone B — import graph vs SCIP, per language (intentional, keep).**
Every language ships _both_ a fast import-graph tool and a slow SCIP indexer. They
overlap (SCIP references ⊃ import edges) but are not interchangeable: the import
tools are the <2s gate-path graph; SCIP (12–17s, large indexes, needs a
toolchain/compile) is a strength-_precision_ upgrade. SCIP cannot be the primary
graph — too slow for `check`. Current opt-in design is correct.

**Zone C — complexity/size (minor).**
lizard's NLOC overlaps the pure-Go `loc` line count, and lizard's CCN is the only
non-derivable signal there. `loc` is free (zero prereq, always runs); lizard pins
the **Python runtime even on non-Python projects** purely for CCN. ast-grep
(already present, native) can structurally count decision points (`if/for/match/&&/||`)
— a CCN proxy — which makes lizard a candidate for removal on the Python-pin
grounds (see §5.2).

**Non-overlaps that look like overlaps (leave alone):**

- `dynimports` (3 regexps) vs ast-grep: ast-grep _could_ match these structurally,
  but `dynimports` is **pure-Go with zero prerequisites** and runs even when `sg`
  is absent. Folding it into ast-grep _adds_ a prereq to a signal that has none —
  it fails the "fewer prereqs" test. Keep as-is.
- jscpd (textual near-clone similarity) — ast-grep and SCIP do exact structural /
  symbol matching, **not** similarity. jscpd's signal is unique; dropping it loses
  the symmetric-strength-from-clones upgrade. Tradeoff, not redundancy.
- ast-grep cannot replace grimp / dependency-cruiser / go-packages / cargo: those
  do real cross-file **resolution** (tsconfig path aliases, package resolution,
  build constraints, Cargo.lock) that single-file pattern matching cannot. This is
  a hard ceiling, not a gap.

---

## 5. Recommendations (prioritized)

### 5.1 Drop gitnexus — HIGH value; the refinement it feeds is recoverable from SCIP with new wiring (corrected, §7.2)

gitnexus is already the most conditional tool — it runs only when a
`.gitnexus`/`.codegraph` index exists — so dropping it is the lowest-risk cut.
Doing so removes a **Node** occupant and a separate index-build step. It has **two**
advisory consumers, both SCIP-gated: `risk_hub` (the `[1.0, 2.0]` factor) and the
per-module `gitnexus_impact` facts/JSON field (`facts.Build`, which returns empty
when the SCIP symbol graph is empty). gitnexus's per-file dependants count
(symbol-level `CALLS/ACCESSES/IMPORTS/EXTENDS` references) is a strict **subset of
what the SCIP indexers already compute** — SCIP references are a true superset of
the same static fact.

Two corrections to the earlier framing, both grounded in the code:

- **There is no SyntaxFacts fallback.** `risk_hub`'s surface breadth is computed
  from the **SCIP symbol graph** (`in.Symbol.Graph`, populated by `ex.scipSymbols`),
  and the metric returns **n/a** when that graph is empty (`risk_hub.go:198`). So
  with both SCIP and gitnexus off, `risk_hub` is n/a — it does **not** fall back to
  a SyntaxFacts computation (the prior draft's claim at "risk_hub.go:190" was wrong;
  line 190 is just the `hasGitnexus` check). gitnexus therefore only ever does
  anything when SCIP is already enabled.
- **SCIP does not yet auto-subsume the factor.** `risk_hub` reads the gitnexus
  contribution from exactly one field — `in.Symbol.GitnexusImpact` (file → distinct
  dependant count) — and that field is populated **only** by `gitnexus.Run`
  (`pipeline.go:228`). SCIP feeds _different_ fields (`ex.scipSymbols` + edge
  `StrengthHint`). So dropping gitnexus loses the `[1.0, 2.0]` factor **even when
  SCIP is on**: `risk_hub` keeps working (SCIP breadth is intact) but its ranking
  degrades from `breadth × config-volatility × dependant-impact` to `breadth ×
config-volatility`. The factor is _recoverable_ — the dependant count is a strict
  subset of SCIP's reverse-reference graph — but only with **new wiring** that
  aggregates SCIP reverse-refs per file into the `GitnexusImpact` map. That wiring
  does not exist today and is the real cost of the cut.

§7.2 separately refuted the **in-process graph reverse fan-in** as a drop-in
fallback: package-level import fan-in correlates with gitnexus dependants at only
**Spearman ρ=0.43** (7/10 top hubs agree, mid-tier ranking diverges), because
import-count ≠ symbol-call breadth. So do not substitute raw fan-in.

Net recommendation: **drop gitnexus** (one fewer Node tool + no index build).
Because it is only ever live alongside SCIP, and the data it adds is a static
subset of SCIP references, the clean path is to wire SCIP reverse-refs into the
dependant-count factor and retire gitnexus entirely. Until that wiring lands, the
cut's cost is twofold: `risk_hub` loses its dependant-impact refinement (it stays
informative on `breadth × config-volatility` whenever SCIP is on — not n/a), and the
per-module `gitnexus_impact` facts/JSON field disappears. Both are recoverable from
SCIP reverse-refs with the same wiring — no metric is lost permanently.

### 5.2 Replace lizard's CCN with ast-grep-derived complexity — validated (§7.1)

Enabling `complexity` today drags in the **Python runtime** (lizard) on top of
whatever else is on — the very cost that keeps it opt-in. ast-grep (native, and
already opted into for `syntax`) can count decision points per function
structurally; combined with the pure-Go `loc` for NLOC, this reproduces both
complexity inputs **without Python**. The payoff is not just one fewer tool — it
lets `complexity` be turned on for the **price of a prereq you already paid**
(`sg`), instead of a new runtime. Cost: CCN becomes approximate (lizard's is
exact, language-tuned). **Validated on Go** (§7.1): a 7-pattern ast-grep
decision-point rule, assigned per enclosing function, catches **34/34 of lizard's
CCN>15 hotspots (recall 1.00, zero misses)** at precision 0.69 (~44% over-flag),
ρ=0.838, 88% exact-CCN. Good enough for the report-only hotspot metric as-is.
Remaining action: tighten the rule to lift precision, port it to TS/Py/Rust, and
keep lizard as the option for exact per-function CCN if false positives matter.

**Go-specific exact alternative — gocyclo.** For Go targets specifically, `gocyclo`
gives _exact_ per-function CCN as a **single static Go binary** — no Python pin,
no ast-grep approximation. Measured on archfit: **0.08s warm**, 49 functions over
CCN 15 (test files included). It shares the Go toolchain archfit already requires,
so for Go it dominates both lizard (exact but +Python) and the ast-grep proxy
(native but approximate). It does not generalize — Go-only, so lizard or the
ast-grep proxy still covers TS/Py/Rust — but it is the obvious complexity source
when exactness matters and only Go is in scope (see §8).

### 5.3 cargo-modules and scip are COMPLEMENTARY, not substitutes — corrected (do NOT demote cargo-modules)

The earlier framing here ("prefer scip over cargo-modules for the module graph")
was **wrong**, corrected against the code. In archfit's Rust pipeline the two tools
do different jobs:

- **cargo-modules** (`internal/extract/rust/rust.go` `runModuleGraph`) is the **sole
  source of intra-crate `<crate>::<mod>` graph _structure_** — it appends the module
  nodes + `uses` edges to `facts.Nodes/Edges`. `classify.AugmentModulesFromGraph`
  (`classify.go:139`) only promotes graph nodes that already contain `::`; those
  nodes exist only because cargo-modules created them.
- **rust-analyzer scip** (`internal/extract/scip`) produces a symbol graph + a
  `<crate>::<mod>` **strength map**; the engine uses it to _enrich existing edges_
  with `StrengthHint` (`engine.go:342`) and to feed risk_hub / `symbol_dependants`.
  It does **not** create module nodes/edges.

So scip cannot replace cargo-modules: with scip on but cargo-modules off, a
single-crate Rust project has no `<crate>::<mod>` nodes → the graph stays degenerate
and `cycle`/`blast_radius`/`cohesion_lcom` go n/a, and scip's strength map has
nothing to attach to. Demoting cargo-modules would **regress** Rust, and removes no
prerequisite (cargo-modules is already opt-in). Keep both. cargo-modules' real
costs remain real (per-crate ×N, misses `#[path]`/macros), but the lossless fix is
not "prefer scip" — it is a future feature: **derive `<crate>::<mod>` module edges
from the scip symbol graph's `Refs`** so scip can supply structure too, making
cargo-modules genuinely optional. That is additive work, not a swap.

### 5.4 Stream ast-grep output instead of buffering — DONE for the scan path (commit `a7dc937`); residual = pattern path

**Status: implemented for the syntax-scan path.** `syntax.go` now streams `sg scan`
output element-by-element through a `json.Decoder` (`decodeSyntaxStream`) into a
lean `sgSyntaxMatch` struct, off the subprocess pipe via the new `Runner.Stream`
hook — no full-output buffer, no reflective full-struct `Unmarshal`. (It still
retains the decoded `[]sgSyntaxMatch` slice because the two-pass import-gating logic
needs the whole set, so the win is "no raw buffer + lean-vs-full struct," not O(1)
memory.)

**Benchmarked** (100 MB herdr output, Go 1.26, `CGO_ENABLED=0`): the streaming +
lean-struct path drops peak RSS from **221 MB → 13 MB at identical wall time** — no
new dependency, no lost matches. NDJSON (`--json=stream`) + `bufio.Reader.ReadBytes`
is an equivalent 16 MB path — but **never `bufio.Scanner`** (validated: silently
truncates at 64 KB, dropping 78% of herdr's matches). A faster JSON lib is the wrong
fix — go-json/gjson cut CPU but _raise_ peak memory; the parse isn't the bottleneck.
Full method, table, and the `encoding/json/v2` trajectory:
**`docs/research/big-json-in-go-2026-06.md`**.

**Residual:** the **pattern** path (`astgrep.go:86`, `sg run --pattern`) still
`json.Unmarshal`s its whole output. That output is far smaller than `sg scan`'s (a
single pattern, not a rule pack), so it is lower-priority — but it is the last
buffer-the-world site in the fast tier and should get the same `Stream` treatment
for consistency.

### 5.5 Keep dynimports, jscpd, SCIP-as-opt-in as they are — defensible non-changes

- dynimports: zero-prereq pure-Go; moving it to ast-grep would _add_ a dependency.
- jscpd: unique near-clone signal. It becomes the _sole_ Node pin on a non-TS repo
  once gitnexus is gone (§5.1). Eliminate Node entirely for non-TS projects only if
  you accept losing clone-similarity, or replace it with a ~100-line pure-Go
  token-shingle duplicate detector (build-vs-prereq call — only if Node-elimination
  is a hard goal).
- SCIP indexers stay opt-in and off the gate path — the measured 12–17s cost
  confirms they cannot be primary.

---

## 6. Net prerequisite reduction

This table is for a **fully-instrumented config** (syntax + complexity + clones +
gitnexus all enabled, like archfit's own `.archfit.yaml`) — the only case where
prerequisites pile up. A **default/minimal config needs none of this**: just the
language's primary graph tool + in-process signals (see §1). The reduction below
is the payoff for users who want the rich signals without the install burden.

| Project type | Fully instrumented today                                                | After §5.1 (drop gitnexus)                 | After §5.1 + §5.2 + §5.5-jscpd                 |
| ------------ | ----------------------------------------------------------------------- | ------------------------------------------ | ---------------------------------------------- |
| **Go-only**  | go, git, sg, lizard(+Python), jscpd(+Node), gitnexus(+Node+index build) | go, git, sg, lizard(+Python), jscpd(+Node) | **go, git, sg**                                |
| TS           | + Node (depcruise)                                                      | + Node                                     | + Node (depcruise; irreducible)                |
| Python       | + uv/python (grimp)                                                     | + uv/python                                | + uv/python (grimp; irreducible)               |
| Rust         | + cargo                                                                 | + cargo                                    | + cargo; prefer scip over cargo-modules (§5.3) |

The headline (Go-only, fully instrumented): from **6 tools across 3 runtimes**
down to **3 tools / 1 runtime + 2 standalone binaries** (go, git, sg) — but only
with **all three** of §5.1 (drop gitnexus), §5.2 (lizard→ast-grep CCN), and the
§5.5 pure-Go jscpd replacement applied. §5.1 alone removes the Node+index occupant;
jscpd's Node pin survives until §5.5. The four language primary resolvers
(go/packages, dependency-cruiser, grimp, cargo) are irreducible — each is pinned by
its language and does resolution ast-grep cannot — and that "cannot be replaced" is
itself a firm result, not an open gap.

---

## 7. Validation (empirical, run on archfit's own repo)

The two unmeasured proposals were tested against ground truth, not asserted.

### 7.1 §5.2 — ast-grep CCN proxy vs lizard — HOLDS (per-function)

The complexity metric (`intramodule/complexity.go`) is **per-function** — it flags
each function with `CCN > 15` by `file:line`, ranked — so the swap must be tested
per function, not per file. (A first per-file run gave ρ=0.987, but that is partly
tautological: lizard's per-file CCN sum ≈ Σ the same decision nodes the proxy
counts, so it mostly confirms the rule counts the right node kinds.) The honest
test assigns each ast-grep decision-point match to its enclosing function (by line
range) and compares per-function CCN across **1,985 matched functions** on archfit:

- **Per-function Spearman ρ = 0.838**; **exact CCN match 88%**, within ±2 = 94%.
- At archfit's real hotspot threshold (CCN > 15): lizard flags 34 functions, the
  proxy flags 49 → **recall = 1.00 (34/34, zero false negatives), precision =
  0.69 (15 over-flags)**.
- Verdict: the proxy reproduces archfit's per-function hotspot detection with **no
  misses** but ~44% more candidates. Over-flagging is the safe direction for a
  report-only metric (worst case `enrich_values.go`: lizard 3 vs proxy 47, from
  `||`/`&&` inside value tables). Tightening the rule (or matching lizard's
  threshold) closes the precision gap; lizard stays the option for exact
  per-function CCN if the false-positive rate matters.

### 7.2 §5.1 — graph reverse fan-in vs gitnexus dependants — PARTIAL

Parsed archfit's real 75 MB gitnexus index (the actual `parseDependants` markdown
format) and compared per-package dependants against `go list` import reverse fan-in
across 58 packages:

- **Spearman ρ = 0.43** — moderate, _not_ equivalent.
- **Top-10 hub overlap: 7/10** — the biggest hubs mostly agree; the mid-tier ranking
  diverges (e.g. `cmd/archfit`, `internal/initcfg`, `metrics/modularity` are
  symbol-impact hubs but not import-fan-in hubs, and vice-versa for
  `model/symbol`, `scope`).
- Conclusion: import-count ≠ symbol-call breadth. This **refuted** the "graph
  fan-in is an equivalent replacement" framing and is why §5.1 routes the precision
  path through SCIP references (a true superset of gitnexus dependants). The
  _decision to drop gitnexus still holds_ (cost + the data being an SCIP subset); the
  _replacement mechanism_ was corrected by the data — and on re-reading the code, two
  further claims in the earlier draft were wrong and are fixed in §5.1: there is **no
  SyntaxFacts fallback** (`risk_hub` is n/a when the SCIP graph is empty,
  `risk_hub.go:198`), and SCIP does **not yet** auto-supply the dependant-count
  factor (only `gitnexus.Run` populates `GitnexusImpact`; SCIP→factor needs new
  wiring).
- Granularity caveat: this proxy used package-level dependants with `sum`
  aggregation, whereas `risk_hub` aggregates to **module** level with `max`
  (`moduleImpactFromFiles`). ρ=0.43 is therefore directional, not the exact signal
  risk_hub consumes — but the conclusion (route precision through SCIP; gitnexus is
  only ever active alongside SCIP) is unaffected by the aggregation choice.

### 7.3 Already measured (not re-validated)

§5.3 (scip one-pass 14.8s vs cargo-modules 5–7s × 31 crates) is a direct benchmark
observation from §3. §5.4 was both benchmarked (221 MB → 13 MB) and **shipped** for
the scan path (commit `a7dc937`); the benchmark detail lives in
`big-json-in-go-2026-06.md`. §8's adjacent-tool numbers are fresh single-host runs.

### 7.4 Cross-repo before/after validation of the executed changes — 0 regressions

Captured a full `check --full --format json` baseline on all 7 repos **before** any
change, then re-ran after gitnexus-drop + complexity-backends + Go-strength and
diffed every metric's band/confidence (`scratchpad/baseline/` vs `scratchpad/after/`).

- **No metric regressed on any repo** (0 of 7) — no dimension that measured in the
  baseline went `n/a`, and `risk_hub`/`coupling_balance` held their band+confidence.
- **gitnexus removed from every run** (`tool_coverage` no longer lists it); a stale
  `tools.gitnexus` stanza in an existing config (herdr) is harmlessly ignored, not a
  decode error — no config-compat break.
- **risk_hub preserved + coverage up** (now SCIP-derived `symbol_dependants`):
  identical hub counts and top hubs on archfit (98), ccgram (166), yazi (1180), with
  files covered rising 53→98 / 104→166 / 24→1208 (SCIP graph ⊃ the old gitnexus index).
- **complexity lossless without lizard**: the two repos that enable it kept `info` —
  archfit via gocyclo, herdr via the ast-grep proxy — with lizard absent from coverage.
- **Go strength gain**: on a scip-off Go repo (pumba) 10 previously-abstained edges
  became scored (`coupling_balance` 63/high); scip-on archfit unchanged (78/high,
  SCIP strength still wins).

Acceptance gates on the final tree: `make test` (53 pkgs ok), `make lint` (0 issues),
`TestArchImports`, `TestGolden`, and `make archfit` dogfood all green.

---

## 8. Adjacent tool lanes evaluated (hygiene, not coupling)

A retry review surfaced four tool families archfit does not run. Recording them with
a sharp boundary: **none measures coupling, distance, or volatility** — they detect
_hygiene_ (unused deps, dead surface) or _security_, which sit next to
`dependency_graph_health`/`deprecated_dep_count`, not inside `coupling_balance`. Each
reuses a runtime archfit already pins for that language's primary graph tool, so on
an already-instrumented repo they add **no new prerequisite**. Numbers: this host,
warm cache.

| Tool              | Lane                       | Runtime (pinned by)   | Measured                                                              | Verdict                      |
| ----------------- | -------------------------- | --------------------- | --------------------------------------------------------------------- | ---------------------------- |
| **gocyclo**       | exact Go CCN               | Go (go/packages)      | archfit **0.08s**, 49 fns > CCN 15 (tests incl.)                      | adopt for Go — §5.2          |
| **cargo-machete** | Rust unused deps           | Rust (cargo metadata) | yazi **0.54s**, 5 unused deps / 4 crates                              | candidate (opt-in)           |
| **deptry**        | Python missing/unused deps | uv/Python (grimp)     | ccgram fast; DEP001 count inflated by first-party self-imports        | candidate _with config only_ |
| **knip**          | TS dead files/exports/deps | Node (depcruise)      | codegraph **6.62s cold** (npx-installs knip); rich dead-export output | candidate (TS only)          |

- **gocyclo** is the standout — exact per-function Go CCN, one static binary, no
  Python pin — and the only one that improves an _existing_ signal; promoted into
  §5.2.
- **deptry needs config to be trustworthy.** On ccgram its "imported but missing"
  list is dominated by the package importing _itself_ (`'ccgram' imported but
missing…`), a src-layout false positive. A raw "183 missing-import hits" count is
  inflated by this; the real external misses are few (`PIL`, `telegram`). Any
  integration must pass known-first-party config or the signal is noise.
- **cargo-machete** is fast and clean (no false positives observed on yazi); the
  count is checkout-/flag-dependent — `--with-metadata` finds more but mutates
  `Cargo.lock`, so report the conservative default (5/4 here).

**Keep / skip (confirmed):** keep rust-analyzer scip over cargo-modules (§5.3) and
jscpd (unique near-clone signal, §5.5); skip tokei/scc (LOC already covered by the
in-process `loc` signal) and go-callvis as a _metric_ (visualization only, not a
scorable fact). The **security lane** (semgrep / CodeQL / govulncheck) is
deliberately out of scope — a different question (vulnerabilities/taint vs coupling)
that belongs in a separate gate, noted only to make the boundary explicit. Installed
Go dead-surface tools (`deadcode`, `staticcheck`) additionally fail on Go 1.26 repos
when built with an older toolchain — a version-pin hazard if ever wired in.

---

## 9. Using the tools correctly — configuration audit (a tool that returns "none" is often misconfigured)

Per the operating principle: before deciding a tool can't supply a metric, confirm
archfit is invoking it correctly. Auditing every extractor's flags against each
tool's current docs surfaced **one real metric-quality loss in a primary tool** and
several recoverable-with-config cases. Verified against code and re-measured.

### 9.1 dependency-cruiser `--ts-config` is never auto-detected — silent edge loss in a primary metric (FIX)

archfit invokes depcruise as `depcruise <src> --output-type json --no-config`
(`internal/extract/ts/ts.go:88`) and appends `--ts-config` **only when the user has
explicitly set `TSConfig` in config** (`ts.go:89–91`). There is **no
auto-detection** — the default path passes no `--ts-config`. Consequence: without
the tsconfig, depcruise falls back to bare Node resolution and **cannot resolve
`paths`/`baseUrl` aliases** (`@app/x`, workspace aliases). Those imports come back
`couldNotResolve`, and archfit's normalizer reclassifies unresolved targets as
**external** nodes (`22401`) — so internal cross-module edges via path aliases are
**silently dropped from `coupling_balance` and `blast_radius`**. This is a quality
loss in a _required, gate-path_ metric, not an opt-in extra.

- **Measured (codegraph, depcruise v18):** `--no-config` → 406 edges, 13
  `couldNotResolve`; adding `--ts-config tsconfig.json` → 406 edges, 12
  unresolved, and the one alias edge (`grammars.ts → web-tree-sitter`) flips from
  external back to a resolved internal file. codegraph has almost no path aliases so
  the delta is 1 edge; on an alias-heavy monorepo the loss is systematic (every
  cross-module aliased import disappears from coupling).
- **Root cause:** config gap in archfit, not a depcruise limit. `--ts-config` and
  `--no-config` compose fine (verified).
- **Fix (no new prereq):** implement the documented `(empty = auto)` intent — probe
  `s.Root` for `tsconfig.json`/`tsconfig.base.json` and pass `--ts-config` when
  found. One `os.Stat`, no wall-time change. Keep `--ts-pre-compilation-deps` off
  (type-only imports are not runtime coupling).

### 9.2 deptry (candidate, §8) — naive invocation is mostly noise; correct config makes it trustworthy

deptry's raw signal on ccgram is dominated by misconfiguration, confirming §8's
caveat with numbers (deptry 0.25.1):

| Invocation                                    | DEP001 | self-import false positives |
| --------------------------------------------- | ------ | --------------------------- |
| `deptry .` (naive — scans the package itself) | 183    | 76                          |
| `deptry src` (scan the source root)           | 107    | 0                           |
| `deptry src` + `package_module_name_map`      | ~0     | 0                           |

Two independent fixes: (1) pass the **source root** as deptry's ROOT (not `.`), which
auto-classifies the package as first-party and kills the self-import hits; (2) a
`[tool.deptry] package_module_name_map` for pypi-name ≠ import-name packages
(`python-telegram-bot`→`telegram`, `Pillow`→`PIL`) clears the rest. With both,
DEP001/DEP002 → ~0 real findings. **For archfit:** auto-detect the src root from
`[tool.hatch…]`/`[tool.setuptools…]` packaging config, run `deptry <srcroot>`, let it
read any existing `[tool.deptry]` block, and **surface deptry's name-map warning
count as a confidence signal** (high warnings → lower confidence). The map itself is
per-project knowledge archfit cannot synthesize — so the honest design is opt-in +
confidence-weighted, never blocking. (Codex's raw "183 missing imports" was this
noise; real external misses ≈ 0 once configured.)

### 9.3 archfit's own `n/a` dimensions are principled abstentions, not misconfiguration

Running `archfit check --full` on its own fully-instrumented repo, exactly two
dimensions read `n/a` — and both are correct abstentions, not tool failures:

- **`encapsulation` n/a** — it scores `contract / (contract + intrusive)`
  cross-boundary edges; archfit's Go graph has no edges classified contract/intrusive
  (functional/model/unknown are excluded by design), so there is nothing to score.
  Recoverable only by real intrusive edges + strength labels, never by config. This
  is the abstain-not-fake rule working as intended.
- **`change_locality` n/a** — it is **delta-mode-only** (per-change drift; report-only,
  never gates). In a plain non-delta `check` it is n/a by definition; even with
  `--base HEAD~5` it stays n/a here when the diff yields no cross-module drift. Its
  `n/a` is invocation-scoped (needs a delta with qualifying changes), not a defect.

The contrast with §9.1/§9.2 is the point: distinguish **"tool misconfigured →
fix the invocation"** (depcruise, deptry) from **"signal genuinely absent → abstain"**
(encapsulation, change_locality). Only the first kind is a bug.

### 9.4 Coverage gains available from already-pinned tools (no new prerequisite)

The largest untapped metrics (detailed in §2.1) need **no new tool** — only deeper
use of one already required for that language. Highest-value, in priority order:

1. **depcruise `--ts-config`** (§9.1) — recover the dropped alias edges. _Fixes an
   active loss; do first._
2. **grimp `find_illegal_dependencies_for_layers`** — ready-made layer-violation
   checks archfit currently reimplements internally; grimp already pins the Python
   runtime, so this is free coverage.
3. **go/packages `NeedTypes`/`TypesInfo`** — a type-checked call/reference graph
   (SCIP-grade precision) without the SCIP indexer's 12–17s cost, on the Go
   toolchain already required.
4. **ast-grep relational rules** (`inside`/`has`/`follows`, metavar constraints) —
   more structural facts (and the §5.2 CCN proxy) for the `sg` prereq already paid.

These are pure coverage additions; none changes the prerequisite set. They are the
"maximize metrics" half of the operating principle, complementary to the §5 cuts.

---

## Appendix: confidence & caveats

- Timings are single-host, warm-cache where noted; cold caches (first scip-go,
  first `uv run --with`, gitnexus index build) are called out individually.
- `go/packages` is in-process; its row uses `go list -e -deps -json ./...` as a
  faithful proxy (archfit's `NeedDeps` load is comparable).
- "~13 graph metrics" is the count of metrics reading `in.Graph`; exact list in §2 fn1.
- **Cold tool-resolve was not measured.** All tools were pre-installed, so every
  run hit warm caches and "no network." The known CI flakiness from a cold
  `npx`/`bunx depcruise` fetch or a first-ever `uv run --with grimp` download is
  real but untested here — treat first-run-on-fresh-CI as an unmeasured risk.
- §5.5's pure-Go jscpd replacement is a _proposal_, not a measured win. §5.2 was
  validated per-function (ρ=0.838, recall 1.00 at CCN>15; the per-file ρ=0.987 is
  partly tautological — see §7.1). §5.1's drop holds but its _mechanism_ was
  corrected: fan-in refuted (ρ=0.43); SCIP encodes the same data but needs new
  wiring to supply the factor; and there is no SyntaxFacts fallback — see §7.
- §7 was validated on archfit's own Go repo only; §7.1's proxy still needs the
  TS/Py/Rust ports confirmed, and §7.2's ρ=0.43 is one repo's package graph.
- §9.1's depcruise edge-loss is **verified in code** (`ts.go:89–91`, no
  auto-detect) but its _magnitude_ was measured only on codegraph, which is
  alias-light (delta = 1 edge); the systematic-loss claim for alias-heavy monorepos
  is reasoned from the resolution mechanism, not measured on such a repo. §9.2's
  deptry counts are one repo (ccgram). The §9.4 coverage-gain list is capability
  (per tool docs/API), not yet wired or benchmarked.
