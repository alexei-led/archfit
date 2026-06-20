## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: serviceable**

### Dimensions

**cohesion_modularity**: 76 hidden-coupling pairs, 3 god modules, and 15 duplication pairs signal a drift toward a big ball of mud. Hidden coupling crosses knowledge boundaries without explicit contracts, so co-evolution happens implicitly and maintenance effort compounds. The god modules (initcfg ~2018 LOC, modularity ~1515, cmd/archfit ~2896) concentrate functional strength at low distance — but the duplication and hidden links erode internal cohesion.
**dependency_graph_health**: Zero import cycles and a low propagation cost (0.09) keep blast radius contained. But 17 unstable modules (I>0.7) and 5 blast-radius hubs mean some knowledge boundaries are thin; high fan-in models like diagnostic, config, and graph must stay stable to avoid cascading changes.
**coupling_balance**: No unbalanced-coupling advisories over classified edges. Strength, distance, and volatility appear balanced — high-strength dependencies sit at low distance (cohesion), which is healthy and not penalised.
**change_locality**: Only 1 co-changing module pair and 0 change-amplifying hubs. Change stays local to intended module boundaries, so co-evolution rarely spills across knowledge boundaries — minimal distributed-monolith risk on the temporal axis.
**boundary_integrity**: No active gate violations and intended boundaries hold, but confidence is low because no cross-boundary edges were classified. Integrity is asserted rather than demonstrated.
**architecture_fitness**: 2 of 3 enforcement signals present (arch_tests plus ci_linter). Executable fitness checks back the intended architecture, though one signal is missing to fully guard against drift.

### Top Risks

**Hidden coupling erodes cohesion** (modules: internal/initcfg, internal/metrics/modularity)
76 hidden-coupling pairs and 15 duplication pairs mean modules co-evolve through implicit, undeclared links rather than explicit contracts. Combined with god modules, this drives a big-ball-of-mud trajectory where edits cascade across knowledge boundaries despite a clean import graph.
Balancing move: Lower strength: extract duplicated logic into shared contract-level modules so hidden functional coupling becomes explicit and weaker.

**God modules concentrate change** (modules: cmd/archfit, internal/initcfg)
cmd/archfit (~2896 LOC, fan-out 40) and initcfg (~2018 LOC) carry outsized responsibility. Their LOC skew makes them maintenance bottlenecks where every concern co-evolves, raising effort even at low distance.
Balancing move: Shorten distance/responsibility: split each god module along subdomain seams to restore single-purpose cohesion.

**Unstable high-fan-in models** (modules: internal/config, internal/model/diagnostic)
diagnostic (fan-in 49) and config (fan-in 32) are widely depended-upon. If they sit among the 17 unstable modules, any volatility there triggers cascading changes across many consumers.
Balancing move: Reduce volatility: freeze these as stable contract-level models so dependents stop co-evolving with their churn.

### Subdomain Suggestions

- internal/model/diagnostic: generic — High fan-in, tiny outbound shared data model that many consumers depend on — should be stable and low-volatility.
- internal/config: supporting — Widely consumed configuration with moderate size; supports the core but should not churn frequently.
- internal/metrics/modularity: core — Largest analysis module driving the product's central cohesion scoring — expect high volatility, isolate it.
- internal/classify: core — Subdomain/coupling classification is central differentiating logic and likely high-volatility.
- internal/toolrun: generic — High fan-in, no outbound — an infrastructure tool-runner that should remain stable and low-volatility.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
