# Coupling model

archfit scores module and declared-external dependencies with strength, distance, and volatility.
The score describes balance. The report keeps metrics and seams as measurements;
it does not compare them with a stored reference. See [metrics](metrics.md) for metric behavior,
[configuration](configuration.md) for policy keys, and [results](results.md) for
report fields.

## The three inputs

### Strength

Strength measures how much knowledge crosses a boundary. Lower ordinals mean
weaker coupling.

| Strength     | Ordinal | Meaning                                                          |
| ------------ | ------: | ---------------------------------------------------------------- |
| `contract`   |       1 | The dependency uses an interface or declared `public:` contract. |
| `model`      |       3 | The dependency uses a shared data model.                         |
| `functional` |       8 | The dependency calls a non-public module surface.                |
| `symmetric`  |       9 | Both modules contain duplicated knowledge.                       |
| `intrusive`  |      10 | The dependency reaches a target declared `internal:`.            |

Strength follows declared surfaces and extractor evidence.
An `internal:` match is authoritative and means `intrusive`. A `public:` match
means at least `contract`.
Connascence does not set strength.

A callable hint does not raise a `public:` target above `contract`. Data evidence
can raise it to `model` for a concrete non-DTO type, field, variable, constant,
or method on a concrete receiver. A DTO remains `contract`. An `intrusive` hint
raises it to `intrusive`: a C# `internal` member used through
`InternalsVisibleTo`. Intrusive coupling integrates through details not meant for
integration. A `public:` glob declares a file's intended surface, so it cannot
make such a use a contract. An
`internal:` match stays `intrusive` regardless of hints. Without sufficient evidence, strength is
unknown and the scorer abstains.

### Distance

Distance measures the cost of changing modules together. The shipped v7 model is
level-relative:

| Boundary                 |          Distance |
| ------------------------ | ----------------: |
| Same module              |                 2 |
| Any module boundary      |                 9 |
| Declared external system |                10 |
| Unresolved boundary      | Unknown. Abstain. |

Owner and deploy-unit labels name module boundaries. They do not change the
distance. A cross-owner or cross-deploy-unit dependency still has D=9. Path
containment provides the boundary context used in reports.

### Volatility

Volatility measures the expected rate of change. The ordinals are `frozen` 1,
`low` 3, `medium` 6, and `high` 10. Undeclared volatility scores as 10, but
remains unrated. Reports identify this gap and can recommend `declare_volatility`.

Explicit `volatility:` takes precedence over `subdomain:`. `core` means high,
`supporting` and `generic` mean low. Contract, model, and intrusive edges use the
target module's volatility. Functional and symmetric edges use the higher
volatility of both modules. The optional cascade propagates volatility through the module graph to
a fixed point.

Finding explanations use **effective volatility**, not a claim about target churn.
For internal functional and symmetric coupling, a high-volatility source can produce a
high-volatility edge even when its target is declared low. Declared external systems
use their own target volatility. Cascade can also raise
a declared value. The seam ledger's `volatility_provenance` distinguishes declared,
inherited, cascade, and undeclared evidence. A seam aggregates edges, so its
strongest classification need not describe every advisory subgroup. These static
signals indicate change-propagation risk, not proof of an observed runtime cascade.

## Balance formula

For scored edges, archfit uses the Chapter 10 formula:

```text
balance = max(|S − D|, 10 − V) + 1
```

The result is an integer from 1 to 10. Higher values mean better balance.
Unknown strength or distance abstains. Same-module edges appear in
`local_coupling`. They do not enter `coupling_balance`.

The score maps to these severity bands:

| Balance | Band       |
| ------: | ---------- |
|     1–2 | `critical` |
|     3–4 | `high`     |
|     5–6 | `medium`   |
|     7–8 | `low`      |
|    9–10 | `none`     |

### Examples

An interface call across a module boundary has S=1, D=9, and V=10. Its balance
is 9, with band `none`.

A call into an internal package across a module boundary has S=10, D=9, and
V=10. Its balance is 2, with band `critical`.

The same internal call into a module with V=3 has balance 8, with band `low`.

## The `coupling` rule

Archfit 4 adds a built-in `coupling` rule to the check of policy version 3. It
is not released yet. The rule judges a dependency a change adds: new knowledge
across a module boundary needs a balance of at least 7. The balance uses the
formula above at D=9 and is shown as `9/10 (min 7)`.

| Volatility that applies | Use of the public part | Use past the public part | Copy |
| ----------------------- | ---------------------: | -----------------------: | ---: |
| `low`                   |                 9 or 8 |                        8 |    8 |
| `medium`                |                 9 or 7 |                      5 ✗ |  5 ✗ |
| `high`                  |                 9 or 7 |                      2 ✗ |  1 ✗ |

- A use of the public part is `contract` (9) or `model` (8 or 7). A use past it
  is `functional`, a copy `symmetric`.
- The volatility that applies is the target's for a use of the public part, and
  the higher of the two modules for a use past it and for a copy.
- An `intrusive` use reaches past the public part, so it also takes the higher
  of the two: a high-volatility source into a low target gives 2. This differs
  from the report's edge classification above, which keeps the target's
  volatility for intrusive edges. The rule lets a use past the public part
  happen only into a stable module, so a volatile source counts too.
- The rule uses the declared volatility. The cascade is not used, because with
  it one new dependency changes the balance of code the change did not touch.
- The `public` globs alone tell a use of the public part from a use past it. Both
  kinds of public use pass at every volatility, so the check needs no type
  analysis.

In words: a module may use the public part of a module it is allowed to depend
on. It may reach past the public part only when neither module is more volatile
than `low`.

## Distributed-monolith seam measurements

The report groups dependencies into ordered module pairs, called seams. One
seam can contain many dependency edges. A seam qualifies as a distributed-monolith
measurement when at least one import edge meets all of these conditions:

- Its strength is `functional`, `intrusive`, or `symmetric`.
- It crosses a module boundary.
- Its effective volatility is `high` and comes from a declaration, inheritance,
  or the enabled cascade.
- Its source is not a `composition_root`, `generated`, or `test` role, unless
  the edge is intrusive.

Medium volatility does not qualify. Qualification is separate from
the balance score. Owner and deploy labels do not change D=9 or make a seam
qualify by themselves.

The seam ledger, `coupling_balance`, and `unbalanced_edge` are report measurements.
They have no stored-reference gate or metric ratchet. The Archfit 4 check applies
the `coupling` rule to errors added by the change, independently of these report
measurements. See [metrics](metrics.md#deltas-and-gates) and [rules](rules.md).

## Copies and limits

The `duplication` rule checks identical literals added across module boundaries.
It is separate from import-edge scoring. See [rules](rules.md) for its limits.

Go uses `go/types` data for report hints. .NET uses Roslyn compiler symbols from the `archfit-roslyn` helper.

Published-language coupling between services has no type edge and is not
measured in any language: message records duplicated in each service, a
`.proto` that another project compiles directly, and HTTP or YARP runtime calls.
These stay outside the dependency graph.

TypeScript and Python imports have unknown strength unless declared `public:`
or `internal:` surfaces classify the target. Runtime async evidence is report-only. It does not
change distance or gate results. Unknown facts can lower confidence or leave a
dimension partial. See [evidence](evidence.md) for claim boundaries and
[configuration](configuration.md) for policy and extractor settings.
