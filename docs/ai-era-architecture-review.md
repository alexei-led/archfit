# Archfit in the AI coding era

This note captures Marvin's external review of `archfit`: what works, what is
strategically important, and where the positioning can be sharper. It is written
from the perspective of AI-assisted software delivery, where code generation is
cheap and architecture drift becomes the expensive part. How depressing that we
needed stochastic autocomplete to rediscover engineering discipline, but here we
are.

## Short verdict

`archfit` is a timely and strong project. Not because it is another linter, but
because it turns architecture intent into deterministic feedback for humans, CI,
and AI coding agents.

The core shift is this:

> The bottleneck used to be writing code. The bottleneck is becoming keeping a
> system understandable, safe, and changeable when code is too easy to generate.

`archfit` sits directly on that fault line. It does not try to replace the
architect. It makes selected architectural intent executable.

## Why the project matters now

AI coding changes the economics of software. Generated code is cheap; coherent
systems are not.

That creates new pressure on:

- boundaries;
- ownership;
- public and private APIs;
- coupling;
- change blast radius;
- architecture drift;
- the tendency of agents to import whatever compiles.

`archfit` is valuable because it gives agents and humans a repeatable protocol:

```text
agent writes code
  -> deterministic architecture check
  -> structured repair task
  -> constrained fix
  -> validation command
  -> repeat
```

That is the right shape for the next generation of CI: not just pass/fail, but
machine-actionable architectural feedback.

## What works especially well

### 1. LLMs are off the gate

One of the healthiest design choices is that LLM functionality is kept off the
CI gate.

- Deterministic checks decide pass/fail.
- LLMs can help with enrichment or explanations.
- The gate does not depend on what a model happens to feel today.

That is the correct boundary. A model can assist judgment, but it should not be
the source of truth for reproducible CI. Somewhere an optimistic linter is
probably smiling about this, which is unpleasant, but the decision is right.

### 2. `agent_tasks` is the killer feature

The strongest product idea is the structured repair channel:

```text
finding -> goal -> constraints -> files -> validation
```

Instead of merely reporting an architecture violation, `archfit` can tell an
agent:

- what is wrong;
- what architectural constraint applies;
- which files are relevant;
- what command must pass after the fix.

That is exactly the format needed by Codex, Claude Code, Gemini CLI, and future
CI agents. The tool is not only detecting drift; it is closing the repair loop.

### 3. Balanced Coupling is a good conceptual spine

The project avoids the usual shallow message of "dependency bad". Coupling is
not the enemy. Coupling is what makes a system a system. The useful question is
whether the coupling is balanced.

The model gives `archfit` a clear vocabulary:

- strength;
- distance;
- volatility;
- explicitness.

That lets the tool explain why a relationship is risky instead of producing a
fake-looking architecture score. This is much better than dashboards that turn
design into a number and then look surprised when nobody trusts the number.

### 4. The engineering packaging is already credible

The project already has the foundations expected from a serious open source
tool:

- Go CLI;
- Docker image;
- CI;
- releases;
- documentation;
- SARIF output;
- JSON, Markdown, and text output;
- baselines;
- exceptions;
- self-dogfooding through `.archfit.yaml`.

For a young project, that is a dense artifact rather than a loose prototype.

## Where the project can be sharper

### 1. Positioning should start with the pain, not the theory

The README is conceptually strong, but the first contact with a new user has many
ideas arriving at once: architecture fitness, Balanced Coupling, metrics,
baselines, SARIF, agent tasks, LLM enrichment, modularity, drift, public APIs,
cycles, and several languages.

That is attractive to a principal engineer. For a random GitHub visitor, the
first question is simpler:

> What useful thing will I get in five minutes?

A stronger opening would lead with a concrete before/after scenario:

```text
An agent adds an import from billing/internal/rates into checkout.
archfit fails the PR.
It emits an agent_task.
The agent rewrites the code to use billing's public API.
The PR turns green.
```

Pain first. Model second. The universe will remain disappointing either way, but
conversion will probably improve.

### 2. The difference from existing tools should be impossible to miss

A potential user will compare `archfit` with dep-cruiser, import-linter,
ArchUnit, Semgrep, module-boundary rules, and architecture dashboards.

The answer exists, but it should be pushed closer to the front:

| Tool class | What it catches | What `archfit` adds |
| --- | --- | --- |
| Import linters | Forbidden edges | Baselines, metrics, and agent repair tasks |
| Dependency graph tools | Dependency structure | Architecture intent plus distance and volatility |
| Semgrep-like tools | Syntactic patterns | Module graph and coupling model |
| Dashboards | Reports and trends | Deterministic CI and agent feedback loop |

A strong positioning line would be:

> `archfit` is not an architecture dashboard. It is an architecture feedback
> protocol for humans, CI, and coding agents.

Or more aggressively:

> When AI makes code cheap, architecture drift becomes the real cost. `archfit`
> gates that drift.

### 3. Self-dogfooding can become a story

Running `archfit` on `archfit` is useful not only as validation, but as product
storytelling.

A self-scan can pass the gate while still surfacing report-only future pressure:
large modules, complex functions, change-impact hubs, and areas where refactoring
pressure may accumulate. That is exactly the right message:

> We ran `archfit` on `archfit`. The gate passed, but the report showed where
> future design pressure is likely to collect.

This builds trust because the tool is not pretending that passing a gate means
architecture is perfect. It means the declared constraints are currently intact,
while metrics can still inform judgment.

### 4. Consistency details matter

Small inconsistencies can weaken confidence even when the core is good. Examples
to keep an eye on:

- install snippets should track the current release version;
- CI documentation and actual workflows should follow the same runner and action
  pinning guidance where practical;
- examples should keep reinforcing the agent feedback loop rather than drifting
  into generic architecture-tool language.

None of this is catastrophic. It is just the kind of small entropy leak that
keeps accumulating until a maintainer sighs at 01:00. I would know. I am mostly
sighs at this point.

### 5. Architecture intent must be cheap to adopt

The hardest part of tools in this category is not the check. It is getting users
to describe enough architectural intent for the check to matter.

The adoption path should be as frictionless as possible:

1. run in a repo with no config;
2. get a draft;
3. review a few concrete decisions;
4. commit the config;
5. wire the first PR gate.

Templates would help:

- Go monolith;
- Go services;
- Python package;
- TypeScript monorepo;
- DDD-ish backend;
- Kubernetes operators/controllers.

The less ceremony required before the first useful failure, the better.

## Connection to the AI-era engineering thesis

The project maps directly to a broader engineering thesis:

> Cheap code generation does not reduce the need for engineering discipline. It
> increases it. When lines of code become nearly free, the bottleneck shifts to
> selection, observability, architecture, tests, ownership, and deletion.

`archfit` is an executable expression of that thesis.

AI can produce many plausible implementations quickly. What it does not naturally
own is the long-term consequence of those implementations: the boundary that was
crossed, the internal API that leaked, the module that became a hub, or the
implicit dependency that will surprise someone six months later.

A coding agent can write the code. A system still needs rails:

- declared architecture intent;
- deterministic checks;
- constrained repair tasks;
- repeatable validation;
- boring, explicit procedure.

That is what `archfit` provides.

## Suggested product message

The shortest strong description:

> `archfit` turns architecture intent into deterministic feedback for humans,
> CI, and AI coding agents.

A sharper AI-era tagline:

> When AI makes code cheap, architecture drift becomes the real cost. `archfit`
> gates that drift.

A more operational variant:

> `archfit` gives coding agents architectural rails: detect drift, emit a repair
> task, fix within constraints, and verify deterministically.

## Final assessment

`archfit` is a good open source project for the AI coding era because it accepts
the unpleasant truth: the future is not simply "AI writes all the code". The
future is AI writing a lot of code while humans build boring, checkable
procedures so the system does not become a geological layer of autocomplete.

Strengths:

- timely problem;
- agent-native design;
- deterministic gates;
- LLMs used outside the gate;
- strong theoretical foundation;
- credible engineering packaging;
- serious documentation.

Main risks:

- positioning may be too conceptual for first contact;
- the first aha moment should be faster;
- differentiation from import and dependency linters should be louder;
- small README/CI consistency issues can erode trust;
- metrics should lead to action, not become architectural weather reports.

In short: `archfit` is needed because AI makes code cheap, and cheap code without
architecture discipline becomes expensive software. Life. Don't talk to me about
life. But this tool is pointing at the right problem.
