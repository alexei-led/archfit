# BC Book-Model Dogfood Scorecard

**Date:** 2026-06-23  
**Plan:** `docs/plans/completed/20260623-bc-book-alignment.md` Tasks 10, 15
**Binary (Task 10):** `v0.7.0-10-g5b15405`  
**Binary (Task 15):** `v0.7.0-17-g2df36b2`  
**Config hash (Task 15):** `ac84f630b84fa69321258e6129842f6790ded6fe5c820a3b70d652679c41b3eb`

---

## Complete Journey: BEFORE → AFTER

| Stage                             | Overall | coupling_balance            | Scored | Abstained | External | Notes                                         |
| --------------------------------- | ------- | --------------------------- | ------ | --------- | -------- | --------------------------------------------- |
| Pre-change (bespoke scorer)       | ~82     | ~60 / mixed / **false**     | 536    | 0         | 0        | Invented ordinals for unknown-strength edges  |
| Task 10: book-model first dogfood | 57      | 60 / mixed / **low**        | 89     | 447       | 0        | Honest abstain; external in denominator       |
| Task 14: external-edge exclusion  | 60      | 78 / serviceable / **high** | 89     | 0         | 447      | External edges excluded from scored/abstained |

### What changed at each stage

**Pre-change → Task 10:** The homegrown formula (`risk = (1−|Sₙ−Dₙ|)×Vₙ`) invented ordinals for every edge including 447 with unknown strength. Replacing it with the book formula (`balance = max(|S−D|, 10−V) + 1`) required abstaining on unknown-strength edges. The 447 abstained edges diluted the scored fraction to 16%, capping `coupling_balance` confidence at `low` and applying a confidence penalty to the value (60 instead of 78). The drop from ~82 to 57 overall is the honest price of removing invented surface.

**Task 10 → Task 14:** The 447 abstained edges were all external/library imports (stdlib, third-party packages) whose distance is `DistanceUnknown` — they are not internal coupling seams at all. Excluding them from the `coupling_balance` denominator is correct: external dependency coupling is a `dependency_graph_health` concern, not a `coupling_balance` measurement. With the exclusion, the internal-only scored fraction rises to 100% (89/89), confidence becomes `high`, and the value rises to 78 — reflecting the true mean book balance of 8.0/10 across all 89 internal cross-boundary edges. The overall score rises from 57 to 60.

**Interpretation:** The score rose because external/library edges are no longer diluting the internal coupling measurement — not because anything was faked or thresholds were relaxed. The 447 external edges are still surfaced (transparently, in the scorecard evidence string and `classified_edges.external` count). The 78/serviceable/high reading is honest: all 89 internal cross-boundary edges are classified `contract` or `symmetric`, same-owner, low-volatility — which the book formula scores as well-balanced.

---

## Full Banded Scorecard — Task 15 (current)

| Dimension               | Score      | Band        | Confidence |
| ----------------------- | ---------- | ----------- | ---------- |
| **Overall**             | **60/100** | **mixed**   | —          |
| boundary_integrity      | 50/100     | mixed       | low        |
| coupling_balance        | 78/100     | serviceable | high       |
| dependency_graph_health | 63/100     | serviceable | high       |
| cohesion_modularity     | 5/100      | critical    | high       |
| change_locality         | 94/100     | strong      | high       |
| architecture_fitness    | 67/100     | serviceable | high       |
| analysis_confidence     | 90/100     | strong      | high       |

---

## Task 10 Scorecard (book-model, pre-exclusion, for reference)

| Dimension               | Score      | Band        | Confidence |
| ----------------------- | ---------- | ----------- | ---------- |
| **Overall**             | **57/100** | **mixed**   | —          |
| boundary_integrity      | 50/100     | mixed       | low        |
| coupling_balance        | 60/100     | mixed       | low        |
| dependency_graph_health | 63/100     | serviceable | high       |
| cohesion_modularity     | 5/100      | critical    | high       |
| change_locality         | 94/100     | strong      | high       |
| architecture_fitness    | 67/100     | serviceable | high       |
| analysis_confidence     | 80/100     | serviceable | high       |

---

## Edge Distribution (Task 15)

| Category                            | Count                                |
| ----------------------------------- | ------------------------------------ |
| Total edges                         | 774                                  |
| Same-module (excluded from scoring) | 238                                  |
| Cross-boundary internal scored      | 89 (100% of internal cross-boundary) |
| Cross-boundary internal abstained   | 0                                    |
| External/library edges excluded     | 447                                  |

### Strength breakdown (89 scored internal edges)

| Strength  | Count |
| --------- | ----- |
| contract  | 84    |
| symmetric | 5     |

### Distance breakdown (89 scored)

| Distance                | Count |
| ----------------------- | ----- |
| cross_module_same_owner | 89    |

### Volatility breakdown (89 scored)

| Volatility | Count |
| ---------- | ----- |
| low        | 89    |

### Severity breakdown (89 scored)

| Severity | Count |
| -------- | ----- |
| low      | 89    |

Mean book balance: **8.0/10** → value 78 (linear rescale: `round(100 × (8−1)/9)`)

---

## Honestly-Low Dimensions (and Why)

### cohesion_modularity: 5/100 / critical (high confidence)

Pre-existing structural debt. Not caused by BC book changes. Flagged correctly at high confidence:

- 5 god modules: `cmd/archfit` (4158 LOC, 17×), `internal/initcfg` (2698 LOC, 11×), `internal/metrics/modularity` (1513 LOC, 6×), `internal/score` (1001 LOC, 4×), `internal/engine` (994 LOC, 4×)
- 75 hidden-coupling pairs (top: `internal/engine` at 18)
- 18 clone-duplicated cross-module pairs

Gap-6 exemption (role-aware god-module exemption for `composition_root`/`generated` modules) was explicitly rejected in the plan — keeps scorer honest. This debt warrants a separate refactoring effort.

### boundary_integrity: 50/100 / mixed / low confidence

Low confidence because `encapsulation` is `n/a`. No intrusive cross-boundary edges exist (correct for a Go module where `internal/` visibility is compiler-enforced). `boundary_integrity` lifts when encapsulation becomes measurable via real intrusive edges or SCIP private-access evidence — not by crediting rule presence (Gap-2, also rejected).

### analysis_confidence: 90/100 / strong (improved from 80)

Task 10 had 2 of 4 structural dimensions `n/a` (encapsulation, change_locality). Task 15 has 1 of 4 `n/a` (encapsulation only; change_locality now measures). The cap rose from 80 to 90.

---

## Book Behavior on Archfit Edges

### Symmetric edges: 5 detected, correctly flagged

Five symmetric edges (detected via clone analysis) are scored at book Strength=9. With same-owner/low-volatility, they produce balance values near the top of the scale and appear in the `low` severity bucket — correct, because symmetric same-owner coupling carries no cross-team distributed-monolith risk. No spurious criticals.

### Distributed monolith detection

No high-distance symmetric edges found. Archfit does not call itself across service boundaries. The symmetric+high-distance anti-pattern detection path exists and is covered by engine regression tests (`TestRun_SmallOSS_DeployUnitBoundaryStaysFar`).

### No spurious criticals

All 89 scored internal edges are `low` severity. Gate: 0 violations, 0 warnings, 0 exceptions used.

### Agent tasks / decision tasks

0 agent_tasks emitted. Unknown-strength external edges are batched into the `external` count (transparent, not noisy). No per-edge spam.

---

## Gate Results (Task 15)

| Check                                      | Result                      |
| ------------------------------------------ | --------------------------- |
| `make build`                               | PASS                        |
| `make archfit` (dogfood gate)              | PASS (exit 0, 0 violations) |
| Determinism (double-run byte-identical)    | PASS                        |
| `go test ./internal/ -run TestArchImports` | PASS (at Task 14 commit)    |

---

## Config Follow-Ups (from Task 10 investigation, carried forward)

### 1. SCIP Go strength inference — ceiling reached (Task 12)

`tools.scip.enabled: "on"` was already set. scip-go runs successfully (`scip: ok (1000 files)`). However, Go SCIP strength classification does NOT reduce unknown internal counts further:

- scip-go produces strength edges only for observed symbol references, not for every `import` statement. Of ~1386 go file→pkg import edges, SCIP covers ~126 (9%).
- Those 126 SCIP-covered edges hit packages that config `public:` globs already classify as `contract` — the SCIP `functional` hint is never reached (config globs take precedence in `classify.go`). Net new internal classifications from SCIP: 0.
- The 447 edges turned out to be external/library (Task 14). After exclusion, 0 internal edges remain abstained.

**Conclusion:** The ceiling is reached. 89/89 internal cross-boundary edges are classified. The SCIP and LLM-label tracks (Tasks 12–13) brought the abstained count to 0; Task 14 correctly categorized the remainder as external.

### 2. cohesion_modularity debt (separate effort)

- Split `cmd/archfit` (4158 LOC, 17× median) — kong command dispatch is a natural split boundary.
- Split `internal/initcfg` (2698 LOC, 11×) into config-parse, validation, and scaffold sub-packages.
- Investigate 75 hidden-coupling pairs (top: `internal/engine` at 18) for missed abstractions.
