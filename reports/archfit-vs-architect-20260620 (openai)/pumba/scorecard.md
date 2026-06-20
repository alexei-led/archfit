# archfit scorecard

**Rubric version:** 1
**Overall:** 51/100 (mixed)
**Config hash:** `fd7d471d8d12394486ab49213a19eda1a4715bd5cc6702c8c817cfeff171337d`

## Dimensions

### boundary_integrity — 60/100 (mixed) · confidence: low
no gate-level boundary violations; intended boundaries hold
- encapsulation: n/a (no classified cross-boundary edges)
- 0 active gate violations

### coupling_balance — 50/100 (mixed) · confidence: high
coupling carries elevated maintenance effort but no distributed-monolith edges
- 10 BC edges (2 rollups); weighted mean maintenance-effort 5.0/10
- worst-case high/high/high (distributed-monolith) edges: 0

### dependency_graph_health — 69/100 (serviceable) · confidence: high
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 0
- blast-radius hubs: 3
- unstable modules (I>0.7): 7
- propagation cost: 0.20

### cohesion_modularity — 33/100 (poor) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 1
- hidden-coupling pairs: 16
- cross-module duplication pairs: 13

### change_locality — 96/100 (strong) · confidence: high
change locality: how much change crosses intended module boundaries
- co-changing module pairs: 0
- change-amplifying hubs: 1

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
