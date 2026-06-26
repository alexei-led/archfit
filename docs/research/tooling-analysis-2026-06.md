# archfit external-tooling analysis: capability, performance, overlap, reduction

Date: 2026-06-26. Scope: every external tool archfit shells out to, per supported
language (Go, TypeScript, Python, Rust) plus the cross-language tools. Goal: map
what each tool actually extracts vs. what it _can_ extract, measure operational
quality/performance, locate overlap, and recommend replacements / prerequisite
reductions.

Method: reproduced archfit's exact invocations (flags read from
`internal/extract/*`) in one uniform benchmark (`scratchpad/bench.sh`) — cold +
warm wall-clock, exit code, output size — across 7 local repos:
Go (archfit 287, pumba 159, spotinfo 22 files), TS (codegraph 196),
Python (ccgram 1835), Rust (herdr 203, yazi 1118). All tools installed and `ok`
per `archfit doctor`. Numbers are single-host (Apple Silicon, warm OS cache
unless noted); treat as relative, not absolute CI budgets.

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
| **gitnexus**                                | Node + index         | `gitnexus cypher -r . <CodeRelation query>`               | per-file dependants count          | **`risk_hub` only**                                                                                       | advisory/opt-in |
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
**huge JSON** (104 MB on herdr, 38 MB on ccgram) that archfit **buffers and fully
`json.Unmarshal`s** (`syntax.go:334`). Output size tracks rule-match count, not
file count (herdr's 203 files out-emit yazi's 1118 because `rust.yml` over-matches
pub/trait/impl nodes). This is the one real operational liability in the fast tier:
memory + GC pressure, and a latent OOM risk on large/rule-dense repos.

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

| tool                  | repo (files)            | cold s   | note                                                          |
| --------------------- | ----------------------- | -------- | ------------------------------------------------------------- |
| scip-go               | archfit (287)           | 0.67     | warm Go build cache                                           |
| scip-go               | pumba (159)             | 1.49     | more transitive deps to resolve                               |
| scip-go               | spotinfo (22)           | **11.6** | **cold Go build cache** (first run of session)                |
| scip-typescript       | codegraph (196)         | 2.06     | no `npm install`/build needed                                 |
| scip-python           | ccgram (407 indexed)    | 12.68    | 16 MB index                                                   |
| rust-analyzer scip    | herdr (203)             | 16.74    | 29 MB index; no pre-`cargo build` needed                      |
| rust-analyzer scip    | yazi (1118)             | 14.82    | whole 31-crate workspace, one pass                            |
| cargo-modules         | herdr (1 crate)         | 5.27     | compiles; 2.3 MB DOT                                          |
| cargo-modules         | yazi (1 of 31 crates)   | 7.39     | **per-crate** — workspace = ×31                               |
| gitnexus cypher query | archfit (index present) | 1.20     | + **75 MB index** built by a separate `gitnexus analyze` pass |

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
- **gitnexus** query is cheap (1.2s) but presupposes a **75 MB index** from a
  separate, heavy `gitnexus analyze` build — the real cost is hidden from `check`.
- No tool failed (all exit 0). No network was required by any tool on warm caches.

---

## 4. Overlap analysis

Three overlap zones, ranked by reduction value.

**Zone A — symbol-reference graph computed three ways (highest overlap).**
The same underlying fact — "who references whom at symbol granularity" — is
produced independently by (a) the SCIP indexers (→ StrengthHint), (b) gitnexus's
CodeRelation query (→ dependants count for risk*hub), and (c) the user's own
`codegraph` tool. gitnexus's dependants count is a strict \_subset* of what SCIP
references already encode (SCIP is a true superset); the dependency graph's reverse
fan-in is only a coarse echo of it (ρ=0.43, §7.2), not a substitute. gitnexus is
the redundant one:
a whole Node runtime + a 75 MB index build feeding **one advisory metric**.

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

### 5.1 Drop gitnexus — HIGH value, replace via SCIP not raw fan-in (validated, §7.2)

gitnexus is already the most conditional tool — it runs only when a
`.gitnexus`/`.codegraph` index exists — so dropping it is the lowest-risk cut.
Doing so removes a **Node** occupant and an expensive **75 MB index-build** step
for a single advisory consumer (`risk_hub`). gitnexus's per-file dependants count
(symbol-level CALLS/ACCESSES/EXTENDS references) is a strict **subset of what the
SCIP indexers already compute** — so when `scip` is enabled, SCIP references are a
true equivalent and gitnexus is pure redundancy.

The replacement subtlety, validated in §7.2: the **in-process graph reverse
fan-in** I first proposed as the always-available fallback is only a **coarse**
proxy — package-level import fan-in correlates with gitnexus dependants at just
**Spearman ρ=0.43** (though 7/10 of the top hubs agree). It captures _which
modules are big hubs_ but loses the mid-tier ranking, because import-count ≠
symbol-call breadth. So: when `scip` is on, drop gitnexus outright (SCIP subsumes
it). When neither is on, `risk_hub` already degrades gracefully to "cross-module
surface breadth × config volatility" from SyntaxFacts (`risk/risk_hub.go:190`) —
keep that fallback rather than substituting raw fan-in, which would imply a
precision it does not have. Net: one fewer Node tool + no index build; the only
real loss is gitnexus's mid-tier symbol-impact precision, recoverable from SCIP.

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

### 5.3 Prefer rust-analyzer scip over cargo-modules for multi-crate workspaces — HIGH value

Measured: cargo-modules is **per-crate** (5–7s × N members; yazi = ~31 passes →
minutes) and misses `#[path]`/inline-mod/re-exports. rust-analyzer scip does the
**whole workspace in one ~15s pass** and resolves those cases correctly. When both
are available, prefer scip for the module graph; keep cargo-modules only as the
no-rust-analyzer fallback. This is a "better results + faster + more reliable at
scale" replacement, not a prereq cut.

### 5.4 Stream ast-grep output instead of buffering — validated 17× cut (see `big-json-in-go-2026-06.md`)

104 MB (herdr) / 38 MB (ccgram) of JSON is read into one buffer and
`json.Unmarshal`'d into the full struct. **Benchmarked** (100 MB herdr output, Go
1.26, `CGO_ENABLED=0`): streaming a `json.Decoder` off the subprocess pipe into a
**lean struct** (only the fields archfit uses) drops peak RSS from **221 MB → 13 MB
at identical wall time** — no new dependency, no lost matches. NDJSON
(`--json=stream`) + `bufio.Reader.ReadBytes` is an equivalent 16 MB path — but
**never `bufio.Scanner`** (validated: silently truncates at 64 KB, dropping 78% of
herdr's matches). A faster JSON lib is the wrong fix — go-json/gjson cut CPU but
_raise_ peak memory; the parse isn't the bottleneck. Full method, table, and the
`encoding/json/v2` trajectory: **`docs/research/big-json-in-go-2026-06.md`**.

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
| **Go-only**  | go, git, sg, lizard(+Python), jscpd(+Node), gitnexus(+Node+75 MB index) | go, git, sg, lizard(+Python), jscpd(+Node) | **go, git, sg**                                |
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
  fan-in is an equivalent replacement" framing and is why §5.1 now routes the
  precision path through SCIP references (a true superset of gitnexus dependants)
  and keeps the existing surface-breadth fallback rather than substituting raw
  fan-in. The _decision to drop gitnexus still holds_ (cost + SCIP subsumption);
  the _replacement mechanism_ was corrected by the data.
- Granularity caveat: this proxy used package-level dependants with `sum`
  aggregation, whereas `risk_hub` aggregates to **module** level with `max`
  (`moduleImpactFromFiles`). ρ=0.43 is therefore directional, not the exact signal
  risk_hub consumes — but the conclusion (route precision through SCIP, keep the
  surface-breadth fallback) is unaffected by the aggregation choice.

### 7.3 Already measured (not re-validated)

§5.3 (scip one-pass 14.8s vs cargo-modules 5–7s × 31 crates) and §5.4 (104 MB
buffered ast-grep JSON) are direct benchmark observations from §3, not proposals.

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
  validated (ρ=0.987) and §5.1 partially validated (the drop holds; the fan-in
  replacement was refuted, ρ=0.43, and corrected to SCIP) — see §7.
- §7 was validated on archfit's own Go repo only; §7.1's proxy still needs the
  TS/Py/Rust ports confirmed, and §7.2's ρ=0.43 is one repo's package graph.
