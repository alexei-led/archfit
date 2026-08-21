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
version         — required; must be 1
exclude         — path globs to skip during scanning
languages       — per-language extractor settings (go/typescript/python/rust)
analyzers       — opt-in deeper analysis backends (syntax/scip/clones/cargo_modules)
ai              — off-gate AI provider for enrich/explain/analyze --ai-summary
coupling        — Balanced-Coupling advisory tuning + coupling_balance gate
layers          — ordered architecture layers, inner to outer
modules         — path ownership map
external_systems — declared external integration seams scored at D=10
rules           — executable architecture constraints
waivers         — approved temporary deviations from rules
metrics         — metric policy settings
module_review   — staleness gating of the module declarations
file_class      — file-class override patterns
outputs         — output format preferences
```

## Complete small example

```yaml
version: 1

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
      - internal/domain
    internal:
      - internal/domain/internal/**
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

## `languages`

Per-language extractor settings. Each language has `enabled` and `gate`; some
have extra fields.

`enabled` accepts three values only — `true`, `false`, or `auto`. The legacy
string spellings `"on"` and `"off"` are a **hard error** in this schema.

- `true` — require the adapter; missing project markers or tools are errors.
- `false` — skip the adapter entirely.
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
(see [commands](commands.md#coverage-gaps-and-required-tools)). By default this is
**warn-loud** — surfaced, but exit 0. Set a per-analyzer `gate` to make CI block
on the missing tool:

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

Balanced-Coupling advisory tuning and the `coupling_balance` gate.

```yaml
coupling:
  min_severity: medium # low | medium (default) | high | critical
  duplicated_knowledge: score # score (default) | advisory
  volatility_cascade: false
  gate:
    min_band: mixed # band floor: poor | mixed | serviceable | strong
    max_drop: 5 # tolerated coupling_balance point drop vs the baseline snapshot
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

Makes the synthesised `coupling_balance` score able to fail the run. Without
this block the score is report-only — the default, and backward compatible.
The block requires `min_band` and/or `max_drop`; an empty `gate:` is a config
error (it would gate nothing).

- `min_band` — band floor. Trips when the overall `coupling_balance` band
  ranks below the floor: one of `poor`, `mixed`, `serviceable`, `strong`.
  `critical` is rejected — no band ranks below it, so it could never trip.
- `max_drop` — tolerated point drop of the `coupling_balance` value against
  the score snapshot stored by `archfit baseline`. `0` means any drop trips.
  A baseline written before the snapshot existed, or while the score was
  unmeasured, carries no anchor — the drop check is skipped, never guessed.
  A snapshot recorded under a different scorer version (`score_version`) or a
  different scorecard rubric (`rubric_version`) also carries no anchor: a
  methodology change is not a regression. A snapshot written before rubric
  tracking is read as rubric `1` — the only rubric shipped so far — so it keeps
  anchoring. The skip is disclosed on stderr and names the incompatible input —
  re-run `archfit baseline` to re-anchor.

An unmeasured score (band `n/a`) never trips the gate, whatever the knobs say:
abstention is not failure. Only the unmeasured case is exempt — a run with
partial tool coverage that still produces a measured score (for example TS
with some unresolved imports, confidence capped at `medium`) gates like any
other measured run.

When the gate trips, the verdict becomes FAIL (exit 1), the trip reasons print
to stderr, and the active `bc/imbalanced_coupling` advisories are promoted to
blocking findings — they then flow into `agent_tasks[]` like any other gate
finding. Baselined and waived advisories stay triaged. When no advisory is
available to promote (advisory output disabled, or `coupling.min_severity`
filtering every active edge), the run emits one synthetic `bc/coupling_gate`
finding carrying the trip reasons instead, so the report and `agent_tasks[]`
always explain the failure.

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
    internal: [services/pricing/internal/**]
    layer: domain
    subdomain: core
    volatility: high
    owner: team-pricing
    deploy_unit: pricing-service
```

Fields:

- `paths` — globs that claim files, packages, or modules for ownership.
- `public` — allowed public API surface. Matching targets are classified as
  contract coupling.
- `internal` — private surface. Matching targets are classified as intrusive
  coupling or internal API access when the extractor emits that edge kind.
- `layer` — one of the names from `layers`.
- `subdomain` — DDD subdomain classification: `core`, `supporting`, or `generic`.
  Determines the volatility ordinal when no explicit `volatility` is set.
- `volatility` — explicit override: `high` (=10), `medium` (=6), `low` (=3),
  or `frozen` / `legacy` (=1). Use `subdomain` unless you need a specific value
  that differs from the DDD default.
- `owner` — team or person responsible for the module.
- `deploy_unit` — deployable/runtime unit used for distance classification.
- `role` — optional architectural role. See [Module role vs layer](#module-role-vs-layer).
- `reviewed_at` — date of last architecture-map review.
- `reviewed_by` — reviewer identity.

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

| Field      | Applies to       | Description                                                                                  |
| ---------- | ---------------- | -------------------------------------------------------------------------------------------- |
| `id`       | all              | Stable ID used in findings, baselines, and waivers.                                          |
| `type`     | all              | Built-in rule type (see below). Unknown type is a config error.                              |
| `gate`     | all              | `fail` (or absent for most types), `warn`, or `off`. `public_api_change` defaults to `warn`. |
| `from`     | most             | Source module or path glob.                                                                  |
| `to`       | most             | Target module or path glob.                                                                  |
| `max`      | `public_api_max` | Integer ceiling.                                                                             |
| `patterns` | structural rules | Optional ast-grep patterns for structural evidence.                                          |

`forbidden_layer_direction` takes no `from`/`to` (or `from_layer`/`to_layer`)
keys — it derives layer ordering from `layers:` and each endpoint's layer from
the `modules:` map's `layer:` field, for every module pair in the graph. A rule
of this type needs only `id`, `type`, and `gate`. Declare **at most one** rule
of this type: the check is global, so a second instance re-reports every
violation under its own rule ID (`archfit config init` generates exactly one).

`gate` controls how the rule blocks the run:

- `fail` (or absent) — finding blocks CI; exit 1. **Exception:** `public_api_change`
  defaults to `warn` when `gate` is absent.
- `warn` — finding is advisory; surfaced but exit 0.
- `off` — rule is skipped entirely; no findings emitted.

`gate:` is wired for **all rule types**. An unknown `type` value is a config error.

### Built-in rule types

- `forbidden_dependency` — fires when an edge matches both `from` and `to`
  globs. Both globs are **required**: an empty glob matches nothing, ever
  (`doublestar.Match("", path)` is always false; there is no empty-means-match-all
  special case), so a rule missing either is rejected as a config error at load.
- `public_api_only` — fires on internal-access edges, optionally filtered by
  `from` and `to`. Consults the `modules:` map: an edge where both endpoints
  resolve to the same module (e.g. `domain` importing its own
  `domain/internal`) is idiomatic same-module access and never fires. When
  either endpoint isn't covered by the module map, the edge still fires
  (module-blind fallback).
- `internal_api_access` — same internal-access signal, with a separate rule ID.
  Applies the same module-map same-module skip and module-blind fallback as
  `public_api_only`.
- `forbidden_layer_direction` — fires when a dependency direction violates the
  ordered `layers` list.
- `new_cross_module_dependency` — fires on cross-module edges. Baseline status
  separates known from new findings.
- `cycle` — fires once per detected import cycle.
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

**Note:** when `analyzers.syntax.enabled` is not `true`, the rule types
`public_api_max`, `public_api_change`, and `public_api_type_leak` emit zero
findings silently — they are not errors.

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

- `rule` — rule ID.
- `from` — source glob.
- `to` — target glob.
- `reason` — why the waiver exists.
- `approved_by` — reviewer or owner.
- `expires` — expiry date.

Expired waivers are reported as `expired_waiver`. Active waivers show finding
status `waived`. Waivers require a reason, approver, and expiry — they are
deliberate human friction, not a quiet ignore list.

## `metrics`

Built-in metric names:

- `encapsulation` — ratio of contract cross-boundary edges to classified
  cross-boundary edges.
- `unbalanced_edge` — count of new high-risk intrusive, volatile edges across
  larger boundaries.
- `cycle` — import cycle count.
- `coverage` — extracted files over applicable files, with confidence lowered by
  unresolved imports.

Report-only metrics (band `info`; they never gate the verdict):

- `blast_radius` — modules whose transitive reverse-dependency reach is a large
  share of the codebase.

`coupling_balance` is not a `metrics:` entry — the synthesised score gates
through the [`coupling.gate`](#couplinggate) block.

Metric entry fields:

```yaml
metrics:
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

`module_review` enables staleness gating of the module declarations:

```yaml
module_review:
  stale_after: 2160h
  gate: warn
```

Checks include:

- graph nodes not covered by any module `paths` glob;
- module `paths` globs that match no graph nodes;
- modules whose `reviewed_at` is older than `stale_after`.

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
editor autocomplete and validation. Point your editor at it with a YAML language
server comment:

```yaml
# yaml-language-server: $schema=./archfit.schema.json
version: 1
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
- from: internal/engine
  to: internal/classify
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
actionable `config_warnings[]` decision message is emitted in JSON/Markdown
output prompting the operator to add a label. The same happens for modules with
no declared `subdomain` or `volatility`: a decision message asks for a
declaration or suggests `archfit config enrich subdomain`.

`agent_tasks[]` is reserved for active gate findings that need code repair.

External/library edges (`Distance == DistanceUnknown`, i.e. stdlib,
third-party packages, undeclared imports) are excluded from
`coupling_balance` entirely — they are not internal coupling seams. Their
count is visible in `classified_edges.external` and the `coupling_balance`
evidence string. Review-only `distance_config_candidates` may also surface
stable external targets from that excluded bucket so a human can decide whether
an `external_systems` entry should promote a real seam into the scored model.
