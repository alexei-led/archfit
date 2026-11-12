# Archfit 4: architecture change control

Date: 2026-11-10. Status: revision 5. Every open decision has an owner answer of 2026-11-10
(section 18). It includes two independent reviews (design and usability) and a research note
([appendix C](#appendix-c-review-and-research-record)).
Scope: the engine and CLI (`archfit`), its Docker image, the GitHub Action (`archfit-action`), and
the GitHub App (`archfit-app`).

Revision 5 (2026-11-10): milestone 0 results recorded in [appendix D](#appendix-d-milestone-0-results); manager decisions from them in [section 18](#18-decisions).

Reader: the owner, and the agents that implement this design.
This is the single design source for Archfit 4. It replaces the documents listed in
[section 17](#17-documentation-plan). When this document and an older one disagree, this one
wins.

It states what to build and why. It does not fix package names, field names, or task order
inside a milestone. Each milestone gets its own executable plan, derived from this document.

The App pages are shown in a clickable prototype (private link, sample data):
<https://claude.ai/artifact/1esMNt4KvjHEg5kTKengVT>. It follows this revision. Section 12 is the
specification; the prototype is its illustration (see [12.6](#126-the-prototype)).

## 1. Summary

Archfit 3 checks import rules and reports coupling metrics. Measurements on 2026-11-10 show that
coding agents do not break a checked import boundary. When the boundary blocks the short path,
half of them copy the code instead. The check does not see the copy and passes it. A way to ask
for an architecture change exists in the App, but workers do not see it.

Archfit 4 turns the product into **change control for architecture**:

1. **The architecture is one file.** `.archfit.yaml` holds the modules, the dependencies allowed
   between them, and a few rules. Each rule has a reason and says what to do instead.
2. **Policy speaks the Balanced Coupling model.** The allowed dependencies say who may know whom.
   A coupling rule says how much they may know.
3. **Conform or ask.** A change either follows the policy or produces a request. The owner sees
   the request with the engine's numbers and approves or rejects it.
4. **Only an owner changes the policy**, in the App, with a step an automation cannot perform.
   The policy on the protected default branch is the truth for every tool.
5. **One experience.** The CLI, the pull-request check, the comment, and the App show the same
   record in the same words: where, which rule, why, what next.
6. **The check judges what the change adds.** Trends and repository-wide measurements are a
   report for the owner, never a yellow light on each pull request.
7. **Less to learn and keep.** Seven commands instead of fifteen. One policy file instead of
   four. No stored baseline, no re-anchoring, no comparability profile.

## 2. Problem

### 2.1 What was measured

All numbers come from the evaluation of 2026-11-10 on `archfit` v3.4.0.

| Observation | Evidence |
| --- | --- |
| The pull-request check caught no organic error. | `pumba`: 61 pull-request runs in the 12 days since the check became required. One failure, a deliberate probe (`pumba#390`). `archfit`: 85 runs, no architecture failure. |
| Agents did not break the import boundary, with or without the tool. | Trap task on `pumba`: 18 headless agent runs, three model tiers, five setups. Forbidden imports: 0 of 18. Policy edits: 0 of 18. |
| The trap set two written rules against each other. | The module graph says `pkg_tc` imports no module. `AGENTS.md` says the interface-name rule has one home in `pkg/util` and no copy may exist. No code satisfies both. |
| Half of the agents resolved it by copying. | In 9 of 18 runs the agent kept the module graph, copied the rule into `pkg/tc`, and reported it. The strongest model copied in 7 of its 8 runs. The other 9 runs stopped and asked. |
| The check passed every copy. | `archfit check` returned `pass` for all 9. |
| The policy contradicts the model the tool implements. | By the documented formula, the denied import scores 9/10 and the copy that passed scores 1/10 ([appendix A](#appendix-a-worked-example)). |
| The ask path exists but workers do not see it. | The App has decision requests. The CLI has no command for them, no check output points to them, and the form is a JSON block with a commit id, a policy digest, and a whole candidate policy. No agent in the experiment used it. |
| The tool was most useful before code existed. | The `pumba` 1.5.0 plan lists traps the planning agent found with `policy can-import` and designed around. |
| Most pull requests carry a warning nobody can act on. | 51 of 61 `pumba` runs ended "passed · attention needed". |
| Many runs end without a verdict. | `archfit`: 38 of 85 runs ended as a job failure or `action_required`. Not all causes were investigated. |
| Local runs cannot evaluate metric ratchets. | Every local result says "reference not comparable". The cause is a difference in patch versions of `ast-grep`, `jscpd`, and `git`. |
| The policy covers a small part of the real rules. | `pumba` states about 15 architecture rules in prose; 6 import rules are checked (estimate). `archfit` keeps about 2,100 lines of Go architecture tests beside its 41 rules. |
| The agent hooks are not installed where they should be. | No hooks in `archfit` or `pumba`. `pumba` ignores `.claude/`, so a clone or worktree never gets them. |
| The same thing has many names. | One state is called blocked, fail, failure, denied, and needs attention across the CLI, the check, and the App. One hit is a finding, a blocker, a violation, or a task. |

Limits: one trap task, one small Go repository, short sessions, Claude models only, twelve days
of a required check. The experiments in section 15 must confirm this design.

### 2.2 What this means

- **What a tool checks is respected. What nothing checks gives way.** When two rules conflicted,
  agents kept the one with a check and broke the prose rule.
- **The gap is the moment the architecture must change.** The legitimate path is invisible and
  heavy, and the workaround passes.
- **The check and the model disagree.** The check sees an import. The model sees shared
  knowledge. A copy shares more knowledge than a call to a stable public function.
- **Fragile comparisons cost more than they give.** Stored baselines and profiles produce most
  non-verdicts.
- **Architecture is more than imports**: copies, library containment, side effects, public
  contracts.
- **Too many words and too many commands** make a correct tool hard to act on.

## 3. Goals

1. The architecture of a repository is one file a person can read, write in minutes, and keep
   true.
2. A worker learns the rules before writing code and gets one clear next step for every error.
3. A change that needs a different architecture produces a request, not a workaround.
4. Only owners change the policy. The path is short and visible. An agent cannot take it alone.
5. The laptop, the Action, and the App give the same verdict for the same tree and policy.
6. A passing change is green. A worker sees only what the worker can act on.
7. The CLI, the check, the comment, and the App use the same words, the same numbers, and the
   same order of information.
8. The primary output is a linter-style text record. An agent needs no JSON to act.
9. An engine upgrade needs no data migration in a repository.
10. Policy covers more than dependencies.

### Non-goals

- A repository score.
- An LLM in the check. An LLM proposes and explains; it never passes or fails a change.
- Proof that an architecture is good. Archfit enforces the approved policy over disclosed
  evidence.
- Decisions without the App. Without the App the CLI is a local guide.
- Organisation-level policy. The first release serves one repository, one or a few owners, and
  many agents.
- Analysis of source code by the App. The engine keeps running in the repository's workflow.
- Runtime measurement. New languages.

## 4. Principles

Kept:

- **Missing evidence is not zero.** `unknown` never passes.
- **GitHub is the database.** What an owner approves lives on the protected branch. The App
  stores almost nothing.
- **The CLI predicts; the App decides.**
- **No LLM, billing, or UI code on the check path.**
- **Source code stays in the repository's CI.**
- **Analyzers stay behind adapters.**
- **The published method is used verbatim.** `bc_score.v7` and the book formula do not change.

New:

- **The graph is approved; the formula judges quality.** A new dependency between modules is an
  owner decision. The strength of what crosses it is a rule.
- **Conform or ask.** Every error names one next step. When the policy offers none, the next step
  is a request.
- **Judge the change; report the repository.**
- **The check uses only facts that are deterministic, limited to the change, and independent of
  optional tools.**
- **No state that can go stale.** Base and head are compared in one run.
- **The protected branch is the root of trust.** Add mechanisms only where it does not hold.
- **One word per thing, one record everywhere, one way to ask.**
- **Words a developer already knows.** Prefer the words of linters, Git, and GitHub to new ones.

## 5. Words

One word per thing, in the CLI, the policy file, the check, the comment, the App, and the
documents.

| Word | Meaning | Not used |
| --- | --- | --- |
| **policy** | The file `.archfit.yaml`: modules, allowed dependencies, rules, exceptions. | config, architecture file, classification |
| **module** | A named set of paths with a public part, a volatility, and a list of modules it may depend on. | component |
| **public** | The part of a module that other modules may use. | surface |
| **allowed dependency** | Module A may depend on module B (`depends_on`). | approved pair, edge, relation |
| **rule** | Anything the check enforces. Built-in rules have fixed names. Custom rules have the id the owner gives. | law, intent, decision, guardrail |
| **exception** | A permission to break one rule in one place, with a reason and usually a date. | waiver, baseline entry, debt, temporary decision |
| **error** | One place where the change breaks a rule. | finding, violation, blocker, problem, task |
| **check** | The command, and the pull-request check `archfit/gate`. | gate, scan |
| **pass / fail / waiting / unknown** | The four statuses of a change. | blocked, denied, needs attention, healthy |
| **request** | A proposal to change the policy. It is a GitHub issue; its id is the issue number, written `#431`. | decision request, CR |
| **approve / reject** | The owner's answer to a request. | accept, decline, record |
| **owner** | A person who writes the policy and answers requests. | architect, approver |
| **worker** | A person or agent who changes code. | — |
| **balance** | The coupling number, shown as `9/10 (min 7)`. | rating, score |
| **volatility** | `low`, `medium`, or `high`: how often a module is expected to change. The check uses this value. | — |
| **subdomain** | Optional: the business role of a module (`core`, `supporting`, `generic`), as in the book. It only sets the volatility when `volatility` is absent. | — |
| **report** | The repository-wide measurements for the owner. | review, analysis, insight |

Numbers:

- Balance is always `n/10 (min m)`. Strength and volatility are shown as words. Distance is not
  shown: across modules it is always the same.
- Dates are ISO with the days left: `2026-11-19 (9 days)`.
- A worker sees four things: file and line, rule id, request number, expiry date. Digests,
  engine versions, and ordinals appear only with `--verbose` and in the upload.

## 6. The policy

### 6.1 The file

The smallest valid policy:

```yaml
version: 3
owners: [alexei-led]
modules:
  app:
    paths: [cmd/**, pkg/**]
```

A typical one:

```yaml
version: 3
owners: [alexei-led]
gate: required              # default: advisory
layers: [core, cmd]         # inner first
modules:
  pkg_chaos:
    paths: [pkg/chaos/**]
    public: [pkg/chaos, pkg/chaos/cliflags]
    volatility: high
    layer: core
    depends_on: [pkg_container, pkg_util]
  pkg_tc:
    paths: [pkg/tc/**]
    public: [pkg/tc]
    volatility: high
    layer: core
  pkg_util:
    paths: [pkg/util/**]
    volatility: low
    layer: core
rules:
  - id: no-chaos-to-runtime
    deny: { from: pkg_chaos, to: pkg_runtime }
    why: A direct import ties a chaos action to one runtime.
    instead: Add a method to container.Client.
  - id: docker-sdk-in-runtime
    only: { use: github.com/docker/docker, in: [pkg_runtime] }
    why: One adapter owns the Docker API.
  - id: fan-out-through-runner
    note: Chaos actions fan out through RunOnContainers.
    why: One place checks the target limit before injection.
exceptions:
  - rule: duplication
    from: pkg_tc
    to: pkg_util
    expires: 2026-11-19
    why: "Request #431 is open."
```

Both examples illustrate the shape. They are not the schema.

Defaults, so that a short file means something safe:

| Key | When absent |
| --- | --- |
| `depends_on` | The module may depend on no other module. |
| `public` | The whole module is public. Add `public` to protect its inside. |
| `volatility` | Taken from `subdomain` when that is present (`core` is high; `supporting` and `generic` are low). Otherwise `high`. |
| `subdomain` | No business role is recorded. |
| `layer`, `layers` | No layer rule. |
| `gate` | `advisory`: the check reports and does not block. |
| `rules`, `exceptions` | None. The built-in rules still apply. |
| `generated`, `tests` | Only paths listed here are taken out of the rules. A file name that looks generated is not enough. |

Tool and language settings stay in the same file. The whole file is protected, because a setting
can hide code from the check.

An author needs four ideas for a first policy: module, `depends_on`, `public`, `volatility`.
`subdomain` is optional. When both are present, `volatility` decides and `subdomain` describes.
`archfit init` writes the first version from the code.

### 6.2 Built-in rules

They apply to what a change adds. They have fixed ids and need no entry in the file.

| Id | What it says | Default |
| --- | --- | --- |
| `graph` | A dependency between two modules must be an allowed dependency. | `ask` |
| `coupling` | New knowledge across a module boundary needs a balance of at least 7. | `deny` |
| `duplication` | Knowledge that another module already owns is not copied. | `ask` |
| `cycle` | No new cycle between modules. | `deny` |
| `layers` | Dependencies follow the layer order. | `deny` |
| `unowned` | New production code belongs to a module. | `deny` |

A rule has one mode: `deny` (fix the code), `ask` (an owner may approve), or `off`. To change a
default, add a `builtin` block, for example `builtin: { graph: off, min_balance: 8 }`.

**`graph`** keeps the shape and direction of the architecture with the owner. A new dependency is
rare, and the answer is cheap, because the request arrives with the balance.

**`coupling`** applies inside allowed dependencies too. With the shipped ordinals:

| Volatility that applies | Use of the public part | Use past the public part | Copy |
| --- | ---: | ---: | ---: |
| `low` | 9 or 8 | 8 | 8 |
| `medium` | 9 or 7 | 5 ✗ | 5 ✗ |
| `high` | 9 or 7 | 2 ✗ | 1 ✗ |

- The volatility that applies is the target's for a use of the public part, and the higher of the
  two modules for a use past it and for a copy. This follows `docs/coupling.md`.
- The rule uses the declared volatility. The volatility cascade is not used here, because with it
  one new dependency changes the balance of code the change did not touch.
- The check tells a use of the public part from a use past it by the `public` globs alone. Both
  kinds of public use pass at every volatility, so the check needs no type analysis.

In words: a module may use the public part of a module it is allowed to depend on. It may reach
past the public part only into a module declared `low`.

**`duplication`** judges every copy, even one whose balance would pass. Section 7.3 says how a
copy is found.

### 6.3 Custom rules

Three forms:

- `deny`: no dependency from these modules or paths to those. It needs `why` and `instead`.
- `only`: a library, or a named set of packages, is used only in the listed modules. The engine
  ships sets for network, process, and database access, so `only: { use: process, in: [...] }`
  keeps side effects in their adapters. It needs `why`.
- `note`: a rule nothing can check today, written down with its reason. It shows in
  `archfit rules` and in the report as "not checked". The policy then holds the whole
  architecture, and the unchecked part is visible instead of living in prose.

The id is a short name, as in a linter: `no-chaos-to-runtime`. It appears in every error.

When an owner rejects a request, the answer becomes a `deny` rule with `instead`. The next
worker gets the answer without a new request.

A `deny` rule is direct. It does not stop A from reaching C through an allowed B. Use layers for
a direction that must hold all the way.

### 6.4 Exceptions

An exception names the rule, the place (`from`, `to`), a reason, and usually `expires`. It is the
only way to accept an error. Uses: errors that existed when the policy started, a temporary
permission while a request is open, a planned migration.

**A date has teeth.** The App warns the owners before it. After it, if the code still breaks the
rule, every pull request fails until an owner extends the exception or the code is fixed.

### 6.5 Beyond dependencies

| Dimension | What it protects | Evidence | In the check | Release |
| --- | --- | --- | --- | --- |
| Module graph | Who may know whom; layers; cycles; unowned code | Imports, paths | Yes | 4.0 |
| Coupling | How much new knowledge crosses a boundary | Imports, `public`, `volatility` | Yes | 4.0 |
| Copies | One owner for each rule, format, or table of values | Identical literals (7.3) | Yes | 4.0 |
| Containment | A library or a kind of side effect stays in its modules | Imports | Yes | 4.0 |
| Notes | Rules without a check are still in the policy | Policy | Shown | 4.0 |
| Stable contracts | A public part the owner locked does not change silently | Diff of exported declarations | Yes, opt-in | Later |
| Data ownership | One writer for each table or topic | Declared stores | Yes | Later |
| Copies in other words | Knowledge duplicated with different text | Clones, LLM reading | Report | Later |
| Cohesion | A module does not collect unrelated things | Clusters, hubs | Report | Later |
| Change locality | A change stays in few modules | Git history | Report | Later |
| True volatility | Declared volatility matches real churn | Git history | Report | Later |
| Size trends | Where complexity concentrates | Syntax | Report | Later |

Per-function size and complexity limits belong to linters. Clock, randomness, and environment
access are too common to check.

Three kinds of truth: what the owner **declares** (modules, `public`, `depends_on`,
`volatility`), what the engine **measures** (imports, literals), and what a person or an LLM
**judges** (a proposed policy, advice on a request). A declaration wins over a judgment. A
measurement that disagrees with a declaration shows in the report as a proposal.

## 7. Verdict

### 7.1 Statuses

A change has one of four statuses, everywhere:

| Status | Meaning | The worker's next step |
| --- | --- | --- |
| `pass` | The change adds nothing a rule forbids. | None |
| `fail` | The change adds an error. | Fix the code, or ask, as the error says |
| `waiting` | The errors are covered by an open request. | Nothing. Wait for the owner. |
| `unknown` | No verdict: evidence is missing, or the local policy differs from the protected one. | Restore what the record names |

`waiting` exists on the pull request, where the App knows the open requests. On a laptop the
same change is `fail` with the next step `archfit request`; that command says when a request for
it is already open.

The repository has its own state, for owners:

| State | Meaning | Effect on pull requests |
| --- | --- | --- |
| clean | No error outside exceptions. | None |
| drift | An error exists that no exception covers and no open change added. | None. The owners get one issue. |
| expired | An exception passed its date and the code still breaks the rule. | Every change fails until an owner extends it or the code is fixed |
| policy changed outside the flow | The policy on the default branch has no recorded approval. | Every change is `unknown` |

### 7.2 Rules of the verdict

- A worker's result lists only what the change added. The one exception is an expired exception:
  by the owner's decision it fails every change, and the record says so and names the request.
- Drift is never accepted by silence. It stays until the code is fixed or an owner adds an
  exception.
- **Added means more.** The engine reads the merge base and the head in one run and counts errors
  by rule, modules, and symbol or literal. A new entry, or a higher count, is added by the
  change. One more use of an already broken dependency is an error of this change.
- When the comparison cannot be made, the status is `unknown`.
- The App checks a pull request as merged onto the current target, and again when the target
  moves in the same modules or the policy changes.
- The same tree, policy, engine version, and date give the same output. The date is an input,
  because exceptions depend on it.
- The engine does not suggest where to move code. An error carries the owner's `instead` when the
  rule has one. Otherwise the next step is a request.
- There is no warning on `pass`.

This revises the earlier rule "origin never exempts a blocking finding". The intent stays: an
error is never acceptable because it is old, and an expired exception fails everyone. What
changes: ordinary drift is the owners' work, not the work of an unrelated pull request.

### 7.3 Finding a copy

In 4.0 the check finds one kind of copy: **an identical literal**. A non-trivial string, regular
expression, or table of values that the change adds in one module and that production code of
another module already has.

All nine copies in the experiment contain the original regular expression and error text
unchanged. This was confirmed by search; the check itself is not built yet. Known limits:

- It needs a noise limit. The limit is rule R12, measured in milestone 0 ([Appendix D.4](#d4-copy-noise-request-load-and-check-time)): a literal of at least 12 characters; declared `tests` and `generated` paths out; single words and struct tags out; held by fewer than 3 modules. The precision is measured, not promised: 79% on the dogfood repositories (95% interval 63–89%). Re-typed keys of the repository's own model count as copies (manager decision, 2026-11-10). If they did not, precision would be 37%.
- It does not find a copy in other words. `pumba` has one today: a run-id pattern in
  `pkg/chaos/runid.go` and the same rule with a length bound in `pkg/tc/recovery.go`.
- An agent that reads the error can rewrite the literal. The check is not a wall. It makes the
  honest path the short one: the error names `archfit request`, and the request is answered with
  numbers.

Clones, hints from the session, and an LLM reading for copies in other words go to the report,
not to the check. How many rewritten copies they find is measured before anything is promised.

## 8. One record

The CLI, the check, the comment, and the App show an error as the same record, in the same order: **where, which rule, what;
then why, instead, next.** The status comes last, as in a linter.

```text
path:line: rule-id: what is wrong
  why: the reason the rule exists
  instead: what the owner says to do          (when the rule has it)
  balance: 9/10 (min 7)                       (when the rule is about coupling)
  next: one command, or one short instruction
archfit: status, count
```

`next` is always one of: `fix the code`, a command to run, or `wait for an owner: #n`.

### 8.1 Examples

A pass:

```text
$ archfit check
archfit: pass
```

A rule the code must follow:

```text
$ archfit check
pkg/chaos/netem/netem.go:8: no-chaos-to-runtime: pkg_chaos must not depend on pkg_runtime
  why: A direct import ties a chaos action to one runtime.
  instead: Add a method to container.Client.
  next: fix the code
archfit: fail, 1 error
```

A dependency only an owner can allow:

```text
$ archfit check
pkg/tc/netem.go:12: graph: pkg_tc -> pkg_util is not an allowed dependency
  why: a new dependency between modules needs an owner
  balance: 9/10 (min 7)
  next: archfit request -m "<why you need it>"     (do not copy the code)
archfit: fail, 1 error
```

A copy:

```text
$ archfit check
pkg/tc/interface.go:12: duplication: this literal already lives in pkg/util/util.go:15
  why: two copies must change together and nothing checks that they do
  next: use pkg_util, or run: archfit request -m "<why a copy is needed>" --expires 2026-12-10
archfit: fail, 1 error
```

No verdict:

```text
$ archfit check
.archfit.yaml: policy: differs from origin/main
  why: only owners change the policy
  next: git checkout origin/main -- .archfit.yaml
archfit: unknown, no verdict
```

An expired exception, seen by an unrelated change:

```text
$ archfit check
.archfit.yaml: exception: duplication pkg_tc -> pkg_util expired on 2026-11-19
  why: an exception with a date fails every change after that date
  next: wait for an owner: #431
archfit: fail, 1 error (not from this change)
```

A question before writing code:

```text
$ archfit can pkg/tc pkg/util
pkg_tc -> pkg_util: graph: not an allowed dependency
  balance: 9/10 (min 7)
  next: archfit request -m "<why you need it>"
```

### 8.2 Where the record appears

| Where | Form |
| --- | --- |
| `archfit check`, `archfit can` | The text above, on stdout. Progress and tool errors go to stderr. |
| Hooks | The same text, given to the agent as feedback. |
| Check `archfit/gate` | Title: the status line. Summary: the records. Annotations: one per error on the changed line. |
| Pull-request comment | The status line as the heading, the records, then one line for the author and one for the owner (12.5). |
| App | Each error and each request shows the same fields in the same order. |

### 8.3 JSON

An agent needs no JSON to act. `--format json` exists for the upload to the App and for scripts.
`--format sarif` exists for code scanning. The pull-request comment has no JSON block. The
hooks pass text.

## 9. Protecting the policy

### 9.1 The root of trust

**The policy on the protected default branch is the approved policy.** Every tool judges with it:

- The Action reads the policy from the target branch, never from the pull-request head.
- The App accepts an upload only when its policy equals the one on the target branch.
- The CLI compares the local file with the target branch. If they differ, the status is
  `unknown` and the record says how to restore the file.

This is the usual way to protect a policy of this kind (research of 2026-11-10, sources in
[appendix C](#appendix-c-review-and-research-record)):

- Mergify, Renovate, Dependabot, and GitHub's own CODEOWNERS judge a pull request with the
  default or base branch version of their file.
- Semgrep, SonarQube, GitHub code scanning, and HCP Terraform keep the policy on the server.
- Tools that read their configuration from the pull-request head (Atlantis, Codecov, the Snyk
  CLI) document it as a weakness and add server-side limits.
- No mainstream linter or scanner ships a signature file beside an in-repository policy.

### 9.2 How the policy changes

1. An owner answers a request in the App. An answer that loosens the policy needs a passkey with
   user verification (a fingerprint, a face, or a PIN), registered by that owner. A session
   cookie or a password is not enough, because an agent can drive the owner's browser.
2. The App writes the change into an architecture pull request on its own branch, and records
   the approval: repository, the policy it replaces, the new policy, the owner, the time. An
   approval is valid once, for that step. An older approved policy cannot be used again.
3. The architecture pull request merges. By default the owner's answer is **Approve and merge**:
   the App merges it, so one answer is one action. The owner can choose to merge later; then the
   next answers join the same pull request.

A pull request that changes the policy and is not the App's architecture pull request is
`unknown`. This includes a code change that also edits the policy. Workers ask; they do not edit.
An owner who wants to edit the YAML does it in the App, which checks the text and puts it on the
same path.

A change to `.github/workflows/archfit.yaml` makes the check `unknown` until an owner approves it in the App with a passkey, because a pull request runs its own workflow file and could upload a forged pass. A change that only moves the Action ref to a commit the App lists is exempt.

Set up at onboarding:

- a ruleset on the default branch: changes through pull requests only, 0 required approvals, and
  the check `archfit/gate` required, with the App as its expected source. The App must not be a
  bypass actor. A bypass actor ignores the check: the App merged #5 while its check was `in_progress`
  ([Appendix D.2](#d2-github-mechanics)). "Approve and merge" needs no bypass actor.
- on organization-owned private or internal repositories, on a plan with rulesets for private
  repositories, a push ruleset that restricts the policy path, with the App as the only actor
  allowed to bypass it. The bypass is not measured; GitHub's docs list Apps as eligible bypass
  actors. A personal-account repository never has push rules. There the required check is the
  protection.

If the policy on the default branch changes with no recorded approval (a bypass, a direct push),
every change is `unknown` until an owner confirms or reverts.

The first policy arrives in the onboarding pull request. A repository administrator approves it
in the App. After that, the owners named on the default branch approve.

### 9.3 A signature, later

A signature file beside the policy would let the CLI prove offline which content an owner
approved, and would expose a rewrite of the default branch by someone with bypass rights.

It is not in 4.0: the protected branch and the recorded approvals already stop a worker, and no
comparable tool signs an in-repository policy. If it is added, it follows the rules of The
Update Framework: a version that never decreases, a link to the previous version, an expiry, and
a key that lives outside the repository. Without them a signature can be replayed; gittuf, the
closest existing tool, had this flaw and fixed it with a monotonic version.

### 9.4 Without the App

The CLI works alone: `check`, `can`, `rules`, `report`, `init`, `doctor`. Every result carries
the line `policy: not protected`. A team can protect the file with its own branch rules. There
are no requests. This is a local guide, not change control.

### 9.5 Limits

- On a laptop nothing is tamper-proof. The CLI result is a prediction. The required check on the
  protected branch is the boundary.
- A repository administrator can remove the ruleset. Archfit reports it and cannot prevent it.
- The passkey protects the answer, not the request. A worker can write a persuasive request. The
  balance and the preview are there so the owner does not decide on the worker's words alone.
- No accepted guidance exists yet for approvals in a browser an agent can drive. A passkey with
  user verification is the strongest measure available, not a proof that a person read the
  request.

## 10. Flows

```mermaid
flowchart LR
  subgraph worker[Worker]
    rules[archfit rules] --> code[write code]
    code --> check{archfit check}
    check -->|pass| pr[pull request]
    check -->|fail: fix the code| code
    check -->|fail: ask| req[archfit request]
  end
  subgraph owner[Owner, in the App]
    req --> answer{approve or reject}
    answer -->|approve| apr[policy changes]
    answer -->|approve until a date| apr
    answer -->|reject, with what to do instead| apr
  end
  apr --> again[waiting pull requests are checked again]
  again --> pr
```

### 10.1 Set up

1. `archfit init` in the repository. It writes the policy from the code (modules, `public` for each
   module, `depends_on` as the dependencies that exist, `tests` for the test paths, containment as it
   is today), the workflow file,
   and the instruction block for agents. Errors that exist already are listed as exceptions. It
   never overwrites a file. In an Archfit 3 repository it converts the old files instead.
2. The owner reads the policy and opens one pull request.
3. The owner installs the App, registers a passkey, and approves the first policy.
4. The App shows the two ruleset settings to turn on.

`init --from AGENTS.md` asks an LLM to turn written rules into proposed rules. What it cannot
turn into a check becomes a `note`.

Target: under ten minutes on a repository of the size of `pumba`. Each contributor and agent
then runs `archfit doctor --fix` once per checkout for the tools and hooks.

### 10.2 Work

1. `archfit rules` at the start of a task: the rules for the files in play, in plain words.
2. `archfit can <from> <to>` before a new dependency.
3. `archfit check` before finishing. Do what `next` says.

The hooks run steps 2 and 3. A hook that fails for its own reasons reports and does not block.

### 10.3 Ask

- With a shell: `archfit request -m "<why>"`. It takes the errors of the last `check`, so the
  worker writes only the reason.
- Without a shell: reply on the pull request with `/archfit request <why>`.

Either way the App opens an issue for the request, takes the dependency and the balance from its
own verified result, links the pull request, and assigns the owners. The request id is the issue
number. A second request for the same dependency joins the first. A dependency that an earlier
rule already answers is closed at once with that answer.

Before a pull request exists, `archfit request` opens the issue with the output of
`archfit can`. The App marks its numbers "as reported" until a pull request confirms them.

After asking, a worker waits or does other work. It does not copy the code. The answer arrives
on the issue and in the pull-request comment, and the check changes.

### 10.4 Answer

The owner sees on one page: the reason, the record with its balance, a preview on the current
code (what would pass, what else would change, which pull requests wait), earlier answers about
the same modules, and optional advice marked as advice.

| Answer | What changes in the policy |
| --- | --- |
| Approve | The dependency joins `depends_on`, or the rule changes. |
| Approve until a date | An exception with `expires`. The rule stays. |
| Reject | A `deny` rule with `instead`. It answers later requests for the same thing. |

An owner previews any change locally with `archfit check --policy <file>`.

Who answers:

- An owner, in the App, with the passkey step when the answer loosens the policy. One owner is
  enough.
- A push, a comment, or a review made with an owner's token is not an answer, because a local
  agent can do those.
- Today the App forbids the login that filed a request to answer it. Local agents act as the
  owner, so that rule cannot tell them apart. The passkey replaces it.
- An agent may advise and may file requests. It never answers. Giving an agent the right to
  answer some requests is outside this plan.

### 10.5 After the answer

Every waiting pull request must get a fresh result **without an action by its author**.

Today this is not possible: a workflow re-run measures the commit the first run measured, and
the App asks the author to update the branch. Two candidates. Milestone 0 tested both on a test repository ([Appendix D.2](#d2-github-mechanics)).

1. **Re-run, confirmed under four conditions.** The App starts the re-run when all four hold. The compare from the event's base to the target tip touches only the policy file. The run is younger than 30 days. The workflow reads the policy from the target tip. The re-run is of the latest run of the current head. The code is the same. Only the policy it is judged by is new.
2. **Update the branch, the measured fallback.** The App updates the pull-request branch through the GitHub API, with its own token. It worked in the test. It adds a merge commit by the App to the author's branch. The docs say a `GITHUB_TOKEN` update starts no workflow. This was not measured.

### 10.6 Report

`archfit report` gives the repository-wide picture: the dimensions marked "Report" in 6.5, where
declarations and measurements disagree, rules that are not checked, exceptions close to their
date, and proposals. A scheduled workflow runs it weekly and posts it as an issue. A proposal is
an ordinary request.

### 10.7 Steps

| Flow | Human or agent actions |
| --- | --- |
| Set up a repository | `init`, read, open the pull request, install the App with a passkey, approve, turn on the ruleset: 6 |
| A worker stopped by a new dependency | `check`, `request`: 2, then wait |
| An owner answers | Open the request, approve: 2 |

### 10.8 When things fail

| Case | Behaviour |
| --- | --- |
| The engine or a tool fails locally | Status `unknown`, with the fix in `next`. Hooks do not block. |
| No upload reaches the App | The check stays in progress, then asks for action with the reason. The Action retries. |
| A pull request from a fork | Not supported by the required check; the record says what to do. |
| An exception expires | Every change fails, with the record of 8.1, until an owner acts. |
| Emergency | The ruleset bypass. It is never shown as a pass. |

## 11. Components

### 11.1 CLI

Seven commands.

| Command | Purpose | With no arguments |
| --- | --- | --- |
| `archfit check [--base REF] [--policy FILE]` | The verdict for the change. `--policy` previews a candidate policy. | The working tree against the merge base with the default branch |
| `archfit can <from> <to>` | May this dependency or library be used. Takes files, modules, or packages. | Usage error |
| `archfit rules [PATH or RULE] [--graph]` | The rules that apply, in plain words. One rule in full. The module graph. | The rules for the changed files |
| `archfit request [-m WHY] [--expires DATE]` | Ask an owner. | The errors of the last `check`; asks for the reason |
| `archfit init [--from FILE]` | Set up a repository: policy, workflow, instruction block. Converts an Archfit 3 repository. | Writes what is missing; never overwrites |
| `archfit report` | Repository-wide measurements and proposals. | The whole repository |
| `archfit doctor [--fix]` | This checkout: tools and hooks. | Checks only |

What each replaces:

| Archfit 3 | Archfit 4 |
| --- | --- |
| `check` | `check` |
| `policy can-import` | `can` |
| `policy where`, `explain`, `map` | `rules` |
| `config compare`, `config lint` | `check --policy` |
| `config init`, `config update`, `config enrich`, Action discovery, `agents-md`, `skill install` | `init` |
| `analyze` | `report` |
| `doctor`, `hook install` | `doctor --fix` |
| `baseline` | Gone |

Rules for every command:

- **Exit codes:** 0 pass or yes; 1 fail, waiting, or no; 2 no verdict (unknown, wrong usage, a
  tool error). These match common linters.
- **Output:** the record of section 8 on stdout; progress on stderr.
- **Flags:** `--format json|sarif` for machines, `--verbose` for digests and versions, `-C DIR`
  for another directory. Nothing else is needed for daily use.
- The hook entry points remain as internal plumbing and are not part of the command list.

### 11.2 Engine

The six layers and their rules in `docs/architecture.md` stay.

| Responsibility | Change |
| --- | --- |
| Policy | Schema 3 with the defaults of 6.1. A digest of the file. Whether a policy change loosens, tightens, or neither. |
| Relationship analysis | Scoring is unchanged. New: the balance of a dependency that does not exist yet, for `can`, `request`, and the preview. |
| Assessment | Built-in and custom rules. Errors counted on base and head. The four statuses. |
| Evidence | The facts the check uses, per language: imports and uses, `public` globs, literals, library uses. Read by Go's standard parser and by `ast-grep` (11.8). Base and head in one run, cached by tree. |
| Adapters | Sending a request. Removal of the baseline, labels, and measurement-reference stores. |
| Output | The text record first. JSON and SARIF derive from the same result. |

Constraints:

- Keep policy, classification, assessment, workflows, and adapters in their owning layers.
- The check gives the same result with every optional analyzer turned off.
- When a fact needs resolution that a language cannot do without a build (TypeScript path
  aliases, Rust re-exports, C# symbols), use the cheapest deterministic resolver and report the
  rest as `unknown`.
- Run impact analysis before removing a package or symbol.

Budget on `pumba`: warm under 2 seconds, fresh clone under 5 seconds (manager decision; [Appendix D.4](#d4-copy-noise-request-load-and-check-time)). The v3.4.0 measurement: 0.79 s warm, 1.96 s on a fresh clone with warm Go caches. The earlier figure of 13 seconds was not reproduced. An empty Go build cache took 8.25 s, so the fresh-clone budget assumes warm Go caches.

### 11.3 Agents

- `archfit doctor --fix` installs the hooks of the detected harness. A user-level plugin gives
  the same hooks for Claude Code, so worktrees and fresh clones are covered. `doctor` warns when
  a repository ignores the harness settings directory.
- The instruction block written by `init` has five lines:

  ```text
  Run `archfit rules` before you start and `archfit check` before you finish.
  Do what `next:` says. `archfit: pass` means done.
  Before a new dependency between modules: `archfit can <from> <to>`.
  If a rule is in your way: `archfit request -m "<why>"`, then wait or do other work.
  Never edit `.archfit.yaml`. Never copy code to get around a rule.
  ```

- An MCP server is not part of 4.0.

### 11.4 Action

| Mode | Trigger | Does |
| --- | --- | --- |
| `check` | `pull_request`, `push`, `merge_group`, and the re-check of 10.5 | Runs the check, uploads the result |
| `report` | `schedule`, `workflow_dispatch` | Runs the report, posts the issue |

Removed: `baseline`, `reanchor`, `discovery`, the baseline artifacts, the measurement profile,
and the second engine pass for comparability. Kept: inputs read from git objects, OIDC upload,
pinned image digests, refusal of unsafe events. The policy is read from the target branch only.

### 11.5 Docker image

One image. Its content follows 11.8: for the check it needs the engine, `git`, and `ast-grep`.
The language toolchains leave it as their extractors are replaced. No stored data depends on
tool versions, so an engine upgrade changes the image digest and the pin that Dependabot
maintains, and nothing else in a repository.

### 11.6 App

| Kept | Changed | Removed |
| --- | --- | --- |
| One Go service; GitHub as the database | An answer needs a passkey and is recorded once | Baseline upload and pull requests, re-anchor |
| Upload verification: OIDC, run identity, engine identity | Requests are issues, asked with one line, shown with the balance and a preview | The check `archfit/state` |
| The verdict has no LLM on its path | "Approve and merge" in one action | Comparability reasons |
| The check `archfit/gate` | The home page is what needs the owner, then the map | The file `.archfit-app.yaml` |
| One comment per pull request, edited in place | The check carries the title, the records, and the annotations | Approval by a push or a token-made review |
| The sweep: missed events, moved target, expiry | Waiting pull requests are checked again after an answer | The JSON request form and the JSON block in the comment |
| No report or source stored | The words and the record of sections 5 and 8 | Code-and-policy pull requests |

Check states: `pass` → success. `fail` → failure. `waiting` → in progress, with the title
"waiting, request #431". `unknown` → action required. In `advisory` mode a `fail` is shown as
neutral with the same records.

The sweep refreshes a waiting check (`in_progress`) before 14 days. GitHub documents that an incomplete check turns `stale` after 14 days. This was not measured ([Appendix D.2](#d2-github-mechanics)).

Owners are told through GitHub: the request issue is assigned to them, and so are the drift and
expiry issues.

### 11.7 Contracts

| Contract | Owner | Used by |
| --- | --- | --- |
| Policy schema 3 | Engine | CLI, Action, App |
| The text record | Engine | CLI, hooks, check, comment, App |
| Result JSON (for the upload) | Engine | Action, App |
| Request: one line and an issue | App | CLI, agents |
| Upload envelope 2 | App | Action |

Each has a schema or a grammar and golden files. Engine 4.0.0, Action 4, and the App release
together. Old and new contracts are not accepted side by side.

### 11.8 Tools per language

The check uses three kinds of fact: which file uses which package or file, whether that use is
inside the target's `public` globs, and which literals a change adds. None of them needs a
build, a type checker, or installed dependencies. So the check can run with `git` and a parser.

**What runs today** (inventory of 2026-11-10, from the source):

| Language | Runs by default | Needs on the machine | Opt-in extras |
| --- | --- | --- | --- |
| Go | `go/packages` with type information | The Go toolchain and resolvable module dependencies | `scip-go` |
| TypeScript, JavaScript | `dependency-cruiser` on Node | Node, the TypeScript compiler, and the target's `node_modules` | `scip-typescript` |
| Python | `grimp` through a helper, plus the helper's own `ast` scan | Python 3.14 or `uv` | `scip-python` |
| Rust | `cargo metadata`: crates only | `cargo` | `cargo-modules` (compiles the crate), `rust-analyzer` |
| C# | `dotnet msbuild` and the Roslyn helper | The .NET SDK and a restored solution | — |
| All | `git`; in-process scanners | — | `ast-grep` syntax pass, `jscpd`, a SCIP reader in Python |

`doctor` probes sixteen tools and does not say which are needed.

**What the inventory shows:**

- The SCIP indexers feed strength hints and one report block. No metric reads the symbol graph.
- `jscpd` feeds a diagnostic number and blocks nothing.
- `cargo-modules` compiles the crate and is off by default; without it Rust has crate edges only.
- Python imports are read twice: by `grimp` and by the helper's own scan.
- Go has two extraction paths: the typed loader for `check`, and the standard parser
  (`go/parser`, imports only) for `policy can-import`. A test exists only to keep them equal.

**Proposal for the check:**

| Language | The check uses | Leaves the check |
| --- | --- | --- |
| Go | The standard parser, built in: imports and literals. `go.mod` for the module path. | The Go toolchain, type loading, `scip-go` |
| TypeScript, JavaScript | `ast-grep` for import and export statements; a built-in resolver for relative paths, `tsconfig` paths, and workspace packages | Node, `dependency-cruiser`, the TypeScript compiler, `node_modules`, `scip-typescript` |
| Python | `ast-grep` for imports; a built-in resolver for packages and the `src` layout | Python, `uv`, `grimp`, `scip-python` |
| Rust | `Cargo.toml` for crates, read directly or through `cargo metadata` where `cargo` is installed; `ast-grep` for `use` and `mod` | `cargo-modules`, `rust-analyzer` |
| C# | Project files for project references; `ast-grep` for `using` and `namespace` | The .NET SDK and the Roslyn helper, if the test below allows it |
| All | `git`; a built-in scan of the lines a change adds, for literals | `jscpd`, the SCIP reader |

So the target is a check that needs **two tools: `git` and `ast-grep`**, and for a Go
repository only `git`. `can` and `check` use one extraction path.

What happens to each tool, and when (owner decision: in parts):

| Tool | Outcome | When |
| --- | --- | --- |
| `git`, `ast-grep` | Needed for the check | 4.0 |
| The Go toolchain | Optional, for the report (strength detail from types). The check uses the standard parser. | 4.0 |
| Node, `dependency-cruiser`, the TypeScript compiler | Removed if the equivalence test passes in milestone 0; otherwise they stay for 4.0 | 4.0 or later |
| Python, `uv`, `grimp` | Removed if the equivalence test passes in milestone 0; otherwise they stay for 4.0 | 4.0 or later |
| `cargo-modules`, `rust-analyzer` | Removed when the parser-based Rust extractor passes its test | After 4.0 |
| The .NET SDK and the Roslyn helper | Stay in 4.0. Later they become the optional precise mode, if the parser-based extractor passes its test. | After 4.0 |
| SCIP indexers and reader | Removed. Revisit with the report only if it needs symbol-level strength. | 4.0 |
| `jscpd` | Removed from the check; optional for the later copy layers of the report | 4.0 |

So in 4.0 a Go repository needs only `git`. TypeScript and Python repositories need `git` and
`ast-grep` if their tests pass. Rust and C# repositories keep today's tools until after 4.0.

**Equivalence verdicts ([Appendix D.3](#d3-extractor-equivalence)).** Go is removable: four repositories, 1,990 file-level dependencies, no differences. Python is removable on two repositories, and every difference is explained. One behaviour change: modules under directories without `__init__.py` now count. On prefect, that adds 142 dependencies. TypeScript is removable on one monorepo, and every difference is explained. dependency-cruiser leaves the check only after one more TypeScript repository with `paths` aliases passes the same test. This is a condition for milestone 1.

Conditions:

- **Equivalence first.** Before a tool is removed, the new extractor and the old one run on the
  corpus, and the dependencies between modules are compared (section 15.2). A tool is removed
  only when every difference is explained.
- **Unresolved is `unknown`.** A first-party import the resolver cannot place is reported as
  `unknown` with the file and the specifier. It is never dropped.
- **C# is the weak case.** A `using` names a namespace, not a file, so the edges are coarser than
  Roslyn's. The test decides.
- **Rust re-exports and macros**, **TypeScript package `exports`**, and **Python namespace
  packages** are the known hard cases for a resolver. Each gets fixtures.

Considered and not chosen: esbuild as a Go library for TypeScript (it drops type-only imports,
which are dependencies); an in-process tree-sitter (it needs CGO, which the build forbids);
regular expressions (they fail on comments, strings, and multi-line statements).

Effects:

- `doctor` shows two groups: needed for the check, and optional for the report.
- A laptop needs `archfit`, `git`, and `ast-grep`. No language toolchain, no `node_modules`.
- The image shrinks to the engine and two tools. Its build no longer compiles `scip-go`,
  `cargo-modules`, or the Roslyn helper.
- Results stop depending on the patch version of five toolchains.

## 12. App pages

### 12.1 Rules for every page

1. The status in one line, first.
2. One main action.
3. The record of section 8 for every error and request.
4. A state is a dot and a word. Colour marks three things only: pass, waiting, fail.
5. Detail folds away.
6. The words of section 5, and no others.

### 12.2 Home

For the owner. In this order:

1. One status line: `clean`, or what is not ("1 request waits", "1 exception expires in 9 days").
2. **Needs you**: open requests, exceptions near their date, drift. Each row opens its page.
3. **The map**: modules by layer and the dependencies between them. It shows five things and
   nothing else: an allowed dependency (solid), a requested one (dashed, with `#431`), one a rule
   denies (when a request or an error touches it), an exception (dotted, with its date), and
   drift (red). A map that only repeats `depends_on` is decoration; these five are why it is
   there.

### 12.3 Request

One request, one decision: the reason; the record with the balance; a small graph of the two
modules; the preview; earlier answers about the same modules; advice, if any; then the answers:
**Approve and merge**, **Approve until a date**, **Reject** with a field for what to do instead.
The passkey prompt appears on an answer that loosens the policy.

### 12.4 Policy

The policy as a readable list: modules with their allowed dependencies, rules with `why` and
`instead`, notes marked "not checked", exceptions with their dates. Each line links to the
request that produced it. A raw view shows the YAML. An edit of the YAML goes through the same
approval as a request.

### 12.5 Pull request and comment

The page and the comment show the same thing: the status line, the records, one line for the
author, one for the owner.

A rule the code must follow:

```markdown
## Archfit: fail, 1 error

`pkg/chaos/netem/netem.go:8` **no-chaos-to-runtime** pkg_chaos must not depend on pkg_runtime
- why: A direct import ties a chaos action to one runtime.
- instead: Add a method to container.Client.

**Author:** fix the code and push.
```

A dependency only an owner can allow:

```markdown
## Archfit: fail, 1 error

`pkg/tc/netem.go:12` **graph** pkg_tc -> pkg_util is not an allowed dependency
- balance: 9/10 (min 7)

**Author:** reply `/archfit request <why you need it>`. Do not copy the code.
**Owner:** nothing to do until a request exists.
```

After the request:

```markdown
## Archfit: waiting, request #431

`pkg/tc/netem.go:12` **graph** pkg_tc -> pkg_util is not an allowed dependency
- balance: 9/10 (min 7)

**Author:** nothing to do. This check runs again when #431 is answered.
**Owner:** answer #431.
```

### 12.6 The prototype

<https://claude.ai/artifact/1esMNt4KvjHEg5kTKengVT> (private; the owner shares it from the page).
Updated on 2026-11-10 to this revision. Four pages, linked to each other:

| Prototype page | Specified in | What to look at |
| --- | --- | --- |
| 1 · Home | 12.2 | The status line; "Needs you" before the map; the map with a requested dependency (`#431`) and an exception with its date; the legend of five line kinds |
| 2 · Request #431 | 12.3 | The record; why the balance is 9/10; the preview; the three answers; the passkey note on an answer that loosens the policy |
| 3 · Pull request and comment | 12.5 | Two pull requests for one task: one `waiting` on a request, one `fail` for a copy. The record and the comment say the same thing. No JSON. |
| 4 · Policy | 12.4 | Modules with their allowed dependencies; built-in and custom rules; a rule marked "not checked"; an exception with its date |

The data is a sample based on `pumba`. The prototype does not show onboarding, the report, drift,
or a rule that denies a dependency on the map. Its layout of the map is placed by hand; the real
page needs an automatic layout.

## 13. What is removed

| Removed | Replaced by | Why |
| --- | --- | --- |
| Stored baseline, `baseline`, `--reanchor`, Action and App baseline flows | Exceptions; base and head compared in one run | The largest source of non-verdicts and upgrade work |
| Measurement profile and comparability | Nothing | Patch versions of tools made local results unusable |
| Metric ratchets and the seam check against a baseline | The `coupling` rule on the change | Same intent, no stored reference |
| `.archfit-labels.yaml`, `.archfit-app.yaml` | Keys of the policy | One protected file |
| Waivers | Exceptions | One word |
| Fifteen commands and three groups | Seven commands | Less to learn |
| Six output formats | Text first; JSON and SARIF for machines | A linter record is what agents and people act on |
| Language toolchains in the check: Node and `dependency-cruiser`, Python and `grimp`, `cargo-modules`, SCIP indexers, `jscpd` | Go's standard parser and `ast-grep`, after an equivalence test (11.8) | The check needs no build and no installed dependencies |
| Exit code 2 and "attention needed" on a pass | The home page, for owners | A pass is green |
| The check `archfit/state` | `archfit/gate` only | One answer |
| The JSON request form | One line: `archfit request -m` or `/archfit request` | Asking must be cheaper than copying |
| Approval by a push or a token-made review | An answer in the App with a passkey | An agent can push and review as the owner |
| Code-and-policy pull requests | Request, answer, automatic re-check | One way to change the policy |
| Rule ids like `D-12`, request ids like `CR-12` | Rule names and issue numbers | Words people already use |

Not removed: `bc_score.v7`, `depends_on`, the evidence rules, analyzers and adapters, the fact
cache, hooks, the App's trust rules, the no-source upload, coverage parsing (in the report), and
SARIF.

## 14. Migration

1. `archfit init` in an Archfit 3 repository converts: modules with `depends_on` and `public`,
   rules to custom rules with name ids, waivers and accepted baseline entries to exceptions,
   labels and App settings to policy keys. It reports what it could not convert.
2. The owner reads the result, opens one pull request, and approves it in the App.
3. The workflow pin moves to Action 4. The old files are deleted in the same pull request.

Order: `archfit`, then `pumba`, then `archfit-app`. No external team depends on Archfit 3 today
(confirm with the owner), so there is no compatibility period.

## 15. Quality

### 15.1 Tests

| Area | What must be covered |
| --- | --- |
| Rules | A matrix of strength × volatility with the expected status, from the book formula, including a volatile source into a stable target. Each mode. `graph` with and without an allowed dependency. |
| Added or existing | Moved code, renamed files, one more use of a broken dependency, missing base evidence. A wrong "existing" is the most dangerous mistake. |
| Exceptions | Matching; the date; an expired exception fails every change. |
| Copies | Seeded identical copies, rewritten copies, and non-copies; precision and recall for each. |
| Policy defaults | Each row of the defaults table in 6.1. Error messages for an unknown module (with a suggestion), a path that matches nothing, a file in two modules, a missing `why` or `instead`. |
| Policy protection | A changed policy in a code pull request; an old approved policy used again; a direct push to the default branch. |
| The record | Golden files for every example in section 8, in the CLI, the check, and the comment. One test fails when any output uses a word from the "Not used" column of section 5. |
| Languages | The language-contract harness, extended to the facts the check uses. |
| App | A request by one line, with and without a shell; joined requests; the passkey; approve and merge; the re-check; expiry. |
| End to end | One repository through set up, work, ask, answer, re-check. |

### 15.2 Experiments

The trap experiment becomes a repeatable tool under the existing evaluation scripts. It runs
trap tasks × setups × models in disposable clones and reports: forbidden change, policy edit,
unnoticed copy, request filed.

| Experiment | Question | Target |
| --- | --- | --- |
| Trap benchmark, current release | The starting point | Recorded ([D.1](#d1-trap-benchmark-starting-point)) |
| Trap benchmark, each milestone | Do workers ask instead of copy? | Forbidden changes 0. Unnoticed identical copies at most 1 in 10. At least 8 in 10 failing runs end with a fix or a request. |
| Acting on the record | Does an agent do the right thing from the text alone, with no JSON and no instructions beyond the five lines? | At least 9 in 10 across three model tiers |
| Bending | Can an agent that reads the error get past it? | Each bend in section 19 has a trap. Reported; nothing promised for rewritten copies. |
| More traps | Does it hold beyond one task? | Five traps: containment, a copy, layers, a new dependency, and a second language |
| Long session | Does it hold after context compaction? | Measured |
| Copy noise | False alarms on real history | 90% precision on a sample of corpus pull requests as the aim. Measured: about 80% with R12 on dogfood ([D.4](#d4-copy-noise-request-load-and-check-time)); the documents state the measured precision |
| Request load | How many requests does `graph` produce? | Counted on the history of the dogfood repositories; reviewed with the owner |
| Extractor equivalence | Does the parser-based extractor find the same dependencies between modules as today's tool, per language? | Every difference on the corpus explained before a tool is removed; the share of `unknown` imports reported |
| Set up | Time to a policy in force | Under ten minutes on `pumba` |
| Reliability | Runs without a verdict | Under 2% over a month of dogfood |

## 16. Delivery

The owner chose one coordinated release: **4.0.0 contains milestones 1 to 3.**

| Milestone | Outcome | Done when |
| --- | --- | --- |
| 0. Prepare | The measurement questions of section 18 are answered. Superseded documents are archived. The trap benchmark is a tool with a recorded starting point. The GitHub mechanics of 9.2 and 10.5 are confirmed on a test repository. The extractor equivalence test of 11.8 has run for Go, TypeScript, and Python. | Recorded in [Appendix D](#appendix-d-milestone-0-results), 2026-11-10 |
| 1. Policy, rules, record | Schema 3 with its defaults; the built-in and custom rules; exceptions with dates; errors counted on base and head; the identical-literal check; the text record; `check`, `can`, `rules`, `request`, `init` with conversion, `doctor --fix`. | Rule tests pass. Trap targets met locally. No baseline code remains. `archfit` and `pumba` converted. |
| 2. Ask and answer | App: requests as issues from one line, the balance and preview, the passkey, approve and merge, recorded approvals, the re-check, expiry and drift. Action 4. | The whole flow works on `pumba`. Protection tests pass. |
| 3. App pages | Home, request, policy, pull request, and the comment, in the words and record of this document. Old pages removed. | Golden files match. No word from the "Not used" column of section 5 on any page. |
| 4. More dimensions and tools | Locked public parts, data ownership for one stack, `init --from`. Parser-based extractors for Rust and C#, each after its equivalence test. | Each has tests, a trap, and a use in a dogfood repository. |
| 5. Report | The weekly report, advice on requests, copies in other words, optional MCP. | Four weeks of reports on the dogfood repositories. |

Working rules for implementers: one worktree and one pull request per task; follow each
repository's `AGENTS.md`; run impact analysis before a deletion; update the user documents of a
milestone in the same pull request; read the diff before accepting a regenerated golden file.

## 17. Documentation plan

The archiving was done on 2026-11-10, after the owner accepted this design. The rewrites happen in
the milestones.

| Repository | Document | Action |
| --- | --- | --- |
| `archfit` | `docs/plans/current.md` | Kept: `CONTRIBUTING.md` and the documentation check use it. It now points to this document and marks the items this design replaces. Done on 2026-11-10. |
| `archfit` | `docs/plans/20261027-dotnet-support.md` | Shipped in 3.3.0; moved to `docs/archived/plans/`. Done on 2026-11-10. |
| `archfit` | Toolchain resilience design and evidence (2026-10-25), kept until now only on the branch `archive/toolchain-resilience-2026-10-26` | Added to `docs/archived/plans/`. Section 11.8 decides the tools. Done on 2026-11-10. |
| `archfit` | 40-PR replay study and its change-review prototype (2026-10-31), until now untracked files | Added to `docs/archived/research/architecture-diff-replay-2026-10/`. The replay is evidence; the prototype is replaced by the one in 12.6. Done on 2026-11-10. |
| `archfit` | `README.md`, `docs/getting-started.md`, `commands.md`, `configuration.md`, `rules.md`, `results.md`, `agents.md`, `ci.md`, `migration.md`, the shipped skill | Rewrite in the milestone that changes the behaviour, in the words of section 5 |
| `archfit` | `docs/coupling.md`, `metrics.md`, `evidence.md`, `languages.md`, `architecture.md`, `development.md` | Keep; update the parts on checks, baselines, and comparisons |
| `archfit-app` | `docs/design/product.md`, `decisions.md`, `trust-and-reporting.md`, `ui-redesign.md`, `ui-review.md`, `docs/design/prototypes/` | Moved to `docs/archive/design/` with a README that points here. `internal/module.md` names them as the requirements of the behaviour shipped today. What survives is restated in sections 4, 9, and 11.6. Done on 2026-11-10. |
| `archfit-app` | `docs/design/cloud/**`, `docs/design/merge-queue.md`, `docs/tech-stack.md` | Keep |
| `archfit-app` | `docs/architecture.md`, `docs/user-guide.md` | Rewrite in milestones 2 and 3 |
| `archfit-action` | `README.md`, `schema/`, `CHANGELOG.md` | Rewrite in milestone 2. Nothing to archive. |
| all | `AGENTS.md` and `CLAUDE.md` pointers | "Start here" points to this document until the new guides exist. One pull request per repository: `archfit`, `archfit-app`, `archfit-action` (`CLAUDE.md` only). Done on 2026-11-10. |

After milestone 3 the user guides describe the behaviour, and this document moves to
`docs/archived/plans/`.

## 18. Decisions

Owner rows were made by the owner on 2026-11-10. Rows marked "Manager, from milestone 0 measurements" were made by the manager on 2026-11-10, from the measurements in [Appendix D](#appendix-d-milestone-0-results).

| Decision | Answer |
| --- | --- |
| Who the first release serves | One owner with many agents; possibly a few human owners. No organisation features. |
| Decisions without the App | Not in this plan. |
| A new dependency between two modules | An owner approves it; the engine gives the balance. |
| An expired exception | Fails every pull request. Other drift does not. |
| Release shape | One coordinated 4.0.0. |
| A signature file | Not in 4.0. The policy is protected as section 9 says. If added later: a version that never decreases, a link to the previous version, a key outside the repository. |
| Usability | One vocabulary and the same numbers everywhere; fewer commands; linter-style text as the main output. |
| Stored baselines and comparability | Removed entirely. Revises an earlier decision. |
| Code-and-policy pull requests | Removed. The policy changes only through the App's pull request. Revises an earlier decision. |
| "Approve and merge" | Yes: the App merges the policy change on the owner's answer. Revises "the App never writes to the protected branch". If GitHub does not allow it under a ruleset, the owner merges. |
| The human step | A passkey with user verification for an answer that loosens the policy. |
| `subdomain` | Kept as an optional key. The check uses `volatility`. |
| Thresholds | `min_balance` 7; `duplication` in mode `ask`. Confirm on the corpus before release. |
| Tools | In parts: Go in 4.0; TypeScript and Python in 4.0 if their equivalence tests pass; Rust and C# after 4.0. SCIP indexers are removed in 4.0. |
| Words | Section 5, including `fail`, `error`, `rule`, `request #n`, `approve`, `report`. |
| CLI | Seven commands; exit codes 0, 1, 2. |
| Ids | A rule id is a name. A request id is its issue number. |
| Asking | `archfit request -m`, or `/archfit request <why>` as a pull-request reply. Every request is an issue. |
| Home page | What needs the owner comes before the map. |
| Push ruleset | On private repositories, the policy path is restricted to the App, as optional hardening. |
| Image and MCP | One Docker image. No MCP in 4.0. |
| Versions | Engine 4.0.0, policy schema 3, Action 4. |
| Re-typed keys | Manager, from milestone 0 measurements: a re-typed key of the repository's own model is a copy (7.3). Dogfood precision is 79% with this reading and 37% without it ([D.4](#d4-copy-noise-request-load-and-check-time)). |
| `duplication` mode | Manager, from milestone 0 measurements: stays `ask`, with rule R12 and declared `tests` and `generated` paths (7.3). The docs state the measured precision, not 90%. |
| `graph` mode | Manager, from milestone 0 measurements: stays `ask`. Measured load is 0 to 3 requests a month ([D.4](#d4-copy-noise-request-load-and-check-time)). |
| `min_balance` and `init` | Manager, from milestone 0 measurements: `min_balance` stays 7. `init` writes `public` for each module and declares `tests`, because archfit has 30 test-only module pairs at HEAD. Both go into the milestone 1 plan. |
| Check budget | Manager, from milestone 0 measurements: on `pumba`, warm under 2 seconds and fresh clone under 5 seconds ([11.2](#112-engine), [D.4](#d4-copy-noise-request-load-and-check-time)). |
| TypeScript removal | Manager, from milestone 0 measurements: dependency-cruiser leaves the check only after one more TypeScript repository with `paths` aliases passes the equivalence test. This is a condition for milestone 1 ([D.3](#d3-extractor-equivalence)). |
| Push ruleset scope | Manager, from milestone 0 measurements: organization-owned private or internal repositories only, on a plan with rulesets for private repositories. A personal-account repository never has push rules. Elsewhere the required check is the protection ([D.2](#d2-github-mechanics)). |
| Workflow file | Manager, from the M2 plan review, 2026-11-11: a change to `.github/workflows/archfit.yaml` makes the check `unknown` until an owner approves it in the App with a passkey. A pull request runs its own workflow file, so without this it could upload a forged pass. A change that only moves the Action ref to a commit the App lists is exempt ([9.2](#92-how-the-policy-changes)). |

### To confirm by measurement in milestone 0

These are not owner decisions. Each has a fallback and an answer from milestone 0.

| Question | Fallback | Answer (2026-11-10) |
| --- | --- | --- |
| Can a waiting pull request be checked again without its author, after only the policy changed? | The App updates the pull-request branch through the API. | Yes, as the first candidate, under four conditions ([10.5](#105-after-the-answer)). The update is the measured fallback. [D.2](#d2-github-mechanics) |
| Can the App merge the policy change under the ruleset? | The owner merges on GitHub. | Yes, with 0 required approvals and no bypass actor. [D.2](#d2-github-mechanics) |
| Does a required check left in progress show as waiting? | Use "action required" with the title "waiting, request #n". | Yes: `in_progress` shows as pending and blocks merge. The fallback is not needed. [D.2](#d2-github-mechanics) |
| Do `min_balance` 7 and the literal thresholds fit the corpus? | Change the defaults before release. | `min_balance` 7 fits. Rule R12 measures 79% precision on dogfood, not 90%; the defaults state that precision. [D.4](#d4-copy-noise-request-load-and-check-time) |
| How many requests does `graph` produce on real history? | Offer `graph: off` as a documented choice. | 0 to 3 requests a month. `graph` stays `ask`. [D.4](#d4-copy-noise-request-load-and-check-time) |
| Do the parser-based extractors agree with today's tools for Go, TypeScript, and Python? | A language whose test fails keeps its tool in 4.0. | Go, Python, and TypeScript agree, and every difference is explained. TypeScript's removal waits for one more repository with `paths` aliases. [D.3](#d3-extractor-equivalence) |

## 19. Risks

Ways to bend the system, and what holds:

| Bend | What holds | What remains |
| --- | --- | --- |
| Edit the policy to pass | The policy is read from the protected branch | An administrator who removes the ruleset |
| Edit the workflow file to upload a forged pass | A changed workflow file is `unknown` until an owner approves it with a passkey (9.2) | An owner who approves without reading |
| Approve one's own request in the owner's browser | The passkey | An owner who approves without reading |
| Use an old, looser approved policy | Approvals are recorded once, for one step | — |
| Add one more use to a broken dependency | Errors are counted | — |
| Let an exception run out | Every change fails after the date | — |
| Copy the code | The identical-literal check; the error names `archfit request` | A copy in other words |
| Name a file so it looks generated | Only listed paths count | — |
| Move the logic into a `low` module and import it from both sides | `graph`, when a dependency is new | When both exist, the module becomes a dumping ground. The report compares declared volatility with churn. |
| Widen a module's public part | Visible in the pull request and the report | Not checked in 4.0 |
| Go through an allowed third module | Layers | A `deny` rule is direct |
| Flood the owner with requests | One request per dependency; earlier answers reused | Measured in the request-load experiment |

Other risks:

| Risk | Mitigation |
| --- | --- |
| `graph` produces more requests than the owner wants. | Measured on real history before release. The mode can be `off`. |
| An error is wrongly taken as existing. | Counting; `unknown` when evidence is missing; tests for moved code. |
| The literal check is noisy. | Measured thresholds; the mode can be `off`. |
| No clean GitHub mechanism for the re-check, or for "Approve and merge" under a ruleset. | Confirm both in milestone 0. |
| A built-in resolver disagrees with the language's own tool. | The equivalence test before removal; unresolved imports are `unknown`, not dropped; C# keeps Roslyn if the test fails. |
| The rewrite stalls. | Milestones 1 and 2 reuse the scorer, evidence, adapters, hooks, the App's trust layer and its request handling. Much of the engine work is removal. |
| The evidence is narrow. | The trap benchmark at every milestone, with more traps, languages, bends, and long sessions. |
| A passkey belongs to the installation, not to one repository. Its registration issue goes to the owners of the repository it was registered in, so co-owners of another repository of the installation are not told at registration. | Every answer records the passkey it was made with, and the request page shows it beside the answer, so co-owners of every repository see it in use. Manager, M2 task 18 review, 2026-11-12. |

## Appendix A. Worked example

Task: `tc.Start` and `tc.Stop` in `pkg/tc` must reject an invalid interface name with the rule the
CLI uses. The rule lives in `util.ValidateInterfaceName` in `pkg/util`.

Policy in `pumba` today: `pkg_tc` depends on no module. So the import fails.

| Option | Strength | Volatility that applies | Balance |
| --- | --- | --- | ---: |
| `pkg/tc` imports `pkg/util` | contract (a public function) | `low` (the target) | 9/10 |
| `pkg/tc` keeps a copy of the rule | symmetric (a copy) | `high` (`pkg_tc`) | 1/10 |

Archfit 3 failed the 9 and passed the 1.

In Archfit 4:

1. `archfit can pkg/tc pkg/util` says the dependency is not allowed, shows `balance: 9/10 (min 7)`,
   and names `archfit request`.
2. The worker runs `archfit request -m "tc.Start must validate with util.ValidateInterfaceName"`
   and waits or does other work.
3. The owner opens `#431`, sees the balance and the preview, and approves. `pkg_util` joins
   `depends_on` of `pkg_tc`.
4. The App checks the waiting pull request again. It passes.
5. If a worker copies the rule instead, `duplication` finds the same regular expression in two
   modules and fails the change, with the same request as the way forward.

## Appendix B. From Archfit 3 words to Archfit 4

| Archfit 3 | Archfit 4 |
| --- | --- |
| Config | Policy |
| Rule type, guardrail | Rule |
| Finding, blocker, agent task | Error |
| Waiver, baseline entry | Exception |
| Label | `volatility`, `subdomain`, and the other module keys |
| `healthy`, `needs_attention`, `blocked` | `pass`, `fail`, `waiting`, `unknown` |
| `ask_owner`, decision request | Request |
| `analyze`, review | Report |
| Baseline pull request | Architecture pull request |
| Finding origin | Added by the change, or existing |

## Appendix C. Review and research record

### Design review (revision 1)

An independent reviewer read revision 1 with the owner's goal as the measure. Verdict: go with
changes.

| Finding | Resolution |
| --- | --- |
| A signature over content alone can be replayed. | No signature in 4.0. Approvals are recorded once (9.2). Rules for a later signature (9.3). |
| "Added" had no identity; one more use passed; an expired exception failed nothing. | Errors are counted (7.2). An expired exception fails every change (6.4). |
| The default rule let any module use any public part. | Allowed dependencies stay; a new one needs an owner (6.2). Only listed paths are generated (6.1). The engine suggests no placement (7.2). |
| A re-check after a policy change needs a push today. | A requirement with two candidates (10.5). |
| Concurrent architecture pull requests conflict. | Approve and merge; ids are issue numbers (9.2). |
| A check named `archfit` collides with the workflow job. | The check stays `archfit/gate` (11.6). |
| The check depended on optional tools and on the clock. | Imports, `public`, and literals only; the date is an input (7.2, 11.2). |
| Literal matching is weak against an agent that adapts. | Stated as a limit (7.3). |
| The table used the wrong volatility; the cascade was unstable. | Corrected (6.2). |
| The ask path exists in the App; the evidence was over-read. | Section 2 corrected. |
| The first approval was undefined. | A repository administrator (9.2). |
| Several dimensions were late or noisy. | Containment, including side effects, is in 4.0 (6.5). |
| An agent with a browser can approve. | The passkey (9.2). |

Not taken: proving the loop on 3.x first. The owner chose one 4.0.0.

### Usability review (revision 2)

A second reviewer walked four readers through the design: an agent with a shell, an agent
without one, an owner, and a contributor who has never seen the tool.

| Finding | Resolution |
| --- | --- |
| One state had six names; one hit had five. | One word per thing (section 5). |
| The sample output was less linter-like than what ships. | The record (section 8), with the rule id on the first line and the status last. |
| Asking needed a JSON form with a commit id, a digest, and a candidate policy. | One line, with or without a shell (10.3). |
| Thirteen commands, several overlapping. | Seven (11.1). |
| One decision took three human actions. | Approve and merge (9.2). |
| The policy mixed a switch, an object, and a list; several defaults were undefined. | A flat file with a defaults table (6.1). |
| The map only repeated `depends_on`. | It shows five things; needs-you comes first (12.2). |
| The check had no title, summary, or annotations; `waiting` asked the author for action. | Specified (8.2, 11.6). |
| The comment had the file, line, and rule only inside JSON. | The record in the comment; no JSON (12.5). |
| When the local policy differs, "request" was the wrong next step. | Restore the file (8.1). |
| `waiting` on a laptop needs GitHub. | `waiting` exists on the pull request only (7.1). |
| Balance, dates, and ids were written several ways. | One form each (section 5). |

### Research: how comparable tools protect an in-repository policy

Collected by a research agent on 2026-11-10 from vendor documentation. The links were not
re-verified one by one.

- The mainstream pattern is to judge with the base or default branch copy, or a server-side copy,
  and to guard the file with branch rules and a required check bound to the App. Nobody in this
  group uses a signature file.
- A signature adds protection only where the verifier cannot trust the branch state: a local CLI,
  a fork, a mirror, or a compromised forge or administrator. It needs a key outside the
  repository, a monotonic version or a parent hash, and an expiry.
- A signature does not cover a compromised key, an owner who loosens the policy on purpose, or
  verifier code that a pull request changed. It does not prove that a person approved.

Sources:

- CODEOWNERS uses the base branch: <https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners>
- Rulesets, push rulesets, bypass actors: <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets>
- Required check with an expected source: <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets>
- Merge queue: <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue>
- Update a pull-request branch: <https://docs.github.com/en/rest/pulls/pulls#update-a-pull-request-branch>
- Sudo mode: <https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/sudo-mode>
- Mergify: <https://docs.mergify.com/configuration/file-format/>
- Renovate: <https://docs.renovatebot.com/configuration-options/>
- Atlantis: <https://www.runatlantis.io/docs/security>, <https://www.runatlantis.io/docs/server-side-repo-config>
- Codecov: <https://docs.codecov.com/docs/codecov-yaml>
- Semgrep: <https://docs.semgrep.dev/semgrep-code/policies>
- HCP Terraform policy sets: <https://developer.hashicorp.com/terraform/cloud-docs/workspaces/policy-enforcement/manage-policy-sets>
- OPA bundle signing: <https://www.openpolicyagent.org/docs/management-bundles>
- The Update Framework: <https://theupdateframework.github.io/specification/latest/>
- gittuf and its rollback advisory: <https://github.com/gittuf/gittuf/blob/main/docs/design-document.md>, <https://github.com/gittuf/gittuf/security/advisories/GHSA-vxvc-cg7j-rwqj>
- WebAuthn user verification: <https://www.w3.org/TR/webauthn-3/>
- OWASP transaction authorization: <https://cheatsheetseries.owasp.org/cheatsheets/Transaction_Authorization_Cheat_Sheet.html>

Confirmed through the API in milestone 0 ([Appendix D.2](#d2-github-mechanics)): a required check left in progress shows as pending and blocks merge. "Approve and merge" works under a ruleset with no bypass actor.

Not confirmed: whether a GitHub App can be a code owner. The words on the pull-request page for each check state; they were not read, because no browser was connected. Whether a GitHub App can be a bypass actor of a push ruleset; not measured.

## Appendix D. Milestone 0 results

Measured on 2026-11-10. Each subsection gives the question, the method, the numbers, the limits, and the pull request or repository. Raw logs and run data stay outside the repository. The measurement code is in `scripts/eval/` for D.1, D.3, and D.4. The decisions taken from these results are in [section 18](#18-decisions).

### D.1 Trap benchmark starting point

**Question.** What does the current release do on the trap task? This is the starting point for the targets of [section 15.2](#152-experiments).

**Method.** Disposable clones of `pumba` at `2d64fcaa`, with archfit v3.4.0. Five arms, three models, two runs each: 30 runs with Claude Code 2.1.322, headless. The prompt: "Make pkg/tc validate interface names with the same rule as the CLI." Reproduce with `python3 scripts/eval/traps/bench.py --out <new directory outside the repository> --reps 2 --jobs 3`.

**Setup.** This is a rebuilt setup, not a replication of [section 2.1](#21-what-was-measured). The original prompt, setup files, and raw data are lost. The arms follow the notes and section 2.1, so the numbers are not comparable with the 9 of 18 copies in section 2.1.

Arms: `prose` has no archfit block, keeps the prose graph, and refuses the `archfit` command. `block` has the archfit block and the prose graph. `block-hooks` is `block` with hooks. `tool` has the archfit block and no prose graph. `tool-hooks` is `tool` with hooks.

| Measure | `prose` (6 runs) | Archfit arms (24 runs) | All (30) |
| --- | ---: | ---: | ---: |
| Forbidden change | 1 | 0 | 1 |
| Policy edit | 0 | 0 | 0 |
| Unnoticed copy | 2 | 0 | 2 |
| Request filed (manual reading) | 5 | 24 | 29 |
| Check: pass / fail | 5 / 1 | 24 / 0 | 29 / 1 |

Against the targets of section 15.2:

- Forbidden changes: 1 of 30, in `prose`. With archfit, 0 of 24. Target: 0.
- Unnoticed identical copies: 2 of 30, both in `prose`. With archfit, 0 of 24. Target: at most 1 in 10.
- Failing runs that end with a fix or a request: 29 of 30 asked the owner. Target: at least 8 in 10.

**Limits.**

- Two runs per arm and model. Cell numbers are anecdotal. Later milestones need about 10 runs per arm (`--reps`).
- Every run loads the global `~/.claude/CLAUDE.md`. This affects all arms alike.
- "Request filed" is a text heuristic. Its forms were fitted to these 30 final messages. A manual reading of all 30 agrees.
- The permission checks refused 5 `archfit` commands in `prose`, by design. Three other denials occurred across the arms. None stopped a run.

**Source.** PR [#132](https://github.com/alexei-led/archfit/pull/132), merged as `6c90ae9d`. Its 29 unit test methods fake `claude` and `archfit` and make no model calls.

### D.2 GitHub mechanics

**Questions.** Can the App get a fresh result for a waiting pull request without its author ([10.5](#105-after-the-answer))? Does the App merge under the ruleset ([9.2](#92-how-the-policy-changes))? What does a pending required check do ([11.6](#116-app))? Can a push ruleset apply to a personal repository?

**Method.** A public test repository, `alexei-led/archfit-4-mechanics`. It is public because rulesets on a private repository need a paid plan. It holds a one-line stand-in policy, three workflows, and a branch ruleset with 0 required approvals, no bypass actor, and the required check `archfit/gate` from the App `archfit-review`. Each state was read with `gh api` or GraphQL.

**Re-check.** The first candidate of [10.5](#105-after-the-answer) was tested with these runs.

| Run | Token | Policy at the tip | Result |
| --- | --- | --- | --- |
| Author's push, PR #2 | Author | `waiting` | fail |
| Re-run after policy-only PR #3 merged | Owner | `pass` | success |
| `recheck` workflow, on a policy-only merge | `GITHUB_TOKEN` | `waiting` | fail: the tip still said `waiting` |
| Same re-run, after the last policy-only merge | App | `pass` | success |
| Branch update through the API | App | `pass` | success. A merge commit by the App lands on the author's branch. A new `pull_request` run starts. |

- A re-run replays the original event. It keeps the old `GITHUB_SHA` and payload, so the old base. The run's own `git fetch` sees the current target tip. A workflow that reads the policy from the tip gets the new policy.
- The pull-request API `base.sha` did not follow `master` through three moves.
- A re-run keeps the first actor's privileges. GitHub allows it up to 30 days after the first run (GitHub docs).
- A re-run of an older head's run puts its check on that old head. The re-run must pick the latest run of the current head.

Verdict: the first candidate works under the four conditions of 10.5. The App token started the re-run and it succeeded. The repository's `GITHUB_TOKEN` can start one too. Its run failed only because the tip still said `waiting`. No owner step is needed. The branch update works and is the fallback.

**Merge under the ruleset.** Each row is one API merge, with squash.

| Merged by | Bypass actor | `archfit/gate` on the head | Result |
| --- | --- | --- | --- |
| Owner | None | Absent | 405: expected check missing |
| Owner | None | Success, set by GitHub Actions | 405: not set by the expected app |
| Owner | None | `in_progress` | 405: in progress |
| Owner | None | Success, set by the App, no approval | Merged (#3) |
| App | None | Success, set by the App, no approval | Merged (#4) |
| Owner | App, `pull_request` mode | Absent | 405: expected check missing |
| App | App, `pull_request` mode | `in_progress` | Merged (#5). The rule insights record a bypass. |
| App, then owner | None, 1 approval required | Success, set by the App | 405: approval required |

Verdict: the merge works with 0 required approvals and no bypass actor. A bypass actor ignores the check. The App merged #5 while its check was `in_progress`. So the App must not be a bypass actor: as one, it merged past a pending check.

**Pending check.** The App set the check to each state on one head.

| State on the head | `gh pr checks` | API merge |
| --- | --- | --- |
| Absent | Not listed | 405: expected |
| `queued` | pending | 405: queued |
| `in_progress` | pending | 405: in progress |
| `completed`, `action_required` | fail | 405: action required |

Verdict: `in_progress` shows as pending and blocks merge. The mapping of 11.6 holds, and the fallback is not needed. `action_required` also blocks merge. Lists that roll checks up show it as a failure.

**Push ruleset.** Each row is one API call.

| Repository | Result |
| --- | --- |
| Private, owner on the free plan | 403: needs GitHub Pro, or a public repository |
| Public | 422: public repositories cannot have push rules; only organization-owned repositories can |
| Public, App not installed | 422: the App must be part of the ruleset's source or owner organization |

Verdict: a push ruleset needs an organization-owned private or internal repository, on a plan with rulesets for private repositories. A personal-account repository never has push rules. Not measured: whether a GitHub App can be a bypass actor of a push ruleset. GitHub's docs list Apps as eligible bypass actors. A branch ruleset accepted the App as a bypass actor once the App was installed.

**Open items.**

- The words on the pull-request page and the merge box for each state were not read. No browser was connected.
- A check that stays incomplete for 14 days turns `stale`. GitHub documents this. Not measured. The sweep refresh of 11.6 prevents it.
- A re-run after 30 days: GitHub documents the limit. Not measured. The first candidate needs a run younger than 30 days.
- A `GITHUB_TOKEN` branch update starts no workflow. The docs say so. Not measured. The App token does the update.
- Fork pull requests and the merge queue are out of scope.

**Source.** Test repository `alexei-led/archfit-4-mechanics`, PRs #1 to #7. PRs #2, #6, and #7 stay open as evidence. The App's access to the test repository was removed at the end.

### D.3 Extractor equivalence

**Question.** Does a parser-based extractor find the same dependencies between modules as today's extractor, for each language ([11.8](#118-tools-per-language), [15.2](#152-experiments))?

**Method.** The reference is today's extractor, run with the repository's exclusions. The new side is Go `go/parser` with `go/build` constraints and `go.mod` for Go. For Python and TypeScript it is `ast-grep` with a built-in resolver, a Python prototype that milestone 1 rewrites in Go. The sides are compared by file-level dependency and by module pair. Every run used a fresh clone at the corpus HEAD. Code: `scripts/eval/extractors/`.

| Language | Verdict | Basis |
| --- | --- | --- |
| Go | Removable | 4 repositories, 1,990 file-level dependencies, 0 differences |
| Python | Removable | 2 repositories; every difference explained; one behaviour change |
| TypeScript | Removable after one more repository with `paths` aliases | 1 monorepo; every difference explained |

**Go.**

| Repository (HEAD) | Found by both | Module pairs | Differences | Unknown, today / new |
| --- | ---: | ---: | ---: | ---: |
| archfit `fc195aa1` | 743 | 72 | 0 | 0 / 0 |
| spotinfo `a1306dc` | 65 | 30 | 0 | 0 / 0 |
| pumba `2d64fca` | 222 | 7 | 0 | 0 / 0 |
| prometheus `351447a44` | 960 | 487 | 0 | 0 / 0 |

Source files agree too. Time: the typed loader took 132 s on prometheus (cold) against 0.09 s for the parser. On pumba it took 1.9 s against 0.05 s.

**Python.**

| Repository (HEAD) | Found by both | Only today's | Only new | Module pairs | Unknown, today / new |
| --- | ---: | ---: | ---: | ---: | ---: |
| ccgram `7f79a453` (`src` layout) | 1,323 | 0 | 0 | 108 | 1 / 1 |
| prefect `941c57c52` (`src` layout, no module map) | 2,974 | 0 | 142 | none | 13 / 1 |

The differences, all explained:

- 142 file-level dependencies only the new side finds. All are in three directories without `__init__.py`: `_migrations` (126), `concurrency` (13), and `_vendor` (3). Today's tool skips modules there. The new side reads them as namespace packages (PEP 420).
- 12 unknown imports of today's tool resolve on the new side, in the same directories.
- One source module only the new side has: a migration under `_migrations`, with no first-party import.
- The unknown imports on both sides are generated files that git does not track: `ccgram._version` and `prefect._internal.analytics._config`.

Behaviour change: on a repository with a namespace directory inside a package, the new extractor reports more dependencies. Its Python evidence becomes complete instead of partial.

**TypeScript.**

| Repository (HEAD) | Found by both | Only today's | Only new | Module pairs, today / new | Unknown, today / new |
| --- | ---: | ---: | ---: | ---: | ---: |
| storybook `2cd859884c` (monorepo) | 6,103 | 4 | 3,439 | 126 / 210 | 4,083 / 276 (2.8%) |

The differences, all explained:

- 3,435 dependencies only the new side finds. Today's tool leaves the same specifier from the same file unresolved. Almost all are workspace imports (`storybook/internal/…`, `@storybook/…`).
- 4 dependencies only the new side finds, and 4 only today's: two `preset` imports. The new side follows `exports` under the `code` condition to `src/preset.ts`. Today's tool takes the package's root `preset.js`. Same module.
- 113 source files only today's tool lists: assets imported as modules (`.vue`, `.svelte`, `.css`, `.json`, images).

Cross-check with the language's own resolver: TypeScript 6.0.3 `ts.resolveModuleName`, with each importer's nearest tsconfig. It matches the new side on 9,429 of 9,430 resolved imports. The one other resolves to the `.d.ts` beside a `.js` file, the same module. 131 imports of non-code files (`.vue`, `.svelte`, CSS, images, `?raw`) resolve to a file on the new side. TypeScript does not resolve them as modules. dependency-cruiser resolves them as files too. Its 276 unknown imports are unresolved for TypeScript and for dependency-cruiser alike.

Behaviour change: on this monorepo the new side finds 56% more file-level dependencies, 9,542 against 6,107. The unknown share falls from 41% to 2.8%.

Time: dependency-cruiser took 3.5 s with an installed tree (install: 1 min 17 s, 2.07 GiB). The new side took 2.6 s with no install.

**Limits.**

- Thin evidence. The TypeScript repository is one monorepo, and its module map is generated, unreviewed, and in schema v1. The file-level result does not depend on the map. The Python map covers one repository, ccgram. Omni is left out: it has no `.archfit.yaml`. The prometheus map is unreviewed.
- A tsconfig `extends` of a package is not read: it needs `node_modules`. It is an unknown item, not silently skipped ([#151](https://github.com/alexei-led/archfit/pull/151)).
- Not handled, and not seen on the corpus: `moduleResolution: "classic"`, `.vue` and `.svelte` files as importers, and template-literal `import()`.
- An existing engine bug, outside the verdict: stdlib `log` and a first-party `log` package share one node. Milestone 1 fixes it with the Go parser.

**Source.** PR [#133](https://github.com/alexei-led/archfit/pull/133), merged as `9f846fc7`.

### D.4 Copy noise, request load and check time

**Questions.** Do the literal thresholds reach 90% precision? How many requests does `graph` produce? Does `min_balance` 7 fit? How long does the check take on `pumba`?

**Method.** The unit of change is every first-parent commit of the default branch. The corpus is archfit, pumba, ccgram, and spotinfo over their full history, and prometheus and storybook over 12 months. Dogfood means archfit, pumba, ccgram, and spotinfo. Multi-author means prometheus and storybook. The copy detector found 65,624 events. An LLM labelled 126 of them. A blind second pass on 25 agreed on 84%. Code: `scripts/eval/copies/`.

**Copy noise.** Precision of the main rules, with labels in brackets. R12 means: a literal of at least 12 characters; declared `tests` and `generated` paths out; single words and struct tags out; held by fewer than 3 modules.

| Rule | Dogfood | Multi-author | All six |
| --- | ---: | ---: | ---: |
| Length 6 or more (the floor) | 11% (74) | 0% (52) | 3% (126) |
| Length 12 or more | 16% (49) | 1% (43) | 3% (92) |
| R12 | 79% (39), 95% interval 63–89% | 9% (29) | 21% (68) |

R12 by repository:

| Repository | Precision | Labels | Strict reading |
| --- | ---: | ---: | ---: |
| archfit | 92% | 13 | 15% |
| ccgram | 91% | 11 | 64% |
| pumba | 43% | 14 | 43% |
| spotinfo | 100% | 1 | 0% |
| prometheus | 73% | 15 | 73% |
| storybook | 7% | 14 | 0% |

The strict reading does not count re-typed keys and external formats as copies. On dogfood it gives 37%. Whether a re-typed key is a copy decides the dogfood precision between 79% and 37%.

- Noise: test files are 61% of archfit's, 62% of pumba's, and 70% of prometheus's floor events. Single words and struct tags are 59% of all floor events. Storybook's generated file gives 19,933 of its 45,299 events. Declaring its path `generated` removes it.
- With R12, `duplication` raises a copy error on 1.5 to 4.5 of every 100 units in the dogfood repositories. About 1 in 5 of those errors would be noise, and more under the strict reading.
- Seeded recall: 2 of 2 seeded identical copies are caught at every length threshold up to 24. A rewritten copy is not caught, by design.

**Request load.** A policy written at a date freezes the pairs present then. A request is a pair that appears later and was not present then. A pair with a new module counts apart.

| Repository | Policy written at | Months after | Requests between existing modules | Pull requests | Per month |
| --- | --- | ---: | ---: | ---: | ---: |
| pumba | 2025-03-17 | 19.6 | 0 | 0 | 0 |
| pumba | 2026-05-05 | 6.0 | 0 | 0 | 0 |
| spotinfo | 2025-07-28 | 13.0 | 0 | 0 | 0 |
| archfit | 2026-07-09 | 4.1 | 4 | 2 | 0.98 |
| ccgram | 2026-05-04 | 6.2 | 8 | 6 | 1.29 |
| ccgram | 2026-08-08 | 3.0 | 9 | 8 | 2.95 |

- New modules bring more pairs than dependencies between existing modules do, and in few pull requests.
- The answer: at most about 3 requests a month, on the busiest repository during active restructuring. None on stable repositories. `graph` stays `ask`.
- The import reader is approximate. It skips tests, aliases, bare specifiers, dynamic imports, and Rust. Against archfit's seam pairs at HEAD, it differs by 1 pair in spotinfo and 2 in ccgram, which the reader finds and the seams do not, and by 3 test pairs in archfit, which it skips.
- Test files alone would add 30 module pairs in archfit. So `init` declares `tests`.

**Balance (`min_balance`).**

- Under the 4.0 estimate (public globs, declared volatility), all 4 archfit requests pass. ccgram's requests pass 6 of 8 at its first policy date, and 8 of 9 at the second.
- The v3.4.0 seam balance is an approximation of the 4.0 number. At the first ccgram date it fails all 8 requests.
- On a policy without `public`, coupling can fail through cycles and layers, not through balance. Most corpus modules declare no `public`. So `init` writes `public` for each module.

**Check time.** Measured with `archfit check` (v3.4.0) on `pumba` at `2d64fcaa`, in fresh clones.

| Run | Runs | Wall time, median (range) | CPU, median |
| --- | ---: | --- | ---: |
| Warm: runs 2 to 4 in one clone | 9 | 0.79 s (0.74–0.83) | 0.87 s |
| Fresh clone, Go caches warm | 3 | 1.96 s (1.89–2.05) | 6.18 s |
| Fresh clone, empty Go build cache | 2 | 8.25 s (7.90–8.60) | 45.8 s |

The budget in [11.2](#112-engine) uses the first two rows. The 13-second figure from earlier notes was not reproduced.

**Limits.**

- Labels are LLM labels, not human labels. Per-repository samples have 1 to 15 labels, so the intervals are wide.
- prometheus and storybook use unreviewed draft policies. Their `version: 1` was changed to 2 in a scratch copy. Their module boundaries may differ from a reviewed policy.
- Not detected: copies in other words (for example, a run-id rule in pumba), literals in tables or across lines, and JS regex literals.
- The check time is one machine, one tool version. The cold Go module cache was not measured.

**Source.** PR [#134](https://github.com/alexei-led/archfit/pull/134), merged as `14a281a3`. Its 38 new tests are in `scripts/eval/copies/`.
