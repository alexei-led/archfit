# Dual-approach comparison — collected data (working notes)

Date: 2026-06-20. archfit v0.4.0-2-g8243a08. LLM: anthropic/claude-opus-4-8 (off-gate).

## A. archfit (deterministic) cross-repo scorecard — latest

| repo | overall | boundary | coupling_bal | dep_health | cohesion | change_loc | arch_fitness | conf(meta) | coverage gaps |
|---|---|---|---|---|---|---|---|---|---|
| pumba (Go) | 62 serviceable | 60 mixed/lo | **90 strong/med** | 69 | 58 | 96 | **0 crit** | 80 | scip,gitnexus,lizard,jscpd absent |
| spotinfo (Go) | 70 serviceable | 60 mixed/lo | **90 strong/med** | 77 | **100 strong** | 92 | **0 crit** | 80 | scip,gitnexus,lizard,jscpd absent |
| ccgram (Py) | 47 mixed | 33 poor/lo | 50 mixed/hi | **18 crit** | 36 poor | 80 | 67 | 90 | lizard,jscpd absent (scip PRESENT) |
| codegraph (TS) | 54 mixed | 60 mixed/lo | **90 strong/med** | 43 mixed | 30 poor | 70 | 33 poor | 85 | scip absent (dep-cruiser only) |
| archfit (Go) | 68 serviceable | 60 mixed/lo | **90 strong/med** | 63 | 34 poor | 94 | 67 | 95 | jscpd absent (fullest coverage) |

archfit findings count (check --full): pumba 0, spotinfo 0, **ccgram 61 (1 high/60 med)**, codegraph 0, archfit 0.
→ Only ccgram (scip on) + archfit produced edge-classified findings. Go/TS repos: scip off → 0 classified edges.

## B. archfit delta (score --base=<prev tag>) — version trend

| repo | base tag | overall now→base | change_locality now→base |
|---|---|---|---|
| pumba | 1.1.3 | 62→62 (flat) | 96→96 (stable) |
| spotinfo | 2.0.1 | 70→67 | 92→72 (improved) |
| ccgram | v3.3.3 | 47→39 | 80→30 (big improve) |
| codegraph | v0.9.4 | 54→46 | 70→20 (big improve) |
| archfit | v0.2.0 | 68→60 | 94→44 (big improve) |
→ Reproducible quality trend over releases. Architect arm has NO native equivalent.

## C. archfit JSON (machine-actionable) keys
schema_version, verdict, base, head, config_hash, metrics, findings, file_facts, dynamic_imports, **agent_tasks**, tool_coverage, summary. SARIF format also available. → AI-agent repair blocks; architect arm produces prose+plan only.

## D. archfit LLM `review` arm
- Interprets archfit's OWN deterministic metrics in BC language (strength×distance×volatility, "distributed monolith", "balancing move: lower strength / reduce volatility"). Does NOT gather independent evidence.
- Succeeded: pumba, spotinfo, codegraph, archfit. **FAILED: ccgram** — "model response is not the required JSON: unexpected end of JSON input" (largest repo, 61 findings + 15 under-specified modules → oversized payload, no chunking). Reproducible.
- Minor bug: empty "Balancing move:" line in pumba review (line 19).

## E. architect skill (LLM, independent evidence) — scorecards

### pumba — Overall GOOD ~6.0/10
modularity 7, coupling_balance 6, dep_direction 7, encapsulation 5, blast 6, change_locality 7, testability 7, arch_fitness 3.
Findings: (1) mock_*.go in production `pkg/container` package (intrusive, wrong layer) — `pkg/container/mock_Client.go`; (2) `pkg/runtime/podman/client.go:10,43` imports sibling `pkg/runtime/docker` (lateral adapter coupling); (3) fat `container.Client` passed through generic builder `pkg/chaos/cmd/builder.go:23` bypassing narrow per-cmd interfaces; (4) `.archfit.yaml:13` gate warn-only, no arch_test, no CI gate; (5) `mocks/` mislabeled layer:core.

### spotinfo — Overall ADEQUATE (5–6)
dims (section 4): 5 Adequate, 6 Adequate, 9 Exemplary, 6 Adequate, 6 Adequate/med, 7 Good (+arch_fitness low). 
Findings: F1 duplicate `spotClient` interface `main.go:159`+`mcp/server.go:26` (coupling drift); F2 dual AWS config load no shared factory `score.go:70`+`liveprice.go:60`; F3 **silent mock fallback in production** `score.go:75` (correctness); F4 score-assignment bug `score.go:155`; F5 712-line god file `main.go`; F7 exported impl detail `score.go:30`.

### codegraph — Overall POOR (4.3/10)
modularity 5, coupling_balance 4, dep_direction 4, encapsulation 5, blast 4, testability 5, arch_fitness 3 (critical).
Findings: (1) **extraction↔resolution mutual cycle** (intrusive, both highest-churn) `src/extraction/index.ts:24`↔`src/resolution/frameworks/drupal.ts:50`; (2) `src/db/queries.ts:21-23` imports search/extraction — layer inversion; (3) concrete `QueryBuilder` injected into 6 domain modules, no port/interface; (4) `mcp/tools.ts` god file 3,375 LOC, imports 8 modules; (5) no enforced fitness (eslint/dep-cruiser), `.archfit.yaml` all 12 modules in one core layer, gate:warn.

### ccgram — Overall GOOD 5.9/10
Modularity 6, Coupling 5 Fair, Dep Direction 6 (+ encaps/blast/test/fitness). Findings: god modules hook.py(1234 LOC), directory_callbacks(1076), polling_state(1018); providers→tmux_manager functional/dist=2 unbalanced; bot↔main bidirectional cycle; providers→cc_commands inverted layering; deferred imports mask latent cycles. Biggest risk: hottest path (text handling) coupled to polling singleton.
→ vs archfit ccgram (BOTH had scip/full coverage): archfit 47 mixed, dep_health **18 critical (cycles)**, cohesion 36 (god modules), coupling 50, 61 findings. **CONVERGENCE: both found the cycles + god modules.** archfit more granular (61 specific findings); architect more contextual. Divergence here is small — proving earlier divergences were COVERAGE-driven, not methodology-driven.

### archfit (self) — Overall GOOD 8.0/10
Architect verified the import-ring fitness test is REAL (uses golang.org/x/tools/go/packages import-graph traversal, not string matching; all 30+ pkgs ok). Hexagon holds for domain; small subprocess-boundary gap. Couplings: C1 core→config (model, low dist, med volatility = balanced); C2 ports→config (model — config view types in port signatures); adapters→toolrun not checked by arch_test. Standout strength = fitness enforcement.
→ vs archfit-self deterministic: overall 68 serviceable, arch_fitness 67, cohesion 34 poor, coupling 90. **archfit UNDER-rates itself**: mechanical signal-counting gives fitness 67 + cohesion 34, while the architect (recognizing the real working import-ring test + hexagon) gives 8.0. archfit's mechanical scoring both over-scores coupling (blind) AND under-scores fitness/cohesion.

## E2. RESEARCH (grounded)
- Khononov BOOK = CONTINUOUS quantified model: numeric (1–5) per strength/distance/volatility → composite risk (multiplicative/weighted); high-risk = all three high together. **Volatility from domain/subdomains, NOT commit history** (commits = org noise). archfit's measured strength×distance×volatility scorecard ≈ closest to book's quantified ambition; BUT gitnexus change-coupling leans on commit history (cautioned against); pinned-subdomain volatility (correct) only when configured.
- Fitness functions (Ford/Parsons): objective, triggered-static = build-breaking CI gates. **archfit IS a fitness-function tool; architect RECOMMENDS them.** Limits: tunnel vision, false sense of security (passing only covers selected dims).
- Reliability (2023–26): static = deterministic, reproducible, near-zero FP on exact matching. LLM = fragile on exhaustive graph reasoning (hallucinated/missed links, misread contracts → FP). LLM value = local abductive/semantic judgment. **Hybrid consensus: deterministic exhaustive graph facts + LLM local abductive judgment.**
- Sources: coupling.dev, oreilly Balancing Coupling, nealford Building Evolutionary Architectures, thoughtworks fitness-function-driven, dev.to 2026 deterministic-vs-LLM study, arxiv 2601.17915.

## H0. CODEX SECOND OPINION (verdict OVERCLAIMED) + empirical confirmation
Codex critique (integrated):
- (a) "archfit more reliable" defensible ONLY as "reproducible/auditable/operationalizable", NOT "trustworthy". A deterministic 90/strong on missing evidence is WORSE than honest non-determinism — false assurance at the layer users depend on. Reproducible wrongness helps debug the tool, not assess architecture. Correct claim: archfit is more reproducible/automatable/regression-friendly; NOT more reliable until coverage failure degrades scores to n/a/low-confidence instead of green.
- (b) BC split too generous to archfit. Khononov ≠ "quantify edges"; needs MEANINGFUL strength + semantic distance + DOMAIN volatility + design judgment. archfit lacks semantic volatility unless humans pin subdomains, and defaults high with no edges → does NOT currently win BC alignment. Architect aligns better with BC INTENT; archfit aligns with measurement FORM + repeatability, only AFTER coverage gates + domain labels exist.
- (c) "Combine" additive only with STRICT roles: deterministic extractor = facts; LLM = critique gaps + semantics; humans = pin domain volatility; deterministic gate = enforce drift. Feeding archfit's LLM review INTO another LLM = redundant + framing-laundering. Additive part is NOT "LLM+LLM"; it is machine graph facts + INDEPENDENT semantic review + human domain labels + CI fitness gate.
- (d) BIGGEST under-weighted: coverage confidence must DOMINATE headline score. "strong" while required evidence source disabled = CONTRACT BUG, not minor. Invalidates cross-repo comparison unless coverage normalized / missing-data dims excluded.
- Verdict: OVERCLAIMED. Strongest correction: archfit is not "more reliable"; it's more reproducible/automatable but less trustworthy on uncovered dims unless missing evidence forces n/a/low-confidence instead of green.

**EMPIRICAL CONFIRMATION (spotinfo, full tools scip+gitnexus+lizard all "ok"):** coupling_balance STILL 90/strong, cohesion STILL 100/strong — yet architect found duplicate spotClient interface, silent mock fallback, 712-line god file. => The false-green is NOT only coverage; archfit's METRICS also miss semantic coupling/cohesion issues even at full coverage, AND the headline doesn't degrade on thin evidence. Both = contract/metric gaps, not config. (codegraph full-tools re-run pending = decisive test for the intrusive cycle.)

## H. REVISED VERDICT (post-codex, post-spotinfo-empirical; codegraph re-run pending)
- **Reliability:** archfit is MORE REPRODUCIBLE / auditable / automatable / regression-friendly — NOT more trustworthy. Until coverage gates the headline score (n/a on missing evidence, as encapsulation already does), its green can be false assurance. The architect is non-reproducible but its findings are evidence-cited and it does not emit false-greens (it investigates until it finds or honestly reports limits).
- **BC alignment:** ARCHITECT currently aligns better with Khononov's INTENT (semantic distance, domain volatility, design judgment, catches unmodeled coupling). archfit aligns with the measurement FORM (continuous strength×distance×volatility scorecard) + repeatability + IS a fitness function — but only realizes BC intent once (1) coverage gates exist and (2) subdomain/volatility labels are pinned (which most repos lack, and archfit's gitnexus signals lean on commit history Khononov warns against).
- **Better overall:** complementary, but the honest framing is role-separated (codex c), not "feed one to the other." archfit = exhaustive deterministic graph facts + CI gate + machine output (agent_tasks/SARIF) + version deltas. Architect = independent semantic review that catches what archfit's metrics/coverage miss + domain judgment.
- **COMBINE (strict roles):** machine graph facts (archfit deterministic core) → INDEPENDENT semantic review (architect-style LLM gathering its OWN evidence, NOT narrating archfit's metrics) → human pins domain volatility (archfit enrich --subdomains drafts, human approves) → archfit re-measures with correct volatility + coverage-gated scores → deterministic CI fitness gate enforces drift. archfit's own `review` (narrate-own-metrics) is the WEAKEST link (codex: laundering) and should be replaced by the independent-evidence LLM role.
- **#1 archfit fix surfaced:** coverage must DOMINATE the headline — degrade coupling_balance/cohesion to n/a/low-confidence when classified-edge or complexity coverage is insufficient (mirror the existing encapsulation n/a behavior). Cross-repo score comparison is invalid until then.

## H1. FULL-TOOLS RE-RUN (fair comparison) — KEY RESULT, rebalances verdict
Decisive codegraph re-run with scip+gitnexus+lizard+jscpd all "ok":
- Overall 50 mixed (≈ architect 4.3 Poor — now CONVERGES). cohesion 30→**5 CRITICAL** (god files caught by lizard/jscpd — matches architect). dependency_graph_health 43 with **"import cycles: 2"** = the extraction↔resolution cycle IS detected (in dep_health, the architect's #1 finding).
- BUT coupling_balance STILL **90 strong** with scip on. The intrusive cycle is accounted in dependency_graph_health, not coupling_balance.
=> REVISED interpretation: archfit, FULLY TOOLED, broadly AGREES with the architect at the OVERALL + cohesion + dep_health level. The criticism NARROWS from "blind false-green" to "the `coupling_balance` DIMENSION LABEL is misleading" — it measures unbalanced *classified-edge advisories* only; cycles/layer-inversions live in dep_health. Overall band is right; one dimension's name oversells. Much fairer to archfit than the under-tooled run implied. The earlier cohesion divergences WERE coverage (now fixed); the coupling_balance one is metric-semantics.
- archfit `review` (LLM) FAILS on high-evidence repos (ccgram, codegraph-full) — payload overflow → empty/invalid JSON. Robustness scales inversely with finding count.
- Before/after proves the USER'S POINT: comparing under-tooled archfit to the architect was unfair; properly tooled, the gap is much smaller and mostly about dimension labeling + the headline-coverage-gate bug, not detection capability.
(full 5-repo full-tools table → distilled2.txt once re-run completes)

## H_OLD. (superseded draft — kept for trace)
- **More reliable / reproducible: archfit, clearly.** Deterministic, config_hash, same input→same output, machine-actionable (agent_tasks/SARIF), version deltas, self-reported coverage. Research backs static > LLM on reproducibility + FP. CAVEAT: archfit's reliability is COVERAGE-CONDITIONAL — silent false-greens (codegraph coupling 90) when scip off; deterministic but under-detecting. Fix: gate coupling_balance to n/a/low-conf when no classified edges (as encapsulation already does).
- **More aligned with Balanced Coupling: a split.** archfit aligns with the MEASUREMENT ambition (continuous quantified strength×distance×volatility = the book's direction) and IS a fitness function. Architect aligns with the JUDGMENT nuance (semantic subdomain/volatility, intent, config-independent). Khononov's book wants quantified composite scoring → archfit edges ahead WHEN configured; architect handles volatility-from-domain more faithfully (archfit needs pinned labels most repos lack, and leans on commit-history signals the book warns against).
- **Better overall: neither alone — they're complementary by construction.** archfit = exhaustive deterministic graph facts + reproducible gate; architect = abductive semantic judgment + catches what archfit's coverage misses. Equal-coverage convergence (ccgram) proves they measure the same reality; archfit faster/repeatable, architect broader/blind-spot-resistant.
- **COMBINE (the real answer):** (1) archfit produces deterministic evidence (classified edges, metrics, change-coupling, deltas, agent_tasks) → grounding. (2) Feed it to the architect/LLM for semantic judgment + to catch coverage gaps. (3) Close the loop with archfit `enrich --subdomains`: LLM drafts the subdomain+volatility labels (archfit's biggest config gap, and BC's hardest dimension) → human pins → archfit then measures with CORRECT volatility → archfit `review` narrates. This is the research's hybrid consensus, and archfit already ships 80% of it.

## F. KEY DIVERGENCES (the heart of the comparison)
1. **codegraph coupling_balance: archfit 90/strong (0 findings) vs architect 4/10 Poor (intrusive cycle + layer inversion + god file).** Root: scip absent in codegraph archfit run → no classified edges → default 90. SILENT FALSE-GREEN.
2. **pumba: archfit 90 coupling/strong vs architect found podman→docker lateral coupling + fat interface + mocks-in-prod.** archfit missed (scip absent).
3. **spotinfo cohesion: archfit 100/strong vs architect found 712-line god file `main.go`.** archfit god-module (LOC-skew, package-level) didn't trip on file-level god file.
4. **Consistency bug in archfit:** same zero-classified-edge data → `boundary_integrity` says "encapsulation: n/a (no classified edges)" (honest) but `coupling_balance` says "90 strong" (false-green). coupling_balance should degrade to n/a/low-confidence like encapsulation does when scip coverage absent.
5. **Agreement:** both arms flag architecture_fitness/enforcement as the weakest dimension everywhere (no executable gates). archfit binary 0; architect graded 3.
6. archfit catches what it's configured+covered for, reproducibly + with machine output + version deltas; architect catches more (config-independent evidence gathering) but slower, non-reproducible, prose-only.

## G. PENDING
- architect ccgram, architect archfit (self) summaries
- research agent synthesis (BC theory, fitness functions, LLM-vs-deterministic reliability)
- advisor + codex second opinions on the verdict
