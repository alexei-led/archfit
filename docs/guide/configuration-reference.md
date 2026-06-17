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

See [Language support](languages.md) for per-language install details.

### Opt-in analysis tools

Three additional tools are **off by default** and feed the report-only metrics. They
are opt-in because they are slower and/or need extra binaries:

```yaml
tools:
  scip:
    enabled: on # symbol-level analysis (SCIP indexers + uv); powers risk_hub
  clones:
    enabled: on # clone detection (jscpd / PMD-CPD); powers functional_candidates
  gitnexus:
    enabled: on # historical change-impact; enriches risk_hub ranking
```

- `scip` — runs a SCIP indexer (`scip-go`/`scip-python`/`scip-typescript`) plus `uv`
  to build the symbol graph. Required for `risk_hub`; without it `risk_hub` is `n/a`.
- `clones` — runs a clone detector to find duplicated logic. Required for
  `functional_candidates`; without it that metric is `n/a`.
- `gitnexus` — optional enrichment of `risk_hub` with historical impact. Never auto.

`gitnexus` and `clones` accept only `on`/`off` (not `auto`). When absent or disabled,
the dependent metric simply reports `n/a` — it never fails the run.

## `layers`

Layers are ordered from inner to outer.

```yaml
layers: [domain, application, adapter]
```

The `forbidden_layer_direction` rule treats dependencies from an earlier layer to
a later layer as violations. With the example above, `domain -> adapter` is a
violation, while `adapter -> domain` is allowed.

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
- `subdomain` — `core`, `supporting`, `generic`, or `unknown`.
- `volatility` — optional explicit override: `high`, `medium`, or `low`.
- `owner` — team or person responsible for the module.
- `deploy_unit` — deployable/runtime unit used for distance classification.
- `reviewed_at` — date of last architecture-map review.
- `reviewed_by` — reviewer identity.

Balanced Coupling classification uses this metadata:

- target `public` match -> `contract` strength;
- target `internal` match -> `intrusive` strength;
- same module -> same-module distance;
- same owner -> cross-module same-owner distance;
- different deploy units -> cross-deploy-unit distance;
- `volatility` or `subdomain` -> target volatility.

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
- `type` — built-in rule type.
- `from` — optional source glob.
- `to` — optional target glob.
- `from_layer`, `to_layer` — parsed fields reserved for layer-specific policy.
- `gate` — policy marker such as `fail` or `warn`.
- `patterns` — optional ast-grep patterns for future structural evidence.

Current behavior note: active structural rule findings are emitted in the gate
channel. Use baselines and exceptions while calibrating noisy rules.

Built-in rule types:

- `forbidden_dependency` — fires when `from` and `to` match an edge.
- `public_api_only` — fires on internal-access edges, optionally filtered by
  `from` and `to`.
- `internal_api_access` — same internal-access signal, with separate rule ID.
- `forbidden_layer_direction` — fires when dependency direction violates the
  ordered `layers` list.
- `new_cross_module_dependency` — fires on cross-module edges. Current
  implementation reports all cross-module edges, then baseline status separates
  known from new findings.
- `cycle` — fires once per detected import cycle.

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

## Glob tips

- Use repo-relative paths.
- Prefer explicit `**` globs, such as `internal/domain/**`.
- Keep module names stable; baselines and exceptions refer to rule IDs and
  finding fingerprints.
- For Python, see [Language support](languages.md#python) before writing globs.

## tools.llm (off-gate)

Used by `archfit init --llm`, `archfit update --llm`, `archfit enrich`, and
`archfit explain --llm`; never by `check`.

```yaml
tools:
  llm:
    provider: anthropic # anthropic | openai | ollama
    model: claude-opus-4-8
    base_url: "" # ollama only; default http://localhost:11434/v1
```

API keys come from `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` env vars — never
from config. The LLM response cache lives at `.archfit-cache/llm/`.

## .archfit-labels.yaml

Pinned coupling-strength labels (the reviewed output of `archfit enrich`).
`check` consumes `status: approved` entries with precedence: config
public/internal globs > approved labels > extractor hint. See
[llm-enrich.md](llm-enrich.md).
