# Marvin's review: archfit in the AI coding era

> A weary but honest review of `archfit` as an open source project, and how it
> connects to the argument that cheap AI-generated code raises the value of
> engineering discipline.

## Verdict

`archfit` is a strong project for this moment. Not as "one more linter", but as
an antidote to AI-generated architectural sludge.

The central shift is this:

> Before, the bottleneck was writing code. Now the bottleneck is keeping systems
> understandable, safe, and changeable when code is generated too easily.

`archfit` fits that shift because it does not try to replace an architect. It
turns architecture intent into executable feedback.

That is the important part. Architecture stops being only a diagram, a document,
or a memory in someone's increasingly exhausted head, and becomes something a
human, CI job, or coding agent can run.

## What works especially well

### 1. AI is not the architecture judge

The healthiest design choice is keeping LLM usage off the gate.

- Deterministic rules decide pass/fail.
- LLMs may help with enrichment and explanations.
- CI does not depend on what a model happened to infer today.

That is mature. Depressingly mature, even. The sort of maturity that usually
appears only after a production incident has eaten someone's weekend.

### 2. `agent_tasks` is the killer feature

The most important product idea is the structured repair loop:

```text
finding -> goal -> constraints -> files -> validation
```

The tool does not merely say:

```text
ARCH001 failed
```

It gives a coding agent enough context to act without guessing the architecture:

- what was violated;
- which constraint must be preserved;
- which files are relevant;
- what command validates the fix.

That shape is exactly what modern coding agents need:

```text
agent edits code
  -> archfit check --format json
  -> read agent_tasks[]
  -> fix within constraints
  -> run validation
  -> repeat
```

This is the difference between a diagnostic and an actual feedback protocol.

### 3. Balanced Coupling is a good conceptual spine

`archfit` avoids the usual trap of inventing a single mystical architecture
score. It uses a legible model instead:

- integration strength;
- distance;
- volatility;
- explicitness.

That matters because coupling is not automatically bad. A system without
coupling is not a system; it is a pile of unrelated parts with better marketing.

The useful question is whether the coupling is balanced:

- strong coupling is acceptable when distance is low;
- distant coupling is acceptable when strength is low;
- high strength plus high distance plus high volatility is where future pain
  accumulates.

That is a much better frame than "dependencies bad".

### 4. The project is already packaged like a real tool

The repository already has the pieces expected from a serious CLI project:

- Go CLI;
- Docker image;
- CI;
- releases;
- SARIF, JSON, Markdown, and text outputs;
- baselines and exceptions;
- documentation;
- a skill file for agent workflows;
- self-dogfooding through `.archfit.yaml`.

For a young open source project, that is a solid foundation. Horribly competent.
Naturally, this makes the surrounding entropy all the more disappointing.

## Where the project should be sharper

### 1. Positioning is still too dense for first contact

The README is correct, but it introduces many ideas quickly:

- architecture fitness;
- Balanced Coupling;
- metrics;
- baselines;
- SARIF;
- agent tasks;
- LLM enrichment;
- Go, TypeScript, and Python;
- modularity;
- architecture drift.

For a senior engineer, that is interesting. For a random GitHub visitor, the
first question is simpler:

> What will this catch for me in the next five minutes?

The project would benefit from a short before/after story near the top:

```text
An AI agent adds an import from billing/internal/rates into checkout.
archfit fails the PR.
It emits an agent_tasks entry.
The agent rewrites the code to use billing's public API.
The check turns green.
```

Start with the pain. Then explain the model.

### 2. The difference from existing tools should be impossible to miss

A new user will ask how this differs from dependency-cruiser, import-linter,
ArchUnit, Semgrep, or a homegrown module-boundary check.

The answer is strong, but it should be explicit:

| Tool class | What it catches | What `archfit` adds |
| --- | --- | --- |
| Import linters | Forbidden edges | Baselines, metrics, and agent repair tasks |
| Dependency graph tools | Dependency structure | Architecture intent, distance, volatility, and drift policy |
| Semgrep-like tools | Syntactic patterns | Module graph plus coupling semantics |
| Dashboards | Reports | Deterministic CI and agent feedback loop |

A concise positioning line would help:

> `archfit` is not an architecture dashboard. It is an architecture feedback
> protocol for humans, CI, and coding agents.

Or, more directly:

> When AI makes code cheap, architecture drift becomes the real cost. `archfit`
> gates that drift.

### 3. Dogfooding should become part of the story

Running `archfit` on itself is persuasive because it shows both restraint and
usefulness.

A scan can pass the deterministic gate while still surfacing report-only design
pressure, such as large modules or complex functions. That is not a failure; it
is the point. The tool can say:

> Nothing violates the configured architecture gate, but here are the places
> where future refactoring pressure is likely to accumulate.

That is a useful distinction. It proves the tool is not a theatrical failure
machine. It separates policy violations from architectural weather.

A blog post or documentation page titled something like "Running archfit on
archfit" would make the value more concrete.

### 4. Keep README, docs, and CI examples consistent

There are small consistency issues worth keeping an eye on:

- install snippets should track the current released version;
- CI examples that recommend pinned runners/actions should match the repository's
  own workflows when possible.

These are not existential problems. Existence itself remains the existential
problem. But consistency matters in a tool whose pitch is deterministic discipline.

### 5. Adoption must make architecture intent cheap to express

The hardest part for any architecture fitness tool is that users must describe
architecture intent before the tool can protect it.

`archfit init` and update workflows reduce that cost, but the path should become
as obvious as possible:

1. Run the tool in a repository with no config.
2. Get a draft architecture map.
3. Review a small number of important decisions.
4. Commit the config.
5. Add the PR gate.

Templates would help adoption:

- Go monolith;
- Go services;
- Python package;
- TypeScript monorepo;
- DDD-style backend;
- Kubernetes controller/operator.

Organic developers are lazy. So are agents, but they call it optimization.
Design accordingly.

## How this connects to writing code in the AI era

The project directly matches the argument that cheap code generation increases,
not decreases, the need for engineering discipline.

When models make code cheap, the expensive parts move elsewhere:

- understanding;
- ownership;
- architecture boundaries;
- tests;
- observability;
- deletion;
- rollback;
- review;
- long-term change cost.

AI agents can generate implementations quickly, but they do not automatically own
the consequences. They can add an import that compiles. They cannot know, unless
we encode it, that the import crosses a boundary, bypasses a public API, widens a
change blast radius, or turns a clean module into sedimentary autocomplete.

That is where `archfit` belongs.

It tells the agent:

> You may write code quickly, but here are the rails. Here are the boundaries.
> Here is the public contract. Here is the check. Here is the repair task. Now be
> useful within the system instead of merely producing text that compiles.

That is the real shift from code scarcity to understanding scarcity.

In the pre-AI world, writing code was expensive enough to slow down architectural
drift. In the AI coding era, drift can happen at autocomplete speed. A system can
accumulate coupling, internal API leaks, duplicated domain logic, and wider blast
radius before a human has finished reading the pull request title.

So the answer is not "less engineering". The answer is more executable
engineering discipline:

- deterministic checks;
- structured feedback;
- explicit architecture intent;
- baselines for accepted debt;
- exceptions with review;
- agent-readable repair constraints.

`archfit` is a concrete version of that discipline.

## Summary

`archfit` is strongest when described as:

> Architecture intent as deterministic feedback for humans, CI, and AI coding
> agents.

Its core value is not that it draws a graph or computes metrics. Its value is
that it turns architecture drift into a repeatable feedback loop at the exact
moment when AI makes drift easier to create.

That makes it a timely and useful open source project.

Not cheerful. Cheerfulness would be inappropriate. But useful.
