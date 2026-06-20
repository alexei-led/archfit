# archfit scorecard

**Rubric version:** 1
**Overall:** 47/100 (mixed)

## Dimensions

### boundary_integrity — 50/100 (mixed) · confidence: low
no gate-level boundary violations; encapsulation unmeasured, so boundary integrity is unconfirmed
- encapsulation: n/a (no classified cross-boundary edges) — boundary baseline unmeasured
- 0 active gate violations

### coupling_balance — 50/100 (mixed) · confidence: low
coupling unmeasured: no classified edges and insufficient extraction coverage
- no edges classified; extraction coverage insufficient (0 classified edges)

### dependency_graph_health — 60/100 (mixed) · confidence: low
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 0

### cohesion_modularity — 60/100 (mixed) · confidence: low
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- hidden-coupling pairs: 0

### change_locality — 60/100 (mixed) · confidence: low
change locality: how much change crosses intended module boundaries
- co-changing module pairs: 0

### architecture_fitness — 0/100 (critical) · confidence: low
architecture intent enforced by executable fitness checks
- enforcement signals present: 0.0/10 (0/3 signals)

### analysis_confidence — 0/100 (critical) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage: n/a (no extractor contributed)
- scip: absent
- gitnexus: absent
- lizard: absent
- jscpd: absent

## Required tools missing (6)

These analyzers did not run; the metrics they feed are n/a, not strong.

- **dependency-cruiser** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `npm install -g dependency-cruiser`
- **gitnexus** [gate: warn] — affects risk_hub; install: `see docs/guide — git-history change-coupling index`
- **go/packages** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `https://go.dev/dl (bundled with the Go toolchain)`
- **grimp** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `uv tool install grimp / pip install grimp`
- **jscpd** [gate: warn] — affects functional_candidates; install: `npm install -g jscpd`
- **lizard** [gate: warn] — affects complexity; install: `uv tool install lizard / pip install lizard`
