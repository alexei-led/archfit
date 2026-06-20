## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: mixed**

### Dimensions

**coupling_balance**: Weighted mean maintenance effort sits at 5.0/10 with zero distributed-monolith edges. Two advisories flag imbalanced coupling from cmd into pkg/chaos and pkg/runtime, where cmd's broad fan-out (10) carries elevated effort but no cascading high/high/high knowledge boundaries.
**cohesion_modularity**: 16 hidden-coupling pairs and 13 cross-module duplication pairs signal co-evolution pressure not captured by explicit imports. One god module plus duplication risks drifting toward a big ball of mud as shared logic mutates independently across boundaries.
**architecture_fitness**: Zero of three enforcement signals present. Without executable fitness checks, intended knowledge boundaries are unguarded and any future cascading change can erode structure silently.
**dependency_graph_health**: No import cycles and low propagation cost (0.20) keep the graph healthy. However 3 blast-radius hubs and 7 unstable modules (I>0.7) concentrate change risk at a few high fan-in nodes like pkg/container and pkg/chaos.
**change_locality**: Zero co-changing module pairs means change rarely crosses intended boundaries. A single change-amplifying hub is the only locality concern, so co-evolution is currently well contained.
**boundary_integrity**: No gate-level boundary violations and intended boundaries hold, but confidence is low because no cross-boundary edges were classified. Integrity is unverified rather than proven strong.
**analysis_confidence**: Full file coverage (1.00) with scip, gitnexus, lizard, and jscpd all healthy. Findings rest on trustworthy tool coverage.

### Top Risks

**Unenforced architecture intent** (modules: cmd, pkg/container)
No executable fitness checks exist (0/3 signals). With hub modules like pkg/container at fan-in 16, structural drift can accumulate unnoticed and cascading changes will not be caught at build time.
Balancing move: Add one fitness check asserting the intended dependency direction into hub modules.

**Hidden coupling and duplication** (modules: pkg/runtime/docker, pkg/runtime/podman)
16 hidden-coupling pairs and 13 duplication pairs imply shared logic co-evolving across large runtime modules. This functional-strength coupling without explicit contracts risks divergence and a creeping big ball of mud.
Balancing move: Reduce strength by extracting duplicated runtime logic into a shared contract module.

**cmd imbalanced coupling fan-out** (modules: cmd, pkg/chaos)
Two advisories flag imbalanced coupling from cmd into pkg/chaos and pkg/runtime. cmd's fan-out of 10 reaches across many knowledge boundaries, raising maintenance effort even without distributed-monolith edges.
Balancing move: Lower strength by narrowing cmd's dependencies to thin contract interfaces.

### Subdomain Suggestions

- pkg/chaos: core — High fan-in (13) chaos orchestration is the product's distinctive purpose and likely volatile.
- pkg/runtime/docker: supporting — Large runtime adapter (1503 LOC) supports chaos execution against a specific platform.
- pkg/container: generic — Stable hub with fan-in 16 and zero outbound, behaving as shared infrastructure.
- pkg/util: generic — Small utility module with no outbound dependencies is low-volatility infrastructure.
- cmd: supporting — CLI entry wiring coordinates core subdomains but holds little distinctive logic itself.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
