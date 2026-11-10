# Current follow-ups

## Current plan

The active design is [Archfit 4: architecture change control](20261110-archfit-4-architecture-change-control.md).
It covers the engine, the Docker image, the GitHub Action, and the GitHub App.
Read it before you start any item below.
Items that the design replaces are marked "Superseded". Do not start them.

## About this file

This file preserves unresolved items from archived plans.
It does not assign delivery dates or treat proposals as approved work.
Check source and tests again before implementing an item.

## Recorded handoffs

The [delivered state-reporting plan](../archived/plans/architecture-state-reporting.md) explicitly records three skipped handoffs:

1. Deliver the staged dogfood configurations to the owner-controlled repositories.
2. Complete the scoped architecture review and obtain its recorded GO decision.
3. Complete the post-review baseline handoff and commit.

The old plan does not establish that these external actions occurred.
Check their status with the owners before treating them as complete.
Do not regenerate policy or baselines as part of documentation cleanup.

## Engineering review candidates

| Item | Current evidence | Next action |
| --- | --- | --- |
| Configuration application ports | Update adapters still retain plans in adapter state. | Review whether returning plans through application contracts removes a real constraint. |
| Remaining test gaps | The post-migration plan lists gaps, but newer tests cover some of them. | Check each claim against current tests before creating work. |
| Config comparison JSON | `archfit.config-compare.v1` retains scorecards and `score_delta`. Text output has no repository score. | Superseded: Archfit 4 replaces `config compare` with `check --policy` (design section 11.1). |
| Base comparison and seam gates | `--base` reports a separate comparison. Persisted baselines remain the gate reference. | Superseded: Archfit 4 removes stored baselines and compares base and head in one run (design section 7.2). |

Source: [post-migration follow-ups](../archived/plans/20260826-post-migration-followups.md).
Current behavior lives in [Results](../results.md) and [Evidence](../evidence.md).

## Proposals without a delivery commitment

- A JVM language adapter. Not part of the Archfit 4 design. .NET shipped in 3.3.0; see [the .NET support plan](../archived/plans/20261027-dotnet-support.md).
- A generic SCIP primary extractor. Superseded: Archfit 4 removes the SCIP indexers (design section 11.8).
- An MCP interface for coding agents. Not in 4.0 (design section 11.3).
- Stored-history queries through a possible `archfit history` command. Superseded: Archfit 4 keeps no stored measurements; the report is posted as an issue (design section 10.6).
- Coupling-strength constraints and additional context-map support. Superseded by the `coupling` rule (design section 6.2).
- Starter-rule inference from declared layers. Superseded by `archfit init` (design section 10.1).

These proposals come from the archived [language](../archived/plans/erosion-prevention/language-extensibility.md),
[agent](../archived/plans/erosion-prevention/agent-guardrails.md),
[tracking](../archived/plans/erosion-prevention/erosion-tracking.md),
[policy](../archived/plans/erosion-prevention/guardrail-language.md), and
[architect workflow](../archived/plans/erosion-prevention/architect-workflow.md) plans.
Existing commands and configuration fields remain the supported interface.

## External App and Action work

The old [App plan](../archived/plans/github-app.md) and
[App/Action flow](../archived/plans/erosion-prevention/app-action-flow.md) describe work in separate repositories.
This repository does not establish their current deployment or storage status.
Superseded as a roadmap: the Archfit 4 design is the plan for both repositories.
