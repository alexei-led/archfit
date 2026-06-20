# archfit scorecard

**Rubric version:** 1
**Overall:** 67/100 (serviceable)
**Config hash:** `25f594782358e5c2786fd8a0d32104e7e7c421dec76cab1bb3ae5d51a9ec2264`

## Dimensions

### boundary_integrity — 60/100 (mixed) · confidence: low
no gate-level boundary violations; intended boundaries hold
- encapsulation: n/a (no classified cross-boundary edges)
- 0 active gate violations

### coupling_balance — 90/100 (strong) · confidence: medium
no unbalanced coupling detected (strength × distance × volatility balanced, or cohesive)
- no unbalanced-coupling advisories over classified edges

### dependency_graph_health — 77/100 (serviceable) · confidence: high
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 0
- blast-radius hubs: 2
- unstable modules (I>0.7): 1
- propagation cost: 0.50

### cohesion_modularity — 95/100 (strong) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 0
- hidden-coupling pairs: 0
- cross-module duplication pairs: 1

### change_locality — 82/100 (strong) · confidence: high
change locality: how much change crosses intended module boundaries
- cross-module edges from changed files: 2
- co-changing module pairs: 0
- change-amplifying hubs: 2

### architecture_fitness — 0/100 (critical) · confidence: high
architecture intent enforced by executable fitness checks
- enforcement signals present: 0.0/10 (0/3 signals)

### analysis_confidence — 100/100 (strong) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage 1.00
- scip: ok
- gitnexus: ok
- lizard: ok
- jscpd: ok
