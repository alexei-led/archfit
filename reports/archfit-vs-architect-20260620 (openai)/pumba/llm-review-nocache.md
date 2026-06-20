## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: mixed**

### Dimensions

**boundary_integrity**: No gate-level boundary violations were detected and intended knowledge boundaries currently hold, but confidence is low because there are no classified cross-boundary edges to test against. The integrity signal is therefore unproven rather than strong.
**coupling_balance**: Coupling carries elevated maintenance effort (weighted mean 5.0/10 across 10 BC edges) but no edge reaches the worst-case high strength + high distance + high volatility distributed-monolith condition. The two imbalanced advisories from cmd flag effort hotspots worth rebalancing before they harden.
**dependency_graph_health**: The graph is acyclic with a low propagation cost of 0.20, so cascading changes are contained. However 3 blast-radius hubs and 7 unstable modules (I>0.7) concentrate co-evolution pressure on a few nodes.
**cohesion_modularity**: One god module plus 16 hidden-coupling pairs and 13 cross-module duplication pairs signal eroding knowledge boundaries drifting toward a big ball of mud. The duplication implies functional coupling that is not expressed through explicit contracts, raising maintenance effort.
**change_locality**: Zero co-changing module pairs means change stays inside intended boundaries with minimal cascading changes. A single change-amplifying hub is the only locality concern.
**architecture_fitness**: No executable fitness checks enforce architecture intent (0/3 enforcement signals), so any of the cohesion or coupling weaknesses can regress silently. Without guardrails the healthy change-locality numbers cannot be defended over time.
**analysis_confidence**: Full file extraction coverage (1.00) with all tools reporting ok makes the evidence trustworthy. Findings here can be acted on directly.

### Top Risks

**Hidden coupling and duplication eroding cohesion** (modules: pkg/chaos, pkg/runtime/containerd, pkg/runtime/docker, pkg/runtime/podman)
With 16 hidden-coupling pairs and 13 duplication pairs, functional coupling is leaking across modules without explicit contracts, raising maintenance effort. Left unchecked this drifts toward a big ball of mud where edits must co-evolve across knowledge boundaries.
Balancing move: Lower strength: extract the duplicated logic into a shared generic/util contract so the implicit functional coupling becomes an explicit contract dependency.

**cmd imbalanced coupling to chaos and runtime** (modules: cmd, pkg/chaos)
cmd has outbound fan-out of 10 and carries two medium imbalanced-coupling advisories toward pkg_chaos and pkg_runtime, concentrating maintenance effort at the entrypoint. These edges are not yet distributed-monolith risks but amplify change as the orchestration layer grows.
Balancing move: Lower strength: route cmd's dependencies on chaos and runtime through narrow command/contract interfaces rather than reaching into internals.

**No fitness functions to defend boundaries** (modules: cmd, pkg/chaos)
Architecture fitness scored 0 with no enforcement signals, so cohesion erosion and coupling imbalance can regress without detection. The strong change-locality result is currently unguarded.
Balancing move: Reduce volatility of intent: add at least one executable fitness check (e.g. cycle/boundary assertion) to lock the currently-healthy structure in place.

### Subdomain Suggestions

- pkg/chaos: core — It is the highest-fanin domain hub (inbound 13) coordinating the chaos behaviours that define the product.
- pkg/runtime/docker: supporting — Large runtime adapter (1503 LOC) implementing a swappable backend behind the chaos core.
- pkg/runtime/containerd: supporting — An interchangeable runtime integration adapter serving the core rather than holding domain rules.
- pkg/runtime/podman: supporting — Another runtime backend adapter, parallel to docker and containerd in role and volatility.
- pkg/container: generic — High-fanin (16) zero-outbound leaf providing shared container primitives, typical of a stable generic dependency.
- pkg/util: generic — Small leaf utility module (77 LOC, no outbound) with low volatility used across the system.
- cmd: supporting — CLI orchestration entrypoint wiring subdomains together rather than owning core chaos logic.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
