# Why architecture fitness matters

Working code is not the same as sustainable software.

Tests, type checkers, formatters, and language-specific boundary linters catch
many local defects. They do not, by themselves, protect the design of a long-lived
system:

- which modules may know about each other;
- where responsibilities live;
- how far a change is allowed to spread;
- whether coupling is healthy enough for the system to keep evolving.

AI coding agents make this problem sharper. Agents are useful at local code
production, but they work inside a bounded context window and optimize for the
current task. Without an explicit architecture signal, an agent can pass the
tests while it quietly adds a forbidden dependency, bypasses a public API,
duplicates a domain rule, or turns a clean module boundary into a shortcut.

Repeat that across many small changes and the architecture drifts.

## The failure mode

Architecture erosion is the gap between intended architecture and implemented
architecture. Architecture drift is the slow movement away from the intended
shape, often before anyone notices a hard violation.

Research on architecture erosion points to recurring causes:

- high change pressure;
- weak architecture conformance checks;
- missing or stale architecture knowledge;
- local changes that are not reviewed against system-level intent.

The cost is not cosmetic:

- **More coupling.** Modules know more about each other than they should.
- **Higher change cost.** A local feature starts requiring edits across distant
  files, teams, packages, or deploy units.
- **Cascading bugs.** A fix in one place breaks behavior in another because the
  dependency graph no longer matches the design.
- **Harder debugging.** Engineers need more context to explain one failure.
- **Higher AI cost.** Agents need more tokens and more retries to understand the
  blast radius, then often add more shortcuts while trying to fix the cascade.
- **Lost ownership.** Humans remain responsible for maintenance, incidents, and
  complexity even when an agent wrote the patch.

A green test suite can coexist with all of those problems. Tests prove selected
behavior. They rarely prove that the next change will stay local.

## Why Balanced Coupling

Vlad Khononov's Balanced Coupling model treats coupling as something to design,
not something to eliminate.

Some coupling is cohesion: nearby parts with a shared purpose should move
together. Risk appears when coupling is too strong, too far away, and too volatile
for the relationship it represents.

`archfit` turns that idea into measurable feedback:

- **Strength** — how intrusive the dependency is, from a stable contract to
  direct internal knowledge.
- **Distance** — how far apart the coupled parts are, from the same module to a
  different deploy unit or ownership boundary.
- **Volatility** — how often the target changes and how likely the relationship
  is to force coordinated edits.

That makes architecture feedback more useful than a raw dependency edge.

The question is not only: "is there an import?"

The better question is: "is this relationship balanced, or is it likely to cause
cascading change?"

## What archfit adds

Language-specific tools remain useful. They provide facts about the code.

`archfit` uses those facts as inputs, then evaluates them against architecture
intent and a Balanced-Coupling model.

The output is shaped for both CI and AI-agent repair:

- deterministic gates for boundaries, layers, public APIs, cycles, and thresholds;
- `coupling_balance` band and complementary metrics (`blast_radius`, `cycle`, `encapsulation`, `coverage`);
- coupling advisories that explain strength, distance, and volatility;
- `agent_tasks` that tell an AI agent what to fix, which constraints to preserve,
  and how to prove the repair;
- coverage gaps reported as `n/a`, so missing analyzers do not create false
  confidence.

The goal is a watchdog for architectural fitness: a repeatable signal that runs
while the code is changing, before local shortcuts become the system design.

## Evidence base

Architecture erosion is a known software evolution problem. Surveys and mapping
studies describe it as a source of maintainability loss, complexity growth,
quality degradation, and higher evolution cost:

- [Understanding software architecture erosion: A systematic mapping study](https://onlinelibrary.wiley.com/doi/10.1002/smr.2423)
- [Controlling software architecture erosion: A survey](https://www.sciencedirect.com/science/article/abs/pii/S0164121211002044)
- [Understanding Software Architecture Erosion: A Systematic Mapping Study](https://arxiv.org/abs/2112.10934)
- [Drift and Erosion in Software Architecture: Summary and Prevention Strategies](https://dl.acm.org/doi/10.1145/3404663.3404665)

AI-specific architecture evidence is newer and still developing. Current research
and practitioner reports mostly show the risk indirectly: AI tools increase local
change velocity, work with incomplete project context, and can produce code that
is functionally plausible but weak on maintainability, design, or security. The
Hacker News discussion ["When I reject AI code even if it works"](https://news.ycombinator.com/item?id=48614631)
captures the same practitioner concern: code can pass the narrow task and still
be rejected because it does not fit the system.

That is the gap `archfit` targets.

It does not claim an LLM can judge architecture on its own. It gives humans, CI,
and agents a measurable architecture fitness signal before erosion becomes
normal.
