# archfit agent feedback loop

archfit is built to sit inside a coding agent's loop: run it after every change,
read machine-actionable output, fix, re-run. The deterministic gate is
`archfit check` — same repo + same config + same tool/cache inputs = stable
output.

## The loop

```text
agent edits code
  → archfit check --json
  → exit 0 or 2?  no blocker remains. (2 = needs_attention: read the active
     diagnostic or named missing evidence; never fabricate it to force 0.)
  → exit 1?  read agent_tasks[] — goal, constraints, files, validation
  → fix within the constraints, touching only the listed files where possible
  → run the task's validation command verbatim
  → repeat
```

Use `archfit analyze` for local reports and AI summaries; do not use it as the CI
or repair-loop gate.

## agent_tasks[] — the actionable channel

Every active gate finding produces one structured repair task:

```json
{
  "finding_id": "8a4be7…",
  "rule_id": "no_internal_access",
  "repair_kind": "code_change",
  "origin": "introduced",
  "goal": "Replace the internal-API access from pkg/a/a.go with b's public API.",
  "constraints": [
    "Use only the public API of module b",
    "public surface of module \"b\": [pkg/b/api/**]"
  ],
  "files": ["pkg/a/a.go", "pkg/b/internal/impl.go"],
  "validation": ["archfit check -c .archfit.yaml"]
}
```

- `origin` — present with `--base`: `introduced`, `pre_existing`, or `unknown`.
  `unknown` is conservative and means the two runs did not have comparable
  analyzer evidence for this task; it never means introduced.
- `repair_kind` — `code_change` when the agent can repair source, or
  `needs_owner_decision` when the finding requires a policy or accepted-debt
  decision.
- `goal` — deterministic template per rule type. A `forbidden_dependency` goal
  never proposes the target module's public API, and a
  `new_cross_module_dependency` goal never proposes baseline capture as a code
  fix.
- `constraints` — the rule's constraint text plus allowed alternatives and the
  target module's public globs.
- `files` — candidate files to touch.
- `validation` — the exact command that must pass. It preserves effective
  `--base`, repeated `--lang`, and `--require-tools`; `--refresh` is omitted so
  cache-control cannot change validation output.

Advisory findings never produce tasks — they are signals, not orders.

`agent_tasks[]` are for `check --json`. Report-only `analyze --json` is for
inspection and summaries; do not treat a successful analyze exit as a clean gate.

## Status lifecycle the agent must respect

- `new` — active gate finding.
- `baseline` — accepted; do not "fix" unprompted.
- `waived` — time-boxed waiver.
- `expired_waiver` — gates again.
- `fixed` — gone since baseline.

`archfit baseline` accepts the current state; waivers live in config with expiry
dates. Waivers require a valid rule reference, at least one of `from` or `to`,
`reason`, `approved_by`, and a `YYYY-MM-DD` `expires` value. Invalid metadata is
rejected at config load. Matching is independent of YAML order: an active match
wins, and an expired match is used only when no active match applies. Baseline
capture skips findings covered by temporary waivers, including expired waivers,
and prints the count.

## Required rule evidence

For an applicable `gate: fail` rule whose producer evidence is incomplete,
canonical JSON includes sorted `decision.unevaluated_required_rules` entries:

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

The field is omitted when empty. A known blocker still yields
`decision.hard_gates: "fail"`; otherwise a non-empty list yields
`"unmeasured"`, so `check` remains exit `2`. Read this structured field rather
than searching prose or treating an empty finding list as proof of a pass.

## SARIF — the CI annotation channel

`archfit check --sarif` emits SARIF 2.1.0 (schema-validated): active gate
findings as `error`, advisories as `warning`, resolved/baselined as `note`, with
file+line locations and stable `archfit/v1` fingerprints. The whole architecture
state — verdict, decision, the nine dimensions with their status/gate/confidence,
and the coverage split — rides in `runs[0].properties`. Pipe it to GitHub code
scanning for inline PR annotations.

## Comparison with a Git reference (--base)

`archfit check --base <ref>` adds comparison and task-origin metadata for the
selected Git reference. Its gate still evaluates the current tree against
policy and the persisted approved baseline. `archfit analyze --base <ref>` is
the report-only equivalent. Text/Markdown disclose comparison status and
reference. JSON/SARIF stay the normal HEAD
architecture-state contract; there is no separate delta schema or parallel task
list. With `--base`, canonical JSON classifies each current `agent_tasks[]` entry
through its optional `origin` field. This metadata never changes the verdict,
gates, or exit code. `--require-tools` applies exactly as without `--base`.

`comparison.measurement_profile` records the profile version, settings hash, and
producer semantics/status/tool versions. Symmetric completed partials carry
`partial_basis` (`unresolved_specifiers` or `degraded_precision`); opaque
unknowns are not made comparable by matching text. A missing or incompatible
profile makes the comparison `non_comparable` and keeps unmatched origins
`unknown`; observed matching findings can remain `pre_existing`.
The persisted baseline used by gates is reported separately as
`gate_reference`; `--base` never replaces it.

## Coverage gaps — missing evidence is loud, not green

A metric reading `n/a` (and a `coverage.tools[]` row that is not `ok` in the
primary JSON) means an analyzer did not run — archfit refuses to score absence as
health, it does not fail by default. An agent treats a gap as "install this tool /
fill this config", not as a passing gate. Coverage gaps do **not** produce
`agent_tasks` and do not fail unless the run opts in with `archfit check
--require-tools` (or `analyzers.<x>.gate: fail`), which exits `1`.

## What an agent sees

- The architecture state — one `verdict`, nine `dimensions` each with its own
  status/gate/confidence/denominator and an explicit list of what it could not
  measure, and a `coverage` split. No repository score.
- The coupling `seams` ledger — one record per ordered module pair, with a stable
  ID, a score distribution, the book Ch10 quadrant, and a balancing hypothesis.
  This is the unit to redesign; individual import edges are not.
- Gate findings — boundary violations (forbidden deps, internal access, layer
  inversions, cycles, unreviewed new cross-module deps).
- BC advisories — Balanced Coupling imbalances (strength × distance × volatility).
- Metrics — boundary health, modularity, structural risk, and drift; honestly
  `n/a` when the evidence is missing.
- Coverage gaps + config warnings — missing tools and under-specified modules.
- Structural facts — neutral per-module evidence (fan-in/out, LOC, co-change).
