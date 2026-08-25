# The AI-agent feedback loop

archfit is built to sit inside a coding agent's loop: run it after every
change, read machine-actionable output, fix, re-run. Everything in this loop
is deterministic — same repo + same config = byte-identical output.

## The loop

```text
agent edits code
  → archfit check [--base main] --json
  → exit 0?  done.
  → exit 1?  read agent_tasks[] — goal, constraints, files, validation
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
`agent_tasks[]` is gate-only. Advisory findings stay out of this channel unless
a tripped
[`coupling.gate`](configuration-reference.md#couplinggate) promotes active
`bc/imbalanced_coupling` advisories to gate kind; then the edges behind the
failing score arrive in `agent_tasks[]` with file evidence
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

## git_finding_delta — which repair tasks this change introduced

`--base <ref>` adds one report-only JSON block that sorts the CURRENT
`agent_tasks[]` by git origin. It appears only with `--base`, only in `--json`
output, and never changes the verdict or the exit code.

```json
{
  "git_finding_delta": {
    "base_ref": "main",
    "comparison_status": "comparable",
    "introduced_finding_ids": ["finding-a"],
    "pre_existing_finding_ids": ["finding-b"],
    "unknown_origin_finding_ids": [],
    "comparison_reasons": []
  }
}
```

- `introduced_finding_ids` — no matching finding on the base ref. Your change
  brought this task in; fix it before merging.
- `pre_existing_finding_ids` — the same stable finding ID was also observed on
  the base ref. Pre-existing debt, not a merge blocker.
- `unknown_origin_finding_ids` — archfit could not place the task. Treat it as
  possibly introduced.
- `comparison_status` — `unknown` when any task has unknown origin, otherwise
  `comparable`.

All three lists are sorted, non-null arrays, and every current repair task lands
in exactly one of them.

`comparison_status` reports task placement, not evidence quality, so a run can
report `comparable` alongside a non-empty `comparison_reasons`: with zero tasks
to place, or with every task matched on the base ref, no task ends up unknown
even though an analyzer failed to compare. Read `comparison_reasons` whenever it
is non-empty — a later change that adds a task would then be `unknown`.

**Conservative by construction.** A task is called `introduced` only when every
active finding-producing analyzer covered both sides equivalently. A missing or
duplicated coverage row, a timed-out analyzer, a partial from a run that did not
complete, one-sided evidence of any kind (absent, disabled, or unresolved on one
side only), or a config-hash mismatch moves unmatched tasks to
`unknown_origin_finding_ids` and names the family in `comparison_reasons`
(`"scip: head ok, base absent"`). Missing evidence never manufactures a "new"
task. The synthetic `bc/coupling_gate` task is per-run trip state with no stable
counterpart, so it is always `unknown`.

**Symmetric degradations pair, and say so.** Three shapes would otherwise make
the block permanently inert on whole classes of repos and hosts, so they stay
comparable instead:

- Both sides `partial` from unresolved import specifiers — the normal steady
  state for dependency-cruiser and grimp, so treating it as unusable disabled
  the feature on every TypeScript and Python repo.
- Both sides `partial` with every input covered and only edge precision
  degraded — `go/packages` reports this when some packages fail to type-check.
  Their imports are all in the graph; only the `go/types` strength hints are
  missing, so nothing can hide behind them.
- Both sides `absent` for an analyzer the config activated — typically its tool
  is not installed on this host (archfit's own runtime image ships no Rust
  toolchain and no SCIP indexer).

The safety argument is symmetry: neither side ran what the other could hide
behind. None is ever paired silently — each always emits a `comparison_reasons`
entry, and both partial cases carry each side's count, so `3 unresolved` and
`5000/6000 unresolved` are distinguishable, as are `2 degraded` and
`900 degraded`. The two partial shapes never pair with each other:
complete-but-imprecise is not the same evidence as incomplete. A `go/packages`
`partial` earned by packages that failed to LOAD is the incomplete kind and
pairs with nothing — see the known ceiling in [ci.md](ci.md#2-github-actions-recipe).
An **asymmetric** absence or partial is still unavailable evidence.

Matching uses stable finding IDs only: lifecycle labels (`new`, `waived`,
`baseline`) and gate-vs-advisory promotion do not affect it, and a base entry
reported as `fixed` never makes a current task pre-existing.

## advisory_tasks — the report-only rollup channel

Grouped `bc/imbalanced_coupling` advisories (`group_count > 1`) also produce
`advisory_tasks[]`. These are deterministic rollups for humans and agents to
triage advisory noise without changing CI semantics. They are never consumed by
verdict, gate status, baseline deltas, or `agent_tasks[]`.

```json
{
  "finding_id": "8a4be7…",
  "rule_id": "bc/imbalanced_coupling",
  "status": "new",
  "severity": "high",
  "group_count": 12,
  "group_members": ["8a4be7…", "9c12d0…"],
  "goal": "Review 12 same-shape Balanced-Coupling advisory edges...",
  "cheapest_move": "reduce_distance",
  "score_value": 8,
  "top_files": ["pkg/a/a.go", "pkg/b/b.go"],
  "constraints": [
    "report-only advisory; do not promote to a gate unless coupling.gate policy changes",
    "keep agent_tasks[] reserved for active gate findings"
  ],
  "validation": ["archfit check -c .archfit.yaml"]
}
```

Use `advisory_tasks[]` for review/refactor planning only. If a run exits `1`,
fix `agent_tasks[]` first; advisory tasks may be deferred unless your team has
chosen to treat the score gate as a refactoring trigger.

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
