## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: mixed**

### Dimensions

**cohesion_modularity**: 11 god modules, 22 hidden-coupling pairs, and 87 duplication pairs signal a big-ball-of-mud drift. Duplicated logic forces co-evolution across knowledge boundaries with no contract to absorb change, multiplying maintenance effort whenever a shared concept shifts.
**dependency_graph_health**: 2 import cycles and 1 blast-radius hub create cascading-change paths, and 20 modules are unstable (I>0.7). Propagation cost stays low at 0.11, so blast radius is contained today, but cycles harden into distributed-monolith coupling if left unbroken.
**architecture_fitness**: Only 1 of 3 enforcement signals present (3.3/10). Intended knowledge boundaries are not guarded by executable fitness checks, so cohesion erosion and new cycles can land silently and accumulate maintenance effort.
**coupling_balance**: No unbalanced-coupling advisories over classified edges. Strength × distance × volatility stays balanced; high-strength edges that exist are local and cohesive, which is healthy and not penalised.
**change_locality**: 23 co-changing module pairs but zero change-amplifying hubs. Most change stays inside intended boundaries, so cascading changes are limited; the co-change pairs are worth watching against the hidden-coupling findings.
**boundary_integrity**: Zero active gate violations and intended boundaries hold, but confidence is low because no cross-boundary edges were classified. Encapsulation is effectively unmeasured rather than proven strong.

### Top Risks

**Duplication-driven hidden coupling** (modules: src/extraction/tree-sitter.ts, src/resolution/import-resolver.ts)
87 duplication pairs and 22 hidden-coupling pairs mean concepts are copied across modules with no shared contract. Each change must be replicated everywhere, forcing implicit co-evolution across knowledge boundaries and inflating maintenance effort.
Balancing move: Lower strength: extract duplicated logic behind a stable contract module so co-evolution flows through one seam.

**God modules concentrating change** (modules: src/extraction/tree-sitter.ts, src/mcp/tools.ts)
11 LOC-skewed god modules (tools.ts 3375, tree-sitter.ts 4214) bundle unrelated responsibilities. Their high internal strength is local so it is not flagged, but their size makes them blast-radius hubs that amplify cascading changes when touched.
Balancing move: Shorten distance: split each god module by responsibility so high-strength logic stays cohesive within smaller local boundaries.

**Import cycles unguarded by fitness checks** (modules: src/extraction/index.ts, src/resolution/index.ts)
2 import cycles exist while only 1 of 3 enforcement signals is active. Cycles create bidirectional co-evolution and, without executable fitness checks, new cycles can accrue undetected, edging toward a distributed monolith.
Balancing move: Reduce strength: break the 2 cycles by inverting one dependency edge onto a contract interface.

### Subdomain Suggestions

- src/extraction/tree-sitter.ts: core — Largest module (4214 LOC) driving parsing logic, central to product value and likely high volatility.
- src/resolution/import-resolver.ts: core — 1825 LOC of resolution logic is the differentiating analysis behaviour and changes frequently with language support.
- src/mcp/tools.ts: supporting — Large tool-surface module that orchestrates capabilities but is adjacent to, not the heart of, the analysis domain.
- src/installer/targets/registry.ts: generic — Installer target wiring is low-volatility infrastructure shared across many similar target files.
- src/types.ts: generic — High fan-in (55) shared type definitions act as a stable contract surface with no outbound dependencies.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
