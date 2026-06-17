# archfit agent feedback loop

archfit is built to sit inside a coding agent's loop: run it after every change,
read machine-actionable output, fix, re-run. The loop is deterministic — same
repo + same config = byte-identical output.

## The loop

```text
agent edits code
  → archfit check [--base main] --format json
  → exit 0?  done.
  → exit 1?  read agent_tasks[] — goal, constraints, files, validation
  → fix within the constraints, touching only the listed files where possible
  → run the task's validation command verbatim
  → repeat
```

## agent_tasks[] — the actionable channel

Every ACTIVE gate finding produces one structured repair task:

```json
{
  "finding_id": "8a4be7…",
  "rule_id": "no_internal_access",
  "goal": "Replace the internal-API access from pkg/a/a.go with b's public API.",
  "constraints": [
    "Use only the public API of module b",
    "public surface of module \"b\": [pkg/b/api/**]"
  ],
  "files": ["pkg/a/a.go", "pkg/b/internal/impl.go"],
  "validation": ["archfit check -c .archfit.yaml --full"]
}
```

- `goal` — deterministic template per rule type.
- `constraints` — the rule's constraint text plus allowed alternatives and the
  target module's public globs.
- `files` — the files to touch.
- `validation` — the exact command that must pass.

Advisory findings never produce tasks — they are signals, not orders.

## Status lifecycle the agent must respect

- `new` — active gate finding.
- `baseline` — accepted; do not "fix" unprompted.
- `excepted` — time-boxed waiver.
- `expired_exception` — gates again.
- `fixed` — gone since baseline.

`archfit baseline` accepts the current state; exceptions live in config with
expiry dates.

## SARIF — the CI annotation channel

`--format sarif` emits SARIF 2.1.0 (schema-validated): active gate findings as
`error`, advisories as `warning`, resolved/baselined as `note`, with file+line
locations and stable `archfit/v1` fingerprints. Metrics and the verdict ride in
`runs[0].properties`. Pipe it to GitHub code scanning for inline PR annotations.

## change_locality — the drift number

In delta mode (`--base <ref>`), `change_locality` quantifies the change's blast
surface: cross-module edges originating in changed files plus the forward graph
reach. Report-only — the `new_cross_module_dependency` rule is the gate; the
metric is the trend an agent or reviewer watches.

## What an agent sees

- Gate findings — boundary violations (forbidden deps, internal access, layer
  inversions, cycles, unreviewed new cross-module deps).
- BC advisories — Balanced Coupling imbalances (strength × distance × volatility).
- Metrics (13) — boundary health, modularity, structural risk, and drift.
- Structural facts — neutral per-module evidence (fan-in/out, LOC, co-change).
