# Archfit 4.1: declared surfaces and suggestions

Date: 2026-11-17. Status: proposed, revision 2.
Scope: the engine and CLI (`archfit`), its Docker image, the GitHub Action (`archfit-action`), the
GitHub App (`archfit-app`), and the three onboarded repositories (`archfit`, `pumba`, `ccgram`).

Reader: the owner, the manager agent, and the worker agents that build this release.
This document is the design and the delivery plan in one place. It extends
[Archfit 4: architecture change control](20261110-archfit-4-architecture-change-control.md)
("the 4.0 design"). For the topics it names, this document wins: the `coupling` rule (4.0 design
6.2), the three kinds of truth (6.5), the engine's classification (11.2), the contracts (11.7), and
milestones 4 and 5 (16). Everything else in the 4.0 design stays.

Backward compatibility is not a goal. There are no external users. The owner migrates the three
repositories in the release window ([section 9](#9-release-window-and-migration)).

## 1. Summary

Archfit 4.0 gates a change with a few declared facts: the modules, their public part, their
volatility, and the allowed dependencies. The report then relabels the same dependencies with a
type-shape guess, so one dependency has two balances. The policy holds exceptions without dates.
A model that judges code has no place to put its answer.

Archfit 4.1 does six things:

1. **One classification rule.** The declared surface of the target decides the strength of a
   dependency, in the check, in `can`, in a request, in the report, and in the App. A balance always
   shows its inputs.
2. **The policy speaks three strengths.** A module declares its contract part (`public`) and its
   shared model part (`model`). A use of anything else is intrusive. Copies stay the `duplication`
   rule. The names and the volatility follow the book.
3. **Suggestions for the owner.** The engine lists what the policy leaves undecided or what the code
   contradicts: an exception without a date, a surface nobody declared, a package used past the
   surface. The owner applies a suggestion in the App with a passkey. A worker never sees one.
4. **Judges suggest; they never decide.** A classifier model (Jev, Clef) answers narrow questions
   about a surface or a pair of fragments. Its answer becomes a suggestion with a probability. It
   never sets a strength, a balance, or a verdict. A judge speaks only after it passes a measured
   bar on labelled real code.
5. **Measured before released.** A labelled gold set scores the current classifier and each judge.
   The trap benchmark runs on frontier models. Every changed number on the three repositories is
   explained.
6. **A worker always has a legal next step.** A denial names the declared surface and offers one
   question: should this package be part of the surface ([6.9](#69-the-workers-side))?

## 2. What was measured

Measurements of 2026-11-17 unless a row says otherwise. [Appendix A](#appendix-a-how-to-reproduce)
gives the commands.

| Observation | Evidence |
| --- | --- |
| The report relabels most dependencies with a type-shape guess. | `archfit report` on `archfit` at `081a21b0`: 67 module pairs, 440 edges. Strength: model 335 edges (76%), contract 64, functional 41. None abstained. "Model" comes from the Go hint "concrete non-DTO type or method on a concrete receiver". |
| One dependency has two balances. | `analysis-application -> architecture-policy`: `archfit can` prints `9/10 (min 7)`; the report scores all 10 edges of the pair 7. |
| The gate does not read the relabel. | `internal/assessment/change/judge.go`: a use of the public part is `contract`, a use past it is `functional`. No type fact is read. No verdict changes between 9 and 7 at `min_balance: 7`. |
| The policy holds undated exceptions. | `archfit` `.archfit.yaml`: 30 exceptions (14 `coupling`, 14 `duplication`, 2 `cycle`), none with `expires`. |
| A coupling exception is an undecided surface. | `assessment-repair` declares `public: [internal/assessment/finding]`. Other modules import `evaluation`, `result`, `score`, `change` and `count`. The surface or the code is wrong. The tool offers only an exception. |
| The book review ended at two findings that shape cannot fix. | Book-alignment review of 2026-11-16, findings 3 and 4: a public interface, a DTO or a concrete method does not prove contract or model; "past the public part" is intrusive in the book, not functional. Both need a declaration or a judgment. |
| The behaviour claim has no frontier measurement. | Trap gate on the 4.0.0 release build, 2026-11-15: one model (`deepseek-flash`), 10 runs. Without hooks 4 of 4 runs kept the forbidden import. With hooks 4 of 4 asked. Two of three targets missed. |
| A classifier can answer a surface question fast. | Feasibility probe ([Appendix B](#appendix-b-jev-feasibility-probe)): `jev-1.14.0`, five packages, about 200 ms each, graded probabilities, repeat drift up to 0.04. It chose `model` for all five and never `internal`. Five answers prove nothing about accuracy. |
| Judges disagree without a rubric and labels. | Project notes of June 2026 (not re-run): two capable models disagreed on 11 of 56 strength labels and on about half of the subdomain labels. |
| The App has the parts a suggestion needs. | Code map of 2026-11-17: the Action runs only `archfit check`; the report is not transported; the App re-reads the result from the run artifact; a generic policy edit path exists and always asks for a passkey; no page shows a strength word. A task that touches these facts verifies them first. |
| The book's volatility for an intrusive use changes no verdict on `archfit`. | Of the 14 coupling exceptions, 0 change status when the target's volatility is used; one balance moves from 2 to 5. |

Limits: one repository measured for the report numbers. The local `pumba` checkout was stale and
`ccgram` was not run. Milestone 0 repeats the measurement on all three.

## 3. Goals

1. One dependency has one strength and one balance in every tool, with the inputs shown.
2. Every strength comes from a declaration. Its origin (declared or assumed) is visible.
3. The policy uses the book's words: contract, model, intrusive, and a copy as symmetric coupling.
4. The owner gets a short list of decisions the policy still needs, each with one action.
5. A model's judgment helps the owner decide. It is measured, labelled, and never authoritative.
6. No exception without a date remains in the three repositories after the migration.
7. The release reaches the engine, the image, the Action, the App, and the three repositories in
   one window, with a rollback at each step.

### Non-goals

- A model in the check. The check stays deterministic and reads no judge.
- A model that drafts `volatility` or `subdomain`. Earlier results were at chance level, and the
  input is business knowledge the code does not hold.
- A new distance scale. D=9 for a module boundary stays as a disclosed adaptation.
- Inferred volatility as a score input. The unreachable cascade is removed
  ([6.3](#63-evidence-rules)).
- New languages, an MCP server, organisation features, a stored baseline.

## 4. Principles

All principles of the 4.0 design stay. New:

- **A declaration decides. An observation and a judgment suggest.** Types the code uses and answers
  a model gives never change a strength. They produce a suggestion the owner can apply.
- **One rule, one number, with its inputs.** A balance is never shown without its strength and the
  volatility that applied.
- **A judge speaks only after it is measured.** Each judge model version has a recorded score on
  the gold set. An unmeasured version produces no suggestion.
- **A secret is never in a container that holds the repository.** The step that reads the code has
  no key. The step that has the key has no code.
- **Source leaves the workflow only when the protected policy says so**, and only to the named
  judge. The App never receives source.

## 5. Words

Additions and changes to the word table of the 4.0 design (section 5).

| Word | Meaning | Not used |
| --- | --- | --- |
| surface | What a module offers to other modules: its public part and its model part. | API, interface inventory |
| public part | The contract: files made for other modules. Policy key `public`. | — |
| model part | The module's own model, shared on purpose. Policy key `model`. | shared kernel, types |
| contract, model, intrusive | The strength of a dependency: a use of the public part, of the model part, or of anything else. | functional (for an import) |
| copy | Knowledge two modules both hold. The `duplication` rule. In the book: symmetric functional coupling. | clone |
| suggestion | A policy decision the engine proposes to the owner, with what it saw. | proposal (the App uses it for the policy pull request), finding, advice |
| judge | A classifier model that answers one closed question with a probability. | LLM, AI, assistant |
| judged | The origin of a value a judge gave. Beside: declared, assumed, seen. | inferred, detected |
| declared, assumed, seen | The origin of a value: written in the policy, a default, read from the code. | asserted, assessed, observed (in user text) |

The report JSON keeps the origin values of report v2 (`asserted`, `assumed`, `observed`) and adds
`judged`. The text record and the App use the words above.

## 6. Design

### 6.1 One classification rule

The strength of a dependency between two modules comes from where its target is declared:

| The target is in | Strength | Volatility that applies | `low` | `medium` | `high` |
| --- | --- | --- | ---: | ---: | ---: |
| the public part (`public`) | contract (1) | the target's | 9 | 9 | 9 |
| the model part (`model`) | model (3) | the target's | 8 | 7 | 7 |
| neither: past the surface | intrusive (10) | the target's | 8 | 5 ✗ | 2 ✗ |
| a copy (`duplication`) | symmetric (9) | the higher of the two | 8 | 5 ✗ | 1 ✗ |

The numbers are the book formula at D=9, `max(|S − D|, 10 − V) + 1`, with `min_balance: 7`.

Policy:

```yaml
modules:
  assessment-repair:
    paths: [internal/assessment/**]
    public: [internal/assessment/finding]   # contract
    model: [internal/assessment/result]     # shared model
    volatility: high
```

- `model` is a list of globs inside the module's `paths`. A path in both `public` and `model` is a
  policy error.
- When a module declares `public` or `model`, its surface is exactly what the two keys list.
- When it declares neither, the whole module is its public part, as in 4.0. The origin is
  `assumed`, and the owner gets a suggestion ([6.4](#64-suggestions)).
- `min_balance: 8` gets a meaning the owner can choose: no model coupling to a module that is not
  `low`.

One function in the relationship layer returns the strength for a declared role. The check, `can`,
the request preview and the report call it. Nothing else sets a strength.

What changes against 4.0:

| 4.0 | 4.1 | Why |
| --- | --- | --- |
| The gate calls a use past the public part `functional`. | `intrusive`. | The book: intrusive coupling uses what was not made for integration. Functional coupling is shared business responsibility; an import does not show it. The balance at D=9 is the same number. |
| A use past the public part takes the higher volatility of the two modules. | The target's. | The book: the upstream module's volatility decides. The 4.0 design already said "only into a module declared `low`" in words while its table counted the source too. |
| The report raises a public use to `model` from Go type facts, and labels an unmatched call `functional` from a hint. | Type facts are evidence. They never set a strength. | The review found no shape that proves contract or model. |
| A balance is a bare number. | `balance: 7/10 (min 7): model, architecture-policy is high`. | The number explains itself in the record, `can`, a request, SARIF, and the App. |

The check still reads only imports, globs and declared volatility. It needs no type analysis.

Limit: a use of a member the language itself keeps private (a C# `internal` member through
`InternalsVisibleTo`, a Python private name) inside a public file is listed as evidence. In 4.1 it
does not change the balance, because the check cannot see it and one dependency must have one
number.

### 6.2 The two balances of 4.0, explained

Take `internal/application/analysis.go`, which imports `internal/policy`. The target is in the
public part of `architecture-policy`, declared `high`.

- The gate, `can` and a request use the declared surface: contract, S=1.
  `max(|1 − 9|, 10 − 10) + 1 = 9`.
- The report starts from the same surface and then reads Go types. The caller uses a concrete
  struct, so the report raises the strength to model, S=3. `max(|3 − 9|, 0) + 1 = 7`.

Both pass at `min_balance: 7`, so no verdict differs. Three things are wrong all the same. The owner
approves a request at 9 and later reads 7 for the same edge. The 7 rests on a guess: in Go nearly
every use of another package touches a concrete type, so 76% of the edges of `archfit` became
"model". And the 9 is also a claim nobody checked, only declared.

4.1 keeps the declaration as the one source and makes the claim explicit. The owner says which
part of a surface is a contract and which is the shared model. Type facts and a judge can question
that declaration through a suggestion. After the migration the pair above scores 9 if
`internal/policy` stays `public`, or 7 if the owner moves it to `model`, in every tool.

### 6.3 Evidence rules

- **An assumed value never qualifies a report finding.** A volatility nobody declared still scores
  as `high`, the safe side for the gate. It does not make a pair a distributed-monolith measurement
  and does not count as rated volatility. It produces a suggestion. The regression must cover seam
  counts, dimension status and confidence, `unbalanced_edge`, hypotheses, and the capped repository
  value, as the review listed. Equal edge balances are not enough.
- **The volatility cascade is removed.** The review traced that no schema 3 policy can turn it on,
  while the documents still offer it. Task 1.4 confirms this, then the code, its tests and the
  claims go. The cascade flag is part of the documented classification hash, so that recipe in
  `docs/evidence.md` changes with it. Inferred volatility can return later as a
  suggestion; it will not change a score silently.
- **Adapted values are called adapted.** D=9 for every module boundary and the `medium` ordinal are
  adaptations the book allows. The code comments and documents stop calling them verbatim.
- **Connascence stays report-only evidence**, as PR #212 left it.

### 6.4 Suggestions

A suggestion is one policy decision for the owner. It has a kind, a subject, what the engine saw,
at most one edit, and the effect of that edit.

```text
suggestion: surface: internal/assessment/score is used past the surface of assessment-repair
  seen: 3 modules import it; 3 exceptions cover it, none has a date
  judged: model 0.91 (jev-1.14.0, rubric 1)
  edit: add internal/assessment/score to modules.assessment-repair.model
  effect: loosens
```

| Kind | When | Edit | Source |
| --- | --- | --- | --- |
| `exception-date` | An exception has no `expires`. | Set a date (the owner chooses it). | Policy |
| `surface-missing` | A module declares no surface and other modules use it. | Set `public` to the packages they use today. | Imports |
| `surface-used` | Modules use a package past a declared surface. | Add it to `public` or `model`; or date the exception. With a judge, the judged role picks the first option. | Imports, exceptions, judge |
| `volatility-missing` | A module's volatility is assumed. | Set `volatility` (the owner chooses it). | Policy |
| `surface-role` | A judge rates a declared public package as the module's model. | Move it from `public` to `model`. | Judge |
| `copy` | A judge rates two near-identical literals in two modules as the same rule. | None. The owner opens an issue or dismisses. | Literals, judge |

Rules:

- The first four kinds need no judge. They come from the same facts the check reads, and they are
  part of the result on a push to the default branch.
- `surface-role` and `copy` exist only when a judge passed the bar for that question
  ([7.1](#71-gate-g1-the-judge-bar)).
- A suggestion has a stable key: kind, module or pair, and path. The same input gives the same key.
- Suggestions go to the owner: `archfit report`, `archfit suggest`, and the App. They are never in
  the record of a pull request, and they never change a status. There is still no warning on `pass`.
- A suggestion text is built from templates and facts. No text written by a model is shown.
- The engine never applies an edit. The App applies it through its policy pull request.

### 6.5 Judges

**Authority** (owner decision, 2026-11-17): a judged value is a suggestion. It is never an input
to strength, distance, volatility, balance or status. When the owner applies a suggestion, the
policy declares the value and the origin becomes `declared`.

**Questions.** Closed questions, one answer each, with a probability for every choice:

| Question | Unit | Choices | Feeds |
| --- | --- | --- | --- |
| `surface-role` | One package or file that other modules import, with its exported declarations and its importers | `contract`, `model`, `internal`, `uncertain` | `surface-used`, `surface-role` |
| `same-rule` | Two near-identical literals in two modules, each with its enclosing function | `same`, `different`, `uncertain` | `copy` |

The rubric of each question is text in the engine with a version. It paraphrases the book
(chapters 7 and 13) and is the same text the gold set was labelled with.

**Judges.** Jev (typesafe.ai) and Clef, Clef Flash, Clef Omni (Cloudflare Workers AI) speak one
protocol: a typed state, named questions of type `choice`, and an answer with a choice and
probabilities. The engine ships one adapter for that protocol. Frontier models label the gold set
and set the ceiling in the bake-off; they are not a product judge in 4.1.

**Policy.**

```yaml
judge: jev        # jev, clef, clef-flash, clef-omni, or absent
```

The key is in the protected policy, so only an owner turns a judge on. The credential comes from
the environment (`TYPESAFE_API_KEY`, or `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID`).

**Thresholds.** The engine holds a table: judge model version, question, choice, the probability
needed to emit a suggestion. The table is filled from the bake-off. A model version without a row
gives no suggestion and one `unknown` item, "judge version not measured". Probabilities of
different judges are not comparable, so each has its own rows.

**Stability.** Answers are cached by judge version, rubric version and the hash of the question.
An unchanged question is not asked again. The probe showed repeat drift up to 0.04 without a
cache, so a suggestion near the threshold would otherwise appear and vanish.

**Failure.** A judge that is absent, slow or wrong in format gives an `unknown` item in the
suggestions and nothing else. Missing evidence is not zero, and it is not a suggestion either.

**What leaves the workflow.** Per question: paths, exported declaration signatures, importer
paths, and for `same-rule` the two literals and their functions. Each question has a size cap.
Paths the snapshot helper treats as secret are never read. Nothing goes to the App except paths,
kinds, choices and probabilities.

**Two steps, two containers.**

```text
archfit suggest --questions q.json     # repository mounted, no key, no network needed
archfit suggest --answer q.json        # no repository, key in the environment
```

The first step writes the questions with the edit each answer would produce. The second step asks
the judge and writes `archfit.suggestions.v1`. It is a pure function of the questions and the
answers. On a laptop `archfit suggest` runs both in one process. `suggest` is the eighth command;
it is for owners.

An architecture test forbids every package of the check path to import the judge packages.

### 6.6 Contracts

| Contract | Change |
| --- | --- |
| Policy schema 3 | New keys `modules.<name>.model` and `judge`. The version stays 3. A 4.0 engine rejects the new keys, so a repository moves its Action pin before its policy uses them. |
| `archfit.result.v2` | Replaces v1. `balance` gains `strength` and `volatility`. The `repository` block gains `suggestions` (the four kinds that need no judge). |
| `archfit.architecture-state.v2` | Not released yet (PR #213); amended in place. The seam strength follows 6.1; type facts stay under `evidence`; `suggestions` is added; origin `judged` is added. |
| `archfit.questions.v1`, `archfit.suggestions.v1` | New. The file between the two `suggest` steps, and its output. |
| Upload envelope 2 | Unchanged. The body is result v2. |
| Run artifacts | `archfit-result` as today. New: `archfit-suggestions`, on a push to the default branch when a judge is on. |
| App manifest | A new entry: engine 4.1.0, its two image digests, the Action commit. |
| Text record | The `balance:` line carries its inputs. New `surface:` line on a coupling error (6.9). New `suggestion:` record for the report and `suggest`. |

Old and new result versions are not accepted side by side. The window in section 9 keeps that gap
short.

### 6.7 Components

**Engine.** One strength function in `internal/relationship`; the check (`internal/assessment/change`)
and the report classifier both use it. `internal/policy/resolved` gains the surface roles.
Suggestions without a judge are assessment results over check facts. The judge is a port with one
adapter; `suggest` is an application use case wired in `cmd/archfit`. Hint-based strength, the
unreachable cascade and the leftover `internal:` glob path are deleted. Layer rules of
`docs/architecture.md` stay; run the impact analysis the repository rules ask for before each edit.
The engine and the App already use the name `judge` in code for the verdict logic. Task 0.2 picks
package names that do not collide; the user-facing word stays "judge".

**Image.** No new tool. The image is rebuilt for 4.1.0 and its digests go to the Action and the App.

**Action.** Pins engine 4.1.0 by digest and vendors the result v2 schema. On a push to the default
branch, when the policy names a judge and the new input `judge-key` is set, it runs the two
`suggest` steps and uploads the `archfit-suggestions` artifact. The key goes only to the second
container, which has no checkout. Pull-request and merge-queue runs never run `suggest`. The rule
"no environment is forwarded to the engine" stays true for every container that holds the code.

**App.**

- Reads result v2 and the new policy keys; lists engine 4.1.0 in the manifest.
- Request and pull-request pages show the balance with its inputs. The policy page shows each
  module's public and model part with the origin (declared or assumed).
- Home shows suggestions under "needs the owner": the ones in the result's repository block, and
  the judged ones read from the `archfit-suggestions` artifact of the same run.
- "Apply" turns the selected suggestions into one policy edit, shows the preview, asks for the
  passkey, and uses the existing policy pull request. A suggestion without a complete edit opens the
  policy editor at its place. "Dismiss" stores the suggestion key so it does not return.
- The request page shows the matching `surface-used` suggestion and offers the surface answers
  ([6.9](#69-the-workers-side)).
- The words test of the 4.0 design covers the new words.

### 6.8 Book alignment ledger

This closes the review of 2026-11-16. Each finding has one disposition.

| Finding | Disposition |
| --- | --- |
| Go object kinds do not establish algorithm or meaning connascence. | Fixed, PR #212. |
| A default `high` volatility was reported as declared. | Fixed in the report by PR #213 (origins). 6.3 adds the rule that an assumed value never qualifies a finding. |
| Public, DTO or concrete-method shape does not prove contract or model. | Fixed by design: 6.1 removes shape from strength; the owner declares; a measured judge suggests. |
| "Past the public part" was labelled functional and took the worse volatility. | Fixed: intrusive, the target's volatility (6.1). Copies remain the only functional coupling Archfit claims. |
| The inferred-volatility cascade was documented but unreachable. | Removed (6.3). |
| D=9 for every boundary loses hierarchy and ownership. | Justified adaptation. The wording changes; the scale does not. |
| Seam provenance names the target even when the source drives the volatility. | Gone with 6.1: the target's volatility applies to every import. |
| Synchronous calls, churn as volatility, connascence as strength. | Rejected by the review. No change. |
| Functional coupling without a copy, runtime coupling, published-language coupling between services. | Not measurable from the facts Archfit reads. Stated in `docs/coupling.md`. |

### 6.9 The worker's side

Added in revision 2. A rule prevents drift only when the worker that hits it has a legal next step
that is shorter than the illegal one. Output of the engine at `081a21b0`:

```text
$ archfit can internal/application/analysis.go internal/assessment/score
analysis-application -> assessment-repair: coupling: reaches past the public part
  balance: 2/10 (min 7)
  next: fix the code
```

The record does not say what the module offers. `coupling` is a `deny` rule, so the worker cannot
ask for the surface to change. `duplication` is an `ask` rule, so the worker can ask for a copy.
The short path is the copy, which is the thing the tool exists to prevent.

4.1 changes four things:

- **The record names the surface.** A `coupling` error for a use past the surface carries a
  `surface:` line with the declared public and model part of the target. It states a declared
  fact. The engine still does not suggest where to move code (4.0 design 7.2).
- **`archfit rules` lists surfaces.** For each allowed dependency of the file's module, `rules`
  lists the surface with its roles.
- **The surface request.** A use past the surface may ask one question: should this package be
  part of the surface? `coupling` stays `deny` for everything else. An unbalanced coupling is
  never approved as such; only the declaration can change.
- **Decide once.** The request page shows the matching `surface-used` suggestion. The answers
  are: declare as `public`, declare as `model`, allow until a date, or reject with `instead`.
  A second request for the same package joins the first, and a rejection answers later requests,
  as in the 4.0 design (10.3, 6.3).

```text
$ archfit can internal/application/analysis.go internal/assessment/score
analysis-application -> assessment-repair: coupling: internal/assessment/score is past the surface
  balance: 2/10 (min 7): intrusive, assessment-repair is high
  surface: internal/assessment/finding (public)
  next: use the surface, or: archfit request -m "<why score belongs to the surface>"
```

Tasks 1.3 and 1.8 fix the wording. A worker still never sees a suggestion: an unapproved
suggestion reads as permission.

## 7. Measurement and gates

### 7.1 Gate G1: the judge bar

**Gold set.** Units from `archfit`, `pumba` and `ccgram` at pinned commits:

- `surface-role`: every package or file that another module imports. `archfit` has 51. The target
  is about 120 across the three repositories.
- `same-rule`: about 100 pairs. Positive: the `duplication` exceptions and drift entries, the
  `pumba` run-id pattern, copies the trap runs produce, and rewritten copies of known single-home
  rules. Negative: similar literals that are different rules.

Two frontier models label each unit blind with the rubric, in headless agent sessions as the trap
benchmark runs them. The owner decides every disagreement and a sample of 20% of the agreements.
Labels, rationale and the commit live under `scripts/eval/judges/gold/`.

**Bake-off.** One script scores, per question: the 4.0 classifier (type-shape hint), Jev, each
Clef size the owner gave credentials for, and the frontier ceiling. Per judge it reports
agreement with gold by choice, the precision and coverage at each threshold, the repeat stability
over three calls, the latency and the cost.

**The bar**, fixed before the run. A judge model version may emit suggestions for a question when,
at one threshold:

- precision is at least 0.90, with the lower end of the 95% interval at least 0.80;
- coverage is at least 0.40 of the units whose gold label is not `uncertain`;
- the same choice comes back in at least 98% of repeated calls;
- the median latency is at most 2 seconds.

A question no judge passes ships without judged suggestions. The release does not wait for it.
The 4.0 classifier's score is recorded as the answer to "how wrong was the guess".

### 7.2 Gate G2: the trap benchmark

The trap benchmark of the 4.0 design (15.2) runs on the 4.1 build with three Claude tiers, the
hook arms and the prose arm, two repetitions. Targets are unchanged: forbidden changes 0, unnoticed
identical copies at most 1 in 10, at least 8 in 10 failing runs end with a fix or a request. The
new `balance:` line is part of the record the agents read. A missed target is recorded with its
runs; the manager and the owner decide whether hooks become a required setup step.

### 7.3 Gate G3: one number

A corpus test proves, for every dependency between modules in the three repositories, that `can`,
the request preview and the report give the same strength and balance. A second run compares the
4.0.0 and 4.1.0 builds on the same commits and policies. Every changed number has one of the
causes in the table of 6.1 or the rules of 6.3. An unexplained change stops the release.

### 7.4 Gate G4: end to end

On the rehearsal repository, then on `pumba`: a push produces suggestions; the owner applies two of
them in one edit with the passkey; the App opens and merges the policy pull request; the next push
no longer lists them; `archfit can` shows the new surface. With a judge on: the artifact is read,
a judged suggestion appears with its probability, and a judge outage shows as `unknown`, not as an
empty list.

## 8. Delivery

| Milestone | Outcome | Done when |
| --- | --- | --- |
| 0. Prepare and measure | Contracts written and frozen. Gold set labelled. Bake-off and trap benchmark run. Baseline reports of the three repositories stored. | G1 decision and G2 results recorded. Contracts merged. |
| 1. One rule | Surface roles, one strength function, the balance with inputs, the evidence rules, suggestions without a judge, the worker's side (6.9). | Rule matrix and equality tests pass. G3 passes on fixtures and the corpus. |
| 2. Judges | The judge port and adapter, the cache, `suggest`, the judged kinds G1 allows. | Skipped as a whole when G1 allows no question; the bake-off script stays. |
| 3. Action and image | Engine pin, result v2, the `suggest` steps and the artifact. | The Action's own tests pass. The pin pull request is ready; its pinned-image checks go green after the tag, as in 4.0. |
| 4. App | Result v2, the new policy keys, balance inputs, surfaces on the policy page, suggestions with apply and dismiss. | Golden files and the words test pass on the integration branch. |
| 5. Release, migrate, validate | The window of section 9. The three repositories migrated. G4 passed. Clean-up done. | Section 10 holds. |

Milestones 1, 3 and 4 start when the contracts are merged. Milestone 2 starts on the G1 decision.

### Tasks

Lane: C is a Claude worker, P a Pi worker. Merge: M means the manager merges after reading the
diff; W means the worker merges on green checks. The manager may split or regroup tasks, and
writes one brief per task.

| Id | Repository | Task | Depends on | Lane | Merge |
| --- | --- | --- | --- | --- | --- |
| 0.1 | team | Team directory, charter and protocol for this release, from the 4.0 team's scripts | — | manager | — |
| 0.2 | archfit | Contract PR: policy keys `model` and `judge`, result v2, report v2 amendment, questions v1 and suggestions v1, schemas and golden files, no behaviour | 0.1 | C | M |
| 0.3 | archfit | Baseline: 4.0.0 `check`, `can` and report outputs of the three repositories at pinned commits, stored for G3 | 0.1 | P | — |
| 0.4 | archfit | `scripts/eval/judges`: unit extraction, labelling sheets, scorer, bake-off runner, on the model of `scripts/eval/traps` | 0.1 | C | W |
| 0.5 | archfit | Rubric v1 for both questions, from the book chapters 7 and 13; reviewed by a second agent against the EPUB | 0.1 | C | M |
| 0.6 | archfit | Gold set: blind labels by two frontier models; owner adjudication; labels committed | 0.4, 0.5 | P + owner | M |
| 0.7 | archfit | Bake-off run and report; the G1 decision and the threshold table | 0.6 | P | M |
| 0.8 | archfit | Trap benchmark on the 4.0.0 build with three Claude tiers and hooks, as the starting point for G2 | 0.1 | P | — |
| 1.1 | archfit | Surface roles in `internal/policy/resolved` and the policy file adapter; policy errors for overlap and paths outside the module | 0.2 | C | M |
| 1.2 | archfit | One strength function; the check and the report classifier use it; intrusive and the target's volatility; hint-based strength and the `internal:` path deleted | 1.1 | C | M |
| 1.3 | archfit | The balance with its inputs in the record, JSON, SARIF, `can` and the request preview | 1.2 | C | W |
| 1.4 | archfit | Evidence rules of 6.3: assumed values, cascade removal, wording; the regression list of the review | 1.2 | C | M |
| 1.5 | archfit | Suggestions without a judge: the four kinds, keys, the repository block, the report section | 1.2 | C | M |
| 1.6 | archfit | G3 tests: equality of `can`, preview and report; the 4.0.0 against 4.1 corpus comparison with every difference explained | 1.3, 1.4, 0.3 | P | W |
| 1.7 | archfit | User documents: `coupling.md`, `rules.md`, `configuration.md`, `results.md`, `evidence.md`, `commands.md`, `agents.md`, the shipped skill | 1.2 to 1.5, 1.8 | P | W |
| 1.8 | archfit | The worker's side (6.9): the `surface:` line on a coupling error, surfaces in `archfit rules`, the surface request | 1.3, 1.5 | C | M |
| 2.1 | archfit | Judge port, the adapter for the typed-questions protocol, the cache, the architecture test for the check path | 0.7 | C | M |
| 2.2 | archfit | `archfit suggest` with the two steps; question building for `surface-role`; the threshold table; `unknown` items | 2.1, 1.5 | C | M |
| 2.3 | archfit | `same-rule`: near-identical literal candidates and the `copy` kind. Only when G1 allows the question. | 2.2 | C | M |
| 2.4 | archfit | Documents for `suggest`, the judge key, what leaves the workflow | 2.2 | P | W |
| 3.1 | archfit-action | Vendor result v2; prepare the pin pull request; set the digests after the tag (R1) | 0.2, 5.1 | C | M |
| 3.2 | archfit-action | The two `suggest` steps on a default-branch push, input `judge-key`, the artifact; tests that the key never reaches a container with the checkout | 2.2 | C | M |
| 3.3 | archfit-action | README, `CLAUDE.md` rule text, changelog | 3.1, 3.2 | P | W |
| 4.1 | archfit-app | Integration branch. Vendor result v2 and the policy keys; the manifest entry gets its digests and the Action commit after R2 | 0.2 | C | M |
| 4.2 | archfit-app | Balance with inputs on the request and pull-request pages; surfaces and origins on the policy page | 4.1 | C | W |
| 4.3 | archfit-app | Suggestions: read from the repository block and the artifact; store dismissals; the Home list | 4.1 | C | M |
| 4.4 | archfit-app | Apply: suggestions to one policy edit, preview, passkey, the existing policy pull request. The request page shows the matching suggestion and offers the surface answers (6.9). | 4.3 | C | M |
| 4.5 | archfit-app | User guide, architecture document, screenshots, the words test | 4.2 to 4.4 | P | W |
| 5.1 | archfit | Release notes; tag `v4.1.0` through the release workflow | milestones 1, 2 | manager | — |
| 5.2 | all | The window of section 9 | 5.1, 3.x, 4.x | manager + owner | — |
| 5.3 | archfit, pumba, ccgram | Migration of each repository (section 9) | 5.2 | manager + owner | — |
| 5.4 | archfit | G2 on the 4.1.0 build; G4; the validation record | 5.2 | P | — |
| 5.5 | all | Clean-up: integration branches, worktrees, worker sessions, the 4.0 design pointers in `current.md`, `CLAUDE.md` and `AGENTS.md` | 5.3, 5.4 | manager | — |

Rules for every task: one worktree and one pull request; the repository's `AGENTS.md` and
`CLAUDE.md`; a test that fails before the change for each behaviour; user documents in the same
pull request as the behaviour; read the diff before accepting a regenerated golden file.
The policy of `archfit` itself does not use the new keys before the window: its gate runs the
released 4.0.0 engine until then.

## 9. Release window and migration

Before the window: all tasks of milestones 1 to 4 merged (the App on its integration branch); G1
and G3 recorded; the rehearsal repository exists and has the App installed; the owner has confirmed
the hour. A merge freeze on the three repositories starts with the window.

| Step | Who | Do | Rollback |
| --- | --- | --- | --- |
| R1 | manager | Tag engine `v4.1.0`. The workflow builds the binaries and the image. | Fix forward with `v4.1.1`. |
| R2 | manager | Action: merge the final digest pin; tag `v4.1.0` and move `v4`. | Fix forward. |
| R3 | manager | App: manifest lists engine 4.1.0 and the Action commit; merge the integration branch to `main`; the deploy promotes after the canary is ready. Result v1 uploads fail from here. | Traffic back to the recorded revision. |
| R4 | manager + owner | Rehearsal repository: pin bump, a push, a suggestion applied with the passkey (G4, first half). | R3 rollback. |
| R5 | manager + owner | `pumba`: pin bump pull request, then G4 in full. | Revert the pin; R3 rollback. |
| R6 | manager + owner | `archfit`, then `ccgram`: pin bump pull requests. | Revert the pin. |
| R7 | manager | Window closes. Release notes published. | — |

A pin bump that only moves the Action ref to a commit the App lists needs no passkey. Adding the
`judge-key` input changes the workflow file, so the owner approves it once per repository.

Migration of each repository, after its pin moved (task 5.3). Every policy change goes through the
App with the owner's passkey, in as few edits as the App allows:

1. Apply or dismiss every suggestion without a judge. Each exception gets a date, becomes a surface
   declaration, or is fixed in code. Target: no exception without a date.
2. Declare the model part where the owner agrees a surface is the module's model.
3. When G1 allowed a question: add the repository secret, set `judge` in the policy, and work
   through the judged suggestions.
4. Confirm: the gate passes on the default branch; the suggestion list is empty or every entry is
   dismissed on purpose; the drift issue is closed or current.

## 10. Validation

The release is done when all of these hold, and the manager has recorded each with its evidence:

| Check | Evidence |
| --- | --- |
| One number | G3 passes on the three repositories with the released binaries. |
| Explained change | The 4.0.0 against 4.1.0 comparison lists every changed number with its cause. |
| Judges measured | The bake-off report and the G1 decision are in the repository. Each shipped threshold has its gold-set score. |
| Behaviour | G2 results on the 4.1.0 build, three tiers. Targets met, or the misses recorded with a decision. |
| End to end | G4 passed on the rehearsal repository and on `pumba`. |
| Migrated | The three repositories run engine 4.1.0 through the App gate. No exception without a date. |
| No regression | Check time on `pumba` under 2 seconds warm. No new run without a verdict in the week after the window. |
| Documents | User guides of the three repositories describe 4.1. The words test passes. |
| Clean | Worktrees, branches, sessions and the integration branch are removed. |

## 11. Team

The 4.0 release ran with one manager agent and worker agents in agterm. This release uses the same
model. Task 0.1 copies the scripts and the worker protocol of the 4.0 team directory into a new
team directory and writes the charter from this section.

- **Manager.** One Claude Code session. Owns the plan, the briefs, the contracts, the merge of
  every `M` task, the release window, and the status ledger. Works in no repository checkout.
- **Workers.** One session, one repository, one worktree, one branch, usually one pull request.
  Each follows the phases analyse, propose, build, review, pull request, merge, report, with the
  message prefixes `PLAN:`, `Q:`, `READY:`, `DONE:`, `BLOCKED:`. At most 8 Claude workers; Pi
  workers take evaluation runs, mechanical edits, documents and independent reviews.
- **Contracts first and alone.** Task 0.2 merges before any task that reads a contract. No two
  workers edit the same files at one time.
- **Heavy slot.** One worker at a time runs a full `make all`, a corpus run or a benchmark.
- **Reviews.** Every `M` task gets a review by a session that did not write it, on a pinned commit.
- **App work** goes to an integration branch, because `main` deploys on push.
- **Authority.** As in the 4.0 charter: the manager approves plans, merges, tags through the
  release workflows, and deploys with a rollback path. It never prints or copies a secret, never
  forces a default branch, never bypasses a required check.
- **Gates decide scope.** The manager applies the bars of section 7 as written. Changing a bar
  after its run needs the owner.

What needs the owner's hands:

| When | What |
| --- | --- |
| Milestone 0 | Decide the gold-set disagreements (one sitting). Give Cloudflare credentials if Clef is to be measured; the Jev key is in the repository's `.env`. |
| Before the window | Confirm the hour. Create the rehearsal repository if it no longer exists. |
| In the window | Every passkey step. The repository secret for the judge, per repository. |
| Migration | Choose dates and surfaces for the suggestions. |

## 12. Decisions

Made by the owner on 2026-11-17:

| Decision | Answer |
| --- | --- |
| Authority of a judged value | A suggestion only. Never an input to a score or a status. |
| Source to a judge | Allowed for the public repositories. Jev uses the key in `.env`. |
| Backward compatibility | Not needed. The owner migrates the repositories. |
| Delivery | One team of agents, manager and workers, through release, migration and validation. |

Defaults of this design. They stand unless the owner changes them before task 0.2 merges:

| Decision | Default |
| --- | --- |
| Source of strength | The declared surface, in every tool. Type facts are evidence. |
| Model coupling | A policy key `model` beside `public`. |
| Past the surface | `intrusive`, with the target's volatility. |
| Assumed values | Never qualify a report finding. |
| Cascade | Removed. |
| A new command | `archfit suggest`, the eighth command, for owners. |
| Product judges | Jev and Clef through one adapter. Frontier models only label and set the ceiling. |
| The bar | Section 7.1. A question no judge passes ships without judged suggestions. |
| Versions | Engine 4.1.0, module path unchanged, result v2, report v2 amended, policy schema 3 with new keys, Action 4.1.0. |
| The record names the surface (revision 2) | A `surface:` line on a coupling error, and surfaces in `archfit rules`. |
| The surface request (revision 2) | A use past the surface may ask for the surface to change. `coupling` stays `deny` for everything else. |

## 13. Risks

| Risk | Mitigation |
| --- | --- |
| No judge passes the bar. | The release still ships one rule, the surface roles and the suggestions without a judge. The bake-off stays as the instrument for the next model version. |
| A judge is biased toward one choice. The probe chose `model` five times of five. | The gold set has all three roles. The bar is per choice. A suggestion never changes the policy without the owner. |
| The owner faces too many suggestions. | Keys and dismissals; one edit for many suggestions; the first four kinds are finite and shrink to zero. |
| The owner declares everything `public` to make the list empty. | The policy page shows the surface and its origin. `surface-role` questions a public package a judge rates as model. Widening a surface is a loosening edit with a passkey. |
| Workers send surface requests instead of fixing the code. | A second request for the same package joins the first. A rejection becomes a `deny` rule with `instead` and answers later requests. G2 counts requests per run. |
| Moving a surface to `model` fails changes at `min_balance: 8`. | The default stays 7, where model passes. The balance line shows the cause. |
| A judge model version changes under the same name. | Thresholds are per version. An unmeasured version is silent and says so. The bake-off is one command. |
| Source reaches a third party by mistake. | The judge is off unless the protected policy names it. The key is only in the container without the checkout. Secret paths are never read. |
| The result v2 gap breaks a gate. | Three repositories, one window, pin bumps directly after the App deploy, rollback by traffic. |
| The target's volatility loosens the gate. | Measured: no status change on the 14 exceptions of `archfit`. Task 1.6 measures all three repositories before release. |
| The plan is larger than one release. | Milestone 2 is optional by gate. The manager may ship milestones 1, 3 (pin only), 4 (without judged suggestions) and 5 first. |

## Appendix A. How to reproduce

- Report numbers: build `./cmd/archfit` at the commit, run `archfit report --format json` in the
  repository root, and group `.seams[]` by `.strength` with the sum of `.edges`. A stale local
  `go.work` makes the typed Go load partial; run with `GOWORK=off` or remove the file.
- Two balances: `archfit can internal/application/analysis.go internal/policy`, and the seam
  `analysis-application -> architecture-policy` in the report.
- The dead end of 6.9: `archfit can internal/application/analysis.go internal/assessment/score`.
- Exceptions: count `- rule:` and `expires:` under `exceptions:` in `.archfit.yaml`.
- Volatility comparison: for each `coupling` exception, the book formula at S=8 with the higher
  volatility of the two modules, against S=10 with the target's.
- Trap gate: the team report "Trap gate on the Archfit 4.0.0 release build", 2026-11-15.
- Book review: the team report "bc-research: primary-source verification", 2026-11-16, read against
  the local EPUB, chapters 6 to 10 and 13.

## Appendix B. Jev feasibility probe

One script, 2026-11-17, model `jev-1.14.0`, endpoint of the typed-questions protocol. State: the
package path, the output of `go doc -short` (at most 140 lines), and up to 10 importing files.
One question of type `choice` with the four choices of `surface-role` and a short rubric.
Each package was asked twice.

| Package | Choice | Probability (run 1, run 2) | Next choice |
| --- | --- | --- | --- |
| `internal/policy/resolved` | model | 0.93, 0.93 | contract 0.06 |
| `internal/model/graph` | model | 0.96, 0.95 | contract 0.02 to 0.03 |
| `internal/assessment/finding` | model | 0.89, 0.88 | contract 0.09 to 0.10 |
| `internal/assessment/score` | model | 0.77, 0.76 | contract 0.17 |
| `internal/relationship/classify` | model | 0.58, 0.54 | contract 0.21 to 0.23 |

Latency 200 to 252 ms. Request size 2.3 to 3.1 KB. The service accepted a free-form state and a
custom question. What this shows: the protocol fits, the answers are graded, and repeats drift a
little. What it does not show: accuracy. No gold label existed, and the answers lean to one choice.
