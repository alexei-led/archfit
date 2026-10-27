# Coupling model

archfit scores module and declared-external dependencies with strength, distance, and volatility.
The score describes balance. The seam gate checks qualifying dependencies against
a comparable baseline. See [metrics](metrics.md) for metric behavior,
[configuration](configuration.md) for policy keys, and [results](results.md) for
report fields.

## The three inputs

### Strength

Strength measures how much knowledge crosses a boundary. Lower ordinals mean
weaker coupling.

| Strength | Ordinal | Meaning |
| --- | ---: | --- |
| `contract` | 1 | The dependency uses an interface or declared `public:` contract. |
| `model` | 3 | The dependency uses a shared data model. |
| `functional` | 8 | The dependency calls a non-public module surface. |
| `symmetric` | 9 | Both modules contain duplicated knowledge. |
| `intrusive` | 10 | The dependency reaches a target declared `internal:`. |

Strength follows declared surfaces, approved labels, and extractor evidence.
An `internal:` match is authoritative and means `intrusive`. A `public:` match
means at least `contract`.
An approved human or tool label can pin strength unless an `internal:` match applies.
Unset label provenance means human.
An LLM label only fills strength that declared surfaces and extractor evidence leave unknown.
See [label precedence](configuration.md#labels-and-draft-files) for the complete contract.
Connascence does not set strength.

A callable hint does not raise a `public:` target above `contract`. Data evidence
can raise it to `model` for a concrete non-DTO type, field, variable, constant,
or method on a concrete receiver. A DTO remains `contract`. An `internal:` match
stays `intrusive` regardless of hints. Without sufficient evidence, strength is
unknown and the scorer abstains.

### Distance

Distance measures the cost of changing modules together. The shipped v7 model is
level-relative:

| Boundary | Distance |
| --- | ---: |
| Same module | 2 |
| Any module boundary | 9 |
| Declared external system | 10 |
| Unresolved boundary | Unknown. Abstain. |

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
volatility of both modules. Clone facts use the higher volatility of both
modules. The optional cascade propagates volatility through the module graph to
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

| Balance | Band |
| ---: | --- |
| 1–2 | `critical` |
| 3–4 | `high` |
| 5–6 | `medium` |
| 7–8 | `low` |
| 9–10 | `none` |

### Examples

An interface call across a module boundary has S=1, D=9, and V=10. Its balance
is 9, with band `none`.

A call into an internal package across a module boundary has S=10, D=9, and
V=10. Its balance is 2, with band `critical`.

The same internal call into a module with V=3 has balance 8, with band `low`.

## Distributed-monolith seam gate

The gate counts ordered module pairs, called seams. One seam can contain many
dependency edges. A seam qualifies when at least one import edge meets all of
these conditions:

- Its strength is `functional`, `intrusive`, or pinned `symmetric`.
- It crosses a module boundary.
- Its effective volatility is `high` and comes from a declaration, inheritance,
  or the enabled cascade.
- Its source is not a `composition_root`, `generated`, or `test` role, unless
  the edge is intrusive.

Medium volatility and clone facts do not qualify. Qualification is separate from
the balance score. The gate compares qualifying seam identities with a
comparable baseline. It counts newly introduced seams, not imports. Owner and
deploy labels do not change D=9 or make a seam qualify by themselves.

`coupling_balance` is diagnostic. It does not gate directly. The seam gate can
block when its configured limit is exceeded. `unbalanced_edge` also supports a
baseline ratchet. See [metrics](metrics.md#deltas-and-gates) for both paths.

## Clones, labels, and limits

A cross-module clone pair is symmetric coupling evidence. It does not upgrade an
import edge. With `coupling.duplicated_knowledge: score` (the default), each
clone pair contributes a scored fact to `coupling_balance`. A connected pair
attaches to its existing seam. A clone-only pair has no seam, but still
contributes to `coupling_balance`. With `advisory`, clone facts stay outside the
score and appear as advisory evidence. Clone facts never qualify a seam.

SCIP refines symbol-level hints for TypeScript, Python, and Rust. Go uses
`go/types` data. .NET uses Roslyn compiler symbols from the `archfit-roslyn` helper. SCIP cannot identify a method receiver, so concrete-method data
evidence is available only for Go.

TypeScript without SCIP treats runtime imports as functional unless a target
matches a `public:` glob. Runtime async evidence is report-only. It does not
change distance or gate results. Unknown facts can lower confidence or leave a
dimension partial. See [evidence](evidence.md) for claim boundaries and
[configuration](configuration.md) for labels and extractor settings.
