# Balanced Coupling measurement engine — design v7

Date: 2026-10-07. Status: shipped in v3.0.0 (`ScoreVersion = "bc_score.v7"`).

This page is the contract for the coupling score. It replaces the strength,
distance, volatility and clone rules of
[bc-measurement-v4.md](bc-measurement-v4.md) and
[20260705-bc-score-v6.md](20260705-bc-score-v6.md). The reasons and the
rejected options are in the
[plan](../plans/erosion-prevention/coupling-score-v7.md). The measured effect on
real repositories is in the [corpus report](../reports/20261007-bc-v7-corpus.md).

## What did not change

The formula is Khononov, _Balancing Coupling in Software Design_, Chapter 10,
without change:

```text
balance = max(|S − D|, 10 − V) + 1        range 1..10, higher is better
```

Strength ordinals (contract 1, model 3, functional 8, symmetric 9, intrusive
10), volatility ordinals, severity bands and abstain-not-fake are as before. An
edge with unknown strength or distance is not scored.

## What changed

The formula inputs and the seam gate change. The gate counts **seams**: one
ordered module pair, however many imports express it.

### Distance is level-relative

| Boundary                   | Token                           | D  |
| -------------------------- | ------------------------------- | -- |
| Same module                | `same_module`                   | 2  |
| Module boundary            | `cross_module`                  | 9  |
| Owner boundary             | `cross_module_different_owner`  | 9  |
| Deploy-unit boundary       | `cross_deploy_unit`             | 9  |
| `external_systems:` target | `declared_external`             | 10 |
| Unresolved                 | `unknown`                       | abstain |

Nine is the top internal rung. The book puts in-house services at 9 and vendors
at 10. Owner and deploy unit name the boundary only. A containment tree from the
`paths:` globs gives each seam a container (`raw_distance.basis` reads
`<boundary>@<container>`), so a key rename changes nothing.

### Strength

- A call to an interface type or an interface method is **contract**.
- A `public:` target is the integration contract. A callable hint never raises
  it. Only data evidence does (`Edge.DataStrengthHint`: a concrete non-DTO type,
  field, var, const, or a method on a concrete receiver). A DTO stays contract.
- An `internal:` target is intrusive. A call into a non-public module is
  functional. Pinned labels set strength.
- Connascence never sets strength.

### Volatility

Contract, model and intrusive edges use the target's effective V. Functional and
symmetric edges and clone facts use the worse V of the two modules. Declared
`high` outranks undeclared. Undeclared still scores as V=10 but is **unrated**: a
seam flagged only because of it never qualifies, and its guidance is
`declare_volatility`. The coupling dimension has the required fact
`coupling volatility`.

### Clone facts

A clone pair does not upgrade an import edge. Each cross-module pair is its own
symmetric fact (S=9, D=9, worse V). A pair whose modules share an import edge
attaches to one seam (only with `coupling.duplicated_knowledge: score`, the
default; `advisory` keeps clone facts out of seams); it counts in
`scored_edges`, can set severity and
hypothesis, and never makes a seam qualify.

### Severity across a module boundary

| S \ V      | frozen  | low    | medium   | high or undeclared |
| ---------- | ------- | ------ | -------- | ------------------ |
| contract   | 10 none | 9 none | 9 none   | 9 none             |
| model      | 10 none | 8 low  | 7 low    | 7 low              |
| functional | 10 none | 8 low  | 5 medium | 2 critical         |
| symmetric  | 10 none | 8 low  | 5 medium | 1 critical         |
| intrusive  | 10 none | 8 low  | 5 medium | 2 critical         |

The high band cannot occur across a module boundary. An edge is critical exactly
when it is strong (functional, symmetric or intrusive) and V is 10.

### Seam gate

A seam qualifies when one scored **import edge** is functional, intrusive or
pinned symmetric; crosses a module boundary; has effective V `high` from a
declared, inherited or cascade end; and does not start in a cohesive role
(`composition_root`, `generated`, `test`) unless it is intrusive. Medium V and
clone facts never qualify. Qualification is a subset of critical severity. The
seam shows its lowest-balance qualifying edge. Gate reasons name the boundary
and container; only a deploy-unit boundary says "distributed monolith". The gate
stays score-blind.

### Guidance

One function (`relationship.BalancingHypothesis`) feeds seams, advisories and
agent tasks: `balanced`, `accept_low_volatility`, `introduce_contract`,
`move_functionality`, `declare_volatility`, `expected_by_role`, `follow_rule`.

### Measurement contract

Measurement contracts: go/packages v2 and scip v2. Fact-cache schema 4 adds a
Go semantics version to the cache key of each Go member. Metric versions:
unbalanced_edge v3 and encapsulation v2. A baseline written under v6 is
non-comparable until `archfit baseline` runs again.

## Worked examples

All modules are core (V high) unless stated. The tests in
`internal/relationship/analysis/v7_examples_test.go` build each scenario with
these names and fail when a rule changes.

| #  | Scenario                                                                                          | Result                                                                                     |
| -- | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| 1  | One owner, one deploy unit. `orders` calls an interface in `billing`.                             | Contract, D=9: 9 none, for any key spelling.                                               |
| 2  | An adapter reads `billing/store`, declared `internal:`.                                           | Intrusive, D=9: 2 critical, qualifies. With `billing` supporting: 8 low.                   |
| 3  | Two teams, one deploy unit. `orders` (core) calls non-public `pricing/engine` (supporting).        | Functional, V = worse(high, low) = high: 2 critical, qualifies. Through `pricing/api`: 9 none. |
| 4  | Two deploy units. `web` calls an interface in `inventory`.                                        | Contract, D=9: 9 none. Never a distributed monolith.                                       |
| 5  | `sales/cart` calls sibling `sales/invoice` and `stock/lookup` on another branch, both functional. | Both 2 critical. Containers: `sales` and `system`.                                         |
| 6  | No volatility declared. `app` calls an interface and a concrete function.                         | Interface: 9 none. Concrete: 2 critical, unrated, `declare_volatility`. Coupling `partial`. |

## Limits

- Without a `public:` target, a call that is not to an interface reads as
  functional. TypeScript without SCIP reads every runtime import as functional
  unless the target matches a `public:` glob. Measure before you enable
  `mode: fail`.
- `config init` for TypeScript still writes the whole module as `public:`, so
  those imports read as contract until the owner narrows the surface.
- SCIP does not see the receiver of a method, so concrete-method data evidence
  exists only for Go.
- `config init` does not write `public:` for Go. A `public:` entry claims a
  published contract; the owner declares it.
- Runtime async stays report-only. It never enters distance.

Chapter references only. No book text beyond the formula is copied.
