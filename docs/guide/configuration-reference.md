# Configuration reference

`archfit` reads `.archfit.yaml` from the repository root by default.

The config parser is strict. Unknown YAML fields are errors. Start with
`archfit config init`, then edit the generated file.

```sh
archfit config init --root . --output .archfit.yaml
archfit analyze --config .archfit.yaml
```

## Top-level layout

```
version         — required; must be 2
exclude         — path globs to skip during scanning
languages       — per-language extractor settings (go/typescript/python/rust)
analyzers       — opt-in deeper analysis backends (syntax/scip/clones/cargo_modules)
coverage        — opt-in ingestion of caller-supplied test coverage
ai              — off-gate AI provider for enrich/explain/analyze --ai-summary
coupling        — Balanced-Coupling advisory tuning + distributed-monolith seam gate
layers          — ordered architecture layers, inner to outer
modules         — path ownership map
external_systems — declared external integration seams scored at D=10
rules           — executable architecture constraints
waivers         — approved temporary deviations from rules
metrics         — metric policy settings
module_review   — review of the module declarations; can block on unowned source
file_class      — file-class override patterns
outputs         — output format preferences
```

## Complete small example

```yaml
version: 2

exclude:
  - "**/generated/**"
  - "**/*.pb.go"

languages:
  go:
    enabled: auto
  typescript:
    enabled: false
  python:
    enabled: true
    package: myapp

coupling:
  min_severity: medium
  duplicated_knowledge: score

layers:
  - domain
  - application
  - adapter

modules:
  domain:
    paths:
      - internal/domain/**
    public:
      - internal/domain # the one package other modules import
    internal:
      - internal/domain/** # every other domain package is private
    layer: domain
    subdomain: core
    volatility: high
    owner: team-domain
    deploy_unit: api
    reviewed_at: "2026-06-01"
    reviewed_by: "@architect"
  http:
    paths:
      - internal/http/**
    public:
      - internal/http
    layer: adapter
    subdomain: supporting
    owner: team-platform
    deploy_unit: api

rules:
  - id: domain_no_http
    type: forbidden_dependency
    from: internal/domain/**
    to: internal/http
    gate: fail
  - id: layer_direction
    type: forbidden_layer_direction
    gate: fail
  - id: no_internal_access
    type: public_api_only
    gate: fail

waivers:
  - rule: domain_no_http
    from: internal/domain/legacy/**
    to: internal/http
    reason: migration in progress
    approved_by: "@lead"
    expires: "2026-12-01"

module_review:
  stale_after: 2160h
  gate: warn

metrics:
  encapsulation:
    enabled: true
    gate: warn
    min_delta: 0
  cycle:
    enabled: true
    gate: fail
    max_new: 0

outputs:
  json: true
  markdown: true
  sarif: false
```

## `exclude`

`exclude` is a list of repo-relative globs skipped during extraction:

```yaml
exclude:
  - "**/generated/**"
  - "**/*.pb.go"
```

archfit also ships a built-in default exclusion set — tool-artifact, cache,
dependency directories, and test fixtures it never analyses, because measuring them
yields non-deterministic or irrelevant facts (a vendored tree's size metrics, a
generated index, a report written back into the scanned repo, or a fixture repo
inside `testdata/` distorting coverage signals):

```text
.archfit-cache/  .archfit-baseline.json  .gitnexus/  .codegraph/  reports/
.venv/  node_modules/  vendor/  dist/  build/  **/testdata/**
```

`**/testdata/**` is excluded by default because test fixture repos are not
production architecture: analysing them creates false coverage gaps and phantom
language detections. Re-include intentionally with `!testdata` or
`!**/testdata/**` in your `exclude` list.

The built-ins are **merged with** your config `exclude`, not replaced. To
re-include one of them, prefix it with `!`:

```yaml
exclude:
  - "!reports" # analyse reports/ despite the default
```

archfit also warns (a config warning + stderr line) when an output/report path
resolves **inside** the analyzed root — write reports outside `--root`, or exclude
their directory, to keep scans deterministic.

Excluded files also leave every rule's scope. A rule is evaluated over the
languages of the source files its selectors reach; a stray tooling script (a
`.cjs` config wrapper, a Python helper) in a Go repository would otherwise put
TypeScript or Python in scope and keep the rule unevaluated until
dependency-cruiser or grimp ran. Exclude the script, or switch its language off
(`languages.<id>.enabled: false`), to declare it out of scope. Size and
file-class metrics still count excluded files the LOC walk visits.

Rule scope also follows each language's own applicability. When a language's
extractor finds no project under the analysis root — TypeScript with no
`package.json` at the root (a `web/ui/` app inside a Go repository), Go files
with no `go.mod`, Python with no `pyproject.toml`/`setup.py` — its coverage row
is `absent` with no gap, and its files leave the scope of every rule that reads
dependency edges, so they cannot hold a rule unevaluated waiting for a producer
that would never run. The same holds for a Rust file outside every cargo
workspace member (a `fuzz/` crate the workspace excludes). An explicit
`languages.<id>.gate: warn|fail` keeps the files in scope. `forbidden_pattern`
reads the ast-grep pattern pass, not the dependency graph, and keeps them.
Analyse such source by pointing `--root` at its project.

## `languages`

Per-language extractor settings. Each language has `enabled` and `gate`; some
have extra fields.

`enabled` accepts three values only — `true`, `false`, or `auto`. The legacy
string spellings `"on"` and `"off"` are a **hard error** in this schema.

- `true` — require the adapter; missing project markers or tools are errors.
- `false` — skip the adapter entirely. The language's source files also leave
  rule scope, so they cannot hold a rule unevaluated. Set an explicit `gate:` to
  keep them in scope and be told the analyzer did not run.
- `auto` — use the adapter when project markers and tools are found (default).

Use `auto` for mixed repos while calibrating. Use `true` in CI when a language
must be analyzed.

### `languages.go`

```yaml
languages:
  go:
    enabled: auto # true | false | auto
    gate: warn # off | warn (default) | fail
    modules:
      include: [] # ScanRoot-relative globs; empty = all in-scope members
      exclude: [] # drop members matching these (applied after include)
```

`gate` controls the CI posture when the Go extractor is absent. `warn` (default)
surfaces the gap but exits 0. `fail` makes CI exit 1 on a missing extractor.

`languages.go.modules` restricts Go workspace analysis to a subset of `go.work`
members. Useful when the workspace has many members (hundreds) and you want a fast
focused run, or when the workspace root run times out:

```yaml
languages:
  go:
    enabled: true
    modules:
      include:
        - server/shared/** # keep only members whose RelDir matches
        - server/auth/**
      exclude:
        - "**/testdata/**" # drop members matching these after include
```

When the resulting member set is empty the Go extractor reports `absent`. No
coverage gap is raised for it: the gap probe applies the same include/exclude
filter, so scoping the member set never reads as a missing Go toolchain.

**Scale note:** a full `NeedTypesInfo` load of 100+ members takes 1–2 minutes
wall-clock on a warm build cache. Use `languages.go.modules` to scope the load;
also set a timeout (see [analyzers timeouts](#analyzersxtimeout)) as a watchdog
for pathological cases.

### `languages.typescript`

```yaml
languages:
  typescript:
    enabled: auto
    gate: warn
```

### `languages.python`

```yaml
languages:
  python:
    enabled: auto
    gate: warn
    package: myapp # top-level Python package; required when it differs from the repo name or uses src/ layout
```

`package` replaces the old top-level `python_package` key.

### `languages.rust`

```yaml
languages:
  rust:
    enabled: auto
    gate: warn
    manifest: "" # path to a non-root Cargo.toml; empty = auto (root manifest)
    features: [] # cargo features to activate for the metadata run
    include_dev_deps: false # include dev-dependencies as crate edges
```

See [Language support](languages.md) for per-language setup and
[Tooling reference](tooling.md) for platform-specific install commands, versions,
home pages, and PATH checks.

## `coverage`

`coverage` supplies test-execution evidence that another step already produced.
It is opt-in and disabled by default:

```yaml
coverage:
  enabled: true
  gate: warn
  sources:
    - path: coverage.out
      format: go-coverprofile
      sidecar_path: coverage.out.sidecar.json
      max_bytes: 67108864
      max_facts: 1000000
    - path: coverage/lcov.info
      format: lcov
```

**Archfit never runs the target repository's tests.** `analyze` and `check` only
read the configured artifacts and their attestation sidecars. Running foreign
tests inside architecture analysis would execute arbitrary repository code, make
the result depend on network/database/runtime state, and make repeated analysis
non-hermetic. Generate coverage in the repository's existing test job, then run
Archfit against those files.

Fields:

- `enabled` — `false` by default. When false or absent, Archfit does not read any
  coverage artifact and emits no `supplied-coverage` tool row. Production source
  still leaves the testability dimension `partial`; disabled measurement is not
  evidence of zero coverage.
- `gate` — `off`, `warn` (default), or `fail`. It controls the required-tool
  posture when none of the configured artifacts can be read. `fail` can block
  `check`; `warn` and `off` do not. `--require-tools` escalates an unset/warn gap
  but never overrides explicit `off`. A partially usable, invalid, stale, or
  unattributed input keeps testability `partial` but does not become this hard
  gate. This is an evidence-availability gate, not a minimum coverage-ratio
  threshold.
- `sources` — one or more artifacts; required when `enabled: true`. Artifact and
  sidecar paths are resolved under the analysis root and may not escape it.
- `path` — artifact path relative to the analysis root.
- `format` — `auto` (the default), `go-coverprofile`, `lcov`,
  `coverage-py-json`, or `llvm-cov-json`. `auto` requires one unambiguous match
  between the extension and file content; set an explicit format when a generic
  filename could be ambiguous.
- `sidecar_path` — optional path to the versioned freshness sidecar. The default
  is `<path>.sidecar.json`.
- `max_bytes` — positive per-source byte limit for the artifact, sidecar, and
  producer-enumerated source files read for freshness verification. The default
  is `67108864` (64 MiB). Exceeding it rejects the input; Archfit never truncates
  coverage evidence.
- `max_facts` — positive per-source limit on normalized coverage facts. The
  default is `1000000`. Exceeding it rejects the whole artifact before caching
  or module attribution rather than returning an incomplete prefix.

Supported formats and units:

| Format             | Typical producer                            | Unit read by Archfit |
| ------------------ | ------------------------------------------- | -------------------- |
| `go-coverprofile`  | `go test -coverprofile`                     | statements           |
| `lcov`             | c8, Vitest/Jest, or `cargo llvm-cov --lcov` | lines                |
| `coverage-py-json` | coverage.py / pytest-cov JSON               | statements           |
| `llvm-cov-json`    | `cargo llvm-cov --json --summary-only`      | lines                |

Archfit normalizes every parsed file to a slash-separated, analysis-root-relative
path before module attribution. Absolute paths inside the root and Go import-path
prefixes are normalized; paths outside the root, missing files, or escaping
symlinks increment `unresolved_coverage_paths` and keep testability `partial`.
Nothing is silently discarded as uncovered.

### Coverage freshness sidecar

A coverage artifact cannot prove which worktree bytes it measured. Its producer
must write a JSON sidecar alongside the artifact (or at `sidecar_path`) with this
version 1 shape:

```json
{
  "schema_version": 1,
  "source_ref": "0123456789abcdef",
  "modules": ["api", "worker"],
  "sources": {
    "internal/api/handler.go": "<lowercase-sha256-of-file-bytes>",
    "internal/worker/job.go": "<lowercase-sha256-of-file-bytes>"
  }
}
```

- `schema_version` must be exactly `1`.
- `source_ref` is producer metadata, usually a commit SHA. It is not a freshness
  gate because Archfit scans a worktree, which may be dirty or contain untracked
  files.
- `modules` names the declared modules the producer intended to represent. Each
  value must be non-empty; Archfit independently attributes parsed source paths
  to the current module map.
- `sources` enumerates the covered source universe with lowercase SHA256 hashes
  of the exact file bytes. Include every source represented by the artifact.

Freshness is `matched` when `sources` is non-empty, every normalized file fact
parsed from the artifact has a matching sidecar entry, and every enumerated
source exists under the current analysis root with bytes hashing to the sidecar
value. A missing or changed listed source, or a parsed covered path omitted from
`sources`, is `stale` with reason `worktree_differs_from_ref`. Empty or missing
`sources`, or a missing, unreadable, malformed, or unknown-version sidecar, is
`unverified` with reason `freshness_unverified`. Only `matched` can promote
testability; both other values keep it `partial`. The comparison runs on every
analysis, including parsed-fact cache hits.

The sidecar is producer-attested and unsigned. Cross-binding prevents an empty,
partial, or unrelated source map from attesting the files described by the
artifact, but Archfit cannot authenticate the producer or prove the artifact
itself did not omit source facts. A faulty or malicious producer can alter the
artifact and sidecar together. Signature-backed attestation remains the recorded
upgrade trigger in the
[evidence contract](../design/evidence-contract.md#accepted-ceilings-and-upgrade-triggers).

### Testability promotion and multiple artifacts

Testability becomes `measured` only when production source exists, supplied units
are valid and compatible, every declared module is represented, every path and
module is attributed, unresolved paths are zero, and every sidecar is `matched`.
The numeric `coverage_ratio` may be zero: measurement status means the denominator
is complete, not that the observed value is desirable. Assertion quality,
mutation resistance, and meaningful boundary-test semantics remain out of claim.

Multiple artifacts may be combined only when their units are compatible. For a
duplicate file with the same unit and total, Archfit keeps the greatest covered
count as a conservative lower bound and reports `merged_coverage_facts`.
Differing units or totals keep the dimension `partial` and suppress an aggregate
ratio rather than inventing a union.

Concrete producer commands for all four supported languages are in
[Language support](languages.md#supplied-test-coverage). Coverage-producer tools
are intentionally not Archfit dependencies; see
[Tooling reference](tooling.md#coverage-producers-are-external).

## `analyzers`

Opt-in deeper analysis backends. They produce extra facts; gates live in `rules:`.

Each analyzer has `enabled` and `gate`; timed analyzers add `timeout` (a Go
duration string, e.g. `"5m"`). On timeout the result is dropped cleanly and
dependent metrics report `n/a (timed out)` — the run continues.

```yaml
analyzers:
  syntax:
    enabled: auto # structural declaration facts (ast-grep)
  scip:
    enabled: auto # symbol-level strength (SCIP indexers)
    timeout: 10m
  clones:
    enabled: auto # clone detection (jscpd)
    timeout: 5m
  cargo_modules:
    enabled: auto # Rust intra-crate module graph
```

For Rust projects with a root `Cargo.toml`, `archfit config init` and
`archfit config update --apply` emit explicit deep-analysis defaults:

```yaml
languages:
  rust:
    enabled: auto
analyzers:
  cargo_modules:
    enabled: true
  scip:
    enabled: true
```

This avoids single-crate Rust runs collapsing to one crate-level node. It also
adds cost: `cargo-modules` may compile crates, and SCIP requires `rust-analyzer`
plus `uv`. Missing tools are reported as coverage gaps, not hard crashes.

### `analyzers.syntax`

Runs the ast-grep adapter to extract declaration-level facts for Go, TypeScript,
Python, and Rust. Unlike the dependency extractors, which answer _who imports
whom_, `analyzers.syntax` answers _what declarations exist_ — exported names,
kinds, and framework routes.

Enable with `true`:

```yaml
analyzers:
  syntax:
    enabled: true
```

**What it does:**

- Emits a `syntax_facts` block in the diagnostic (neutral, off-gate, omitted
  when empty).
- Each fact records: `language`, `file`, `kind` (function/method/class/struct/
  interface/trait/enum/type_alias/annotation/route/type_leak/lazy_import), `name`, `exported`,
  `framework`, `start_line`, `end_line`.
- The analyze output gains a **Syntax surface** section listing declaration counts,
  detected routes, and public API totals per module.
- `agent_tasks` evidence is enriched with per-node declaration counts when
  syntax facts are present.

**Supported languages:** Go, TypeScript, Python, Rust. Requires `sg` (ast-grep)
on PATH; `archfit doctor` checks it.

### `analyzers.scip` and `analyzers.clones`

```yaml
analyzers:
  scip:
    enabled: true # symbol-level edge strength (SCIP indexers)
    timeout: 10m
  clones:
    enabled: true # clone detection (jscpd)
    timeout: 5m
```

- `scip` — runs a SCIP indexer (`scip-go`/`scip-python`/`scip-typescript`/`rust-analyzer scip`) plus
  `uv` to build the symbol graph. Upgrades edge strength for TypeScript/Python/Rust.
  For Go, SCIP is supplementary — Go type-info from `go/packages` is the primary
  strength source.
- `clones` — runs `jscpd` to find cross-module duplicated logic. When a clone pair
  spans two modules, their shared edge strength is upgraded to `symmetric` in the
  `coupling_balance` scorer, reflecting undeclared hidden coupling. When the two
  modules share **no** import edge at all, the pair is clone-only duplicated
  knowledge (book Ch7): by default (`coupling.duplicated_knowledge: score`) it
  enters `coupling_balance` as a symmetric-strength coupling fact and also
  surfaces as a `bc/duplicated_knowledge` advisory when its severity passes
  filters. Set `coupling.duplicated_knowledge: advisory` to preserve the v4
  report-only behavior. Its severity comes from the standard formula (symmetric
  strength × module-pair distance × worst-of-pair volatility); `coupling.min_severity`
  and approved `.archfit-labels.yaml` labels (either direction) suppress the
  advisory; the [`coupling.gate`](#couplinggate) never promotes the advisory.

`scip` and `clones` are opt-in: `auto` and `false` (and absent) all disable them;
the run continues without them and the gate verdict is unaffected.

### `analyzers.cargo_modules`

```yaml
analyzers:
  cargo_modules:
    enabled: true
```

Runs `cargo-modules` to emit `<crate>::<mod>` nodes and aggregated `uses` edges,
providing intra-crate module depth for Rust repos. Pair it with
`analyzers.scip.enabled: true` for Rust so those module edges also receive
symbol-level strength from `rust-analyzer scip`.

### `analyzers.<x>.gate` (coverage gate)

When an analyzer is **absent** (tool not installed or not found), its metrics
drop to `n/a` and a coverage gap is reported with an install hint
(see [`archfit check`](commands.md#archfit-check)). By default the
gap is reported without setting the repository hard gate to `fail`; incomplete
dimension or required-rule evidence can still make `check` exit `2`. Set a
per-analyzer `gate` to make CI block on the missing tool:

When an analyzer is **disabled by config** (`enabled: false`), it is simply
skipped — no coverage gap is emitted and no install prompt is shown.
Disabled-by-config is distinct from absent: a tool you deliberately turned off
should not appear as a gap to resolve. A language switched off with
`languages.<id>.enabled: false` **on a repo that contains that language** reports
`disabled` in `tool_coverage`, not `absent`, so "measurement switched off" is
never read as "this language is not in the tree" by `config compare` or
`analyze --base`. Switching off a language the repo does not contain changes
nothing: the row stays `absent`, because there was nothing to stop measuring.
Pinning an explicit `gate:` on a disabled language opts back in: the row stays
`absent` and still raises its gap.

```yaml
languages:
  go:
    enabled: true
    gate: fail # block CI when the go/packages analyzer is missing
  python:
    enabled: auto
    gate: warn # default: surface the gap, do not fail
```

`gate` accepts `off`, `warn` (default), or `fail`. A `fail` gate exits `1` (a
policy violation, distinct from exit `3` tool errors). The `--require-tools` flag
on `analyze` is the run-level shortcut — it raises **every** gap to `fail`
without editing config.

### `analyzers.<x>.timeout`

Per-analyzer watchdog for subprocess analyzers: `analyzers.scip` and
`analyzers.clones`. When the subprocess exceeds the timeout, the result is dropped
cleanly and the gate verdict is determined by the remaining analyzers.

```yaml
analyzers:
  scip:
    enabled: true
    timeout: 10m # Go duration string; e.g. "5m", "10m30s"
  clones:
    enabled: true
    timeout: 5m
```

Zero or absent means the built-in package default (`scip`: 20 minutes, `clones`:
5 minutes). Set an explicit timeout when a generated or very large file causes a hang.

## `ai`

Off-gate LLM provider configuration. Consumed only by `config init --ai-classify`,
`config update --ai-classify`, `config enrich`, `analyze --ai-summary`, and `explain --ai-summary` — never
by the deterministic gate.

```yaml
ai:
  provider: anthropic # anthropic | openai | ollama
  model: claude-opus-4-8
  base_url: "" # ollama only; default http://localhost:11434/v1
```

API keys come from `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` env vars — never from
config. archfit also best-effort loads a local `.env` (cwd) at startup, but only
sets a key that is currently **unset** — real environment variables and CI secrets
always win. Keep `.env` out of version control (it is gitignored by default). The
LLM response cache lives at `.archfit-cache/llm/`.

## `coupling`

Balanced-Coupling advisory tuning and the distributed-monolith seam gate.

```yaml
coupling:
  min_severity: medium # low | medium (default) | high | critical
  duplicated_knowledge: score # score (default) | advisory
  volatility_cascade: false
  gate:
    distributed_monolith:
      mode: warn # warn (default, diagnostic) | fail (blocking)
      max_new_seams: 0 # tolerated newly introduced qualifying seams
```

- `min_severity` — minimum advisory severity to show: `low` (all cross-module
  deps, noisy), `medium` (default, over-decoupled volatile seams and tight
  cross-boundary coupling), `high` or `critical` (intrusive/functional coupling
  across large boundaries only).
- `duplicated_knowledge` — clone-only cross-module duplicated knowledge policy:
  `score` (default) includes clone-only pairs in `coupling_balance` as
  symmetric-strength coupling facts; `advisory` preserves the v4 behavior where
  clone-only pairs can emit `bc/duplicated_knowledge` advisories but do not move
  the headline score. JSON exposes the policy effect through
  `classified_edges.clone_only_scored` and `classified_edges.clone_only_advisory`.
- `volatility_cascade` — opt-in book Ch9 propagation: when `true`, a module
  strongly coupled (`functional`, `symmetric`, or `intrusive` strength) to a
  high-effective-volatility module inherits raised effective volatility (`high`).
  The pass runs to a deterministic fixpoint and never lowers configured values.
  Disabled by default; safe to enable once `subdomain` fields are complete.

### `coupling.gate`

The only coupling gate. It counts **logical seams**, not import edges: one
ordered module pair is one seam however many imports express it, so a seam
written forty times does not read as forty times the risk.

The block requires `distributed_monolith`; an empty `gate:` is a config error
(it would gate nothing). Omitting the whole block is fine and is **not** the
same as switching the rule off — an absent block means `mode: warn`,
`max_new_seams: 0`.

A seam qualifies as a distributed monolith when it has at least one **active
edge** — a current source-graph edge between two resolved modules — in the
critical band at high distance (`cross_module_different_owner` or
`cross_deploy_unit`). Advisory display filters, baseline acceptance, and waivers
cannot hide a qualifying seam: the rule reads the full classified edge set.

- `mode` — `warn` (default) reports qualifying seams and never fails the run.
  `fail` blocks, but only on seams **newly introduced** against a comparable
  reference. Opt into `fail` after a report-only run against a comparable
  baseline shows the seam count you expect.
- `max_new_seams` — tolerated count of newly introduced qualifying seams in
  `fail` mode. Unset means `0`.

"Newly introduced" needs a reference whose config, module map, labels, and
rubric all match this run. Without one the rule reports the seam total, states
that no new-seam count is claimed, and does not block — an unrated gate never
fails. The reference is the stored `.archfit-baseline.json` written by
`archfit baseline`, exposed in architecture-state JSON as `gate_reference`.
`analyze --base <ref>` produces a report-only comparison against another tree;
it is **not** a seam-gate reference, so a run with `--base` and no committed
baseline still makes no "new seam" claim. The root `comparison` block describes
`--base`; `gate_reference` describes the persisted baseline independently.

When the gate blocks, the verdict becomes FAIL (exit 1), the reasons print to
stderr, and the run emits one `bc/coupling_gate` gate finding **per newly
introduced seam**, each naming its module pair, so `agent_tasks[]` points at
the seams that blocked rather than at unrelated advisories.

### Measurement compatibility

Every architecture-state run carries `comparison.measurement_profile`. It is
the identity of the measurement conditions, separate from the four policy
fingerprints. The object contains:

- `version` — the Archfit measurement-profile contract, currently
  `archfit.measurement.v1`;
- `settings_hash` — the normalized extractor and acquisition settings;
- `producers[]` — one row per evidence producer with `tool`,
  `semantics_version`, `status`, and (when applicable) `tool_version`;
- `unknowns[]` — reasons Archfit could not establish a producer or environment
  fact.

Comparisons require compatible settings and producer semantics, availability,
and supported external tool versions. An unknown or incompatible profile makes
the comparison `non_comparable` with named reasons; Archfit does not invent a
delta. This applies to `config compare` and to `comparison.status` under
`--base`. A profile difference does not change `--base` origin; it is named in
`comparison.origin_reasons`. The
Unresolved dynamic dependency-cruiser configuration inputs and unsupported
TypeScript config resolution are recorded as unknown profile inputs and disable
comparability. The
persisted baseline carries the profile in `state`; its compatibility is exposed
as `gate_reference` and is independent of the report-only `comparison` block.
Do not use a blanket `archfit baseline` run as a profile migration.

#### Unsupported older schemas

A config declaring `version: 1`, `coupling.gate.min_band`, or
`coupling.gate.max_drop` is rejected with exit `3`. Archfit does not ship a
compatibility loader or migration command. Follow the exact manual transform in
[`skills/archfit/references/migration.md`](../../skills/archfit/references/migration.md),
then validate with report-only `archfit analyze --format json`. The replacement
`distributed_monolith` rule starts in `warn`; promoting it to `fail` is an owner
decision after comparison against a representative base.

`coupling_balance` is not a `metrics:` entry; a `metrics.coupling_balance:`
key is a config error that points here.

## `layers`

Layers are ordered from inner to outer.

```yaml
layers: [domain, application, adapter]
```

The `forbidden_layer_direction` rule treats dependencies from an earlier layer to
a later layer as violations. With the example above, `domain -> adapter` is a
violation, while `adapter -> domain` is allowed.

### Worked example: closing a layer-direction gap

Repos like yazi (Rust) have clear architectural layers (`yazi-core`, `yazi-adapter`,
`yazi-plugin`) but no declared `layers:` or `forbidden_layer_direction` rule in
`.archfit.yaml`. The capability exists; the config gap is the missing step. Adding
the rule closes the gap:

```yaml
layers:
  - core # innermost
  - plugin # extension layer
  - adapter # I/O and system adapters (outermost)

modules:
  yazi-core:
    paths: [yazi-core/**]
    layer: core
  yazi-plugin:
    paths: [yazi-plugin/**]
    layer: plugin
  yazi-adapter:
    paths: [yazi-adapter/**]
    layer: adapter

rules:
  - id: no_core_to_adapter
    type: forbidden_layer_direction
    gate: warn # start advisory; flip to fail when the layer map is stable
```

With this in place, any `yazi-core` → `yazi-adapter` import becomes a finding.

**How to get there without authoring manually:** `archfit config enrich` can propose a
layer structure and `forbidden_layer_direction` rules from the module graph; draft
the output, review, then move approved entries into `.archfit.yaml`. See
[llm-enrich.md](llm-enrich.md).

## `modules`

`modules` maps a stable module name to owned paths and metadata.

```yaml
modules:
  pricing:
    paths: [services/pricing/**]
    public: [services/pricing/contracts/**]
    internal: [services/pricing/**]
    layer: domain
    subdomain: core
    volatility: high
    owner: team-pricing
    deploy_unit: pricing-service
```

Fields:

- `paths` — globs that claim files, packages, or modules for ownership.
- `public` — declared public API surface. Matching targets are classified as
  contract coupling, and a target that matches its own module's `public` glob is
  never internal access, even when it also matches an `internal` glob or the
  extractor marked it internal.
- `internal` — private surface. Matching targets are classified as intrusive
  coupling, and `public_api_only`/`internal_api_access` treat them as internal
  access in every language.
- `layer` — one of the names from `layers`.
- `subdomain` — DDD subdomain classification: `core`, `supporting`, or `generic`.
  Determines the volatility ordinal when no explicit `volatility` is set.
- `volatility` — explicit override: `high` (=10), `medium` (=6), `low` (=3),
  or `frozen` / `legacy` (=1). Use `subdomain` unless you need a specific value
  that differs from the DDD default.

`layer`, `subdomain`, and `volatility` values are matched as listed
(`subdomain` and `volatility` case-insensitively). Any other value still loads in
schema v2 but classifies as if nothing were declared: `analyze` and `check`
print a config warning, and `archfit config lint` reports `undeclared_layer`,
`unknown_subdomain`, or `unknown_volatility` as an error.

- `owner` — team or person responsible for the module.
- `deploy_unit` — deployable/runtime unit used for distance classification.
- `role` — optional architectural role. See [Module role vs layer](#module-role-vs-layer).
- `reviewed_at` — date of last architecture-map review.
- `reviewed_by` — reviewer identity.
- `depends_on` — outbound allowlist: the only declared modules this module can
  import. See [Module allowlists](#module-allowlists).
- `visible_to` — inbound allowlist: the only declared modules that can import
  this module. See [Module allowlists](#module-allowlists).

`public` alone only exempts paths; it never flags an import that misses it. To
make archfit block imports that bypass a module's public package, pair the
public package with an `internal` glob over the rest of the module, as
`pricing` does above: `contracts/**` stays public and everything else under
`services/pricing/` is private. A nested `internal/` directory inside its own
module is not enough on Go: the Go compiler already rejects every import of it
from outside the parent tree, so no cross-module edge to it can exist.

### Module allowlists

`depends_on` and `visible_to` state the allowed module graph. The
`module_dependencies` rule enforces both lists. Without that rule, the lists
have no effect.

```yaml
modules:
  billing:
    paths: [billing/**]
    public: [billing/api]
    visible_to: [shipping, api-gateway] # only these modules can import billing
  shipping:
    paths: [shipping/**]
    depends_on: [billing, shared-kernel] # shipping can import only these modules
  shared-kernel:
    paths: [shared/**]
    depends_on: [] # imports no first-party module
rules:
  - id: boundaries
    type: module_dependencies
    gate: fail
```

Rules for `depends_on` (outbound):

1. An absent key means no constraint. An empty list allows no first-party
   module.
2. Each import from the module into another declared module must name that
   module in the list.
3. A target that no declared module owns is out of scope: standard library,
   third-party packages, and unowned first-party source. Use
   `forbidden_dependency` to ban those.

Rules for `visible_to` (inbound):

1. An absent key means every module can import the module.
2. Each importer from another declared module must be in the list.
3. An importer that no declared module owns is always denied. The allowlist
   fails closed: a new package outside every module cannot import a module that
   sets `visible_to`.

Each entry is a [module selector](#module-selectors): a module name, a
`layer:` or `role:` selector, or a glob over module names. `visible_to:
[layer:app]` admits every module in the `app` layer. An entry that selects no
module still loads, but it allows nothing: `analyze` and `check` print a
config warning, and `archfit config lint` reports `unknown_module` as an
error.

Modules resolve against the declared `modules:` map only. Code that only an
auto-registered module owns (a `go.work` member or a Rust `crate::mod` node
that no declared module claims) counts as unowned. A `crate::mod` node of a
crate that a declared module owns belongs to that module.

The lists do not change `model_hash`: they move neither distance nor seam
identity. They do change `config_hash`, so an allowlist edit makes a stored
baseline non-comparable until you run `archfit baseline` again.

### Module selectors

A module selector picks declared modules. Allowlist entries (`depends_on`,
`visible_to`) and the `from_module`/`to_module` keys of `forbidden_dependency`
use it.

| Selector       | Selects                                             |
| -------------- | --------------------------------------------------- |
| `layer:<name>` | Every module whose `layer` is `<name>`              |
| `role:<role>`  | Every module whose `role` is `<role>`               |
| Any other text | A doublestar glob over declared module names        |

A plain module name is a glob that selects that one module. Selectors see
declared modules only: an auto-registered module (a `go.work` member, a Rust
`crate::mod` node no declared module claims) is never selected.

### Module role vs layer

`layer` and `subdomain` are complementary — they capture different dimensions:

- **`layer`** — topological position in the dependency DAG (domain, application,
  adapter, …). Controls `forbidden_layer_direction`.
- **`subdomain`** — DDD subdomain classification (`core`, `supporting`, `generic`).
  Controls the volatility ordinal used by the Balanced Coupling scorer.
- **`role`** — optional architectural _function_ within a layer. Lets archfit
  adjust coupling scoring for modules that are _supposed_ to fan out.

`role` refines Balanced-Coupling distance classification for modules that are
legitimately wide. In a one-binary CLI, the `cmd` package wires every adapter
together — that is composition-root cohesion, not high-distance coupling. Without
a role, archfit scores those outbound edges as unbalanced and emits
false-positive advisories.

```yaml
modules:
  cli:
    paths: [cmd/archfit/**]
    role: composition_root
```

Accepted `role` values:

- `composition_root` — wiring/entrypoint that legitimately depends on many
  modules (e.g. `cmd`, `main`).
- `generated` — generated code (its fan-out is mechanical, not designed).
- `test` — test-support code.
- `adapter`, `core`, `shared_model` — descriptive; reserved for future
  refinement.

For a `composition_root`, `generated`, or `test` source module, archfit
downgrades its outbound cross-deploy / different-owner edges to
cross-module-same-owner, so the advisory severity reads cohesion. A `core -> core`
unbalanced edge is **still** flagged, and inbound edges to a wiring module are
unaffected.

### Volatility and subdomain

Volatility is **not** guessed from directory names. The old path-heuristic
(`db/`, `infra/`, `lib/` → volatility) is removed. A module that declares neither
`subdomain` nor `volatility` resolves to `"undeclared"` — archfit reports it and
emits agent tasks asking for a declaration.

Declare intent explicitly:

```yaml
modules:
  domain:
    subdomain: core # → volatility ordinal high (10)
  infra:
    subdomain: supporting # → volatility ordinal low (3)
  utils:
    subdomain: generic # → volatility ordinal low (3)
  config:
    subdomain: supporting
    volatility: medium # explicit override; medium only via direct declaration
  retired-api:
    volatility: frozen # explicit stable/legacy override (ordinal 1)
```

Methodology (Khononov book):

- `core` → `high` volatility (ordinal 10)
- `supporting` → `low` volatility (ordinal 3)
- `generic` → `low` volatility (ordinal 3)

`medium` volatility (ordinal 6) is only reachable via explicit `volatility: medium`.
`frozen` and `legacy` both resolve to the frozen/legacy anchor (ordinal 1).
Declaring `subdomain: supporting` never implies medium — it implies low.

### Distance classification

Balanced Coupling classification uses module metadata:

- target `public` match → `contract` strength;
- target `internal` match → `intrusive` strength;
- `volatility` or `subdomain` → target volatility.

Distance is a **composite** of three signals, not a single-winner precedence chain:

1. **Code structure** — always-available baseline. Sibling or parent-child packages
   (shared subtree) → `cross_module_same_owner`; different subtrees →
   `cross_module_different_owner`. Two unrelated flat (single-segment) names have
   no tree evidence of separate teams, so they stay at the honest floor:
   `cross_module_same_owner`.
2. **Ownership** — contributes only when ownership is informative. In repos where
   every module has the same owner (single-maintainer or one-team repos), ownership
   becomes **neutral** and does not collapse far-apart modules to "same owner = low
   risk". When multiple distinct owners exist, ownership overrides code structure.
3. **Deploy unit** — absolute boundary. If the two modules have different
   `deploy_unit` values, distance is always `cross_deploy_unit` regardless of owner
   or structure.

Composite resolution order (first applicable wins):

1. same module → `same_module`;
2. different `deploy_unit` on the two modules → `cross_deploy_unit`;
3. ownership is informative (two or more distinct owners in the repo) →
   same owner → `cross_module_same_owner`; different (or one unknown) →
   `cross_module_different_owner`;
4. otherwise → code structure decides (shared subtree or unrelated flat names →
   `cross_module_same_owner`; different subtrees → `cross_module_different_owner`).

A detected runtime async bridge is recorded as report-only evidence in the
`runtime_async` JSON field per module and the `runtime_async_edges` field per
source-module→runtime-target relation; it does not annotate graph edges, does not
affect distance or score, and does not change the gate verdict.

The `distance_basis` field on each advisory edge (`code_structure`, `ownership`,
or `deploy_unit`) shows which signal drove the composite, so the result is
auditable. Analyze output also includes `distance_context`, whose `owner_model`
identifies `single_owner_degenerate`, `multi_owner`, or `no_owner_signal`; its
`interpretation` explains when low same-owner distance is an intentional
socio-technical signal rather than missing ownership.

> **Small-OSS note:** a repo with one maintainer is not a flat distance space.
> Code structure is the baseline and still distinguishes close vs far modules.
> Same-owner is the lowest cross-module distance; it is a low socio-technical
> distance signal. Ownership only contributes when there are genuinely distinct
> owners to compare.

## `external_systems`

Declares external integration seams that enter `coupling_balance` scoring at the
distance ladder's far end — `declared_external`, D=10.

```yaml
external_systems:
  aws:
    targets: ["github.com/aws/aws-sdk-go-v2/**"]
    # volatility defaults to low
  payment-gateway:
    targets: ["node_modules/@stripe/**", "stripe"]
    volatility: medium
```

**Why declared, not automatic.** The book's Ch10 Example 1 — a cross-vendor
integration — sits at the maximum distance: different codebase, different
company, no shared governance. But scoring **every** library import at D=10
would flood the metric with vendor noise (`fmt`, `lodash`, `serde`, …).
An external system is a _declared integration seam_: a vendor SDK, a payment
gateway client, a generated API stub — a dependency the architect chose to
treat as an architectural boundary worth measuring. Everything undeclared keeps
today's disclosed exclusion: counted in `classified_edges.external`, never
scored, never fabricated.

Field reference:

- `targets` (required, ≥1) — globs matched against the classified edge target,
  in the form the language extractor emits: a Go import path
  (`github.com/aws/aws-sdk-go-v2/**`), a TypeScript resolved package path
  (`node_modules/@aws-sdk/**`), a Python dotted module/root glob
  (`{boto3,boto3.*}` to match both `boto3` and `boto3.session`), or a Rust
  crate name (`aws_sdk_s3`). The match is
  language-independent.
- `volatility` (optional) — `high | medium | low | frozen`. Defaults to `low`,
  per the book's generic-subdomain guidance: an external vendor system is a
  generic capability, presumed stable unless you declare otherwise. Declare
  `high` for an API that churns under you — combined with D=10, strong coupling
  to it scores toward the critical band (the vendor-lock distributed monolith).

Matched edges carry `distance_basis: declared_external` on their advisories and
count in `classified_edges.declared_external`; strength still comes from the
usual sources, and an edge with unknown strength still abstains (abstain rules
are unchanged). When nothing is declared, behavior is identical to previous
versions.

## `rules`

Each rule needs a stable `id` and a `type`.

```yaml
rules:
  - id: no_domain_to_http
    type: forbidden_dependency
    from: internal/domain/**
    to: internal/http/**
    gate: fail
```

### Rule field reference

| Field      | Applies to       | Description                                                                                                            |
| ---------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `id`       | all              | Stable ID used in findings, baselines, and waivers.                                                                    |
| `type`     | all              | [Built-in rule type](#built-in-rule-types). Unknown type is a config error; the schema enumerates the allowed values.  |
| `gate`     | all              | `fail` (or absent for most types), `warn`, or `off`. `public_api_change` and `public_api_type_leak` default to `warn`. |
| `from`     | most             | Source module or path glob.                                                                                            |
| `to`       | most             | Target module or path glob.                                                                                            |
| `max`      | `public_api_max` | Integer ceiling.                                                                                                       |
| `patterns` | `forbidden_pattern` | ast-grep patterns (`id`, `lang`, `rule`) the rule forbids. On any other type they still run but never produce a finding, and `analyze`/`check` print a warning. |
| `guard`    | selector rules   | `true` marks a rule whose `from`/`to` selector is expected to match nothing (see below). Default `false`.              |
| `from_module` | `forbidden_dependency` | [Module selector](#module-selectors) for the importing side, instead of `from`. |
| `to_module` | `forbidden_dependency` | Module selector for the imported side, instead of `to`. |
| `rationale` | all | Why the rule exists. Appended to every finding's `why` as ` — <rationale>` and repeated in the repair task's `constraints`. |
| `alternatives` | all | What to do instead. Sets the finding's `allowed_alternatives`; the repair task repeats each one in `constraints`. |
| `docs` | all | A document reference, such as an ADR path. Appended to every finding's `why` and `constraint` as ` (see <docs>)`, so SARIF and the repair task carry it. |

`rationale`, `alternatives`, and `docs` never enter a finding ID, so editing
them re-keys no finding. Report text is bounded at projection, so a long
rationale is cut, never rejected.

```yaml
rules:
  - id: domain_no_http
    type: forbidden_dependency
    from_module: "layer:domain"
    to: net/http
    rationale: "Domain code stays free of I/O"
    alternatives: ["Depend on a port in the application layer"]
    docs: docs/adr/003.md
  # Two independent contexts ("separate ways"): one rule for each direction.
  - id: billing_separate_from_catalog
    type: forbidden_dependency
    from_module: billing
    to_module: catalog
  - id: catalog_separate_from_billing
    type: forbidden_dependency
    from_module: catalog
    to_module: billing
```

`forbidden_layer_direction` takes no `from`/`to` (or `from_layer`/`to_layer`)
keys — it derives layer ordering from `layers:` and each endpoint's layer from
the `modules:` map's `layer:` field, for every module pair in the graph. A rule
of this type needs only `id`, `type`, and `gate`. Declare **at most one** rule
of this type: the check is global, so a second instance re-reports every
violation under its own rule ID (`archfit config init` generates at most one).

`gate` controls how the rule blocks the run:

- `fail` (or absent) — finding blocks CI; exit 1. **Exceptions:**
  `public_api_change` and `public_api_type_leak` default to `warn` when `gate`
  is absent.
- `warn` — finding is advisory; surfaced without setting `hard_gates` to `fail`.
  `check` can still return exit `2` for the active diagnostic or other missing
  evidence; report-only `analyze` exits `0` after a successful run.
- `off` — rule is skipped entirely; no findings emitted.

`gate:` is wired for **all rule types**. An unknown `type` value is a config error.

### Selectors that match nothing

A rule whose selector matches no scanned source cannot find a violation, so it
is never counted as evaluated conformance. This applies to the selectors of
`forbidden_dependency`, `public_api_only`, and `internal_api_access`, and to
the `from:` selector of `forbidden_pattern`.

The dependency rules match graph edge endpoints, spelled per language. A
`to:` matches the target module node: a Go package directory, a TypeScript
file, a dotted Python module, or a Rust crate or `crate::mod` module. A `from:`
matches the importing file for Go and TypeScript, and the module node for
Python and Rust. So a Go file glob (`internal/domain/*.go`) never matches a
`to:`, a bare Go package directory never matches a `from:` (write
`internal/domain/**`), and a slash path never matches a Python or Rust
endpoint. A `forbidden_pattern` `from:` matches the file path or its node
selector.

A `crate::mod` selector is judged against the Rust module graph
(`analyzers.cargo_modules`): under a crate the graph covers, it is live or dead
like any other selector; under a crate the graph does not cover, it cannot be
judged and the rule stays unevaluated. A selector is dead when:

- a `from:` selector matches no in-scope source;
- a `to:` selector spelled as first-party source (it starts with a wildcard or
  with a top-level directory, Python package, or crate the tree contains) that
  matches nothing — usually a typo or a renamed directory. The first segment
  is read in the selector's own spelling: `go.uber.org/**` is an external ban
  even beside a top-level `go/` directory, and `app.**` is checked against
  Python packages;
- a selector spelled with the Go module path (`example.com/shop/internal/x`;
  rule selectors are scan-root-relative), a leading `./`, `../`, `/` or `!`, or
  an extglob negation `!(...)`, none of which a graph node ID can match.

A `to:` selector that names no first-party source, such as `net/http` or
`github.com/sirupsen/logrus`, is a ban on an external package: it is evaluated
normally. A `gate: fail` rule with a dead selector is listed in
`decision.unevaluated_required_rules` with the reason
`selector matches nothing: <from|to> <glob>`, which keeps `check` at exit `2`.
A `gate: warn` rule with one is reported as a config warning. `archfit config
lint` reports both as `dead_selector` and exits `1`.

A dependency-rule selector that matches only source no dependency producer
analyses (see rule scope above) is not a typo: the code is there, and nothing
reads its relationships. Such a rule is listed with the reason
`selector matches only source no dependency producer analyses: <from|to> <glob>`,
guard or not, and `config lint` reports it as `dead_selector` naming the
cause.

Set `guard: true` on a rule that is meant to match nothing, such as a ban on
re-introducing a deleted package. A guard counts as evaluated while either
selector matches nothing, whatever state the dependency producer is in: no
edge can start at, or reach, source that is not there, so a `partial`
dependency-cruiser run (normal on TypeScript) does not list it. Once the
guarded path exists again the guard is an ordinary rule: it is evaluated over
complete producer evidence, and listed in `decision.unevaluated_required_rules`
when that evidence is incomplete. `archfit config lint` lists a holding guard
as `guard_rule` and warns with `guard_matches_source` once the guarded path
exists again.

```yaml
rules:
  - id: no_legacy_reports
    type: forbidden_dependency
    from: internal/**
    to: internal/legacy/reports/** # deleted; must not come back
    gate: fail
    guard: true
```

### Built-in rule types

- `forbidden_dependency` — fires when an edge matches both `from` and `to`
  globs. Both globs are **required**: an empty glob matches nothing, ever
  (`doublestar.Match("", path)` is always false; there is no empty-means-match-all
  special case), so a rule missing either is rejected as a config error at load.
  Either side can take a [module selector](#module-selectors) instead:
  `from_module` for `from`, `to_module` for `to`. Each side takes exactly one
  of the two keys; both or neither is a config error. With a module selector:
  - a module side matches an endpoint whose declared module the selector
    selects; an endpoint no declared module owns never matches it;
  - an edge inside one declared module never matches;
  - one finding per pair of sides, with `kind: module_dependency`. A module
    side is keyed by its module (`edge.<side>.module`). A `from` glob side is
    keyed by the importing package (`edge.from.path`): a Go package directory,
    a TypeScript file, a Python dotted module, or a Rust crate. A `to` glob
    side is keyed by the target node (`edge.to.path`). So a new or moved file
    keeps the finding ID on a module side and inside a Go package; moving a
    TypeScript or Python importer, or the target of a `to` glob, re-keys it.
    The finding lists the import lines (at most 50; the full count is in
    `matched_by.locations_total`);
  - a selector that selects no declared module, or only modules that
    provably own no source, still loads. A module provably owns no source
    when no scanned file is in it and each of its paths is a source-file path
    that does not exist or a `crate::mod` path the module graph lacks. A
    fail-gated rule goes to `decision.unevaluated_required_rules` with the
    reason `selector matches nothing: from_module <selector>` (or
    `to_module`), a warn-gated rule prints a config warning, and
    `archfit config lint` reports `unknown_module` (no module selected) or
    `dead_selector` (the selected modules own no source). Selected modules
    that own only source no dependency producer analyses give
    `selector matches only source no dependency producer analyses: …`, as a
    path glob does. When the inventory cannot judge a selected module (a
    directory glob with no scanned file, a Rust package name without cargo
    metadata, a `crate::mod` path without the module graph), the rule stays
    unevaluated with the generic scope reason, as `module_cycle` does.
    `guard: true` exempts a selector that matches nothing, as for a path
    glob;
  - the rule's scope is the languages of the modules a `from_module` selects,
    restricted to the languages of the modules a `to_module` selects.
- `public_api_only` — fires on edges into internal surface, optionally filtered
  by `from` and `to`. The declared surfaces decide first, in every language:
  1. a target matching a `public` glob of the module that owns it never fires;
  2. a target matching any module's `internal` glob fires;
  3. only a target no declaration covers falls back to the extractor's
     internal-access edge kind: the Go `/internal/` path segment, or the
     TypeScript/Python extractors' own `internal` glob match. Rust has no
     fallback.

  Consults the `modules:` map: an edge where both endpoints resolve to the same
  module (e.g. `domain` importing its own `domain/internal`) is idiomatic
  same-module access and never fires. A Rust `crate::mod` node no `paths:` glob
  claims resolves to the module that declares its crate, so two modules of one
  crate are the same module and that module's `public:` globs apply. When either endpoint isn't covered by the
  module map, the edge still fires (module-blind fallback). A finding decided by
  a declaration names the glob in `matched_by.internal_glob`.

- `internal_api_access` — same internal-access signal, with a separate rule ID.
  Applies the same declared-surface precedence, module-map same-module skip,
  and module-blind fallback as `public_api_only`.
- `forbidden_layer_direction` — fires when a dependency direction violates the
  ordered `layers` list.
- `new_cross_module_dependency` — fires on cross-module edges. Baseline status
  separates known from new findings.
- `cycle` — fires once per node-level import cycle: a strongly-connected
  component of size > 1 over the dependency graph's own nodes (TypeScript
  files, Python dotted modules, Rust crates or `crate::mod` nodes). It is always
  silent on compiling Go, whose edges run file → package, and it never sees a
  cycle that closes across modules through different files. Use `module_cycle`
  for that.
- `module_cycle` — fires on dependency cycles among **declared modules**. It
  builds the module graph from classified dependency edges whose endpoints
  resolve to two different declared modules (unowned code, external targets,
  and auto-registered synthetic modules stay out), and emits one finding per
  ordered module pair inside a strongly-connected component: billing → shipping
  and shipping → billing are two findings, each located at its import lines
  (sorted, at most 50; the full count is in `matched_by.locations_total`).
  Removing one direction fixes that finding, and breaking the cycle fixes the
  rest. One cycle reports at most 200 pairs, the first in (from, to) order;
  every finding of the cycle carries the full count in
  `matched_by.cycle_pairs_total`, and a capped cycle's `why` says how many pairs
  it reports. The cap keeps a densely coupled cycle from growing the report
  quadratically. Its cost: once reported pairs are fixed, pairs past the cap
  appear as new findings that a baseline captured earlier does not cover.
  Module-cycle agent tasks carry no `declarations`; their locations name the
  import lines. The finding edge is `{module, path: ""}` on both sides with
  `kind: module_dependency`, and its ID is keyed on the rule ID and the ordered
  module pair, so a moved import keeps it. Takes no `from`/`to`: a scope glob is
  a config error. Needs at least two declared modules; otherwise it does not
  apply. Only **production** edges count, as for `forbidden_pattern`: an edge
  whose importing file is a test, generated or vendored file, or a file an
  `exclude:` glob or a switched-off language declares out of scope, never
  closes a cycle. A file the source walk skips (a dot directory such as
  `.storybook/`, or `target/`) but an analyzer still loads is classified from
  its path with the same `file_class` globs, so it follows the same rule. A Rust crate dependency (located at `Cargo.toml`) and a
  `crate::mod` edge have no importing file and count; dev-dependencies are
  left out by the extractor unless the config includes them. Story files,
  `.storybook/` and tool configs such as `vitest.config.ts` are production by
  default: classify them with `file_class.test_globs` to keep them out.
- `module_dependencies` — enforces the [module allowlists](#module-allowlists)
  (`depends_on`, `visible_to`). It emits one finding for each denied ordered
  module pair, also when both lists deny it; `matched_by.violates` names the
  list or lists that deny the pair (`depends_on`, `visible_to`, or
  `depends_on,visible_to`). The finding edge is `{module, path: ""}` on both
  sides with `kind: module_dependency`, and its ID is keyed on the rule ID and
  the ordered module pair, so a new or moved file on that pair keeps the ID. The
  finding lists the import lines (sorted, at most 50; the full count is in
  `matched_by.locations_total`). An importer that no declared module owns gets
  one finding for each importing package (a Go package directory, `.` at the
  repository root, a TypeScript file, a Python dotted module, or a Rust crate): `edge.from.path` names the
  package, `edge.from.module` is empty, and `matched_by.from_package` repeats
  it. Only production edges count, as for `module_cycle`. Takes no
  `from`/`to`: a scope glob is a config error. The rule's scope is the
  languages of the modules that declare a list, so a missing TypeScript
  analyzer does not hold back a rule whose lists are on Go modules. When no
  module declares either list, the rule has nothing to enforce: a fail-gated
  rule goes to `decision.unevaluated_required_rules` with the reason
  `selector matches nothing: no module declares depends_on or visible_to`, and
  `archfit config lint` reports it as `dead_selector`. A waiver matches the
  module names: `waivers: [{rule: boundaries, from: catalog, to: billing, ...}]`.
  The repair task never names the target module's public API: an import
  through it is still outside the allowlist.
- `public_api_max` — fires when any module's exported declaration count exceeds
  `max` (requires `analyzers.syntax.enabled: true`). Scoped per module. No baseline
  — static ceiling.
- `public_api_change` — emits one finding per exported declaration; baseline
  suppresses known ones so only newly-added surface shows as `new`. Defaults to
  `gate: warn`. Requires `analyzers.syntax.enabled: true`.
- `public_api_type_leak` — fires when an exported struct field or function return
  type names a type from an external (non-first-party) package (Go only; requires
  `analyzers.syntax.enabled: true`). Flags API surface that couples callers to a
  transitive dependency. Defaults to `gate: warn`.

**Note:** without syntax evidence, `public_api_max`, `public_api_change`, and
`public_api_type_leak` cannot establish conformance. Applicable fail-gated rules
appear in `decision.unevaluated_required_rules`; advisory rules disclose the
gap in the intent dimension.

Example syntax-facts rule:

```yaml
rules:
  # Warn when any module's exported API exceeds the ceiling.
  - id: api_size_ceiling
    type: public_api_max
    max: 200
    gate: warn

  # Surface newly-added public API (baseline suppresses known surface).
  - id: track_public_api
    type: public_api_change
    gate: warn

  # Warn when exported API leaks an external type to callers.
  - id: no_type_leak
    type: public_api_type_leak
    gate: warn
```

### `forbidden_pattern`

`forbidden_pattern` fires when source code under `from` contains a construct
one of its `patterns` matches. It is the only rule type that reads `patterns`.
Use it for logic that belongs in another module: a domain that reads the wall
clock, opens files, or reads the environment.

```yaml
rules:
  - id: domain_no_clock
    type: forbidden_pattern
    from: internal/domain/**
    gate: fail
    patterns:
      - id: clock
        lang: go
        rule: time.Now()
```

How it decides:

- `patterns` is required: each entry is an ast-grep pattern with an `id`, the
  ast-grep `lang`, and the pattern `rule`. Pattern IDs must be unique across
  **all** rules' patterns, because every rule's patterns run in one ast-grep pass
  and a match carries only its pattern ID.
- `from` is optional (empty means every production file). It matches the
  repo-relative file path, or the file's module selector (a dotted Python
  module such as `myapp.domain.**`). `to` is a config error.
- Only **production** files in the source inventory fire: test, generated, and
  vendored files never do (see
  [file classification](languages.md#file-classification-per-language); a Rust
  inline `#[cfg(test)]` block inside a production file is not separated and
  still fires), and neither do directories the source walk skips
  (`testdata`, `vendor`, `node_modules`, and similar), files an `exclude:` glob
  matches, or files of a language switched off with
  `languages.<id>.enabled: false` and no explicit `gate:`.
- Findings are one per pattern, file, and matched text (whitespace removed),
  located at every line of that text in the file (sorted, at most 50). The ID
  leaves the line out, so a moved or reformatted match keeps it. Findings name
  the pattern ID and `file:line`, and never carry the matched source text.
- The rule is evaluated only when the ast-grep pattern pass completed (`sg`
  present and every pattern run accepted). An absent `sg`, or a pattern run
  `sg` rejects (for example an unknown `lang`), leaves it in
  `decision.unevaluated_required_rules` instead of passing with zero findings.

Write and check patterns with `sg run --lang <lang> --pattern '<rule>' .`
before you gate on them. In Go, a call with exactly one argument, such as
`os.Getenv($KEY)`, parses as a type conversion and matches nothing; patterns
with no arguments (`time.Now()`) or with several (`fmt.Sprintf($F, $$$)`)
match calls as expected.

`patterns` on any other rule type is accepted for compatibility, but its
matches never produce a finding; `analyze` and `check` print a warning naming
the rule.

## `waivers`

Waivers accept a finding temporarily without deleting the rule.

```yaml
waivers:
  - rule: no_domain_to_http
    from: internal/domain/legacy/**
    to: internal/http/**
    reason: migration in progress
    approved_by: "@lead"
    expires: "2026-12-01"
```

Fields:

- `rule` — required declared rule ID or supported synthetic rule ID.
- `from` — source glob. Edge-backed rules require at least one of `from` or
  `to`; an empty selector pair does not create a global waiver.
- `to` — target glob. For module-only findings, selectors match module names.
- `reason` — required explanation of why the waiver exists.
- `approved_by` — required reviewer or owner metadata.
- `expires` — required `YYYY-MM-DD` expiry date, valid through that UTC day and
  expired at the following midnight UTC.

The edge-less diagnostics `map/uncovered_path`, `map/dead_rule`, and
`map/stale_review` are scoped by their exact rule ID and must omit `from` and
`to`. Other supported synthetic waivers are `bc/imbalanced_coupling`,
`bc/duplicated_knowledge`, and `labels/stale`; these require endpoint scope.
`bc/coupling_gate` cannot be waived. A change to that gate is a separate,
owner-approved coupling policy decision. The `approved_by` string records
metadata; the CLI does not authenticate the approver.

Expired waivers are reported as `expired_waiver`. Active waivers show finding
status `waived`. Waivers require a reason, approver, and expiry — they are
deliberate human friction, not a quiet ignore list. Rule references, selectors,
glob syntax, and all required metadata are validated while loading the config;
an invalid waiver is a configuration error (exit `3`) before analysis starts.
When several waivers match a finding, matching is independent of YAML order: an
active match wins, and an expired match is used only when no active match
applies.

`archfit baseline` does not turn temporary exceptions into permanent debt:
findings covered by temporary waivers, including expired waivers, are omitted
from the accepted set and the command prints the number skipped. Review the
full capture before committing it. A measurement-profile mismatch or an
incomplete stored seam snapshot makes the baseline `gate_reference`
non-comparable; do not treat a blanket re-baseline as the migration for that
condition.

## `metrics`

Built-in metric names:

- `encapsulation` — ratio of contract cross-boundary edges to classified
  cross-boundary edges.
- `unbalanced_edge` — count of new high-risk intrusive, volatile edges across
  larger boundaries.
- `cycle` — node-level import cycle count (always `0` on compiling Go; see the
  `module_cycle` rule for cycles among declared modules).
- `coverage` — extracted files over applicable files, with confidence lowered by
  unresolved imports.

Report-only metrics (band `info`; they never gate the verdict):

- `blast_radius` — modules whose transitive reverse-dependency reach is a large
  share of the codebase.

`coupling_balance` is not a `metrics:` entry, and a `metrics.coupling_balance:`
key is a config error. It is a diagnostic that never gates; the only coupling
gate is [`coupling.gate.distributed_monolith`](#couplinggate).

Metric entry fields:

```yaml
metrics:
  function_loc_threshold: 60 # diagnostic only; positive integer
  encapsulation:
    enabled: true
    gate: warn
    min_delta: 0
  unbalanced_edge:
    enabled: true
    gate: fail
    max_new: 0
  cycle:
    enabled: true
    gate: fail
    max_new: 0
  coverage:
    enabled: true
    gate: warn
    min_delta: 0
```

- `function_loc_threshold` — positive integer, default `60`. Counts functions
  and methods whose inclusive LOC is above the threshold. It is an out-of-claim
  complexity diagnostic, not a metric gate and never changes promotion.
- `enabled` — `false` removes the metric from the run. Metrics absent from the
  config default to enabled, as do knob-only entries (e.g. just `gate: warn`) —
  only an explicit `enabled: false` disables.
- `gate` — what a baseline regression does to the verdict: `off` skips the
  check, `warn` caps at WARN (exit 2), `fail` or unset blocks (exit 1) — the
  same convention as rule gates.
- `max_new` — count metrics only (`cycle`, `unbalanced_edge`): the allowed
  increase over the baseline value before the gate trips. Default 0: any new
  occurrence trips.
- `min_delta` — ratio metrics only (`encapsulation`, `coverage`): the tolerated
  drop below the baseline value before the gate trips. Default 0: any drop
  trips.

Setting a knob on a metric of the wrong kind (e.g. `min_delta` on `cycle`) is
a config error, not a silent no-op. `blast_radius` is informational and never
gates — it accepts only `enabled`. Metric gates fire only against a baseline
(`.archfit-baseline.json`); without a stored value for the metric there is no
delta and nothing to trip.

## `module_review`

`module_review` turns on the review of the module declarations:

```yaml
module_review:
  stale_after: 2160h
  gate: fail
```

The review gives three findings:

| Rule ID              | Finding                                                         | Can block |
| -------------------- | --------------------------------------------------------------- | --------- |
| `map/uncovered_path` | A directory holds production source that no module owns.        | Yes       |
| `map/dead_rule`      | A module `paths` glob matches no graph node.                    | No        |
| `map/stale_review`   | A module `reviewed_at` is older than `stale_after`.             | No        |

`gate` sets how `map/uncovered_path` affects the verdict:

- `fail` makes each finding a blocker, so `check` exits `1`. A finding that the
  baseline accepts does not block. A new package outside every module blocks.
- `warn`, or no `gate` with `stale_after` set, makes each finding a diagnostic.
- `off` turns the review off.

`map/dead_rule` depends on a complete dependency graph, and `map/stale_review`
depends on the clock, so both stay diagnostics with `gate: fail`.

`map/uncovered_path` reads the files that `check` walks, not the dependency
graph, so a package that failed to load is still checked. A file counts only
when all of these are true:

- Its file class is production. Test, generated, and vendor files do not
  count.
- It is in the analysis scope: no `exclude:` glob matches it, and its language
  is not switched off.
- A dependency producer reads it. For example, TypeScript with no root
  `package.json` does not count, and neither does a Go member that
  `languages.go.modules` removes.

A module owns a file when one of its `paths` globs matches the file path, or
the file's node ID: the package directory for Go, the dotted module for Python,
or the crate for Rust. A Rust file whose crate is unknown (no `cargo metadata`)
is not checked.

There is one finding for each directory. Its ID comes from the rule ID and the
directory, so it does not change when files are added. The finding has these
`matched_by` keys:

- `subject`: the directory.
- `uncovered_files`: the number of unowned files in it. `locations` lists at
  most five of them.
- `suggested_path`: a `paths:` glob that owns every unowned file in the
  directory, when one glob can do it. A bare directory owns only a Go
  package. TypeScript and Rust need a glob over the files (`web/src/**`),
  Python needs a dotted glob (`acme.ops.**`), and a root directory needs a
  glob over the extension (`*.go`).

A run reports every unowned directory, in path order. There is no limit on
the count, so `archfit baseline` accepts all of them and a new directory is
never hidden. The repair task asks the
architecture owner which module owns the directory
(`repair_kind: needs_owner_decision`) and names `suggested_path`. In a delta
run, a change to any file directly in the directory touches the finding.

`stale_after` uses Go duration syntax. Use `2160h` for 90 days.

## `file_class`

Overrides per-project file classification patterns for `Production`, `Test`,
`Generated`, and `Vendor`. Auto-detection runs first; `file_class` adds
project-specific patterns on top.

```yaml
file_class:
  generated_globs:
    - "**/gen/**"
    - "**/*.generated.go"
  test_globs:
    - "**/*_test.go"
  mock_frameworks:
    - "moq"
```

See [Language support → File classification](languages.md#file-classification-per-language)
for the full policy and per-language defaults.

## `outputs`

```yaml
outputs:
  json: true
  markdown: false
  sarif: false
```

The config accepts these fields. The current CLI selects output with command-line
flags:

```sh
archfit analyze --format text
archfit analyze --format json
archfit analyze --format markdown
archfit analyze --format sarif
```

`--format` is repeatable: `--format json --format sarif` writes both to stdout.
Shorthands `--json`, `--markdown`, and `--sarif` are mutually exclusive
alternatives to `--format`.

## Glob tips

- Use repo-relative paths.
- Prefer explicit `**` globs, such as `internal/domain/**`.
- Keep module names stable; baselines and waivers refer to rule IDs and
  finding fingerprints.
- For Python, see [Language support](languages.md#python) before writing globs.

## Editor support

A JSON schema (`archfit.schema.json`) ships at the repository root for YAML
editor autocomplete and validation. It enumerates the allowed values of
closed fields such as `rules[].type` ([built-in rule types](#built-in-rule-types)),
so an editor flags a typo before `archfit` rejects the config. Point your editor at it with a YAML language
server comment:

```yaml
# yaml-language-server: $schema=./archfit.schema.json
version: 2
```

VS Code users can configure the schema in `.vscode/settings.json`:

```json
{
  "yaml.schemas": {
    "./archfit.schema.json": ".archfit.yaml"
  }
}
```

## Draft and pin files

LLM authoring commands are draft-first. They write proposals to review files,
side files, or reports by default; `config init --ai-classify --apply` is the direct-write
exception and should be reviewed before the generated config is used as a gate.

- `.archfit-labels.yaml` — pinned coupling-strength labels (`archfit config enrich labels`).
  `analyze` consumes `status: approved` entries with precedence: config
  public/internal globs > approved labels > extractor hint.
- `.archfit-owners.yaml` — owner drafts (`archfit config enrich owner`).
- `.archfit-volatility.yaml` — volatility drafts (`archfit config enrich volatility`).
- `.archfit-subdomains.yaml` — subdomain drafts (`archfit config enrich subdomain`).
- `.archfit-init-llm.yaml` — a full commented config draft (`archfit config init --ai-classify -o <file>`).

For module-field draft files, review each entry, set keepers to `status: approved`,
then run `config enrich <field> --apply` to write approved values into
`modules.<name>`. For a full `config init --ai-classify` side file, copy approved fields
manually. Pinning never overwrites a live field.

Module-field draft entries include review metadata:

```yaml
- module: payments
  value: "@team-payments" # or subdomain: core / volatility: high
  rationale: "doc:README.md describes Payments as the core domain"
  evidence_refs:
    - doc:README.md
    - api:payments
  basis: semantic_judgment # deterministic_fact | semantic_judgment
  status: draft
```

`config update --ai-classify` uses the same metadata in its report and can propose only
existing deterministic rule mechanisms (`forbidden_dependency`,
`forbidden_role_dependency`, `public_api_max`, `public_api_change`, and
`coupling.gate` tuning). These rule suggestions are review-only text; plan/default
mode never mutates `.archfit.yaml`, and unsupported or uncited suggestions are
rejected before a draft is written. See [llm-enrich.md](llm-enrich.md).

### Label `confidence` and `provenance` fields

Each label entry in `.archfit-labels.yaml` may carry two optional fields:

```yaml
- from: internal/evidence/acquisition
  to: internal/relationship
  strength: functional
  status: approved
  confidence: medium # high | medium | low
  provenance: llm # human | llm | tool
  reviewed_by: "@architect"
  reviewed_at: "2026-06-23"
```

- `provenance` — source of the strength judgment: `human` (direct human
  decision), `llm` (drafted by `archfit config enrich`, then human-approved), or
  `tool` (deterministic extractor hint).
- `confidence` — how certain the judgment is: `high`, `medium`, or `low`.

**Effect on scoring:** when an approved label has `provenance: llm` and
`confidence` below `high`, `coupling_balance` confidence is lowered by one
band. Rationale: LLM drafts have been human-reviewed, but they are not as
certain as a config-glob or SCIP symbol-kind classification. `provenance:
human` and `provenance: tool` do not lower confidence.

`analyze` reads only `status: approved` labels. Draft labels (no `status` or
`status: draft`) are never consumed by the gate.

### Abstain and decision tasks

When an edge's strength cannot be classified (`unknown`) but its distance
resolves to an internal module, archfit **abstains** — the edge is excluded
from the `coupling_balance` scored distribution (honest denominator) and an
actionable config warning is emitted in text and Markdown, prompting the
operator to add a label. The same happens for modules with no declared
`subdomain` or `volatility`: a decision message asks for a declaration or
suggests `archfit config enrich subdomain`.

`agent_tasks[]` is reserved for active gate findings that need code repair.

External/library edges (`Distance == DistanceUnknown`, i.e. stdlib,
third-party packages, undeclared imports) are excluded from
`coupling_balance` entirely — they are not internal coupling seams. Their
count is visible in `classified_edges.external` and the `coupling_balance`
evidence string. Review-only `distance_config_candidates` may also surface
stable external targets from that excluded bucket so a human can decide whether
an `external_systems` entry should promote a real seam into the scored model.
