## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: serviceable**

### Dimensions

**coupling_balance**: No unbalanced-coupling advisories over classified edges. Integration strength, distance, and volatility appear balanced, so maintenance effort stays predictable with no distributed-monolith risk surfacing.
**cohesion_modularity**: No god modules or hidden-coupling pairs; only 1 cross-module duplication pair. High strength at low distance reads as healthy cohesion, not a knowledge-boundary problem.
**change_locality**: Zero co-changing module pairs means change rarely crosses intended boundaries, so co-evolution stays contained. Two change-amplifying hubs are worth watching but do not currently force cascading changes.
**dependency_graph_health**: No import cycles, but 2 blast-radius hubs, 1 unstable module (I>0.7), and propagation cost of 0.50 mean roughly half the graph can feel a change. Hubs concentrate maintenance effort across knowledge boundaries.
**boundary_integrity**: No active gate violations and intended boundaries hold, but confidence is low because no cross-boundary edges were classified. Integrity is plausible rather than proven.
**architecture_fitness**: Zero enforcement signals (0/3) means architectural intent is not protected by executable fitness checks. Without guardrails, today's clean boundaries can erode into a big ball of mud unnoticed over time.

### Top Risks

**No executable fitness enforcement** (modules: internal/mcp, internal/spot)
Architecture fitness scores 0/10 with no enforcement signals present. The current healthy cohesion and balanced coupling rely on convention alone, so drift toward a big ball of mud would go undetected.
Balancing move: Reduce volatility of intent by adding one executable fitness check (e.g. import-boundary assertion).

**Blast-radius hubs raise propagation cost** (modules: cmd/spotinfo, internal/spot)
Two change-amplifying hubs and a 0.50 propagation cost mean a single edit can ripple across half the graph. internal/spot carries the highest fan-in and LOC, making it a natural amplifier of cascading changes.
Balancing move: Shorten distance/concentration by splitting internal/spot's most-depended-on surface into a stable contract module.

### Subdomain Suggestions

- internal/spot: core — Highest fan-in and LOC and a blast-radius hub, indicating the central, high-volatility domain logic.
- internal/mcp: supporting — Moderate fan-in with one outbound dependency suggests a supporting integration layer around the core.
- cmd/spotinfo: generic — Entry-point command wiring with outbound-only dependencies behaves like low-volatility composition glue.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
