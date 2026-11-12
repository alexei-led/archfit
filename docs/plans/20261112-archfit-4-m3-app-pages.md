# Archfit 4, milestone 3: App pages

## Overview

This is the executable plan for milestone 3 of the
[Archfit 4 design](20261110-archfit-4-architecture-change-control.md). The design wins when the two disagree.
Milestones 1 and 2 have their own plans: [milestone 1](20261110-archfit-4-m1-policy-rules-record.md) and
[milestone 2](20261111-archfit-4-m2-ask-and-answer.md).

Milestone 3 (design section 16) delivers the App pages of design section 12: home, request, policy, pull request,
and the comment, in the words of section 5 and the record of section 8. It removes the old pages. It is done when:

- the golden files of every page and of the comment match (tasks 2 to 7 and 9 to 11);
- no word from the "Not used" column of section 5 appears on any page or in the comment (task 1 adds the test, every
  page task runs it, task 12 proves every route has it).

Milestone 3 is the last milestone before the release. Design section 16 says that 4.0.0 contains milestones 1 to 3
(owner decision), so the release window of the milestone 2 plan (steps R0 to R10, tasks 28 to 32) runs after this
milestone is done (D1).

Each `### Task N` is one pull request, except task 8 (folded into milestone 2 task 19, D8). Tasks 1 to 13 are in archfit-app and target the integration branch `archfit-4`
(milestone 2, D1). Tasks 14 to 16 run after the release window, one per repository. One worker finishes a task, and a
reviewer reads it in one sitting. The team executes this plan, not ralphex. Task numbers are names, not an order: the
dependencies and the wave file (`archfit-4-team/reports/a4-m3-plan-waves.md`) give the order. Every task names the
milestone 2 tasks it needs ("M2: …").

## Context (from discovery)

Read on 2026-11-12 at archfit `03b446f1` and archfit-app `archfit-4` `37a9fd2` (milestone 2 tasks 1 to 4, 12, 13 and
18 merged), and the prototype of design 12.6.

- Web app: Svelte 5, Vite, vitest, Playwright and axe under `web/`. Pages: Admin, DataSharing, Decisions, Graph,
  Landing, Overview, Policy (CodeMirror editor), Pull, Repos, Waiting. Routes in `web/src/lib/route.ts`, the page
  choice in `web/src/App.svelte`, tabs in `web/src/Header.svelte`.
- Server routes in `internal/control/ui/ui.go`: pages `/r/{o}/{n}`, `/graph`, `/pull/{number}`, `/pull/{number}/graph`,
  `/data-sharing`, `/policy`, `/decisions`, `/passkeys`, `/consent`; repository API `overview`, `findings`,
  `decisions`, `graph`, `pulls/{number}`, `pulls/{number}/graph`, five pull-request owner actions, two decision
  actions, `onboarding`, `discover`, `data-sharing` (read and write), `policy` (read, save, check, ready), `assist`,
  and six passkey routes.
- Milestone 2 changes pages as its D12 says: task 24 adds the request page and the passkey page and removes the
  Decisions page, the policy editor with its AI assist, and the owner actions of the Pull page; task 26 removes their
  server code; tasks 15 and 16 give the check and the comment the record. The Overview, Graph, Pull and Data sharing
  pages still read Archfit 3 reports and checks, which milestone 2 stops producing (tasks 15, 25). They stay broken
  until this milestone replaces them. Nothing is released in between (D1).
- Milestone 2 task 23 ships the owner actions outside requests (first approval, workflow change, confirm or revert
  a policy changed outside the flow, the ruleset settings) as server code and an API. No page shows them yet.
- Today the comment and the pull-request page share one builder (`screenOf` in `internal/gate/feedback`, exposed to
  the page as `FirstScreen`), so the two cannot word a result differently. Milestone 3 keeps that property for the
  Archfit 4 comment of task 15.
- Result JSON v1 gives a balance as `value` and `min` only. It has no strength.
- The e2e test is one file, `web/e2e/smoke.spec.ts` (2,813 lines), against `vite preview` with the API mocked in the
  browser. It also takes the user guide's screenshots (`npm run docs:shots`, `docs/images/`). Its test at line 2270
  (a policy save that answers 404) is flaky and leaves with the editor.
- Playwright names snapshots by platform by default (`-darwin`, `-linux`), so a golden made on a laptop would not
  match in CI without a fixed path.
- The App vendors the engine's "Not used" list (`internal/testutil/wording.go`) as
  `internal/wire/request/testdata/wording.go.txt` with a `SCHEMA_SOURCE`, and its matcher is private to the
  `request_test` package.
- The Graph pages need `@xyflow/svelte` and `elkjs` (layout in `web/src/lib/layout.worker.ts`); the policy editor
  needs the `@codemirror/*` packages.
- The Data sharing page records consent for managed discovery and the AI assist. Milestone 2 removes both (Action
  discovery in task 11, the assist in tasks 24 and 26).

## Decisions

The manager decided D1 and D2 and approved D3 to D14 on 2026-11-12.

| Id | Decision | Status |
| --- | --- | --- |
| D1 | One release. 4.0.0 contains milestones 1 to 3 (design 16, owner decision). Tasks 1 to 13 merge into `archfit-4` before release step R0 of the milestone 2 plan; the window R0 to R10 and milestone 2 tasks 28 to 32 run after them, unchanged except that task 28 also reads the pages of this milestone. Milestone 2 tasks 1 to 27 and 33 stay as they are, except task 19, which takes D8. The milestone 2 plan changed with this plan: its release order says the window opens when milestone 3 is done, R0 adds "milestone 3 tasks 1 to 13 merged", R10 deletes `archfit-4` after this release, its Post-Completion line on milestone 3 is gone, its D1 limits `archfit-4` to milestone 3 tasks 1 to 13, its D10 points to D9 here, task 19 takes D8, and task 28 also reads the pages of this milestone. Tasks 14 to 16 run after R10. | Manager, 2026-11-12. |
| D2 | The map keeps `elkjs` (a dependency today) for its layout, in the existing worker. `@xyflow/svelte` leaves with the Graph pages in task 12 when no rewritten page imports it. | Manager, 2026-11-12. |
| D3 | Pages and routes. `/r/{o}/{n}` is Home; it replaces Overview and the Graph page. The request page keeps the route of milestone 2 task 24. `/r/{o}/{n}/policy` is Policy. `/r/{o}/{n}/pull/{number}` is the pull-request page. The tabs are Home and Policy. A pull-request page opens from Home, from a request page, and from GitHub; design 12 has no list of pull requests, so the App gets none. `/` (repositories), `/admin`, the passkey page, the sign-in and waiting pages stay, in the words of section 5. Removed: `/graph`, `/pull/{number}/graph`, `/decisions`, `/data-sharing`, `/consent`. | Approved 2026-11-12, manager. |
| D4 | One model for the comment and the pull-request page (design 12.5). Task 15's comment builder returns a model: the status line, the records, the author line, the owner line. The comment renders it as Markdown, the check summary as the record, and the page API returns it as JSON. All three take their goldens from one fixture set. | Approved 2026-11-12, manager. |
| D5 | Page goldens. Each new page API has Go goldens: store and GitHub fakes in, JSON out. The Playwright tests of a new page mock the API with those JSON goldens, never with hand-written JSON, so the page tests cannot drift from the API. Each page state has a text golden of its visible text under `web/e2e/goldens/`, at a path without the platform. Dates come from a fixed clock on both sides. A golden changes only with its diff read in the pull request. | Approved 2026-11-12, manager. |
| D6 | One "Not used" helper. The vendored engine file and its `SCHEMA_SOURCE` move to a new package `internal/wire/wording` with `NotUsedIn(text) []string`, matching as the engine does. The request, feedback and ui tests call it. A Go test keeps `web/e2e/not-used.json` (the words and the gate-name pattern) equal to the vendored file. The Playwright helper `expectNoNotUsedWord(page)` checks the visible text after every page state, and a vitest test runs the matcher cases the Go test runs. | Approved 2026-11-12, manager. |
| D7 | The map of Home draws the five kinds of design 12.2 from these sources: an allowed dependency from the target policy's `depends_on`; a requested one from the open requests (milestone 2 task 16); one a rule denies (kind `deny`) when a `deny` rule's `from` and `to` match an open request or a stored error key; an exception from the target policy's exceptions with their dates; drift from the drift errors of the latest default-branch result. Task 4 adds the drift errors (rule, from, to) to the `v4/repo` record when milestone 2 task 22 does not store them. Modules sit in the bands of `layers`, the outer layer on top; modules without a layer have their own band. A fold gives the map as a list. The kinds are `allowed`, `requested`, `deny`, `exception` and `drift`; the legend uses the words of design 12.2 ("Map rules"). | Approved 2026-11-12, manager. |
| D8 | Each policy line links to the request that produced it (design 12.4). The answer writer puts the request number in a YAML line comment on the line it adds (`# request #431`), and the policy page reads it. The file on GitHub then carries the link, and it survives a reinstall of the App. A `depends_on` written as a flow list (`[a, b]`) becomes a block list, one entry per line, the first time an answer adds to it, so each entry carries its own comment; comments already there stay. Milestone 2 task 19 writes it (its plan text and its brief); task 9 reads it, and task 8 is not needed. | Approved 2026-11-12, manager. |
| D9 | The YAML edit of design 12.4 is an answer of a new kind, `edit`: the whole new file, bound to the digest it was edited from. The editor starts from the open `archfit/policy` proposal's file when one is open, otherwise from the target policy. The App decodes it strictly (task 3 of milestone 2), always asks for a passkey, and joins it to the `archfit/policy` proposal as the last change. When the proposal's bytes are not the bytes it was edited from, the App refuses it and says to reload. When the target moves before the merge, the App does not replay an edit on the new tip: the edit's approval voids and the owner edits again (an edit has no request to reopen). The App does not compare loosen or tighten (milestone 2 D10 planned a general comparison here); a passkey on every edit is the same protection with less code. The page shows the text diff and says to preview with `archfit check --policy <file>` (design 10.4), because the App has no source to check path globs against. | Approved 2026-11-12, manager. |
| D10 | The Data sharing page, its API, the `discover` endpoint and the `consent` redirect are removed in task 12. After milestone 2 no feature sends data out of a customer's environment. The consent records stay unread in the store and go with the Archfit 3 keys 30 days after the release (milestone 2 D7). | Approved 2026-11-12, manager. |
| D11 | The request page explains the balance with what the App knows: the balance line of the record and the declared volatility of both modules from the target policy. It shows no strength: result JSON v1 has none, and adding it is an engine contract change that this milestone does not need. | Approved 2026-11-12, manager. |
| D12 | Owner actions of milestone 2 task 23 get one page each, built like the request page: the status line, what changed (the policy or the workflow file as a diff), one main action, the passkey. Home lists them under "Needs you". A repository without its first approval shows the set-up steps of design 10.1 on Home instead of "Needs you" and the map. | Approved 2026-11-12, manager. |
| D13 | The e2e tests split by page: `web/e2e/<page>.spec.ts` and shared `web/e2e/fixtures.ts`. A task edits only its page's spec. Two tasks of one wave may edit `web/src/App.svelte`, `web/src/lib/route.ts` or `internal/control/ui/ui.go` only on the lines of their own route, and the second to merge merges `archfit-4` first (as milestone 1 D14). `web/src/Header.svelte` is never shared in a wave. | Approved 2026-11-12, manager. |
| D14 | The prototype is an illustration. Where it shows more than design 12, the design wins: Home has no "Answered without you" section, the request page has no advice until milestone 5, and there is no "Pull requests" tab. | Approved 2026-11-12, manager. |

## Development approach

- One worktree, one branch, one pull request per task. Follow archfit-app `AGENTS.md`, the team's worker protocol,
  and the development approach of the milestone 2 plan (new code beside old code, `module.md` kept current,
  `internal/imports_test.go` respected, `make all`).
- Every pull request of tasks 1 to 13 targets `archfit-4`, never `main` (milestone 2 D1).
- Run impact analysis before a change or a deletion. archfit-app has no GitNexus index: list the callers (`go list`,
  `grep`, `svelte-check`) in the pull-request description. A task marked **impact first** names the symbols.
- A page task carries its API, its page, its spec file, its goldens and its words check together, so a reviewer sees
  one page whole. The old page behind the same route goes in the same pull request; code that other old pages still
  use waits for task 12.
- Keep the axe checks, the phone-width checks and the bundle budget (`web/scripts/size.mjs`) green on every page.
- Each page follows design 12.1: the status in one line first, one main action, the record of section 8 for every
  error and request, a state as a dot and a word with colour for pass, waiting and fail only, detail folded away, and
  only the words of section 5.
- "Merge: manager" marks the required check's output, protection, authentication, storage and removals of routes.
  Other tasks merge by the worker when CI is green and the pull request matches its task.

## Coverage

### Milestone 3 row of section 16

| Outcome or "Done when" | Tasks |
| --- | --- |
| Home | 4, 5, 7 |
| Request | 6 |
| Policy | 9, 10; milestone 2 task 19 (D8) |
| Pull request | 3 |
| The comment | 2 |
| In the words and record of this document | 1, every page task, 11 |
| Old pages removed | 3, 5, 9, 12 |
| Golden files match | 2 to 7, 9 to 11 (D5) |
| No word from the "Not used" column on any page | 1 (the helper), every page task, 12 (every route) |

### Section 12, App pages

| Item | Task |
| --- | --- |
| 12.1 rules for every page | every page task; 11 for the pages that stay |
| 12.2 status line, "Needs you", each row opens its page | 4, 5, 7 |
| 12.2 the map: allowed, requested, one a rule denies, exception, drift; automatic layout | 4, 5 (D2, D7) |
| 12.3 the request page, the passkey prompt on a loosening answer | 6 |
| 12.4 the policy as a list, request links, notes "not checked", exceptions with dates, raw YAML | 9; milestone 2 task 19 (D8) |
| 12.4 an edit of the YAML goes through the same approval | 10 (D9) |
| 12.5 the pull-request page and the comment show the same thing | 2, 3 (D4) |

### Section 13 removals that are pages

| Removed | Replaced by | Removed in |
| --- | --- | --- |
| Pull page with findings, reasons and its graph | Pull-request page (task 3) | 3 (page and its API), 12 (the pull graph) |
| Overview, with onboarding steps, counters and the watch line | Home (task 5) | 5 (page), 12 (API) |
| Graph page and pull graph, `control/ui/graph` | The map on Home (task 5) | 12 |
| Policy editor page | Policy (task 9), YAML edit (task 10) | milestone 2 tasks 24 and 26; 9 (the read API) |
| Decisions page and the JSON request form | Request page (milestone 2 task 24, task 6) | milestone 2 tasks 24 and 26 |
| Data sharing, `discover`, `consent` | Nothing (D10) | 12 |
| Archfit 3 first screen, decision banner, policy card, repair issue in `gate/feedback`, if milestone 2 left them | The task 15 model (task 2), the drift issue (milestone 2 task 22) | 12 |
| Old screenshots in `docs/images/` | New screenshots | 13 |

### Section 15.1 tests

| Area | Task |
| --- | --- |
| The record: golden files for every example of section 8 in the check and the comment | 2 |
| The record: one test fails when any output uses a "Not used" word | 1, 2, every page task, 12 |
| App pages: golden files | 3 to 7, 9 to 11 |

### Section 17 documents

| Repository | Document | Task |
| --- | --- | --- |
| `archfit-app` | `docs/user-guide.md` (the pages, the comment, the YAML edit, screenshots), `docs/architecture.md` (UI routes), `README.md`, `docs/README.md` | 13 |
| `archfit-app` | `internal/control/ui/module.md`, `internal/gate/feedback/module.md`, each touched package's `module.md` | each task |
| `archfit` | The design and the milestone plans move to `docs/archived/plans/`; `docs/plans/current.md`, `AGENTS.md` and `CLAUDE.md` point to the user guides | 14 |
| `archfit-app`, `archfit-action` | `AGENTS.md`, `CLAUDE.md` "Start here" point to the user guides | 15, 16 |

### Milestone 2 tasks this milestone depends on

| Milestone 2 task | Needed by | Why |
| --- | --- | --- |
| 3 Policy schema 3 in the App | 9, 10 | decode and write the policy |
| 15 The check carries the record | 2 | the comment builder |
| 16 Requests as issues | 2, 4 | `waiting`, open requests |
| 17 Balance and preview | 4, 6 | the request data |
| 18 The passkey | 7, 10 | the step-up token |
| 19 Answers: approve and merge | 9, 10 | the writer with request numbers (D8), proposals |
| 20 Recorded approvals | 4, 10 | the state "policy changed outside the flow" |
| 22 Expiry and drift | 2, 4, 11 | expired exceptions, drift, repository state |
| 23 Onboarding and owner actions | 4, 7, 11 | set-up state, owner actions API |
| 24 Request and passkey pages | 1, 6 | the pages and the spec file it changes |
| 25, 26 Removals | 12 | the Archfit 3 ingest and decisions are gone |
| 27 App documents | 13 | the guide this milestone extends |

## Implementation Steps

### Task 1: The "Not used" helper and page tests by page

Repository: archfit-app. Design: 5, 12.1, 15.1. Packages: `internal/wire/wording` (new), `internal/wire/request`
(tests). Files: `web/e2e/` (split), `web/playwright.config.ts`, `internal/imports_test.go` (only when the new package needs
an entry; then not beside milestone 2 task 25, which edits it too). Depends on: none. M2: 24, 26. Merge: worker.

- [ ] move `wording.go.txt` and its `SCHEMA_SOURCE` to `internal/wire/wording` and add `NotUsedIn(text) []string`
      with the engine's matching (whole words in any case, phrases across white space, gate names removed); the
      request tests call it (D6)
- [ ] write `web/e2e/not-used.json` from the vendored file and a Go test that fails when the two differ; put the
      matcher cases of `TestNotUsedMatchesAsTheEngine` in one case file that the Go test and a vitest test both run
- [ ] add the Playwright helpers `expectNoNotUsedWord(page)` and `expectPageText(page, name)`; set
      `snapshotPathTemplate` (or a file helper) so goldens live under `web/e2e/goldens/` without the platform (D5)
- [ ] split `web/e2e/smoke.spec.ts` into one spec per page and `web/e2e/fixtures.ts`, with no change of behaviour;
      delete the flaky policy-save test if milestone 2 left it (its page is gone)
- [ ] run the words helper on the pages this milestone does not rewrite (sign-in, waiting, admin, error page, passkey
      page) and fix the words it finds on them
- [ ] run `make all`

### Task 2: One model for the comment, the check and the pull-request page

Repository: archfit-app. Design: 7.1, 8, 8.1, 8.2, 11.6, 12.5, 15.1. Packages: `internal/gate/feedback` (the task 15
builder), `internal/gate` (new façade file `pullview.go` for the page form: `control` reaches gate data only through
the façade). Depends on: 1. M2: 15, 16, 22. Merge: manager (the required check). Impact first: the task 15 comment and
summary builders.

- [ ] make the task 15 builder return one model per result: the status line, the records, the author line and the
      owner line, with the request each waiting error names (D4); the comment, the check title and summary, and a new
      exported page form all render from it
- [ ] the author and owner lines for every state: `pass`; `fail` with only `fix the code` errors; `fail` with an
      error that asks; `waiting`; `unknown` (policy differs, workflow changed, policy changed outside the flow,
      missing evidence); an expired exception "not from this change"
- [ ] goldens for every example of design 8.1 as a comment, a check title and a check summary, and the three comments
      of 12.5 byte for byte; the page form of each as JSON from the same fixture
- [ ] run the words helper on every golden
- [ ] run `make all`

### Task 3: The pull-request page

Repository: archfit-app. Design: 8.2, 12.1, 12.5. Packages: `internal/control/ui` (`pullapi.go`, `pullpage.go`, routes
in `ui.go`). Files: `web/src/pages/Pull.svelte`, `web/src/components/` (record), `web/e2e/pull.spec.ts`. Depends on:
1, 2. M2: 16. Merge: worker. Impact first: `apiPull`, `pullContext`, `FirstScreen` callers.

- [ ] `GET /api/repos/{o}/{n}/pulls/{number}`: the pull record (`v4/pull`) and the task 2 model (through
      `internal/gate/pullview.go`) of its latest verified result, read back from the run's `archfit-result` artifact with its digest checked (milestone 2 D6),
      the requests it waits on, and the check state; a pull request with no result says so in one line
- [ ] the page: the status line, the records as the CLI prints them, the author line and the owner line, links to
      the requests; nothing else
- [ ] remove the old page's API, the components only it used, and its tests; the pull graph waits for task 12
- [ ] API goldens (D5) for each state of task 2; text goldens and the words check for each; one test that the page
      text equals the comment text of the same fixture, line for line after Markdown marks are reduced
- [ ] run `make all`

### Task 4: The Home API

Repository: archfit-app. Design: 7.1, 12.2, 10.1. Packages: `internal/control/ui` (`home.go`, routes in `ui.go`),
`internal/gate` (new façade file `home.go`), `internal/gate/expiry` or `internal/gate/sweep` (only for the D7 drift
field). Depends on: none. M2: 16, 17, 20, 22, 23. Merge: manager (storage, when it adds the drift field).

- [ ] `GET /api/repos/{o}/{n}/home`: the status line (`clean`, or what is not: "1 request waits", "1 exception
      expires in 9 days", drift, policy changed outside the flow); "Needs you": open requests, exceptions within 7
      days of their date (milestone 2 D19), drift, and the owner actions of task 23, each with its link (D12)
- [ ] the map model of D7: modules with layer and volatility, and each line with its kind (`allowed`, `requested`,
      `deny`, `exception`, `drift`), its request number or date; no positions (the page lays it out)
- [ ] a repository without its first approval answers the set-up state of task 23 instead (D12), with the ruleset
      settings to turn on and what is missing
- [ ] if task 22 stores no drift errors, add rule, from and to of each to the `v4/repo` record (additive) and write
      them on each push result
- [ ] Go goldens with a fake clock: clean; one request; an exception at 9 days; drift; policy changed outside the
      flow; a deny rule touched by a request; not set up
- [ ] run `make all` and the Firestore emulator tests when the record changes

### Task 5: The Home page and the map

Repository: archfit-app. Design: 12.1, 12.2. Files: `web/src/pages/Home.svelte` (new), `web/src/components/map/`
(new), `web/src/lib/layout.worker.ts`, `web/src/App.svelte`, `web/src/Header.svelte`, `web/e2e/home.spec.ts`.
Depends on: 1, 4. M2: none. Merge: worker.

- [ ] the page in the order of 12.2: the status line, "Needs you" (each row opens its page), the map; the set-up
      steps, with the ruleset settings to turn on and what is missing, when the API says the repository is not set up
- [ ] the map: `elkjs` places the modules in the worker (D2), the page draws plain SVG; the five line kinds with
      their labels (`#431`, the date); a legend of the five kinds in the words of "Map rules"; a fold with the map as a list; it fits a phone
      width by scrolling inside its box
- [ ] a map component the request page can reuse for two modules (task 6)
- [ ] `/r/{o}/{n}` shows Home; remove the Overview page; tabs Home and Policy (D3)
- [ ] text goldens and the words check for each API golden of task 4; axe; the layout worker failing says so
- [ ] run `make all`

### Task 6: The request page

Repository: archfit-app. Design: 9.2, 10.4, 12.1, 12.3. Files: the request page of milestone 2 task 24,
`web/e2e/request.spec.ts`; `internal/control/ui/ui.go` (its page route, D13); the request API file of milestone 2 task
17 in `internal/control/ui` and a new façade file
`internal/gate/requestview.go`, only for the fields below that task 17 lacks. Depends on: 1, 5. M2: 17, 24. Merge: worker.

- [ ] the order of 12.3: the status line, the reason, the record with its balance ("as reported" when unverified),
      a small map of the two modules (task 5 component), why the balance (D11), the preview, earlier answers about
      the same modules, then **Approve and merge**, **Approve until a date**, **Reject** with a field for what to do
      instead
- [ ] the passkey prompt appears on an answer that loosens (approve, approve until a date); reject needs none
- [ ] after an answer the page says what happens next; a closed request shows its answer, the owner who gave it,
      the passkey it was made with (design 19), and no buttons
- [ ] register the request page route in `ui.go` when milestone 2 task 24 did not (its files are `web/src/` only), so
      a link from the request issue opens the page
- [ ] the request data gains what the page needs and task 17 lacks: the declared volatility of both modules (D11),
      and the owner and the passkey of an answer
- [ ] API goldens of task 17 for an open, an "as reported", a joined and an answered request; text goldens and the
      words check
- [ ] run `make all`

### Task 7: Owner action pages

Repository: archfit-app. Design: 7.1, 9.2, 10.1, 12.1. Files: `web/src/pages/` (one page per action kind),
`web/src/App.svelte`, `web/src/lib/route.ts`, `internal/control/ui/ui.go` (its page routes, D13),
`web/e2e/owner.spec.ts`. Depends on: 1, 4, 5. M2: 18, 23. Merge:
worker.

- [ ] pages for the actions of task 23 (D12): approve the first policy; approve a workflow change; confirm or revert
      a policy changed outside the flow; each shows the status line, what changed as a diff, one main action and the
      passkey step
- [ ] register each page route in `ui.go`, so a direct load or a link from a GitHub issue opens the page, not Not
      found
- [ ] API goldens of task 23; text goldens and the words check
- [ ] run `make all`

### Task 8: Request numbers in the policy (folded into milestone 2 task 19)

No pull request. D8 is done by milestone 2 task 19, whose writer adds the line comment; task 9 reads it. The number
stays so the other task names do not change.

### Task 9: The policy page

Repository: archfit-app. Design: 6, 12.1, 12.4. Packages: `internal/control/ui` (`policy.go`, routes in `ui.go`).
Files: `web/src/pages/Policy.svelte`, `web/e2e/policy.spec.ts`. Depends on: 1. M2: 3, 19, 26. Merge: worker. Impact
first: `apiPolicy`.

- [ ] `GET /api/repos/{o}/{n}/policy`: the target policy decoded by `policy3`: modules with paths, `public`,
      volatility, layer and allowed dependencies; built-in rules with their modes and `min_balance`; custom rules with
      `why` and `instead`; notes marked "not checked"; exceptions with their dates and days left; each with its
      request number from its line comment (D8); the raw YAML and its digest; when an `archfit/policy` proposal is
      open, also its file and digest, the base of an edit (D9)
- [ ] the page: the readable list first, the raw YAML folded, one main action (Edit, task 10)
- [ ] remove the old policy API handlers that milestone 2 left
- [ ] API goldens of the 6.1 typical policy and of `pumba`'s converted policy; text goldens and the words check
- [ ] run `make all`

### Task 10: The YAML edit

Repository: archfit-app. Design: 9.2, 12.4, 10.4. Packages: `internal/control/answers` (the `edit` kind),
`internal/gate` (new façade file `edit.go`), `internal/control/ui` (the edit API and its route). Files: `web/src/pages/Policy.svelte` (edit view),
`web/e2e/policy.spec.ts`. Depends on: 9. M2: 18, 19, 20. Merge: manager (protection).

- [ ] the answer `edit` of D9: the whole file, the digest it was edited from, a passkey token bound to the new
      digest; decode strictly and list every error by line, worded as the record (`.archfit.yaml:12: policy: …`), before any
      passkey step
- [ ] join the `archfit/policy` proposal as its last change; refuse when the proposal's bytes are not the bytes the
      edit started from, with "the policy changed; reload"; the merge push consumes it (task 20)
- [ ] the edit view: a text area with the YAML, the errors by line, the diff against the current policy, the line
      `preview it: archfit check --policy <file>`, and the passkey step
- [ ] a target that moves before the merge voids the edit's approval; the App does not replay it (D9)
- [ ] tests: an edit approved and merged; an edit after a "merge later" answer starts from the proposal's file and
      succeeds; a decode error; no passkey token; a reused token; an edit from the target's file while a proposal is
      open is refused; a moved target voids the edit
- [ ] run `make all`

### Task 11: The pages that stay, in the words of section 5

Repository: archfit-app. Design: 5, 7.1, 12.1. Packages: `internal/control/ui` (`/api/repos`, `status.go`), `internal/gate`
(new façade file `repostate.go`). Files: `web/src/pages/Repos.svelte`, the passkey page, `Landing.svelte`, `Waiting.svelte`, `Admin.svelte`, `web/src/Header.svelte`, the
footer, `web/e2e/`. Depends on: 1. M2: 22, 23. Merge: worker.

- [ ] repositories: each with its repository state as a dot and a word (`clean`, `drift`, `expired`, policy changed
      outside the flow, not set up) from `v4/repo`; colour only for `expired`, where every pull request
      fails (design 12.1: colour marks pass, waiting and fail); the other states are a dot and a word without colour
- [ ] the sign-in page says what Archfit 4 does in section 5 words
- [ ] API goldens of `/api/repos`; text goldens and the words check on every page this task owns, including the
      passkey page and the error page
- [ ] run `make all`

### Task 12: Remove the old pages and their API

Repository: archfit-app. Design: 11.6, 13. Packages: `internal/control/ui` (`overview.go`, `graphpage.go`,
`pullgraph.go`, `datasharing.go`, `graph/`, Archfit 3 testdata; `reportcache.go`, `hints.go` and `inbox.go` when
nothing else calls them), `internal/gate/feedback` (Archfit 3 rendering), `internal/control/account` (consent, if unused). Files:
`web/src/pages/Graph.svelte`, `DataSharing.svelte`, `web/src/components/` and `web/src/lib/` that only old pages use,
`web/package.json`. Depends on: 3, 5, 6, 7, 9, 11. M2: 25, 26. Merge: manager (routes and a consent surface). Impact
first: `apiStatus`, `apiFindings`, `apiGraph`, `apiPullGraph`, `apiDataSharing`, `apiShareData`, `apiDiscover`,
`FirstScreen`, `screenOf`, `Repairer`.

- [ ] remove the routes and handlers of section "Removed" of D3 and D10, the Graph pages, `control/ui/graph`, the
      components and libraries only old pages used, the Archfit 3 rendering left in `gate/feedback`, and their tests
      and fixtures
- [ ] remove `@xyflow/svelte` when nothing imports it (D2), with its licence entries; keep `elkjs`; check the bundle
      budget
- [ ] a test that lists every page route of `ui.go` and every route kind of `route.ts`, and fails when one has no
      spec that runs the words helper, or when a route kind of `route.ts` has no page route in `ui.go`
- [ ] update `internal/imports_test.go` and the `module.md` files of the removed and changed packages
- [ ] verify that no route serves a removed handler and that `make all` passes

### Task 13: App documents

Repository: archfit-app. Design: 5, 12, 17. Files: `docs/user-guide.md`, `docs/architecture.md`, `README.md`,
`docs/README.md`, `docs/images/`, `web/e2e/` (the screenshot tests). Depends on: 2 to 12. M2: 27. Merge: worker.

- [ ] the user guide: Home, the map and its five line kinds, a request and its answers, the passkey, the policy page
      and the YAML edit, the pull-request page and the comment, in the words of section 5
- [ ] new screenshots by `npm run docs:shots` from the page goldens' fixtures; delete the old images and their tests
- [ ] `docs/architecture.md` "UI routes" and the page part of the 4.0 flows; `README.md` and `docs/README.md` links
- [ ] check that the ruleset advice says 0 required approvals and no bypass actor (milestone 2 task 27)
- [ ] run the App's documentation checks and `make all`

### Task 14: Archive the design and the plans

Repository: archfit. Design: 17. Files: `docs/plans/`, `docs/archived/plans/`, `docs/plans/current.md`, `AGENTS.md`,
`CLAUDE.md`. Depends on: release step R10. Merge: manager.

- [ ] move the design and the plans of milestones 1 to 3 to `docs/archived/plans/` and fix every link to them
- [ ] `docs/plans/current.md`, `AGENTS.md` and `CLAUDE.md` "Start here" point to the user guides
- [ ] run `make docs`

### Task 15: archfit-app "Start here"

Repository: archfit-app, branch `main` after the release. Design: 17. Files: `AGENTS.md`, `CLAUDE.md`. Depends on: 14.
Merge: worker.

- [ ] "Start here" points to `docs/user-guide.md` and `docs/architecture.md`, and to the archived design for history

### Task 16: archfit-action "Start here"

Repository: archfit-action. Design: 17. Files: `CLAUDE.md`. Depends on: 14. Merge: worker.

- [ ] "Start here" points to `README.md`, and to the archived design for history

## Technical Details

### Pages, data and tests

| Page | Route | API | Data (milestone 2 task) | Goldens | Spec | Replaces |
| --- | --- | --- | --- | --- | --- | --- |
| Home | `/r/{o}/{n}` | `GET …/home` (task 4) | target policy (3), requests (16, 17), outside the flow (20), expiry and drift (22), set-up and owner actions (23) | task 4 API, task 5 text | `home.spec.ts` | Overview, Graph |
| Request | task 24's route | task 17's API | requests (16, 17), answers (19), passkey (18) | task 17 API, task 6 text | `request.spec.ts` | Decisions (gone in M2) |
| Owner action | one per kind | task 23's API | onboarding and owner actions (23), passkey (18) | task 23 API, task 7 text | `owner.spec.ts` | Overview onboarding steps |
| Policy | `/r/{o}/{n}/policy` | `GET …/policy` (task 9), edit (task 10) | target policy (3), request numbers and answers (19) | task 9 API, tasks 9 and 10 text | `policy.spec.ts` | policy editor (gone in M2) |
| Pull request | `/r/{o}/{n}/pull/{number}` | `GET …/pulls/{number}` (task 3) | pull record (14, 16), task 2 model | task 2 comment and page JSON, task 3 text | `pull.spec.ts` | Pull, pull graph |
| The comment | GitHub | — | task 15 builder | task 2 | Go tests | Archfit 3 comment |
| Repositories | `/` | `GET /api/repos` | `v4/repo` (22, 23) | task 11 | `repos.spec.ts` | the same page |

### Golden layout

- Go: `internal/gate/feedback/testdata/` for the comment, the check title and summary, and the page JSON of each
  example (task 2); `internal/control/ui/testdata/<page>/` for each page API (tasks 3, 4, 9, 11; tasks 6 and 7 use
  the goldens of milestone 2 tasks 17 and 23, and add them if missing).
- Web: `web/e2e/goldens/<page>-<state>.txt`, the visible text of `main` after the page settles, with a fixed clock.
- An API golden feeds the page test through the mocked route. A changed API golden changes the page's text golden in
  the same pull request, and the reviewer reads both diffs.

### The "Not used" check on pages

- Source: the engine's list, vendored once (`internal/wire/wording`, D6). Matching: as the engine's
  `testutil.NotUsedIn`: whole words in any case, a phrase across white space, the check name `archfit/gate` and the
  policy key `gate` exempt. "record" is not tested (milestone 1 plan).
- Pages: the visible text after every state a spec shows, including errors, empty states and the phone width.
- Comment and check: every golden of task 2.
- Coverage: task 12's test fails when a page route has no spec that runs the helper.

### Map rules (D7)

- Modules come from the target policy, in the bands of `layers` (outer on top), then one band for modules without a
  layer. Each shows its name and its volatility as a word.
- A line has one kind. When two apply to one pair, the order is `drift`, `deny`, `requested`, `exception`, `allowed`.
- A `deny` line appears only when an open request or a stored error touches its pair; otherwise the map would repeat
  every `deny` rule.
- The legend says, in the words of design 12.2: an allowed dependency, a requested one (with its `#431`), one a rule
  denies, an exception (with its date), drift. No label uses a "Not used" word; "denied" is one.
- The legend lists the five kinds. The text "A dependency that is not drawn needs a request." closes the map.

## Post-Completion

_No checkboxes: work outside these repositories or after milestone 3._

- The release window of the milestone 2 plan (R0 to R10, tasks 28 to 32) runs after tasks 1 to 13 (D1); task 28
  also reads every page of this milestone with the production App.
- Release notes: the pages and their routes, the YAML edit with a passkey, the removed pages (Graph, Data sharing).
- Milestone 5 adds advice on the request page and the weekly report.
