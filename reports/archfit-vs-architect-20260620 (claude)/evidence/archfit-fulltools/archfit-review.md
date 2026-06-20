## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: serviceable**

### Dimensions

**boundary_integrity**: No gate-level boundary violations were classified, so intended knowledge boundaries appear to hold, but confidence is low because there were no classified cross-boundary edges to test encapsulation against.
**coupling_balance**: No unbalanced-coupling advisories surfaced over the classified edges, meaning the strength × distance × volatility product stays in balance and there is no distributed-monolith signal at the edge level.
**dependency_graph_health**: There are zero import cycles and propagation cost is low at 0.09, but 5 blast-radius hubs and 17 unstable modules (I>0.7) concentrate co-evolution pressure, so changes in those hubs can trigger cascading changes across dependents.
**cohesion_modularity**: 76 hidden-coupling pairs, 3 god modules, and 15 cross-module duplication pairs indicate cohesion is eroding toward a big ball of mud. Hidden coupling that crosses knowledge boundaries raises maintenance effort even though high-strength low-distance cohesion itself is healthy.
**change_locality**: Only 1 co-changing module pair and zero change-amplifying hubs mean that, historically, change has stayed local and rarely crosses intended module boundaries.
**architecture_fitness**: Executable enforcement is partly present (2 of 3 signals, 6.7/10) with arch tests and a CI linter, so architectural intent is co-evolving with code but not yet fully guarded by fitness checks.
**analysis_confidence**: File extraction coverage is complete at 1.00 and all extractors (scip, gitnexus, lizard, jscpd) ran cleanly, so the findings rest on trustworthy tool coverage.

### Top Risks

**Hidden coupling erodes module cohesion** (modules: cmd/archfit, internal/initcfg, internal/metrics/modularity)
76 hidden-coupling pairs combined with 3 god modules (modularity at 1515 LOC, initcfg at 2018 LOC, archfit at 2752 LOC) mean strength accumulates implicitly outside declared interfaces. This raises maintenance effort and risks cascading changes across knowledge boundaries even though no gate violation fires.
Balancing move: Lower strength: extract the hidden-coupling seams in the god modules into explicit contract-level interfaces so the implicit functional coupling becomes a visible, weaker dependency.

**Cross-module duplication invites silent co-evolution** (modules: internal/metrics/boundary, internal/metrics/modularity)
15 cross-module duplication pairs mean the same model knowledge lives in two places that must co-evolve manually. Divergence between duplicates is a classic distributed-monolith maintenance tax where one change forces an unseen second change.
Balancing move: Reduce distance: consolidate the duplicated logic into a single shared local module so the duplicated knowledge lives behind one boundary.

**Blast-radius hubs concentrate change pressure** (modules: internal/config, internal/model/diagnostic, internal/model/graph)
diagnostic (fan-in 49), config (fan-in 32), and graph (fan-in 32) are high-fan-in hubs among the 5 blast-radius hubs and 17 unstable modules. If their models are volatile, any change cascades to many dependents and amplifies maintenance effort.
Balancing move: Reduce volatility: stabilize these widely-depended-on hub modules as low-change contract/model surfaces so dependents stop co-evolving with them.

### Subdomain Suggestions

- internal/model/diagnostic: generic — With fan-in 49 and tiny outbound it is a stable shared model contract consumed everywhere, so it should be treated as low-volatility generic infrastructure.
- internal/model/coupling: core — At 689 LOC with fan-in 17 it encodes the central coupling model that the tool exists to compute, making it high-value core domain.
- internal/metrics/modularity: core — As the largest metrics module (1515 LOC) implementing the cohesion analysis logic, it carries the differentiating domain logic and is change-prone.
- internal/config: generic — Fan-in 32 with minimal outbound marks it as broadly-shared configuration plumbing that should stay low-volatility.
- internal/initcfg: supporting — At 2018 LOC but low fan-in it is large bootstrap/initialization scaffolding supporting the core rather than differentiating it.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
