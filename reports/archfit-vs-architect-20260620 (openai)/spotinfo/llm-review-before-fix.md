## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: serviceable**

### Dimensions

**boundary_integrity**: No gate-level boundary violations were observed and intended knowledge boundaries appear to hold, though confidence is low because no cross-boundary edges were classified. There is no evidence of intrusive coupling crossing module lines.
**coupling_balance**: No unbalanced-coupling advisories surfaced across the classified edges, so strength × distance × volatility appears balanced or cohesive. There is no current distributed-monolith signal where high strength meets high distance and high volatility.
**dependency_graph_health**: The graph is acyclic with no import cycles, but 2 blast-radius hubs and 1 unstable module (I>0.7) sit alongside a propagation cost of 0.50, meaning roughly half the graph can feel a given change. This raises maintenance effort through potential cascading changes around the hub modules.
**cohesion_modularity**: No god modules and no hidden-coupling pairs were detected, indicating healthy cohesion where strength stays local. A single cross-module duplication pair is the only minor blemish and does not constitute a big ball of mud.
**change_locality**: No co-changing module pairs were found, so change tends to stay within intended boundaries rather than forcing co-evolution across them. The 2 change-amplifying hubs are worth watching but are not currently spilling changes across knowledge boundaries.
**architecture_fitness**: Zero of three enforcement signals are present (0.0/10), so architectural intent is not protected by any executable fitness checks. Without enforcement, balanced coupling today can silently erode toward a distributed monolith as the code evolves.
**analysis_confidence**: File extraction coverage is complete at 1.00 and all tools (scip, gitnexus, lizard, jscpd) ran cleanly. The review rests on trustworthy, full-coverage evidence.

### Top Risks

**No executable fitness checks guarding intent** (modules: cmd/spotinfo, internal/mcp, internal/spot)
Architecture fitness scored critical with 0/3 enforcement signals, so the currently balanced coupling has no automated guardrail. As modules co-evolve, drift toward intrusive coupling or new boundary crossings would go undetected until maintenance effort spikes.
Balancing move: Add at least one executable fitness check (e.g. a layering/dependency assertion) to lock in the existing boundaries — a single enforcement dimension change.

**Blast-radius hubs with moderate propagation cost** (modules: internal/mcp, internal/spot)
Two blast-radius hubs plus one unstable module combine with a propagation cost of 0.50, so a change in a hub can ripple across roughly half the graph. internal/spot carries the highest inbound fan-in (3) and largest LOC (1433), making it the most likely source of cascading changes.
Balancing move: Reduce strength of dependencies on internal/spot by introducing a stable contract/interface so dependents bind to a thinner surface rather than the full module.

### Subdomain Suggestions

- internal/spot: core — Highest inbound fan-in and largest codebase, indicating it carries the central spot-pricing domain logic the rest of the system depends on.
- internal/mcp: supporting — Acts as a mediating hub with moderate fan-in and outbound to core, supporting the protocol/integration concern rather than owning core domain rules.
- cmd/spotinfo: generic — Entry-point/wiring module with low fan-in that orchestrates rather than holds volatile domain knowledge.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
