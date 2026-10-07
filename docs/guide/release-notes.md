# Release notes

## v2.5.0 — (unreleased)

New:

- Module allowlists. `modules.<m>.depends_on` lists the only modules a module
  can import; `modules.<m>.visible_to` lists the only modules that can import
  it. An absent key means no constraint; an empty list allows nothing. One new
  rule type, `module_dependencies`, enforces both lists. An importer that no
  declared module owns is denied by every `visible_to` list. Findings are keyed
  by the module pair, so a new or moved file on the pair keeps the finding ID;
  `matched_by.violates` names the list that denies the pair. Repair tasks never
  route the dependency through the target's public API. See
  [Module allowlists](configuration-reference.md#module-allowlists).
- `archfit config lint` reports `unknown_module` (error) for an allowlist entry
  that names no declared module, and `dead_selector` for a
  `module_dependencies` rule when no module declares a list. `check` prints the
  unknown entry as a config warning.
- Module selectors. `forbidden_dependency` takes `from_module` and `to_module`
  instead of `from` and `to`: `layer:<name>`, `role:<role>`, or a glob over
  module names. A module selector matches only edges between two different
  modules. A module side keys its findings by the module, so a moved file
  keeps the finding ID. Allowlist entries (`depends_on`, `visible_to`) accept the
  same selectors. A selector that selects no module makes the rule not
  evaluated (`selector matches nothing: from_module …`), and `config lint`
  reports it as `unknown_module`. See
  [Module selectors](configuration-reference.md#module-selectors).
- Rule rationale. Every rule type takes `rationale`, `alternatives`, and
  `docs`. The rationale ends the finding `why` (and so the SARIF message) and
  appears in the repair task's constraints; the alternatives become the
  finding's `allowed_alternatives`, the task's constraints, and the SARIF
  result property `allowed_alternatives`; the docs reference ends the finding
  `why` and `constraint`. None of them changes a finding ID.
- `--format agent` on `check` and `analyze` writes `archfit.agent-result.v1`:
  the verdict, one `next_action` (`repair`, `ask_owner`, `restore_evidence`,
  `report_blocked`, or `none`), and the active gate tasks grouped by edge into
  repairs, in at most 8 KB. With `--base`, a repair whose tasks are all
  `pre_existing` is outside the scope. A metric ratchet block names the
  worsened metrics. The exit code stays the verdict. See
  [The agent result](agent-feedback.md#the-agent-result---format-agent).
- `archfit policy where <path>...` and `archfit policy can-import <from>
  <target>...` answer before an edit, in about 50 ms. They read the config,
  the waivers, and the baseline, and run no analyzer. `where` names the module
  that owns a path, its layer, role, owner, public surface, and allowlists,
  whether `check` reads the path, and the rules that select it. `can-import`
  answers `denied`, `not_decided`, `allowed`, or `unconstrained` for each
  target, with the same relationship analysis, rule pass, and finding IDs as
  `check`. It exits `1` when a target is denied and `2` when a target is not
  decided. Rust imports are not decided: crate roots need `cargo metadata`.
  See [`archfit policy can-import`](commands.md#archfit-policy-can-import).
- Agent hooks. `archfit hook claude` is a Claude Code `Stop`/`SubagentStop`
  hook: on a dirty tree it runs the agent result against `--base HEAD` and
  exits 2 with the repair on stderr, at most once per stop; it fails open on an
  archfit error. `archfit hook git` is a pre-commit hook (exit 1 on a repair or
  an owner decision), published in `.pre-commit-hooks.yaml` as id `archfit`.
- `archfit agents-md [--write] [--check]` renders one generated block of agent
  instructions (loop, module table, rules that block) into `AGENTS.md`;
  `--check` exits 1 on drift.
- `archfit skill install` installs the archfit agent skill embedded in the
  binary. The skill is rewritten around three steps: ask (`policy can-import`),
  check (`--format agent`), and hooks.
- Map completeness. `map/uncovered_path` now reports each directory that holds
  production source no declared module owns, and `module_review.gate: fail`
  makes it a blocker: a new package outside every module fails `check`. The
  check reads the walked source, so test, generated, vendor, excluded, and
  unanalysed files never count, and a package that failed to load is still
  checked. One finding per directory, with at most five file locations.
  `matched_by.suggested_path` gives a `paths:` glob that owns the directory.
  The repair task asks the owner (`needs_owner_decision`). `map/dead_rule` and `map/stale_review`
  stay diagnostics. See [`module_review`](configuration-reference.md#module_review).

Contract notes:

- Config schema v2 gains the optional module keys `depends_on` and
  `visible_to`, the rule type `module_dependencies`, and the optional rule
  keys `from_module`, `to_module`, `rationale`, `alternatives`, and `docs`.
  `archfit.schema.json` is regenerated. Engines up to v2.4.1 reject the new
  keys: pin the engine before you use them.
  The archfit App pins the engine's config schema and must re-pin it for this
  release.
- `model_hash` does not include the allowlists. An allowlist edit changes
  `config_hash`, so the stored baseline becomes non-comparable until you run
  `archfit baseline` again.
- New answer document `archfit.policy-answer.v1` for `archfit policy`. It
  has no published schema yet; the App does not read it.
- New published schema `archfit.agent-result.schema.json` for
  `archfit.agent-result.v1`. The architecture state, `archfit.state.schema.json`,
  baseline v2, and the comparison fingerprints do not change.
- `map/uncovered_path` changes its subject from a graph node to a directory.
  A Go package keeps its finding ID. A finding about a single file gets a new
  ID: the old one reads `fixed`, and the directory finding is new. Before
  v2.5.0, `module_review.gate: fail` blocked nothing.
- Configs that do not use the new keys or `module_review` give byte-identical output.

Fixed:

- The Go extractor dropped the imports of every Go file that imports `"C"` when
  cgo preprocessing succeeded. The load parses a preprocessed copy in the build
  cache, outside the scan root, and the extractor skipped it. Edges, rule
  violations, cycles and coupling facts from cgo files were missing. The
  extractor now maps the copy back to the original file and reads the imports
  the author wrote (including `import "C"`, as before for a failed preprocess),
  not the rewrite cmd/cgo makes. Edge IDs, locations and strength hints of a
  cgo file are built like those of a plain file, and do not depend on whether
  the host has a working C toolchain.
  `archfit policy can-import` reads a cgo file when `CGO_ENABLED=1`. A failed
  cgo preprocess still makes the `go/packages` row `partial`, and a cgo file
  that cgo-off ignores is still counted in the build-constraint disclosure.
- Cached Go facts from older binaries are not reused (the cache key has a new
  facts revision). The measurement profile and the settings hash do not change, so
  a baseline stored by an older binary still compares as comparable. On a
  repository with cgo files, the new edges read as introduced against it: run
  `archfit baseline` again after reviewing them. Repositories without cgo files
  give byte-identical output.

## v2.4.1 — schema lists rule types

The published config schema (`archfit.schema.json`) now enumerates the allowed
`rules[].type` values, so editors and other JSON Schema validators flag an
unknown type such as `forbiden_dependency` while you type. The engine behaves
as before: it already rejected unknown rule types at load.

## v2.4.0 — guardrails that fire

This release fixes guardrails that silently did not fire, adds the rules an
architect needs to state module boundaries, and stops the engine from
misdirecting agents. The architecture-state JSON shape, baseline schema v2 and
the comparison fingerprints are unchanged. Verdicts can change on upgrade:
read the "Upgrade effects" list before enabling it in CI.

New:

- `module_cycle` rule: dependency cycles between declared modules, in every
  language. One finding per ordered module pair inside a cycle; its repair task
  names the cycle and asks to remove one direction. The node-level `cycle` rule
  is unchanged and is always zero on compiling Go (edges run file -> package).
- `forbidden_pattern` rule: the only consumer of `rules[].patterns`. It fires on
  ast-grep matches in production files under `from:`. Patterns on any other
  rule type load with a warning; they never produced findings.
- `archfit config lint` (`archfit.config-lint.v1`): dead selectors, unknown
  volatility/subdomain values, undeclared layers, public surfaces outside their
  module or matching nothing, and equal-specificity ownership ties. Exit 0 clean,
  1 on errors, 3 when the config cannot be read.
- Rule key `guard: true` marks a rule that is meant to match nothing (a guard
  against a removed package returning).
- `config init` emits failable starter rules: `no-module-cycles`, and
  `no-layer-back-edges` when it inferred two or more layers. Each is
  `gate: fail` when the init-time graph is complete and clean, otherwise
  `gate: warn` with the reason. Init writes `gate: fail` only when it can prove
  the rule is evaluable over a complete import graph; it writes `warn` with a
  `# Why:` comment when Python or TypeScript modules exist, when Rust is present,
  or when a Go module also owns source in another language (TypeScript under
  `web/ui/`). Without inferred layers it writes a commented `layers:` how-to
  instead of guessing layers from directory names, including from Python
  sub-package names. Rust workspace tiers still come from the crate dependency
  graph, without dev-dependencies.
- Releases attach `image-identity.json` with the per-platform image digests.

Fixed:

- `public_api_only` and `internal_api_access` decide from the declared
  `public:` and `internal:` globs in every language. A Go import that bypasses a
  declared surface now fires when `go.mod` sits at the scan root, and imports
  through a declared `public:` surface no longer fire when `go.mod` sits in a
  subdirectory. Existing finding IDs are unchanged.
- A gated rule whose `from:`/`to:` selector matches nothing is never counted as
  evaluated. A fail-gated one is listed in `decision.unevaluated_required_rules`
  with the reason `selector matches nothing: <from|to> <glob>`; a warn-gated one
  is a config warning.
- Rule scope honors `exclude:` and languages switched off, so a stray helper
  script no longer keeps Go rules unevaluated waiting for dependency-cruiser or
  grimp.
- Agent repair tasks: forbidden-dependency, layer, cycle, module-cycle and
  new-cross-module tasks no longer list the target's public surface as a route
  (going through it keeps the violation); seam-gate tasks name the files of the
  qualifying edges.
- A tripped metric ratchet is named in text and Markdown output with its
  baseline and current values, also beside other blockers for each dimension
  whose failing gate only a ratchet explains.
- `forbidden_pattern` never fires in files outside the declared scope
  (`exclude:` globs, languages switched off).
- A `to:` selector that names a Go standard-library package (`database/sql`) is
  an external ban, even when a top-level directory shares its first segment.
- Dead-selector checks read each dependency-rule selector the way the rule
  matches graph edges in its language. A Go file glob in `to:`, a bare Go
  package directory in `from:`, and a slash path over Python or Rust source can
  never match, and are now reported dead. A Go import-path domain such as
  `go.uber.org/**` is no longer reported dead beside a top-level `go/`
  directory. Rust `crate::mod` selectors and module paths are judged against
  the cargo-modules module graph: dead ones report `selector matches nothing`,
  `guard: true` works on them, and a rule on a `crate::mod` is no longer both
  fired and listed in `unevaluated_required_rules`. Under a loaded crate the
  graph does not cover, the selector stays undecided. A loaded crate is named
  by every target cargo metadata lists (`yazi` for package `yazi-fm`, and the
  binary `tool_cli` beside library `tool`), and a crate cargo-modules
  graphed with no submodule has an empty module graph, not a missing one.
- `config lint` judges `public_outside_module` by the packages a `public:`
  entry matches, so brace and class globs (`internal/{a,b}/**`) no longer give a
  false error.
- `forbidden_pattern` reports every distinct match on a line; a second match
  on a line that already had one is no longer dropped.
- Rule scope follows each extractor's own applicability. TypeScript with no
  root `package.json` (for example `web/ui/` in a Go repository), Go files with
  no `go.mod`, and Rust files outside every cargo workspace member no longer
  hold dependency or module rules unevaluated waiting for a producer that
  cannot run. An explicit `languages.<id>.gate` keeps them in scope. A selector
  that matches only such source is reported as `selector matches only source no
  dependency producer analyses: <side> <glob>`.
- `module_cycle` counts only production edges. An import from a test,
  generated or vendored file, or from a file declared out of scope, no longer
  closes a module cycle. Files the source walk skips (such as `.storybook/`) are
  classified from their path, so `file_class.test_globs` applies to them.
  Finding IDs are unchanged.
- TypeScript analysis drops `node_modules` at any depth, including a workspace
  package's nested `node_modules`, in root and subtree mode. Third-party files
  no longer become first-party nodes, `module_cycle` locations or agent-task
  files.
- A `guard: true` rule is no longer listed in `unevaluated_required_rules`
  while its `to:` matches nothing, even when producer evidence is partial
  (normal on TypeScript).
- Rust: `public_api_only` and `internal_api_access` no longer fire on edges
  between two modules of one crate when an `internal:` glob names
  cargo-modules nodes (`yazi_shared::url::**`) of a crate declared by package
  name. Such nodes belong to the module that declares their crate, so its
  `public:` globs exempt them; `new_cross_module_dependency` and
  `forbidden_layer_direction` resolve them the same way. On yazi this removes
  63 false blocking findings and keeps the real cross-crate one.
- Rust: crate dependency findings point at the member's own `Cargo.toml` and
  the line that declares the dependency (for example `yazi-adapter/Cargo.toml:22`),
  not the root `Cargo.toml:0`. A declaration the scan cannot place keeps line 0;
  a member outside the analysed root has no location. Finding IDs are unchanged.
- Rust agent tasks list files for crate-level and `crate::mod` nodes, including
  `module_cycle` tasks. Before, the package name (`yazi-shared`) and the crate
  name (`yazi_shared`) did not match and these tasks had `files: []`.
- Rust test files are classified Test: `tests.rs`, `*_test.rs`, `*_tests.rs`,
  and files under `tests/`, `benches/`, `*_tests/` or `*-tests/`, so
  `forbidden_pattern` no longer fires there (ruff: 42 of 113 findings removed).
  An inline `#[cfg(test)]` block inside a production file is still production.
- `config init` and `config update` no longer propose modules for trees `check`
  cannot see: `mocks/`, packages with only generated or only test files, and
  default-excluded trees such as `reports/` and `testdata/`. On pumba the
  generated config passes `config lint` and `check` evaluates the
  `no-module-cycles` starter rule. A dropped package still owned by a parent
  module's glob (`api/v1` under `api/**`) keeps its incoming edges under that
  parent, so the starter gate counts the cycles `check` sees. Go and TypeScript discovery over one
  directory no longer produce two modules that tie in ownership
  (`ambiguous_ownership`); the Go module keeps the directory.
- `config update` checks every Python package and module in a discovered
  package against the configured map, not only the package root.
- `cycle` and `module_cycle` findings keep `why` and the repair goal bounded on
  large cycles; the full member list stays in `matched_by.cycle_modules`.
  `module_cycle` reports at most 200 module pairs per cycle, the first in
  (from, to) order; `matched_by.cycle_pairs_total` gives the full count and a
  capped cycle's `why` says how many pairs are reported. Reported pairs keep
  their finding IDs. Module-cycle agent tasks no longer carry `declarations`
  (on a 19-module cycle the report drops from 1.29 MB to 0.41 MB); locations
  still name the import lines.
- A state report could not be decoded when an analyzer exited non-zero with
  multi-line output, for example dependency-cruiser on a Svelte 5 top-level
  `await`. Every free-text field is now one line, with line breaks, tabs and
  colour codes collapsed: coverage reasons, unevaluated-rule reasons, `why`,
  `constraint`, allowed alternatives, dimension text, and agent task goal and
  constraints. Text over 400 characters (3600 for task text) is cut with `…`,
  keeping the tool name, exit code and first error line. The full analyzer
  output is printed on stderr. Text that was already valid is unchanged.
- `config lint` prints ownership ties in a stable order.
- `model_hash` and Go deploy units no longer depend on how the shell spells the
  repository path (a `/tmp` symlink or an APFS case variant).
- Go files excluded by build constraints (`_windows.go`, tag-gated files) are
  counted in the go/packages coverage reason and one stderr warning.
- `config init`/`update` detect languages the way analysis does: a `go.work`
  monorepo without a root `go.mod` gets Go enabled and one module per member.
  `config update` no longer proposes catch-all modules over code the configured
  map already owns.

Upgrade effects:

- Repositories with declared `internal:` globs can get new `public_api_only` /
  `internal_api_access` findings that were previously missed.
- Configs whose `no_cycles` rule moves to `module_cycle` can get new findings.
- Rules with selectors that match nothing move from "evaluated" to
  unevaluated, so `check` can move from exit 0 to exit 2. Fix the selector, or
  mark an intentional guard rule `guard: true`. archfit-app reports these as
  `policy_defect`. This includes dependency rules with a bare Go package
  directory in `from:` (write `dir/**`), a Go file glob in `to:` (write the
  package directory), or a slash path over Python or Rust source.
- A pattern that ast-grep rejects now marks the `ast-grep` coverage row
  partial instead of reading as "no match".
- The fact cache schema is `3`; the first run after upgrading is cold.
- `check` and `analyze` read the Go standard-library list (`go list std`)
  from the fact cache (`.archfit-cache/facts/go-std`), keyed on the toolchain
  that `go env` reports in the repository, so warm runs do not run it again.
  `config lint` runs it uncached. If it fails, the previous selector judgment
  applies.
- Rule scope and `module_cycle` shrink as described under Fixed, so rules that
  waited on a producer that cannot run (TypeScript without a root
  `package.json`, Go without `go.mod`) can now be evaluated and `check` can
  move from exit 2 to 0. A `module_cycle` that existed only through test,
  generated or out-of-scope imports disappears. Pairs past the 200-pair cap of
  a large cycle appear as new findings once the reported ones are fixed.
- Where nested `node_modules` exist, the dependency-cruiser specifier count
  shrinks, so the unresolved percentage it reports can rise.
- Rust repositories: `production_files`, `production_loc`, `test_files` and
  `test_to_production_files` move because of the test-file reclassification.
  These are report-only dimension metrics and no gate reads them.

## v2.3.1 — corpus correctness

Release date: 2026-09-20

This patch fixes report-contract and evidence-attribution defects reproduced
with the published v2.3.0 binary across the real repository corpus.

- Finding lifecycle reconciliation considers every live finding before
  declaring an accepted ID fixed. Accepted warning-rule findings no longer
  appear as both `baseline` and `fixed` in the same report.
- Findings without locations emit `locations: []`; findings without matching
  details emit `matched_by: {}`. Both follow the existing state schema.
- Rule applicability recognizes Python dotted selectors and Rust crate names
  independently of source directory names. Completed zero-violation checks
  remain distinguishable from missing evidence.
- Go deployment discovery uses static main-package discovery without resolving
  imported dependencies, and propagates failed or incomplete discovery instead
  of reporting it as completed. Successful facts from other members remain
  available when one workspace member fails.
- Git history maps source paths through language/module identities and keeps
  timeout, execution failure, and successful no-match outcomes distinct.
- Python finding locations resolve actual module, package and src-layout files
  rather than publishing extensionless guesses.

Existing accepted debt remains readable. More accurate source attribution or
newly disclosed incomplete evidence can change findings, dimensions, or baseline
comparability. The deployment, Git-history, and Python normalization producer
contracts advance to v2; v2.3.0 numerical snapshots therefore require a reviewed
replacement before comparisons resume. Accepted finding fingerprints keep applying.
Review current debt before capturing that replacement. Optional analyzer gaps remain explicit; this patch does not require
every repository to report `healthy`.

## v2.3.0 — architecture guardrails

Release date: 2026-09-20

This release closes correctness gaps in cached evidence, required-rule
evaluation, temporary exceptions, agent repair tasks, and cross-run measurement
compatibility.

- Fact-cache input scope is analyzer-aware and conservative. Source under names
  such as `target`, `venv`, or `node_modules` is not dropped by a shared name
  filter when the analyzer may read it. TypeScript cache keys include the full
  supported `extends` chain; an unresolvable chain bypasses the cache safely.
  Regression coverage pins Go source beneath analyzer-sensitive directory names
  and inherited TypeScript configuration; unresolved TypeScript config chains
  safely bypass caching. Dynamic dependency-cruiser configuration inputs and
  unsupported TypeScript config resolution are also unknown for measurement
  compatibility. Other analyzer input sets remain defined by their extractor
  contracts.
- Required `gate: fail` rules no longer look passed when their producer evidence
  is incomplete. Canonical JSON exposes sorted
  `decision.unevaluated_required_rules` entries with `rule_id` and `reason`;
  known blockers remain `hard_gates: fail`, while an otherwise clean run with
  unevaluated required rules remains `hard_gates: unmeasured` and `check` exit
  `2`. Go applicability is determined from the selected source inventory even
  when package loading fails.
- Waivers are validated at config load. They require a declared or supported
  synthetic rule, applicable endpoint scope, `reason`, `approved_by`,
  and a valid `YYYY-MM-DD` expiry. Matching is independent of YAML order:
  active matches win, and expired matches are used only when no active match
  applies. Baseline capture skips findings covered by temporary waivers,
  including expired waivers, and prints the count instead of accepting them as
  permanent debt.
  Edge-less `map/*` diagnostics retain exact-rule waivers without endpoint
  selectors. The coupling gate itself remains unwaivable.
- Agent repair tasks include `repair_kind` (`code_change` or
  `needs_owner_decision`). Forbidden-dependency goals do not recommend a public
  route, and new-cross-module goals do not recommend baseline capture as a
  repair. Validation commands replay effective `--base`, `--lang`, and
  `--require-tools` flags. `--refresh` is intentionally omitted because cache
  control must not change validation output.
- Each run publishes `comparison.measurement_profile` with the profile version,
  settings hash, producer semantics, statuses, and supported external tool
  versions. Unknown or incompatible profiles produce named
  `non_comparable` reasons and suppress unsupported deltas. The persisted
  baseline is reported separately as `gate_reference`; `--base` is a report-only
  comparison and never replaces the gate reference.
  Symmetric completed partials retain comparison through a typed
  `partial_basis`; missing inputs, timeouts, and opaque unknowns do not.
  Standard JS dependency-cruiser configurations are evaluated once through the
  tool's native loader; the same invocation consumes a frozen snapshot used
  for measurement identity. Unsupported integrations retain facts with an
  explicit unknown identity.
- Migration is deliberate. A profile mismatch or incomplete seam snapshot is
  not repaired by a blanket `archfit baseline`; review current findings and
  capture a new baseline only after an owner accepts the resulting debt.
  Pre-v2.3.0 baselines have no profile and cannot support numerical comparisons,
  although their accepted finding fingerprints remain usable. Capture the new
  baseline in the same pinned analyzer image/platform and build environment as
  CI; a local host's profile is not automatically compatible with CI.
- Python fact caching is disabled until its transient execution environment can
  be identified before lookup. Each analysis obtains fresh grimp facts and
  producer identity; uv's package cache remains available. Cache-key work for
  other analyzers is bounded, and unsupported or oversized inputs run fresh.

## Earlier compatibility changes (already present in v2.2.1)

- Remove the one-release `legacy-json` output and config migration command.
- Accept only the current JSON, baseline, and config schemas.
- Remove the separate report-only git finding-delta block; preserve its useful
  triage signal as optional `agent_tasks[].origin` metadata in canonical JSON.
- Keep task-origin classification report-only: it never changes verdict, gates,
  or exit code.
- Keep `md` as a short alias for the canonical `markdown` output format.
- Old config and baseline files now fail with actionable manual-migration or
  review-and-regenerate guidance.

## v2.1.0 — architecture-state reporting

Release date: 2026-08-28

Archfit now reports architecture state instead of one repository score. It
measures nine dimensions and shows the evidence that supports each result.

### Main changes

- Report structure, coupling, complexity, testability, operations, drift, and
  five related dimensions with explicit status and evidence.
- Add module-graph complexity, dependency-chain depth, fan-in, and fan-out
  metrics.
- Add declared deployment topology and owner provenance metrics.
- Read Go, LCOV, coverage.py, and llvm-cov coverage artifacts with source-hash
  freshness checks.
- Keep missing or stale evidence as `partial` or `unmeasured`. Do not report a
  healthy result without the required producer evidence.
- Keep text, Markdown, SARIF, scorecard, and JSON results aligned.

### Upgrade impact

This release changes the JSON state contract and requires configuration schema
v2. Read the breaking changes and follow the upgrade checklist before you
upgrade a CI or agent integration.

This release was tested against 11 repositories. All strict corpus records
passed. The reachability fixture measured all nine dimensions and returned
`healthy` with check exit `0`.

Breaking changes:

- **`--format json` now emits `archfit.architecture-state.v1` at the document
  root.** Root keys are `verdict`, `decision`, `comparison`, `measurement`,
  `dimensions` (nine envelopes), `coverage`, `findings`, `agent_tasks`, and
  `seams`. There is no repository scalar anywhere in it. Diagnostic-only blocks
  (`tool_coverage` detail, `owner_source`, `config_warnings`,
  `git_finding_delta`, `advisory_tasks`) are not part of the state contract.
- **`--format legacy-json` emits the pre-cutover diagnostic envelope**
  (`archfit.diagnostic.v2`) for exactly one release. It must be selected
  explicitly and never reaches the verdict or the exit code. Consumers that need
  a retired block should migrate now.
- **Config schema v2 is required.** `version: 1` and the retired
  `coupling.gate.min_band` / `coupling.gate.max_drop` keys are rejected with
  exit `3`. Migrate with `archfit config update --migration-only --apply`, which
  bumps the version, removes the retired keys with their documenting comments,
  and splices in a warn-mode replacement. It never infers `mode: fail`, running
  it twice is byte-identical, and the write is validated, backed up, and atomic.
- **The coupling gate is now
  `coupling.gate.distributed_monolith: {mode, max_new_seams}`.** It counts
  logical seams — one ordered module pair, however many imports express it — that
  are newly introduced against a _comparable_ reference. `coupling_balance` no
  longer gates at all. Advisory promotion is gone: the seam gate names its own
  seams.
- **`archfit check`'s exit code IS the state verdict**: `healthy` -> 0,
  `needs_attention` -> 2, `blocked` -> 1, error -> 3. Exit `0` is reachable with
  complete evidence for all nine dimensions, passing hard gates, and no active
  diagnostic. Missing supplied coverage, operational corroboration, or a
  comparable baseline produces `2`, never a fabricated healthy zero.
- **Baseline schema v2** (`archfit.baseline.v2`) stores the four comparison
  fingerprints, hard-gate finding IDs, seam IDs, and the nine dimension
  snapshots — no repository scalar. A v1 file stays readable for its accepted
  fingerprints, is never rewritten on read, and can never support a
  state/dimension/seam comparison (reported as `legacy_score_snapshot_ignored`).
- `archfit baseline` capture is now a pure function of tree + config. Reading the
  file it was about to overwrite made the capture self-referential and it never
  settled; three captures on this repo produced 108, 164, then 148 entries.

New:

- Nine dimension envelopes (`intent`, `structure`, `modularity`, `coupling`,
  `change_locality`, `complexity`, `testability`, `operations`, `drift`), each
  with its own status, gate, confidence, denominator, metrics, and an explicit
  list of what it could not measure.
- Architecture-level complexity from complete declared-module dependency-chain
  and fan-in/fan-out distributions; function-size tails stay diagnostic.
- Declared operational-topology completeness from independent deploy-unit
  corroboration and declared/CODEOWNERS owner provenance.
- Opt-in top-level `coverage:` ingestion for Go coverprofile, LCOV, coverage.py
  JSON, and llvm-cov JSON. Archfit reads CI-produced artifacts and versioned
  source-hash sidecars; it never executes the target repository's tests.
- A coupling **seam ledger**: one record per ordered module pair with a stable
  ID, a score distribution, raw owner/deploy/structural distance facts, the book
  Ch10 quadrant, and a balancing hypothesis.
- `archfit config update --migration-only [--json|--apply]`.
- Text, Markdown, SARIF, and scorecard all report the same verdict, dimension
  statuses, coverage split, and finding IDs. SARIF carries the state in
  `run.properties`; finding identity (ruleId, ruleIndex, `archfit/v1`
  fingerprint) is unchanged by the cutover.

Also, under `--format legacy-json` only:

- `classified_edges.by_balance_driver` / `by_critical_driver` — whether `|S-D|`
  or `10-V` drove each scored edge's book balance.
- `classified_edges.by_module_pair` — scored-edge concentration per module
  boundary. Like every other histogram the dimension reports, it counts scored
  CROSS-BOUNDARY edges only; same-module coupling is reported under
  `local_coupling`.
- Scorecard dimensions carry `raw_value` — the normalized mean before any
  confidence cap — and `cap_applied`, the name of the cap that moved it.
  `raw_value` is emitted whenever it is non-zero, so it appears alongside an
  equal `value` on an uncapped dimension; `cap_applied` appears only when a cap
  actually lowered the value.

The `coupling_balance` `summary` and `evidence` **values** changed (the keys did
not): the summaries now point at the driver histograms, and the scored-fraction
and critical-band lines were moved ahead of the histograms so the critical /
distributed-monolith count stays inside the truncated `why` text.

Internal: the analysis stage sequence moved behind one application-owned stage
executor; `internal/engine`, `internal/analysispipeline`, and `internal/view` are
gone.

Release validation:

- The final branch binary passed the strict 11-repository corpus across Go,
  Python, TypeScript/JavaScript, and Rust. Config migration, repeated JSON byte
  identity, five-format finding parity, and the `0/2/1/3` exit contract were
  checked.
- The Rust follow-up used rustup toolchain `1.98.0`, `rust-analyzer 1.98.0`, and
  `cargo-modules 0.26.0`. The owned corpus harness pins
  `RUSTUP_TOOLCHAIN=1.98.0` by default without modifying third-party projects.
- `partial` and `unmeasured` remain honest outcomes: assertion quality,
  cognitive complexity, observed runtime topology, SBOM, and vulnerability state
  are outside the shipped claims. Missing, stale, or unattributed supplied
  coverage remains partial rather than becoming a healthy zero.

Upgrade checklist:

1. Run `archfit config update --migration-only --json -c .archfit.yaml` and
   review the candidate.
2. Apply it with `archfit config update --migration-only --apply -c
.archfit.yaml`.
3. Update CI to accept `check` exit `2` as `needs_attention` and block on `1` or
   `3` according to local policy.
4. Migrate JSON consumers to `archfit.architecture-state.v1`; use
   `legacy-json` only during this one-release compatibility window.
5. If testability must be measured, generate coverage plus its source-hash
   sidecar in the trusted test job and configure the top-level `coverage:` block.
6. Regenerate a v2 baseline only after reviewing the new state and findings.

## v1.7.0 configuration confidence

Breaking changes:

- `archfit baseline --base <ref>` removed. It never changed the saved baseline —
  a baseline always records the tree as checked out. To compare against a ref,
  use `archfit check --base <ref>` or `archfit analyze --base <ref>`. The removed
  flag now returns `archfit: unknown flag --base` with exit `3`.

New:

- `archfit config compare <candidate>` measures one source tree under two
  configurations and reports the difference. Report-only: exit `0` on success,
  exit `3` on an input or runtime error, and findings never move the exit code.
  Both sides use an empty accepted baseline, so the comparison is raw
  measurement. A higher candidate score is never reported as better.
- `archfit config update --json` emits one machine-readable review document
  (`archfit.config-review.v1`) with status `action_required`, `review_available`,
  or `no_known_issues`. `--json` with `--apply`, `--ai-classify`, or `--refresh`
  is a usage error (exit `3`) rejected before any discovery or write.
- `config update --apply` no longer deletes, comments out, or re-keys a
  configured module stanza. A configured module and a discovered module that own
  the same paths under different names are reported as `name_drift`, and
  configured modules discovery did not emit stay under `removed_modules`. Both
  are review-only and neither raises the status to `action_required`.
- `--base <ref> --json` adds a report-only `git_finding_delta` block that sorts
  the current `agent_tasks[]` into `introduced`, `pre_existing`, and
  `unknown_origin`. Missing analyzer evidence produces `unknown`, never a false
  `introduced`.

Baseline file:

- `.archfit-baseline.json` records `rubric_version` alongside the scorer version
  in its score snapshot, so `coupling.gate.max_drop` refuses to anchor against an
  incompatible snapshot instead of comparing across rubrics. Existing baselines
  without the field are read as rubric version `1`; the file changes on the next
  `archfit baseline` run.

## v1.6.0 CLI redesign

Breaking changes:

- `archfit check` replaces `archfit analyze --gate` and `archfit --gate` for CI
  and agent validation.
- Removed flags:
  - `--gate` → use `archfit check`
  - `--full` → full scan is now the default
  - `--advisory` → advisories are on by default; use `--no-advisories` to hide them
  - `--no-config` → initialize config first with `archfit config init --root .`
- Renamed flags:
  - `--severity` → `--min-severity`
  - `analyze --llm` → `analyze --ai-summary`
  - `explain --llm` → `explain --ai-summary`
  - `config init --llm` → `config init --ai-classify`
  - `config update --llm` → `config update --ai-classify`
  - `--llm-provider` → `--ai-provider`
  - `--llm-model` → `--ai-model`
  - `--no-cache` → `--refresh`

`--refresh` bypasses cache reads but still writes fresh results back to cache.

Release notes for all versions are maintained on GitHub Releases:

<https://github.com/alexei-led/archfit/releases>

The canonical changelog, migration notes, and per-version details are there.

## Next release note draft

Use this entry in the next annotated tag message:

```text
LLM semantic labels and module-role review are now available as an off-gate
workflow.

- `archfit config enrich labels` drafts `.archfit-labels.yaml` entries for
  weak static strength classifications; `archfit config enrich abstained`
  targets cross-module edges whose strength was unknown after all static
  sources and includes source snippets plus model-reported confidence.
- Approved labels are deterministic committed YAML. Drafts are inert, stale
  evidence hashes are ignored with a `labels/stale` advisory, and
  `provenance: llm` labels with medium/low confidence lower
  `coupling_balance` confidence by one band.
- Strength precedence is explicit:
  config-authoritative > compiler-grade > SCIP/heuristic static facts >
  approved LLM label > abstain. LLM labels fill unknown cells; they do not
  override static classifications.
- `archfit config update --ai-classify` proposes review-only module subdomains
  (`core|supporting|generic`), derived volatility, layer suggestions, and
  optional architectural roles, including synthetic module keys, so repos with
  many generated module declarations can review differentiated domain
  volatility without putting model calls on the gate.
```
