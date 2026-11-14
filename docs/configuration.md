# Configuration

The policy is the file `.archfit.yaml` at the root of the repository: modules, allowed dependencies, rules, and exceptions. `archfit check`, `archfit can`, and `archfit rules` read policy version 3, described on this page. The decoder rejects unknown keys and names the key of each problem. A version 2 file is the Archfit 3 format: see [Version 2 (Archfit 3)](#version-2-archfit-3).

The smallest valid policy:

```yaml
version: 3
owners: [alexei-led]
modules:
  app:
    paths: [cmd/**, pkg/**]
```

A typical one:

```yaml
version: 3
owners: [alexei-led]
gate: required              # default: advisory
layers: [core, cmd]         # inner first
modules:
  pkg_chaos:
    paths: [pkg/chaos/**]
    public: [pkg/chaos, pkg/chaos/cliflags]
    volatility: high
    layer: core
    depends_on: [pkg_container, pkg_util]
  pkg_container:
    paths: [pkg/container/**]
    layer: core
  pkg_util:
    paths: [pkg/util/**]
    volatility: low
    layer: core
rules:
  - id: no-chaos-to-runtime
    deny: { from: pkg_chaos, to: pkg/runtime/** }
    why: A direct import ties a chaos action to one runtime.
    instead: Add a method to container.Client.
  - id: fan-out-through-runner
    note: Chaos actions fan out through RunOnContainers.
    why: One place checks the target limit before injection.
exceptions:
  - rule: duplication
    from: pkg_chaos
    to: pkg_util
    expires: 2026-11-19
    why: "Request #431 is open."
tests: ["**/*_test.go"]
```

An author needs four ideas for a first policy: module, `depends_on`, `public`, and `volatility`. The whole file is protected: `check` compares it with the policy of the target branch, because a setting can hide code from the check.

## Top-level keys

| Key | Type | When absent | Purpose |
| --- | --- | --- | --- |
| `version` | `3` | Required. | The policy schema version. |
| `owners` | list of GitHub logins | Required, at least one. | The people who write the policy and answer requests. |
| `gate` | `advisory` or `required` | `advisory`: the check reports and does not block. | Whether the check blocks a pull request. |
| `layers` | list of names | No layer rule. | The layers, inner first. |
| `modules` | map | Required, at least one. | The modules, by name. See [Layers and modules](#layers-and-modules). |
| `builtin` | object | The default mode of each built-in rule. | The modes of the built-in rules and `min_balance`. See [Rules](rules.md#built-in-rules). |
| `rules` | list | None. The built-in rules still apply. | The custom rules. See [Rules](rules.md#custom-rules). |
| `exceptions` | list | None. | Permissions to break one rule in one place. See [Rules](rules.md#exceptions). |
| `tests` | list of globs | Only listed paths are out. | Test code, which the rules skip. |
| `generated` | list of globs | Only listed paths are out. A file name that looks generated is not enough. | Generated code, which the rules skip. |
| `exclude` | list of globs | Only the built-in exclusions. | Paths no extractor reads, such as vendored or fixture trees. |
| `languages` | object | Each language is `auto`. | The settings of each language extractor. |
| `analyzers` | object | Each optional analyzer has its own default. | The optional analyzers of the report. |
| `coverage` | object | Off. | Coverage files that your CI writes, for the report. |

## Paths and globs

Write paths relative to the repository root, with forward slashes. Globs use doublestar rules: `**` matches across directories. `.git`, `node_modules`, and `vendor` are always excluded. Add more with `exclude`.

A file belongs to the one module whose `paths` match it. Two modules that own the same file are an error.

## Layers and modules

| Module key | Type | When absent |
| --- | --- | --- |
| `paths` | list of globs | Required, at least one. |
| `public` | list of globs | The whole module is public. Add `public` to protect its inside. |
| `volatility` | `low`, `medium`, or `high` | Taken from `subdomain` when it is present (`core` is high; `supporting` and `generic` are low). Otherwise `high`. |
| `subdomain` | `core`, `supporting`, or `generic` | No business role is recorded. |
| `layer` | a name from `layers` | The module has no layer. |
| `depends_on` | list of module names | The module may depend on no other module. |

- `depends_on` lists the allowed dependencies. The built-in rule `graph` reports any other dependency between modules.
- `public` is the part of a module that other modules may use. The built-in rule `coupling` gives a use past it a lower balance.
- `volatility` is how often the module is expected to change. When both are present, `volatility` decides and `subdomain` describes.
- `layers` lists the layer names, inner first. The built-in rule `layers` reports a dependency from an inner layer to an outer one.

## Languages

The check reads a language where its files exist. Each language has `enabled`: `true`, `false`, or `"auto"` (the default). `false` turns the extractor off. `true` reads as `"auto"` does. When a language is read and its tool is missing, the result is `unknown`.

| Key | Settings |
| --- | --- |
| `languages.go` | `enabled`; `modules.include` and `modules.exclude`: globs of `go.work` members, relative to the root. Absent: every member. |
| `languages.typescript` | `enabled`. |
| `languages.python` | `enabled`; `package`: the top-level importable package. Absent: found from the layout, `src` first. |
| `languages.rust` | `enabled`; `manifest` (absent: the root `Cargo.toml`), `features` (absent: the default features), `include_dev_deps` (absent: `false`). |
| `languages.dotnet` | `enabled`; `projects`: the `.slnx`, `.sln`, `.slnf`, or `.csproj` files to read. Absent: the first `.slnx` or `.sln` at the root, else every `.csproj` outside `obj/`, `bin/`, and excluded paths. |

See [Languages](languages.md) for the tools of each language.

## Optional analyzers

The check runs no optional analyzer. The report reads them.

| Key | Settings | When absent |
| --- | --- | --- |
| `analyzers.syntax` | `enabled` | Off. The ast-grep pass; the report uses it for function size. |
| `analyzers.cargo_modules` | `enabled` | Off. The module graph inside Rust crates; it compiles the crate. |
| `analyzers.roslyn` | `enabled`, `timeout` (a Go duration, such as `5m`) | On where the helper is installed. The .NET type graph. |

Only `true` turns on an analyzer that is off by default.

## Supplied coverage

Archfit never runs tests. The report reads the coverage files your CI writes.

| Key | Meaning |
| --- | --- |
| `coverage.enabled` | Makes the report read the sources. Absent: `false`. It needs at least one source. |
| `coverage.sources[].path` | The coverage file, relative to the root. Required. |
| `coverage.sources[].format` | `auto` (default), `go-coverprofile`, `lcov`, `coverage-py-json`, `llvm-cov-json`, or `cobertura`. |
| `coverage.sources[].sidecar_path` | The file of source hashes. Absent: `<path>.sidecar.json`. |
| `coverage.sources[].max_bytes` | The largest file the report reads. Absent: 64 MiB. |
| `coverage.sources[].max_facts` | The largest number of coverage facts the report keeps. Absent: 1000000. |

## Policy checks

The decoder checks each key on its own: unknown keys, types, closed values, ranges, dates, required keys, and one form per rule. `archfit check` then checks the keys against each other and against the files of the tree. Each problem is one line that names the key:

- a module, layer, or rule name that does not exist, with a suggestion when one is close;
- a path glob that matches no file;
- two modules that own the same files;
- a rule without `why`, a `deny` rule without `instead`, or a rule with two forms;
- an exception for an unknown rule, or with a `to` that its rule does not take.

A policy that fails a check gives the status `unknown`, and the record names the line to fix.

## Schema and editor support

The repository ships [`archfit.schema.json`](../archfit.schema.json), generated from the policy types and their comments.
It describes policy version 3, which Archfit 4 reads.
Archfit 3 reads `version: 2` files, which this schema rejects.
An Archfit 3 repository pins the schema from tag `v3.4.0`:
`https://raw.githubusercontent.com/alexei-led/archfit/v3.4.0/archfit.schema.json`.

Copy the schema beside your policy, then add this comment for YAML language-server support:

```yaml
# yaml-language-server: $schema=./archfit.schema.json
version: 3
```

VS Code can bind the schema in `.vscode/settings.json`:

```json
{
  "yaml.schemas": {
    "./archfit.schema.json": ".archfit.yaml"
  }
}
```

The schema checks keys, types, closed values, ranges, dates, and required keys.
It cannot check names against the rest of the file, such as a `depends_on` entry that names no module.
It cannot check globs against the files in the repository.
`archfit check` does both, and reports a policy that fails them as `unknown` (see [Policy checks](#policy-checks)).

The policy version is separate from the Archfit release version.

## Version 2 (Archfit 3)

A version 2 file is the Archfit 3 format. `archfit report` can still measure it, but it has no repository state until conversion. `archfit init` converts it to version 3; see [Migration](migration.md). The full reference is the [Configuration page of v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/configuration.md). The starter configurations in [`examples/`](../examples) are version 2 files.

### AI provider

The version 2 key `ai` is still decoded for legacy files, but no command uses a model. `archfit init` drops these settings during conversion. See [v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/configuration.md#ai-provider).

### Labels and draft files

Analysis ignores `.archfit-labels.yaml` and the value draft files at every policy version. Volatility and subdomain come from the policy. `archfit init` reads and removes the old labels file during conversion; it leaves unapproved value drafts untouched. See [v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/configuration.md#labels-and-draft-files).

### Module review

The version 2 key `module_review` reviews module owners and review dates. See [v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/configuration.md#module-review).

### Metrics

The version 2 key `metrics` sets metric gates and baseline thresholds. Version 3 has no metric gates. See [v3.4.0](https://github.com/alexei-led/archfit/blob/v3.4.0/docs/configuration.md#metrics).
