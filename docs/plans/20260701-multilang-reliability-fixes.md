# Multi-language reliability — root-cause fixes

Every bug in `reports/eval-2026-07-01-v1.1.1/00-FINDINGS.md` (A1, A2, B3-B6, plus the
still-open H2/H3 from earlier evals) was individually true but individually patched —
that's why v1.1.1 shipped two fixes and immediately surfaced two new ones. This plan
stops treating each bug as its own incident. Every finding below is traced to one of
**six structural causes** in the shared core (`internal/classify`, `internal/config`,
`internal/ownership`, `internal/extract/*`, `internal/engine`), each with file:line
evidence gathered by direct code reading (not inference from symptoms) — several
verified by rebuilding archfit and re-running it against the live repo. **No finding
in the report is left unmapped** — the ledger in the final section proves it.

Cadence, matching `20260630-v1-release-fixes.md`: **Fix group 0 (guard tests) first,
as RED tests against today's binary** — this is the actual fix for the "fix a bug,
find two more" cycle, not the code changes that follow. Then one cause per fix group,
one PR per fix group: reproduce (RED) → fix → verify (GREEN) → corpus re-run. The
corpus regression gate is `spotinfo/pumba/archfit/yazi/herdr` staying
**byte-identical** `full.json` after every fix group unless that fix group targets one of
them directly. **omni and ccgram are excluded from "must stay identical"**:

- **omni** — Fix group 2 is expected to move it, direction is back down to the v1.1.0
  numbers (1845 scored, not higher) — a restoration, not an improvement.
- **ccgram** — verified empirically (not assumed) that it's exposed to Fix group 4.1:
  ccgram's config uses Python dotted globs (`ccgram.handlers`, ...) and has
  `analyzers.clones.enabled: true`. A raw `jscpd` run against the live repo found 2
  production, cross-module-candidate clone clusters
  (`session.py`↔`window_state_store.py`, `command_catalog.py`↔`providers/pi_discovery.py`)
  that today's `classified_edges.by_strength` shows **zero** `symmetric` edges for —
  consistent with `buildClonePairSet`'s `ModuleFor(realFilePath)` silently failing on
  ccgram's dotted globs (the same bug as A2, on the clone-pairing consumer instead of
  ownership). Fix group 4.1 is expected to surface at least one `symmetric`-strength
  edge for ccgram that doesn't exist today; if `by_strength.symmetric` stays 0 after
  4.1, re-check the fix, don't assume it's inert.

If a fix group's expected-stable set surprises you (a repo outside {omni, ccgram}
moves, or one of them moves in an unexpected direction), stop and re-diagnose before
merging — an unverified "should be a no-op" claim is exactly the failure mode this
plan exists to close. `make all` + `make archfit` gate every fix group.

Status legend: `[ ]` todo · `[x]` done · `[~]` in progress · `[defer]` out of scope

## Why this keeps happening (read before touching code)

1. **Overloaded path semantics.** `config.ModuleDef.Paths` is one glob field forced
   to match two incompatible string spaces depending on the caller: graph node IDs
   (dotted for Python, crate-name for Rust) for the ~10 edge-based consumers, vs real
   repo file paths for the ~11 file-based consumers (CODEOWNERS, `public_api_*`
   rules, deploy-unit, dynamic-imports, clone-pairing, syntax-facts module backfill).
   Nothing enforces which space a caller needs; fixing one side for one language
   silently breaks the other side for that language. The exact same anti-pattern
   reappears one field over in `Config.ForExtract()`'s TS scan-root derivation
   (H2): a classification glob gets reused as if it were a filesystem path.
2. **No language tag on graph nodes.** `graph.Node{Kind, Path}` carries no language.
   `graph.Edge` already has a `Language` field (precedent exists); `Node` does not.
   Graph-mutation heuristics (`Augment*` functions) infer language from `NodeKind`
   or path shape (presence of `::`) instead of an explicit field, so a
   Rust-scoped function can silently process Go nodes.
3. **Ad-hoc, incomplete attribute inheritance.** Every `Augment*` function that
   registers a synthetic module hand-writes its own ancestor-lookup and copies only
   `Owner`. `Volatility`, `Subdomain`, `Layer`, `DeployUnit` are dropped for every
   synthetic module, in all three functions — not a typo in one place, the pattern
   itself has no shared, exhaustive implementation.
4. **Module-scalar attributes computed by "any() over the whole subtree."**
   `volatility_cascade` and deploy-unit auto-detection both let a single local
   signal (one strong edge; one buried `main.go`) taint an entire coarse
   config-declared module, with no locality or evidence-threshold safeguard. Both
   are _working exactly as coded and tested_ — the gap is in the data model
   (module-only granularity), not a logic error.
5. **No per-extractor failure isolation.** `engine.go`'s extract loop aborts the
   **entire** run on any single extractor's error. All four language extractors are
   equally exposed (Go/Python/Rust are actually more brittle than TS — they have
   _no_ partial-degrade path at all). One flaky subprocess in one language currently
   zeroes out every other language's results too.
6. **Silent-fail over surfaced diagnostics.** Timeouts, empty evidence, and
   under-configured boundaries collapse to the same plausible-looking-but-wrong
   output as genuine absence, with no signal distinguishing them: a 15s git-log
   timeout looks identical to "no git history" (H3); clone-derived strength upgrades
   report the crate's `Cargo.toml:0` with no way to say "trust me, but I can't show
   you where" (B6); an all-zero encapsulation score looks identical whether the
   module truly has no interface discipline or the config never declared
   `public:`/`internal:` globs to measure it.

## Fix group 0 — guard tests (write first, RED against today's binary)

These are the tests that end the cycle: each maps to one structural cause above and
generalizes past the single bug that surfaced it, so the _next_ language/extractor/
Augment* function added is checked automatically instead of relying on a reviewer
remembering this plan.

### Task 0.1: [ ] cross-language Augment* no-op test

Build a synthetic Go-only graph (`NodeKindPackage` nodes, no `::`, no CrateRoots) and
assert `classify.AugmentCargoCrateNodes` returns the input map unchanged. Then build a
mixed graph (Go nodes + Rust crate nodes) and assert only the Rust nodes are ever
touched by any `Augment*` function. Verify: RED today (`AugmentCargoCrateNodes`
currently registers synthetic modules for uncovered Go nodes) — reproduces A1
directly, without needing omni.

### Task 0.2: [ ] `ModuleFor` consumer-consistency property test

For a config using Python dotted globs (mirror `testdata/fixture-py/.archfit.yaml`),
build a graph where a Python edge endpoint resolves to module `M` via
`ModuleFor(dottedNodeID)`. Assert that the _same underlying file's real path_ also
resolves to `M` via whatever the file-based consumers use. Verify: RED today
(`ModuleFor(realFilePath)` fails to match the dotted glob) — reproduces A2 as a unit
test, would have caught Fix B before it shipped.

### Task 0.3: [ ] per-extractor isolation test

Fake `toolrun.Runner` so exactly one language extractor's subprocess fails (non-zero
exit) while 1-2 others succeed, in a multi-language config. Assert the run still
produces output/coverage for the extractors that succeeded, with a `coverage_gap`
for the one that failed — not a total abort. Verify: RED today (`engine.go`'s extract
loop returns the first error and discards everything) — reproduces the shared cause
behind H2 in isolation, no storybook checkout required.

### Task 0.4: [ ] ancestor-attribute-inheritance completeness test

Construct a parent `config.ModuleDef` with every field set (`Owner`, `Volatility`,
`Subdomain`, `Layer`, `DeployUnit`) and assert a synthetic module registered by each
of the three `Augment*` functions inherits **all** of them, not just `Owner`. Write
this so a future new `ModuleDef` field breaks the test by default (reflect over the
struct fields, or an explicit list a reviewer must extend) rather than silently
passing. Verify: RED today for `Volatility`/`Subdomain`/`Layer`/`DeployUnit` on all
three functions — reproduces B3 and forecloses the next attribute from repeating it.

## Fix group 1 — per-extractor failure isolation (cause 5)

Highest leverage, independent of the others: today, a Python timeout can hide a
correct Rust result. Ship this before the language-specific fixes so their own
verification runs are trustworthy.

### Task 1.1: [ ] `engine.go` extract loop degrades per-extractor, not fatally

`internal/engine/engine.go:334-371`: on one extractor's error, record a
`coverage_gap` (mirroring the existing SCIP-empty-index `StatusPartial` pattern) and
continue with the remaining extractors' facts, instead of returning the error and
discarding everything. Verify: Task 0.3 goes GREEN; corpus byte-identical (no
extractor currently fails on the 6-repo gate set, so this should be a pure no-op
there); confirm via a forced-failure integration test that a 2-language config with
one broken tool still emits the healthy language's metrics.

### Task 1.2: [ ] Go/Python/Rust extractors gain the same soft-degrade branch TS has

`internal/extract/{golang,py,rust}/*.go`: today they hard-fail unconditionally on
their core subprocess's non-zero exit, with no `ModeOn`/`ModeOff`-style distinction
at all (TS is currently the _only_ one of the four with any partial path). Bring them
to parity with `ts.go`'s existing `ModeOn` check so a real user opt-in
(`enabled: true`) doesn't mean "crash the whole run if this tool hiccups." Verify:
same forced-failure test as 1.1, repeated for each of the three extractors.

## Fix group 2 — language-scoped graph mutation (cause 2, closes A1)

### Task 2.1: [ ] `graph.Node` gains a `Language` field

Mirror the existing `graph.Edge.Language` field. Every extractor (`golang.go`,
`rust.go`, `py.go`, `ts.go`) stamps it on every emitted node — mechanical, one line
per `emitNode` call site. Verify: `make test` unaffected (additive field); golden
output unchanged (field not yet consumed).

### Task 2.2: [ ] `AugmentCargoCrateNodes` gates on `Language == graph.LangRust`, not `NodeKind`

`internal/classify/classify.go:233-260`: replace the `NodeKind != Package` heuristic
with an explicit language check. Verify: Task 0.1 goes GREEN. Corpus: omni
`scored` reverts from 2277 back to **1845** (the v1.1.0 number — a restoration, not
a new value) and `diff_owner` reverts from 1591 to 1293; tokio's small +5 leak also
reverts; spotinfo/pumba/archfit/ccgram/yazi/herdr byte-identical; ruff/prefect
unaffected (this fix group doesn't touch the Rust-legitimate binding path). Re-run the
full 12-repo matrix and diff against `reports/eval-2026-07-01-v1.1.1/` — the only
expected deltas are omni and tokio, both moving toward the pre-A1 numbers.

### Task 2.3: [defer] apply the same explicit-language-gate audit to `AugmentModulesFromGraph` and `AugmentGoWorkspaceModules`

Both currently infer language from separator/gate heuristics rather than
`Node.Language` too (`::`-presence, `GoModules()` non-empty). They haven't been
caught leaking cross-language in an eval yet, but Task 2.1 makes the explicit check
free — do it for consistency once 2.1/2.2 are proven, same PR discipline (own
fix group, not bundled). Deferred out of this plan's critical path since no observed
bug motivates it yet; tracked so it isn't forgotten.

## Fix group 3 — attribute inheritance (cause 3, closes B3)

### Task 3.1: [ ] centralize synthetic-module attribute inheritance

Replace the three separate `ancestorOwner`/`ancestorOwnerByPath` call sites with one
shared helper (`inheritFromAncestor(childKey string, byPath bool, modules map[string]config.ModuleDef) config.ModuleDef`)
used by all three `Augment*` functions, copying every field Task 0.4 checks for, not
just `Owner`. Verify: Task 0.4 goes GREEN for all three functions.

### Task 3.2: [ ] re-run tokio and confirm the flood shrinks

tokio's ~424 synthetic `tokio::<mod>` submodules should inherit the parent crate's
declared `Volatility` instead of falling to
`VolatilityUndeclared`→worst-case-`V=10`. Verify: tokio's finding count and
critical/medium ratio drop measurably from the 917/909-undeclared baseline recorded
in `reports/eval-2026-07-01-v1.1.1/00-FINDINGS.md`; `cheapest_move=declare_volatility`
frequency drops correspondingly. This requires the config to actually declare
volatility on tokio's top-level crate modules — if `reports/eval-2026-06-30-corpus/configs/tokio.yaml`
doesn't yet declare it per-crate, add it as part of this task's verification (config
authoring, not a code change) so the fix has something to inherit from.

## Fix group 4 — `ModuleFor` path-space unification (cause 1, closes A2 + the H2 sibling)

Two-phase per advisor review — do not conflate them into one PR.

### Task 4.1 (Phase 1): [ ] wire the already-built `FileToModuleKey` bridge into the ~11 file-path `ModuleFor` consumers

`internal/model/graph/convention.go`'s `NodeConvention.FileToModuleKey` already
implements exactly the file-path→node-key conversion needed (dotted for Python,
crate-name for Rust) — it has **zero callers today**, confirmed by grep. Since `lang`
is not available as a parameter at 8 of the 11 file-based call sites
(`ownership.go`, `rules_api.go`, `deployunit.go`, `assemble.go`, `engine.go:130`,
`labels.go:108`), add `ModuleMap.ModuleForFile(file string) (string, bool)`: derive
the language from the file's extension via a new reverse index built from
`graph.BuiltinConventions[*].FileExtensions`, call `FileToModuleKey`, then delegate
to the existing `ModuleFor`. Migrate all 11 file-based call sites to it, leaving the
~10 edge-based call sites (`pathFromID`/`graph.NodePath`/`stripPrefix` derived) on
plain `ModuleFor` unchanged. Verify: Task 0.2 goes GREEN; corpus: prefect's
`owner_source` reverts from `none` back to `codeowners`; `public_api_*` rules and
clone-pairing become live for Python again; spotinfo/pumba/archfit/yazi/herdr
byte-identical (they're Go/Rust configs, where node-ID already coincides with
file-path form, so this is genuinely a no-op there). **ccgram is NOT in the
byte-identical set** — see the corpus-regression-gate section above for the verified
(not assumed) reason: expect `classified_edges.by_strength.symmetric` to move off 0.

### Task 4.2 (Phase 2): [defer] accept file-path globs uniformly across all languages; normalize to node-keys at config-load time

The real fix per the project's own "prefer structural over discipline" principle:
stop requiring architects to know that Python configs need dotted globs at all.
`Config.Load` would normalize every module's `Paths` once, per-language, into
whatever internal key-space each consumer needs — so `src/prefect/states.py` (the
intuitive, currently-broken form) works identically to today's mandated
`prefect.states`. This removes the exact footgun that caused the original prefect
config miss (H1) in the first place, and is the only fix that closes the "config
convention itself is a trap" problem, not just the code inconsistency downstream of
it. Deferred: larger blast radius (config-loading semantics, possible migration for
existing configs including `testdata/fixture-py`), needs its own design pass and
explicit user sign-off before starting — Task 4.1 alone closes the correctness bug
without this scope.

### Task 4.3: [ ] apply the same `Src`-derivation fix Task-4.1 revealed a sibling of (H2)

`internal/config/views.go:82-99` `ForExtract()` derives the TS extractor's scan-root
(`ExtractConfig.Src`) from the _first alphabetically-sorted module's_ `Paths[0]` — a
classification glob, not a filesystem path. Stop reading it from `Modules` entirely;
default to `.`/`src` unless `languages.typescript.src` is explicitly configured.
Verify: storybook's `full-json`/`full-md` steps stop crashing (exit 0, real output)
combined with Fix group 1's per-extractor isolation as a second line of defense; add a
regression test asserting `ForExtract().Src` is never derived from module glob order.

## Fix group 5 — module-scalar contamination (cause 4, B4 + B5) — needs a design decision, not just a fix

Both are _working as documented and tested_ today — changing them changes scored
output for every repo using `volatility_cascade` or deploy-unit detection, which is
a model/behavior change, not a bug fix. Do not implement silently; get explicit
sign-off on the target semantics before writing code, same bar as the BC book-formula
adoption itself.

### Task 5.1: [ ] decide: cascade keyed by module (current) vs. leaf-node with module fallback, and/or add a minimum strong-edge threshold

`internal/classify/classify.go:698-734`. Present both options with the prometheus
counterfactual (identical S/D, `V=high`→critical vs `V=low`→low, a 4-band swing) as
the motivating case. `volatility_cascade_test.go:66-186` encodes today's behavior as
intended — update it deliberately, not as a side effect.

### Task 5.2: [ ] decide: deploy-unit stamping — module-root-only main.go vs. current subtree-any-main.go

`internal/extract/deployunit/deployunit.go:134-165`, `KeyByModule` (lines 58-79).
Proposed minimal fix (root-only) is in the report; confirm it doesn't regress any
repo currently relying on subtree detection (audit `omni`/`prometheus`/any config
using `deploy_unit` inference before changing).

### Task 5.3: [ ] implement whichever 5.1/5.2 decisions were made; re-run prometheus

Verify: prometheus's cascade-driven critical-bucket contamination (~64% per the
eval) and the `cmd_promtool -> promql_promqltest` false-positive both resolve;
confirm via the same live-repo re-run method the investigating agent used
(`.bin/archfit analyze --full --root ~/workspace/prometheus ...`).

## Fix group 6 — diagnostics over silent-fail (cause 6)

Smaller, independent tasks — no shared code, group only because they're the same
principle.

### Task 6.1: [ ] bound the git-history owner-resolution window; distinguish timeout from empty

`internal/ownership/ownership.go:298-356` `resolveFromGitAuthor` runs unbounded
`git log --format=%ae --name-only` (no `--since`/`-n`) with a fixed 15s timeout
(`gitTimeout`, line 37). Empirically timed at 14.3s on tokio's full history in
isolation — margin-zero even without concurrent load. Bound by **commit count**
(`-n <maxCommits>`, e.g. a few thousand), not calendar time — a `--since=12-24
months` window would reintroduce the exact silent-none this task is closing: a
stable, low-churn module with no commits inside the window would resolve to _no
owner_, trading "large repos time out" for "stable modules lose ownership." If the
bounded log comes back with zero data (pathological case), fall back to an unbounded
attempt rather than silently accepting emptiness. Make a timeout surface as its own
`coverage_gap` reason distinct from "ran clean, found nothing."
Verify: tokio's `owner_source` moves off `none` (H3 closes); a forced-timeout test
(fake runner with an injected delay) confirms the new coverage_gap message appears
instead of silent `SourceNone`.

### Task 6.2: [ ] carry clone-instance evidence through to findings (B6)

`internal/extract/clones/clones.go:172-195`, `internal/model/clone/clone.go:24-45`,
`internal/engine/labels.go:98-120`. Capture jscpd's per-side line locations in
`jscpdFile`/`clone.Cluster` instead of discarding them at parse time; stop
collapsing straight to a boolean `CrossModuleClonePairs`; append the real clone
location to the finding (extra `Locations` entry or `MatchedBy["clone_evidence"]`)
instead of leaving only the edge's baseline `Cargo.toml:0`. Verify: re-run ruff,
confirm at least one finding's `locations` cites a real source file:line pair
instead of `Cargo.toml:0`.

### Task 6.3: [ ] encapsulation confidence/label review for zero-basis measurements

`prefect`'s `encapsulation: 0.0, critical` (0 contract / 126 intrusive, arithmetically
correct but likely because no `public:`/`internal:` globs are declared) reads as "no
interface discipline" when it means "unmeasured." Same family as 6.1/6.2: a
technically-correct number standing in for "no real signal here." Minimal fix:
when the contract+intrusive denominator is entirely driven by an under-configured
module (no `public:`/`internal:` declared anywhere), keep confidence `low` (already
correct) but consider suppressing the numeric band in favor of the existing `n/a`
convention this codebase already uses elsewhere for "measured nothing." Needs the
same design-sign-off bar as Fix group 5 — a scoring-contract change, not a pure bug
fix.

## Fix group 7 — smaller, already-scoped items

### Task 7.1: [defer] `agent_tasks` for BC advisories (M1)

Confirmed unchanged across three consecutive evals: `agent_tasks[]` only populates
on gate-blocking rule violations, never on advisory-only `bc/imbalanced_coupling`
findings. This is a scoped feature gap (BC advisories currently only produce prose),
not a reliability bug — deferred out of this plan; track separately if the product
decision is to make BC advisories agent-actionable.

## Findings ledger — confirms nothing from the report is unaddressed

| Finding                                           | Structural cause | Fix group                 | Notes                                                                                                                                                                                                                                                                                                                                     |
| ------------------------------------------------- | ---------------- | ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A1 (AugmentCargoCrateNodes leaks into Go)         | 2                | Fix group 2               | Guard test 0.1                                                                                                                                                                                                                                                                                                                            |
| A2 (Fix B breaks file-path `ModuleFor` consumers) | 1                | Fix group 4.1             | Guard test 0.2. Restores `owner_source`, `public_api_*`, clone-pairing. Does **not** fully resolve prefect's triageability cliff — at S=functional/D=same-owner the balance is formula-pinned at 5 regardless of volatility (book formula, not a bug), so owner-distance only rediscriminates the subset of edges that become cross-owner |
| B3 (Owner-only inheritance)                       | 3                | Fix group 3               | Guard test 0.4                                                                                                                                                                                                                                                                                                                            |
| B4 (volatility_cascade granularity)               | 4                | Fix group 5.1/5.3         | Design decision required                                                                                                                                                                                                                                                                                                                  |
| B5 (deploy-unit false positive)                   | 4                | Fix group 5.2/5.3         | Design decision required                                                                                                                                                                                                                                                                                                                  |
| B6 (clone evidence not exported)                  | 6                | Fix group 6.2             | Data-model gap                                                                                                                                                                                                                                                                                                                            |
| H2 (storybook TS18003 crash)                      | 1 + 5            | Fix group 4.3 + Fix group 1 | Two independent causes, both fixed                                                                                                                                                                                                                                                                                                        |
| H3 (tokio owner_source=none)                      | 6                | Fix group 6.1             | Empirically timed at 14.3s/15s margin                                                                                                                                                                                                                                                                                                     |
| M1 (agent_tasks empty on advisories)              | —                | Fix group 7.1             | Scoped feature gap, deferred, not a reliability bug                                                                                                                                                                                                                                                                                       |
| encapsulation critical-mislabel (prefect)         | 6                | Fix group 6.3             | Design decision required                                                                                                                                                                                                                                                                                                                  |

## Sequencing recommendation

Fix group 0 → 1 → 2 → 3 → 4.1 → 4.3 → 6.1 → 6.2, each its own PR, each re-running the
full 12-repo matrix and diffing against `reports/eval-2026-07-01-v1.1.1/` before
merging the next. Fix groups 4.2, 5, 6.3, 7.1 need an explicit product decision before
implementation — surface them to the user as a batch once Fix group 0-4.3/6 land and
the corpus is stable, rather than deciding unilaterally mid-plan.

## Optional Fix group 8 — make repeatability structural, not a habit (scope extension, needs explicit sign-off)

Fix group 0's guard tests kill the six _known_ causes above. They do not change how the
_next_ regression gets found — that's still a human running the 12-repo matrix and
reading the output, which is the actual mechanism behind "fix a bug, find two more":
discovery, not just fixing, has been manual every time. The highest-leverage
structural change left is turning a small pinned subset of the corpus into an
automated CI gate: 3-4 repos (one per language — e.g. archfit/Go, ccgram/Python,
herdr/Rust, one small TS fixture once Fix group 4.3 makes TS not crash) with committed
expected `full.json` snapshots, diffed on every commit. "Did we break language X" then
gets answered by CI, not by remembering to run an eval. This is explicitly a scope
extension beyond the bug list — flagging it because it's what "более повторяемой"
(more repeatable) actually requires, not because it's implied by the six fixes above.
