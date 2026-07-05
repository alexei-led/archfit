# The AI-agent feedback loop

archfit is built to sit inside a coding agent's loop: run it after every
change, read machine-actionable output, fix, re-run. Everything in this loop
is deterministic — same repo + same config = byte-identical output.

## The loop

```text
agent edits code
  → archfit analyze --gate [--base main] --json
  → exit 0?  done.
  → exit 1?  read agent_tasks[] — goal, constraints, files, validation
  → fix within the constraints
  → run the task's validation command
  → repeat
```

## agent_tasks — the actionable channel

Every ACTIVE gate finding produces one structured repair task:

```json
{
  "finding_id": "8a4be7…",
  "rule_id": "no_internal_access",
  "goal": "Replace the internal-API access from pkg/a/a.go to
           pkg/b/internal/impl.go with b's public API.",
  "constraints": [
    "Use only the public API of module b",
    "public surface of module \"b\": [pkg/b/api/**]"
  ],
  "files": ["pkg/a/a.go", "pkg/b/internal/impl.go"],
  "validation": ["archfit analyze --gate -c .archfit.yaml --full"]
}
```

Goals are deterministic templates per rule type; constraints join the rule's
configured constraint text, allowed alternatives, and the target module's
public globs; validation is the exact command that must pass. Advisory
findings normally produce no tasks — they are signals, not orders. The one
exception: a tripped [`coupling.gate`](configuration-reference.md#couplinggate)
promotes the active `bc/imbalanced_coupling` advisories to gate kind, so the
edges behind the failing score arrive in `agent_tasks[]` with file evidence
(`bc/duplicated_knowledge` stays advisory — it never gates).

**`files[]` existence guarantee.** Every entry is a repo-relative path that
exists on disk — this is the field an agent trusts blindly to open the right
file. Config module keys (Go), dotted module IDs (Python), and `crate::mod`
keys (Rust) are resolved against the extractors' own file/crate-root facts
before being emitted; an entry that cannot be resolved is dropped rather than
emitted as a bare key or ID. If dropping empties the set, `files` falls back to
the target module's config `paths:` root — itself resolved to a real path (a
Python dotted glob root goes through the module-file probe); if even that
isn't resolvable, `files` is legitimately empty — never a fabricated string
(`internal/agenttask/agenttask.go`, `filesFor`).

**`edge.path` group semantics.** For a rolled-up finding (`group_count > 1`),
`edge.from.path`/`edge.to.path` are taken from whichever member edge owns
`locations[0]` — never an arbitrary hash-ordered representative. When no
member owns `locations[0]` (TypeScript edges carry no locations), the paths
fall back to the representative member's own edge
(`internal/engine/advisories.go`, `groupEdgePaths`). Either way the pair
names one genuine member edge of the group. The path form is the graph
node's: a repo-relative file for Go and TypeScript, a dotted module ID for
Python (`myapp.domain`), a crate or `crate::mod` name for Rust — the
module-graph forms do not literally match the `locations[]` file entries.
For paths guaranteed to exist on disk, use `agent_tasks[].files[]`.
Only `bc/imbalanced_coupling` findings are rolled up (cap 8 members per
group); `bc/duplicated_knowledge` findings pass through individually and
never carry a `group_count`.

## Optional LLM review

`archfit analyze --llm` is not part of the repair channel. It appends a cited,
advisory architect review after the deterministic output. Treat `claim_type:
recommendation` entries as suggestions only, and check their `finding_ids`,
`metric_ids`, and `evidence_refs` before acting. The LLM review never changes
`verdict`, `findings`, `metrics`, `score`, or `agent_tasks[]`; agents should still
use `agent_tasks[]` as the actionable source of truth.

LLM config drafts are also non-actionable until reviewed. An agent may surface
`config update --llm` proposals or draft files from `config enrich owner`,
`volatility`, and `subdomain`, but it must not pin them into `.archfit.yaml` or
`.archfit-labels.yaml` without explicit human approval.

## SARIF — the CI annotation channel

`--format sarif` emits SARIF 2.1.0 (schema-validated): active gate findings as
`error`, advisories as `warning`, resolved/baselined as `note`, with file+line
locations and stable `archfit/v1` fingerprints. Metrics and the verdict ride
in `runs[0].properties`. Pipe it to GitHub code scanning to get findings as
inline PR annotations.

## The dimensions an agent sees

- **Gate findings** — boundary violations (forbidden deps, internal access,
  layer inversions, cycles, unreviewed new cross-module deps).
- **BC advisories** — Balanced Coupling imbalances (strength × distance ×
  volatility) at or above the configured severity, plus report-only
  `bc/duplicated_knowledge` for cross-module clone pairs with no import edge.
- **Metrics** — `coupling_balance` (scored; gates via the opt-in
  `coupling.gate`), the baseline-delta gated `unbalanced_edge`, `cycle`,
  `encapsulation`, `coverage`, and report-only `blast_radius`.
- **Structural facts** — neutral per-module evidence (fan-in, fan-out, LOC)
  for downstream judgment.

## Lifecycle the agent must respect

Findings carry status: `new` (gates), `baseline` (accepted — do not "fix"
unprompted), `waived` (time-boxed waiver), `expired_waiver` (gates
again), `fixed` (gone since baseline). `archfit baseline` accepts the current
state; waivers live in config with expiry dates.
