# Archfit 0.9: new home, one version, guided beta

Status: proposed, 2026-11-20. Owner: Alexei Ledenev.

Sources:

- The beta readiness review v0.4 of 2026-11-19 (private page, not in this repository).
- The 4.1 design, `docs/plans/20261117-archfit-4-1-surfaces-and-suggestions.md` on branch
  `docs/archfit-4-1-design`. This plan takes four pieces from it and leaves the rest.
- Facts checked on 2026-11-20 in the three repositories, the Go proxy and GitHub. Section 7
  lists what was not checked.

## 1. Outcome

1. The engine, the Action and the App live in the `coupling-dev` organization.
2. Every artifact a user installs has one number: **0.9.0**.
3. The four engine errors that block a beta are fixed in that release.
4. The four onboarded repositories run 0.9.0 and nothing else.
5. A stranger gets a verdict on a pull request in under ten minutes (release 0.10.0).
6. A guided beta with 3 to 5 teams starts, with a scorecard and an exit rule.

## 2. Decisions

Made by the owner on 2026-11-20:

| Decision | Answer |
| --- | --- |
| Home | New repositories in `coupling-dev`. History is pushed without tags. |
| Old repositories | Not changed until the last onboarded repository is migrated. Then archived with a pointer. Not deleted. |
| Number | 0.9.0 for the engine, the image, the Action, the Homebrew formula and the App. |
| Old tags, releases, images | Stay in the old home. Nothing is retagged or deleted. |
| Scope of 0.9.0 | The rename, the deletions, and the four fixes of epic 3. |
| Shape | One ordered plan. Migration first, then code. |

| Policy key `model` (D1) | In 0.9.0. |
| Advisory before the first passkey (D2) | Yes. |
| Policy file version (D3) | `version: 1`. |
| Design names (D4) | Versions 4 and 5 are released, so the designs do not carry a number. The 4.0 design is "the change-control design". The 4.1 design is "the surfaces design". This plan is "the beta plan". Files keep their paths; titles and the `CLAUDE.md` and `AGENTS.md` pointers change in task 1.4. |
| Rule for 0.x (D5) | A minor release may break. A patch never does. 1.0.0 freezes the policy file, the exit codes and record text of `check` and `can`, and the result JSON. |
| Priority | Onboarding of the engine and of the App is the main goal after the reset. No partner is invited before epic 8 is done. |

Open:

| Id | Decision | Default |
| --- | --- | --- |
| D6 | Who may loosen the policy, and how the tool knows it was a person. | The passkey stays for the beta. Task 8.1 writes the threat model and compares the options before any code. |

## 3. The target names

Every task copies from this table. Nothing else is a source for a name or a number.

| Thing | Value |
| --- | --- |
| Engine repository | `coupling-dev/archfit` |
| Go module | `github.com/coupling-dev/archfit` (no `/vN` suffix) |
| Install with Go | `go install github.com/coupling-dev/archfit/cmd/archfit@v0.9.0` |
| Install with Homebrew | `brew install coupling-dev/tap/archfit` |
| Image | `ghcr.io/coupling-dev/archfit:v0.9.0`, pinned by digest for each platform |
| Action | `coupling-dev/archfit-action@<commit> # v0.9.0` |
| App | `coupling-dev/archfit-app`, module `github.com/coupling-dev/archfit-app`, tag `v0.9.0` |
| Schema `$id` | `https://raw.githubusercontent.com/coupling-dev/archfit/main/<file>` |
| Policy file | `version: 1` |
| Endpoint | `https://app.coupling.dev` (not changed) |
| Contract ids | `archfit.result.v1` and the others stay internal. No user document names them. |

## 4. How to run this plan

### 4.1 Start here, for a new session

1. Read this file, then the two designs it builds on: the change-control design
   (`20261110-archfit-4-architecture-change-control.md`) and the surfaces design
   (`20261117-archfit-4-1-surfaces-and-suggestions.md`), sections 6.1, 6.2, 6.6 and 6.9.
2. Read section 8, the record. It holds what earlier sessions produced: commits, digests,
   pull requests, answers to open points. Trust it over memory, and check a value before use.
3. Local checkouts: `~/Workspace/archfit` (engine), `~/Workspace/archfit-action`,
   `~/Workspace/archfit-app`. Worktrees go to `~/Workspace/<repo>.worktrees/<branch>`.
   Onboarded repositories: `archfit-4-mechanics`, `archfit`, `ccgram`, `pumba` (default
   branch `master`).
4. Find the first task that is not `[x]` and whose dependencies are done.

### 4.2 Rules

1. One task is one worktree and one pull request in the repository the task names. Follow that
   repository's `AGENTS.md` and `CLAUDE.md`. In the engine: impact analysis before an edit,
   `archfit check` before finishing, and read the diff before accepting a regenerated golden
   file.
2. A behaviour change brings a test that fails before the change, and its user documents, in
   the same pull request.
3. A task is done when its pull request is merged, the checks below pass, and its "done when"
   holds. Then mark it `[x]` here with the pull request number, and add what later tasks need
   to section 8.
4. Stop at a gate:
   - **HUMAN** — only the owner can do it: a token, an organization setting, an App
     installation, a passkey, a ruleset. Say exactly what is needed, then continue with tasks
     that do not depend on it.
   - **CONFIRM** — the agent does it after the owner says yes for that task: create a public
     repository, push history, tag a release, deploy the App, merge into a repository with a
     required gate, archive. An earlier yes does not cover a later task.
5. Never delete or move a tag, a release or an image in the old home.
6. A name or a number comes from section 3. A finding that changes this plan is written into
   this plan in the same pull request as the work.

Checks before a pull request is ready:

| Repository | Run |
| --- | --- |
| Engine | `make all` (build, tests, lint, schema, the architecture checks). `GOWORK=off` when the local `go.work` breaks the build. |
| Action | The jobs of its `ci.yaml`, run locally from `tests/`. |
| App | `make all`. The required checks on `main` are: Test, Web app, Store on the Firestore emulator, Lint, Terraform, Deploy scripts, Image builds, Pinned Action commit, Action envelope schema. |

### 4.3 Who runs what

| Epics | Mode | Why |
| --- | --- | --- |
| 0 and 1 | One Claude Code session with the owner near. | Serial, mostly mechanical, a gate every few tasks. The rename touches about 450 files, so no other engine branch may be open while it lands. |
| 2 to 5 | A manager session with workers in agterm, on the model of `~/Workspace/archfit-4-team` (charter, worker protocol, spawn and close scripts). | Four lanes run at once, in three repositories, for days. The manager keeps this file and section 8 current and holds the gates. |
| 3, inside the team | The engine worker may run tasks 3.1 to 3.6 with ralphex from an executable plan in `docs/plans/`. | One repository, tests decide, no gate inside. |
| 6 and 7 | One session with the owner present. | One sitting, a passkey at each step. |
| 8 | Its own plan, from the design of task 2.4. | Not specified here on purpose. |

Lanes for epics 2 to 5, after task 1.8:

| Lane | Tasks | Waits for |
| --- | --- | --- |
| Engine fixes | 3.0, then 3.1 → 3.2 → 3.3; 3.4, 3.5 and 3.6 beside them; 3.7; 3.8 | 1.8 |
| App | 5.1 at once; 5.2 after 3.0; 5.3 and 5.4 after 4.3 | — |
| Front page and design | 2.1 to 2.4 | 1.3; the README record is refreshed after 3.2 |
| Action | 4.1 and the rename part of 4.2 at once; the pin after 3.8 | 3.8 |

Rollback. Until task 5.4 every step adds something and the old home keeps working: a wrong
pull request is reverted, a wrong release is fixed forward with 0.9.1. Task 5.4 is the one
step that can break the onboarded repositories; its rollback is traffic back to the recorded
revision.

## 5. Epics

Order: 0 → 1 → 3 → 4 → 5 → 6 → 7 → 8 → 9. Section 4.3 says what runs side by side.

### Epic 0. Prepare

Done when the old repositories are quiet and a baseline exists.

- [ ] **0.0 The plan travels.** Merge this plan and the surfaces design (today an uncommitted
      file on branch `docs/archfit-4-1-design`) into `main` of the old engine repository, so
      both move with the history in task 1.3. CONFIRM.
- [ ] **0.1 Freeze.** Merge or close the open pull requests and branches in the three old
      repositories (three dependabot pull requests; branches `feat/ratchet-check-inventory`
      and `ci/archfit-4-report`). List the open issues of the two old repositories (the engine
      has #210 drift on main, #205 and #118): each is closed, or written down in section 8 to
      be opened again in the new home. Issues do not move. CONFIRM for each merge.
- [ ] **0.2 Baseline.** For `archfit-4-mechanics`, `archfit`, `ccgram` and `pumba` at recorded
      commits: the policy, the workflow, the gate mode, the ruleset, and the v5.0.0 output of
      `archfit check --format json` and `archfit report`. Stored outside the repositories, in
      `~/Workspace/archfit.reset-evidence/`.
- [x] **0.3 Decisions.** Made on 2026-11-20; see section 2.
- [ ] **0.4 Count the steps today.** One cold run of the current path on a scratch repository:
      install v5.0.0, `init`, the workflow, the App, the first verdict. Record each step, each
      place (terminal, GitHub, App), each wait and each stumble, with the time. This is the
      number epic 8 must beat. The passkey and the App sign-in are HUMAN.

### Epic 1. New home for the engine

Done when `v0.9.0-rc.1` is installable from the new home and gives the baseline verdicts.

- [ ] **1.1 Repositories.** Create `coupling-dev/archfit`, `coupling-dev/archfit-action` and
      `coupling-dev/homebrew-tap`, public and empty. CONFIRM.
- [ ] **1.2 Organization.** Allow public container packages. Create a token that can write
      only to the new tap, and store it as `TAP_GITHUB_TOKEN` on the engine repository. HUMAN.
- [ ] **1.3 History.** Push `main` without tags. Point the local checkout at the new origin,
      keep the old one as a remote that fetches no tags, and delete the 57 local tags. The
      repository's own `archfit.yaml` workflow is off until task 6.2: it pins the old Action,
      which runs engine v4.0.0. Copy the settings: description and topics, branch protection on `main` with the six checks
      that exist (Lint, Test, four builds), dependabot, the docs workflow. Open the issues kept
      in task 0.1. Done when `git ls-remote --tags origin` prints nothing and a pull request
      cannot merge red. CONFIRM.
- [ ] **1.4 Delete.** One pull request: `docs/migration.md`, the Archfit 3 conversion in
      `init`, the `archfit config update` hints, and the old `CHANGELOG.md` moved to
      `docs/archived/`. The design names of section 2 are applied here. Less text to rename in
      the next task.
- [ ] **1.5 Rename.** One mechanical pull request, no behaviour change: the module path in
      `go.mod` and all Go files; the three schema `$id` values; `Makefile`; `release.yaml`;
      `scripts/update_homebrew_formula.rb`; the source label in `Dockerfile`; the constant
      `actionRepo` in `cmd/archfit/change_wiring.go`; the policy version in `archfit.schema.json`, its golden
      contract and `.archfit.yaml`; every install line in the user documents and the shipped
      skill. Done when `make all` passes and no golden file changed except by the new names.
- [ ] **1.6 Guard.** A CI step that fails when a file outside `docs/archived/` names the old
      owner, a `/vN` module path, or a version that is not in the table of section 3. The rule
      for 0.x is written in `docs/development.md`.
- [ ] **1.7 Images.** There are two. The engine image is built by `release.yaml` under the
      name of the repository, so the first tag in the new home publishes
      `ghcr.io/coupling-dev/archfit`. Nothing is copied from the old image; the old digests stay
      where the old pins need them. The App image is built by the App's `deploy.yaml` into the
      Google Cloud project and does not move; it is built again in task 5.4 because the module
      path is compiled into it. The Action has no image.
- [ ] **1.8 Release candidate.** Tag `v0.9.0-rc.1`. Check the binaries, the image and its
      visibility, the tap, and `go list -m -versions` on the new path. Run the candidate on the
      four repositories: every verdict equals the baseline of task 0.2. CONFIRM for the tag.

### Epic 2. README and front page

Done when a visitor understands the problem, the value and the loop without scrolling twice.
Starts after task 1.3.

- [ ] **2.1 Hero.** One animated SVG of the Pumba loop: change, check fails, request, owner
      answers, one policy line, check passes, the next change does not ask. Animation by CSS
      only, colours for light and dark, a static last frame for reduced motion. Test how GitHub
      renders it on a branch before the README depends on it. Fallback: a Mermaid diagram.
- [ ] **2.2 README.** In this order: the problem in one sentence; the value ("ask once, and the
      answer is remembered for the next change"); the hero with links to Pumba #414, #415 and
      #418 under it; one real record of `archfit check`; a start in three commands; what
      Archfit is and is not, beside ArchUnit and import-linter; links. The Docker paragraph and
      the migration links leave the README.
- [ ] **2.3 Other front pages.** The Action README and the App README use the same sentence and
      the same names. The "input contract" paragraph leaves the App README.
- [ ] **2.4 Onboarding design.** One document for the engine and the App, written while
      epics 1 and 3 run and accepted before task 3.8. It uses the count of task 0.4. It names
      the route, the three rungs, each command and page, what `init` and the hooks do by
      default, and what the result and the manifest must already carry in 0.9.0 so that 0.10.0
      breaks nothing. Epic 8 is built from it after the reset. HUMAN to accept.

### Epic 3. Engine fixes for 0.9.0

Done when the engine is tagged `v0.9.0` and every number that differs from the baseline is
explained. Tasks 3.1 to 3.4 are from the 4.1 design; its sections are named.

- [ ] **3.0 Contracts first.** One pull request with no behaviour: the policy key `model` in
      the schema; the result with the two balance inputs and the `surface` field; golden files.
      The App lane starts task 5.2 from this commit. Section 6.6 of the surfaces design lists
      the contracts; the judge parts are left out.
- [ ] **3.1 One strength function** (4.1 design 6.1 and 6.2, tasks 1.1 and 1.2). One function
      returns the strength of a dependency from the declared surface of its target. `check`,
      `can`, the request preview and the report call it. The strength that the report guesses
      from Go types is deleted; type facts stay as evidence. A use past the surface is
      `intrusive` and takes the target's volatility. It reads the policy key `model`.
- [ ] **3.2 A balance that names its inputs** (6.1, task 1.3). `balance: 7/10 (min 7): model,
      architecture-policy is high` in the record, `can`, the request, JSON and SARIF. The
      fields come from task 3.0.
- [ ] **3.3 A legal next step past a surface** (6.9, task 1.8, engine part). A `coupling` error
      for a use past the surface has a `surface:` line. `archfit rules` lists the surface of
      each allowed dependency. `archfit request` accepts one question for such an error:
      should this package be part of the surface? `coupling` stays `deny` for everything else.
- [ ] **3.4 Exceptions with dates** (4.1 design section 2). `init` writes `expires` on every
      exception it creates and ends with the list of them as decisions for the owner. The
      engine's own 30 exceptions get a date or leave.
- [ ] **3.5 Hooks by default.** `init` installs the agent hooks unless a flag says no.
      `doctor` reports when they are missing.
- [ ] **3.6 A first run that cannot end in an error.** `init` writes a policy for the code it
      can read and lists the paths it could not, with one install line. `init` adds
      `.archfit-cache/` to `.gitignore`. `doctor` shows only the tools for the languages found
      and one line "the local engine matches the workflow pin". The text of `report` uses the
      word list of the design. `request` and its guide agree.
- [ ] **3.7 Explain the numbers.** Run the fixed engine on the four repositories and compare
      with the baseline. Every changed balance, status and count has a written reason.
- [ ] **3.8 Release.** A new `CHANGELOG.md` that starts at 0.9.0. Tag `v0.9.0`. Check Go,
      Homebrew, the image and the binaries. Record the two image digests. CONFIRM for the tag.

### Epic 4. Action 0.9.0

Done when the Action is tagged and its commit is recorded. Depends on 3.8.

- [ ] **4.1 History.** Push `main` without tags to the new repository. CONFIRM.
- [ ] **4.2 Pin and rename.** One pull request: the image repository, the engine version and
      the two digests in `scripts/lib.sh`; the vendored result contract; the test fixtures made
      again from the 0.9.0 image; `schema/SCHEMA_SOURCE` when it names the old owner; `action.yml`, the README and `CLAUDE.md`; the guard of task
      1.6.
- [ ] **4.3 Release.** Tag `v0.9.0`. Record the commit. CONFIRM.

### Epic 5. App 0.9.0

Done when the App is deployed with the 0.9.0 manifest. Tasks 5.1 and 5.2 can start after 3.2.

- [ ] **5.1 Rename.** One mechanical pull request: the Go module path; the constant
      `ActionRepository`; the onboarding template; `Setup.svelte`; the README; the Action URL
      in `ci.yaml`; file names that carry a "4"; the guard of task 1.6.
- [ ] **5.2 Contracts.** The result with balance inputs; policy `version: 1` and the key
      `model`, also in the onboarding template and the policy edit path; the balance with its
      inputs on the request and pull-request pages; the answers to a surface request on the request page (declare as public, declare as model,
      allow until a date, reject with `instead`).
- [ ] **5.3 Manifest.** One entry: engine `v0.9.0`, its two digests, the Action commit of 4.3.
      First check whether the App can accept the old and the new result together. If it can,
      keep the v4.0.0 entry until task 6.5. If it cannot, epic 6 is a window with a merge
      freeze.
- [ ] **5.4 Deploy.** Record the running revision for rollback. Deploy. Tag `v0.9.0`. CONFIRM.

### Epic 6. Move the onboarded repositories

One sitting, the owner present for the passkey. Done when all four pass on 0.9.0 and the
manifest lists nothing older. For each repository: one pull request with the new `uses:` line
and `version: 1`; compare `check` and `report` with the baseline; give each exception a date,
turn it into a surface declaration, or fix the code.

- [ ] **6.1 `archfit-4-mechanics`**, the rehearsal. One request approved with the passkey
      proves the whole loop. HUMAN for the passkey.
- [ ] **6.2 `coupling-dev/archfit`.** Install the App on the organization and admit it. Turn
      the workflow on. First approval. Add `archfit/gate` to the required checks. HUMAN.
- [ ] **6.3 `ccgram`.** Advisory gate.
- [ ] **6.4 `pumba`.** The gate is required and the policy is read from the target branch, so
      the migration pull request may not be able to pass its own check. The owner relaxes the
      ruleset for that one merge and restores it. HUMAN.
- [ ] **6.5 Manifest.** Remove every entry older than 0.9.0. Deploy. CONFIRM.

### Epic 7. Close the old home

Done when one number appears everywhere and a cold install works.

- [ ] **7.1 Pointers.** A pointer at the top of both old READMEs. Archive `alexei-led/archfit`
      and `alexei-led/archfit-action`. Remove `archfit.rb` from `alexei-led/homebrew-tap`.
      CONFIRM.
- [ ] **7.2 Outside consumers.** The `tools-archfit` skill of the architect plugin; the chezmoi
      pins; the `archfit-4-team` scripts; local remotes, worktrees and the GitNexus index.
- [ ] **7.3 Cold install.** From a throwaway account and a clean machine or container: install,
      `init`, a pull request, a verdict. Write down every stumble as an issue. HUMAN.

### Epic 8. Onboarding (0.10.0)

The main goal after the reset. Done when the cold run of task 0.4, repeated by a person who
has never used Archfit, reaches a verdict on a pull request in under ten minutes and in at most
four steps: install the CLI, `archfit setup`, install the App, merge the setup pull request.
Three rungs, each with its own reward: try it in the terminal with no account; advisory on one
pull request with no passkey; required when the team asks for it.

- [ ] **8.1 Threat model and approval options** (D6). One page, no code: what an approval must
      prove, against which actor (a coding agent with the developer's GitHub token, a
      developer who is not an owner, a stolen laptop), and what each option proves: the passkey
      in the App; a signed policy commit checked against the owners' public keys; a GitHub
      code-owner review; a GitHub environment approval. The owner decides. HUMAN.

Engine:

- [ ] **8.2 `archfit setup`.** One resumable command: writes the policy and the workflow with
      the right pins, installs the hooks, opens the setup pull request and prints the two App
      links. Each run does the next step and prints where the user is and one `next:` line.
      A person and an agent follow the same path.
- [ ] **8.3 `archfit setup --agent`.** Prints the runbook for a coding agent, matched to the
      installed version. The prompt a user copies is one line.
- [ ] **8.4 `archfit replay --last N`.** The policy run over merged pull requests: how many
      stay quiet, how many would have asked an owner. The reward of rung 1, and the text of the
      setup pull request.
- [ ] **8.5 Open decisions at the end of `init`** (the surfaces design, task 1.5). What the
      policy leaves undecided, in place of reading the whole file. Also in the report. Never
      shown on a pull request.
- [ ] **8.6 Ruleset file.** A ruleset JSON for GitHub's import and the `gh api` line.

App:

- [ ] **8.7 Advisory first** (D2). With `gate: advisory` the App comments and shows a neutral
      check from the first merge of the setup pull request. No sign-in, no passkey.
- [ ] **8.8 Admission by invite.** A partner's account is admitted before the invite is sent,
      so the install never waits for the operator.
- [ ] **8.9 One setup page.** The page shows the state of one repository and one next action.
      It never asks the user to copy a workflow by hand. The last action is "Make it
      required": the approval of task 8.1, the first approval and the ruleset belong there.
- [ ] **8.10 Welcome and digest.** A comment on the setup pull request: how to ask, what
      happens next. After seven days: checked, quiet, asked.

Both:

- [ ] **8.11 One route in the documents.** One page, "Try it on your pull request": the Pumba
      story, one route, two expected results, the fork limit. The engine guide, the Action
      README and the App guide link to it and do not repeat it.
- [ ] **8.12 `archfit why <a> <b>`.** Who approved a dependency, when, and the reason, read
      from the history of the policy file.
- [ ] **8.13 Release 0.10.0** of the engine, the Action and the App, and the pins of the four
      repositories. Repeat the cold run. CONFIRM.

### Epic 9. Guided beta

Six weeks, 3 to 5 teams, led by the owner. The agent prepares and measures.

- [ ] **9.1 First partners.** The owner opens the setup pull request for each, with the replay
      attached. Two weeks advisory. HUMAN.
- [ ] **9.2 Log.** Every real finding is recorded as: code changed, policy changed, noise, or
      `unknown`. Each case where `archfit can` changed a plan before a pull request is recorded.
- [ ] **9.3 Gardener recipe.** A documented prompt and a scheduled workflow on Pumba for a
      month: one debt item for each pull request, a human merges. No product code.
- [ ] **9.4 Trap benchmark.** Ten runs for each cell, a second agent, a second language.
      Published.
- [ ] **9.5 Scorecard and exit.** Time to the first verdict under 10 minutes. Runs without a
      verdict under 2%. 0 to 3 requests for each repository each month. At least one decision
      the tool changed, named by each team. Continue when two teams keep Archfit on and make
      one boundary required by their own choice.

## 6. Not in this plan

Judges and `archfit suggest`; the erosion trend; the impact preview; target presets with a
burndown; stable contracts; data ownership; hidden coupling; copies in other words. Each waits
until a beta partner asks for it. An item no partner asks for in six weeks leaves the list.

## 7. Not checked

| Open point | Checked where |
| --- | --- |
| Does the App read the policy `version`, and can it accept two result shapes for a window? | Task 5.3. |
| Does `release.yaml` update Homebrew on an `-rc` tag? | Task 1.8. |
| Does `coupling-dev` allow public packages by default? | Task 1.2. |
| Does GitHub play the CSS animation of an SVG in this README? | Task 2.1. |
| Can Pumba's migration pull request pass its own required check? | Task 6.4. |
| Where is `pumba-pilot`? It is not in the organization's repository list. | Task 0.2. |
| Does `go install …@latest` on the new path see only the new tags? | Task 1.8. |
| Does `init` fail without ast-grep, as the review says? | Task 3.6, reproduce first. |

## 8. Record

Filled in as tasks finish. A later task reads its inputs here.

| Item | Value | Task |
| --- | --- | --- |
| Baseline commits of the four repositories | | 0.2 |
| Steps and minutes of the cold run today | | 0.4 |
| Issues to open again in the new home | | 0.1 |
| Engine `v0.9.0-rc.1` commit | | 1.8 |
| Engine `v0.9.0` commit | | 3.8 |
| Image digest `linux/amd64` | | 3.8 |
| Image digest `linux/arm64` | | 3.8 |
| Action `v0.9.0` commit | | 4.3 |
| App revision before the deploy (rollback) | | 5.4 |
| App revision of 0.9.0 | | 5.4 |
| Answers to the open points of section 7 | | as found |

