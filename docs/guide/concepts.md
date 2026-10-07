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
| `intrusive`  | 10      | Depends on private interfaces / implementation details not meant to be shared.                    | `internal:` globs, `_private.py`, SCIP "private" symbol kind. |
| `symmetric`  | 9       | Duplicated functionality — both sides must change together (DRY violation across a boundary).     | Cross-module clone pair detected by the clone detector (`analyzers.clones`).  |
| `functional` | 8       | Shares knowledge of business requirements; the two must change together when requirements change. | A call or reference into a non-public module surface (Go function, concrete-receiver method, func/chan var; SCIP function kind). The same use of a `public:` target is contract. |
| `model`      | 3       | Shares a domain model / schema that must be updated in both when the model changes.               | Data evidence on a `public:` target: a concrete non-DTO type, field, var, const, or a method on a concrete receiver (Go; SCIP concrete data symbol). A DTO is model outside a `public:` target. |
| `contract`   | 1       | Integrates through an explicit, intention-revealing contract that hides implementation.           | A `public:` target; a call to an interface type or interface method; a data-only DTO struct across a `public:` boundary. |

A `public:` target is the integration contract (bc_score.v7). A call through a
public surface, and any call to an interface method, is `contract`. Only data
evidence raises a public target to `model`. An `internal:` target is
`intrusive`. A call into a non-public module is `functional`. Each cross-module
clone pair is its own `symmetric` fact on the seam between its modules; it never
upgrades an import edge. Go reads the object kind from compiler type info, so
SCIP does not override it. For TypeScript, Python and Rust, SCIP symbol kinds
refine the extractor hint. Pinned labels set strength, refined under human
review by
`archfit config enrich` (see [LLM enrichment](llm-enrich.md)). When nothing classifies
an edge, its strength is `unknown` — the edge is **abstained** (excluded from
scoring), never assigned an invented ordinal.

The ordinals are the book's published values and drive the balance formula
directly.

#### Connascence evidence — what shared knowledge was seen

Connascence (book Ch6) is a lower-level vocabulary for the same shared knowledge
that drives integration strength. `archfit` reports deterministic static
connascence as evidence, not as another score. It never sets strength:

| Connascence kind | Status in archfit                                                        |
| ---------------- | ------------------------------------------------------------------------ |
| Name             | deterministic static evidence from imports/references and SCIP           |
| Type             | deterministic static evidence from Go types, TS type imports, and SCIP   |
| Meaning          | deterministic static evidence from Go const/var/data refs and SCIP terms |
| Algorithm        | deterministic static evidence from Go callable refs and SCIP functions   |
| Position         | unmeasured unless deterministic argument/order evidence is supplied      |
| Execution        | unmeasured dynamic category                                              |
| Timing           | unmeasured dynamic category                                              |
| Runtime value    | unmeasured dynamic category                                              |
| Identity         | unmeasured dynamic category                                              |

JSON/Markdown `connascence.roadmap` carries this checklist in machine-readable
form. Dynamic/lazy imports and runtime async bridges stay separate report-only
signals (`dynamic_imports`, `runtime_async_edges`). Excluded external/library
edges can also surface as review-only `distance_config_candidates` when they
look like stable seams worth declaring. None of these signals are connascence
measurements and none feed the deterministic gate. This keeps the gate LLM-free
and prevents semantic naming guesses from becoming score inputs.

### 2. Distance — how expensive it is to change them together

Distance is the physical and organizational separation of the two components'
source code. The farther apart, the more cognitive and coordination effort a
joint change costs (different files < different packages < different services <
different teams < different deploy units).

> It is significantly easier to co-evolve two components in the same file than
> two objects in different microservices.
> — <https://coupling.dev/posts/dimensions-of-coupling/distance/>

`archfit` levels, nearest to farthest. Distance is **level-relative**: any
module boundary is D=9, the book's rung for an in-house service.

| Level                          | D  | Derived from                                                         |
| ------------------------------ | -- | -------------------------------------------------------------------- |
| `same_module`                  | 2  | Both endpoints map to the same module (`local_coupling`, report-only). |
| `cross_module`                 | 9  | Two modules. Same owner (or no owner) and same deploy unit.         |
| `cross_module_different_owner` | 9  | Two modules with different, non-empty `owner` values.                |
| `cross_deploy_unit`            | 9  | Both modules set `deploy_unit` and the values differ.                |
| `declared_external`            | 10 | The target matches an `external_systems:` entry.                     |

The token names the boundary. It never moves the score: the three cross-module
levels all score at D=9. `owner` and `deploy_unit` still matter, because they
decide the token, the `raw_distance.basis` text (`<boundary>@<container>`) and
the wording of the gate reason. Only a deploy-unit boundary is called a
distributed monolith. Module key spelling decides nothing: renaming a module or
nesting it differently leaves every score unchanged.

Nested modules form a **containment tree** from the `paths:` globs. A seam
reports the container it sits in (the last node both sides share), so a seam
inside `sales` reads `module_boundary@sales`, and two modules with no common
ancestor read `@system`. Plain folders add no level.


**Owner resolution** takes one owner per module: the value declared in config, or
the most-frequent owner from CODEOWNERS or git-author history. The owner decides
only the `cross_module_different_owner` token and the seam's raw owner facts. It
never changes a score or makes a seam qualify. The `owner_source` field
(`config` | `codeowners` | `git` | `git_timeout` | `codeowners_no_match` |
`none`) tells you which path produced the owners. The two degraded sources, a
CODEOWNERS file that matched none of the configured modules and a git-author
history walk that timed out, also emit a stderr warning.

**Deviation from the book:** Khononov also counts _runtime coupling_ (synchronous
vs asynchronous integration) and lifecycle coupling as part of distance. `archfit`
deliberately does **not** fold runtime/async coupling into distance — detected
async bridges are recorded as report-only `runtime_async` module rollups plus
`runtime_async_edges` source-module→runtime-target facts, never a scored distance
factor. The distance-confidence report may still explain that async bridges
increase perceived distance by reducing lifecycle coupling, but that narrative is
explanatory only until synchronous peers and first-party runtime counterparts are
detected deterministically (see [bc-measurement-v4.md §9](../design/bc-measurement-v4.md#9-non-goals-and-rejected-designs) for the rationale).

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

In `archfit` you set base volatility per module (`volatility:` or `subdomain:` in
`.archfit.yaml`). Git churn is never used as a volatility source for scoring — it
measures observed change, a mix of essential and accidental factors, and `archfit`
cannot separate them automatically. Declared subdomain volatility is the primary
input; the separate report-only `volatility_corroboration` block keeps git history
as supporting evidence only. When the opt-in cascade below is enabled,
deterministic strong-coupling chains can raise effective volatility before
scoring.

**Inferred-volatility cascade (opt-in, book Ch9):** when
`coupling.volatility_cascade: true` is set in `.archfit.yaml`, a deterministic
fixpoint propagation pass runs before scoring. If a module is strongly coupled
(`functional`, `symmetric`, or `intrusive`) to a high-effective-volatility module,
its effective volatility is raised to `high` for scoring purposes. This lets
archfit surface coupling chains that inherit core-domain volatility without
requiring every module to be manually annotated. Clone-only pairs are excluded
from the cascade because duplicated code is accidental coupling evidence, not a
runtime/domain dependency.

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
subdomain volatility and does not infer volatility from churn. It only surfaces
that history in the report-only `volatility_corroboration` block so humans can
compare declarations with recent touches.

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
| Distance   | any module boundary            | 9       |
| Distance   | `declared_external`            | 10      |
| Volatility | `supporting` / `generic`       | 3       |
| Volatility | `core`                         | 10      |

Balance maps to severity bands: 1–2 → `critical`, 3–4 → `high`,
5–6 → `medium`, 7–8 → `low`, 9–10 → `none`.

Three things worth noting:

- **The worst pattern** (S=Intrusive/Symmetric, a module boundary at D=9,
  V=high): `max(|10−9|, 10−10) + 1 = 2` (symmetric: 1) → `critical`. Across a
  module boundary an edge is critical exactly when it is strong (functional,
  symmetric or intrusive) and V is 10. The high band cannot occur there.
- **Asymmetric (modular) cases score well.** A Contract edge across a deploy
  boundary (S=1, D=9, V=10): `max(8, 0) + 1 = 9` → `none`. Low strength
  over high distance is balanced — the formula rewards it.
- **Ports are contract.** A call to an interface method, or through a
  `public:` surface, is contract coupling. Across a module boundary (S=1, D=9,
  V=10) it scores `max(8, 0) + 1 = 9` → `none`. The same call into a
  non-public package of a high-volatility module is functional and scores 2.

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
the v4 report-only behavior. `ScoreVersion` is `bc_score.v7`. See the [v7 design](../design/bc-measurement-v7.md) for the strength, distance, volatility and clone-fact rules.

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
   (`archfit config enrich`, `archfit explain --ai-summary`), but gate verdicts and metric
   values are computed by deterministic code only. The model finds candidates;
   humans pin labels; the gate stays reproducible. See
   [LLM enrichment](llm-enrich.md).

Book alignment status in the deterministic gate:

| Category         | What falls here                                                                                                                                                |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Book-exact       | Ch10 formula, published strength ordinals, scored cross-module `coupling_balance`, and abstain-not-fake for unknown S or D.                                    |
| Sound adaptation | Static extractor facts mapped onto book strength/connascence vocabulary; same-module `local_coupling`; transitive Ch9 cascade.                                 |
| Policy choice    | Level-relative distance (D=2, 9, 10), declared-only external seams, clone-only score/advisory policy, conservative undeclared V=10.                                |
| Report-only      | `connascence`, `local_coupling`, `runtime_async`, `runtime_async_edges`, `dynamic_imports`, `distance_config_candidates`, and tail risk. |
| Out of scope     | Dynamic connascence scoring, runtime/lifecycle distance scoring, churn-derived volatility scoring, and LLM-only gate changes.                                  |

The workflow: change code → `archfit check` → deterministic finding or
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
measured signal. The superseded v0.4 draft spec
(`docs/spec/arch-fitness-spec-v0.4.md`) records the original design rationale; it
is a historical document, not the current build spec — see
[Commands](commands.md) and [Configuration](configuration.md) for current
behavior.

[bc-book]: https://www.informit.com/store/balancing-coupling-in-software-design-universal-design-9780137353484
[lddd-book]: https://www.oreilly.com/library/view/learning-domain-driven-design/9781098100124/
