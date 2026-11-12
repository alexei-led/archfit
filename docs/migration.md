# Migrate from Archfit 3 to 4

Archfit 4 reads one file: the policy `.archfit.yaml`, schema version 3. Archfit 3 kept the architecture in four files. `archfit init` converts them in place.

## Convert

Run `archfit init` at the root of a repository whose `.archfit.yaml` has `version: 2`:

```sh
archfit init
```

It does these steps:

1. Reads `.archfit.yaml` with its `waivers`, `.archfit-labels.yaml`, `.archfit-baseline.json` and `.archfit-app.yaml`.
2. Reads the code as `archfit check` reads it.
3. Writes one schema 3 policy over the old `.archfit.yaml`.
4. Writes the five lines for agents between the archfit markers in `AGENTS.md`. They replace the Archfit 3 block.
5. Deletes `.archfit-labels.yaml`, `.archfit-baseline.json` and `.archfit-app.yaml`.
6. Prints what it did not convert, and why.

It writes nothing when a tool the check needs is missing, or when the new policy would not load or fit the code.

It does not read or delete the draft files of Archfit 3's `config enrich`: `.archfit-volatility.yaml`, `.archfit-subdomains.yaml` and `.archfit-owners.yaml`. Nobody approved them. Apply them with Archfit 3 before you convert, or delete them.

## What changes

| Archfit 3                                                                                                         | Archfit 4 policy                                                                             |
| ----------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Module `paths`, `public`, `layer`, `subdomain`, `layers`                                                          | The same keys.                                                                               |
| A bare directory in `paths`, such as `internal`                                                                   | `internal/*`: the files in that directory.                                                   |
| A glob of Go package directories, such as `pkg/*` | `pkg/*/*`: the files of those packages. |
| A Python module name, such as `shop`, `shop.orders`, `shop.orders.*` or `shop.orders.**` | Its files, at the root or under `src/`: `shop/__init__.py`; `shop/orders.py` or `shop/orders/__init__.py`; `shop/orders/**` for both wildcards, because Archfit 3 let `*` cross dots. A namespace package with no `__init__.py` had no node in Archfit 3 and converts to nothing. |
| Module `depends_on`                                                                                               | The same module names. `layer:`, `role:` and name globs become the module names they select. |
| No `depends_on` (any module)                                                                                      | The modules the module uses today.                                                           |
| `volatility: frozen` or `legacy`                                                                                  | `low`.                                                                                       |
| `visible_to`, `internal`, `owner`, `deploy_unit`, `role`, `reviewed_*`                                            | Dropped. `depends_on` and `public` cover the first two.                                      |
| `forbidden_dependency` with `alternatives`                                                                        | A `deny` rule. `rationale` becomes `why`, `alternatives` become `instead`.                   |
| `forbidden_dependency` without `alternatives`                                                                     | A `note`. Nothing checks it until an owner writes `instead`.                                 |
| `module_dependencies`, `new_cross_module_dependency` | The built-in rule `graph`. Its rationale is printed, not kept. |
| `module_cycle`, `cycle` | The built-in rule `cycle`. |
| One of these rules with `gate: warn` | The built-in rule at its own mode: `graph` ask, `cycle` and `layers` deny. |
| One of these rules with `gate: off` | The built-in rule `off`, when every rule that maps to it is off. |
| `forbidden_layer_direction`                                                                                       | The built-in rule `layers`.                                                                  |
| `public_api_only`, `internal_api_access`                                                                          | Dropped: `coupling` and `public` cover them.                                                 |
| `public_api_max`, `public_api_change`, `public_api_type_leak`, `forbidden_pattern`, a `forbidden_dependency` with `gate: warn` | A `note` with the old rationale. |
| A rule id like `D-12`                                                                                             | A name from the type and the id: `forbidden-dependency-d-12`.                                |
| `waivers`                                                                                                         | Exceptions. `reason` becomes `why`; `expires` stays.                                         |
| The `accepted` list of `.archfit-baseline.json`                                                                   | Exceptions, see below.                                                                       |
| `.archfit-labels.yaml`                                                                                            | Dropped: the `coupling` rule takes the strength of a use from `public`.                      |
| `.archfit-app.yaml` `owners`, `gate`                                                                              | `owners`, `gate`.                                                                            |
| `languages`, `analyzers.syntax`, `analyzers.cargo_modules`, `analyzers.roslyn`, `coverage`, `exclude` | The same keys. Other analyzers and settings are dropped. |
| `coupling`, `metrics`, `module_review`, `ai`, `outputs`, `target_branch` and other keys                           | Dropped.                                                                                     |

A key `init` does not convert is printed with its old value, such as a waiver's `approved_by` or a rule's `patterns`, so the report keeps what the policy loses.

A bare directory keeps its Archfit 3 meaning. Archfit 3 matched `internal` as one package, so a package below it was not in the module. In Archfit 4 that code is still in no module, and the `unowned` rule reports new code there.

### Exceptions

Every error the new policy finds in the code today gets an exception with no date, so the first check passes. The policy declares the `tests` and `generated` paths it finds in the code, as for a new policy, so the rules skip that code: a literal that only a test copies is no error and gets no exception. The reason says where the error comes from:

- `Accepted in the Archfit 3 baseline.` when the baseline accepted errors of that rule.
- `The error existed when archfit init wrote this policy.` for the other errors.

An item of the baseline's `accepted` list names a fingerprint, not a place, so `init` cannot turn one item into one exception. It prints, for each rule, how many exceptions it wrote and how many items the baseline accepted. A `waivers` item becomes an exception with its own date and reason.

## Read the output

The first line names the rules that wait for an owner's `instead`. The list below it names every key that `init` dropped or changed:

```text
not checked until an owner writes instead (Archfit 3 named no alternatives): no-chaos-to-runtime
not converted or changed:
  coupling: dropped: the coupling rule replaces it: a new use across modules needs a balance of at least 7
  modules.*.owner: dropped (6 modules): Archfit 4 has no such key
  rules[3] (no-chaos-to-runtime): written as a note: Archfit 3 named no alternatives, and a deny rule needs instead
  exceptions.duplication: 4 written for the errors of today; the Archfit 3 baseline accepted 0
```

## Finish

1. Read the new `.archfit.yaml`: the modules, `public`, `depends_on` and the exceptions. Many `coupling` or `duplication` exceptions are errors that Archfit 3 reported as advice only.
2. For each rule in the first line of the output, write `instead` and change the `note` into a `deny` rule:

   ```yaml
   rules:
     - id: no-chaos-to-runtime
       deny: { from: pkg/chaos/**, to: pkg/runtime/** }
       why: Chaos says what to do through the container.Client interface.
       instead: Call the runtime through container.Client.
   ```

3. Run `archfit check`. It passes on the converted code. Errors in code that is in no module get no exception: `init` prints each one, with its file and line.
4. Open one pull request with the new policy and the deleted files. An owner approves it.
5. Move the workflow to Action 4 in the same pull request.

The Archfit 2 to 3 migration is in [the archived page](archived/migration-v2-to-v3.md).
