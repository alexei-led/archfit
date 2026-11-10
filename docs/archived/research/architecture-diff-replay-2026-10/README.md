# Archfit change review: replay-grounded prototype

**Archived:** 2026-11-10. This is a record of an experiment from 2026-10-31. The current plan is the
[Archfit 4 design](../../../plans/20261110-archfit-4-architecture-change-control.md), which has its own
App prototype. The [40-PR replay](replay/README.md) is the evidence worth keeping; the model-based
interpretation described below was not adopted.

Open [index.html](index.html) directly in a browser. No server, install or build is needed.
The prototype is an offline UI experiment, not a production feature.

## What changed after the replay

The first version used a fictional Pricing/Checkout example and Protect / Review / Plan scopes.
The [40-PR replay](replay/index.html) did not establish that a new architecture graph
would prevent defects better than existing review.

This version has two compact product surfaces:

1. **GitHub PR:** merge effect, one factual headline, a focused before/after diagram and one Next line.
2. **Archfit App:** the existing Findings / Graph pattern, one primary action and folded details.

[Design notes](design.html) explain the classifier separately. The normal review screens do not
show product pitches, pipeline explanations or model controls.

Protect remains the existing deterministic gate. Proactive refactoring planning is deferred.
There is no three-tier product or automatic refactoring in this prototype.

## Real cases and one synthetic failure

| Scenario | Actual replay basis | Proposed presentation |
| --- | --- | --- |
| ccgram #350 | Monitor UI dependencies moved to a callback and narrow adapter | Explain responsibility relocation; preserve lifecycle constraints |
| ccgram #341 | Private runtime-state mutation replaced by a host-supplied callback | Surface the resolved finding and the restart contract |
| ccgram #342 | Only the policy map changed; adjacent report inputs differ | Do not call increased advisory counts code erosion |
| ccgram #363 | Release notes only; zero introduced findings in the historical comment | Keep checks and repository attention, omit irrelevant advice |
| Missing base inventory | Synthetic stress case, not a historical PR | Abstain and restore evidence; do not call a model |

The displayed GitHub cards are counterfactual presentations, not the comments actually posted.
Links point to original PRs, source commits and archived comments.

The historical App results are labelled **neutral / advisory**, not recast as successful hard gates.
For #350, the code diagram uses the final integrated diff; the archived report used an older base.
The UI names this difference and does not combine those counts into one numeric improvement claim.

The existing review already explained several of these changes. The proposed value is clearer,
evidence-qualified feedback—not a verified new discovery or a claim of unique capability.

## Design

### Keep three kinds of information separate

- **Observation:** references, changed files, resolved findings, policy inputs and completeness.
- **Declared intent:** a rationale or source contract, with provenance.
- **Interpretation:** a candidate transformation and review focus; not a fact or gate decision.

The diagram shows the structural effect. An optional short Interpretation line is labelled separately
and disappears when the model is off or abstains. Counts do not determine quality. Policy-only
changes are never presented as code-quality improvements.

### Data flow

~~~text
Customer CI
  existing analyzers + Git + approved policy
                |
                v
Engine
  normalize complete two-sided facts
  identify code / policy / measurement changes
  produce exact relationship-member events
                |
       deterministic routing
        /                 \
no event / incomplete     structural candidate
policy-only                   |
        |                     v
facts / recovery       optional Jev OR Clef
                       bounded transformation label
                       + selected review focus
                              |
                       schema / label validation
                              |
App                           v
  factual template + optional interpretation
  source evidence + constrained next step
  existing gate remains independent
~~~

The engine already has much of the acquisition and comparison machinery.
Exact changed-member events, interpretation routing and this presentation are new work.
Existing grouped-finding origin does not identify each newly changed member or source location.

## How Jev/Clef would be used

These are candidates, not a selected model/version or a measured implementation.

- [Jev documentation](https://docs.typesafe.ai/introduction/coding-agents) describes typed choices and scores rather than free-form code generation.
- [Clef model card](https://huggingface.co/Cloudflare/clef) describes bounded answers and option probabilities.

These sources establish the model shape, not accuracy on architectural transformations.

### Input

A small packet prepared by the engine:

- Exact code and report comparison identities.
- Observed reference/declaration changes and source locations.
- Completeness and comparability status.
- Relevant declared intent with provenance.
- Bounded old/new source context where permitted.
- Explicit allowed transformation labels and review-focus categories.

PR prose, comments and source strings are untrusted input, not instructions.
They cannot alter approved policy, the output schema or available actions.
The production evaluator needs adversarial cases, including instructions embedded in source text.

### Output

The UI shows illustrative application-level outputs, not a vendor API schema:

~~~json
{
  "transformation": "responsibility_move",
  "review_focus": "preserve_callback_and_snapshot_contract",
  "authority": "advisory_only"
}
~~~

The classifier does not generate the review or a new refactoring plan.
A validated label selects a constrained explanation template and bounded review focus.
No probability is shown because no model has been run or calibrated.

Candidate labels include private-state-to-callback, responsibility move, contract expansion,
dependency relocation, mixed change and insufficient context.

### Three interactive model outcomes

- **Proposed label:** a hand-authored simulation shows the contextual interpretation.
- **Facts only:** no classifier interpretation; the same observations and checks remain.
- **Abstention:** insufficient context; the UI retains facts without an improvement claim.

The model selector is a prototype-only control under Test options, not a product setting on every PR.
For policy-only, no-event and incomplete-input cases, it is disabled: routing skips classification.

Production timeout, refusal, invalid schema or unsupported labels must also preserve the
factual fallback. Confidence thresholds require held-out calibration, not an arbitrary default.

### Boundaries

The classifier cannot:

- establish that a dependency exists or disappeared;
- fill missing evidence or claim behavioral equivalence;
- omit required analyzers;
- downgrade or hide a blocker;
- approve a waiver or change policy;
- treat accepted risk as a false positive;
- turn a generic coupling hypothesis into an automatic rewrite.

Source-aware inference belongs in customer CI or needs explicit additional sharing consent.
Even reports and paths can be sensitive. Normal report authentication does not independently
verify execution. Retain input, model/configuration and prompt identities for auditability.

### Prove incremental value first

Compare three conditions on held-out real PRs:

1. Current Archfit feedback.
2. Deterministic change-event grouping and templates.
3. The same events/templates with Jev or Clef labels.

Measure correct review decisions, missed consequential events, distraction, time, latency and
total operating cost. Include valid coupling, ambiguous transformations and adversarial snippets.
Keep the classifier only if it improves the deterministic baseline.

No classifier calls, human decision study or willingness-to-pay validation occurred in this prototype.

## Existing capabilities to reuse

- [Results](../../../results.md): comparison identities, findings, agent tasks and seams.
- [Evidence contracts](../../../evidence.md): completeness and comparison limits.
- [Coupling](../../../coupling.md): strength hints and classification limits.
- [Rules](../../../rules.md): rationale, alternatives and declared surfaces.
- [Agent workflow](../../../agents.md): policy lookup and constrained repair tasks.
- App: `docs/user-guide.md`, “Read the feedback” and “The graph of a pull request”.
- App: `docs/architecture.md`, customer-CI trust boundary.

Current introduced-finding highlighting is not a complete relationship diff.
This prototype does not prove that all fine-grained event facts can be exported without extending
acquisition/projection contracts.

## What not to build yet

- Another repository score or a broad autonomous architect.
- Proactive refactoring on every PR.
- A new language adapter solely to support this experiment.
- A model that suppresses deterministic findings.
- A year-long event platform before the focused review earns trust.
- New pricing tiers based on the three old prototype modes.

[Sonargraph System Diff](https://eclipse.hello2morrow.com/doc/standalone/content/examining_changes.html),
[SonarQube Architecture](https://docs.sonarsource.com/sonarqube-server/architecture-analysis) and
[CodeScene PR review](https://codescene.io/docs/guides/pr-integration/integrate-into-ci-cd.html)
already overlap. In the actual sample, Greptile also supplied diagrams and explanations.
Novelty and demand remain unproven.

## UI alignment and action contract

The prototype follows the existing App's tokens and compact components from
`archfit-app/web/src/app.css`, breadcrumb/header conventions from `Header.svelte`,
and PR-local Findings / Graph navigation from `components/PullHead.svelte`.
It uses the same light/dark palette, six-pixel box corners, small badges, green primary
buttons and folded details. Global repository pages are outside this focused prototype;
no fake navigation to unimplemented pages is added.

- **View change in Archfit:** switches from the GitHub preview to the App Findings view.
- **View evidence:** opens a read-only detail panel with source links and comparison provenance.
- **View policy diff / View release notes:** opens the real PR's files in a new tab.
- **Show recovery steps:** explains the missing evidence; it never retries a workflow itself.
- **Copy review task:** copies structured JSON; success is announced, with a manual-copy fallback.
- **For coding agents:** keeps the full proposed payload folded, including scope, constraints,
  validation, source links and the unchanged existing result.
- **Repository attention:** keeps the old findings available without making them the main PR story.

The GitHub comment uses text, a Mermaid-style diagram, links and native folds. It does not
depend on arbitrary JavaScript controls on GitHub. Custom copy and evidence-panel actions
belong to the App. No approval, waiver or automatic execution is implied by any action.

## Local behavior and verification

All CSS, SVG, JavaScript and case data are embedded.
There are no automatic network requests, provider calls, GitHub writes or persistence.
Brief and feedback dialogs are local previews. Clipboard access requires selecting **Copy brief**.
External source links load only if the viewer chooses them.

The historical [replay report](replay/README.md), [sample](replay/sample.json) and
[reproduction](replay/reproduction.json) are preserved.
Browser checks and screenshots are temporary artifacts outside the repository.
See the completion message for the final verification.
