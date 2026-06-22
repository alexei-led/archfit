# Concepts: Balanced Coupling and modularity

This page is the theory behind `archfit`: what it measures, why those things
matter, and the model it makes executable. The other guide pages tell you how to
run the tool; this one tells you what the output means.

`archfit` is a deterministic implementation of Vlad Khononov's **Balanced
Coupling** model. It does not invent its own architecture theory. It turns the
parts of that model that can be read from code, config, and git history into
checks an agent or CI can run on every change.

- Book: Vlad Khononov, [_Balancing Coupling in Software Design_][bc-book],
  Addison-Wesley Signature Series, 2024 — the primary source for strength,
  distance, volatility, balance, and rebalancing.
- Book: Vlad Khononov, [_Learning Domain-Driven Design_][lddd-book], O'Reilly,
  2021 — the source for subdomains (core / supporting / generic) and volatility.
- Site: <https://coupling.dev> — the companion concept index for the book.
  Specific pages are linked inline below and collected under [References](#references).

---

## Why modularity, not "low coupling"

Khononov frames modularity as **the opposite of complexity**: a design property
that lets you make a change with predictable, low effort. Complexity is what you
feel when a small change forces large, surprising, cascading edits.

Coupling is at the heart of modularity, but coupling is **not the enemy**.
Coupling is what makes a system a system instead of a pile of unrelated parts.
"Decouple everything" is bad advice: it trades local cohesion for distance and
indirection, and often makes change harder, not easier.

The useful question is not _how much_ coupling exists but whether each coupling
is **balanced** — whether the strength of the connection is justified by how
close the two parts are and how often they change. `archfit` is built to answer
that question with evidence instead of opinion.

> Modularity is "a design principle that enables predictable, low-effort change",
> and coupling is at its heart. — <https://coupling.dev/posts/core-concepts/>

---

## The three dimensions

Balanced Coupling describes every cross-boundary relationship along three
dimensions. `archfit` classifies each dependency edge on all three, plus a fourth
lens (explicitness) it uses for severity.

### 1. Integration strength — how much knowledge crosses the boundary

Strength is the amount of shared knowledge between two components. The more one
component must know about another's internals, the stronger — and more
change-propagating — the coupling. Four levels, strongest to weakest
(<https://coupling.dev/posts/dimensions-of-coupling/integration-strength/>):

| Level        | Meaning                                                                                           | `archfit` signal                                                                      |
| ------------ | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `intrusive`  | Depends on private interfaces / implementation details not meant to be shared.                    | `internal:` globs, Go `internal/`, `_private.py`, SCIP "private" symbol kind.         |
| `functional` | Shares knowledge of business requirements; the two must change together when requirements change. | Duplicated logic (clone detector), config-declared, or SCIP function-level reference. |
| `model`      | Shares a domain model / schema that must be updated in both when the model changes.               | Shared exported type, SCIP concrete-class symbol kind.                                |
| `contract`   | Integrates through an explicit, intention-revealing contract that hides implementation.           | `public:` globs, SCIP Protocol/ABC/interface symbol kind.                             |

`contract` and `intrusive` are decided deterministically from config globs and
visibility. `model` and `functional` are weaker signals — inferred from symbol
kinds (SCIP) or duplication, and refined under human review by `archfit enrich`
(see [LLM enrichment](llm-enrich.md)). When nothing classifies an edge, its
strength is `unknown` (absence of evidence, never counted as a leak).

`archfit` treats `functional` and `intrusive` as **high** strength and
`contract` and `model` as **low** strength when it applies the balance rule.

### 2. Distance — how expensive it is to change them together

Distance is the physical and organizational separation of the two components'
source code. The farther apart, the more cognitive and coordination effort a
joint change costs (different files < different packages < different services <
different teams < different deploy units).

> It is significantly easier to co-evolve two components in the same file than
> two objects in different microservices.
> — <https://coupling.dev/posts/dimensions-of-coupling/distance/>

`archfit` levels, nearest to farthest:

| Level                          | Derived from                                                         |
| ------------------------------ | -------------------------------------------------------------------- |
| `same_module`                  | Both endpoints map to the same module.                               |
| `cross_module_same_owner`      | Same `owner`, or sibling/parent-child packages when no owner is set. |
| `cross_module_different_owner` | Different `owner`, or unrelated/flat packages by code structure.     |
| `cross_deploy_unit`            | Both endpoints set `deploy_unit` and they differ.                    |

`owner` and `deploy_unit` are config fields precisely because they change
distance: the same import can be cheap (same team, same service) or expensive
(two teams, two deploy units). `archfit` treats
`cross_module_different_owner` and `cross_deploy_unit` as **high** distance.

Distance is a composite of three signals. A deploy boundary is absolute. When
multiple distinct owners exist, ownership overrides code structure. In repos with
a single maintainer or one team everywhere, ownership is **neutral** — it does not
collapse far-apart modules to "same owner = low risk". Code structure (package-tree
position) is the always-available baseline and distinguishes close from far modules
regardless of owner count. See the
[configuration reference](configuration-reference.md) for the exact composite order.

### 3. Volatility — how likely it is to change at all

Volatility is the probability that a component needs to change. A strong, distant
coupling that never changes costs little; the same coupling in code that changes
weekly is a recurring tax.

> The higher the volatility, the more acute and "painful" design issues will be.
> — <https://coupling.dev/posts/dimensions-of-coupling/volatility/>

Volatility comes primarily from **DDD subdomains**, not from the codebase itself:

| Subdomain    | Volatility | Why                                                   |
| ------------ | ---------- | ----------------------------------------------------- |
| `core`       | high       | Competitive advantage; continuously optimized.        |
| `supporting` | medium     | Custom but not differentiating; changes occasionally. |
| `generic`    | low        | Solved problem / off-the-shelf; rarely changes.       |

In `archfit` you set volatility per module (`volatility:` or `subdomain:` in
`.archfit.yaml`). Git churn can fill in a value only for modules with no declared
volatility, and never overrides a human one.

#### Essential vs accidental volatility

Khononov splits volatility in two, and the distinction is why `archfit` does not
trust churn:

- **Essential volatility** comes from the business domain. A core subdomain is
  volatile because the business keeps improving it. This is the signal you want.
- **Accidental volatility** comes from poor design. Badly balanced coupling makes
  unrelated code change together, so files _look_ volatile when the domain is not.
  The inverse also happens: code looks stable only because it is too risky to
  touch.

Git churn measures observed change — a mix of both. So `archfit` uses
human-declared subdomain volatility as the primary input and treats churn as
supporting evidence only. Churn-derived volatility feeds the report-only
`change_amplification` metric (which is _about_ accidental volatility) but is
deliberately kept out of the `risk_hub` metric, which uses declared volatility
only.

### Explicitness — the fourth lens

`archfit` also tags each edge `explicit` or `implicit`. A contract is explicit
(visible, intentional, versionable); reaching into internals is implicit
(fragile, invisible, easy to break). Explicitness is derived from strength
(`contract` → explicit, `intrusive` → implicit) or an extractor AST hint. It
sharpens severity and points the agent at the safer integration path.

---

## The balance rule

The model's core claim: the pain of a coupling is driven by all three dimensions
together, not by coupling alone.

```text
strength  → how likely a change is to propagate across the boundary
distance  → how expensive each propagated change is to implement
volatility → how often you actually pay that cost
```

A design is **balanced** when strength and distance counterbalance: strong
coupling is fine when distance is low (high cohesion inside a module); high
distance is fine when strength is low (a thin contract between services).
The dangerous combination is **high strength + high distance**, and **high
volatility** is what turns that imbalance from theoretical into painful.

`archfit` implements this as a severity table over every cross-boundary edge
(`internal/model/coupling/coupling.go`, `BalanceResult`):

| Strength                          | Distance                     | Volatility  | Severity                                |
| --------------------------------- | ---------------------------- | ----------- | --------------------------------------- |
| intrusive                         | cross_deploy_unit            | any         | `critical`                              |
| intrusive                         | cross_module_different_owner | high        | `high`                                  |
| intrusive                         | cross_module_different_owner | other       | `medium`                                |
| high (functional/intrusive)       | high                         | high        | `critical`                              |
| high                              | high                         | other       | `medium`                                |
| low (contract/model)              | low                          | high        | `medium` (over-decoupled volatile seam) |
| low                               | low                          | low/unknown | none (balanced)                         |
| asymmetric (high+low or low+high) | —                            | —           | `low`                                   |

Two things worth noting:

- **Intrusive coupling is always surfaced.** Reaching into internals is the one
  case the model treats as a leak regardless of the rest, escalating with
  distance and volatility.
- **Over-decoupling is also a finding.** A thin contract across a tiny distance
  that nonetheless changes constantly (`low strength + low distance + high
volatility`) is flagged `medium`: the seam is more ceremony than the
  relationship warrants. The model is about balance in both directions, not
  minimization.

These severities drive the `bc/imbalanced_coupling` advisories (see
[`archfit scan`](commands.md)) and the `unbalanced_edge` metric.

---

## How `archfit` operationalizes the model

The model is a review method for humans. `archfit` keeps human judgment in charge
and makes only the legible parts executable. Three design rules follow from that:

1. **Two channels, never blended.** A deterministic **gate** (pass/fail) enforces
   explicit rules you declared — forbidden dependencies, public-API-only, layer
   direction, cycles. A **metric** channel reports legible deltas (encapsulation,
   unbalanced edges, …) that _warn_ on regression but do not fail the build by
   default. Classification language explains findings; it is never a single
   blended "architecture score". See [Metrics reference](metrics.md).

2. **Honest about evidence.** Every metric reports its coverage and confidence.
   Low confidence caps the band it can claim. Absent signal is reported as `n/a`,
   never as a bad score. A missing optional tool lowers confidence; it never
   produces a false failure.

3. **LLM is off the gate.** Strength inference and explanations can use an LLM
   (`archfit enrich`, `archfit explain --llm`), but gate verdicts and metric
   values are computed by deterministic code only. The model finds candidates;
   humans pin labels; the gate stays reproducible. See
   [LLM enrichment](llm-enrich.md).

The result is the loop tests and linters already taught agents to run: change
code → `archfit check` → get a deterministic finding or metric delta with the
strength / distance / volatility / explicitness vocabulary attached → repair
within the stated constraint → rerun.

---

## References

Methodology:

- Vlad Khononov, [_Balancing Coupling in Software Design_][bc-book],
  Addison-Wesley, 2024.
- Vlad Khononov, [_Learning Domain-Driven Design_][lddd-book], O'Reilly, 2021.

Concept pages (companion site to the book):

- Core concepts — <https://coupling.dev/posts/core-concepts/>
- Integration strength — <https://coupling.dev/posts/dimensions-of-coupling/integration-strength/>
- Distance — <https://coupling.dev/posts/dimensions-of-coupling/distance/>
- Volatility — <https://coupling.dev/posts/dimensions-of-coupling/volatility/>
- Connascence — <https://coupling.dev/posts/related-topics/connascence/>
- Afferent and efferent coupling — <https://coupling.dev/posts/related-topics/afferent-and-efferent-coupling/>

See the [Metrics reference](metrics.md) for how each of these concepts becomes a
measured signal, and the build spec (`docs/spec/arch-fitness-spec-v0.4.md`) for
the full design rationale.

[bc-book]: https://www.informit.com/store/balancing-coupling-in-software-design-universal-design-9780137353484
[lddd-book]: https://www.oreilly.com/library/view/learning-domain-driven-design/9781098100124/
