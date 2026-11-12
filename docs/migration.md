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
| Module `depends_on`                                                                                               | The same module names. `layer:`, `role:` and name globs become the module names they select. |
| No `depends_on` (any module)                                                                                      | The modules the module uses today.                                                           |
| `volatility: frozen` or `legacy`                                                                                  | `low`.                                                                                       |
| `visible_to`, `internal`, `owner`, `deploy_unit`, `role`, `reviewed_*`                                            | Dropped. `depends_on` and `public` cover the first two.                                      |
| `forbidden_dependency` with `alternatives`                                                                        | A `deny` rule. `rationale` becomes `why`, `alternatives` become `instead`.                   |
| `forbidden_dependency` without `alternatives`                                                                     | A `note`. Nothing checks it until an owner writes `instead`.                                 |
| `module_dependencies`, `new_cross_module_dependency`                                                              | The built-in rule `graph`.                                                                   |
| `module_cycle`, `cycle`                                                                                           | The built-in rule `cycle`.                                                                   |
| `forbidden_layer_direction`                                                                                       | The built-in rule `layers`.                                                                  |
| `public_api_only`, `internal_api_access`                                                                          | Dropped: `coupling` and `public` cover them.                                                 |
| `public_api_max`, `public_api_change`, `public_api_type_leak`, `forbidden_pattern`, other rules with `gate: warn` | A `note` with the old rationale.                                                             |
| A rule id like `D-12`                                                                                             | A name from the type and the id: `forbidden-dependency-d-12`.                                |
| `waivers`                                                                                                         | Exceptions. `reason` becomes `why`; `expires` stays.                                         |
| The `accepted` list of `.archfit-baseline.json`                                                                   | Exceptions, see below.                                                                       |
| `.archfit-labels.yaml`                                                                                            | Dropped: the `coupling` rule takes the strength of a use from `public`.                      |
| `.archfit-app.yaml` `owners`, `gate`                                                                              | `owners`, `gate`.                                                                            |
| `languages`, `analyzers.syntax`, `coverage`, `exclude`                                                            | The same keys. Other analyzers and settings are dropped.                                     |
| `coupling`, `metrics`, `module_review`, `ai`, `outputs`, `target_branch` and other keys                           | Dropped.                                                                                     |

A bare directory keeps its Archfit 3 meaning. Archfit 3 matched `internal` as one package, so a package below it was not in the module. In Archfit 4 that code is still in no module, and the `unowned` rule reports new code there.

### Exceptions

Every error the new policy finds in the code today gets an exception with no date, so the first check passes. The reason says where the error comes from:

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
  exceptions.duplication: 8 written for the errors of today; the Archfit 3 baseline accepted 0
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

3. Run `archfit check`. It passes on the converted code.
4. Open one pull request with the new policy and the deleted files. An owner approves it.
5. Move the workflow to Action 4 in the same pull request.

The Archfit 2 to 3 migration is in [the archived page](archived/migration-v2-to-v3.md).
