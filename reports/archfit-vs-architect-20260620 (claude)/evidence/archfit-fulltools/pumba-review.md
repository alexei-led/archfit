## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: mixed**

### Dimensions

**boundary_integrity**: No gate-level boundary violations were observed and intended knowledge boundaries hold, but confidence is low because no cross-boundary edges were classified. There is no evidence of a big ball of mud at the boundary level.
**coupling_balance**: Coupling carries elevated maintenance effort (weighted mean 5.0/10 across 10 BC edges) but zero worst-case high/high/high edges, so no distributed-monolith risk is present. Two medium advisories on imbalanced coupling from cmd indicate cascading-change pressure worth watching.
**dependency_graph_health**: The graph is acyclic with manageable propagation cost (0.20), but 3 blast-radius hubs and 7 unstable modules (I>0.7) concentrate co-evolution pressure. These hubs raise the cost of cascading changes when they shift.
**cohesion_modularity**: 16 hidden-coupling pairs and 13 cross-module duplication pairs signal eroded knowledge boundaries and implicit co-evolution that the dependency graph does not capture. One god module compounds the maintenance effort by concentrating logic where strength is high.
**change_locality**: Zero co-changing module pairs means change rarely crosses intended boundaries, keeping cascading changes local. Only one change-amplifying hub exists, so locality is healthy.
**architecture_fitness**: No executable fitness checks are present (0/3 enforcement signals), so intended boundaries are unprotected against drift. Without enforcement, the hidden coupling and duplication can silently grow into a big ball of mud.
**analysis_confidence**: Full file extraction coverage (1.00) and all tools (scip, gitnexus, lizard, jscpd) reporting ok make this review highly trustworthy. Findings can be acted on with confidence.

### Top Risks

**No executable enforcement of intended architecture**
Zero of three fitness enforcement signals are present, leaving intended knowledge boundaries unguarded. Combined with poor cohesion, hidden coupling and duplication can accumulate undetected and drift toward a big ball of mud.
Balancing move: Add executable fitness checks to lock the existing boundaries before erosion increases maintenance effort.

**Hidden coupling and duplication eroding cohesion** (modules: pkg/chaos, pkg/container)
16 hidden-coupling pairs and 13 duplication pairs create implicit co-evolution that the dependency graph does not surface, so changes in one place silently force changes elsewhere. The god module concentrates this strength and raises cascading-change risk.
Balancing move: Reduce strength by extracting the duplicated logic into a shared low-volatility module, collapsing the hidden coupling.

**Imbalanced coupling from cmd to chaos and runtime** (modules: cmd, pkg/chaos)
Two medium advisories flag imbalanced coupling from cmd into pkg_chaos and pkg_runtime, where cmd's high outbound fan-out (10) makes it a change-amplifying entry point. Elevated edge maintenance effort here can propagate cascading changes across the runtime adapters.
Balancing move: Lower strength by routing cmd through a thin contract/interface rather than depending on internal models of chaos and runtime.

### Subdomain Suggestions

- pkg/chaos: core — High inbound fan-in (13) and central role in the chaos behaviour make it the volatile core domain.
- pkg/runtime/docker: supporting — Runtime adapters integrate external container engines and evolve at a moderate, vendor-driven pace.
- pkg/container: generic — Highest inbound fan-in (16) with zero outbound suggests a stable shared abstraction with low volatility.
- pkg/util: generic — Small leaf module with no outbound dependencies behaves as low-volatility infrastructure.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
