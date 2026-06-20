# archfit scorecard

**Rubric version:** 1
**Overall:** 36/100 (poor)
**Config hash:** `a32c8a1ba873e6de25d9596a6b052c8ecc7b19b3237b42af245fc652156583d6`

## Dimensions

### boundary_integrity — 20/100 (critical) · confidence: low
boundary violations present: forbidden dependencies cross intended boundaries
- encapsulation: n/a (no classified cross-boundary edges) — boundary baseline unmeasured
- 2 active gate violation(s): 20ebf187, 61223c3a

### coupling_balance — 20/100 (critical) · confidence: high
coupling carries elevated maintenance effort but no distributed-monolith edges
- 14 BC edges (7 rollups); weighted mean maintenance-effort 8.0/10
- worst-case high/high/high (distributed-monolith) edges: 0

### dependency_graph_health — 43/100 (mixed) · confidence: high
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 2
- blast-radius hubs: 1
- unstable modules (I>0.7): 20
- propagation cost: 0.11

### cohesion_modularity — 30/100 (poor) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 11
- hidden-coupling pairs: 22

### change_locality — 70/100 (serviceable) · confidence: high
change locality: how much change crosses intended module boundaries
- co-changing module pairs: 23
- change-amplifying hubs: 0

### architecture_fitness — 33/100 (poor) · confidence: high
architecture intent enforced by executable fitness checks
- enforcement signals present: 3.3/10 (1/3 signals); arch_tests: __tests__/search-query-parser.test.ts

### analysis_confidence — 85/100 (strong) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage 1.00
- scip: absent
- gitnexus: ok
- lizard: absent
- jscpd: absent

## Required tools missing (4)

These analyzers did not run; the metrics they feed are n/a, not strong.

- **go/packages** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `https://go.dev/dl (bundled with the Go toolchain)`
- **grimp** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `uv tool install grimp / pip install grimp`
- **jscpd** [gate: warn] — affects functional_candidates; install: `npm install -g jscpd`
- **lizard** [gate: warn] — affects complexity; install: `uv tool install lizard / pip install lizard`
