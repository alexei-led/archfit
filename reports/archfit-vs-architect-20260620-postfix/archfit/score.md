# archfit scorecard

**Rubric version:** 1
**Overall:** 66/100 (serviceable)
**Config hash:** `c011e7b3502f123890c61c1832405ff60ae357fab73846193ddc94940285df42`

## Dimensions

### boundary_integrity — 50/100 (mixed) · confidence: low
no gate-level boundary violations; encapsulation unmeasured, so boundary integrity is unconfirmed
- encapsulation: n/a (no classified cross-boundary edges) — boundary baseline unmeasured
- 0 active gate violations

### coupling_balance — 90/100 (strong) · confidence: medium
no unbalanced coupling detected (strength × distance × volatility balanced, or cohesive)
- no unbalanced coupling among 0 classified edges

### dependency_graph_health — 63/100 (serviceable) · confidence: high
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 0
- blast-radius hubs: 5
- unstable modules (I>0.7): 17
- propagation cost: 0.09

### cohesion_modularity — 34/100 (poor) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 3
- hidden-coupling pairs: 76

### change_locality — 94/100 (strong) · confidence: high
change locality: how much change crosses intended module boundaries
- co-changing module pairs: 1
- change-amplifying hubs: 0

### architecture_fitness — 67/100 (serviceable) · confidence: high
architecture intent enforced by executable fitness checks
- enforcement signals present: 6.7/10 (2/3 signals); arch_tests: cmd/archfit/autopilot_test.go, cmd/archfit/enrich_subdomains_test.go, cmd/archfit/enrich_test.go (+79 more); ci_linter: .github/workflows/ci.yaml, .github/workflows/release.yaml

### analysis_confidence — 95/100 (strong) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage 1.00
- scip: ok
- gitnexus: ok
- lizard: ok
- jscpd: absent

## Required tools missing (3)

These analyzers did not run; the metrics they feed are n/a, not strong.

- **dependency-cruiser** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `npm install -g dependency-cruiser`
- **grimp** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `uv tool install grimp / pip install grimp`
- **jscpd** [gate: warn] — affects functional_candidates; install: `npm install -g jscpd`
