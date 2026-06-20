## Architecture Review (off-gate LLM narrative, anthropic/claude-opus-4-8)

**Overall: mixed**

### Dimensions

**dependency_graph_health**: 3 import cycles, 115 blast-radius hubs, and 73 unstable modules (I>0.7) signal a graph drifting toward big ball of mud. Propagation cost 0.26 means changes ripple wide. Cycles like app_bootstrap→telegram_adapter force co-evolution across knowledge boundaries.
**cohesion_modularity**: 8 god modules, 3 hidden-coupling pairs, and 41 cross-module duplication pairs erode cohesion. Duplication means the same knowledge lives in many places, so each change cascades. This is low-cohesion sprawl, not the healthy high-strength/low-distance kind.
**boundary_integrity**: Encapsulation 0.78 with 3 active gate violations: forbidden dependencies cross intended knowledge boundaries. Import cycles between handlers, providers, and app_bootstrap punch holes through module walls and invite a distributed monolith.
**coupling_balance**: 421 BC edges with weighted mean maintenance effort 5.0/10 and 58 imbalanced rollups, but zero high/high/high distributed-monolith edges. Coupling is local and internal-remote, so effort is elevated but contained—worth lowering strength on the busiest edges.
**change_locality**: Only 2 co-changing module pairs and 2 change-amplifying hubs cross intended boundaries. Most change stays within its module, so day-to-day edits rarely trigger cascading changes across boundaries.
**architecture_fitness**: Enforcement at 6.7/10 with 2 of 3 signals: arch tests for handler layering and CI linter present. Executable fitness checks hold intent, though cycles still slip through—tighten checks to cover the cycle gate.
**analysis_confidence**: File coverage 1.00 with scip, gitnexus, lizard, and jscpd all healthy. Findings rest on full extraction, so the critical graph and cohesion signals are trustworthy.

### Top Risks

**Import cycles breach module boundaries** (modules: app_bootstrap, handlers, providers, telegram_adapter)
Three cycles, including a high-severity app_bootstrap↔telegram_adapter loop plus self-cycles in handlers and providers, knot modules into co-evolving units. These functional cycles force cascading changes across knowledge boundaries and edge toward a distributed monolith.
Balancing move: Lower strength: break the app_bootstrap→telegram_adapter cycle by inverting it behind a contract/port so dependency flows one way.

**God modules and duplication erode cohesion** (modules: ccgram.handlers.polling.polling_state, ccgram.hook, ccgram.tmux_manager)
8 god modules (tmux_manager 956 LOC, polling_state 1018, hook 1234) concentrate too much knowledge, and 41 duplication pairs scatter the same logic. Edits then ripple through many sites, raising maintenance effort and inviting a big ball of mud.
Balancing move: Reduce strength: extract the duplicated logic into one owning module so co-evolving copies collapse to a single source.

**Hub-and-spoke fan-in around config and clients** (modules: ccgram.config, ccgram.telegram_client, ccgram.thread_router)
115 blast-radius hubs with config at fan-in 52 and telegram_client at 43 mean any change to these shared modules amplifies across the graph. With 73 unstable modules, propagation stays wide even at propagation cost 0.26.
Balancing move: Reduce volatility: freeze config and telegram_client contracts so high-fan-in hubs become stable, low-volatility dependencies.

### Subdomain Suggestions

- ccgram.config: generic — Fan-in 52 with one outbound edge marks it as shared infrastructure that should stay stable and low-volatility.
- ccgram.telegram_client: generic — A 597-LOC adapter with fan-in 43 and no outbound deps is integration plumbing, best kept generic and stable.
- ccgram.handlers.polling.polling_state: core — A 1018-LOC god module with heavy outbound coupling carries core orchestration logic and high volatility.
- ccgram.providers.base: supporting — Fan-in 46 abstraction underpins provider variants but is a supporting contract rather than core domain behaviour.
- ccgram.tmux_manager: supporting — A 956-LOC manager wrapping terminal control is supporting capability that should shed duplication and shrink.

---
_Review generated from deterministic archfit evidence. LLM narratives are advisory
and never affect the `check` gate._
