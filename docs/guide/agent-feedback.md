# The AI-agent feedback loop

archfit is built to sit inside a coding agent's loop: run it after every
change, read machine-actionable output, fix, re-run. Everything in this loop
is deterministic — same repo + same config = byte-identical output.

## The loop

```text
agent edits code
  → archfit check [--base main] --json
  → exit 0?  healthy — done.
  → exit 2?  needs_attention — no blocking finding. Read the dimension whose
             status is not `measured` or the active diagnostic. Supply the named
             missing fact; never treat yellow as a fabricated healthy zero.
  → exit 1?  blocked — read agent_tasks[] — goal, constraints, files, validation
  → fix within the constraints
  → run the task's validation command
  → repeat
```

Use `check` inside repair loops and CI validation. Use `analyze` to generate
reports, diffs, or a post-check narrative with `archfit analyze --ai-summary`.

## agent_tasks — the gate repair channel

Every ACTIVE gate finding produces one structured repair task:

```json
{
  "finding_id": "8a4be7…",
  "rule_id": "no_internal_access",
  "origin": "introduced",
  "goal": "Replace the internal-API access from pkg/a/a.go to
           pkg/b/internal/impl.go with b's public API.",
  "constraints": [
    "Use only the public API of module b",
    "public surface of module \"b\": [pkg/b/api/**]"
  ],
  "files": ["pkg/a/a.go", "pkg/b/internal/impl.go"],
  "validation": ["archfit check -c .archfit.yaml"]
}
```

Goals are deterministic templates per rule type; constraints join the rule's
configured constraint text, allowed alternatives, and the target module's
public globs; validation is the exact `archfit check` command that must pass.
With `--base`, each current task also carries `origin`: `introduced`,
`pre_existing`, or `unknown`. `unknown` means analyzer evidence was asymmetric;
it is never upgraded to `introduced`. Origin is triage metadata and never
changes the verdict, a gate, or the exit code. `agent_tasks[]` is gate-only, and
advisory findings stay out of this channel.
Advisory promotion is gone: a scalar gate had to borrow findings to point at,
whereas
[`coupling.gate.distributed_monolith`](configuration-reference.md#couplinggate)
names its own seams — a blocked run emits one `bc/coupling_gate` gate finding
PER newly introduced seam, keyed `coupling-gate/<seamID>` and carrying the module
pair. `bc/imbalanced_coupling` and `bc/duplicated_knowledge` are diagnostics and
never gate.

**`files[]` existence guarantee.** Every entry is a repo-relative path that
exists on disk — this is the field an agent trusts blindly to open the right
file. Config module keys (Go), dotted module IDs (Python), and `crate::mod`
keys (Rust) are resolved against the extractors' own file/crate-root facts
before being emitted; an entry that cannot be resolved is dropped rather than
emitted as a bare key or ID. If dropping empties the set, `files` falls back to
the target module's config `paths:` root — itself resolved to a real path (a
Python dotted glob root goes through the module-file probe); if even that
isn't resolvable, `files` is legitimately empty — never a fabricated string
(`internal/assessment/agenttask/agenttask.go`, `filesFor`).

**`edge.path` group semantics.** For a rolled-up finding (`group_count > 1`),
`edge.from.path`/`edge.to.path` are taken from whichever member edge owns
`locations[0]` — never an arbitrary hash-ordered representative. When no
member owns `locations[0]` (TypeScript edges carry no locations), the paths
fall back to the representative member's own edge
(`internal/assessment/evaluation/advisories.go`, `groupEdgePaths`). Either way the pair
names one genuine member edge of the group. The path form is the graph
node's: a repo-relative file for Go and TypeScript, a dotted module ID for
Python (`myapp.domain`), a crate or `crate::mod` name for Rust — the
module-graph forms do not literally match the `locations[]` file entries.
For paths guaranteed to exist on disk, use `agent_tasks[].files[]`.
Only `bc/imbalanced_coupling` findings are rolled up (cap 8 members per
group); `bc/duplicated_knowledge` findings pass through individually and
never carry a `group_count`.

## Task origin with `--base`

`--base <ref>` classifies the current repair tasks in canonical JSON. It does
not create a second task list or a separate delta schema:

```json
{
  "comparison": {
    "task_origin_status": "comparable"
  },
  "agent_tasks": [
    { "finding_id": "finding-a", "origin": "introduced" },
    { "finding_id": "finding-b", "origin": "pre_existing" }
  ]
}
```

- `introduced` — the base run had no matching stable finding ID and all active
  finding-producing analyzers had comparable evidence.
- `pre_existing` — the base run observed the same stable finding ID.
- `unknown` — evidence could not place the task. Treat it as possibly
  introduced; missing evidence never manufactures an `introduced` result.
- `comparison.task_origin_status` is `unknown` when at least one task is unknown.
  When present, read `task_origin_reasons` even when the status is `comparable`:
  with no tasks,
  or when all tasks match the base, an analyzer difference can still be relevant
  to the next change.

A missing or duplicated coverage row, timeout, unfinished partial run, one-sided
analyzer evidence, or config-hash mismatch makes unmatched tasks `unknown` and
names the reason. The synthetic `bc/coupling_gate` task is per-run trip state,
not a stable base finding, so its origin is always `unknown`.

Three symmetric degradations remain comparable and are always disclosed:

- both sides have unresolved import specifiers;
- both sides covered every input but lost edge precision;
- the same activated analyzer is unavailable on both sides.

The safety argument is symmetry: neither side ran evidence the other could hide
behind. Asymmetric absence or partial evidence remains unknown. Matching uses
stable finding IDs only; lifecycle labels and gate-versus-advisory promotion do
not change origin, and a base finding reported as fixed does not make a current
task pre-existing. Origin remains triage metadata: it never changes the verdict,
a gate, or the exit code.

## Optional AI narrative

`archfit analyze --ai-summary` is not part of the repair channel. Run it after
`archfit check` when you want a cited, advisory architect review after the
deterministic output. Treat `claim_type: recommendation` entries as suggestions
only, and check their `finding_ids`, `metric_ids`, and `evidence_refs` before
acting. The AI review never changes `verdict`, `findings`, `metrics`, `score`,
or `agent_tasks[]`; agents should still use `agent_tasks[]` as the actionable
source of truth.

AI config drafts are also non-actionable until reviewed. An agent may surface
`config update --ai-classify` proposals or draft files from `config enrich owner`,
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
- **Metrics** — `coupling_balance` (scored; a diagnostic that never gates), the
  baseline-delta gated `unbalanced_edge`, `cycle`, `encapsulation`, `coverage`,
  and report-only `blast_radius`.
- **Structural facts** — neutral per-module evidence (fan-in, fan-out, LOC)
  for downstream judgment.

## Lifecycle the agent must respect

Findings carry status: `new` (gates), `baseline` (accepted — do not "fix"
unprompted), `waived` (time-boxed waiver), `expired_waiver` (gates
again), `fixed` (gone since baseline). `archfit baseline` accepts the current
state; waivers live in config with expiry dates.
