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
findings never produce tasks — they are signals, not orders.

## SARIF — the CI annotation channel

`--format sarif` emits SARIF 2.1.0 (schema-validated): active gate findings as
`error`, advisories as `warning`, resolved/baselined as `note`, with file+line
locations and stable `archfit/v1` fingerprints. Metrics and the verdict ride
in `runs[0].properties`. Pipe it to GitHub code scanning to get findings as
inline PR annotations.

## change_locality — the drift number

`change_locality` quantifies a change's blast surface: cross-module edges
originating in changed files plus the forward graph reach. It is report-only and
computes with `archfit analyze --base <ref>` — the changed-file set comes from
the git diff against `<ref>`. A plain full scan has no change to measure, so it
reports `n/a`. The `new_cross_module_dependency` rule is the actual gate an agent
watches.

## The dimensions an agent sees

- **Gate findings** — boundary violations (forbidden deps, internal access,
  layer inversions, cycles, unreviewed new cross-module deps).
- **BC advisories** — Balanced Coupling imbalances (strength × distance ×
  volatility) at or above the configured severity.
- **Metrics (13)** — boundary health (encapsulation, unbalanced_edge, cycle,
  coverage), modularity (blast_radius, change_amplification, hidden_coupling,
  structural_weight, complexity), structural risk (risk_hub,
  architecture_fitness, functional_candidates), drift (change_locality).
- **Structural facts** — neutral per-module evidence (fan-in, fan-out, LOC,
  co-change, symbol dependant count) for downstream judgment.

## Lifecycle the agent must respect

Findings carry status: `new` (gates), `baseline` (accepted — do not "fix"
unprompted), `waived` (time-boxed waiver), `expired_waiver` (gates
again), `fixed` (gone since baseline). `archfit baseline` accepts the current
state; waivers live in config with expiry dates.
