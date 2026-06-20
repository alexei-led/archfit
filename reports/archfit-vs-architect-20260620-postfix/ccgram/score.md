# archfit scorecard

**Rubric version:** 1
**Overall:** 47/100 (mixed)
**Config hash:** `38fd9d5005aab980d694fc9b99ee73c7f55d76eae11c5da3fc24961b629c6a5d`

## Dimensions

### boundary_integrity — 33/100 (poor) · confidence: low
boundary violations present: forbidden dependencies cross intended boundaries
- encapsulation 0.78 (mixed)
- 3 active gate violation(s): ae4ef4b3, b64908dd, cac3cea9

### coupling_balance — 50/100 (mixed) · confidence: high
coupling carries elevated maintenance effort but no distributed-monolith edges
- 421 BC edges (58 rollups); weighted mean maintenance-effort 5.0/10
- worst-case high/high/high (distributed-monolith) edges: 0

### dependency_graph_health — 18/100 (critical) · confidence: high
dependency graph health: cycles, hubs, instability, and propagation cost
- import cycles: 3
- blast-radius hubs: 115
- unstable modules (I>0.7): 73
- propagation cost: 0.26

### cohesion_modularity — 36/100 (poor) · confidence: high
cohesion: god modules, hidden coupling, and duplication (cohesion = high strength + low distance is healthy, not penalised)
- god modules (LOC skew): 8
- hidden-coupling pairs: 3

### change_locality — 80/100 (serviceable) · confidence: high
change locality: how much change crosses intended module boundaries
- co-changing module pairs: 2
- change-amplifying hubs: 2

### architecture_fitness — 67/100 (serviceable) · confidence: high
architecture intent enforced by executable fitness checks
- enforcement signals present: 6.7/10 (2/3 signals); arch_tests: tests/ccgram/test_handler_layering_invariants.py, tests/ccgram/test_query_layer_only_for_handlers.py; ci_linter: .github/workflows/ci.yml

### analysis_confidence — 90/100 (strong) · confidence: high · meta (scores the review, not the architecture)
review trustworthiness given tool coverage and evidence depth
- file extraction coverage 1.00
- scip: ok
- gitnexus: ok
- lizard: absent
- jscpd: absent

## Required tools missing (4)

These analyzers did not run; the metrics they feed are n/a, not strong.

- **dependency-cruiser** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `npm install -g dependency-cruiser`
- **go/packages** [gate: warn] — affects coverage, coupling_balance, encapsulation, cycle, blast_radius; install: `https://go.dev/dl (bundled with the Go toolchain)`
- **jscpd** [gate: warn] — affects functional_candidates; install: `npm install -g jscpd`
- **lizard** [gate: warn] — affects complexity; install: `uv tool install lizard / pip install lizard`
