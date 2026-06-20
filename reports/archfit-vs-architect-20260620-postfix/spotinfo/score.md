# archfit scorecard

**Rubric version:** 1
**Overall:** 57/100 (mixed)
**Config hash:** `cd7d705a95a4b3e05f6f3864e4158042a3ba8616112fdefb15bd130242e1b4e0`

## Dimensions

### boundary_integrity — 50/100 (mixed) · confidence: low
no gate-level boundary violations; encapsulation unmeasured, so boundary integrity is unconfirmed
- encapsulation: n/a (no classified cross-boundary edges) — boundary baseline unmeasured
- 0 active gate violations

### coupling_balance — 20/100 (critical) · confidence: high
coupling carries elevated maintenance effort but no distributed-monolith edges
- 3 BC edges (2 rollups); weighted mean maintenance-effort 8.0/10
- worst-case high/high/high (distributed-monolith) edges: 0

### dependency_graph_health — 77/100 (serviceable) · confidence: high
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 0
- blast-radius hubs: 2
- unstable modules (I>0.7): 1
- propagation cost: 0.50

### cohesion_modularity — 100/100 (strong) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 0
- hidden-coupling pairs: 0

### change_locality — 92/100 (strong) · confidence: high
change locality: how much change crosses intended module boundaries
- co-changing module pairs: 0
- change-amplifying hubs: 2

### architecture_fitness — 0/100 (critical) · confidence: high
architecture intent enforced by executable fitness checks
- enforcement signals present: 0.0/10 (0/3 signals)

### analysis_confidence — 85/100 (strong) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage 1.00
- scip: absent
- gitnexus: ok
- lizard: absent
- jscpd: absent

## Required tools missing (4)

These analyzers did not run; the metrics they feed are n/a, not strong.

- **dependency-cruiser** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `npm install -g dependency-cruiser`
- **grimp** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `uv tool install grimp / pip install grimp`
- **jscpd** [gate: warn] — affects functional_candidates; install: `npm install -g jscpd`
- **lizard** [gate: warn] — affects complexity; install: `uv tool install lizard / pip install lizard`
