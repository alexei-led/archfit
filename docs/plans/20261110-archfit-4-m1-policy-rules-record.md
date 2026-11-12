# Archfit 4, milestone 1: policy, rules, record

## Overview

This is the executable plan for milestone 1 of the
[Archfit 4 design](20261110-archfit-4-architecture-change-control.md). The design wins when the two disagree.

Milestone 1 (design section 16) delivers policy schema 3 with its defaults, the built-in and custom rules, exceptions
with dates, errors counted on base and head, the identical-literal check, the text record, and the commands `check`,
`can`, `rules`, `request`, `init` with conversion, and `doctor --fix`. It is done when:

- the rule tests pass;
- the trap targets of section 15.2 are met locally;
- no baseline code remains;
- `archfit` is converted, and `pumba` is converted and verified locally (the `pumba` pull request merges at release,
  with the Action 4 pin).

Each `### Task N` below is one pull request that one worker can finish and a reviewer can read in one sitting. The
team executes this plan; ralphex does not. Task numbers are names, not an order: the dependencies and the wave file
(`archfit-4-team/reports/a4-m1-plan-waves.md`) give the order.

## Context (from discovery)

Facts from the code at `6c90ae9d` that shape the plan:

- `cmd/archfit` is one Go package with 15 commands (43 source files, 74 test files). Command tasks that touch it cannot
  run in parallel, so command logic goes into application subpackages and the cmd tasks only wire it.
- The Archfit 3 features of design section 13 are entangled across layers. The stored baseline alone reaches
  `cmd/archfit`, `internal/application`, `internal/assessment/status`, `internal/output/sarif`, `internal/model/report`
  and three JSON schemas. Measurement profiles are built on every acquisition run.
- `.archfit.yaml` (schema 2, 18 modules, 41 rules) is read through the version 2 loader by
  `internal/selfmodel_test.go`, `TestCompositionFanoutRatchet`, three self-config tests in
  `internal/config/config_test.go`, `cmd/archfit/agentsmd_test.go`, `cmd/archfit/config_lint_test.go` and
  `cmd/calibrate`. `TestSelfModelCoversEveryGoPackage` needs every new package to sit under a module path glob;
  `TestSelfModelPublicSurfacesAreRealAndOwned` needs every `public` entry to exist; a path glob that matches nothing
  is an error from task 4 on.
- `internal/arch_test.go` checks imports. Most checks match a path prefix, so new subpackages inherit their parent's
  rules; `TestApplicationImportsNoConcreteAdapters` matches exact paths until task 3. Several checks name files and
  packages (`coreRingPkgs`, `cmdDomainAdapterFiles`, `score/gate.go`, `analyze.go`), so a task that deletes one
  updates the test. `TestTransitionalContractSurfaceRatchet` caps the exported names of `internal/policy` (40) and
  `internal/relationship` (55 plus 28 `Seam*`); both are at their caps, and caps may only fall. Assessment consumes
  only the public `internal/relationship` contract. Renderers have an import allowlist.
- `internal/testdata/model_surface.golden` covers `internal/model/...`, so every task that changes a model package
  regenerates it.
- Go has two extraction paths today: the typed loader (`go/packages`) for the check and the standard parser for
  `policy can-import`. The milestone 0 equivalence test found no difference on four repositories.
- Module matching (`policy.ModuleFor`) is already most-specific, not first-match.
- `--base` today runs a second analysis in a temporary `git worktree` and compares finding fingerprints. There is no
  count of errors on the base.
- The balance formula is `bc_score.v7` in `internal/relationship/scoring` (`BookScorer`). Nothing computes the balance
  of a dependency that does not exist yet.
- The milestone 0 prototypes live in `scripts/eval/extractors/` (Go parser, TypeScript and Python resolvers, fixtures)
  and `scripts/eval/copies/` (literal scan, R12). The trap tool is `scripts/eval/traps/`.
- GitNexus: index each worktree with `node .gitnexus/run.cjs analyze --index-only`, and run
  `node .gitnexus/run.cjs clean -f` in it before the worktree is removed.

Milestone 0 results this plan uses (`archfit-4-team/status.md`):

- Go, Python and TypeScript extractors move to parsers. dependency-cruiser leaves only after one more TypeScript
  repository with `paths` aliases passes the equivalence tool.
- `duplication` stays `ask`. A re-typed key of the repository's own model is a copy. Default literal rule R12: length
  at least 12; declared `tests` and `generated` out; single words and struct tags out; held by fewer than 3 modules.
  The documents state the measured precision (about 80% on the dogfood repositories), not 90%.
- `graph` stays `ask` (measured load: 0 to 3 requests a month). `min_balance` is 7.
- `init` writes `public` for each module and declares `tests`.
- Check budget on `pumba`: under 2 s warm, under 5 s on a fresh clone.
- Python without grimp reads namespace packages, so a repository with a namespace directory inside a package gets more
  edges. This goes into the release notes.
- Engine bug: the standard library `log` and a first-party `<root>/log` package share node `package:log`.

## Decisions

The manager approved these on 2026-11-10.

| Id | Decision |
| --- | --- |
| D1 | `check` dispatches on the policy `version` during milestone 1 only: version 2 runs the old path, version 3 the new one. Task 31 deletes the version 2 path. Nothing ships with it. |
| D2 | `report` in milestone 1 is today's analyze pipeline, renamed, reading schema 3 (task 23), with its gates removed by the removal tasks. It has no gate semantics and no verdict: exit 0, or 2 on a tool error. It uses the words of section 5. It never runs on the `check` path and shares no slow extraction with it. Metrics, coverage parsing and `bc_score.v7` stay (sections 13 and 17). |
| D3 | The `pumba` conversion is done and verified locally in milestone 1 and merges at release with the Action 4 pin. `pumba` requires `archfit/gate`, which runs Action v3.1.1, and that version refuses schema 3. |
| D4 | archfit's own `.github/workflows/archfit.yaml` (Action v3.1.1 and the App, not a required check) keeps only `workflow_dispatch` from task 27 on. A comment in the file says so. Milestone 2 restores the triggers with Action 4. |
| D5 | Without the App, `archfit request` prints the request it built, ends with `next: ask an owner`, and exits 2. The App sender plugs in behind the same port in milestone 2. |
| D6 | Simplification for milestone 1: the CLI prints `policy: not protected` when no workflow in `.github/workflows/` uses `alexei-led/archfit-action`. Milestone 2 refines it. |
| D7 | Two tasks of one wave may both change generated files (`internal/testdata/model_surface.golden`, the JSON schemas). The second to merge regenerates after merging main, and its pull-request description shows the regenerated diff. Hand-written shared files stay strict. |
| D8 | The converted self-policy writes `public` as directory globs where a module exposes all its child packages, so later package deletions do not edit `.archfit.yaml`. |
| D9 | A deletion pull request may delete more than 15 files. The size limit counts edited files. |
| D10 | Task 35 deletes `internal/llm`: after `config enrich`, `--ai-summary` and `--ai-classify` go, nothing calls it. `docs/ai.md` moves to `docs/archived/` with `git mv` and `git add -f`. Milestone 4 (`init --from`) restores what it needs from history. |
| D11 | `init` in milestone 1 writes the policy and the five-line instruction block. The workflow file waits for Action 4 (milestone 2). |
| D12 | The user-level Claude Code plugin (design 11.3) is not in milestone 1 but is in 4.0: a release-blocking follow-up for milestone 2 or 3. Fresh clones and worktrees are the gap of design 2.1. |
| D13 | The worker of task 42 picks the extra TypeScript repository: a non-monorepo with the most `tsconfig` `paths` imports. On a tie, `codegraph`. |
| D14 | Two tasks of one wave may both edit `.archfit.yaml` only when each edit deletes lines and names what it deletes in the pull-request description. Both are Merge: manager. The second to merge merges main, re-checks `archfit check --policy .archfit.yaml`, and shows the result. |

## Development approach

- One worktree, one branch, one pull request per task. Follow `AGENTS.md`, `CLAUDE.md` and the team's worker
  protocol.
- Run GitNexus impact analysis before you change or delete a symbol. A task marked **impact first** names the symbols
  whose analysis must be in the pull-request description before any edit.
- New code goes into new packages or new files next to the old pipeline. Do not edit the old pipeline before the task
  that replaces it has merged.
- Every task adds tests for the behaviour it adds or changes, success and failure. A task is done when `make test`,
  `make lint` and the task's focused checks pass. Read a regenerated golden file before you accept it.
- Update the documents a task makes wrong in the same pull request, in the words of design section 5.
- "Merge: manager" marks a contract or protection change. Other tasks merge by the worker when CI is green and the
  pull request matches its task.
- An application subpackage defines the ports it needs; `cmd/archfit` wires the concrete adapters.
- A task that deletes a file, package or directory named in `internal/arch_test.go` or `.archfit.yaml` updates those
  entries in the same pull request, and so does a task that breaks a test file of the root `internal` test package
  (`internal/coverage_names_test.go` names extractors). A pull request that edits `.archfit.yaml` passes CI through
  `archfit check --policy .archfit.yaml` (task 27) and merges by the manager. Two such tasks share a wave only under
  D14.

## Coverage

### Milestone 1 row of section 16

| Outcome or "Done when" | Tasks |
| --- | --- |
| Schema 3 with its defaults | 1, 4, 23 |
| Built-in and custom rules | 4, 5, 12, 13 |
| Exceptions with dates | 1, 4, 14 |
| Errors counted on base and head | 10, 11, 14, 16 |
| The identical-literal check | 9, 13 |
| The text record | 3, 15, 24, 25, 26 |
| `check`, `can`, `rules` | 16, 17, 18, 24 |
| `request` (CLI side) | 19, 25 |
| `init` with conversion | 20, 21, 22, 25 |
| `doctor --fix` and hooks | 26 |
| Rule tests pass | 5, 12, 13, 14, 44 |
| Trap targets met locally | 29, 44 |
| No baseline code remains | 32, 44 |
| `archfit` converted | 27 |
| `pumba` converted and verified locally; merged at release | 28 |

### Section 13 removals

| Removed | Task | Impact first |
| --- | --- | --- |
| Stored baseline, `baseline`, re-anchor | 31 (the command), 32 | yes |
| Measurement profile and comparability | 33 | yes |
| Metric ratchets and the seam check against a baseline | 34 | yes |
| `.archfit-labels.yaml` (and the value draft files) | 35 | yes |
| `.archfit-app.yaml` | 21 (converted), 27 (deleted) | no |
| Waivers | 36 | yes |
| Fifteen commands and three groups | 24, 25, 26 (new), 31 (retired) | yes |
| Six output formats | 15 (JSON, SARIF), 37 (removed) | yes |
| SCIP indexers and reader | 38 | yes |
| `jscpd` | 39 | yes |
| The typed Go loader in the check | 11 (check uses the parser), 40 | yes |
| grimp, Python and `uv` in the check | 11, 41 | yes |
| dependency-cruiser, Node and `tsc` in the check | 11, 42 (after the extra equivalence run) | yes |
| Exit code 2 and "attention needed" on a pass | 24 | no |
| The `"log"` node collision | 6 | yes (`stripModulePath`) |
| Finding origin and `--base` comparison | 33 | yes |
| The version 2 file format and rule types | 43 | yes |

### Section 15.1 tests

| Area | Task |
| --- | --- |
| Strength × volatility matrix, incl. a volatile source into a stable target | 5 |
| Each mode; `graph` with and without an allowed dependency | 12 |
| Added or existing: moved code, renamed files, one more use, missing base evidence | 14 |
| Exceptions: matching, the date, an expired exception fails every change | 14 |
| Copies: seeded identical, rewritten and non-copies; precision and recall | 13 |
| Policy defaults: each row of 6.1; messages for an unknown module, a path that matches nothing, a file in two modules, a missing `why` or `instead` | 1, 4, 16 |
| Policy protection: a changed policy in a code pull request | 16 (the App cases are milestone 2) |
| The record: golden files for every example of section 8; the "Not used" words test | 3, 15, 24, 25, 26 |
| Languages: the language-contract harness for the facts the check uses | 30 |

### Section 17 documents

| Document | Task |
| --- | --- |
| `README.md` | 25 |
| `docs/getting-started.md` | 25 |
| `docs/commands.md` | 24, 25, 26, 31 |
| `docs/configuration.md` | 1 (one sentence), 24, 43 |
| `docs/rules.md` | 24 |
| `docs/results.md` | 24, 37 |
| `docs/agents.md`, the shipped skill `skills/archfit/SKILL.md` | 26 |
| `docs/ci.md` | 24 (the CLI in CI; the Action is milestone 2), 32 |
| `docs/migration.md` | 21 |
| `docs/coupling.md` | 5, 34, 39 |
| `docs/metrics.md` | 34 |
| `docs/evidence.md` | 11, 33 |
| `docs/languages.md` | 38, 39, 40, 41, 42 |
| `docs/architecture.md` | 16, 37 |
| `docs/development.md` | 29, 38, 41 |
| `docs/troubleshooting.md` | 31 |
| `docs/ai.md` | 35 (archived) |
| `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md` | 27; `CONTRIBUTING.md` also 38, 41, 42 |

### Contracts and what milestone 2 consumes

| Contract (11.7) | Task | Milestone 2 uses |
| --- | --- | --- |
| Policy schema 3 | 1 | The App reads the policy on the target branch, checks an owner's YAML edit, and writes answers into it. It uses the digest and the loosen or tighten answer of task 4. |
| The text record | 3 | The App's check summary, annotations and comment show the same records (milestones 2 and 3). |
| Result JSON | 2 | Upload envelope 2 (App-owned) carries the result JSON unchanged with the policy digest it names. |
| Request: one line and an issue | 19 (engine side) | The request line (App-owned) takes `errors[]` (rule, from, to, balance, path, line) from the result, or the `can` answer before a pull request exists, plus the reason and `--expires`. |
| Upload envelope 2 | – | App-owned; milestone 2. It consumes the result JSON of task 2. |

## Implementation Steps

### Task 1: Policy schema 3 contract

Design: 6.1, 6.2 (the `builtin` block), 6.3, 6.4, 11.7, 18. Packages: `internal/config/policyfile` (new),
`internal/configschema`. Files: `archfit.schema.json`. Depends on: none. Merge: manager (contract).

- [ ] add `internal/config/policyfile` with the schema 3 file types of the Technical Details table and a strict
      decoder: unknown keys rejected, `version` must be 3, a version 2 file fails with a message that names `archfit init`
- [ ] decide which Archfit 3 tool and language keys survive: a key stays only when a 4.0 check or report extractor
      reads it; list each survivor and its reader in the pull request
- [ ] generate `archfit.schema.json` from the new types in `internal/configschema` (version 2 generation goes); keep
      the defaults of 6.1 in the field descriptions
- [ ] add golden fixtures under `internal/config/policyfile/testdata/`: the smallest and the typical policy of 6.1,
      and one invalid file per decode error (unknown key, wrong version, bad `expires`, a rule with two forms, an unknown
      built-in mode, `min_balance` outside 1 to 10)
- [ ] write decode tests for every fixture and keep `TestSchemaNoDrift` green on the new types
- [ ] update the sentence in `docs/configuration.md` that names `archfit.schema.json`: it now describes schema 3,
      which the engine reads from task 24 on
- [ ] run `go test ./internal/config/... ./internal/configschema/` and `make test`

### Task 2: Result contract (JSON)

Design: 5 (numbers), 7.1, 7.2, 8, 8.3, 11.7. Packages: `internal/model/result` (new), `internal/reportschema`.
Files: `archfit.result.schema.json` (new), `.archfit.yaml`, `internal/testdata/model_surface.golden` (generated).
Depends on: none. Merge: manager (contract).

- [ ] add `internal/model/result` with the fields of the Technical Details: the status, the errors in record order,
      the counts, the policy (digest, path, protected, differs), base and head commits, the date, the engine version, the
      reasons for `unknown`, and the `can` answer type; it imports no project package
- [ ] generate `archfit.result.schema.json` in `internal/reportschema` with a no-drift test, as `make schema` does
      for the other schemas
- [ ] add golden JSON for every example of design 8.1 under `internal/model/result/testdata/`
- [ ] add `internal/model/result/**` to the paths and `public` of the report-contract module in `.archfit.yaml`
- [ ] write tests: each golden decodes and validates against the schema; `waiting` is accepted but documented as set
      by the App only
- [ ] regenerate `internal/testdata/model_surface.golden` and read the diff
- [ ] run `go test ./internal/model/... ./internal/reportschema/ ./internal/` and `make test`

### Task 3: Text record contract and the "Not used" words test

Design: 5, 8, 8.1, 8.2, 15.1 (the record). Packages: `internal/output/record` (new), `internal/testutil`.
Files: `internal/arch_test.go`, `internal/application/architecture_test.go`. Depends on: 2. Merge: manager
(contract).

- [ ] add `internal/output/record` with the text renderer of a result and of a `can` answer; write the grammar of the
  Technical Details in the package documentation
- [ ] add golden files under `internal/output/record/testdata/` for every example of design 8.1: pass, a `deny`
  rule, `graph` with its balance, a copy with `--expires`, no verdict (the policy differs), an expired exception from
  another change, and `can`
- [ ] add the "Not used" word list of the Technical Details to `internal/testutil` and a test that fails when any
  golden of this package uses one of the words
- [ ] allow renderers to import `internal/model/result` in the renderer allowlist of `internal/arch_test.go`
- [ ] make `TestApplicationImportsNoConcreteAdapters` match subpackages too (today it matches the exact import path,
  so `internal/output/record` or `internal/evidence/acquisition/checkfacts` would pass); if an application file
  imports such a subpackage today, list it in the pull request and ask the manager before changing it
- [ ] write tests for line order, the `--verbose` lines (digest, engine version), dates with days left, and
  `balance: n/10 (min m)`
- [ ] run `go test ./internal/output/record/ ./internal/ ./internal/application/` and `make test`

### Task 4: Policy model: defaults, validation, digest, loosen or tighten

Design: 6.1 to 6.4, 11.2 (policy), 15.1 (policy defaults). Packages: `internal/policy/resolved` (new),
`internal/config/policyfile` (projection). Depends on: 1. Merge: manager (policy semantics the App uses).

- [ ] put the resolved policy model in the new package `internal/policy/resolved`, not in `internal/policy`:
  `TestTransitionalContractSurfaceRatchet` caps the exported names of `internal/policy` at 40, it is at the cap, and
  the cap may only fall
- [ ] the model: modules with the defaults of 6.1 (`public` is the whole module; `volatility` from `subdomain`, else
  `high`), the built-in modes and `min_balance` 7, custom `deny`, `only` and `note` rules, exceptions, `generated` and
  `tests`; the package imports no config package
- [ ] project `policyfile` into the model inside `internal/config/policyfile`
- [ ] add the shipped package sets for `only` (`network`, `process`, `database`) for Go, Python and TypeScript, and
  best effort for Rust and C#
- [ ] add validation with plain messages: an unknown module with a suggestion, a file in two modules, a missing
  `why` or `instead`, a `layer` not in `layers`, an exception for an unknown rule, and a path glob that matches no
  file (a function that takes the tree's file list; task 16 calls it)
- [ ] add the digest (`sha256:` of the file bytes) and the comparison of two policies: loosens, tightens or neither,
  with the list of changes (rules in the Technical Details); `decision.ComparePolicies` in
  `internal/assessment/decision/policy_change.go` compares version 2 policies, is not reused, and goes in task 33
- [ ] write table tests: one per row of the 6.1 defaults table, each validation message, each loosen or tighten case,
  and a stable digest
- [ ] run `go test ./internal/policy/... ./internal/config/... ./internal/` and `make test`

### Task 5: Balance of a dependency that does not exist yet

Design: 4 (`bc_score.v7` verbatim), 6.2 (`coupling`), 11.2 (relationship), appendix A, 15.1 (matrix). Packages:
`internal/relationship/balance` (new). Depends on: none.

- [ ] add the new package `internal/relationship/balance`, which imports `internal/relationship/coupling` and
  `internal/relationship/scoring`; the function cannot live in `internal/relationship` itself, because
  `scoring` imports `coupling` and `coupling` imports `internal/relationship` (a cycle), and the export ratchet of
  `internal/relationship` is at its cap
- [ ] one exported function: the balance of a use of the public part, a use past the public part, or a copy, between
  two modules, from their declared volatilities, with `scoring.BookScorer` and the cross-module distance; no
  volatility cascade; the formula is never copied
- [ ] the volatility that applies is the target's for a use of the public part, and the higher of the two for a use
  past it and for a copy; a use of the public part is `contract` strength, a use past it `functional`, a copy
  `symmetric`
- [ ] assessment may import `balance`: it is not in the package list of
  `TestAssessmentConsumesOnlyThePublicRelationshipContract` or of the `assessment_no_relationship_internals` rule; do
  not add it there, and say so in the package documentation
- [ ] write the matrix test: strengths `contract`, `model`, `functional`, `symmetric`, `intrusive` × volatilities
  `low`, `medium`, `high` × both directions, including a volatile source into a stable target; assert every cell of
  the 6.2 table and appendix A (9 and 1)
- [ ] add the `coupling` rule's numbers to `docs/coupling.md`: declared volatility, no cascade, `public` globs decide
- [ ] run `go test ./internal/relationship/... ./internal/` and `make test`

### Task 6: Go: one parser path for `can` and the check

Design: 11.2 (one extraction path), 11.8 (Go). Packages: `internal/extract/golang` (new files). Depends on: none.
Impact first: `stripModulePath`, `QueryEdge`, `ImportEdgeKind`.

- [ ] port `scripts/eval/extractors/gofacts/goparse.go` into new files: `go/build` file selection and tags, files that
      import `"C"` dropped when CGO is off, members from `go.mod` and `go.work`, `go/parser` imports with line numbers
- [ ] produce file-level import facts (file, line, import path, first-party package or external) for the check, and
      make `QueryEdge` use the same function
- [ ] fix the `log` collision in the parser path: a standard-library import never gets the node of a first-party
      package; change the shared helper only if its impact is low, otherwise leave the typed loader as it is
- [ ] port the prototype tests (rules, build tags, CGO, the root package) and the parser-versus-typed-loader test on
      the fixtures; add a regression fixture with a root package `log` and a standard-library `log` import
- [ ] run `go test ./internal/extract/golang/` and `make test`

### Task 7: TypeScript: ast-grep and a built-in resolver

Design: 11.8 (TypeScript), 15.2 (equivalence). Packages: `internal/extract/tsimports` (new). Depends on: none.

- [ ] port `scripts/eval/extractors/ts_imports.py` and `rules/typescript.yml` (embedded); run `ast-grep` through
      `toolrun` only
- [ ] resolve: relative paths with extension and `index` probing, `.js` to `.ts` and `.ts` to `.d.ts`; `tsconfig`
      `paths`, `baseUrl`, relative and array `extends`, JSONC, `customConditions`, `moduleResolution`; workspace packages
      (npm, yarn, pnpm with `!` exclusions) with `exports` (strings, conditions, `./*`, `null`) and `main`; package
      `imports` (`#x`); triple-slash `reference path`; `?raw`; `node:` builtins
- [ ] report a first-party specifier the resolver cannot place as `unknown` with the file and the specifier; an
      alias that matches no file is `unknown` unless it is a bare `*`; type-only imports count as dependencies
- [ ] port every fixture of `ts_imports_test.py` as Go table tests, plus one test that runs the real `ast-grep`
      (skipped with a message when it is absent)
- [ ] run `go test ./internal/extract/tsimports/ ./internal/` and `make test`

### Task 8: Python: ast-grep and a built-in resolver

Design: 11.8 (Python), 15.2. Packages: `internal/extract/pyimports` (new). Depends on: none.

- [ ] port `scripts/eval/extractors/py_imports.py` and `rules/python.yml` (embedded); run `ast-grep` through
      `toolrun` only
- [ ] resolve packages from the policy's Python setting or by discovery (`src` layout first), namespace packages
      (PEP 420), `from m import n` as `m.n` when that is a module, relative imports, and drop imports under
      `if TYPE_CHECKING:`
- [ ] report a first-party import the resolver cannot place as `unknown`
- [ ] port every fixture of `py_imports_test.py` as Go table tests, plus one real `ast-grep` test
- [ ] run `go test ./internal/extract/pyimports/ ./internal/` and `make test`

### Task 9: Literals on added lines and the literal index

Design: 6.2 (`duplication`), 7.3, 11.8 (the built-in literal scan). Packages: `internal/extract/literals` (new).
Depends on: none.

- [ ] read string literals with line numbers: Go with `go/scanner` or `go/ast` (import paths and struct tags out);
      TypeScript, JavaScript, Python, Rust and C# with embedded `ast-grep` string rules through `toolrun`
- [ ] give each literal its normalised value, length and shape (single word `^[\w.:@-]+$`, struct tag) and keep only
      literals of length 12 or more in the index of a tree (value to files)
- [ ] keep tables of values and multi-line literals out of milestone 1, and say so in the package documentation
- [ ] write tests per language: struct tags and import paths are not literals; text in comments and docstrings is
      never read; shapes are right; the index of a fixture tree is stable
- [ ] run `go test ./internal/extract/literals/` and `make test`

### Task 10: Check facts model; base tree and added lines from git; cache by tree

Design: 4 (no stale state), 7.2 (added means more), 11.2 (base and head in one run, cached by tree). Packages:
`internal/model/evidence` (new types), `internal/history/git`, `internal/factcache`. Files:
`internal/testdata/model_surface.golden` (generated). Depends on: none.

- [ ] add neutral check-fact types to `internal/model/evidence`: file imports with line and target, literal
      occurrences, added lines per file, `unknown` items (file, specifier, reason)
- [ ] read the merge-base tree from git objects into a temporary snapshot without touching the working tree, or reuse
      `worktree.go` if it is simpler; a shallow clone or a missing merge base returns an explicit error
- [ ] compute added lines of the working tree against the merge base: modified, renamed (`-M`) and untracked files
- [ ] key cached facts by the git tree id for the base and by the content hash for the working tree; the head's
      tree id is `git.WorkingTreeID` (#152), the one "this tree" id
- [ ] write tests on temporary repositories: added lines for modified, renamed, untracked and deleted files; the
      snapshot equals `git archive`; cache hit and miss; a shallow clone returns the error
- [ ] regenerate `model_surface.golden` and read the diff; run `go test ./internal/history/... ./internal/factcache/
./internal/model/... ./internal/` and `make test`

### Task 11: Check facts acquisition

Design: 7.2, 11.2 (evidence, the optional-analyzer rule), 11.8. Packages: `internal/evidence/acquisition/checkfacts`
(new), `internal/extract/registry`. Depends on: 6, 7, 8, 9, 10.

- [ ] add `checkfacts`: extract imports and literals for the head (working tree) and the base (snapshot of task 10),
      cached by tree; Go, TypeScript and Python through the parser extractors; Rust and .NET through today's extractors
- [ ] register the parser extractors in `internal/extract/registry` for the check; never call dependency-cruiser,
      grimp, the typed Go loader, SCIP or `jscpd` on this path
- [ ] turn an unresolved first-party import, a missing base, or a missing needed tool (`ast-grep` for TypeScript or
      Python) into `unknown` items with the fix to name in `next`
- [ ] write tests: the facts of a fixture repository for each language; the same facts with every optional analyzer
      off; a missing `ast-grep` gives `unknown`; base facts come from the cache on a second run
- [ ] measure the extraction on `pumba` (warm and fresh clone) and put the numbers in the pull request; the whole
      check must stay under 2 s warm and 5 s on a fresh clone
- [ ] update `docs/evidence.md`: what the check reads, base and head in one run, when evidence is `unknown`
- [ ] run `go test ./internal/evidence/... ./internal/extract/registry/ ./internal/` and `make test`

### Task 12: Rules on one tree

Design: 6.2, 6.3, 7.3 (the noise limit belongs to task 13), 8.1, 15.1 (rules). Packages: `internal/assessment/change`
(new). Depends on: 4, 5, 10.

- [ ] evaluate one tree's facts against the policy model and return errors with their key (rule, from module, to
      module, symbol) and sites (file and line), as the Technical Details define
- [ ] built-in rules: `graph` (with the balance), `coupling` (a use inside or past the target's `public` globs,
      balance from task 5, error below `min_balance`), `cycle` (between modules), `layers` (inner first), `unowned`
      (production code in no module; declared `tests` and `generated` out)
- [ ] custom rules: `deny` by module or path with `why` and `instead`; `only` with a library or a package set; `note`
      gives no error and is listed as "not checked"
- [ ] modes: `deny` gives `next: fix the code`, `ask` gives `next: archfit request -m "<why you need it>"`, `off`
      skips; the `what`, `why` and `next` texts follow design 8.1 exactly
- [ ] write the rule tests of 15.1: each mode of each rule, `graph` with and without an allowed dependency,
      `coupling` at each volatility for a use inside and past `public`, a cycle, a layer inversion, unowned code with and
      without declared tests, `deny` by module and by path, `only` with a library and with a set, a `note`
- [ ] run `go test ./internal/assessment/... ./internal/` and `make test`

### Task 13: The `duplication` rule

Design: 6.2 (`duplication` judges every copy), 6.4, 7.2, 7.3, 15.1 (copies), status.md (R12). Packages:
`internal/assessment/copies` (new). Depends on: 4, 10.

- [ ] evaluate one tree, as task 12 does: an error for each literal that production code of two or more modules
  holds, after R12 (length at least 12; declared `tests` and `generated` out; single words and struct tags out; held
  by fewer than 3 modules)
- [ ] key the error by (`duplication`, the sorted module pair, the literal digest); the sites are the occurrences;
  then task 14's counting makes a copy added by the change a new key, a moved literal an unchanged key, and an old
  copy under an expired exception an error of every change, as 8.1 shows
- [ ] a `duplication` key counts as added only when one of its head sites is on an added line: R12 counts holders per
  tree, so deleting the third holder of a literal creates a new key without adding a copy, and that change passes
- [ ] for the record: `from` is the module whose site is on an added line (otherwise the first in order); `what`
  names the first site in the other module (`this literal already lives in pkg/util/util.go:15`); `next` names that
  module and `archfit request -m "<why a copy is needed>" --expires <date>`
- [ ] write tests: the seeded `pumba` copy (regular expression and error text) is found and is added on head; a
  rewritten copy, a moved literal, a literal in tests or generated code, a short literal, a single word, a struct
  tag and a literal held by three modules are not; an existing copy is not added; deleting the third holder of a
  literal adds nothing; report precision and recall over the seeded set in the test output
- [ ] run `go test ./internal/assessment/copies/ ./internal/` and `make test`

### Task 14: Counting on base and head; exceptions with dates; the four statuses

Design: 6.4, 7.1, 7.2, 15.1 (added or existing, exceptions). Packages: `internal/assessment/count` (new:
`copies` imports `change`, so counting in `change` would be a cycle). Depends on: 12, 13.

- [ ] compare base and head errors per key (tasks 12 and 13): a new key or a higher count is added by the change;
  an `unowned` key is added only when its file has an added line, so a pure rename passes (#160); report the head
  sites that are not on the base, and prefer sites on added lines
- [ ] apply exceptions: an exception covers the errors of its rule between its `from` and `to` (for `duplication`,
  the unordered pair, because its key is unordered; for a custom `only` rule, its `from` alone (#160)); on or before
  its
  `expires` date it holds; after that date, while the code still breaks the rule on the head, it is an error of every
  change, marked "not from this change"
- [ ] set the status in this order (#160): `unknown` with only its reasons when base evidence is missing; else `fail`
  for an added error or an expired exception, and the unknown items are still listed; else `unknown` when an
  `unknown` item touches a changed file, or when an `unknown` item exists on the head that the base does not have
  (an unchanged file whose import no longer resolves, for example after a change to `tsconfig` `paths`); else
  `pass`; the engine never sets `waiting`
- [ ] write the tests of 15.1: moved code, a renamed file, one more use of a broken dependency, missing base evidence,
  a new `unknown` item in an unchanged file, an exception that matches and one that does not, the date boundary, an
  expired exception seen by an unrelated change (including an old copy), existing drift that the change does not
  touch
- [ ] run `go test ./internal/assessment/... ./internal/` and `make test`

### Task 15: JSON and SARIF from the result

Design: 8.3, 11.2 (output). Packages: `internal/output/record`. Depends on: 2, 3.

- [ ] render the JSON of `internal/model/result`; it validates against `archfit.result.schema.json`
- [ ] render SARIF 2.1.0 from the same result: one result per error, `ruleId` is the rule id, the location is the
      file and line, the message is `what`, then `why`, `instead` and `next`
- [ ] add JSON and SARIF golden files for the examples of design 8.1
- [ ] write tests: schema validation of every JSON golden; SARIF has no result on `pass`; field order is stable
- [ ] run `go test ./internal/output/record/` and `make test`

### Task 16: The check workflow

Design: 7.1, 7.2, 9.1, 9.4, 11.1 (`check`), 14, 15.1 (policy protection). Packages:
`internal/application/changecheck` (new). Depends on: 2, 11, 14. Merge: manager (policy protection).

- [ ] define the ports: the local policy, the policy on the target branch (from git objects), a candidate policy
  (`--policy`), check facts for head and base, the clock, and the store of the last result
- [ ] run the flow: the target branch (`--base REF`, else `origin/HEAD`; neither gives `unknown` with
  `next: pass --base <target branch>`) and the merge base; when HEAD is the target branch, the base is its first
  parent; when the local policy
  differs from the target's, stop with `unknown` and the record that restores the file (8.1); with `--policy FILE`,
  judge with that file and skip the comparison
- [ ] when the target has no policy, or a policy that is not schema 3 (the conversion pull request of design 14),
  judge with the local policy and print one line that says so
- [ ] validate the policy against the tree (the task 4 checks, including a path that matches nothing); an invalid
  policy gives `unknown` with the message
- [ ] assess head and base (tasks 12 to 14), project into `internal/model/result` in one projector file (date,
  digest, engine version, `policy: not protected` per D6), and save the result for `request` in
  `.archfit-cache/last.json` as `{"tree", "output"}`: the `git.WorkingTreeID` and the `--format json` document; a
  `--policy` preview saves nothing (#152)
- [ ] write tests: a code change that also edits the policy is `unknown`; `--policy` previews; no target policy; a
  version 2 target policy; an invalid policy; a shallow clone without a merge base is `unknown` and names the fetch;
  the date as an input gives the same output twice
- [ ] update `docs/architecture.md`: the run flow of `check` and the new packages
- [ ] run `go test ./internal/application/... ./internal/` and `make test`

### Task 17: The `can` query

Design: 6.2 (`coupling`), 8.1 (`can`), 10.2, 11.1 (`can`). Packages: `internal/application/canquery` (new).
Depends on: 4, 5.

- [ ] answer `can <from> <to>` where each side is a file, a directory, a module name, a Go import path (mapped
  through `go.mod` by a port), or, for `to`, a library
- [ ] apply the same rules `check` applies to one new dependency: `graph`, `coupling`, `layers`, custom `deny` and
  `only`; for `coupling`, a target inside the target module's `public` globs is a use of the public part, a target past
  them is a use past it, and a module-name target means its public part; a balance below `min_balance` is a no
- [ ] answer yes (exit 0 in task 24), no with the rule, `why`, `instead`, the balance and `next` (exit 1), or
  `unknown` for a path in no module (exit 2)
- [ ] write tests: the 8.1 example; an allowed dependency with its balance; an allowed dependency past the `public`
  globs of a `high` module (no, `coupling`, 2/10); a `deny` rule with `instead`; a library outside its `only`
  modules; a layer inversion; a path in no module; for each case, the `can` answer agrees with task 12 on the same
  import
- [ ] run `go test ./internal/application/canquery/ ./internal/` and `make test`

### Task 18: The `rules` query

Design: 10.2, 11.1 (`rules`), 6.3 (notes shown as "not checked"). Packages: `internal/application/rulesquery` (new).
Depends on: 4.

- [ ] for paths: each path's module, its allowed dependencies, `public`, volatility and layer, the rules and
      exceptions that apply (with dates), and the notes marked "not checked"
- [ ] for a rule id: the rule in full; with no argument: the rules for the changed files (from a port)
- [ ] `--graph`: the modules by layer and their allowed dependencies, as text
- [ ] write golden tests for each form, checked by the "Not used" words test
- [ ] run `go test ./internal/application/rulesquery/ ./internal/` and `make test`

### Task 19: The `request` builder (CLI side)

Design: 10.3, 11.1 (`request`), 11.7 (the request is App-owned). Packages: `internal/application/request` (new).
Depends on: 2.

- [ ] build a request from the last `check` result (or a `can` answer): the errors (rule, from, to, balance, file and
      line), the reason, `--expires`, the head and base commits, and the policy digest; only errors whose
      `next.kind` is `command` join (#152); "last" means the same HEAD and the same `git.WorkingTreeID` in
      `.archfit-cache/last.json` (#152)
- [ ] validate: a reason is required (the CLI asks for it on a terminal; elsewhere a missing `-m` is a usage error);
      `--expires` is a future ISO date; no last result means "run archfit check first"
- [ ] define the sender port; milestone 1 ships no App sender (D5)
- [ ] write tests for each validation and for the built request of the 8.1 `graph` and copy examples
- [ ] run `go test ./internal/application/request/` and `make test`

### Task 20: The `init` writer

Design: 6.1, 10.1, 11.3 (the five lines), status.md (`public` and `tests`). Packages: `internal/initcfg/policy3`
(new). Depends on: 1, 4, 11.

- [ ] write schema 3 from discovery and the head facts: modules with `paths`, `public` for every module (the packages
      other modules use today), `depends_on` as the production dependencies that exist, `tests` for the detected test
      globs, `generated` for listed generated paths, `owners` from the `origin` remote owner
- [ ] write containment as it is today: an `only` rule for each package set that only some modules use
- [ ] write the five-line instruction block of 11.3 between markers in `AGENTS.md`; create the file when it is
      missing and never change bytes outside the markers
- [ ] never overwrite an existing file; an Archfit 3 policy is task 21's case
- [ ] write golden tests on Go, TypeScript and Python fixture repositories, the no-overwrite rule, and the block splice
- [ ] run `go test ./internal/initcfg/... ./internal/` and `make test`

### Task 21: `init` conversion of an Archfit 3 repository

Design: 14, 6.3 (rejected answers become `deny`), 13. Packages: `internal/initcfg/policy3`. Files:
`docs/migration.md`. Depends on: 20.

- [ ] convert `.archfit.yaml` version 2, `.archfit-labels.yaml`, `.archfit-baseline.json`, waivers and
      `.archfit-app.yaml` into one schema 3 policy, using the mapping table of the Technical Details
- [ ] read the Archfit 3 files with decoders inside `policy3`, not through `internal/baseline` or `internal/labels`,
      so tasks 32 and 35 can delete those packages; the version 2 policy may use `config.Load` until task 43
- [ ] replace the Archfit 3 policy in place, remove the other Archfit 3 files from the working tree, and print what
      could not be converted and why
- [ ] write tests: the real `archfit` and `pumba` policies (copies under `testdata`) give reviewed golden output and
      a reviewed "not converted" list; one synthetic fixture per mapping row
- [ ] rewrite `docs/migration.md` as "Migrate from Archfit 3 to 4"
- [ ] run `go test ./internal/initcfg/... ./internal/` and `make test`

### Task 22: Repository state for owners

Design: 7.1 (the repository table), 10.6. Packages: `internal/application/repostate` (new). Depends on: 11, 14.

- [ ] evaluate the whole head tree with the rules of tasks 12 and 13 and the exceptions of task 14
- [ ] return the state: `clean`, `drift` (an error no exception covers), or `expired` (an exception past its date
      while the code still breaks the rule); the notes ("not checked"); exceptions within 14 days of their date; the
      balance of each allowed dependency
- [ ] give `init` the list of errors that exist, so it can write them as exceptions (task 25)
- [ ] write tests for each state, the expiry window and the balance list
- [ ] run `go test ./internal/application/repostate/` and `make test`

### Task 23: The report pipeline reads schema 3

Design: 6.1, D2. Packages: `internal/config` (new files). Depends on: 4.

- [ ] project a schema 3 policy into the views the analyze pipeline uses (`ModuleMapView`, `ForClassify`, `ForScope`,
      `ForExtract` and the others): modules, `public`, volatility, layers, exclusions, language settings
- [ ] give the Archfit 3 knobs that have no schema 3 key the values that remove gating: no rules, no waivers, no
      coupling gate, default metrics
- [ ] write tests: a projection golden for the typical policy of 6.1; the analyze pipeline runs on a schema 3 fixture
      and produces measurements and no gate result
- [ ] run `go test ./internal/config/... ./internal/` and `make test`

### Task 24: Commands `check`, `can`, `rules`

Design: 7.1, 8, 11.1, D1, D6, 15.1 (the record). Packages: `cmd/archfit`. Files: `docs/commands.md`,
`docs/configuration.md`, `docs/rules.md`, `docs/results.md`, `docs/ci.md`. Depends on: 3, 15, 16, 17, 18. Impact
first: `CheckCmd`, `outcomeExitCode`.

- [ ] `check` dispatches on the policy `version` (D1): version 3 runs `changecheck`, version 2 the old path with its
  exit codes unchanged (0, 1, 2, 3), so `make arch-lint`, the CI image step and `scripts/eval/corpus_sweep.py` keep
  their meaning until tasks 27, 30 and 31
- [ ] wire `check`, `can` and `rules` with the flags of 11.1 (`--base`, `--policy`, `--format json` or `sarif`,
  `--verbose`, `-C DIR`); the record goes to stdout, progress to stderr
- [ ] exit codes of the version 3 `check`, `can` and `rules`: 0 pass or yes; 1 fail or no; 2 `unknown`, wrong usage
  or a tool error; task 31 applies them to the whole binary
- [ ] wire the adapters: check facts, the git policy source, the last-result store under `.archfit-cache/`, the
  clock, and the D6 detection
- [ ] write golden CLI tests for every example of design 8.1 and the `rules` forms, and extend the "Not used" words
  test to these goldens and to the `--help` text of the three commands
- [ ] rewrite the parts of `docs/commands.md` on `check`, `can` and `rules`; rewrite `docs/configuration.md` for
  schema 3; rewrite `docs/rules.md` (built-in and custom rules, exceptions) and `docs/results.md` (the record, the
  statuses, JSON, SARIF, exit codes); update `docs/ci.md` for running `archfit check` in CI without the Action
  (`fetch-depth: 0` and `--base origin/<target branch>`: `actions/checkout` creates no `origin/HEAD`)
- [ ] run `go test ./cmd/archfit/ ./internal/`, `make test`, `make docs`

### Task 25: Commands `request`, `init`, `report`

Design: 10.1, 10.3, 10.6, 11.1, D2, D5, D11. Packages: `cmd/archfit`, `internal/output/reporttext` (new). Files:
`docs/commands.md`, `docs/getting-started.md`, `README.md`. Depends on: 19, 20, 21, 22, 23, 24.

- [ ] `request [-m WHY] [--expires DATE]`: build from the last result; without an App print the request, end with
      `next: ask an owner`, exit 2 (D5)
- [ ] `init`: a fresh repository (task 20) or an Archfit 3 repository (task 21); then list the errors that exist
      (task 22) as exceptions with a reason; never overwrite another file; no workflow file (D11)
- [ ] `report`: the analyze pipeline (schema 2 or 3 through task 23) and the repository state (task 22), as text in
      section 5 words (`internal/output/reporttext`) and as JSON; no verdict; exit 0, or 2 on a tool error (D2)
- [ ] write golden CLI tests for the three commands and extend the "Not used" words test to them
- [ ] update `docs/commands.md` (`request`, `init`, `report`), rewrite `docs/getting-started.md` (init, rules, can,
      check, request) and the quick start of `README.md` (seven commands)
- [ ] run `go test ./cmd/archfit/ ./internal/...`, `make test`, `make docs`

### Task 26: `doctor --fix` and the hook plumbing

Design: 10.1, 10.2, 11.3, 11.8 (`doctor` groups), 8.2 (hooks). Packages: `cmd/archfit`, `internal/extract/registry`.
Files: `docs/agents.md`, `docs/commands.md`, `skills/archfit/SKILL.md`. Depends on: 25. Impact first: `HookCmd`,
`HookInstallCmd`, `DoctorCmd`.

- [ ] move the tool lists into `internal/extract/registry` as two groups per language: needed for the check (`git`;
  `ast-grep` for TypeScript, JavaScript and Python; today's tools for Rust and C#) and optional for the report
- [ ] `doctor` checks and shows the two groups; `doctor --fix` installs the hooks of the detected harness (Claude
  Code, Codex, git) and warns when `.gitignore` ignores the harness settings directory
- [ ] rewrite the hidden hook entry points: after an edit, `can` on the imports it added, with the record as
  feedback; at stop, `check`, blocking once per identical error set; a git pre-commit hook runs `check` on the
  staged tree; a hook that fails for its own reasons reports and does not block
- [ ] write tests: tool groups per language, harness detection, the ignored-directory warning, each hook event with a
  pass, a fail and a hook error, and the "Not used" words test on hook feedback
- [ ] rewrite `docs/agents.md` and `skills/archfit/SKILL.md` (the five lines, the hooks, `doctor --fix`); update the
  `doctor` part of `docs/commands.md`
- [ ] run `go test ./cmd/archfit/ ./internal/extract/registry/`, `make test`, `make docs`

### Task 27: Convert `archfit`

Design: 14, 9.2, 9.4, D4, D8. Files: `.archfit.yaml`, `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `Makefile`,
`.github/workflows/ci.yaml`, `.github/workflows/archfit.yaml`, `.github/CODEOWNERS`, `internal/selfmodel_test.go`,
`internal/selfmodel_allowlist_test.go`, `internal/arch_test.go`, `internal/config/config_test.go`,
`cmd/archfit/agentsmd_test.go`, `cmd/archfit/config_lint_test.go`. Deletes: `.archfit-baseline.json`,
`.archfit-labels.yaml`, `.archfit-app.yaml`. Depends on: 24, 25, 26. Merge: manager (the protected policy and CI).

- [ ] run `archfit init` on the repository and review the converted policy; write `public` (D8) and, where a module
  owns all of a parent directory, `paths` as directory globs, so later package deletions do not leave dead globs;
  keep the exceptions for errors that exist, with reasons
- [ ] move every test that reads the self-policy to schema 3: the self-model tests, `TestCompositionFanoutRatchet`,
  and `TestLoad_SelfConfig`, `TestSelfConfig_CapabilityModuleMap` and `TestSelfConfig_RoleLayerConformance` in
  `internal/config/config_test.go`; delete the self-policy cases of `cmd/archfit/agentsmd_test.go` and
  `cmd/archfit/config_lint_test.go` (their commands go in task 31); `make calibrate` stops working until task 31
  deletes it
- [ ] `Makefile`: `arch-lint` and `archfit` accept exit 0 only; `archfit-report` runs `report`
- [ ] the CI image step: checkout with `fetch-depth: 0`; pass `--base origin/${{ github.base_ref }}` on a pull
  request and `--base ${{ github.event.before }}` on a push (`actions/checkout` creates no `origin/HEAD`); when the
  pull request changes `.archfit.yaml`, add `--policy .archfit.yaml` (the owner approves the file through
  `CODEOWNERS`, the branch rule of design 9.4); accept exit 0 only; drop the comparability comment and the
  `.repository.attention` query
- [ ] `.github/workflows/archfit.yaml` keeps only `workflow_dispatch`, with a comment that milestone 2 restores the
  triggers with Action 4 (D4); `CODEOWNERS` loses the deleted files and keeps `.archfit.yaml`
- [ ] replace the archfit block of `AGENTS.md` with the five lines; update the repository rules in `CLAUDE.md` and
  the architecture-check lines of `CONTRIBUTING.md` (`archfit can`, `archfit check`, exit 0)
- [ ] verify: `make arch-lint`, `make test`, `make docs`; this pull request passes the CI step through
  `--policy`; `archfit check` passes on main and fails on a seeded forbidden import and on a seeded copy

### Task 28: Convert `pumba` (pull request in `pumba`)

Design: 14, D3. Repository: `pumba`, default branch `master`. Depends on: 25. Merge: manager, at release.

- [ ] in a `pumba` worktree, run `archfit init` from the milestone 1 build and review the converted policy; remove
      `.archfit-baseline.json` and `.archfit-app.yaml`; replace the `AGENTS.md` block with the five lines
- [ ] update `.github/workflows/build.yaml` and `.github/scripts/archfit-check.bash`, which read the baseline; leave
      the Action pin for the release, when Action 4 exists
- [ ] verify locally with the milestone 1 build: `archfit check` passes on `master`; the trap import fails with
      `graph` and its balance; the trap copy fails with `duplication`; record the warm and fresh-clone check times
- [ ] open the pull request as a draft that says it merges at release with the Action 4 pin

### Task 29: The trap benchmark for 4.0

Design: 15.2. Packages: `scripts/eval/traps`. Files: `docs/development.md`. Depends on: 26.

- [ ] arms for 4.0: `prose` unchanged; `block` with the five lines instead of the Archfit 3 block; `block-hooks` with
      `archfit doctor --fix`; `tool` and `tool-hooks` without the prose graph
- [ ] give the trap clone a schema 3 policy on its default branch at setup (`archfit init` conversion of the pinned
      commit, or the policy of task 28 as a fixture)
- [ ] judge the change by the exit code and the text record of `archfit check`; count `archfit request` calls as
      requests filed, beside the text heuristic; an unnoticed copy is an identical rule literal with a `pass`
- [ ] write unit tests on recorded fixtures for the new arms and classification; no paid run in this task
- [ ] update the trap benchmark part of `docs/development.md`
- [ ] run `uv run --with jsonschema==4.26.0 python -m unittest discover -s scripts/eval -p '*_test.py'`

### Task 30: The language-contract harness for the check facts

Design: 15.1 (languages), 11.8. Files: `scripts/eval/language_contracts.py`, `scripts/eval/language_contracts_test.py`.
Depends on: 24.

- [ ] give the fixtures schema 3 policies; each fixture's forbidden dependency must give its error in the record and
      exit 1, and a clean fixture must pass
- [ ] Go, TypeScript and Python run with `git` and `ast-grep` only; drop dependency-cruiser, grimp and the typed Go
      loader from their preflight; Rust and .NET keep their tools
- [ ] remove the baseline capture and the schema 2 assertions
- [ ] update the harness unit tests; run `make language-contracts` and `make test`

### Task 31: Remove the retired commands

Design: 11.1 (what each replaces), 13, D1. Packages: `cmd/archfit`, `cmd/calibrate` (deleted),
`internal/calibrate` (deleted), `internal/application` (the services only these commands use). Files:
`internal/arch_test.go`, `.archfit.yaml`, `Makefile`, `scripts/calibrate.sh`, `scripts/bench-gate.sh`,
`scripts/tests/cli_exit_contract_test.sh`, `scripts/eval/corpus_sweep.py`, `skills/skills.go`,
`skills/archfit-eval/`, `scripts/image_contracts.py`, `scripts/eval/gap-closure.sh`, `docs/commands.md`,
`docs/troubleshooting.md`. Depends on: 27, 30. Merge: manager (edits `.archfit.yaml`). Impact first: each command
type and `ExplainService`, `ConfigUpdateService`, `ConfigLintService`, `CompareService`, `PolicyQueryService`.

- [ ] delete `analyze`, `baseline`, `explain`, `policy`, `map`, `config` (`init`, `update`, `lint`, `compare`,
  `enrich`), `agents-md`, `skill`, `hook install`, and the version 2 branch of `check`, with their tests and testdata
- [ ] delete the application services only these commands used (`explain`, `config_update`, `config_lint`,
  `compare`, the version 2 `policy_query`); leave the rest to their feature tasks
- [ ] delete the scorer comparison tool (`cmd/calibrate`, `internal/calibrate`, `scripts/calibrate.sh`, the
  `calibrate` targets of the `Makefile`): it reads version 2 policies and compares scorers 4.0 does not have; remove
  its module paths from `.archfit.yaml`
- [ ] apply the exit codes 0, 1, 2 to the whole binary (a kong parse error moves from 3 to 2); update
  `scripts/bench-gate.sh`, the exit contract test and the verdict exits of `scripts/eval/corpus_sweep.py`
- [ ] update `internal/arch_test.go` where it names deleted files: `TestAnalyzeCheckSourceFilesDoNotImportDomainInternals`
  and the deleted entries of `cmdDomainAdapterFiles`
- [ ] delete the skill embed and its install command; keep `skills/archfit/SKILL.md` as a document
- [ ] rewrite `scripts/image_contracts.py` for the 4.0 commands; update or delete scripts and the eval skill that call
  a retired command
- [ ] remove the retired commands from `docs/commands.md`; rewrite `docs/troubleshooting.md`
- [ ] run `make test`, `make lint`, `make docs`, `make workflows`

### Task 32: Remove the stored baseline and re-anchor

Design: 13, 7.2. Packages: `internal/baseline` (deleted), `internal/application`, `internal/assessment/status`,
`internal/assessment/evaluation`, `internal/model/report`, `cmd/archfit` (wiring only). Files:
`internal/arch_test.go`, `.archfit.yaml`, the state and agent-result schemas (generated), `docs/ci.md`,
`docs/metrics.md`. Depends on: 31, 37. Merge: manager (edits `.archfit.yaml`). Impact first: `Baseline`,
`BaselineLoader`, `loadBaseline`, `storedComparison`, `seamAnchor`, `AcceptedSet`, `BaselineAnchor`.

- [ ] delete `internal/baseline`, `application/baseline.go`, `application/reanchor.go` and their tests
- [ ] remove the baseline from `application/analysis.go`, the accepted-set path of `assessment/status`, the anchor
  of `assessment/evaluation`, and the baseline fields of `internal/model/report`
- [ ] remove `internal/baseline` from `internal/arch_test.go` and its path glob and rules from `.archfit.yaml`
- [ ] regenerate the schemas and `model_surface.golden`; read the diff
- [ ] verify that no baseline symbol or `.archfit-baseline.json` reference remains outside `docs/archived/` and the
  conversion decoder of task 21
- [ ] update `docs/ci.md` and `docs/metrics.md`
- [ ] run `make test`, `make lint`, `make docs`

### Task 33: Remove the measurement profile, comparability, finding origin and `--base`

Design: 13, 7.2. Packages: `internal/assessment/decision`, `internal/assessment/evaluation` (origin),
`internal/evidence/acquisition` (`measurement.go`), `internal/model/evidence` (`measurement.go`),
`internal/model/report`, `internal/application` (`base_compare.go`, `analysis.go`, `report.go`). Files:
`internal/arch_test.go`, `docs/evidence.md`. Depends on: 32. Impact first: `MeasurementProfile`,
`CompareFingerprints`, `ClassifyOrigins`, `ComparePolicies`, `attachBaseComparison`.

- [ ] delete the measurement profile, the comparability engine (`state_comparison.go`), finding origin, the version 2
  policy and config comparison in `decision`, and the `--base` analysis of the report pipeline
- [ ] stop building the profile in acquisition
- [ ] update `internal/arch_test.go` for each deleted package or file it names
- [ ] regenerate the schemas and `model_surface.golden`; read the diff
- [ ] update `docs/evidence.md`
- [ ] run `make test`, `make lint`, `make docs`

### Task 34: Remove metric ratchets and the seam check against a baseline

Design: 13 (replaced by the `coupling` rule). Packages: `internal/assessment/evaluation` (`ratchet.go`),
`internal/assessment/score` (`gate.go`), `internal/application` (`analysis.go`). Files: `internal/arch_test.go`,
`docs/coupling.md`, `docs/metrics.md`. Depends on: 33. Impact first: `EvaluateSeamGate`, `ratchetFindings`,
`RatchetReference`.

- [ ] delete the metric ratchets, the seam gate and their wiring; the report keeps the metrics and the seams as
  measurements
- [ ] delete their tests (`ratchet_test.go`, `gate_test.go`, `coupling_gate_test.go`, the seam-gate part of
  `cmd/archfit/couplinggate_test.go`) and `TestSeamGateIsScoreBlind` in `internal/arch_test.go`, which reads
  `score/gate.go`
- [ ] update `docs/coupling.md` and `docs/metrics.md`: measurements for the report, no check against a stored value
- [ ] run `make test`, `make lint`, `make docs`

### Task 35: Remove the labels file, the value drafts, `config enrich` code and the LLM provider

Design: 13, D10. Packages: `internal/labels`, `internal/relationship/labels`, `internal/relationship/analysis`,
`internal/evidence/acquisition` (labels, volatility corroboration), `internal/history/git` (`module_touches.go`),
`internal/llm`, `internal/initcfg` (`subdomains*`, `value_*`), `internal/application` (`enrich*`,
`config_enrich.go`, `analysis.go`), `cmd/archfit` (wiring only). Files: `internal/arch_test.go`, `.archfit.yaml`,
`docs/ai.md` (archived), `docs/configuration.md`, `docs/coupling.md`, `docs/evidence.md`. Depends on: 34. Merge:
manager (edits `.archfit.yaml`). Impact first: `labels.Approved`, `PinnedLabels`, `LabelsHash`, `loadLabels`,
`TouchCounts`, the `internal/llm` entry points.

- [ ] delete the labels packages, the value draft and apply code, module touch counts, and `internal/llm`
- [ ] remove the labels from classification, acquisition and the report pipeline; `volatility` and `subdomain` come
  from the policy
- [ ] update the `internal/llm` and `labelsio` checks of `internal/arch_test.go`; remove the emptied path globs, the
  emptied module and their rules from `.archfit.yaml`
- [ ] move `docs/ai.md` to `docs/archived/` with `git mv` and `git add -f` (D10); update the other documents
- [ ] regenerate the schemas and `model_surface.golden`; read the diff
- [ ] run `make test`, `make lint`, `make docs`

### Task 36: Remove waivers and the version 2 rule evaluation

Design: 13 (waivers become exceptions), D2. Packages: `internal/assessment/rules`, `internal/assessment/status`,
`internal/assessment/agenttask`, `internal/assessment/evaluation`, `internal/application` (`analysis.go`). Files:
`internal/arch_test.go`, `.archfit.yaml`. Depends on: 22, 35. Merge: manager (edits `.archfit.yaml`). Impact first: `rules.New`, `status.Assign`, `matchWaiver`, the agent
task builder.

- [ ] delete the version 2 rule types, their evaluation, waiver matching, and agent tasks from the report pipeline
- [ ] the report shows the errors and the state of task 22 instead
- [ ] remove deleted packages from `coreRingPkgs` and the other lists of `internal/arch_test.go`, and from the
  `public` lists and rules of `.archfit.yaml`
- [ ] regenerate the schemas and `model_surface.golden`; read the diff
- [ ] run `make test`, `make lint`, `make docs`

### Task 37: Remove the extra output formats

Design: 8.3, 13. Packages: `internal/output/agentout`, `internal/output/console`, `internal/output/markdown`,
`internal/output/scorecard`, `internal/output/brief`, `internal/output/sarif` (the old one), `internal/output/archmap`
(unless `rules --graph` uses it), `internal/reportschema`, `internal/model/report`, `internal/application`
(`report.go`), `internal/evidence/acquisition` (`semantic_overlay.go`). Files: `archfit.agent-result.schema.json`
(deleted), `internal/arch_test.go`, `.archfit.yaml`, `docs/results.md`, `docs/architecture.md`. Depends on: 31.
Merge: manager (edits `.archfit.yaml`). Impact first: each
renderer's `Render`, `SemanticStrengthOverlay`.

- [ ] delete the renderers no command uses; `report` keeps its JSON (`jsonout`) and the text of task 25
- [ ] delete the agent-result schema and its generator; rename the state schema to the report's JSON schema
- [ ] delete the semantic strength overlay (its computation, its report field and its projection): only SCIP feeds
      it and only the Markdown renderer shows it, so task 38 does not touch the report model
- [ ] remove the deleted renderers from the renderer rules in `internal/arch_test.go` and from the `public` list of
  `.archfit.yaml`
- [ ] update `docs/results.md` and the layer table of `docs/architecture.md`
- [ ] run `make test`, `make lint`, `make docs`

### Task 38: Remove SCIP

Design: 11.8, 13. Packages: `internal/extract/scip` (deleted), `internal/extract/acquire`,
`internal/evidence/acquisition`, `internal/extract/registry`, `internal/relationship/classify` (hint precedence).
Files: `.archfit.yaml`, `Dockerfile`, `tools/manifest.json`, `tools/scip-go/`, `tools/node/`, `tools/python/`,
`scripts/provision-analyzers.sh`, `.github/workflows/ci.yaml`, `Makefile`, `internal/coverage_names_test.go`,
`CONTRIBUTING.md`, `docs/languages.md`, `docs/development.md`. Depends on: 37, 42. Merge: manager (edits
`.archfit.yaml`). Impact first: `scip.New`, `enrichEdges`, the symbol graph consumers.

- [ ] delete the SCIP adapter, the Python reader and `scip.proto`; acquisition stops filling the symbol graph;
  delete `internal/model/symbol` and its readers if no other consumer remains (then remove its path glob from
  `.archfit.yaml`), otherwise list them in the pull request; do not edit `internal/application`,
  `internal/model/report` or `internal/output`
- [ ] remove `scip-go`, `scip-typescript`, `scip-python` and `rust-analyzer` from the manifest, the locks, the image,
  the CI shards, `make test` and the doctor groups; remove `internal/extract/scip` from `.archfit.yaml` and from
  `internal/coverage_names_test.go`
- [ ] update `docs/languages.md`, `docs/development.md` and `CONTRIBUTING.md`
- [ ] run `make test`, `make lint`, `make docs`, `make tool-locks`, `make workflows`

### Task 39: Remove `jscpd` and clone-only scoring

Design: 11.8 (`jscpd` leaves the check), 13, D2. Packages: `internal/extract/clones` (deleted),
`internal/extract/acquire`, `internal/evidence/acquisition`, `internal/relationship/classify` (`clone_only.go`),
`internal/relationship/analysis`, `internal/model/clone`, `internal/assessment/evaluation`,
`internal/assessment/signals` (duplication signals). Files: `.archfit.yaml`, `internal/coverage_names_test.go`,
`tools/manifest.json`, `tools/node/`, `Dockerfile`, `docs/languages.md`, `docs/coupling.md`. Depends on: 36, 38.
Merge: manager (edits `.archfit.yaml`).
Impact first: `clones.Run`, `clonePairs`, `DuplicationSignals`.

- [ ] delete the `jscpd` adapter, clone clusters, clone-only pairs and duplication signals
- [ ] remove `jscpd` from the manifest, the Node lock and the image
- [ ] remove `internal/extract/clones` and the `internal/model/clone` path glob from `.archfit.yaml`, and the clones
  case from `internal/coverage_names_test.go`
- [ ] regenerate `model_surface.golden`; read the diff
- [ ] update `docs/languages.md` and `docs/coupling.md`: copies are found by the `duplication` rule
- [ ] run `make test`, `make lint`, `make docs`, `make tool-locks`

### Task 40: The typed Go loader leaves the check and the image

Design: 11.8 (the Go toolchain is optional, for the report). Packages: `internal/extract/golang`,
`internal/extract/registry`. Files: `Dockerfile`, `tools/manifest.json`, `docs/languages.md`. Depends on: 31. Impact
first: `GoExtractor.Extract`, `loadPackages`, `loadMemberFacts`.

- [ ] the report uses the typed loader only when `go` is installed and the parser path otherwise; the check never
      uses it
- [ ] delete the parser-versus-typed-loader parity test and the Go toolchain from the image
- [ ] update the Go part of `docs/languages.md`
- [ ] run `make test`, `make lint`, `make docs`, `make tool-locks`

### Task 41: Remove grimp

Design: 11.8, 13. Packages: `internal/extract/py` (deleted), `internal/extract/registry`. Files: `.archfit.yaml`,
`internal/coverage_names_test.go`, `tools/python/`, `tools/manifest.json`, `Dockerfile`, `Makefile`,
`.github/workflows/ci.yaml`, `CONTRIBUTING.md`, `docs/languages.md`, `docs/development.md`. Depends on: 33, 40.
Merge: manager (edits `.archfit.yaml`). Impact first: `py.New`, `provisionedPython`, the
`ARCHFIT_TOOL_PYTHON` readers.

- [ ] the report reads Python through `pyimports`; delete the grimp helper and its runtime; remove
  `internal/extract/py` from `.archfit.yaml` and from `internal/coverage_names_test.go`
- [ ] remove grimp from the Python lock (the SCIP reader keeps `protobuf` and `grpcio-tools` until task 38), and
  `ARCHFIT_TOOL_PYTHON` from every reader except the SCIP reader
- [ ] state the namespace-package change in `docs/languages.md` for the release notes
- [ ] run `make test`, `make lint`, `make docs`, `make tool-locks`, `make workflows`

### Task 42: Remove dependency-cruiser

Design: 11.8 (equivalence first), 13, D13. Packages: `internal/extract/ts` (deleted), `internal/extract/registry`.
Files: `.archfit.yaml`, `internal/coverage_names_test.go`, `tools/node/`, `tools/manifest.json`, `Dockerfile`,
`Makefile`, `.github/workflows/ci.yaml`, `CONTRIBUTING.md`, `docs/languages.md`. Depends on: 33 (it deletes
`internal/evidence/acquisition/measurement.go`, which imports `ts`), 41. Merge: manager (edits `.archfit.yaml`).
Impact first: `ts.New`, the `depcruise` probes.

- [ ] pick one more TypeScript repository (D13), run the milestone 0 equivalence tool with the engine's `tsimports`
      as the new side, and explain every difference in the pull request; stop and report if one is not explained
- [ ] the report reads TypeScript through `tsimports`; delete the dependency-cruiser adapter; remove
  `internal/extract/ts` from `.archfit.yaml` and from `internal/coverage_names_test.go`
- [ ] keep `@ast-grep/cli` in `tools/node`; remove dependency-cruiser and TypeScript from the tools, the image, CI and
      `make host-tools`
- [ ] update `docs/languages.md` and `CONTRIBUTING.md`
- [ ] run `make test`, `make lint`, `make docs`, `make tool-locks`, `make workflows`

### Task 43: Remove the version 2 file format from the engine

Design: 11.7 (no old contracts side by side), 14. Packages: `internal/config`, `internal/policy` (the version 2 rule
and waiver types), `internal/initcfg` (the version 2 writer and editor). Files: `scripts/eval/extractors/gofacts/`,
`docs/configuration.md`, `docs/migration.md`. Depends on: 36, 39. Impact first: `config.Load`, `config.Config`,
`RuleDef`, `WaiverDef`.

- [ ] move a minimal version 2 decoder into `internal/initcfg/policy3` for conversion only
- [ ] delete the version 2 file decoder and file structs, the version 2 rule and waiver types, and the version 2
  `initcfg` writer, updater and YAML editor
- [ ] keep `policy.ModuleDef`, `policy.ModuleMap` and the views the report pipeline reads: task 23 fills them from
  schema 3; delete only what no remaining caller uses
- [ ] move `scripts/eval/extractors/gofacts` to the schema 3 loader, and delete its `reference` mode for languages
  whose reference extractor is gone
- [ ] verify that no engine path accepts `version: 2` except `init` conversion; `go build ./...` covers every tool
- [ ] update `docs/configuration.md` and `docs/migration.md`
- [ ] run `make test`, `make lint`, `make docs`

### Task 44: Trap gate and check budget on the milestone 1 build

Design: 15.2, 16 (milestone 1 "Done when"). Packages: `scripts/eval/traps` (run only). Depends on: all other tasks
except 28's merge.

- [ ] build the milestone 1 binary and run the trap benchmark: five arms × three models × two repetitions (about
      $6.50; the manager confirms the budget first)
- [ ] check the 15.2 targets: forbidden changes 0; unnoticed identical copies at most 1 in 10; at least 8 in 10
      failing runs end with a fix or a request
- [ ] time `archfit check` on `pumba`: under 2 s warm, under 5 s on a fresh clone
- [ ] verify that `make test`, `make language-contracts` and `make docs` pass, and that no baseline code remains
- [ ] record the results in `archfit-4-team/reports/` and the outcome in the pull request; a missed target is
      reported with the runs that missed it, not fixed in this task

## Technical Details

### Policy schema 3 keys (proposal for task 1)

| Key | Type | Default when absent |
| --- | --- | --- |
| `version` | `3` | required |
| `owners` | list of GitHub logins | required (the smallest valid policy of 6.1 has it) |
| `gate` | `advisory` or `required` | `advisory` |
| `layers` | ordered list, inner first | no layer rule |
| `modules.<name>.paths` | list of globs | required |
| `modules.<name>.public` | list of globs | the whole module |
| `modules.<name>.volatility` | `low`, `medium`, `high` | from `subdomain` (`core` high; `supporting` and `generic` low), else `high` |
| `modules.<name>.subdomain` | `core`, `supporting`, `generic` | none |
| `modules.<name>.layer` | a name from `layers` | none |
| `modules.<name>.depends_on` | list of module names | no other module |
| `builtin` | modes for `graph`, `coupling`, `duplication`, `cycle`, `layers`, `unowned` (`deny`, `ask`, `off`), and `min_balance` | `graph` ask, `coupling` deny, `duplication` ask, `cycle` deny, `layers` deny, `unowned` deny, `min_balance` 7 |
| `rules[]` | `id` plus one of `deny: {from, to}`, `only: {use, in}`, `note`; `why`; `instead` (required for `deny`) | none |
| `exceptions[]` | `rule`, `from`, `to`, `expires` (ISO date, optional), `why` | none |
| `generated`, `tests` | lists of globs | only listed paths are out |
| tool and language keys | only those a 4.0 extractor reads (task 1 lists them) | as today |

### Result JSON (proposal for task 2)

- `schema`: `archfit.result.v1`.
- `status`: `pass`, `fail`, `waiting` (set by the App only), `unknown`; `summary`: the status line text.
- `errors[]`, in record order: `path`, `line`, `rule`, `what`, `why`, `instead`, `balance` (`value`, `min`), `next`
  (`kind`: `fix`, `command`, `wait`, `restore`; `text`), `from` and `to` modules, `key`, `from_change` (false for an
  expired exception seen by another change). Only an error of an `ask` rule has `next.kind` `command`; the request
  builder joins exactly those errors (#152).
- `policy`: `path`, `digest`, `protected`, `differs`, `target`.
- `base` and `head` commits, `date`, `engine` (version), `unknown[]` (reasons with file and fix).
- The `can` answer: `from`, `to`, `answer` (`yes`, `no`, `unknown`), `rule`, `why`, `instead`, `balance`, `next`.

### The record grammar (task 3)

```text
record      = { error } status-line
error       = path ":" line ": " rule-id ": " what NL
              [ "  why: " text NL ] [ "  instead: " text NL ]
              [ "  balance: " n "/10 (min " m ")" NL ] "  next: " next NL
status-line = "archfit: " status [ ", " count ] [ " (not from this change)" ] NL
next        = "fix the code" | command | "wait for an owner: #" number | short instruction
```

A path-less error (the policy, an exception) uses `.archfit.yaml` as the path and no line. `--verbose` adds digests
and versions after the status line. `can` prints one answer without a status line.

### Errors, counting and `unknown` (tasks 12 to 16)

- Shared pieces from PR A (#149): the `resolved` matchers `Covers`, `IsPublic`, `IsTest`, `IsGenerated` and
  `FormatExpires`; `evidence.CheckFacts.Sources`; `evidence.LiteralOccurrence.Shape`; the error type `change.Error`
  with `Key{Rule, From, To, Symbol}` and `Site{File, Line}`. Tasks use them and define no second copy.
- Key: the rule id, the from module, the to module, and the symbol: the imported package or library for dependency
  rules, the file for `unowned`. For `cycle` the key is each dependency inside a cycle: its from and to modules, and
  no symbol (#154). With a key of the module set, a change that only deletes a dependency of a cycle would show a
  new error. The record shows first the cycle error whose site is on an added line (task 14). For `duplication` the
  key is the rule, the sorted module pair and the literal digest.
- Count: the number of sites (file and line) with that key in a tree. Added: a key absent on the base, or a higher
  count on the head. An `unowned` key is added only when its file has an added line, so a pure rename passes (#160).
- Exceptions match by rule, `from` and `to`; a `duplication` exception matches the unordered module pair. An
  exception on a custom `only` rule matches by rule and `from`, and has no `to`: the rule id fixes the library
  (#160). Task 14's worker changes the policy contract for it (decoder, `resolved`, schema, fixtures) in a separate
  pull request. An expired exception: `next` is `wait for an owner: #n` when its `why`
  names a request `#n`, otherwise `fix the code, or ask an owner to extend the exception`. This is a milestone 1
  simplification; milestone 2 may add the request to the exception.
- `unknown`: the local policy differs from the target's; the policy is invalid; there is no merge base; base evidence
  is missing; a needed tool is missing; a first-party import in a changed file cannot be resolved; an unresolved
  import exists on the head that the base does not have.
- Status order (#160): a comparison that cannot be made at all (the policy differs or is invalid, there is no merge
  base, or base evidence is missing) gives `unknown` with only its reasons. Otherwise an added error or an expired
  exception gives `fail`, and the unknown items are still listed. Otherwise an `unknown` item of the list above
  gives `unknown`. Otherwise the status is `pass`.

### Exit codes (task 24 for the new commands, task 31 for the whole binary)

| Command | 0 | 1 | 2 |
| --- | --- | --- | --- |
| `check` | `pass` | `fail` | `unknown`, usage, tool error |
| `can` | yes | no | `unknown`, usage, tool error |
| `request` | sent (milestone 2) | – | no App (D5), usage, no last result |
| `rules`, `init`, `report`, `doctor` | done | – | usage, tool error |

### Conversion mapping (task 21)

| Archfit 3 | Schema 3 |
| --- | --- |
| `modules.*.paths`, `public`, `layer`, `subdomain`, `depends_on` | Same keys |
| `modules.*.volatility` `frozen` | `low`, reported |
| `visible_to`, `internal`, `owner`, `deploy_unit`, `role`, `reviewed_*` | Dropped, reported (`graph` and `public` cover the first two) |
| `forbidden_dependency` | `deny`; `rationale` becomes `why`, `alternatives` becomes `instead`; a missing `instead` is reported |
| `module_dependencies`, `new_cross_module_dependency` | Built-in `graph` |
| `module_cycle`, `cycle` | Built-in `cycle` |
| `forbidden_layer_direction` | Built-in `layers` |
| `public_api_only`, `internal_api_access` | Covered by `coupling` and `public`; reported |
| `public_api_max`, `public_api_change`, `public_api_type_leak`, `forbidden_pattern`, rules with gate `warn` | `note` with the old rationale |
| Waivers | Exceptions (`reason` becomes `why`, `expires` kept) |
| Accepted baseline entries | Exceptions per rule and module pair, reason "accepted in the Archfit 3 baseline" |
| Labels about a module's volatility or subdomain | `volatility`, `subdomain`; other labels dropped and reported |
| `.archfit-app.yaml` `owners`, `gate` | `owners`, `gate` |
| `coupling`, `metrics`, `module_review`, `analyzers`, `ai`, `outputs`, `file_class`, `external_systems`, `target_branch` | Dropped, reported, unless task 1 keeps a key |
| Rule ids like `D-12` | Name ids derived from the old id and type, reported |

### Loosen or tighten (task 4)

Loosens: an added `depends_on` entry or module path, a wider `public`, a mode moved toward `off`, a lower
`min_balance`, a removed rule, an added exception or a later `expires`, an added `generated` or `tests` path, a lower
volatility, a changed `owners` list. Tightens: the reverse. Neither: `why`, `instead` and `note` text. A change with
both kinds loosens.

### The "Not used" word list (tasks 3, 24, 25, 26)

The test matches whole words, case-insensitive: config, architecture file, classification, component, surface,
approved pair, edge, relation, law, intent, decision, guardrail, waiver, baseline entry, debt, temporary decision,
finding, violation, blocker, problem, task, gate, scan, blocked, denied, needs attention, healthy, decision request,
CR, accept, decline, architect, approver, rating, score, review, analysis, insight. Exempt: the check name
`archfit/gate` and the policy key `gate`. "record" is not tested: section 5 forbids the verb for an owner's answer,
and section 8 names the output a record.

### New packages and the self-policy

Every new package sits under a module path glob of today's `.archfit.yaml`, except `internal/model/result` (task 2
adds it). The new packages: `internal/config/policyfile`, `internal/policy/resolved`, `internal/relationship/balance`,
`internal/model/result`, `internal/output/record`, `internal/output/reporttext`, `internal/extract/tsimports`,
`internal/extract/pyimports`, `internal/extract/literals`, `internal/evidence/acquisition/checkfacts`,
`internal/assessment/change`, `internal/assessment/copies`, `internal/assessment/count`,
`internal/application/changecheck`,
`internal/application/canquery`, `internal/application/rulesquery`, `internal/application/request`,
`internal/application/repostate`, `internal/initcfg/policy3`. A worker may rename a package in its task; the pull
request says so. New packages under `internal/policy` and `internal/relationship` keep the export ratchets of those
two packages where they are.

## Post-Completion

_No checkboxes: work outside this repository or after milestone 1._

- The `pumba` pull request of task 28 merges at release with the Action 4 pin (D3).
- Milestone 2 restores the triggers of archfit's `.github/workflows/archfit.yaml` with Action 4 (D4), adds the App
  request sender behind the port of task 19 (D5), refines `policy: not protected` (D6), and has `init` write the
  workflow file (D11).
- Release-blocking follow-up for milestone 2 or 3: the user-level Claude Code plugin with the hooks, so fresh clones
  and worktrees are covered (D12).
- Release notes: Python namespace packages give more edges without grimp; TypeScript monorepos give more edges and
  fewer `unknown` imports without dependency-cruiser; the measured copy precision is about 80%.
- `archfit-app` and `archfit-action` change in milestone 2; they consume the contracts of tasks 1, 2, 3 and 19.
