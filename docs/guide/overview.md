# Overview

`archfit` is a local CLI for architecture fitness checks.

It is built for teams that want a watchdog for architecture drift, especially
when AI coding agents are changing code quickly. It reads code structure from
supported language analyzers, compares the observed dependency graph with
architecture intent from `.archfit.yaml`, and emits findings that humans or AI
agents can repair.

Use it to answer questions like:

- Did this change cross a forbidden boundary?
- Did an inner layer start importing an outer layer?
- Did code bypass a module's public API?
- Did a new import cycle appear?
- Did coupling risk or architecture metric health get worse?

`archfit` is not a formatter, style linter, security scanner, or replacement for
architecture review. It makes selected architecture rules executable and gives AI
agents constructive, measurable repair feedback before local shortcuts become
system design.

It implements Vlad Khononov's Balanced Coupling model. For why architecture
fitness matters in AI-assisted development, see
[Why architecture fitness matters](why-architecture-fitness.md). For the theory
behind the strength / distance / volatility vocabulary, see [Concepts](concepts.md);
for every signal it computes and how each is scored, see the
[Metrics reference](metrics.md).

## What it checks

`archfit` focuses on architecture drift, not general code quality.

It can check:

- forbidden dependencies between paths or modules;
- public API boundaries and internal API access;
- layer direction rules;
- import cycles;
- new cross-module dependencies;
- coupling advisories based on strength, distance, volatility, and explicitness;
- metric deltas such as encapsulation, unbalanced edges, cycles, and coverage.
