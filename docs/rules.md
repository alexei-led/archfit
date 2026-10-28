# Rules

Rules declare architecture constraints in `.archfit.yaml`. Each rule needs a stable `id` and a supported `type`. Configuration fields live in [Configuration](configuration.md).

```yaml
rules:
  - id: domain_no_http
    type: forbidden_dependency
    from: internal/domain/**
    to: internal/http/**
    gate: fail
```

## Rule fields

| Field | Applies to | Meaning |
| --- | --- | --- |
| `id` | All rules | Stable ID used by findings, baselines, and waivers. |
| `type` | All rules | Rule implementation to run. Unknown values are errors. |
| `gate` | All rules | `fail`, `warn`, or `off`. Most rules default to `fail`; `public_api_change` and `public_api_type_leak` default to `warn`. |
| `from` | Selector rules | Source path or node glob. For `forbidden_pattern`, omission means all production files. |
| `to` | Dependency rules | Target path or node glob. |
| `from_module` | `forbidden_dependency` | Module selector for the importing side. Use instead of `from`. |
| `to_module` | `forbidden_dependency` | Module selector for the imported side. Use instead of `to`. |
| `max` | `public_api_max` | Maximum exported declarations per module. |
| `threshold` | Reserved | Accepted by the schema for a reserved per-rule integer threshold. No built-in rule uses it. |
| `patterns` | `forbidden_pattern` | Required ast-grep pattern entries. Accepted on other rules but ignored, with a warning. |
| `guard` | Selector rules | Marks an expected empty selector. Default is false. |
| `rationale` | All rules | Adds a reason to the finding and repair constraints. Give every `gate: fail` rule one: it names the consequence the rule prevents, so an agent that hits the rule fixes the cause. `config lint` warns when it is missing (`--strict`: error). |
| `alternatives` | All rules | Lists allowed alternatives in findings and repair constraints. |
| `docs` | All rules | Adds a document reference to findings and repair constraints. |

`from` and `from_module` are mutually exclusive. `to` and `to_module` are mutually exclusive. `forbidden_dependency` requires one selector form on each side. `forbidden_layer_direction`, `cycle`, `module_cycle`, and `module_dependencies` do not accept `from` or `to`.

`rationale`, `alternatives`, and `docs` do not change finding IDs. Archfit folds whitespace and line breaks to spaces when it reads these fields. Long text is truncated in reports.

## Gate behavior

| Gate | Behavior |
| --- | --- |
| `fail` or omitted | Active findings block the check and produce exit code 1. |
| `warn` | Findings remain advisory. The check can still return exit code 2 for incomplete evidence. |
| `off` | Archfit skips the rule and emits no findings. |

`public_api_change` and `public_api_type_leak` default to `warn`. Set `gate` explicitly when the default does not match the intended policy.

## Built-in rule types

| Type | Behavior and key requirements |
| --- | --- |
| `forbidden_dependency` | Finds edges matching both endpoint selectors. Requires one source selector and one target selector. Each side uses a path glob or module selector. A module selector matches only edges between different declared modules. |
| `forbidden_layer_direction` | Finds dependencies from an inner layer to an outer layer. Uses `layers` order and module `layer` values. Declare at most one global rule of this type. |
| `forbidden_pattern` | Finds ast-grep patterns in production files. Requires `patterns`; `from` is optional; `to` is invalid. Pattern IDs must be unique across all rules. |
| `internal_api_access` | Finds access to internal module surfaces. Uses module `public` and `internal` declarations, then extractor hints when no declaration covers the target: an edge the extractor marks `intrusive` (a C# internal member reached through `InternalsVisibleTo`, a Python underscore import, a Rust private symbol) is a use of an undeclared internal surface and fires the rule. A strength an approved label pinned is the owner's verdict on coupling depth, not access evidence, so it does not fire the rule on its own. |
| `public_api_only` | Finds access to internal module surfaces. Uses the same surface resolution as `internal_api_access`. |
| `cycle` | Finds cycles between dependency graph nodes. Go's file-to-package graph does not produce these cycles. Use `module_cycle` for cycles across declared modules. |
| `module_cycle` | Finds cycles between declared modules. It ignores unowned and external endpoints. Needs at least two declared modules with dependency evidence. One finding covers one strongly-connected component: its members are listed once, sorted, and `matched_by.cycle_modules` names the same set, so an eleven-module cycle is one finding and one agent task, not one per pair. |
| `module_dependencies` | Enforces module `depends_on` and `visible_to` lists. It takes no `from` or `to`. |
| `new_cross_module_dependency` | Finds cross-module edges. Baseline status separates known findings from new findings. |
| `public_api_max` | Finds modules whose exported declaration count exceeds `max`. Requires `analyzers.syntax.enabled: true`. |
| `public_api_change` | Reports exported declarations. A baseline suppresses known declarations so new surface remains visible. Requires syntax analysis. Defaults to `warn`. |
| `public_api_type_leak` | Finds exported Go fields or return types that expose external package types. Requires syntax analysis. Defaults to `warn`. |

`public_api_max`, `public_api_change`, and `public_api_type_leak` cannot establish conformance without syntax evidence. Required rules remain unevaluated when that evidence is missing.

## Path and module selectors

Path globs use repository-relative paths and doublestar matching. Their meaning depends on the rule and language graph. A dependency rule's `from` can match an importing file or node. Its `to` matches a target node. A Go `from` needs a package path glob, such as `internal/domain/**`. A Go file glob does not identify the importing package. Python selectors use dotted module names. Rust selectors can name crates or `crate::mod` nodes when Cargo module evidence exists.

`from_module` and `to_module` accept these selectors:

| Selector | Selects |
| --- | --- |
| `layer:<name>` | Declared modules with that layer. |
| `role:<role>` | Declared modules with that role. |
| Any other string | A doublestar glob over declared module names. A plain module name selects that module. |

Module selectors do not select auto-registered modules. Edges within one declared module do not match module selectors.

### Selectors that match nothing

A dead selector cannot prove that a rule passes. A fail-gated rule remains unevaluated and can make `check` return exit code 2. A warn-gated rule produces a configuration warning. `archfit config lint` reports dead selectors as errors.

This check applies to `from` and `to` in `forbidden_dependency`, `public_api_only`, and `internal_api_access`, and to `from` in `forbidden_pattern`. A target selector that names an external package is still evaluated as an external dependency ban. A brace glob of external names is one too. A selector that names first-party source but matches nothing is dead. A `to` path that ends in a file type no dependency producer analyzes, such as `src/**/*.vb`, leaves the rule unevaluated; the owner must change the rule.

Dependency selectors must use the endpoint spelling of the language graph. Go `from` matches the importing package. Go `to` matches a target package. Python uses dotted module names. A .NET `to` matches a file path or an external assembly name, such as `Microsoft.EntityFrameworkCore*`. An external assembly target is the bare assembly name: the graph node is `external:<assembly>`, but a selector names the endpoint path, so `to: Npgsql` fires and `to: external:Npgsql` matches nothing. A selector prefixed with any other node kind (`file:`, `package:`, `module:`, `project:`, `repo:`) is dead for the same reason: `config lint` reports it, and a required rule that carries one stays unevaluated. Rust crate module selectors need Cargo module evidence. A rule can remain unevaluated when its selector matches source that no active dependency producer analyzes.

Set `guard: true` when an empty selector is intentional, such as a ban on a deleted path returning. A guard counts as evaluated while a selector is empty. If the guarded path appears again, the rule becomes a normal rule and needs complete evidence.

```yaml
rules:
  - id: no_legacy_reports
    type: forbidden_dependency
    from: internal/**
    to: internal/legacy/reports/**
    gate: fail
    guard: true
```

## Module allowlists

The `module_dependencies` rule enforces module `depends_on` and `visible_to` lists. See [Configuration: modules](configuration.md#layers-and-modules) for the field definitions.

```yaml
modules:
  billing:
    paths: [billing/**]
    visible_to: [shipping, api-gateway]
  shipping:
    paths: [shipping/**]
    depends_on: [billing, shared-kernel]
  shared-kernel:
    paths: [shared/**]
    depends_on: []
rules:
  - id: module_boundaries
    type: module_dependencies
    gate: fail
```

An absent `depends_on` leaves outbound dependencies unconstrained. An empty list denies dependencies to all declared modules. An absent `visible_to` permits all declared importers. An unowned importer is always denied when the target module sets `visible_to`.

External targets and unowned first-party targets are outside `depends_on`. Use `forbidden_dependency` to ban those targets. Allowlist entries can use module names, `layer:` or `role:` selectors, or module-name globs. An entry that matches no declared module allows nothing.

The rule reports each denied ordered module pair. When both lists deny a pair, the finding names both lists. Only production dependency edges count. If no module declares either list, the rule has nothing to enforce and remains unevaluated.

## Forbidden patterns

`forbidden_pattern` uses ast-grep to reject syntax in production files. Each pattern has an ID, language, and ast-grep rule.

```yaml
rules:
  - id: domain_no_clock
    type: forbidden_pattern
    from: internal/domain/**
    gate: fail
    patterns:
      - id: clock_call
        lang: go
        rule: time.Now()
```

The `from` selector matches a repository-relative file path or the file's module selector. Omit it to scan all production files. Test, generated, and vendored files do not produce findings. Excluded files and languages switched off without an explicit gate are outside the rule scope.

The ast-grep pass must complete for the rule to be evaluated. A missing `sg` tool or rejected pattern leaves required rules unevaluated. Test patterns with `sg run --lang <lang> --pattern '<rule>' .` before gating on them. A Go call pattern with exactly one argument, such as `os.Getenv($KEY)`, parses as a type conversion and matches no calls.

Findings identify the pattern and file locations. They do not include matched source text. Pattern IDs must be unique across all rule entries.

## Waivers

Waivers accept findings until a UTC expiry date. They require a rule ID, reason, approver metadata, and expiry.

```yaml
waivers:
  - rule: domain_no_http
    from: internal/domain/legacy/**
    to: internal/http/**
    reason: Replace this dependency during the migration.
    approved_by: "@team-domain"
    expires: "2026-12-01"
```

| Field | Requirement |
| --- | --- |
| `rule` | Required declared or supported synthetic rule ID. |
| `from` | Source selector. Edge-based findings need `from` or `to`. |
| `to` | Target selector. Module-only findings match module names. |
| `reason` | Required explanation. |
| `approved_by` | Required reviewer or owner metadata. Archfit does not authenticate this value. |
| `expires` | Required `YYYY-MM-DD` date. The waiver expires at midnight UTC after that date. |

The edge-less diagnostics `map/uncovered_path`, `map/dead_rule`, and `map/stale_review` require exact rule IDs and no endpoint selectors. `bc/imbalanced_coupling`, `bc/duplicated_knowledge`, and `labels/stale` require endpoint scope. `bc/coupling_gate` and `map/uncovered_change` cannot be waived: unread code is unknown, not accepted.

An active matching waiver takes precedence over an expired match, regardless of YAML order. Active waivers mark findings `waived`. Expired waivers report `expired_waiver`. Invalid selectors or missing metadata reject the configuration before analysis. Baseline capture omits findings covered by active or expired waivers.
