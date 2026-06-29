# archfit as an AI-agent feedback loop — design v0.1

Date: 2026-06-11. Status: APPROVED — implementation plan
`docs/plans/20260611-archfit-completion-deterministic.md`.

## 1. Purpose

archfit's end state is a **deterministic architecture review and drift-prevention
tool for AI coding**: an AI agent (or its harness) runs archfit after every change
and receives a machine-readable feedback loop — scores, warnings, and errors across
architecture dimensions grounded in the Balanced Coupling model (integration
strength × distance × volatility) and modularity metrics. The agent uses that
feedback to stay inside the intended architecture instead of drifting.

The feedback loop has three consumer-facing channels:

1. **Exit code + verdict** — pass/warn/fail; the hard gate.
2. **Findings + metrics** — the dimensions: BC advisories, modularity metrics
   (12 today), structural facts. Already shipped.
3. **`agent_tasks` repair blocks + SARIF** — what THIS design adds: structured,
   actionable instructions an agent can execute without interpreting prose, and a
   standard format CI surfaces inline on PRs.

Everything in this document is **deterministic** and runs on the `check` gate.
The LLM layer (Tranche 2, `docs/design/hybrid-llm-strength-v0.1.md`) stays
off-gate and is designed separately.

## 2. agent_tasks — structured repair blocks (spec §13)

Today `AgentTask` is an empty placeholder; `agent_tasks` always serializes `[]`.

**Design:** one AgentTask per active gate finding (status `new` or
`expired_exception`), derived deterministically from the finding + rule config —
no LLM, no fabrication:

```go
type AgentTask struct {
    FindingID   string   `json:"finding_id"`   // joins back to findings[]
    RuleID      string   `json:"rule_id"`
    Goal        string   `json:"goal"`         // rule-type template, instantiated with edge endpoints
    Constraints []string `json:"constraints"`  // from rule def (constraint, allowed alternatives) + module public globs
    Files       []string `json:"files"`        // edge endpoints + finding locations (repo-relative)
    Validation  []string `json:"validation"`   // exact commands: "archfit check ..." + focused re-check
}
```

- **Goal templates per rule type** (e.g. `forbidden_dependency`: "Remove the
  dependency from {from} on {to}; route through {to_module}'s public API or move
  the shared code"). Templates live next to the rule registry — one source of truth.
- **Constraints** join the rule's `constraint`/`alternatives` config fields and the
  target module's `public` globs ("only these import paths are allowed").
- **Validation** always includes the reproducible gate command; deterministic order.
- Engine stage: built during diagnostic assembly from gate findings — pure function
  `agenttask.Build(findings, rulesCfg, modules) []AgentTask`.
- Advisory findings get NO agent task (they are signals, not orders).

## 3. SARIF 2.1.0 output (spec §12)

`--format sarif` renders the diagnostic as a single-run SARIF log:

- one `rule` per distinct RuleID (gate rules + `bc/imbalanced_coupling` +
  `map/staleness`), with help text from the rule definitions;
- one `result` per finding: level `error` for active gate findings, `warning` for
  advisories, `note` for fixed/excepted; locations from finding locations
  (file + line when known, file-only otherwise);
- metric results attached as run `properties` (SARIF has no metric concept;
  properties keep the scores visible to tooling that wants them);
- byte-deterministic: stable ordering (findings already ordered), no timestamps —
  `automationDetails.id` carries base/head refs instead.

New renderer `internal/output/sarif` implementing `engine.Renderer`; `sarif` joins
the `--format` enum. GitHub code scanning is the reference consumer.

## 4. change_locality metric (spec §10.4)

The drift signal for agents: did THIS change expand the architecture's blast
surface?

- **Inputs:** diff scope (changed files, already resolved by `scope`), graph,
  baseline edge set.
- **Value:** count of NEW cross-module edges introduced by changed files (edges
  whose fingerprint is absent from baseline, from-side in the diff). Display adds
  changed-module count and graph reach from changed nodes.
- **Band:** `warn` on regression vs baseline (delta), per spec; n/a in `--full`
  mode without a base ref (no diff to localize) and when no baseline exists.
- Deterministic; pure function of its CommonInput (the diff file set is
  CommonInput.ChangedFiles — engine already has scope).

## 5. Correctness + resilience fixes (gate quality)

These make the existing gate trustworthy enough to put in an agent loop:

1. **`new_cross_module_dependency` status filter** (rules.go TODO): the rule fires
   on ALL cross-module edges because `Evidence.Findings` is never wired. Fix: the
   engine passes baseline fingerprints into rule evidence; the rule emits only
   edges absent from baseline. Without this, agents get permanent false errors.
2. **SCIP single pass** (ports.go TODO(perf)): `Strengths` and `Symbols` each run
   the indexer; one `scan` indexes the repo twice. Cache the index artifact per
   (root, indexer) within a run — halves SCIP wall time; an agent-loop tool must
   be fast enough to run on every change.
3. **`explain` uses the real pipeline**: ExplainCmd currently runs Nop providers
   and empty change history, so its evidence is weaker than what produced the
   finding. Route it through `runPipeline`.
4. **gitnexus adapter**: calls `gitnexus impact --json <root>` which the real CLI
   does not provide. Fix against the real interface (per-module dependant counts
   via `gitnexus cypher`, requires a `.gitnexus` index) or remove the provider;
   no silently-dead integrations.

## 6. Explicitly deferred (out of completion scope)

- **MCP server** — revisit once the JSON contract has been stable across a release;
  the CLI + JSON/SARIF + the bundled skill are the agent integration for now.
- **GitHub Action wrapper** — packaging, not capability; `archfit check --format
sarif` in a workflow step covers CI today.
- **Plugin protocol (spec §14)** — no external demand yet.
- **`map_staleness` as a metric** — staleness ships as advisory findings; a
  duplicate metric adds noise without new signal. Spec deviation, recorded here.
