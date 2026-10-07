# The AI-agent feedback loop

archfit is built to sit inside a coding agent's loop: run it after every
change, read machine-actionable output, fix, re-run. Everything in this loop
is deterministic — same repo + same config = byte-identical output.

## The loop

```text
agent asks archfit policy can-import before a new cross-module import
agent edits code
  → archfit check [--base main] --json    (or --format agent: one next_action)
  → exit 0?  healthy — done.
  → exit 2?  needs_attention — no blocking finding. Read the dimension whose
             status is not `measured`, the active diagnostic, or
             `decision.unevaluated_required_rules`. Supply the named missing
             fact; never treat yellow as a fabricated healthy zero.
  → exit 1?  blocked — read agent_tasks[] — goal, constraints, files, validation
             (empty agent_tasks[]: a required analyzer blocked; a tripped
             metric ratchet is a task, rule metric/<name>)
  → fix within the constraints
  → run the task's validation command
  → done when that run no longer lists the task's finding_id and is not blocked
  → repeat
```

Use `check` inside repair loops and CI validation. Use `analyze` to generate
reports, diffs, or a post-check narrative with `archfit analyze --ai-summary`.

## Before an edit: `archfit policy`

Two queries answer before an edit, in about 50 ms. They read the config, the
waivers, and the baseline. They run no analyzer and never read the fact cache.

```text
agent plans a new import from F to T
  → archfit policy where F            which module owns F, its layer, allowlists, rules
  → archfit policy can-import F T
  → denied?        do not add it; follow the denial's goal and constraints
  → not_decided?   add it, then run check: a cycle or seam rule may still block it
  → allowed?       an allowlist or the layer order permits it
  → unconstrained? no rule decides it; this is not permission
```

`can-import` judges the import with the same relationship analysis and rule
pass as `check`, under the same baseline and waivers. A `denied` answer names
the finding ID and the rule ID that `check` reports for the same import, and
carries the repair goal and constraints. See
[`archfit policy can-import`](commands.md#archfit-policy-can-import).

## The agent result: `--format agent`

`archfit check --format agent` writes `archfit.agent-result.v1`. It is a small
digest of the same run for a coding agent. It gives the verdict, ONE next
action, and the repairs. The exit code stays the verdict. The full run is
in `--format json`.

```json
{
  "schema_version": "archfit.agent-result.v1",
  "verdict": "blocked",
  "next_action": "repair",
  "summary": "blocked: 1 repairs in scope, 0 outside scope; 0 unevaluated required rules; 0 evidence gaps",
  "repairs": [
    {
      "finding_ids": ["ba3803eca947bf3c1b7efa8f37854d5e"],
      "rule_ids": ["no_direct_b_dependency"],
      "repair_kind": "code_change",
      "in_scope": true,
      "severity": "medium",
      "edge": { "from": "pkg/a/a.go", "to": "pkg/b", "kind": "imports" },
      "at": [{ "file": "pkg/a/a.go", "line": 3 }],
      "goal": "Remove the forbidden dependency from pkg/a/a.go on pkg/b; move shared behavior to a location permitted by the existing dependency rules.",
      "constraints": ["Remove the dependency or move the code"],
      "edit": ["pkg/a/a.go"]
    }
  ],
  "evidence_gaps": [],
  "unevaluated_rules": [],
  "omitted": { "repairs": 0, "unevaluated_rules": 0, "advisories": 1 },
  "validate": "archfit check -c .archfit.yaml --format agent"
}
```

The schema is `archfit.agent-result.schema.json` in the repository root.

**Next action.** The first matching row wins:

| `next_action`      | Condition                                                                                                   | What the agent does                                    |
| ------------------ | ----------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `repair`           | An in-scope repair needs a code change. A tripped metric ratchet is such a repair (`metric/<name>`).   | Change the code within the constraints, then run `validate` (or the command you ran). |
| `ask_owner`        | Every in-scope repair needs an owner decision, or a required rule has a selector that matches nothing.      | Stop and report to the architecture owner. Do not edit policy. |
| `restore_evidence` | A required analyzer failed its gate (`evidence_gaps`), or a required rule lacks producer evidence.          | Install or fix the analyzer, then run `validate` (or the command you ran). |
| `report_blocked`   | The verdict is `blocked`, but no repair is in scope.                                                        | Report the blockers. They are not from this change.    |
| `none`             | Nothing to do.                                                                                              | Finish. Exit 2 with `none` is a correct finish.        |

A `blocked` verdict never gives `none`.

**Repairs.** archfit groups the active gate tasks by edge (`from`, `to`,
`kind`). One import that breaks two rules gives one repair with two finding
IDs. `repair_kind` is `code_change` when any grouped task needs a code change.
A finding that names no dependency between two different endpoints (a
`public_api_*` finding names its module on both sides) is its own repair.
`goal` is the goal of the first code-change task. `constraints` holds the
distinct constraints of all grouped tasks. `at` lists the source locations.
`edit` lists the task files on the source side of the edge: the import sites
and the importing file. The target of a forbidden edge is never a file to
edit. When the edge has no location and its source node is not a file (a
Rust `crate::mod` edge), archfit cannot tell the sides apart, and `edit` is
empty: `edge.from` names the source. A finding that names only a module pair
(a seam-gate finding) lists every task file, because either module can
change. Repairs are sorted: in scope first, then code
changes, then severity, then the lowest finding ID.

**Scope.** Without `--base`, every repair is in scope. With `--base <ref>`,
a repair is outside the scope only when every grouped finding has the origin
`pre_existing`. An `introduced` or `unknown` origin is in scope, so a
comparison that cannot place a task never hides a blocker. To scope the result
to your uncommitted edits, run `archfit check --format agent --base HEAD`.

**Budget.** The result is one line of JSON of at most 8 KB. Every free-text
string is at most 400 characters. When the result is larger than 8 KB,
archfit cuts free text to 200 characters and each repair's `at`, `edit`, and
`constraints` to 5 entries, and counts the cut entries in the repair's
`at_omitted`, `edit_omitted`, and `constraints_omitted`. Then it moves tail
repairs into `omitted.repairs` and tail unevaluated rules into
`omitted.unevaluated_rules`. It always keeps the header and the first repair,
and it sets `"truncated": true`. No ID, path, edge, or `validate` command is
shortened. `next_action` and `summary` are decided before the cut, so the
budget never changes them. Advisories are never listed; `omitted.advisories`
counts the active ones.

**Validate.** `validate` is the run's validation command with
`--format agent`, read from the repair or advisory tasks. It is absent when
the run has neither: then re-run the command that produced the result.

## Hooks, instructions, and the skill

**Claude Code Stop hook.** `archfit hook claude` reads the `Stop` or
`SubagentStop` event on stdin and resolves `--config` against the event `cwd`.
Register it in `.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [{ "hooks": [{ "type": "command", "command": "archfit hook claude" }] }],
    "SubagentStop": [{ "hooks": [{ "type": "command", "command": "archfit hook claude" }] }]
  }
}
```

A clean working tree (`git status --porcelain` lists nothing but archfit's own
`.archfit-cache`) skips the run.
Otherwise the hook runs `check --format agent --base HEAD` in process. The
`--base` ref scopes the result to the uncommitted change: a repair whose
findings all exist at `HEAD` is outside the scope.

| Result                                    | Hook output                                                                |
| ----------------------------------------- | -------------------------------------------------------------------------- |
| `repair` or `ask_owner` with an in-scope repair, first stop | Exit 2. Stderr holds the IDs, `file:line`, goal, constraints, and `validate`. |
| The same, with `stop_hook_active: true`   | Exit 0 with a `systemMessage`. The hook blocks at most once.               |
| Any other action except `none`            | Exit 0 with a `systemMessage`. A dead selector or an unmeasured ratchet is not scoped to the change, so it never blocks a stop. |
| `none`                                    | Exit 0, silent.                                                            |
| An archfit error                          | Exit 0 with a `systemMessage`. The hook fails open.                        |
| Malformed stdin                           | Exit 1.                                                                    |
| Any other event                           | Exit 0.                                                                    |

Exit 2 is the Claude Code protocol for "do not stop yet", not the engine verdict.

**Git pre-commit hook.** `archfit hook git` exits 1 with the repair on stderr
when the next action is `repair` or `ask_owner` and a repair is in scope, 0
otherwise, and 3 when archfit cannot run. It judges the staged content, which
is what the commit will contain: unstaged edits and untracked files do not
count. It checks the index out in a temporary worktree and leaves the working
tree, the index, and the refs as they are (see
[`archfit hook git`](commands.md#archfit-hook-claude--archfit-hook-git)). The
repository publishes it for pre-commit:

```yaml
- repo: https://github.com/alexei-led/archfit
  rev: v2.5.0
  hooks:
    - id: archfit
```

It needs `archfit` on `PATH` (`language: system`).

**AGENTS.md block.** `archfit agents-md --write` renders one block between
`<!-- archfit:start -->` and `<!-- archfit:end -->` in `AGENTS.md` (`--file`
for another file): the agent loop, the module table, and the rules that block
as sentences, grouped by the code they constrain. Text outside the markers
stays byte-identical, and a second write changes nothing. Run
`archfit agents-md --check` in CI: it exits 1 when the block is missing or out
of date.

**Skill.** `archfit skill install` writes the archfit agent skill that ships in
the binary to `.claude/skills/archfit`. It refuses to overwrite a local change
without `--force`.

## The output has a published JSON Schema

`archfit.state.schema.json` (repo root) describes the
`archfit.architecture-state.v1` document that `--format json` emits. Validate
against it, or generate your consumer types from it, instead of hand-modelling
the output. Note that `archfit.schema.json` is a different contract — that one
describes `.archfit.yaml`.

Nullable by contract, not by accident: `seams[].scores.p10` and `p90` are `null`
when a seam has fewer than ten scored edges. A percentile nobody can compute is
reported as absent, never as `0`.

Free text is single-line and bounded by contract. Every reason, `why`,
`constraint`, allowed alternative, dimension basis, and unknown-fact text is one
line of at most 400 characters; an agent task's `goal` and `constraints` are at
most 3600. Analyzer output reaches some of these fields, for example the reason
of a `dependency-cruiser` row that exited non-zero. archfit collapses its line
breaks, tabs, and colour codes into single spaces and keeps the leading text,
which holds the tool, the exit code, and the first error line. Text cut at the
limit ends in `…`. The full analyzer output is printed on stderr as a warning,
never in the report. Text that was already one short line is published
unchanged.

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
constraint text (with the rule's `docs` reference, when declared), the rule's
`rationale` as `rationale: <text>`, each declared alternative as
`allowed alternative: <text>`, plus the target module's public globs, except on
`forbidden_dependency`, `forbidden_layer_direction`, `cycle`, `module_cycle`,
`module_dependencies`, and `new_cross_module_dependency` tasks, which never
list the target's public surface: routing through it keeps the dependency
forbidden or outside the module allowlist, the layer inverted, the cycle
closed, or the cross-module dependency new. A `module_dependencies` task tells
the agent to remove the import and to leave an allowlist change to the
architecture owner.
Validation is the exact `archfit check` command to re-run. The repair is done
when that run no longer lists the task's `finding_id` and its verdict is not
`blocked`; exit 2 can remain and is not a failed repair. One import that breaks
two rules is two findings and therefore two tasks, keyed by their finding IDs.
The command replays the effective analysis flags (`--base`, repeated `--lang`,
and `--require-tools`) so the repair is checked under the same conditions.
`--refresh` is deliberately not serialized: cache-control must not change the
validation result. `--no-advisories` and output-format flags are not part of
the validation contract.
With `--base`, each current finding also carries `origin`: `introduced`,
`pre_existing`, or `unknown`. Each task copies the origin of its finding.
`unknown` means analyzer evidence was asymmetric;
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
source, then the target, module's `paths:` root. Seam-gate and module-cycle
tasks carry no `declarations`: their files span many import sites, and the
declarations in them would bury the import to cut.

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

## Origin with `--base`

`--base <ref>` classifies every current finding in canonical JSON. One
classifier serves findings, repair tasks, and SARIF results. It does not create
a second list or a separate delta schema:

```json
{
  "comparison": {
    "origin_status": "comparable",
    "introduced_finding_ids": ["finding-a"],
    "resolved_finding_ids": ["finding-c"]
  },
  "findings": [
    { "id": "finding-a", "origin": "introduced" },
    { "id": "finding-b", "origin": "pre_existing" }
  ],
  "agent_tasks": [
    { "finding_id": "finding-a", "origin": "introduced" },
    { "finding_id": "finding-b", "origin": "pre_existing" }
  ]
}
```

- `introduced` — the base run did not observe the finding, and every active
  finding-producing analyzer family had comparable evidence.
- `pre_existing` — the base run observed the same stable finding ID. This holds
  even when the evidence is degraded.
- `unknown` — evidence could not place the finding. Treat it as possibly
  introduced; missing evidence never manufactures an `introduced` result.
- `findings[].origin` is present only with `--base`. A `fixed` finding never
  carries it. `agent_tasks[].origin` is a copy of the origin of its finding.
- `comparison.origin_status` is `comparable` when the analyzer evidence of both
  runs pairs. Otherwise it is `unknown`. Read `origin_reasons` even when the
  status is `comparable`. It names each degraded or differing analyzer, and an
  analyzer difference can still be relevant to the next change. The key is
  absent when there is nothing to name.
- `comparison.introduced_finding_ids` lists the findings the change added.
  `comparison.resolved_finding_ids` lists the base findings the change removed.
  Both keys are present (possibly `[]`) exactly when `--base` ran. When
  `origin_status` is `unknown`, `resolved_finding_ids` is empty, because a
  vanished finding then proves nothing.

Two findings use a different match rule:

- A `bc/imbalanced_coupling` rollup is matched by its member edges, not by its
  representative ID. It is `pre_existing` only when every member edge existed in
  the base run. One new member edge makes it `introduced`.
- A `bc/coupling_gate` finding is `pre_existing` when its module pair was a
  qualifying distributed-monolith seam in the base run. Otherwise it follows
  the general rule: `introduced` when the evidence pairs, else `unknown`.

A missing or duplicated coverage row, timeout, unfinished partial run, or
one-sided analyzer evidence makes an unmatched finding `unknown` and names the
reason. The config hash is not checked, because both sides read one config file.

The run publishes a `comparison.measurement_profile` with a profile version,
settings hash, and the producer/tool versions and statuses that supplied the
evidence. A missing, unknown, or incompatible profile makes `comparison.status`
`non_comparable`. It does not make origins `unknown`. Both sides run one binary
over one config file, so a profile difference comes from the trees (for example
a `toolchain` line in `go.mod` or a `tsconfig` change). It is part of the
change. The sides still pair. Each differing producer tool version, semantics
version, and partial basis, and a differing settings hash, is named in
`origin_reasons`. The reference and
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
stable finding IDs, with the rollup and seam rules above; lifecycle labels and
gate-versus-advisory promotion do not change origin, and a base finding reported
as fixed does not make a current finding pre-existing. Origin remains
presentation only: it never changes the verdict, a gate, the exit code, or the
baseline.

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

Each result also carries the SARIF baseline fields:

| Field | Value |
| --- | --- |
| `baselineState` | Written only when the run loaded a baseline file (`gate_reference.baseline_present`). `unchanged` for a baselined finding, `absent` for a baseline entry that this run no longer sees, `new` for every other finding. A waived finding is `new`: a waiver accepts a finding, it does not put it in the baseline. archfit never writes `updated`. |
| `suppressions` | One `{kind: external, status: accepted}` entry for a baselined or a waived finding. An expired waiver accepts nothing, so its finding has none. |
| `partialFingerprints.primaryLocationLineHash` | The finding ID. GitHub code scanning matches alerts across runs on this key, so an alert does not move when the code around it moves. |

GitHub code scanning ignores `baselineState` and `suppressions`: it tracks new
and fixed alerts per branch itself. Other SARIF consumers, such as the VS Code
SARIF viewer, read them.

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
code change to make. A selector that matches only source no dependency
producer analyses (its language's extractor finds no project under the
analysis root) is listed with the reason
`selector matches only source no dependency producer analyses: <from|to> <glob>`:
the fix is to analyse that source, not to edit the selector. A declared guard
(`guard: true`) is not listed while its selector matches nothing, whatever
state the dependency producer is in. Once the guarded path exists again the
guard is an ordinary rule and is listed when its producer evidence is
incomplete.
