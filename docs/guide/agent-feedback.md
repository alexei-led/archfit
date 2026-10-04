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
             status is not `measured`, the active diagnostic, or
             `decision.unevaluated_required_rules`. Supply the named missing
             fact; never treat yellow as a fabricated healthy zero.
  → exit 1?  blocked — read agent_tasks[] — goal, constraints, files, validation
             (empty agent_tasks[]: a metric ratchet or a required analyzer
             blocked; the text/Markdown METRIC RATCHET section names the metric)
  → fix within the constraints
  → run the task's validation command
  → done when that run no longer lists the task's finding_id and is not blocked
  → repeat
```

Use `check` inside repair loops and CI validation. Use `analyze` to generate
reports, diffs, or a post-check narrative with `archfit analyze --ai-summary`.

## The output has a published JSON Schema

`archfit.state.schema.json` (repo root) describes the
`archfit.architecture-state.v1` document that `--format json` emits. Validate
against it, or generate your consumer types from it, instead of hand-modelling
the output. Note that `archfit.schema.json` is a different contract — that one
describes `.archfit.yaml`.

Nullable by contract, not by accident: `seams[].scores.p10` and `p90` are `null`
when a seam has fewer than ten scored edges. A percentile nobody can compute is
reported as absent, never as `0`.

## agent_tasks — the gate repair channel

Every ACTIVE gate finding produces one structured repair task:

```json
{
  "finding_id": "8a4be7…",
  "rule_id": "no_internal_access",
  "repair_kind": "code_change",
  "origin": "introduced",
  "goal": "Replace the internal-API access from pkg/a/a.go to
           pkg/b/internal/impl.go with b's public API.",
  "constraints": [
    "Use only the public API of module b",
    "public surface of module \"b\": [pkg/b/api/**]"
  ],
  "files": ["pkg/a/a.go", "pkg/b/internal/impl.go"],
  "validation": ["archfit check -c .archfit.yaml --lang go --require-tools"]
}
```

`repair_kind` is `code_change` for a finding the agent can address in source and
`needs_owner_decision` for a policy or accepted-debt decision. Goals are
deterministic templates per rule type; constraints carry the rule type's fixed
constraint text plus the target module's public globs, except on
`forbidden_dependency` and `forbidden_layer_direction` tasks, which never list
the target's public surface (the dependency is forbidden through it too).
Validation is the exact `archfit check` command to re-run. The repair is done
when that run no longer lists the task's `finding_id` and its verdict is not
`blocked`; exit 2 can remain and is not a failed repair. One import that breaks
two rules is two findings and therefore two tasks, keyed by their finding IDs.
The command replays the effective analysis flags (`--base`, repeated `--lang`,
and `--require-tools`) so the repair is checked under the same conditions.
`--refresh` is deliberately not serialized: cache-control must not change the
validation result. `--no-advisories` and output-format flags are not part of
the validation contract.
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

**Policy-correct repair goals.** A `forbidden_dependency` goal never proposes
the target module's public API as a repair, because the dependency itself is
forbidden. A `new_cross_module_dependency` goal asks for removal or an explicit
architecture-owner decision; it does not suggest `archfit baseline` as a code
fix. Use a public API only when the active rule explicitly permits that
alternative.

**`files[]` existence guarantee.** Every entry is a repo-relative path that
exists on disk — this is the field an agent trusts blindly to open the right
file. Config module keys (Go), dotted module IDs (Python), and `crate::mod`
keys (Rust) are resolved against the extractors' own file/crate-root facts
before being emitted; an entry that cannot be resolved is dropped rather than
emitted as a bare key or ID. If dropping empties the set, `files` falls back to
the target module's config `paths:` root — itself resolved to a real path (a
Python dotted glob root goes through the module-file probe); if even that
isn't resolvable, `files` is legitimately empty — never a fabricated string
(`internal/assessment/agenttask/agenttask.go`, `filesFor`). A
`bc/coupling_gate` finding names only a module pair, so its task resolves the
node paths and import sites of up to 20 of the seam's qualifying edges
(critical band at high distance, in endpoint order) and falls back to the
source, then the target, module's `paths:` root. Seam-gate tasks carry no
`declarations`.

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

Measurement compatibility is part of this comparison. The run publishes a
`comparison.measurement_profile` with a profile version, settings hash, and
the producer/tool versions and statuses that supplied the evidence. A missing,
unknown, or incompatible profile makes the comparison `non_comparable` and
keeps unmatched task origins `unknown`; observed matching findings can still be
`pre_existing`. Reasons name the producer or profile field. The reference and
status in `comparison` describe `--base`; its fingerprints describe the current
run. The persisted baseline used
by the gate is reported separately as `gate_reference`, so a base comparison
does not silently become a gate reference and a baseline mismatch does not
pretend to be a base delta.

Three symmetric degradations remain comparable and are always disclosed:

- both sides have unresolved import specifiers;
- both sides covered every input but lost edge precision;
- the same activated analyzer is unavailable on both sides.

Completed partial producers carry `partial_basis`: `unresolved_specifiers` or
`degraded_precision`. Matching statuses without a recognized basis do not prove
compatibility. Missing inputs, timeouts, and opaque configuration unknowns still
prevent comparison, even if two runs report the same unknown text.

The history producer identifies its fixed recent-500/full-history-fallback
algorithm. The observed fallback choice and sample counts stay in measurement
metadata; an ordinary new commit does not change the profile merely by making
the bounded history query sufficient.

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
state; waivers live in config with expiry dates. Waivers must name a declared or
supported synthetic rule, at least one scope selector (`from` or `to`), a
non-empty `reason`, `approved_by`, and an ISO date in `expires`; invalid
metadata is rejected while loading the config. Matching is independent of YAML
order: an active match wins, and an expired match is reported as
`expired_waiver` only when no active match applies. Baseline capture skips
findings covered by temporary waivers, including expired waivers, and prints
how many were skipped, so a temporary exception is never silently converted
into permanent accepted debt. Review the full capture before committing it.

## Required rule evidence

When a configured `gate: fail` rule applies to the source tree but its required
producer evidence is incomplete, JSON includes
`decision.unevaluated_required_rules`:

```json
{
  "unevaluated_required_rules": [
    {
      "rule_id": "no_internal_access",
      "reason": "go/packages evidence is partial: ..."
    }
  ]
}
```

The list is sorted by `rule_id` and omitted when empty. A known active blocker
still sets `decision.hard_gates` to `fail`; otherwise a non-empty list sets it to
`unmeasured`, which keeps `check` at exit `2`. Read these structured fields
directly. Do not infer required-rule coverage by searching prose or by treating
an empty finding list as proof that the rule passed. A rule whose selector
matches no scanned source is listed with the reason
`selector matches nothing: <from|to> <glob>`: it could never find a violation,
so it is a policy defect for the config owner, not missing evidence and not a
code change to make. A declared guard (`guard: true`) is not listed.
