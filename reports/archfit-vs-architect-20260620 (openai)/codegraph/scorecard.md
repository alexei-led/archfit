# archfit scorecard

**Rubric version:** 1
**Overall:** 50/100 (mixed)
**Config hash:** `674de3b456721db4b547d15f551fec2b2f43c74bd71a93120f34c428ad0c2925`

## Dimensions

### boundary_integrity — 60/100 (mixed) · confidence: low
no gate-level boundary violations; intended boundaries hold
- encapsulation: n/a (no classified cross-boundary edges)
- 0 active gate violations

### coupling_balance — 90/100 (strong) · confidence: medium
no unbalanced coupling detected (strength × distance × volatility balanced, or cohesive)
- no unbalanced-coupling advisories over classified edges

### dependency_graph_health — 43/100 (mixed) · confidence: high
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 2
- blast-radius hubs: 1
- unstable modules (I>0.7): 20
- propagation cost: 0.11

### cohesion_modularity — 5/100 (critical) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 11
- hidden-coupling pairs: 22
- cross-module duplication pairs: 87

### change_locality — 70/100 (serviceable) · confidence: high
change locality: how much change crosses intended module boundaries
- co-changing module pairs: 23
- change-amplifying hubs: 0

### architecture_fitness — 33/100 (poor) · confidence: high
architecture intent enforced by executable fitness checks
- enforcement signals present: 3.3/10 (1/3 signals); arch_tests: __tests__/search-query-parser.test.ts

### analysis_confidence — 100/100 (strong) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage 1.00
- scip: ok
- gitnexus: ok
- lizard: ok
- jscpd: ok
