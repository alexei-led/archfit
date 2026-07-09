# archfit agent feedback loop

archfit is built to sit inside a coding agent's loop: run it after every change,
read machine-actionable output, fix, re-run. The deterministic gate is
`archfit check` — same repo + same config + same tool/cache inputs = stable
output.

## The loop

```text
agent edits code
  → archfit check --json
  → exit 0?  done.
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
  "goal": "Replace the internal-API access from pkg/a/a.go with b's public API.",
  "constraints": [
    "Use only the public API of module b",
    "public surface of module \"b\": [pkg/b/api/**]"
  ],
  "files": ["pkg/a/a.go", "pkg/b/internal/impl.go"],
  "validation": ["archfit check -c .archfit.yaml"]
}
```

- `goal` — deterministic template per rule type.
- `constraints` — the rule's constraint text plus allowed alternatives and the
  target module's public globs.
- `files` — candidate files to touch.
- `validation` — the exact command that must pass.

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
dates.

## SARIF — the CI annotation channel

`archfit check --sarif` emits SARIF 2.1.0 (schema-validated): active gate
findings as `error`, advisories as `warning`, resolved/baselined as `note`, with
file+line locations and stable `archfit/v1` fingerprints. Metrics and the
verdict ride in `runs[0].properties`. Pipe it to GitHub code scanning for inline
PR annotations.

## Scorecard delta (--base)

`archfit check --base <ref>` gates against a git ref in addition to HEAD.
`archfit analyze --base <ref>` is the report-only equivalent. Text/markdown
output adds a "CHANGE VS BASE" section. JSON/SARIF stay the normal HEAD
diagnostic — there is no separate delta schema and no finding-delta bucket
output. `--require-tools` applies exactly as without `--base`.

## Coverage gaps — missing evidence is loud, not green

A metric reading `n/a` (and a `coverage_gaps[]` entry in JSON) means an analyzer
did not run — archfit refuses to score absence as health, it does not fail by
default. Each gap carries `tool`, `install_cmd`, `affected_metrics`, and `gate`;
the `config_warnings[]` array carries under-specified-module advisories. An agent
treats a gap as "install this tool / fill this config", not as a passing gate.
Coverage gaps do **not** produce `agent_tasks` and do not fail unless the run
opts in with `archfit check --require-tools` (or `analyzers.<x>.gate: fail`),
which exits `1`.

## What an agent sees

- Gate findings — boundary violations (forbidden deps, internal access, layer
  inversions, cycles, unreviewed new cross-module deps).
- BC advisories — Balanced Coupling imbalances (strength × distance × volatility).
- Metrics — boundary health, modularity, structural risk, and drift; honestly
  `n/a` when the evidence is missing.
- Coverage gaps + config warnings — missing tools and under-specified modules.
- Structural facts — neutral per-module evidence (fan-in/out, LOC, co-change).
