# archfit scorecard

**Rubric version:** 1
**Overall:** 64/100 (serviceable)
**Config hash:** `d50dccb86aa11c804fe5ebd44ec525e446bf2fa29f13b1e89fb8bfbbd78f2cf4`

## Dimensions

### boundary_integrity — 60/100 (mixed) · confidence: low
no gate-level boundary violations; intended boundaries hold
- encapsulation: n/a (no classified cross-boundary edges)
- 0 active gate violations

### coupling_balance — 90/100 (strong) · confidence: medium
no unbalanced coupling detected (strength × distance × volatility balanced, or cohesive)
- no unbalanced-coupling advisories over classified edges

### dependency_graph_health — 63/100 (serviceable) · confidence: high
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 0
- blast-radius hubs: 5
- unstable modules (I>0.7): 17
- propagation cost: 0.09

### cohesion_modularity — 9/100 (critical) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 3
- hidden-coupling pairs: 76
- cross-module duplication pairs: 15

### change_locality — 94/100 (strong) · confidence: high
change locality: how much change crosses intended module boundaries
- cross-module edges from changed files: 0
- co-changing module pairs: 1
- change-amplifying hubs: 0

### architecture_fitness — 67/100 (serviceable) · confidence: high
architecture intent enforced by executable fitness checks
- enforcement signals present: 6.7/10 (2/3 signals); arch_tests: cmd/archfit/enrich_subdomains_test.go, cmd/archfit/enrich_test.go, cmd/archfit/init_classify_test.go (+75 more); ci_linter: .github/workflows/ci.yaml, .github/workflows/release.yaml

### analysis_confidence — 100/100 (strong) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage 1.00
- scip: ok
- gitnexus: ok
- lizard: ok
- jscpd: ok
