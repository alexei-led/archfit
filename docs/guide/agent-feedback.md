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
promotes the active Balanced-Coupling advisories to gate kind, so the edges
behind the failing score arrive in `agent_tasks[]` with file evidence.

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
  volatility) at or above the configured severity.
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
