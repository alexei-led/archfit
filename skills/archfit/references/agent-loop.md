# archfit agent feedback loop

archfit is built to sit inside a coding agent's loop: run it after every change,
read machine-actionable output, fix, re-run. The deterministic gate is
`archfit check` — same repo + same config + same tool/cache inputs = stable
output.

## The loop

```text
agent plans an import
  → archfit policy can-import <file> <target>   (denied? do not add it)
agent edits code
  → archfit check --format agent   (one next_action), or for the full state:
  → archfit check --json
  → exit 0 or 2?  no blocker remains. (2 = needs_attention: read the active
     diagnostic or named missing evidence. Never fabricate evidence to force 0.)
  → exit 1?  read agent_tasks[] — goal, constraints, files, validation
     (empty agent_tasks[]: a required analyzer blocked. A metric ratchet
      creates a task for rule metric/<name>.)
  → fix within the constraints. Change the importing side of the edge.
  → run the task's validation command verbatim
  → done when that run no longer lists the task's finding_id and is not
     blocked. Exit 2 after a correct fix is not a failed repair.
  → repeat
```

Use `archfit analyze` for local reports and AI summaries. Use `archfit check` as
the CI and repair-loop gate.

## agent_tasks[] — the actionable channel

Every active gate finding produces one structured repair task. One import that
breaks two rules is two findings, so it yields two tasks:

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
  `unknown` means the runs lacked comparable analyzer evidence. It never means
  introduced.
- `repair_kind` — `code_change` when the agent can repair source, or
  `needs_owner_decision` when the finding requires a policy or accepted-debt
  decision.
- `goal` — deterministic template per rule type. A `forbidden_dependency` goal
  never proposes the target module's public API, and a
  `new_cross_module_dependency` goal never proposes baseline capture as a code
  fix.
- `constraints` — the rule type's fixed constraint text, plus the target
  module's public globs. `forbidden_dependency` and `forbidden_layer_direction`
  tasks never list those globs: the dependency is still forbidden through the
  target's public API.
- `files` — existing repo paths for both edge ends and the import sites. The
  target end is also listed. For a forbidden-dependency or layer task, change
  the importing side. A `bc/coupling_gate` task lists files for up to 20
  qualifying seam edges.
- `validation` — the exact command to re-run. It preserves effective `--base`,
  repeated `--lang`, and `--require-tools`. It omits `--refresh` so cache control
  cannot change validation output. The repair is done when the run no longer
  lists the task's `finding_id` and its verdict is not `blocked`. Exit `2` is
  still possible.

Advisory findings never produce tasks — they are signals, not orders.

`agent_tasks[]` are for `check --json`. Report-only `analyze --json` is for
inspection and summaries. A successful analyze exit does not prove a clean gate.

## Status lifecycle the agent must respect

- `new` — active gate finding.
- `baseline` — accepted. Do not "fix" it without a request.
- `waived` — time-boxed waiver.
- `expired_waiver` — gates again.
- `fixed` — gone since baseline.

`archfit baseline` accepts the current state. Waivers live in config with
expiry dates. Each waiver requires a valid rule reference, `reason`,
`approved_by`, and a `YYYY-MM-DD` `expires` value.

Most waivers also require at least one of `from` or `to`. Metric ratchets
(`metric/<name>`) and the edge-less rules `map/uncovered_path`, `map/dead_rule`,
and `map/stale_review` require a rule-only waiver with no endpoints.
`bc/coupling_gate` cannot be waived. Configure
`coupling.gate.distributed_monolith.mode` instead. Config loading rejects
invalid waiver metadata.

Waiver matching does not depend on YAML order. An active match wins. Archfit
uses an expired match only when no active match applies. Baseline capture skips
findings covered by temporary waivers, including expired waivers, and prints
the count.

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
`decision.hard_gates: "fail"`. Otherwise, a non-empty list yields
`"unmeasured"`, so `check` remains exit `2`. Read this field. Do not search
prose or treat an empty finding list as proof of a pass.

## SARIF — the CI annotation channel

`archfit check --sarif` emits schema-validated SARIF 2.1.0. Active gate findings
use `error`. Advisories use `warning`. Resolved or baselined findings use `note`.
Findings have file and line locations and stable `archfit/v1` fingerprints.
`runs[0].properties` contains the verdict, decision, dimensions, and coverage
split. Send the file to GitHub code scanning for inline PR annotations.

## Comparison with a Git reference (--base)

`archfit check --base <ref>` adds comparison and origin metadata for the Git
reference. Its gate still checks the current tree against policy and the
persisted approved baseline. `archfit analyze --base <ref>` is the report-only
equivalent. Text and Markdown show comparison status and reference.

JSON and SARIF keep the HEAD architecture-state contract. There is no separate
delta schema or task list. Canonical JSON adds an optional `origin` field to
each current finding and its `agent_tasks[]` entry. The fields
`comparison.origin_status`, `origin_reasons`, `introduced_finding_ids`, and
`resolved_finding_ids` describe the result. This metadata does not change the
verdict, gates, or exit code. `--require-tools` works the same with or without
`--base`.

`comparison.measurement_profile` records the profile version, settings hash,
producer semantics, status, and tool versions. Symmetric completed partials
carry `partial_basis` (`unresolved_specifiers` or `degraded_precision`). Opaque
unknowns do not become comparable when their text matches.

A missing or incompatible profile sets `comparison.status` to
`non_comparable`. It does not make origins `unknown`. Each differing producer
version appears in `origin_reasons`. The persisted baseline used by gates
appears separately as `gate_reference`. `--base` never replaces it.

## Coverage gaps — missing evidence is loud, not green

A metric reading `n/a` or a `coverage.tools[]` row with a status other than `ok`
means an analyzer did not run. Archfit does not treat missing evidence as
healthy. It does not fail by default. An agent treats a gap as a task to install
the tool or fill the config. It does not treat the gap as a passing gate.

Coverage gaps do **not** produce `agent_tasks` and do not fail by default. Use
`archfit check --require-tools` or `analyzers.<x>.gate: fail` to make them fail
with exit `1`.

## What an agent sees

- The architecture state has one `verdict` and nine `dimensions`. Each dimension
  has status, gate, confidence, denominator, and an explicit list of unmeasured
  evidence. It also has a `coverage` split. There is no repository score.
- The coupling `seams` ledger has one record per ordered module pair. Each record
  has a stable ID, score distribution, book Ch10 quadrant, and balancing
  hypothesis. Redesign the seam, not individual import edges.
- Gate findings — boundary violations (forbidden deps, internal access, layer
  inversions, cycles, unreviewed new cross-module deps).
- BC advisories — Balanced Coupling imbalances (strength × distance × volatility).
- Metrics — boundary health, modularity, structural risk, and drift. Missing
  evidence appears as `n/a`.
- Coverage gaps + config warnings — missing tools and under-specified modules.
- Structural facts — neutral per-module evidence (fan-in/out, LOC, co-change).
