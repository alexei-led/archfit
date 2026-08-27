# Archfit language-evidence improvements

Date: 2026-08-27
Status: PLANNED — awaiting owner review, not started.
Branch: `plan/archfit-language-evidence-improvements` (base `733b36c`).
Contract under change: `archfit.architecture-state.v1`
(`docs/design/architecture-state-reporting.md`).
Design review: Fusion run `7bb04756-4400-465a-8f45-4b1ae34d5cd7` (2026-08-27),
NO-GO on the first draft. Its corrections are folded into §2 and the task list;
§8 records what changed and why.

---

## 1. The problem

`archfit check` cannot return `HEALTHY` on any repository, however good its
architecture is.

`internal/assessment/state/decision.go` `Decide` returns `Healthy` only when all
nine dimensions are `Measured`. Three collectors in
`internal/assessment/evaluation/dimensions.go` assign `state.Partial`
**unconditionally** — verified by scanning every `dim.Status` assignment in the
file:

| Collector              | Line | Reason recorded in source                              |
| ---------------------- | ---: | ------------------------------------------------------ |
| `complexityDimension`  |  422 | "v1 ships no cognitive-complexity analyzer"            |
| `testabilityDimension` |  460 | "v1 does not execute a target repository's test suite" |
| `operationsDimension`  |  524 | "nothing observes what actually runs"                  |

Measured on this branch, on archfit's own repository — 0 blockers, hard gates
passing, the most favourable possible input:

```
verdict: needs_attention
decision: {"hard_gates":"pass","active_blockers":0,"attention_dimensions":2,"unknown_dimensions":4}
coverage: {"measured":5,"partial":3,"unmeasured":1}

intent  measured   structure measured   modularity measured
coupling measured   change_locality measured
complexity partial ←  testability partial ←  operations partial ←
drift    unmeasured ←  (comparison not requested)
```

`check` exits 2 here and on every repository. The exit-0 branch of the frozen
exit table is dead code in production; only synthetic unit tests that construct
a state directly (`internal/assessment/state/decision_test.go:40`) reach it.

Consequences: the verdict carries no information (a clean repo and a broken one
both print `NEEDS_ATTENTION`); CI adoption is blocked because the success exit
code is unreachable; and the honesty contract inverts — "never false green"
became "permanent false amber", which trains readers to ignore the field that
matters.

### 1.1 Three hardcoded collectors are necessary but not sufficient

Fusion review found the first draft of this plan too narrow, and the objection
verified. Closing the three hardcoded collectors does **not** by itself make
`HEALTHY` reachable, because two other dimensions have their own gates:

- **drift** returns `Unmeasured` whenever `!ref.SeamsComparable`
  (`dimensions.go:571-581`), which includes the ordinary "no baseline stored"
  and "comparison not requested" cases. Confirmed above: archfit's own repo
  reports drift `unmeasured` with `comparison.status: not_requested`.
- **coupling** is forced to `Partial` when any edge abstains
  (`dimensions.go:265-275`) or when TypeScript import resolution is materially
  incomplete (`dimensions.go:302-310`).

Coupling's behaviour is correct and evidence-dependent — it reports `measured`
on archfit's own repo because nothing abstained there. Drift's is a **product
lifecycle gap**: on a first run there is nothing to compare against, and calling
that "unmeasured evidence" is different from "no comparison was requested".
Task 4 resolves it as an explicit policy decision, not a collector change.

### 1.2 `Measured` currently means two different things

The deeper defect, and the reason no collector should be written first. Measured
on archfit's own repo:

```
structure        measured  + unknown: "structure of 719 edges leaving the module map"
change_locality  measured  + unknown: "essential vs accidental volatility"
coupling         measured  + (no unknowns in this run)
complexity       partial   + unknown: (hardcoded)
```

`intent`, `structure`, `modularity`, and `change_locality` report `Measured`
while naming an `UnknownFact` — a *lenient* reading: "the denominator is
complete; these are footnotes." `coupling` reports `Partial` for the structurally
identical situation (denominator measured, unknowns inside it) — a *strict*
reading. Verified by counting status assignments per collector:

```
intent          Measured=1 Partial=0 AppendsUnknown=1
structure       Measured=1 Partial=0 AppendsUnknown=1
modularity      Measured=1 Partial=0 AppendsUnknown=1
coupling        Measured=1 Partial=2 AppendsUnknown=2
changeLocality  Measured=1 Partial=1 AppendsUnknown=2
```

Any new collector must answer "do I report `Measured`+unknown, or `Partial`?"
and today there is no contract to answer it. Writing three collectors against an
undefined term would triple the inconsistency. **Task 1 fixes the term before
any collector is touched.**

### 1.3 What this plan is not

Not "lower the bar so the tool shows green." The governing rule stays: missing
evidence is `partial`/`unmeasured`, never a healthy zero. Task 1 fixes a
definition; it does not weaken one. Every promotion added later is gated by that
definition and by a test that fails if a collector promotes while a required
fact is missing. A repository with no coverage file keeps `testability` partial
and keeps exiting 2 — by design.

---

## 2. Evidence-source matrix

Candidate → dimension → fact → value → cost → default → failure semantics.
`native` means the fact is already computed inside archfit today.

### Stage A — already computed, zero new tools, zero new subprocesses

| Source                                                                                              | Dimension    | Fact produced                                                              | Decision value                                                                         | Cost   | Default | Failure semantics                                   |
| --------------------------------------------------------------------------------------------------- | ------------ | -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- | ------ | ------- | --------------------------------------------------- |
| `deployunit.Detect` (Dockerfiles `:335`, k8s manifests `:364`, TS workspaces `:213`, `pyproject.toml` `:267`, Go mains `:134`) — **already runs** | `operations` | corroborated deploy units per path; declared-vs-corroborated reconciliation | separates "a module declares a deploy unit" from "a manifest for it exists in the tree" | native | on      | no manifest → `partial`, fact named                  |
| `ownership.Resolve` (CODEOWNERS, `:56-60`) — **already runs**                                       | `operations` | owner provenance per module (`codeowners` \| `git-author` \| `declared`)    | a git-author fallback owner is not an ownership statement, yet distance depends on it   | native | on      | no CODEOWNERS → provenance `git_author`, fact named  |
| `evidence.SyntaxFact.StartLine/EndLine` (`evidence.go:64-76`) — **already collected**                | `complexity` | function/method length distribution: count, p50, p90, max, over-threshold tail, per module | locates god-functions at seams; diagnostic only, does not promote on its own            | native | on      | no syntax facts → `partial`, fact named              |
| module-graph shape (existing relationship facts)                                                    | `complexity` | dependency-chain depth and degree tail per module                          | the architecture-level complexity claim; this is what promotes the dimension            | native | on      | graph incomplete → `partial`, fact named             |

### Stage B — supplied coverage, opt-in, parsed not executed

| Source                                                            | Dimension     | Fact produced                                | Decision value                                              | Cost           | Default | Failure semantics                                  |
| ----------------------------------------------------------------- | ------------- | -------------------------------------------- | ------------------------------------------------------------ | -------------- | ------- | -------------------------------------------------- |
| Go coverprofile (`go test -coverprofile`)                         | `testability` | covered/total statements per file → per module | whether the modules archfit judges are actually exercised     | file read (ms) | opt-in  | absent/unparsable/stale/unmapped → `partial` + named unknown |
| LCOV (`c8`, `vitest`, `jest`, `cargo llvm-cov --lcov`)            | `testability` | covered/total lines per file → per module     | same, TS/JS and Rust                                          | file read (ms) | opt-in  | same                                               |
| coverage.py JSON (`pytest-cov --cov-report=json`)                 | `testability` | covered/total statements per file → per module | same, Python                                                  | file read (ms) | opt-in  | same                                               |
| `cargo llvm-cov --json --summary-only`                            | `testability` | per-file summary counts                       | same, Rust when LCOV is not produced                          | file read (ms) | opt-in  | same                                               |

**Coverage is ingested, never executed.** See §3.2.

### Stage C — deferred, explicitly not in this plan

| Candidate                                              | Dimension     | Why deferred                                                                                                                                                    |
| ------------------------------------------------------ | ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| archfit executing `go test` / `pytest` / `cargo test`  | `testability` | arbitrary code execution in a tree the user may not own; non-hermetic; network/DB dependent; not byte-reproducible. Violates the approved v1 decision. Panel consensus: reject from `analyze` and `check`. A separate future command could produce an artifact that `analyze` then reads. |
| `gocognit` / `lizard` cognitive complexity             | `complexity`  | Stage A module-graph evidence is native and states an architecture claim. Revisit only if corpus evidence shows graph shape ranks a seam materially worse than a code-level metric. |
| `knip` (ISC, maintained)                               | `modularity`  | overlaps `scip-typescript` unused-export facts already collected; no non-duplicated evidence demonstrated.                                                        |
| `import-linter`                                        | `intent`      | duplicates archfit's own `rules:` engine; two rule languages in one report is a UX regression.                                                                    |
| `cargo audit` / `cargo deny`, SBOM                     | `operations`  | security/supply-chain is a separate report family. CVEs in an architecture verdict is the "do not blur concerns" failure.                                          |
| `tsc --noEmit`, `pyright --outputjson` as findings     | —             | compiler/type correctness is not architecture. Keep as tool-health signals only.                                                                                   |
| Observed **runtime** topology (live cluster, applied state) | `operations` | requires cluster credentials; out of scope for a static CLI. This is why Task 5 claims *declared-topology completeness*, not observation.                          |

---

## 3. Design decisions

### 3.1 The promotion criterion (the abstain-vs-fake line)

Each dimension declares a fixed **required-fact set**. A collector may report
`measured` **iff every required fact is either observed, or provably
not-applicable to this tree**:

```
measured   ⟺  required ∖ (observed ∪ not_applicable) = ∅
partial    ⟺  otherwise, and at least one required fact observed
unmeasured ⟺  no required fact observed
```

`UnknownFact` entries remain permitted alongside `measured` **only** for facts
declared out-of-claim in the contract (Task 1) — e.g. structure's "edges leaving
the module map" is outside structure's claim about *declared* modules. Anything
inside the claim that is missing forces `partial`. This is what makes the
lenient and strict readings in §1.2 one rule instead of two.

Three properties make it testable, not aspirational:

1. The required-fact set is a compile-time constant per dimension, not a runtime
   count, so a fact cannot be quietly dropped from the denominator.
2. `TestPromotionIsMonotonic` (Task 4) asserts, per dimension, that removing any
   single required fact flips `measured` → `partial`.
3. `TestNoMeasuredDimensionHasInClaimUnknowns` (Task 4) asserts every
   `UnknownFact` carried by a `measured` dimension is declared out-of-claim.

"Not applicable" is deliberately narrow and must be established by the
producer's own probe — the discipline `registry.ProjectPresent` already enforces
for language analyzers. A tree with no Rust crate is not missing Rust coverage. A
tree with a Rust crate and no coverage file is. Per panel: in a monorepo,
denominators stay per module and per language; a supported language's evidence is
never averaged over an unsupported one.

### 3.2 Coverage is supplied, not executed

`docs/design/architecture-state-reporting.md` records as approved: *"V1 does not
execute a target repository's test suite as measurement evidence."* This plan
keeps that decision and builds the supplied-input path it implies but which does
not exist (verified: no `CoverageFile`, `SuppliedCoverage`, or `lcov` symbol
exists anywhere in `internal/` or `cmd/`).

Executing a target's tests would be non-hermetic, slow, non-reproducible, and an
arbitrary-code-execution surface. Ingesting a file the user's CI already produced
is deterministic, hermetic, cacheable by content hash, and honest.

Panel correction — **freshness must bind to a commit, not a timestamp.** Coverage
formats carry no content hash of the sources they describe, so file mtime proves
nothing: a profile from a different commit can be newer than the checkout. The
contract is therefore:

- the coverage source is accepted only when accompanied by the commit SHA it was
  produced from (config `source_ref`, or a sidecar `<file>.ref`);
- a profile whose SHA ≠ the analyzed `source_ref` is reported `stale` and forces
  `partial`, regardless of mtime;
- when no SHA is available, the source is `unverified` and forces `partial` — it
  is reported and used for diagnostics, never for promotion.

Config shape (new `coverage:` block, schema additive):

```yaml
coverage:
  enabled: true          # default false — opt-in
  gate: warn             # off | warn | fail, reusing the analyzer GateMode
  require_ref: true      # a profile without a matching commit SHA cannot promote
  sources:
    - path: coverage.out
      format: go-coverprofile   # auto | go-coverprofile | lcov | coverage-py-json | llvm-cov-json
      ref_file: coverage.out.ref
    - path: coverage/lcov.info
      format: lcov
```

### 3.3 Path normalization is the load-bearing risk

Coverage formats disagree on path shape and two are documented to emit absolute
paths. A path that fails to normalize yields zero covered files, which a naive
implementation reads as "nothing is tested" — the worst failure mode here.

Verified upstream behaviour:

- **Go coverprofile** prefixes blocks with the *import path* plus filename for
  non-local packages but the *full file path* for local packages
  (golang/go#40251); one profile can contain both shapes.
- **coverage.py** stores **absolute** paths by default; relative only with
  `[run] relative_files = True`.
- **LCOV** `SF:` records are whatever the producer wrote — absolute under most
  Node runners.
- **llvm-cov JSON** filenames are absolute.

Contract (Task 7): every ingested path is reduced to a ScanRoot-relative slash
path using the module-prefix strip `internal/extract/golang` already applies via
`pkg.Module.Dir`. A path that cannot be reduced is **not** dropped silently — it
increments `unresolved_coverage_paths`, which is published as a metric and forces
`partial` when non-zero.

### 3.4 JSON envelope shape is established once, in Task 3

Fusion review found the first draft inconsistent: Task 3 read
`d["architecture_state"]["dimensions"]` while Tasks 6, 7 and 10 read top-level
`d["dimensions"]` and `d["comparison"]`. Only one can be right, and an executor
hitting the wrong one gets a `KeyError` mid-task.

Task 3 is the first task that reads the JSON output, so it **records the actual
envelope shape in its fixture README**, and every later verification command in
this plan uses that recorded shape. The Task 3 snippet reads the root
defensively (`raw.get("architecture_state", raw)`) precisely so it cannot fail
before the shape is confirmed; later tasks must not copy that defensive form once
the answer is known.

### 3.5 What must not change

`gitnexus_impact(target="Decide", direction="upstream", depth=3, repo="archfit")`
reports **CRITICAL**: 5 impacted symbols across 6 execution flows
(`application.Execute` ×3, `scoreBaseTree`, `attachBaseComparison`, `assess`).

**No task modifies `Decide`, its signature, or its semantics.** The verdict
aggregator stays metric-blind and rule-identical; only what the collectors
legitimately report changes. By contrast
`gitnexus_impact(target="testabilityDimension", …)` reports **LOW** — 3 impacted
symbols contained inside `Evaluation` → `State`. The blast radius of this plan is
deliberately kept on the low-risk side of that boundary.

Also unchanged: finding IDs and statuses, baseline behaviour, `--base`,
deterministic serialization, five-format parity, comparability rules, the exit
table 0/1/2/3, and the absence of any repository-level scalar.

---

## 4. Staging and rollback

Each task is one commit and is independently revertible.

| Stage | Tasks | Ships                                                                        | Revert boundary                                                    |
| ----- | ----- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| 0     | 1–3   | contract, audit, and reachability characterization; no *production*-code changes (Task 3 adds a test and fixture) | revert commits; docs, fixture and test disappear, no behaviour change |
| 1     | 4     | implement Task 2 audit decisions (status semantics and implementation fixes) | revert commit; Task 3 reachability outcome may change              |
| A     | 5–7   | drift lifecycle policy; `operations` and `complexity` reach `measured`      | revert commit; dimensions return to current state                   |
| B     | 8–10  | supplied-coverage ingestion; `testability` reaches `measured`               | revert commit; `coverage.enabled` defaults false, no behaviour change |
| C     | 11–13 | renderers, docs/schema, corpus + baseline gate                             | revert commit                                                       |

Stage 0 must complete before Stage 1: Task 3 characterizes reachability (Outcome
A or B), and the owner decides from the Task 2 audit whether to attempt the
status-semantics changes that might unlock Outcome A. If Outcome B is permanent
(required by product constraints), Stage 1 is skipped and the plan is closed as
"informed decision: HEALTHY is not reachable, by design".

---

## 5. Tasks

### Task 1: Define the nine-dimension evidence contract

- [ ] Write the contract: claim, applicable-when, denominator, in-claim vs out-of-claim facts, and the exact measured/partial/unmeasured conditions per dimension
- [ ] Record the lenient-vs-strict decision from §1.2 explicitly
- [ ] Record the drift "comparison not requested" decision (input to Task 4)
- [ ] Reconcile the contract against current behaviour and list every deviation

Justification: §1.2. `Measured` currently means "denominator complete, footnotes
allowed" in four collectors and "denominator complete and nothing unknown" in
another. Three new collectors written against an undefined term would triple the
inconsistency. This task writes no code and changes no output; it produces the
definition every later task is tested against. Panel consensus recommendation #1.

Files:

- `docs/design/evidence-contract.md` (new) — one section per dimension with the
  eight fields above, plus a deviation table listing, for each of the nine, where
  today's behaviour differs from the contract and whether Task 2 ratifies it or
  Task 3 changes it.

Preconditions: none — this is the entry point.

Postconditions: a single definition of `Measured` exists. For every dimension it
is decidable, from the document alone, whether a given run should report
measured, partial, or unmeasured. Every deviation is listed with an owner task.

Fitness gate:

- No two dimensions carry contradictory `Measured` definitions.
- Every `Measured` condition is stated as a predicate over facts, not prose
  judgement.
- Every `UnknownFact` that a `measured` dimension may carry is explicitly marked
  out-of-claim, with the reason.
- The deviation table accounts for all nine dimensions — none omitted.
- The drift decision is stated with its consequence for the exit table.

Impact commands:

- `gitnexus_context(name="Dimension", file="internal/assessment/state/state.go", repo="archfit")`
- `gitnexus_impact(target="buildDimensions", direction="downstream", depth=2, repo="archfit")`

Verification commands:

```sh
test -f docs/design/evidence-contract.md
for d in intent structure modularity coupling change_locality complexity testability operations drift; do
  grep -q "^## $d" docs/design/evidence-contract.md || echo "MISSING SECTION: $d"
done
grep -c "Measured when:" docs/design/evidence-contract.md   # expect 9
grep -c "Out-of-claim:" docs/design/evidence-contract.md    # expect 9
npx --yes markdownlint-cli2 'docs/design/evidence-contract.md' || true
```

Manual checks: for each of the nine, read the contract next to the collector body
and confirm the contract describes the collector's *intent*, not merely its
current code. Confirm the four lenient collectors' unknown facts are genuinely
out-of-claim and not an excuse.

Rollback: delete the document; nothing depends on it yet.

---

### Task 2: Characterize current behaviour against the contract

- [ ] Audit all nine dimensions against their Task 1 contract definitions
- [ ] Record every mismatch between contract and current code
- [ ] Classify each mismatch: status-semantics change, implementation fix, or new collector required
- [ ] Do not assume current behaviour satisfies the contract — assume it does not

Justification: Task 1 is prose; this measures reality against it. The first draft
of this plan assumed the six evidence-dependent collectors already satisfied the
contract and could be routed through a shared `Promote` with byte-identical
output. Fusion review rejected that: the audit must come first, because the
mismatch set determines whether Task 4 onward needs status changes, code fixes,
or genuinely new evidence. Writing the shared promotion helper before knowing the
gap would bake in an unvalidated contract.

**Status reclassification is a verdict change.** If the audit concludes a
dimension's status semantics must change, that changes verdicts, five-format
output, exit behaviour, schema, and goldens — even though `Decide` is untouched.
That is honest and expected, not a defect, but it is an owner decision recorded
here rather than an implementation detail buried in a later task.

Files:

- `docs/design/evidence-contract-audit.md` (new) — nine rows, one per dimension:
  contract definition (Task 1), current behaviour with the symbol and branch that
  produces it, mismatch description, and classification.
- `docs/design/evidence-contract-audit-scope.txt` (new) — machine-readable
  companion: one line per file path that the audit authorises a later task to
  change, in the form `<path>\t<classification>\t<dimension>`. Task 4 diffs its
  changed-file set against this artifact. The first draft had Task 4 read a
  `/tmp/audit-scope.txt` that no task produced; this replaces it with a committed,
  reviewable file.
- No code changes in this task.

Preconditions: Task 1 merged; contract locked.

Postconditions: a complete mismatch table exists. Every dimension is classified.
The owner can decide, from this table alone, which mismatches are ratified and
which are fixed, before any code is written.

Fitness gate:

- All nine dimensions have a row; none omitted.
- Each row cites the symbol and branch producing current behaviour, not a bare
  line number — line citations drift, symbols do not.
- Each mismatch carries exactly one classification.
- The three hardcoded-`Partial` collectors are confirmed to have no code path
  reaching `Measured`, and are classified as "new collector required".
- No code changes: `git diff --stat` touches only the new document.

Impact commands:

- `gitnexus_context(name="buildDimensions", file="internal/assessment/evaluation/dimensions.go", repo="archfit")`
- `gitnexus_impact(target="Decide", direction="upstream", depth=3, repo="archfit")` — record the CRITICAL blast radius in the audit so any proposed status change is reviewed against it

Verification commands:

```sh
test -f docs/design/evidence-contract-audit.md
test -f docs/design/evidence-contract-audit-scope.txt
grep -c '^## ' docs/design/evidence-contract-audit.md                      # expect 9
grep -c 'Mismatch classification:' docs/design/evidence-contract-audit.md  # expect 9
# every scope line must be tab-separated path/classification/dimension
awk -F'\t' 'NF!=3{print "MALFORMED SCOPE LINE: " $0; bad=1} END{exit bad}' \
  docs/design/evidence-contract-audit-scope.txt && echo "SCOPE FILE OK"
# every scope path must exist today (a new file must be declared in a later task instead)
cut -f1 docs/design/evidence-contract-audit-scope.txt | while read -r p; do
  test -e "$p" || echo "SCOPE PATH MISSING: $p"
done
git diff --stat                        # expect only the two new docs
make build && make test-fast           # unchanged behaviour
```

Manual checks: for each of the nine, read the contract next to the collector body
and confirm the classification is right. An easy classification is not an excuse
for missing a real gap.

Rollback: delete the audit document; no code was changed.

---

### Task 3: Characterize `HEALTHY` reachability with a hermetic fixture

- [ ] Build a minimal, hermetic, Go-only fixture
- [ ] Run `analyze` and `check` against it and record the outcome
- [ ] Outcome A: `healthy` reached — assert it, with parity and determinism
- [ ] Outcome B: not reached — emit an impossibility report naming each blocker and its remedy

Justification: the exit-0 branch has never executed end-to-end. This task
establishes ground truth for Stages A and B.

**Dual-outcome gate.** Fusion review found the first draft's gate impossible:
it asserted a blocking set of exactly `{complexity, testability, operations,
drift}`, but a fixture alone cannot change those, so a success-only gate would
never pass and would pressure an executor to weaken the contract to force green.
Both outcomes below are valid completions:

- **A:** the fixture reaches `healthy` and `check` exits 0. `Decide`
  (`internal/assessment/state/decision.go:68-75`) verifies **three** conditions,
  not just nine measured dimensions, so Outcome A asserts all three explicitly:
  `unknown_dimensions == 0`, `active_diagnostics == 0`, and `hard_gates == pass`.
  Review round 7 verified that active_diagnostics, not active_blockers, controls
  the healthy verdict. The fixture must emit no advisory, no diagnostic, and no
  unresolved facts.
- **B:** it does not, and the test emits a report naming each blocking dimension
  or diagnostic, the symbol that blocks it, and the remedy class.

**Outcome B has two sub-kinds and they must be distinguished.** Fusion review
found the first draft ambiguous — Task 13 read any Outcome B as grounds to close
the plan, while Task 3 expected Outcome B as the normal pre-collector result. The
report must classify every blocker as exactly one of:

- **B-temporary:** remedy is "new collector required" or "status semantics change
  approved in Task 2". Stages 1/A/B are expected to clear it. This is the normal
  result at this point in the plan and does **not** close it.
- **B-permanent:** remedy is "none — this is an accepted product constraint".
  Only this kind can close the plan, and only with explicit owner ratification
  recorded in the Task 2 audit.

Outcome B-temporary naming exactly `{complexity, testability, operations}`
is the expected result before Tasks 5–12 land. **Drift must NOT be in the blocking
set** because `materializeFixture` persists a comparable baseline before returning
(§3.4, lines ~512-517), so drift becomes measured on the first analyze run inside
Task 3. Task 13 flips the test to require Outcome A once those three collectors
exist.

**Hermeticity.** Fusion flagged that a multi-language fixture pulls in tool and
portability dependencies before basic reachability is established. The fixture is
therefore Go-only, needs no network, no language servers, and no tools beyond the
Go toolchain. Git history and CODEOWNERS are created deterministically by the
test's setup helper, not assumed from the developer's environment.

Files:

- `testdata/fixtures/reachability/` (new) — minimal Go module: two declared
  modules with owners, `CODEOWNERS`, `Dockerfile`, one enabled rule that passes,
  no abstained edges.
- `testdata/fixtures/reachability/` holds only **committed source**: the Go
  module, `CODEOWNERS`, `Dockerfile`, and `.archfit.yaml.tmpl`. It contains no
  git repository, no baseline, no coverage file, and no ref file.
- `testdata/fixtures/reachability/README.md` (new) — every file, the dimension it
  serves, the contract fact it satisfies, and the **recorded JSON envelope shape**
  observed on first run (see §3.4).
- `cmd/archfit/integration_reachability_test.go` (new) — owns a
  `materializeFixture(t, withCoverage)` helper that is the **only** way this
  plan's fixture is run. It copies the committed source into `t.TempDir()`, runs
  `git init` and one commit there, renders `.archfit.yaml` from the template,
  then runs `archfit baseline` to persist a comparable baseline — which is what
  makes drift comparable, per the finding above. It returns the temp repo path
  and the rendered config path.

  When `withCoverage` is true it additionally generates `coverage.out` via `go
  test -coverprofile` inside the temp repo, writes `coverage.out.ref` from that
  repo's `HEAD`, and renders the config with `coverage.enabled: true`. **Task 3
  calls it with `withCoverage=false`**: the `coverage:` config block does not
  exist until Task 8, so rendering that key earlier risks failing config load.
  Task 10 is the first caller to pass `true`.

**Why a temp repo and not committed artifacts.** Fusion review established two
facts that make committed coverage impossible: a tracked `coverage.out.ref`
cannot contain the SHA of the commit that contains it, and a fixture directory
inside this repository has no git history of its own — `git -C
testdata/fixtures/reachability rev-parse HEAD` resolves to the *parent* archfit
repository, so any ref taken that way is meaningless. Materializing into
`t.TempDir()` removes both problems and makes the fixture hermetic. Every CLI
gate in Tasks 3, 10 and 13 runs through this helper; none runs against the
committed directory directly.

Preconditions: Tasks 1–2 merged; audit table reviewed and its classifications
accepted by the owner.

Postconditions: reachability is established as fact, not hypothesis. Either the
exit-0 branch is proven to run, or every blocker is named with a remedy that
Stages A and B must deliver.

Fitness gate:

- Exactly one of Outcome A or Outcome B holds; the test neither hangs nor panics.
- Outcome B's report names, per blocking dimension, the blocking symbol and the
  remedy class from the Task 2 audit.
- The fixture is hermetic: no absolute paths, no environment variables, no tool
  beyond the Go toolchain, and a fresh clone reproduces the same result.
- Two consecutive runs are byte-identical.
- Format parity is asserted through the existing harnesses:
  `cmd/archfit/format_matrix_test.go` for the non-JSON formats and
  `cmd/archfit/byteidentical_test.go` for JSON. Fusion verified these are two
  separate harnesses, not one.
- **Drift is driven by the persisted baseline, not by `--base`.** Verified in
  code, because two review rounds contradicted each other on this point:
  `internal/application/analysis.go:300-302` passes `Anchor: seamAnchor(base,
  runCtx)` into **`evaluation.Score`** (not `Assess`), where `base` is the
  *persisted baseline*; `seamAnchor` (`:336-355`) returns a non-comparable anchor
  when `base.State == nil`, when the baseline is legacy, or when config/model/
  label/rubric fingerprints differ. `driftDimension(diag, ref)`
  (`dimensions.go:568-581`) reads `ref.SeamsComparable` from that anchor.

  `req.BaseRef` *is* passed into `evaluation.Assess` at `:277-280`, so `--base`
  is not inert — but the base-vs-head comparison is attached after scoring at
  `:314-320` and only populates `out.BaseScore`. It does not supply the anchor
  that drift reads.
  So the fixture sequence is: run `archfit baseline` to persist a baseline, then
  re-run `analyze`. `--base` is a separate base-vs-head comparison feature and is
  **not** used to establish drift comparability. An earlier revision of this plan
  asserted the opposite on panel advice and was wrong.
- Drift comparability is by fingerprint, not file existence: it requires matching
  config, module-map, label-set, and rubric fingerprints
  (`seamAnchor` → `decision.CompareFingerprints`). A baseline written under a
  different config is present but non-comparable, and drift stays `unmeasured`
  with a named cause.
- `SeamsComparable` is an internal evaluation field, not a wire field, so the
  test asserts the observable proxy: after `baseline` + re-`analyze`, drift must
  not report `unmeasured` with a non-comparable reason. The test asserts the
  reported drift status and reason, never the private field.

Impact commands:

- No production symbol is modified in this task; fixture and test only.
- `gitnexus_detect_changes(scope="all", repo="archfit")` to confirm no production
  symbol was touched.

Verification commands:

The fixture is only ever exercised through the Go test, because
`materializeFixture` owns the temp repo, the rendered config, the coverage
artifacts, and the persisted baseline. There is no standalone shell sequence to
run against the committed directory — that was the source of the parent-repo
`git -C` defect.

```sh
make build

# The test prints the verdict, per-dimension status, the recorded JSON envelope
# shape, and (on Outcome B) the blocker report with each blocker's remedy class.
go test ./cmd/archfit/ -run IntegrationReachability -count=1 -v

# Determinism and drift are asserted inside the test, against the temp repo:
#   1. analyze twice          -> byte-identical
#   2. archfit baseline       -> persists a baseline in the temp repo
#   3. analyze again          -> seamAnchor finds it; drift must not report
#                                unmeasured-with-non-comparable-reason
#   4. check                  -> exit code recorded for the dual-outcome report
go test ./cmd/archfit/ -run 'TestFormatMatrix|ByteIdentical' -count=1
```

Manual checks: confirm the fixture contains only files justified in its README.
Confirm the test creates git history itself rather than depending on the
developer's checkout. Confirm no `--format console` appears anywhere — the
supported values are `text`, `json`, `markdown`, `sarif`, `scorecard`
(`cmd/archfit/check.go`); the first draft wrongly used `console`.

Rollback: delete the fixture directory and the test file.

---

### Task 4: Implement the Task 2 audit decisions

- [ ] For each mismatch classified as "status-semantics change", implement the decision
- [ ] For each classified as "implementation fix", make the fix
- [ ] Leave the "new collector required" items for Tasks 5–13
- [ ] Keep the three hardcoded-`Partial` collectors unchanged in this task
- [ ] Add `RequiredFact`, `Promote`, and their invariant tests
- [ ] Route the six evidence-dependent dimensions through `Promote`

Justification: Task 2 has determined what needs to change and which changes are
outside the scope of static evidence and existing collectors. This task implements
the decided changes. It is deliberately split from Tasks 5–13 (new collectors)
so the owner can approve status semantics separately from collector work.

**This task owns the promotion machinery.** Fusion review found `Promote`,
`RequiredFact`, `TestPromotionIsMonotonic`, and
`TestNoMeasuredDimensionHasInClaimUnknowns` referenced in §3.1 but assigned to no
code-owning task — Task 2 is documentation-only, so an executor would have had to
invent the API mid-run. They belong here, because this is the first task that
changes status semantics and therefore the first that needs one implementation of
the rule.

Files:

- `internal/assessment/state/state.go` — `RequiredFact` (name + owner + in-claim
  flag), the fixed set per dimension, and
  `Promote(observed, notApplicable) (MeasurementStatus, []UnknownFact)`. Pure
  function, no I/O, no policy.
- `internal/assessment/state/promotion_test.go` (new) — table over all nine:
  full set → `measured`; each single in-claim removal → `partial` naming that
  fact; empty → `unmeasured`; out-of-claim unknown + full set → still `measured`.
- `internal/assessment/evaluation/dimensions.go` — route the six
  evidence-dependent dimensions through `Promote`. The three hardcoded-`Partial`
  collectors keep their literal status until Tasks 6, 7 and 10.
- Additional files per the Task 2 audit rows classified "status-semantics change"
  and "implementation fix". Do not edit symbols classified "new collector
  required".

Preconditions: Tasks 1–3 merged; audit reviewed and owner-approved.

Postconditions: every decided mismatch is implemented. The tests from Task 3 show
the new outcome.

Fitness gate:

- No symbols classified as "new collector required" are edited in this task.
- All output changes from this task trace back to a specific row in the Task 2
  audit with the "status-semantics change" or "implementation fix" label.
- `gitnexus_detect_changes` output exactly matches the audit's scope.
- `TestPromotionIsMonotonic` and `TestNoMeasuredDimensionHasInClaimUnknowns` pass.
- **Goldens are regenerated in this task, not deferred.** This task changes
  verdicts and therefore five-format output. Fusion review flagged that deferring
  golden regeneration to Task 11 would leave every intermediate commit red,
  breaking the "each task is one independently revertible green commit" rule in
  §4. Format-matrix and byte-identical baselines are refreshed here, as a
  reviewed diff called out in the commit message, and every refreshed line must
  trace to an audit row.

  **This rule is global, not local to Task 4.** Every task that changes a
  serialized status, metric, or fact — Tasks 5, 6, 7 and 10 — refreshes the
  goldens it invalidates, in its own commit, with the same trace-to-justification
  requirement. Task 11 owns renderer *presentation* changes and cross-format
  parity for the completed fact set; it is not a cleanup task for baselines other
  tasks broke.
- `.archfit-baseline.json` is refreshed here if and only if this task changes the
  self-analysis finding set; the refresh is a separate reviewed hunk.

Impact commands:

- `gitnexus_detect_changes(scope="all", repo="archfit")`
- Per symbol changed: `gitnexus_impact(target="<symbol>", direction="upstream", depth=3, include_tests=true, repo="archfit")`

Verification commands:

```sh
go test ./internal/assessment/state/ -run TestPromotion -count=1 -v
go test ./cmd/archfit/ -run IntegrationReachability -count=1 -v   # outcome may change

# Scope gate. Must fail CLOSED.
#
# Two traps this avoids, both found in review:
#   1. `git diff --name-only` alone is empty after the task commits, and never
#      lists untracked files such as the new promotion_test.go.
#   2. Setting a flag inside `... | while read` mutates a SUBSHELL, so the
#      parent still sees violations=0 and the gate silently passes. Verified:
#        violations=0; printf 'a\n' | while read -r f; do violations=1; done
#        echo $violations   # -> 0
#      Use a temp file, not a pipeline-scoped variable.

# FIRST executable step: capture the pre-task commit. This is NOT a comment.
# Use git rev-parse --git-path to handle linked worktrees where .git is a file.
set -e
GIT_DIR=$(git rev-parse --git-dir) || { echo "Failed to resolve git dir"; exit 1; }
REF_FILE="$GIT_DIR/archfit-task4-pre-ref"
git rev-parse HEAD > "$REF_FILE" || { echo "Failed to capture pre-task ref"; exit 1; }
test -s "$REF_FILE" || { echo "Pre-task ref is empty"; exit 1; }
PRE=$(cat "$REF_FILE")
test -n "$PRE" || { echo "Failed to load pre-task ref from $REF_FILE"; exit 1; }

SCOPE=docs/design/evidence-contract-audit-scope.txt
VIOL=$(mktemp)
VIOL_ALL=$(mktemp)
trap 'rm -f "$VIOL" "$VIOL_ALL"' EXIT

# Collect both tracked and untracked changes.
{ git diff --name-only "$PRE" || { echo "git diff failed"; exit 1; }; 
  git ls-files --others --exclude-standard || { echo "git ls-files failed"; exit 1; }; 
} | sort -u > "$VIOL_ALL"

# Check each file against explicit authorizations.
while IFS= read -r f; do
  # Explicit pass: test goldens, baseline, and promotion files this task creates.
  case "$f" in
    cmd/archfit/testdata/*|.archfit-baseline.json) continue ;;
    internal/assessment/state/promotion_test.go) continue ;;
    # Task 2 audit explicitly requires editing state.go to add Promote field.
    internal/assessment/state/state.go) continue ;;
  esac
  # All other files must appear in the Task 2 scope authorization.
  if ! cut -f1 "$SCOPE" 2>/dev/null | grep -qxF "$f"; then
    echo "UNAUTHORISED FILE: $f" >> "$VIOL"
  fi
done < "$VIOL_ALL"

if [ -s "$VIOL" ]; then 
  cat "$VIOL"
  echo "SCOPE GATE FAILED"
  exit 1
fi
echo "SCOPE GATE OK"

go test ./cmd/archfit/ -run 'TestFormatMatrix|ByteIdentical' -count=1
make test-fast && make lint
```

Manual checks: for each changed symbol, confirm the change was explicitly
approved in the Task 2 audit, not inferred from it.

Rollback: revert per approval row; each mismatch fix is independent.

---

### Task 5: Implement drift lifecycle policy (stage A, task 1)

- [ ] Implement the Task 1 decision for "comparison not requested"
- [ ] Keep "baseline requested but missing/incomparable" as `unmeasured`
- [ ] Cover both paths with tests

Justification: §1.1. Drift is `unmeasured` on every first run, so `HEALTHY` is
unreachable regardless of the three hardcoded collectors. Panel: this is a
product-contract decision, not a collector detail — hence decided in Task 1 and
merely executed here.

The distinction the implementation must preserve: *no comparison was requested*
is not the same as *comparison was requested but evidence is unavailable*. The
former is a scope/product decision; the latter is an evidence gap. Conflating
them is what makes today's output misleading.

Round 6 Fusion identified that the current `seamAnchor` branches
(`internal/application/analysis.go:336-354`) distinguish:
- `base.State == nil` and not legacy → no baseline persisted.
- `base.Legacy` → baseline exists but predates seam tracking.
- fingerprint mismatch → config/model/label/rubric changed.

**Task 1's policy:** Drift only measures against persisted baselines. The
distinction between "not requested" and "requested but missing" is a workflow
message ("Did you run `archfit baseline`?"), not a drift concern. No code changes
needed; Task 5 just adds test coverage for both cases.

Files:

- `internal/assessment/evaluation/dimensions.go` — no changes; `driftDimension`
  already reads `ref.SeamsComparable` from `seamAnchor` and stays `unmeasured`
  when it is false.
- `internal/assessment/evaluation/dimensions_test.go` — both paths.
- `cmd/archfit/integration_reachability_test.go` (new) — test harness defining the
  `IntegrationReachability` test suite with subtests including `drift_lifecycle`
  (tests both paths: no request, and incomparable baseline). Uses
  `materializeFixture(t, withCoverage=false)`. Verify the named subtests are
  implemented and pass.

Preconditions: Tasks 1–4 merged; drift decision recorded in the Task 2 audit and owner-approved.

Postconditions: drift measurement is clarified as baseline-driven, not
request-driven. The output, schema, and code behavior are unchanged; test
coverage expands to confirm both branches (missing baseline, incomparable
baseline) work as intended.

Fitness gate:

- Both test branches (no baseline ever persisted; baseline incomparable due to
  legacy/fingerprint mismatch) run and pass.
- Output and golden state remain byte-stable; no collection logic changes.
- `MeasurementStatus` still has exactly three values; schema version unchanged.
- The Task 3 fixture's blocking set correctly excludes `drift`.

Impact commands:

- `gitnexus_impact(target="driftDimension", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="attachBaseComparison", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
go test ./internal/assessment/evaluation/ -run 'TestDrift' -count=1 -v
go test ./internal/application/ -count=1
make build
# Runs through materializeFixture, which owns the temp repo, the rendered
# config, and the persisted baseline. There is no committed
# testdata/fixtures/reachability/.archfit.yaml to point at.
go test ./cmd/archfit/ -run 'IntegrationReachability/drift_lifecycle' -count=1 -v
make lint
```

Manual checks: confirm the two reason strings are distinguishable by a reader who
does not know the codebase.

Rollback: single commit revert; drift returns to unconditional `unmeasured`.

---

### Task 6: Give `operations` declared-topology completeness

- [ ] Surface corroborated deploy units from `deployunit.Detect`
- [ ] Surface owner provenance (`codeowners` | `git_author` | `declared`)
- [ ] Reconcile declared vs corroborated; publish both, never merge
- [ ] Promote via `Promote` under the *declared-topology* claim only

Justification: `operationsDimension` says "nothing observes what actually runs"
while archfit already parses Dockerfiles (`deployunit.go:335`), k8s manifests
(`:364`), workspaces (`:213`), `pyproject.toml` (`:267`), Go mains (`:134`), and
CODEOWNERS (`ownership.go:56-60`). Those results reach `acquire.go:96-97`, fill
the topology, and are then discarded as evidence. Surfacing them costs no new
tool, no new subprocess, and no new walk.

**Panel correction, applied.** Parsing a manifest is *not* observing runtime. A
committed Dockerfile may never be built; a Helm chart may never be applied. This
task therefore narrows the dimension's claim to **declared operational topology
completeness** — "every declared module has a corroborating manifest and a real
owner" — and Task 1 must state that claim. Runtime and supply-chain facts stay
out-of-claim and remain named unknowns. The dimension is renamed in the contract,
not in the wire format.

It also fixes a correctness gap: distance depends on owner and deploy unit
(`coupling.DistanceIsHigh`), so a git-author fallback owner silently behaves like
a real ownership boundary. Publishing provenance makes that visible.

Files:

- `internal/model/evidence/evidence.go` — `CorroboratedDeployUnit{Path, Unit,
  Source}` and `OwnerProvenance{Module, Owner, Source}`; `Source` a closed enum
  (`dockerfile`, `k8s_manifest`, `workspace`, `pyproject`, `go_main`,
  `codeowners`, `git_author`, `declared`).
- `internal/extract/acquire/acquire.go` — retain the detection and provenance
  results instead of discarding them after topology fill.
- `internal/extract/deployunit/deployunit.go` — add a sibling of `Detect`
  returning the matched source kind; existing signature preserved so
  `cmd/archfit/config_update_adapters.go:453` is untouched.
- `internal/ownership/ownership.go` — return provenance alongside the owner map.
- `internal/assessment/evaluation/assess.go` — carry both on `Observations`.
- `internal/assessment/evaluation/dimensions.go` — `operationsDimension` gains
  `corroborated_deploy_units`, `modules_with_corroborated_deploy_unit`,
  `owners_from_codeowners`, `owners_from_git_author_fallback`; in-claim required
  facts are declared topology + corroboration + owner provenance; runtime and
  SBOM facts declared out-of-claim.
- Tests in `deployunit_test.go`, `ownership_test.go`, `dimensions_test.go`.

Preconditions: Tasks 1–5 merged.

Postconditions: declared and corroborated topology are separate metric families
with provenance. A tree with a Dockerfile and CODEOWNERS reaches `measured` under
the declared-topology claim. A tree with neither stays `partial` naming which.

Fitness gate:

- Dockerfile + CODEOWNERS fixture → `measured`, `owners_from_codeowners > 0`,
  `owners_from_git_author_fallback == 0`.
- Neither → `partial`, unknown names "corroborating deploy manifest".
- Declared `deploy_unit:` with no corroborating manifest → both metrics
  published, values differ, status `partial`. A declared unit must never be
  counted as corroborated.
- No CODEOWNERS → provenance `git_author`, distance behaviour unchanged.
- The `measured` state still carries the runtime/SBOM unknown facts, marked
  out-of-claim — proving the narrowed claim is explicit, not hidden.
- Goldens invalidated by this task's output change are refreshed in this same
  commit (per the global rule in Task 4), so the commit is green on its own.

Impact commands:

- `gitnexus_impact(target="Detect", file_path="internal/extract/deployunit/deployunit.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Resolve", file_path="internal/ownership/ownership.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="operationsDimension", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
go test ./internal/extract/deployunit/ ./internal/ownership/ ./internal/extract/acquire/ -count=1
go test ./internal/assessment/evaluation/ -run 'TestOperations' -count=1 -v
make build
.bin/archfit analyze --config .archfit.yaml --format json > /tmp/t6.json
# Use the envelope shape Task 3 recorded in the fixture README (§3.4); do not
# assume a container here.
python3 - /tmp/t6.json <<'PY'
import json,sys
raw=json.load(open(sys.argv[1]))
st=raw.get("architecture_state", raw)
s=st["dimensions"]["operations"]
print(s["status"], [m["name"] for m in s["metrics"]], [u["fact"] for u in s.get("unknown",[])])
PY
make lint && make archfit
```

Manual checks: confirm declared and corroborated counts are separate JSON keys
and no code path adds one into the other. Confirm the `Source` enum is closed and
every producer sets it.

Rollback: single commit revert.

---

### Task 7: Give `complexity` an architecture-level claim

- [ ] State the architecture claim in the contract (Task 1) before coding
- [ ] Promote on module-graph complexity: dependency-chain depth and degree tail
- [ ] Publish function-size distribution as an out-of-claim diagnostic
- [ ] Promote via `Promote`

Justification: `complexityDimension` publishes file LOC only. Two native facts
are already available: module-graph shape from existing relationship facts, and
per-function extents from `SyntaxFact.StartLine`/`EndLine`
(`evidence.go:64-76`, syntax analyzer on by default at `.archfit.yaml:694`).

**Panel correction, applied.** The panel split on whether per-function complexity
belongs in an architecture verdict at all, and the majority position was that it
risks code-quality scope creep — one complicated function should not decide
architecture health. So the promoting evidence is **module-graph complexity**
(chain depth, degree tail), which is an architecture claim; function-size
distribution ships as an out-of-claim diagnostic that locates hotspots but cannot
by itself move the dimension.

Deliberate ceiling, recorded: neither metric is cognitive complexity. Accepted
because both are native, deterministic, and architecture-scoped. Upgrade trigger:
corpus evidence that graph shape ranks a seam materially differently from a
code-level metric — then reconsider `gocognit`/`lizard` (§2 Stage C).

Files:

- `internal/assessment/evaluation/complexity.go` (new) — pure helpers: graph
  depth/degree distribution over existing relationship facts; function-length
  distribution over `[]SyntaxFact`. Deterministic ordering; a fact with
  `EndLine == 0` counts as unobserved, never as length zero.
- `internal/assessment/evaluation/dimensions.go` — `complexityDimension` gains
  `max_dependency_chain`, `module_fan_in_p90`, `module_fan_out_p90`
  (in-claim) and `function_loc_p50/p90/max`, `functions_over_threshold`
  (out-of-claim diagnostics), each with denominator and provenance.
- `internal/config/config.go`, `archfit.schema.json` — optional
  `metrics.function_loc_threshold` (default 60), tail counter only, never a gate.
- `internal/assessment/evaluation/complexity_test.go` (new).

Preconditions: Tasks 1–6 merged; complexity claim recorded in the Task 1 contract.

Postconditions: `complexity` reports an architecture-level distribution and
reaches `measured` when the module graph is complete. Absent syntax facts leave
the diagnostics unobserved without blocking promotion; an incomplete module graph
does block it.

Fitness gate:

- Fixture with known graph shape → exact chain depth and p90 values, asserted
  numerically.
- Fixture with known function lengths → exact p50/p90/max.
- `EndLine == 0` excluded from the denominator, not counted as 0 LOC.
- ast-grep disabled → diagnostics absent (not 0), dimension still `measured` if
  the graph is complete — proving the in-claim/out-of-claim split works.
- Incomplete module graph → `partial` naming the graph fact.
- Two identical runs produce byte-identical values.
- Goldens invalidated by this task's output change are refreshed in this same
  commit (per the global rule in Task 4), so the commit is green on its own.

Impact commands:

- `gitnexus_impact(target="complexityDimension", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Syntax", file_path="internal/extract/astgrep/syntax.go", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
go test ./internal/assessment/evaluation/ -run 'TestComplexity' -count=1 -v
make build
.bin/archfit analyze --config .archfit.yaml --format json > /tmp/t7-a.json
.bin/archfit analyze --config .archfit.yaml --format json > /tmp/t7-b.json
diff /tmp/t7-a.json /tmp/t7-b.json && echo "DETERMINISTIC OK"
ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
make lint && make archfit
```

Manual checks: verify percentile computation on an even-length input. Grep that
`function_loc_threshold` reaches no gate or finding constructor.

Rollback: single commit revert.

---

### Task 8: Supplied-coverage ingestion contract and path normalization

- [ ] Add the `coverage:` config block (opt-in, default disabled)
- [ ] Add the normalized internal coverage fact, before any renderer work
- [ ] Implement ScanRoot-relative normalization with explicit unresolved counting
- [ ] Implement commit-SHA freshness binding per §3.2
- [ ] Cache by content hash through the existing fact-cache discipline

Justification: §3.2, §3.3. This builds the ingestion spine and the normalization
contract with **no format parsers** — parsers land in Task 8 against a contract
that is already tested. A bug here silently produces zero coverage, the worst
failure mode in this plan, so it is reviewed on its own.

Files:

- `internal/config/config.go`, `internal/configschema/**`, `archfit.schema.json`,
  `.archfit.yaml` — additive `coverage:` block per §3.2, `enabled: false`
  default, reusing the existing `GateMode`.
- `internal/model/evidence/coverage_facts.go` (new) — `CoverageFact{File,
  CoveredUnits, TotalUnits, Unit, Format, SourcePath, ToolVersion, SourceRef}`;
  `CoverageIngest{Facts, UnresolvedPaths, Freshness, Format, ToolVersion,
  Reason}` where `Freshness ∈ {matched, stale, unverified}`.
- `internal/extract/coverage/coverage.go` (new) — locate, read, normalize, count
  unresolved, resolve freshness. Parsing delegated via a `Parser` interface with
  one registered stub here.
- `internal/extract/coverage/normalize.go` (new) — the path contract.
- `internal/factcache/**` — key covers source content hash + format + parser
  version + ScanRoot + source ref. Stale/unverified/partial ingests are never
  cached, matching `docs/design/fact-cache.md`.
- `internal/extract/coverage/normalize_test.go`, `coverage_test.go` (new).

Preconditions: Tasks 1–7 merged. This task must not change any dimension status.

Postconditions: an opt-in, cached ingestion path exists that reports unresolved
paths and freshness explicitly. `testability` untouched, still `partial`. With
the default `coverage.enabled: false`, `analyze --json` is byte-identical to
Task 6 output.

Fitness gate:

- Path table covers: absolute POSIX; Go import-path-prefixed; Go local-package
  absolute (both shapes in one profile, per golang/go#40251); relative; outside
  ScanRoot; symlinked root; Windows separators in LCOV.
- An unreducible path increments `unresolved_coverage_paths` and does not vanish;
  the test asserts the counter, not just the mapped set.
- Profile SHA ≠ analyzed ref → `stale`; no SHA available → `unverified`; neither
  can promote.
- Missing configured source with `gate: fail` → the existing required-tool
  failure path; with `gate: warn` → reported, non-blocking.
- Cache: identical input → hit; changed file → miss; stale/unverified/partial →
  never written.
- `coverage.enabled: false` → byte-identical output vs Task 6.

Impact commands:

- `gitnexus_impact(target="Acquire", file_path="internal/evidence/acquisition/service.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Load", file_path="internal/config/config.go", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
go test ./internal/extract/coverage/ -count=1 -v
go test ./internal/factcache/ ./internal/config/ -count=1
ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
make build
# Format and golden tests confirm coverage-disabled behavior is stable.
# Task 3's IntegrationReachability fixture verifies end-to-end with and without
# coverage. No new two-binary harness is needed here.
make lint
```

Manual checks: read `normalize.go` and confirm no branch discards an unmapped
path. Confirm the cache key includes parser version.

Rollback: single commit revert; the config key disappears unconsumed.

---

### Task 9: Per-language coverage format parsers

- [ ] Go coverprofile parser
- [ ] LCOV parser
- [ ] coverage.py JSON parser
- [ ] llvm-cov JSON parser
- [ ] Format auto-detection with explicit ambiguity failure

Justification: four formats cover all four supported languages using files CI
already produces. Each is a pure byte→`[]CoverageFact` function behind the Task 8
`Parser` interface, independently testable and revertible. No parser spawns a
subprocess.

Mandatory internal slices — land and verify in this order, one commit each. This
is the single documented exception to the "one task, one commit" rule in §4:
five parsers in one commit would be unreviewable, and each slice is independently
revertible because parsers register independently.


- **9A** Go coverprofile (`mode:` header, `file:line.col,line.col n count`), both
  path shapes.
- **9B** LCOV (`SF:`/`DA:`/`LF:`/`LH:`, `end_of_record`).
- **9C** coverage.py JSON (`files.<path>.summary.{covered_lines,num_statements}`).
- **9D** llvm-cov JSON (`data[].files[].summary.lines.{covered,count}`).
- **9E** auto-detection by extension and magic prefix; ambiguity is a named
  error, never a guess.

Files:

- `internal/extract/coverage/parse_gocover.go`, `parse_lcov.go`,
  `parse_coveragepy.go`, `parse_llvmcov.go`, `detect.go` (new).
- `internal/extract/coverage/testdata/` (new) — one small real sample per format,
  provenance recorded in a `README.md` beside them.
- One `_test.go` per parser.

Preconditions: Task 8 merged; `Parser` and normalization stable.

Postconditions: all four formats parse to normalized facts. Malformed input
yields a named error, never a partial silent result.

Fitness gate:

- Per format: valid sample → exact expected counts, asserted numerically.
- Per format: truncated, empty, and header-only input → named error, no panic, no
  partial fact set.
- Go: a profile mixing import-path and absolute blocks maps both; `set`/`count`/
  `atomic` all parse; `count > 1` is covered once, not double-counted.
- LCOV: `LH`/`LF` disagreeing with per-line `DA` → prefer `DA` and report the
  discrepancy rather than trusting the summary.
- Auto-detect: a `.json` matching neither shape → named ambiguity error.
- Randomized-truncation/fuzz test per parser asserts no panic.

Input-safety gates (Fusion: a coverage file is attacker-influenced input in any
repository archfit does not own):

- **Containment.** A normalized path that escapes ScanRoot after `..` resolution
  is rejected and counted as unresolved, never read and never attributed.
- **Symlinks.** Path resolution does not follow symlinks out of ScanRoot; an
  escaping link is treated as unresolved.
- **Bounded reads.** Each source has a maximum byte size and a maximum fact
  count, both configurable. Exceeding either is a named failure that forces
  `partial`, not a silent truncation. Limits are deterministic counts, not wall
  clock, so results do not vary with machine speed.
- **Duplicate facts — no union arithmetic.** Fusion review established that an
  exact "union of covered units" is *not computable* from the aggregate
  `CoveredUnits`/`TotalUnits` pair in `CoverageFact`: two profiles reporting 6/10
  and 7/10 for one file may overlap anywhere between 3/10 and 10/10. The plan
  therefore does not attempt union arithmetic. The rule is:
  - same file, **same** `Unit` and same `TotalUnits` → keep the maximum
    `CoveredUnits`, increment `merged_coverage_facts`. This is a documented
    lower bound on true coverage, never an overstatement.
  - same file, **differing** `TotalUnits` → named error; the sources disagree
    about the denominator and cannot be reconciled.
  - same file, **differing** `Unit` (statements vs lines) → named error. Units
    are never summed or averaged across kinds, and a module's denominator is
    always single-unit.
  Exact union would require per-unit identities (line numbers or statement
  ranges), which the fact model deliberately does not carry. Accepted ceiling;
  upgrade trigger is a real multi-profile repository where the max-rule bound is
  too loose to be useful.
- **No fabricated zero.** Any of the above failing must produce a named unknown.
  A rejected or unreadable source can never be attributed as zero coverage.

Impact commands:

- `gitnexus_impact(target="Parse", file_path="internal/extract/coverage/coverage.go", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
go test ./internal/extract/coverage/ -count=1 -v
go test ./internal/extract/coverage/ -run Fuzz -count=1
gofmt -l internal/extract/coverage/
make lint
```

Manual checks: grep the package for `exec.`, `http.`, `os.Getenv` — there must be
none. Confirm testdata samples are small and their origin documented.

Rollback: revert per slice; parsers register independently.

---

### Task 10: Promote `testability` on genuine coverage evidence

- [ ] Attribute coverage facts to declared modules
- [ ] Publish covered/total with a real denominator and provenance
- [ ] Promote only when attribution is complete and the ref matches
- [ ] Keep the static file-split metrics alongside

Justification: this makes `testability` capable of `measured`, deliberately last
among evidence tasks so it inherits a tested ingestion path, normalization
contract, and parsers.

Panel correction, recorded in the contract: coverage proves instrumentation
points were exercised, not that assertions are meaningful. The dimension's claim
is therefore *exercised-code attribution*, not test quality; mutation resistance
and assertion strength stay out-of-claim named unknowns.

Files:

- `internal/assessment/evaluation/assess.go` — carry `CoverageIngest` on
  `Observations`.
- `internal/assessment/evaluation/dimensions.go` — `testabilityDimension` gains
  `covered_units`, `total_units`, `coverage_ratio`, `modules_with_coverage`
  (denominator = declared modules), `unresolved_coverage_paths`; provenance
  carries format + tool version + freshness. Existing `test_files`,
  `production_files`, `test_to_production_files` retained unchanged.
- `internal/assessment/evaluation/testability_test.go` (new).
- `cmd/archfit/integration_reachability_test.go` — add or extend with testability
  promotion subtest. Uses `materializeFixture(t, withCoverage=true)` and verifies
  that testability becomes `measured` with full module coverage and matching ref.
  Verify the named subtest(s) for full and partial coverage exist and pass.

Preconditions: Tasks 8–9 merged.

Postconditions: with a fresh, ref-matched, fully-mapped source, `testability`
reports `measured`. Without one it reports `partial` exactly as today. On a
repository with no coverage configured, output is byte-identical to Task 6.

Fitness gate:

- Fresh ref-matched profile covering every declared module → `measured`,
  `unresolved_coverage_paths == 0`.
- Profile covering 3 of 5 declared modules → `partial`; unknown names the two
  unmapped modules; `modules_with_coverage` denominator is 5, not 3.
- `unresolved_coverage_paths > 0` → `partial` regardless of ratio.
- `stale` or `unverified` freshness → `partial`; ratio still published, marked.
- Coverage disabled → status, metrics, and bytes identical to Task 6.
- Zero test files with coverage present → `measured` with ratio 0, not
  `unmeasured`. A tested-nothing repo is a measured fact, not missing evidence.
- Coverage ratio never influences the verdict: assert no finding or gate reads
  `coverage_ratio`. A low ratio makes the dimension `measured`, not `blocked`.
- Goldens invalidated by this task's output change are refreshed in this same
  commit (per the global rule in Task 4), so the commit is green on its own.

Impact commands:

- `gitnexus_impact(target="testabilityDimension", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Score", file_path="internal/assessment/evaluation/assess.go", direction="upstream", depth=4, include_tests=true, repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
go test ./internal/assessment/evaluation/ -run 'TestTestability' -count=1 -v
make test
make build

# 1. Disabled path: no regression when coverage is off (default config).
#    Cannot compare stashed source (mutates tree) or git show .bin/archfit
#    (ignored). Use git archive for HEAD, build both, test on same fixture.
echo "Building HEAD binary from git archive..."
TEMP_HEAD=$(mktemp -d)
trap 'rm -rf "$TEMP_HEAD"' EXIT
git archive HEAD | tar -xC "$TEMP_HEAD" || { echo "Failed to extract HEAD"; exit 1; }
(cd "$TEMP_HEAD" && make build) || { echo "Failed to build HEAD"; exit 1; }
HEAD_BIN="$TEMP_HEAD/.bin/archfit"
test -x "$HEAD_BIN" || { echo "HEAD binary not found"; exit 1; }

# Subtest verifies disabled path on the same fixture with both binaries.
go test ./cmd/archfit -run TestCoverageIngestDisabledPath -v -count=1 || exit 1

# 2. Enabled path: full coverage->testability promotion on genuine evidence.
#    Uses materializeFixture(t, withCoverage=true) to generate coverage,
#    ref, and baseline inside the test harness (no ad-hoc /tmp paths).
go test ./cmd/archfit -run TestCoverageIngestEnabledPath -v -count=1 || exit 1

make lint && make archfit
```

Manual checks: confirm the denominator is the declared module set, not the
covered set. Confirm no path lets a high ratio suppress a finding.

Rollback: single commit revert.

---

### Task 11: Renderers, five-format parity, and SARIF

- [ ] Console, Markdown, JSON, scorecard render the new metric families
- [ ] SARIF carries them in run properties
- [ ] Format-matrix baselines regenerated deliberately
- [ ] Determinism re-pinned

Justification: the state contract requires five-format parity on facts. New
metrics must appear consistently or the formats disagree about what was measured.
Renderer work is last among implementation tasks so it renders a settled fact set.

Files:

- `internal/output/console/report.go`,
  `internal/output/markdown/markdown_metrics.go`,
  `internal/output/markdown/report.go`, `internal/output/jsonout/jsonout.go`,
  `internal/output/scorecard/scorecard.go`, `internal/output/sarif/sarif.go`.
- `cmd/archfit/testdata/format-matrix/**` — regenerate baselines.
- `cmd/archfit/format_matrix_test.go` — extend parity to the new families,
  including the in-claim/out-of-claim unknown distinction.

Preconditions: Tasks 1–10 merged; fact set frozen. Task 4 already refreshed the
goldens for its own status changes; this task regenerates only for the new metric
families introduced in Tasks 6, 7 and 10.

Postconditions: all five formats report the same statuses, coverage split, and
metric families. SARIF keeps finding compatibility; state stays in run properties
per the approved decision and SARIF 2.1.0 property-bag rules.

Fitness gate:

- `TestFormatMatrix_CrossFormatParity` passes with the new families.
- `TestFormatMatrix_DoubleRunIsStable` passes.
- `TestFormatMatrix_ExitCodesUnchanged` passes.
- `TestFormatMatrix_SarifCarriesTheState` passes.
- Baseline regeneration is a reviewed diff called out in the commit message,
  never an incidental update.

Impact commands:

- `gitnexus_impact(target="Render", file_path="internal/output/console/report.go", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_impact(target="Render", file_path="internal/output/sarif/sarif.go", direction="upstream", depth=3, include_tests=true, repo="archfit")`
- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
go test ./cmd/archfit/ -run TestFormatMatrix -count=1 -v
go test ./internal/output/... -count=1
make build
for f in text json markdown sarif scorecard; do
  .bin/archfit analyze --config .archfit.yaml --format "$f" > "/tmp/fmt-$f.a"
  .bin/archfit analyze --config .archfit.yaml --format "$f" > "/tmp/fmt-$f.b"
  diff "/tmp/fmt-$f.a" "/tmp/fmt-$f.b" || echo "NONDETERMINISM IN $f"
done
make lint
```

Manual checks: review the regenerated baseline diff line by line; every change
must trace to a Task 4/5/6/9 metric. An unexplained line is a defect.

Rollback: single commit revert including baselines.

---

### Task 12: Documentation, schema, and config reference

- [ ] Document the `coverage:` block and the supplied-not-executed rationale
- [ ] Publish the evidence contract as user-facing reference
- [ ] Update dimension docs for `operations`, `complexity`, `testability`, `drift`
- [ ] Record the accepted ceilings and their upgrade triggers

Justification: the contract doc states three dimensions are "always partial in
v1" and that v1 "does not execute a target repository's test suite". The first
becomes false; the second stays true and now has an implementation. Leaving
either unstated makes shipped behaviour contradict its own reference.

Files:

- `docs/design/architecture-state-reporting.md` — replace the "always partial"
  statements; link the evidence contract; keep and sharpen the no-execution
  decision.
- `docs/design/evidence-contract.md` — promote from decision record to reference.
- `docs/guide/configuration-reference.md` — the `coverage:` block.
- `docs/guide/metrics.md` — new metric families with denominators, provenance,
  and in-claim/out-of-claim marking.
- `docs/guide/languages.md` — the concrete coverage command per language.
- `docs/guide/tooling.md` — no new binary is required; coverage tools are the
  user's existing CI tools.
- `CLAUDE.md` — invariants: promotion criterion, coverage-is-supplied,
  declared-topology claim.
- `archfit.schema.json` — regenerated, not hand-edited.

Preconditions: Tasks 1–11 merged; behaviour final.

Postconditions: docs match behaviour. A reader can determine from docs alone what
makes each dimension `measured`.

Fitness gate:

- `make schema` produces no drift.
- Every command shown in docs is executed once and exits as documented.
- `docs/guide/languages.md` names a concrete coverage command per language.
- No doc still claims a dimension is permanently partial.

Impact commands:

- `gitnexus_detect_changes(scope="all", repo="archfit")`

Verification commands:

```sh
ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
npx --yes markdownlint-cli2 'docs/**/*.md' 'CLAUDE.md' || true
grep -rn "always partial in v1" docs/ internal/ && echo "STALE CLAIM REMAINS" || echo "CLEAN"
```

Manual checks: read the `coverage:` reference as a new user and confirm it says
plainly that archfit does not run your tests.

Rollback: single commit revert.

---

### Task 13: Full corpus, reachability confirmation, and baseline gate

- [ ] If Task 3 reports Outcome A (healthy reached): flip the test to assert `healthy` and `check` exit 0
- [ ] If Task 3 reports Outcome B-temporary: Stages A/B were expected to clear it — re-run Task 3; a still-blocking dimension here is a defect in Tasks 5–10, not grounds to close
- [ ] If and only if Task 3 reports Outcome B-permanent, owner-ratified in the Task 2 audit: record the constraint and close the plan
- [ ] Extend `corpus_sweep.py` with the promotion-contract assertion (if collectors were added)
- [ ] Run the full 11-repository sweep in strict mode (if collectors were added)
- [ ] Refresh the self-baseline and dogfood gate (if status changed)
- [ ] Record residual coverage gaps honestly

Justification: unit fixtures cannot prove promotion behaves on real trees with
partial toolchains. This task confirms Task 3's reachability finding at corpus
scale. Three outcomes, matching the Task 3 classification:

- **Outcome A:** Stages A/B delivered healthy — verify it holds across the corpus
  and flip the fixture test to require it.
- **Outcome B-temporary still present:** a dimension Tasks 5–10 were supposed to
  close is still blocking. That is a **defect in those tasks**, not grounds to
  close the plan; fix and re-run.
- **Outcome B-permanent, owner-ratified:** ratify the constraint and close.

`scripts/eval/corpus_sweep.py` already asserts nine dimensions, coverage counts
summing to nine, exit-code agreement, format parity, and byte determinism.

Files:

- `cmd/archfit/integration_reachability_test.go` — flip to assert `verdict == healthy`
  and `check` exit 0.
- `scripts/eval/corpus_sweep.py` — per-repo assertion: every `measured` dimension
  carries no *in-claim* unknown; every `partial` dimension carries at least one.
- `scripts/eval/corpus_sweep_test.py` — cover the new assertion.
- `.archfit-baseline.json` — refreshed after the dimension changes.
- `docs/reports/` — sweep record including repos that remain partial and why.

Preconditions: Tasks 1–12 merged. Execution-environment gates, checked before the
sweep runs and reported as skipped rather than failed when absent: the eleven
corpus repositories are present under `~/workspace`, and a Rust toolchain
(`rustup`, `cargo-modules`) is installed for the four Rust members. Fusion review
flagged these as implicit assumptions; they are now explicit preconditions.

Postconditions: strict sweep passes across all 11 repositories. Repos that remain
`NEEDS_ATTENTION` do so for a named, correct reason. The healthy fixture
demonstrates a genuine exit-0 run, exercising the branch that is dead today.

Fitness gate:

- `python3 scripts/eval/corpus_sweep.py --strict` exits 0 across the full corpus
  (spotinfo, pumba, omni/scheduled-tasks, prometheus, ccgram, prefect, storybook,
  yazi, herdr, ruff, tokio).
- No repository reports `measured` with an in-claim unknown.
- No verdict improves without a corresponding evidence gain — for each change the
  record names which dimension became measured and from which fact.
- Known-partial cases stay explicit: yazi `cargo-modules` disabled by config,
  ruff SCIP partial, tokio `cargo-modules` partial for benches/examples/stress-
  test, Python and TS unresolved imports and SCIP timeouts. Disclosed coverage
  gaps, not silent successes.
- `bash scripts/tests/cli_exit_contract_test.sh` passes including the `HEALTHY`
  → 0 row, now reachable end-to-end.
- `make all` green; `make archfit` green on the refreshed baseline.
- No corpus repository is modified.

Impact commands:

- `gitnexus_detect_changes(scope="compare", base_ref="main", repo="archfit")`

Verification commands:

```sh
make build
go test ./cmd/archfit/ -run IntegrationReachability -count=1 -v
# The terminal healthy assertion runs inside the fixture test, which owns the
# temp repo, rendered config, coverage artifacts and persisted baseline.
go test ./cmd/archfit/ -run 'IntegrationReachability' -count=1 -v   # expect Outcome A
python3 scripts/eval/corpus_sweep_test.py
python3 scripts/eval/corpus_sweep.py --strict \
  --repeat-repos spotinfo,ccgram,storybook,yazi \
  --format-repos spotinfo,ccgram \
  --summary-file /tmp/archfit-corpus-results.json
bash scripts/tests/cli_exit_contract_test.sh
make all
.bin/archfit baseline --config .archfit.yaml
make archfit
for r in spotinfo pumba prometheus ccgram prefect storybook yazi herdr ruff tokio; do
  git -C ~/workspace/$r status --porcelain 2>/dev/null | head -1
done   # expect no output — corpus untouched
```

Manual checks: read the sweep summary and confirm every verdict change is
explained by a named evidence gain.

Rollback: revert the baseline refresh and the sweep assertion independently; the
sweep change is test-only.

---

## 6. Explicitly not included

- Archfit executing a target repository's test suite, build, or coverage command.
- Any change to `Decide`, the verdict rules, the exit table, or the four
  comparability hashes. (`Decide` is CRITICAL blast radius; §3.5.)
- A fourth `MeasurementStatus` value.
- Any repository-level scalar, band, or averaged score.
- Cognitive/cyclomatic complexity analyzers (`gocognit`, `lizard`).
- `knip`, `import-linter`, `cargo audit`, `cargo deny`, SBOM, vulnerability
  scanning.
- Compiler and lint diagnostics (`tsc --noEmit`, `pyright`, `ruff`) as
  architecture findings.
- Runtime/cluster topology observation, and any claim that a parsed manifest is
  an observation of what runs.
- Any change to Balanced Coupling scoring, `ScoreVersion`, seam-gate semantics,
  or LLM enrichment.
- New language support.

---

## 7. Research appendix

Primary sources consulted 2026-08-27.

| Fact                                                                                                                   | Source                                                                                                      | Used by            |
| ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ------------------ |
| Go coverprofile records a full file path for local packages and an import path for non-local ones; profiles can mix both | golang/go issue #40251 "cmd/cover: clarify the format of cover profile"; `cmd/cover` source                  | §3.3, Tasks 8, 9A  |
| Profile line format is `name.go:line.col,line.col numStmt count` after a leading `mode:` line                             | Go blog "The cover story" <https://go.dev/blog/cover>; `golang.org/x/tools/cover` `Profile`/`ProfileBlock`   | Task 9A            |
| Coverage modes are `set`, `count`, `atomic`                                                                              | Go blog "The cover story"                                                                                   | Task 9A gate       |
| coverage.py stores **absolute** paths by default; relative only with `[run] relative_files = True`                       | Coverage.py configuration reference <https://coverage.readthedocs.io/en/latest/config.html>                 | §3.3, Task 9C      |
| pytest-cov exposes coverage.py's JSON reporter via `--cov-report=json`                                                   | pytest-cov reporting docs <https://pytest-cov.readthedocs.io/en/latest/reporting.html>                      | §2 Stage B         |
| `cargo llvm-cov --json` calls `llvm-cov export -format=text`; `--summary-only` emits per-file summaries                  | taiki-e/cargo-llvm-cov README                                                                               | Task 9D            |
| cargo-llvm-cov is actively maintained (0.8.x–0.9.x in 2026), dual-licensed Apache-2.0 OR MIT                             | crates.io, docs.rs, Homebrew formula                                                                        | §2 Stage B         |
| Vitest supports `json-summary` and `lcov` reporters via v8 or istanbul providers                                         | Vitest coverage config <https://vitest.dev/config/coverage>                                                 | §2 Stage B         |
| SARIF property bags are the sanctioned place for tool-specific run data                                                  | OASIS SARIF v2.1.0 §3.8                                                                                     | Task 10            |
| CODEOWNERS resolves from `.github/CODEOWNERS`, `CODEOWNERS`, or `docs/CODEOWNERS`                                        | GitHub Docs "About code owners"                                                                             | Task 5 (matches `ownership.go:56-60`) |
| knip is maintained and ISC-licensed                                                                                      | webpro-nl/knip; npm registry                                                                                | §2 Stage C (deferred) |
| import-linter provides `layers` contracts for Python import direction                                                    | import-linter docs <https://import-linter.readthedocs.io>                                                   | §2 Stage C (deferred) |
| lizard computes CCN across 20+ languages with `--csv`; gocognit computes cognitive complexity for Go                     | terryyin/lizard; uudashr/gocognit                                                                           | §2 Stage C (deferred) |

Repository facts verified by direct measurement on `733b36c`, not from docs:

- The 5/3/1 dimension split and `needs_attention` verdict (§1), from a binary
  built on this worktree.
- `dim.Status = state.Partial` is unconditional at `dimensions.go:422`, `:460`,
  `:524` — full scan of every `dim.Status` assignment in the file.
- Status-assignment counts per collector proving the lenient/strict split (§1.2).
- `structure` and `change_locality` emit `measured` alongside a named
  `UnknownFact` on archfit's own repo.
- drift reports `unmeasured` with `comparison.status: not_requested` (§1.1).
- No `CoverageFile`/`SuppliedCoverage`/`lcov` symbol exists in `internal/` or
  `cmd/` — the supplied-coverage path is genuinely absent.
- `deployunit.Detect` reads Dockerfiles (`:335`) and k8s manifests (`:364`); its
  result reaches only `acquire.go:96-97` for topology fill.
- `SyntaxFact` carries `StartLine`/`EndLine` (`evidence.go:64-76`); the syntax
  analyzer is on by default (`.archfit.yaml:694`).
- `gitnexus_impact`: `Decide` CRITICAL (5 impacted, 6 processes);
  `testabilityDimension` LOW (3 impacted).
- Corpus labels available to `corpus_sweep.py`: spotinfo, pumba,
  omni/scheduled-tasks, prometheus, ccgram, prefect, storybook, yazi, herdr,
  ruff, tokio.

---

## 8. Fusion review record

Fusion run `7bb04756-4400-465a-8f45-4b1ae34d5cd7`, profile `quality`, 3 of 4
panelists completing (one timed out; coverage noted as partial).

Verdict on the first draft: **NO-GO — "the defect is real, but the proposed
framing is too narrow."** Corrections applied:

### Round 3 — readiness review of the corrected plan

Fusion run `9df8f1c4-e273-4f8d-ba91-8bdb393951ad`, 4 of 4 panelists completing.
Verdict: **GO for owner review as a draft, NO-GO for Ralphex execution**, with
eight blockers. All eight are fixed in this revision:

| Blocker                                                                                  | Fix                                                                                          |
| ---------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Stale `fixtures/healthy/`, `IntegrationHealthy`, and a `console` format loop in Task 11   | renamed throughout to `fixtures/reachability/` / `IntegrationReachability`; `console` → `text` |
| `baseline` does not establish drift comparability; `check --base` does                    | Task 3 now creates two commits and runs `--base`; §Task 3 quotes `baseline`'s own help        |
| `Promote`, `RequiredFact`, and their invariant tests owned by no code task                | assigned to Task 4, which is the first task that changes status semantics                      |
| Reachability fixture had no fresh, ref-matched coverage, so Task 10 could never be tested | fixture now ships `coverage.out` + `coverage.out.ref`, regenerated by the test setup           |
| Outcome B ambiguous: Task 3 expected it, Task 13 read it as plan closure                  | split into B-temporary (normal, does not close) and B-permanent (closes, owner-ratified only)  |
| Task 4 read `/tmp/audit-scope.txt`, which no task created                                 | Task 2 now emits committed `docs/design/evidence-contract-audit-scope.txt`                     |
| Golden regeneration deferred to Task 11 would leave Task 4's commit red                   | Task 4 refreshes its own goldens; Task 11 regenerates only for its new metric families         |
| Coverage inputs lacked containment, symlink, size, and duplicate-merge safeguards         | added as explicit input-safety gates in Task 9                                                 |

Also applied: preconditions renumbered after the Task 4 insertion; Stage 0
described as "no *production*-code changes"; the parser-slice exception to
one-task-one-commit documented in §4 and Task 9; JSON envelope shape pinned in
§3.4; Task 13 corpus environment gates made explicit.

### Round 4 — readiness review of the corrected plan

Fusion run `3e9d2b57-5e38-433b-a409-dbd174e13bd1`, 3 of 4 panelists completing.
Verdict: **NO-GO** — blockers 2, 4, 5, 7 and 8 were not fully fixed, and two
panelists dissented from the third. All are now fixed:

| Round-4 finding                                                                                       | Fix                                                                                                     |
| ------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| **Round 3's drift advice was wrong.** `seamAnchor` reads the *persisted baseline*; `--base` is attached after `Assess` and never reaches `driftDimension` | verified in `analysis.go:302/314/336` and `dimensions.go:568`; Task 3 reverted to `baseline` + re-`analyze`, with the code citation recorded so this cannot flip again |
| `git -C testdata/fixtures/reachability rev-parse HEAD~1` resolves to the **parent** repo                | fixture is now materialized into `t.TempDir()` with its own `git init`; no shell gate runs against the committed directory |
| A tracked `coverage.out.ref` cannot hold the SHA of the commit containing it                            | coverage and ref are generated at runtime inside the temp repo, never committed                          |
| Nothing enabled `coverage` in the fixture config                                                        | `.archfit.yaml` is rendered from a template with `coverage.enabled: true` by the helper                   |
| Task 10 referenced `/tmp/archfit-cov.yaml`, which no task created                                       | replaced with a subtest that runs through the fixture helper                                              |
| Task 13 still closed the plan on any Outcome B                                                          | closes only on owner-ratified **B-permanent**; B-temporary is a defect in Tasks 5–10                       |
| Task 4 scope gate printed but never failed, and `git diff --name-only` misses untracked files           | gate now fails closed, unions `git ls-files --others`, and diffs against a pre-task ref                   |
| "Union of covered units" is not computable from aggregate counts                                        | replaced with a max-rule lower bound for identical denominators, named errors for differing denominators or units, and an accepted-ceiling note |
| Goldens for Tasks 5, 6, 7, 10 were deferred to Task 11, leaving red intermediate commits                | golden refresh made a global per-task rule; added to all four fitness gates                               |
| Parser timeouts were machine-speed dependent                                                            | bounded reads are deterministic byte and fact counts, not wall clock                                      |

Unresolved and explicitly flagged for the executor rather than guessed: the exact
JSON container for `dimensions` was not verified by any panelist. Task 3 records
the observed envelope shape in its fixture README on first run, and §3.4 requires
every later task to use that recorded shape.

### Round 5 — readiness review

Fusion run `cf7312fb-8e37-4075-84c8-29a3762d5939`, 3 of 4 panelists completing.
All three confirmed the round-4 drift conclusion is correct. Verdict: **NO-GO**,
five true blockers. All fixed:

| Round-5 finding                                                                                          | Fix                                                                                                |
| ---------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| **Task 4 scope gate was still fail-open.** `violations=1` inside `... \| while read` mutates a subshell, so the parent kept `0`; `PRE` was only a comment | rewritten to use a temp file and a redirect instead of a pipeline, with `PRE` captured to `.git/archfit-task4-pre-ref` as a real first step. Bug reproduced locally before fixing |
| Tasks 5 and 13 still ran `--config testdata/fixtures/reachability/.archfit.yaml`, which the helper never commits | both routed through `materializeFixture` subtests; no gate points at the committed directory        |
| Task 5 branched drift on `comparison.status`, which belongs to the `--base` path                          | branches on the persisted-baseline lifecycle from `seamAnchor` instead, with an explicit "do not use `comparison.status`" warning naming the recoupling risk |
| Tasks 8 and 10 diffed against `/tmp/c1.json` written by Task 7 — stale or absent gives a false pass        | each task now captures its own before/after reference via `git stash` in the same command block      |
| Task 6 hardcoded `["dimensions"]`, contradicting §3.4's recorded-envelope rule                             | reads the root defensively and defers to the shape Task 3 recorded                                   |

Also applied from round 5's nice-to-haves: the drift citation corrected to
`evaluation.Score` (the anchor does **not** go to `Assess`, though `req.BaseRef`
does, at `:277-280`); Outcome A now asserts `active_blockers == 0` and
`hard_gates == pass` alongside zero unknown dimensions, per
`decision.go:48-66`; and `materializeFixture` gained a `withCoverage` flag so
Task 3 does not render a `coverage:` key that only exists from Task 8.

### Rounds 1–2 — design and first readiness review

| Panel finding                                                                     | Where applied                                                       |
| --------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| Closing three collectors is insufficient; drift and coupling have their own gates  | §1.1; Task 4 (drift lifecycle); Task 3 fixture requires no abstained edges |
| `Measured` has two contradictory definitions across the nine collectors            | §1.2; Tasks 1–2 now precede all collector work                       |
| Define the evidence contract before implementing collectors                        | Task 1 is the entry point; Task 2 makes it executable                |
| Build one end-to-end healthy fixture and prove exit 0                              | Task 3 ships it; Task 13 flips it to assert healthy                  |
| Do not execute target tests from `analyze`/`check`                                 | §3.2; §2 Stage C rejects it explicitly                               |
| Coverage freshness needs commit binding, not timestamps                            | §3.2 `require_ref`, `Freshness ∈ {matched, stale, unverified}`       |
| Parsed manifests are declarations, not observed runtime                            | Task 5 narrowed to *declared-topology completeness*; §2 Stage C      |
| Per-function complexity risks code-quality scope creep                             | Task 6 promotes on module-graph shape; function size is out-of-claim |
| Coverage proves exercise, not test quality                                         | Task 9 claim narrowed; mutation/assertion strength out-of-claim      |
| Monorepo denominators must stay per module and language                            | §3.1                                                                 |
| Preserve default behaviour with a byte-identical pre/post check                    | Tasks 2, 7, 9 fitness gates                                          |
| Resolve what zero test files means                                                 | Task 9 gate: measured with ratio 0, not unmeasured                   |

Panel disagreements left open for the owner, and this plan's position:

- **Which collector first.** No consensus. This plan sequences contract → fixture
  → drift → operations → complexity → coverage, on the grounds that drift blocks
  healthy for every first run and is the cheapest to resolve.
- **What complexity should mean.** Split. This plan takes the majority
  architecture-level position and keeps function size as a diagnostic.
- **Whether static files can make operations measured.** Split. This plan adopts
  the position that they can *only* under an explicitly narrowed
  declared-topology claim, which Task 1 must state.
