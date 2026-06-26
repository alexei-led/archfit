# Configuration reference

`archfit` reads `.archfit.yaml` from the repository root by default.

The config parser is strict. Unknown YAML fields are errors. Start with
`archfit init`, then edit the generated file.

```sh
archfit init --root . --output .archfit.yaml
archfit check --config .archfit.yaml --full
```

## Top-level fields

- `version` — required positive integer. Current schema version is `1`.
- `bc_advisory_min_severity` — minimum Balanced Coupling advisory severity to
  show: `low`, `medium`, `high`, or `critical`. Default is `medium` when no
  config exists.
- `python_package` — Python top-level package passed to the Python extractor.
- `tools` — language adapter modes.
- `layers` — ordered architecture layers, inner to outer.
- `modules` — path ownership map.
- `rules` — executable architecture constraints.
- `exclusions` — repo-relative globs to skip during extraction.
- `exceptions` — temporary accepted rule findings.
- `metrics` — metric policy settings.
- `map_review` — architecture map staleness advisories.
- `outputs` — parsed output preferences. CLI `--format` is the current output
  selector.

## Complete small example

```yaml
version: 1
bc_advisory_min_severity: medium
python_package: myapp

exclusions:
  - "**/generated/**"
  - "**/*.pb.go"

tools:
  go:
    enabled: auto
  typescript:
    enabled: off
  python:
    enabled: on

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
    volatility: medium
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

exceptions:
  - rule: domain_no_http
    from: internal/domain/legacy/**
    to: internal/http
    reason: migration in progress
    approved_by: "@lead"
    expires: "2026-12-01"

map_review:
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

## `tools`

Use one key per supported language:

```yaml
tools:
  go:
    enabled: auto
  typescript:
    enabled: auto
  python:
    enabled: auto
```

`enabled` accepts `auto`, `on`, `off`, `true`, or `false`.

- `auto` — use the adapter when project markers and tools are found.
- `on` — require the adapter; missing markers or tools are errors.
- `off` — skip the adapter.
- `true` means `on`; `false` means `off`.

See [Language support](languages.md) for per-language setup and
[Tooling reference](tooling.md) for platform-specific install commands, versions,
home pages, and PATH checks.

### Opt-in analysis tools

Three additional tools are **off by default** and feed the report-only metrics. They
are opt-in because they are slower and/or need extra binaries:

```yaml
tools:
  scip:
    enabled: on # symbol-level analysis (SCIP indexers + uv); powers risk_hub
  clones:
    enabled: on # clone detection (jscpd); powers functional_candidates
```

- `scip` — runs a SCIP indexer (`scip-go`/`scip-python`/`scip-typescript`) plus `uv`
  to build the symbol graph. Required for `risk_hub`; without it `risk_hub` is `n/a`.
- `clones` — runs `jscpd` to find duplicated logic. Required for
  `functional_candidates`; without it that metric is `n/a`.

`scip` and `clones` are opt-in: each enables only with `on`.
`auto`, `off`, or absent all disable it, and the dependent metric then reports
`n/a` without ever failing the run.

### `tools.syntax` (syntax-facts provider)

`tools.syntax` runs the ast-grep adapter to extract declaration-level facts for
Go, TypeScript, Python, and Rust. Unlike the dependency extractors, which answer
_who imports whom_, `tools.syntax` answers _what declarations exist_ — exported
names, kinds, framework routes, and architectural roles.

Opt-in only: `auto` and `off` (and absent) all disable it. Enable with `on`:

```yaml
tools:
  syntax:
    enabled: on
```

**What it does:**

- Emits a `syntax_facts` block in the diagnostic (neutral, off-gate, omitted
  when empty).
- Each fact records: `language`, `file`, `kind` (function/method/class/struct/
  interface/trait/enum/type_alias/annotation/route/unsafe_op/struct_field/panic_op/global_state/type_leak/lazy_import/test_fn), `name`, `exported`, `role`
  (handler/service/repository/domain), `role_confidence` (high/medium/low),
  `role_evidence`, `framework`, `start_line`, `end_line`.
- Role derivation (`internal/syntax`) runs heuristics on name patterns,
  annotations, and framework evidence.
- The `scan` output gains a **Syntax surface** section listing declaration counts,
  roles, and public API totals per module.
- `agent_tasks` evidence is enriched with per-node declaration counts when
  syntax facts are present.

**Supported languages:** Go, TypeScript, Python, Rust. Requires `sg` (ast-grep)
on PATH; `archfit doctor` checks it.

**No new binary dependency:** `sg` is already required for ast-grep pattern rules
and is bundled in the runtime image. No CGO; the static binary is unchanged.

### `tools.<x>.gate` (coverage gate)

When an analyzer is **absent** (tool not installed or not found), its metrics drop
to `n/a` and a coverage gap is reported with an install hint
(see [commands](commands.md#coverage-gaps-and-required-tools)). By default this is
**warn-loud** — surfaced, but exit 0. Set a per-tool `gate` to make CI block on
the missing tool:

When an analyzer is **disabled by config** (`enabled: off`), it is simply skipped
— no coverage gap is emitted and no install prompt is shown. Disabled-by-config
is distinct from absent: a tool you deliberately turned off should not appear as
a gap to resolve.

```yaml
tools:
  go:
    enabled: on
    gate: fail # block CI when the go/packages analyzer is missing
  python:
    enabled: auto
    gate: warn # default: surface the gap, do not fail
```

`gate` accepts `off`, `warn` (default), or `fail`. A `fail` gate exits `1` (a
policy violation, distinct from exit `3` tool errors). The tool key governs the
analyzer behind it: `go` → go/packages, `typescript` → dependency-cruiser,
`python` → grimp, `complexity` → lizard, `clones` → jscpd.
The `--require-tools` flag on `check`/`scan` is the run-level shortcut — it raises
**every** gap to `fail` without editing config.

## `layers`

Layers are ordered from inner to outer.

```yaml
layers: [domain, application, adapter]
```

The `forbidden_layer_direction` rule treats dependencies from an earlier layer to
a later layer as violations. With the example above, `domain -> adapter` is a
violation, while `adapter -> domain` is allowed.

### Worked example: closing a layer-direction gap (Cat 10)

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
Without the rule, archfit has no way to know the intent — `forbidden_layer_direction`
fires only on _declared_ rules.

**How to get there without authoring manually:** `archfit enrich` can propose a
layer structure and `forbidden_layer_direction` rules from the module graph; draft
the output, review, then move approved entries into `.archfit.yaml`. See
[LLM enrichment](llm-enrich.md).

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
- `subdomain` — `core`, `supporting`, or `generic`. Controls the volatility
  ordinal used by the book scorer: `core`→10, `supporting`/`generic`→3.
- `volatility` — optional explicit override: `high` (=10), `medium` (=6), or
  `low` (=3). Overrides the subdomain-derived value. Use `subdomain` unless you
  need a different number than the DDD default.
- `owner` — team or person responsible for the module.
- `deploy_unit` — deployable/runtime unit used for distance classification.
- `role` — optional architectural role:
  `composition_root`, `adapter`, `core`, `shared_model`, `generated`, or `test`.
  See [module roles](#module-role) below.
- `reviewed_at` — date of last architecture-map review.
- `reviewed_by` — reviewer identity.

Balanced Coupling classification uses this metadata:

- target `public` match -> `contract` strength;
- target `internal` match -> `intrusive` strength;
- `volatility` or `subdomain` -> target volatility.

Distance is a **composite** of three signals, not a single-winner precedence chain:

1. **Code structure** — always-available baseline. Sibling or parent-child packages
   (shared subtree) → `cross_module_same_owner`; different subtrees or unrelated
   flat (single-segment) names → `cross_module_different_owner`.
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
4. otherwise → code structure decides (shared subtree → `cross_module_same_owner`;
   different subtrees or unrelated flat names → `cross_module_different_owner`).

A detected runtime async bridge is recorded as report-only evidence in the
`runtime_async` JSON field per module; it does not annotate graph edges, does not
affect distance or score, and does not change the gate verdict.

The `distance_basis` field on each advisory edge (`code_structure`, `ownership`,
or `deploy_unit`) shows which signal drove the composite, so the result is auditable.

> **Small-OSS note:** a repo with one maintainer is not a flat distance space.
> Code structure is the baseline and still distinguishes close vs far modules.
> Ownership only contributes when there are genuinely distinct owners to compare.

### Module role

`role` refines Balanced-Coupling distance classification for modules that are
_supposed_ to fan out. In a one-binary CLI, the `cmd` package wires every adapter
together — that is composition-root cohesion, not high-distance coupling. Without
a role, archfit scores those outbound edges as unbalanced and emits
false-positive advisories.

```yaml
modules:
  cli:
    paths: [cmd/archfit/**]
    role: composition_root
```

Accepted values:

- `composition_root` — wiring/entrypoint that legitimately depends on many
  modules (e.g. `cmd`, `main`).
- `generated` — generated code (its fan-out is mechanical, not designed).
- `test` — test-support code.
- `adapter`, `core`, `shared_model` — descriptive; reserved for future
  refinement.

For a `composition_root`, `generated`, or `test` source module, archfit downgrades
its outbound cross-deploy / different-owner edges to cross-module-same-owner, so
the advisory severity, the continuous score, and every distance-reading metric
read cohesion. A `core -> core` unbalanced edge is **still** flagged, and inbound
edges to a wiring module are unaffected. `init --llm` and `autopilot` suggest a
role per module; review before pinning.

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

Rule fields:

- `id` — stable ID used in findings, baselines, and exceptions.
- `type` — built-in rule type. An unrecognised type is a config error.
- `from` — optional source glob.
- `to` — optional target glob.
- `from_layer`, `to_layer` — layer names for `forbidden_layer_direction`.
- `from_role`, `to_role` — role names for `forbidden_role_dependency`.
- `min_confidence` — minimum role confidence to match (`high` by default;
  set to `medium` to relax). Applies to `forbidden_role_dependency`.
- `max` — integer ceiling for `public_api_max`.
- `gate` — controls how the rule blocks the run:
  - `fail` (or absent) — finding blocks CI; exit 1. **Exception:** `public_api_change` defaults to `warn` when `gate` is absent (see below).
  - `warn` — finding is advisory; surfaced but exit 0.
  - `off` — rule is skipped entirely; no findings emitted.
- `patterns` — optional ast-grep patterns for structural evidence.

Built-in rule types:

- `forbidden_dependency` — fires when `from` and `to` match an edge.
- `public_api_only` — fires on internal-access edges, optionally filtered by
  `from` and `to`.
- `internal_api_access` — same internal-access signal, with separate rule ID.
- `forbidden_layer_direction` — fires when dependency direction violates the
  ordered `layers` list.
- `new_cross_module_dependency` — fires on cross-module edges. Baseline status
  separates known from new findings.
- `cycle` — fires once per detected import cycle.
- `forbidden_role_dependency` — fires when an edge goes from a node with
  `from_role` to a node with `to_role` (requires `tools.syntax.enabled: on`).
  Only matches edges where both roles are assigned at or above `min_confidence`
  (default `high`). Example: handlers must not call repositories directly.
- `public_api_max` — fires when any module's exported declaration count exceeds
  `max` (requires `tools.syntax.enabled: on`). Scoped per module using the
  module path map. No baseline — static ceiling.
- `public_api_change` — emits one finding per exported declaration; baseline
  suppresses known ones so only newly-added surface shows as `StatusNew`.
  Defaults to `gate: warn` (advisory drift signal). Requires
  `tools.syntax.enabled: on`.
- `struct_field_max` — fires when any module's struct definition has more fields
  than `max` (Go and Rust; requires `tools.syntax.enabled: on`). Surfaces
  god-struct candidates. `max` is required. Defaults to `gate: warn`.
- `public_api_type_leak` — fires when an exported struct field or function return
  type names a type from an external (non-first-party) package (Go only;
  requires `tools.syntax.enabled: on`). Flags API surface that couples callers to
  a transitive dependency. Defaults to `gate: warn`. TypeScript and Rust type-leak
  patterns require different grammar rules — extend in a future task.

  **Struct field ceiling:** covers direct (`pkg.Type`) and pointer (`*pkg.Type`)
  field types. Does not match slice (`[]pkg.Type`), map, generic (`pkg.Type[T]`), or
  embedded (`pkg.Type` without a name) forms — these are structurally complex in the
  Go tree-sitter grammar and represent a smaller share of real cases. The LLM/human
  path covers the residue.

  **Function return ceiling:** covers single-return, pointer-return, and the common
  multi-result tuple (`(pkg.Type, error)`, `(*pkg.Type, error)`). Does not match
  slice/map/generic returns inside a multi-result tuple. This is an accepted
  ast-grep structural precision ceiling for a candidate-surfacer (bias toward false
  positives, never false negatives).

**Note:** When `tools.syntax.enabled` is not `on`, the rule types `forbidden_role_dependency`, `public_api_max`, `public_api_change`, `struct_field_max`, and `public_api_type_leak` emit zero findings silently — they are not errors.

**`gate:` is now wired for all rule types.** Previously `gate:` was stored but
not applied — that latent bug is fixed. Every rule respects `off`/`warn`/`fail`
regardless of type. An unknown `type` value is now a config error (not silently
ignored).

Example syntax-facts rules:

```yaml
rules:
  # Handlers must not call repositories directly.
  - id: no_handler_to_repo
    type: forbidden_role_dependency
    from_role: handler
    to_role: repository
    gate: warn # advisory; set to fail once roles are stable

  # Warn when any module's exported API exceeds the ceiling.
  - id: api_size_ceiling
    type: public_api_max
    max: 200
    gate: warn

  # Surface newly-added public API (baseline suppresses known surface).
  - id: track_public_api
    type: public_api_change
    gate: warn

  # Warn on structs with more than 30 fields (god-struct candidate).
  - id: no_god_struct
    type: struct_field_max
    max: 30
    gate: warn

  # Warn when exported API leaks an external type to callers.
  - id: no_type_leak
    type: public_api_type_leak
    gate: warn
```

## `exceptions`

Exceptions accept a finding temporarily without deleting the rule.

```yaml
exceptions:
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
- `reason` — why the exception exists.
- `approved_by` — reviewer or owner.
- `expires` — expiry date.

Expired exceptions are reported as `expired_exception`.

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
- `change_amplification` — blast radius weighted by recent churn.
- `hidden_coupling` — module pairs that co-change without a static import edge.
- `structural_weight` — size-skew god-modules (LOC far above the median).
- `complexity` — functions over a cyclomatic-complexity threshold (needs `lizard`).
- `risk_hub` — cross-module symbol surface-breadth × explicit config volatility.
  Requires `tools.scip.enabled: on`; reports `n/a` otherwise.
- `architecture_fitness` — presence of architecture enforcement signals (arch
  tests, import-linter config, arch-linter in CI). Score 0–10.
- `functional_candidates` — module pairs sharing duplicated logic (clone clusters),
  cross-referenced with co-change. Requires `tools.clones.enabled: on`.
- `change_locality` — per-change drift: how far a change reaches beyond its own
  modules (delta mode only; `n/a` in full mode).
- `unsafe_density` — count of unsafe operations per module (Rust; needs
  `tools.syntax.enabled: on`).
- `panic_density` — count of production panic/unwrap operations per module
  (Rust/Go; excludes test files; needs `tools.syntax.enabled: on`).
- `struct_field_density` — per-module count of struct definitions (Go/Rust;
  needs `tools.syntax.enabled: on`).
- `test_density` — per-module count of test functions (Go/Rust/Python proxy;
  needs `tools.syntax.enabled: on`).
- `deprecated_dep_count` — count of locally-declared deprecation/retraction
  markers in manifest files (`go.mod retract`, `package.json deprecated`).
- `file_mutual_import` — count of file pairs that mutually import each other
  (TypeScript file→file cycles; no extra tool required).

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
    max_new_high: 0
  cycle:
    enabled: true
    gate: fail
    max_new: 0
  coverage:
    enabled: true
    gate: warn
    min_delta: 0
```

Current behavior note: the CLI reports the built-in metrics every run. Baseline
metric deltas can produce a warning verdict. The per-metric policy fields are
parsed and kept in config, but detailed threshold gating is still limited.

## `map_review`

`map_review` enables advisories about architecture-map quality:

```yaml
map_review:
  stale_after: 2160h
  gate: warn
```

Checks include:

- graph nodes not covered by any module `paths` glob;
- module `paths` globs that match no graph nodes;
- modules whose `reviewed_at` is older than `stale_after`.

`stale_after` uses Go duration syntax. Use `2160h` for 90 days.

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
archfit check --format text
archfit check --format json
archfit check --format markdown
archfit check --format sarif
```

`--format` is repeatable: `--format json --format sarif` writes both to stdout.

`scan` is the Markdown report shortcut.

## `exclusions` and built-in defaults

`exclusions` is a list of repo-relative globs skipped during extraction:

```yaml
exclusions:
  - "**/generated/**"
  - "**/*.pb.go"
```

archfit also ships a built-in default exclusion set — tool-artifact, cache,
dependency directories, and test fixtures it never analyses, because measuring them
yields non-deterministic or irrelevant facts (a vendored tree's complexity, a
generated index, a report written back into the scanned repo, or a fixture repo
inside `testdata/` distorting coverage signals):

```text
.archfit-cache/  .archfit-baseline.json  .gitnexus/  .codegraph/  reports/
.venv/  node_modules/  vendor/  dist/  build/  **/testdata/**
```

`**/testdata/**` is excluded by default because test fixture repos are not
production architecture: analysing them creates false coverage gaps and phantom
language detections. Re-include intentionally with `!testdata` or
`!**/testdata/**` in your `exclusions` list.

The built-ins are **merged with** your config `exclusions`, not replaced. To
re-include one of them, prefix it with `!`:

```yaml
exclusions:
  - "!reports" # analyse reports/ despite the default
```

archfit also warns (a config warning + stderr line) when an output/report path
resolves **inside** the analyzed root — write reports outside `--root`, or exclude
their directory, to keep scans deterministic.

## Glob tips

- Use repo-relative paths.
- Prefer explicit `**` globs, such as `internal/domain/**`.
- Keep module names stable; baselines and exceptions refer to rule IDs and
  finding fingerprints.
- For Python, see [Language support](languages.md#python) before writing globs.

## `volatility_cascade_enabled`

Top-level boolean (default `false`). When `true`, enables the inferred-volatility
cascade from Khononov Ch9: after classifying each module's volatility from
`subdomain`/`volatility` config, a single-hop propagation pass raises a
module's effective volatility to `high` when it is strongly coupled
(`functional` or `intrusive` strength) to a `core` module. The cascade runs
before scoring, so the raised volatility is visible in advisory output.

```yaml
volatility_cascade_enabled: true
```

Use when you want archfit to dogfood the full book model and surface coupling
chains that inherit core volatility transitively. Set it to `false` (or omit)
to use only declared per-module volatility — safer for repos where the cascade
would produce noise until `subdomain` fields are complete.

## tools.llm (off-gate)

Used by `archfit init --llm`, `archfit update --llm`, `archfit enrich`,
`archfit autopilot`, `archfit review`, and `archfit explain --llm`; never by `check`.

```yaml
tools:
  llm:
    provider: anthropic # anthropic | openai | ollama
    model: claude-opus-4-8
    base_url: "" # ollama only; default http://localhost:11434/v1
```

API keys come from `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` env vars — never from
config. archfit also best-effort loads a local `.env` (cwd) at startup, but only
sets a key that is currently **unset** — real environment variables and CI secrets
always win. Keep `.env` out of version control (it is gitignored by default). The
LLM response cache lives at `.archfit-cache/llm/`.

## Draft and pin files

The LLM authoring commands write proposals to review files, never to
`.archfit.yaml` directly:

- `.archfit-labels.yaml` — pinned coupling-strength labels (`archfit enrich`).
  `check` consumes `status: approved` entries with precedence: config
  public/internal globs > approved labels > extractor hint.
- `.archfit-owners.yaml` — owner drafts (`archfit enrich --owner`).
- `.archfit-volatility.yaml` — volatility drafts (`archfit enrich --volatility`).
- `.archfit-subdomains.yaml` — subdomain drafts (`archfit enrich --subdomains`).
- `.archfit-autopilot.yaml` — a full commented config draft (`archfit autopilot`).

Review each, then `enrich --<field> --pin` (or move the field manually) to write
approved values into `modules.<name>`. Pinning never overwrites a live field. See
[llm-enrich.md](llm-enrich.md).

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
  decision), `llm` (drafted by `archfit enrich`, then human-approved), or
  `tool` (deterministic extractor hint).
- `confidence` — how certain the judgment is: `high`, `medium`, or `low`.

**Effect on scoring:** when an approved label has `provenance: llm` and
`confidence` below `high`, `coupling_balance` confidence is lowered by one
band. Rationale: LLM drafts have been human-reviewed, but they are not as
certain as a config-glob or SCIP symbol-kind classification. `provenance:
human` and `provenance: tool` do not lower confidence.

`check` reads only `status: approved` labels. Draft labels (no `status` or
`status: draft`) are never consumed by the gate.

### Abstain and decision tasks

When an edge's strength cannot be classified (`unknown`) but its distance
resolves to an internal module, archfit **abstains** — the edge is excluded
from the `coupling_balance` scored distribution (honest denominator) and an
`agent_task` is emitted in JSON/SARIF output prompting the operator to add a
label. The same happens for modules with no declared `subdomain` or
`volatility`: an `agent_task` asks for a declaration or suggests
`archfit enrich --subdomains`.

External/library edges (`Distance == DistanceUnknown`, i.e. stdlib,
third-party packages, undeclared imports) are excluded from
`coupling_balance` entirely — they are not internal coupling seams. Their
count is visible in `classified_edges.external` and the `coupling_balance`
evidence string. External dependency hygiene is a `dependency_graph_health`
concern, not a `coupling_balance` concern.
