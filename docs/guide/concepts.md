# Concepts: Balanced Coupling and modularity

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

| Level        | Ordinal | Meaning                                                                                           | `archfit` signal                                                              |
| ------------ | ------- | ------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| `intrusive`  | 10      | Depends on private interfaces / implementation details not meant to be shared.                    | `internal:` globs, Go `internal/`, `_private.py`, SCIP "private" symbol kind. |
| `symmetric`  | 9       | Duplicated functionality — both sides must change together (DRY violation across a boundary).     | Cross-module clone pair detected by the clone detector (`analyzers.clones`).  |
| `functional` | 8       | Shares knowledge of business requirements; the two must change together when requirements change. | Config-declared or SCIP function/method-level reference.                      |
| `model`      | 3       | Shares a domain model / schema that must be updated in both when the model changes.               | Shared exported type, SCIP concrete-class symbol kind.                        |
| `contract`   | 1       | Integrates through an explicit, intention-revealing contract that hides implementation.           | `public:` globs, SCIP Protocol/ABC/interface symbol kind.                     |

`contract` and `intrusive` are decided deterministically from config globs and
visibility. `symmetric` is assigned when clone detection finds duplicated logic
crossing a module boundary. `model` and `functional` are inferred from symbol
kinds (SCIP) or config-declared, and refined under human review by
`archfit config enrich` (see [LLM enrichment](llm-enrich.md)). When nothing classifies
an edge, its strength is `unknown` — the edge is **abstained** (excluded from
scoring), never assigned an invented ordinal.

The ordinals are the book's published values and drive the balance formula
directly.

#### Connascence evidence — what shared knowledge was seen

Connascence (book Ch6) is a lower-level vocabulary for the same shared knowledge
that drives integration strength. `archfit` now reports deterministic static
connascence as evidence, not as another score:

| Connascence kind | Deterministic sources today                                                        |
| ---------------- | ---------------------------------------------------------------------------------- |
| Name             | static imports/references from Go, TypeScript, Python, and SCIP                    |
| Type             | Go type references, TypeScript `import type`, SCIP type/interface/protocol symbols |
| Meaning          | Go const/var/data references, Rust SCIP const/static/field terms                   |
| Algorithm        | Go function/method/callable references, SCIP function/method symbols               |
| Position         | unmeasured unless a future deterministic source proves argument/order coupling     |

Dynamic connascence categories — execution, timing, runtime value, and identity —
are not guessed. They appear in JSON/Markdown `connascence.unmeasured`, while
runtime async and dynamic-import detectors stay separate report-only signals.
This keeps the deterministic gate LLM-free and prevents semantic naming guesses
from becoming score inputs.

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

**Owner resolution is single-dominant-owner per module** (the value declared in
config, or the most-frequent owner from CODEOWNERS / git-author history). Two
consequences to know when reading `cross_module_different_owner`:

- A CODEOWNERS that lists a **common team first on most lines** (e.g. a
  `* @org/maintainers` catch-all, or a maintainers team co-listed on every rule)
  resolves nearly every module to that one owner, so cross-team distance
  under-reports. archfit keeps the first owner per line, not the owner set.
- A CODEOWNERS that **does not cover the analyzed source** (only `/docs`, or the
  backend package has no rule) resolves to a single or empty owner → owner
  distance is neutral and the score falls back to code-structure distance.

Both are faithful reflections of the repo's declared ownership, not bugs — but if
you want cross-team coupling to register, declare `owner:` per module explicitly.
The `owner_source` field (`config` | `codeowners` | `git` | `git_timeout` |
`codeowners_no_match` | `none`) in JSON and the markdown "Distance confidence"
section tells you which path produced the owners. The two degraded sources — a
CODEOWNERS file that matched none of the configured modules, or a git-author
history walk that timed out — also emit a stderr warning instead of silently
falling back to code-structure distance.

**Deviation from the book:** Khononov also counts _runtime coupling_ (synchronous
vs asynchronous integration) and lifecycle coupling as part of distance. `archfit`
deliberately does **not** fold runtime/async coupling into distance — detected
async bridges are recorded as report-only `runtime_async` evidence, never a scored
distance factor (see [bc-measurement-v4.md §9](../design/bc-measurement-v4.md#9-non-goals-and-rejected-designs) for the rationale).

### 3. Volatility — how likely it is to change at all

Volatility is the probability that a component needs to change. A strong, distant
coupling that never changes costs little; the same coupling in code that changes
weekly is a recurring tax.

> The higher the volatility, the more acute and "painful" design issues will be.
> — <https://coupling.dev/posts/dimensions-of-coupling/volatility/>

Volatility comes primarily from **DDD subdomains**, not from the codebase itself.
Declaring `subdomain:` on a module maps to a book-anchored volatility:

| Subdomain    | Book anchor | Ordinal (V) | Why                                                   |
| ------------ | ----------- | ----------- | ----------------------------------------------------- |
| `core`       | core        | 10          | Competitive advantage; continuously optimized.        |
| `supporting` | supporting  | 3           | Custom but not differentiating; changes occasionally. |
| `generic`    | generic     | 3           | Solved problem / off-the-shelf; rarely changes.       |

You can also set `volatility:` directly. The book (Ch10) defines only three
numeric anchors — 1, 3, 10 — so `medium` (V=6) is an **archfit interpolation**
with no book anchor:

| `volatility:`       | Ordinal (V) | Book anchor                                |
| ------------------- | ----------- | ------------------------------------------ |
| `high`              | 10          | core subdomain                             |
| `medium`            | 6           | — (archfit interpolation; not in the book) |
| `low`               | 3           | supporting / generic subdomain             |
| `frozen` / `legacy` | 1           | legacy system that is not being evolved    |

A module that resolves but declares neither `volatility:` nor `subdomain:` is
treated as **undeclared → V=10** (no path/name guessing) — a conservative
worst case that is also archfit-defined, not a book ordinal. The scorer then
advises you to _declare_ the module's volatility rather than silently assuming it
is stable.

In `archfit` you set volatility per module (`volatility:` or `subdomain:` in
`.archfit.yaml`). Git churn is never used as a volatility source — it measures
observed change, a mix of essential and accidental factors, and `archfit` cannot
separate them automatically. Declared subdomain volatility is the only input to
the coupling-balance gate.

**Inferred-volatility cascade (opt-in, book Ch9):** when
`coupling.volatility_cascade: true` is set in `.archfit.yaml`, a single-hop
propagation pass runs before scoring. If a module is strongly coupled
(`functional` or `intrusive`) to a `core` module, its effective volatility
is raised to `high` for scoring purposes. This lets archfit surface coupling
chains that inherit core-domain volatility without requiring every module to be
manually annotated.

#### Essential vs accidental volatility

Khononov splits volatility in two, and the distinction is why `archfit` does not
use churn:

- **Essential volatility** comes from the business domain. A core subdomain is
  volatile because the business keeps improving it. This is the signal you want.
- **Accidental volatility** comes from poor design. Badly balanced coupling makes
  unrelated code change together, so files _look_ volatile when the domain is not.
  The inverse also happens: code looks stable only because it is too risky to
  touch.

Git churn measures observed change — a mix of both. `archfit` uses human-declared
subdomain volatility and does not infer volatility from churn.

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

`archfit` implements Khononov's published per-edge formula (Ch10) verbatim:

```
modularity = |S − D|                    // strength and distance ordinals
balance    = max(modularity, 10 − V) + 1  // 1 (critical) … 10 (perfectly balanced)
```

Ordinal anchors (book-exact):

| Dimension  | Level                          | Ordinal |
| ---------- | ------------------------------ | ------- |
| Strength   | Contract                       | 1       |
| Strength   | Model                          | 3       |
| Strength   | Functional                     | 8       |
| Strength   | Symmetric (clone-detected DRY) | 9       |
| Strength   | Intrusive                      | 10      |
| Distance   | `same_module`                  | 2       |
| Distance   | `cross_module_same_owner`      | 4       |
| Distance   | `cross_module_different_owner` | 7       |
| Distance   | `cross_deploy_unit`            | 9       |
| Volatility | `supporting` / `generic`       | 3       |
| Volatility | `core`                         | 10      |

Balance maps to severity bands: 1–2 → `critical`, 3–4 → `high`,
5–6 → `medium`, 7–8 → `low`, 9–10 → `none`.

Three things worth noting:

- **The distributed monolith** (S=Intrusive/Symmetric, D=cross_deploy_unit,
  V=core): `max(|10−9|, 10−10) + 1 = 1` → `critical`. The worst pattern
  scores 1 exactly as the book says.
- **Asymmetric (modular) cases score well.** A Contract edge across a deploy
  boundary (S=1, D=9, V=10): `max(8, 0) + 1 = 9` → `none`. Low strength
  over high distance is balanced — the formula rewards it.
- **Over-decoupling is also visible.** Contract coupling across a tiny
  same-module distance with high volatility (S=1, D=2, V=10):
  `max(1, 0) + 1 = 2` → `critical`. The seam is more ceremony than the
  relationship warrants.

When strength or distance cannot be classified (`unknown`), the edge is
**abstained** — excluded from scoring rather than assigned an invented ordinal.
Abstained internal edges lower `coupling_balance` confidence and emit decision
tasks. See the [configuration reference](configuration-reference.md) for the
abstain rule and decision-task behavior.

These balance scores drive the `bc/imbalanced_coupling` advisories (see
[`archfit analyze`](commands.md)) and the `unbalanced_edge` metric. Cross-module
clone pairs with no import edge are clone-only duplicated knowledge; by default
(`coupling.duplicated_knowledge: score`) they enter `coupling_balance` as
symmetric-strength coupling facts and may also surface as `bc/duplicated_knowledge`
advisories after severity filtering. Set the policy to `advisory` to preserve
the v4 report-only behavior. `ScoreVersion` is `bc_score.v5`.

---

## How `archfit` operationalizes the model

The model is a review method for humans. `archfit` keeps human judgment in charge
and makes only the legible parts executable. Three design rules follow from that:

1. **Two channels, never blended.** A deterministic **gate** (pass/fail) enforces
   explicit rules you declared — forbidden dependencies, public-API-only, layer
   direction, cycles. A **metric** channel tracks legible deltas (encapsulation,
   unbalanced edges, …) whose direction-aware regressions trip a per-metric gate:
   `metrics.<name>.gate` unset blocks, `warn` caps at WARN, `off` skips, with
   `max_new`/`min_delta` thresholds for tolerated movement. Classification
   language explains findings; it is never a single blended "architecture
   score". See [Metrics reference](metrics.md).

2. **Honest about evidence.** Every metric reports its coverage and confidence.
   Low confidence caps the band it can claim. Absent signal is reported as `n/a`,
   never as a bad score. A missing optional tool lowers confidence; it never
   produces a false failure.

3. **LLM is off the gate.** Strength inference and explanations can use an LLM
   (`archfit config enrich`, `archfit explain --llm`), but gate verdicts and metric
   values are computed by deterministic code only. The model finds candidates;
   humans pin labels; the gate stays reproducible. See
   [LLM enrichment](llm-enrich.md).

The workflow: change code → `archfit analyze --gate` → deterministic finding or
metric delta with strength / distance / volatility vocabulary → repair within the
stated constraint → rerun.

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
