# Configuration

Archfit reads `.archfit.yaml` from the current directory unless `--config` selects another file. The YAML parser rejects unknown fields. The current configuration schema version is `2`.

The project release version is separate from the configuration version. The current release is v3. Version `2` identifies the YAML contract, not the executable.

Generate a starter file with `archfit config init`. Review its modules, layers, and rules before using it in CI.

```yaml
version: 2
languages:
  go:
    enabled: auto
modules:
  domain:
    paths: [internal/domain/**]
    layer: domain
layers: [domain, application, adapter]
rules:
  - id: layer_direction
    type: forbidden_layer_direction
    gate: fail
```

See [Rules](rules.md) for rule fields, rule types, selectors, and waivers. See [AI workflows](ai.md) for labels and draft files.

## Top-level keys

| Key | Type | Default or requirement | Purpose |
| --- | --- | --- | --- |
| `version` | integer | Required. Must be `2`. | Configuration contract version. |
| `target_branch` | string | Empty: `origin/HEAD`, then `main` or `master` when exactly one exists. | Branch changes land on. `check` compares a change with it. `check` reads it from the default branch's copy of this file, so a branch cannot retarget itself. If both `main` and `master` exist, both copies must declare the same value. A config outside the repository is not the branch's to edit and is read as is; a repository with no default branch uses the working-tree value, and then the branch can retarget itself. See [commands](commands.md#check). |
| `exclude` | list of globs | Built-in exclusions plus no user globs. | Skips paths during analysis. |
| `languages` | object | Each language uses `auto`. | Selects language extractors and missing-tool gates. |
| `analyzers` | object | Every analyzer is disabled. | Enables optional evidence producers. |
| `coverage` | object | Disabled. | Reads caller-supplied coverage artifacts. |
| `ai` | object | No provider. | Configures explicit off-gate AI commands. |
| `coupling` | object | `min_severity: medium`, `duplicated_knowledge: score`; seam gate warns. | Tunes coupling advisories and the distributed-monolith gate. |
| `layers` | list of names | Empty. | Orders architecture layers from inner to outer. |
| `modules` | map | Empty. | Maps module names to paths and architecture metadata. |
| `external_systems` | map | Empty. | Declares external integration boundaries for coupling analysis. |
| `rules` | list | Empty. | Declares architecture checks. See [Rules](rules.md). |
| `waivers` | list | Empty. | Temporarily accepts findings. See [Rules](rules.md#waivers). |
| `metrics` | map | Built-in metrics enabled. | Sets metric gates and baseline thresholds. |
| `module_review` | object | Off unless configured. | Reviews module ownership and review dates. |
| `file_class` | object | Built-in classification only. | Adds generated, test, and mock filename patterns. |
| `outputs` | object | Fields decode as booleans. | Accepted configuration fields. They do not select CLI output formats. |

## Paths and globs

Use repository-relative paths with forward slashes. Path globs use doublestar semantics. `**` can match across path separators.

Archfit merges user `exclude` entries with its built-in exclusions. Built-ins include `.archfit-cache/`, `.archfit-baseline.json`, `.gitnexus/`, `.codegraph/`, `reports/`, `.venv/`, `node_modules/`, `vendor/`, `dist/`, `build/`, and `**/testdata/**`.

Prefix a pattern with `!` to re-include a built-in exclusion. For example, `!reports` includes `reports/`. Excluded source leaves the scope of rules. Language applicability can also remove source from dependency-rule scope. See [Languages](languages.md) for those cases.

The `exclude` setting does not change the separate LOC walk for size and file-class metrics. Archfit warns when a report path resolves inside the analyzed root. Write reports outside the root or exclude their directory.

## Languages

Each language accepts `enabled` and `gate`. `enabled` accepts YAML booleans or the string `auto`. The strings `on` and `off` are invalid.

| `enabled` | Behavior |
| --- | --- |
| `auto` or omitted | Use the extractor when project markers and required tools exist. |
| `true` | Require the language adapter. Missing project markers or tools produce an error. |
| `false` | Skip the adapter. Source leaves dependency-rule scope unless an explicit `gate` keeps the missing evidence applicable. |

`gate` accepts `off`, `warn`, or `fail`. Its default is `warn`. It controls the response to missing required evidence. `--require-tools` raises gaps to fail except when configuration explicitly sets `off`.

| Key | Fields | Defaults and behavior |
| --- | --- | --- |
| `languages.go` | `enabled`, `gate`, `modules.include`, `modules.exclude` | `modules.include` absent or empty includes every in-scope Go workspace member. `exclude` removes matches after include. Patterns match ScanRoot-relative workspace member paths. An empty result means absent, with no coverage gap. |
| `languages.typescript` | `enabled`, `gate` | Uses the TypeScript extractor when its project markers and tools exist. |
| `languages.python` | `enabled`, `gate`, `package` | `package` is the top-level importable package name. Empty means auto-detect from the project layout. |
| `languages.rust` | `enabled`, `gate`, `manifest`, `features`, `include_dev_deps` | Empty `manifest` uses the root Cargo manifest. `features` defaults empty. `include_dev_deps` defaults false. |

Example Go workspace filter:

```yaml
languages:
  go:
    enabled: true
    modules:
      include: [server/shared/**, server/auth/**]
      exclude: ["**/testdata/**"]
```

## Optional analyzers

Analyzers are opt-in. Every analyzer requires `enabled: true` to run. `auto`, `false`, and omission disable it. A schema-valid `enabled: auto` value does not enable an analyzer.

| Key | Fields | Purpose and limits |
| --- | --- | --- |
| `analyzers.syntax` | `enabled`, `gate` | Uses ast-grep (`sg`) for declaration facts in Go, TypeScript, Python, and Rust. |
| `analyzers.scip` | `enabled`, `gate`, `timeout` | Uses language SCIP indexers for symbol-level edge strength. Go type information remains primary. |
| `analyzers.clones` | `enabled`, `gate`, `timeout` | Uses `jscpd` for cross-module clone facts. Clone facts do not alter import-edge strength. |
| `analyzers.cargo_modules` | `enabled`, `gate` | Uses `cargo-modules` to extract Rust intra-crate modules. It can compile the crate. |

All analyzer `gate` fields accept `off`, `warn`, or `fail`, defaulting to `warn`. A missing tool creates a coverage gap. `enabled: false` skips the analyzer without a gap. `--require-tools` raises gaps unless the gate is explicitly `off`.

`scip` and `clones` accept Go duration strings in `timeout`. Omitted or zero values use built-in timeouts: 20 minutes for SCIP and 5 minutes for clones. A timeout drops that analyzer's result and the run continues. Dependent measurements can report `n/a`.

### Why the optional analyzers are off

They run extra subprocesses over the whole tree, and their cost is not proportional to the value of the default gate. Measured on real repositories: `scip` costs tens of seconds on a Go repository and several minutes on a Rust workspace (it compiles target metadata), `clones` compares every file pair, `syntax` parses every file with ast-grep, and `cargo_modules` can compile the crate. The default gate answers "is this architecture drifting" from the language adapters alone, so the deeper passes are opt-in and their absence is reported as `disabled`, never as a silent pass.

### Enabling an optional analyzer

```yaml
analyzers:
  scip:    { enabled: true, timeout: "10m" }   # symbol-level coupling strength
  syntax:  { enabled: true }                   # roles, routes, exported surface
  clones:  { enabled: true, timeout: "5m" }    # cross-module duplication
  cargo_modules: { enabled: true }             # Rust intra-crate modules
```

Prerequisites, per analyzer and language. A missing prerequisite shows up as `absent` with the reason that names it.

| Analyzer | Go | TypeScript / JavaScript | Python | Rust |
| --- | --- | --- | --- | --- |
| `scip` | `scip-go` | `node_modules` installed (`npm ci`) and `scip-typescript` | `scip-python` and `uv` | Cargo metadata (`cargo`) and `rust-analyzer` |
| `syntax` | `sg` (ast-grep) | `sg` | `sg` | `sg` |
| `clones` | `jscpd` | `jscpd` | `jscpd` | `jscpd` |
| `cargo_modules` | — | — | — | `cargo-modules` (compiles the crate) |

The shipped image contains all of them. When you run archfit from somewhere else, `--require-tools` turns any missing required analyzer into a failure instead of a gap.

### Turning one off without a gap

`enabled: false` skips the analyzer and does not report a coverage gap. Use it when you know the pass does not apply to your repository — for example `scip` on a Rust workspace where symbol attribution stays partial, or `clones` on a repository with generated code.

## Supplied coverage

Archfit reads coverage artifacts from another test step. It never runs the target repository's tests. Coverage is disabled unless `coverage.enabled: true`.

```yaml
coverage:
  enabled: true
  gate: warn
  sources:
    - path: coverage.out
      format: go-coverprofile
```

| Key | Type | Default and constraints |
| --- | --- | --- |
| `coverage.enabled` | boolean | `false`. When false, no artifact is read. |
| `coverage.gate` | enum | `warn`. Accepts `off`, `warn`, `fail`. Applies when none of the sources can be read. It is not a minimum coverage ratio. |
| `coverage.sources` | list | Required when enabled. Paths resolve under the analysis root and cannot escape it. |
| `sources[].path` | path | Required artifact path. |
| `sources[].format` | enum | `auto`, `go-coverprofile`, `lcov`, `coverage-py-json`, or `llvm-cov-json`. `auto` is the default and requires an unambiguous content and extension match. |
| `sources[].sidecar_path` | path | Defaults to `<path>.sidecar.json`. |
| `sources[].max_bytes` | positive integer | Defaults to `67108864` bytes per source. The limit covers the artifact, sidecar, and enumerated source files. |
| `sources[].max_facts` | positive integer | Defaults to `1000000` normalized facts per artifact. Exceeding the limit rejects the whole artifact. |

Supported formats read statements for Go cover profiles and coverage.py JSON. LCOV and LLVM coverage JSON report lines. Archfit normalizes file names to slash-separated paths relative to the analysis root. Missing files, paths outside the root, and escaping symlinks remain unresolved and keep testability partial.

### Freshness sidecar

Each artifact can use a JSON sidecar. The default path is the artifact path plus `.sidecar.json`.

```json
{
  "schema_version": 1,
  "source_ref": "0123456789abcdef",
  "modules": ["api"],
  "sources": {
    "internal/api/handler.go": "<lowercase-sha256-of-file-bytes>"
  }
}
```

`schema_version` must equal `1`. `source_ref` is metadata and is not a freshness check. `modules` lists non-empty declared module names represented by the artifact. `sources` lists every covered source path and its lowercase SHA-256 hash.

Freshness is `matched` only when `sources` is non-empty, every parsed path has an entry, and every listed file exists with matching bytes. Missing, changed, or unlisted files make the sidecar stale. Missing or invalid sidecars and empty `sources` are unverified. Only `matched` sidecars can promote testability. Archfit checks freshness on every analysis, including cache hits.

The sidecar is unsigned producer data. Archfit cannot verify who made it or prove that the artifact includes all source facts. A producer can change the artifact and sidecar together.

Multiple artifacts combine only when their units are compatible. For duplicate files with equal units and totals, Archfit keeps the greatest covered count as a conservative lower bound. Different units or totals keep the dimension partial and suppress the aggregate ratio.

## AI provider

The `ai` block configures explicit off-gate AI commands. It does not affect deterministic gate analysis. See [AI workflows](ai.md) for command behavior, draft review, and key handling.

| Key | Type | Default or allowed values |
| --- | --- | --- |
| `ai.provider` | string | `anthropic`, `openai`, `deepseek`, `ollama`, or `openai-compatible`. No default in config. |
| `ai.model` | string | Provider API model ID. Required with a provider. |
| `ai.base_url` | URL | Optional except for `openai-compatible`. Defaults are listed below. |
| `ai.api_key_env` | string | Environment-variable name, not a key value. Only for `openai-compatible`. Omission means keyless access. |
| `ai.effort` | string | Provider/model-specific reasoning level. See restrictions below. |
| `ai.max_tokens` | positive integer | Total output cap, including reasoning. Overrides command caps when set. Default command caps: 32,768. |
| `ai.timeout` | duration | Whole provider call, including retries. Default: `5m`. Explicit durations must be positive. |

Default base URLs are `https://api.anthropic.com`, `https://api.openai.com/v1`, `https://api.deepseek.com`, and `http://localhost:11434/v1`.
Custom URLs must use HTTP or HTTPS without credentials, queries, or fragments.
Generic servers receive Chat Completions requests with `max_tokens`, not `max_completion_tokens`.

Effort defaults to `medium` for the exact `claude-opus-5-5` and `gpt-6.1-sol` model IDs.
The DeepSeek preset defaults to `high` and accepts only `low|high|max`.
Other models and generic endpoints receive effort only when configured.
Anthropic and Sol 6.1 reject `none` and `minimal` in these adapters.
Equal effort names do not imply equal reasoning budgets across providers.

Use the provider's API model ID, not a model-card catalog ID.
Availability and settings remain provider/account-specific.
See [AI workflows](ai.md) for examples, credentials, cache behavior, extension steps, and model updates.

## Coupling

| Key | Type | Default and allowed values |
| --- | --- | --- |
| `coupling.min_severity` | enum | `medium`. Accepts `low`, `medium`, `high`, `critical`. |
| `coupling.duplicated_knowledge` | enum | `score`. Accepts `score` or `advisory`. Controls whether clone-only pairs affect `coupling_balance`. |
| `coupling.volatility_cascade` | boolean | `false`. When enabled, strong coupling to high effective volatility can raise a module's effective volatility. |
| `coupling.gate` | object | Optional. If present, requires `distributed_monolith`. |
| `coupling.gate.distributed_monolith.mode` | enum | `warn`. Accepts `warn` or `fail`. |
| `coupling.gate.distributed_monolith.max_new_seams` | integer | `0`. Must not be negative. Applies in fail mode against a comparable reference. |

If the entire coupling gate block is absent, the distributed-monolith rule still runs in warning mode with zero tolerated new seams. A seam counts as one ordered module pair, regardless of import count. Fail mode blocks only newly introduced qualifying seams against a comparable stored baseline. `analyze --base` is report-only and does not supply this gate reference. See [Coupling](coupling.md) for qualification and comparison behavior.

## Layers and modules

`layers` orders names from inner to outer. `forbidden_layer_direction` flags dependencies from an earlier layer to a later one.

```yaml
layers: [domain, application, adapter]
```

Each `modules.<name>` entry accepts these fields:

| Field | Type | Meaning and defaults |
| --- | --- | --- |
| `paths` | list of globs | Paths owned by the module. |
| `public` | list of globs | Declares public API paths. It does not enforce access by itself. |
| `internal` | list of globs | Declares private paths used by API access rules and coupling classification. |
| `layer` | string | Must name an entry in `layers`. |
| `subdomain` | enum | `core`, `supporting`, or `generic`, matched case-insensitively. `core` implies high volatility; the others imply low. |
| `volatility` | enum | `high`, `medium`, `low`, `frozen`, or legacy alias `legacy`, case-insensitive. Overrides the subdomain-derived value. |
| `owner` | string | Team or person responsible for the module. |
| `deploy_unit` | string | Runtime or deployment boundary used by coupling classification. |
| `role` | enum | Empty or `composition_root`, `adapter`, `core`, `shared_model`, `generated`, `test`. |
| `reviewed_at` | date/time | Last architecture map review date. |
| `reviewed_by` | string | Reviewer identity. |
| `depends_on` | list of selectors | Allowed outbound module dependencies. Absent means unconstrained. `[]` allows no declared module. |
| `visible_to` | list of selectors | Allowed inbound modules. Absent means visible to all declared modules. An unowned importer is always denied. |

The `module_dependencies` rule enforces `depends_on` and `visible_to`. Without the rule, the lists have no effect. Entries can name a module, use `layer:<name>` or `role:<role>`, or glob module names. Selectors only see declared modules. A selector that matches no module allows nothing and produces a warning or lint error.

Module path resolution uses the most specific matching glob. Equal-specificity matches use alphabetical module order. Archfit does not infer volatility from paths. A module with neither a known `subdomain` nor a known `volatility` has undeclared volatility.

## External systems

Each `external_systems.<name>` entry needs at least one `targets` glob. `volatility` accepts `high`, `medium`, `low`, or `frozen` and defaults to `low`.

Targets match the language extractor's target spelling: Go import paths, resolved TypeScript package paths, Python dotted names, or Rust crate names. A matching target is classified as `declared_external` at distance 10. Other external dependencies remain excluded from coupling scoring.

## Metrics

Metric entries accept `enabled`, `gate`, `min_delta`, and `max_new`. Metrics run by default. An absent entry and an entry with only thresholds or a gate remain enabled. Only explicit `enabled: false` disables a metric.

| Metric | Kind | Default threshold | Notes |
| --- | --- | --- | --- |
| `encapsulation` | ratio | `min_delta: 0` | A drop from a comparable baseline can gate. |
| `coverage` | ratio | `min_delta: 0` | Measures extracted files over applicable files. |
| `cycle` | count | `max_new: 0` | Node-level cycles. It is always zero for compiling Go. |
| `unbalanced_edge` | count | `max_new: 0` | New high-risk intrusive volatile edges across larger boundaries. |
| `blast_radius` | informational | None | Report-only. Accepts only `enabled`; never gates. |

Metric `gate` accepts `off`, `warn`, or `fail`. Unset means `fail`. A blocking metric regression makes `check` exit `1`. `warn` emits a diagnostic. `off` skips the metric gate. Metric gates need a comparable stored baseline. `min_delta` applies only to ratio metrics. `max_new` applies only to count metrics. Wrong-kind fields are configuration errors.

`metrics.function_loc_threshold` is a positive integer with default `60`. It counts functions and methods whose inclusive lines of code exceed the threshold. It is a diagnostic only and never gates. `coupling_balance` is not a configurable metric.

## Module review

`module_review` accepts `stale_after` and `gate`. `stale_after` is a Go duration string and has no default. The gate accepts `off`, `warn`, or `fail` and defaults to `warn` when review is configured.

The review reports unowned production paths, module path globs that match no graph node, and stale `reviewed_at` dates. Only `map/uncovered_path` can block. The gate applies to unowned production source. The other findings remain diagnostics. `2160h` is 90 days.

## File classification

`file_class` adds patterns after built-in classification. Auto-detection runs first.

| Field | Match target | Meaning |
| --- | --- | --- |
| `generated_globs` | Repository-relative slash path | Classifies matching files as generated. Supports `**`. |
| `test_globs` | Repository-relative slash path | Classifies matching files as tests. |
| `mock_frameworks` | Base filename | Filename prefixes or suffixes that identify generated mocks. |

## Output fields

The parser accepts `outputs.json`, `outputs.markdown`, and `outputs.sarif` as booleans. Runtime output selection uses command-line flags, not these fields. Use `--format` or the `--json`, `--markdown`, and `--sarif` shorthands.

## Labels and draft files

`.archfit-labels.yaml` stores coupling-strength labels separately from `.archfit.yaml`. Its file version is `1`. A missing file is allowed. Archfit uses only entries with `status: approved`.

```yaml
version: 1
labels:
  - from: api
    to: domain
    strength: model
    rationale: Callers use domain types across this boundary.
    evidence_refs: [api:api-domain]
    basis: semantic_judgment
    evidence_hash: "<hash-from-current-evidence>"
    status: approved
    confidence: high
    provenance: human
```

Each entry requires `from`, `to`, `strength`, and `status`. `from` and `to` name different declared modules. `strength` accepts `contract`, `model`, `functional`, `symmetric`, or `intrusive`. Entries must not repeat an ordered module pair. Invalid strengths, self-pairs, duplicate pairs, or undeclared endpoints are hard errors.

`rationale`, `evidence_refs`, `basis`, `evidence_hash`, `confidence`, and `provenance` are optional.
`confidence` accepts `high`, `medium`, or `low`.
`provenance` accepts `human`, `llm`, or `tool`. Empty provenance is treated as human.
Omitted confidence does not penalize a human label.
An applied LLM label needs explicit `confidence: high` to avoid lowering coupling confidence by one band.

`evidence_hash` records the import-edge evidence for the ordered module pair when the label was drafted. Archfit ignores an approved label when current evidence exists and the hash differs. The stale label adds a `labels/stale` diagnostic. It does not gate the run. An empty hash skips freshness checks and can leave an override active permanently. A pair with no current edges is moot, and a run with no measured graph cannot check freshness.

Strength precedence depends on provenance. A config internal glob sets intrusive strength and cannot be overridden. A public glob sets contract strength. An approved human, tool, or unset-provenance label can refine that contract strength and takes precedence over extractor hints. An approved LLM-provenance label only fills an unknown edge after config globs and extractor hints leave it unknown. Draft labels never affect classification.

`archfit config enrich labels` writes label proposals for review. Approve a proposal by setting `status: approved`. Review its evidence and hash before analysis. See [AI workflows](ai.md) for command behavior.

Module annotation drafts use separate files:

| File | Field | Draft value |
| --- | --- | --- |
| `.archfit-owners.yaml` | `owner` | Scalar `value`. |
| `.archfit-volatility.yaml` | `volatility` | Scalar `value`. |
| `.archfit-subdomains.yaml` | `subdomain` | `subdomain`, with optional `volatility`. |

These files use version `1`. Each draft includes `module`, the proposed value, `rationale`, and `status`. `evidence_refs`, `basis`, `confidence`, and `provenance` are optional. `basis` identifies `deterministic_fact` or `semantic_judgment`. Mark reviewed entries `status: approved`, then run `archfit config enrich <owner|volatility|subdomain> --apply`. Apply writes only to an empty target field. Existing approved draft entries are not replaced by new drafts.

```yaml
version: 1
field: owner
drafts:
  - module: payments
    value: "@team-payments"
    rationale: "The ownership map assigns Payments to this team."
    evidence_refs: [doc:README.md]
    basis: semantic_judgment
    status: draft
    confidence: medium
    provenance: llm
```

`config init --ai-classify -o <file>` can also write a full commented config draft. Review it before copying values into `.archfit.yaml`. AI commands and their write behavior belong to [AI workflows](ai.md).

## Schema and editor support

The repository ships [`archfit.schema.json`](../archfit.schema.json), generated from configuration types and comments.
Copy the schema beside your configuration, then add this comment for YAML language-server support:

```yaml
# yaml-language-server: $schema=./archfit.schema.json
version: 2
```

VS Code can bind the schema in `.vscode/settings.json`:

```json
{
  "yaml.schemas": {
    "./archfit.schema.json": ".archfit.yaml"
  }
}
```

The schema helps editors validate keys and closed values. It does not replace `archfit config lint` or runtime validation.
Some conditional rules need runtime context. For example, `coverage.sources` is required only when coverage is enabled.
Rule selector liveness depends on the scanned repository.
The schema treats module `layer`, `role`, `subdomain`, and `volatility` values as strings.
An unknown `role` is a configuration error.
Runtime analysis warns about unknown layer, subdomain, and volatility declarations. `config lint` reports those declarations too.
Check the schema and runtime together when changing configuration contracts.

The JSON schema contract is versioned independently from the Archfit release. `version: 2` is the only configuration version this release accepts. This release rejects version 1 and removed coupling gate fields. Use the migration instructions in [Migration](migration.md) for older files.

## Starter configurations

Copy a template, then replace its paths, owners, and policy with your repository's values.
Declare public surfaces explicitly. An owner value does not reduce coupling distance.

| Template | Shape |
| --- | --- |
| [Go monolith](../examples/go-monolith.archfit.yaml) | Layered packages in one deployable. |
| [TypeScript monorepo](../examples/ts-monorepo.archfit.yaml) | Workspace packages. |
| [Python package](../examples/python-package.archfit.yaml) | Dotted modules in a `src/` layout. |
| [Domain modules](../examples/ddd.archfit.yaml) | Bounded contexts and subdomain metadata. |
| [Microservices](../examples/microservices.archfit.yaml) | Separate deployment units. |
