# Troubleshooting

Use `archfit doctor` when analysis misses tools or evidence. It reports tools that archfit can find on `PATH`.

## Analysis is incomplete

| Symptom | Cause | Action |
| --- | --- | --- |
| A dimension shows `n/a` or partial coverage. | An analyzer is missing, timed out, or could not resolve source files. | Read the coverage details. Install the named tool or fix the source paths. |
| No source files match module paths. | The root or configured globs do not match the repository. | Check `--root` and each module's `paths`. |
| TypeScript imports remain unresolved. | Aliases or project dependencies are unavailable. | Check `tsconfig` paths and install project dependencies. |
| Python imports remain unresolved. | The package path or active environment is incorrect. | Check `languages.python.package` and install the project dependencies. |
| `sg` exists but pattern analysis fails. | Another program named `sg` appears first on `PATH`. | Run `sg --version`. Install ast-grep if the command is not ast-grep. |

See [Language support](languages.md#analyzer-tools) for tool requirements.

## A command returns an unexpected status

`analyze` is report-only. It can print findings and still return `0` after a successful run. Use `check` for a gate.

| Exit code | Meaning |
| --- | --- |
| `0` | Healthy, or a report-only command completed. |
| `1` | A hard gate blocked the change. |
| `2` | The result needs attention or lacks complete evidence. |
| `3` | Configuration, tool, or runtime error. |

Read [Commands](commands.md#exit-codes) for command-specific behavior.

## Refresh analysis data

`--refresh` bypasses cached extractor results and stores fresh results. Use it after you install or update an analyzer:

```sh
archfit check --refresh -c .archfit.yaml
```

The `--refresh` flag is available on `analyze`, `check`, `baseline`, and `explain`. It is also available on `config enrich`. `config compare` has no refresh flag. Run `archfit analyze --refresh -c .archfit.yaml` before a comparison to refresh the shared cache.

To remove all cached facts and AI responses, delete `.archfit-cache/` beside the configuration file. The next run starts with an empty cache.

## Configuration update reports no known issues

`no_known_issues` means that the checks found no known problem. It does not mean that the configuration is complete or correct. Review module paths, ownership, layers, volatility, and source coverage yourself.

## Configuration update lists `name_drift` or `removed_modules`

These entries need review. An automatic rename or removal can discard `owner`, `subdomain`, `volatility`, `layer`, or `public` values. Edit these module entries by hand.

## Configuration update rejects `--json`

The JSON review cannot run with write or refresh options. Run the review and apply commands separately:

```sh
archfit config update --json -c .archfit.yaml
archfit config update --apply -c .archfit.yaml
```

## `config compare` reports no differences

Read the coverage evidence status. `not_comparable` means that analyzer evidence differs between the runs. The command can report no differences even when the evidence is not comparable.

A higher candidate score does not prove a better configuration. Check for `scored_fraction_fell`, `abstained_edges_rose`, or `external_edges_rose`. These signals mean that the candidate measured less coupling evidence.

## A tool is installed but missing

archfit uses tools found on the current process `PATH`. Check which executable runs first:

```sh
command -v <tool>
which -a <tool>
```

Fix the `PATH` order when the command finds an old or incorrect tool. See [Getting started](getting-started.md#check-the-tools) to run the tool check.

## AI commands fail or send text you do not want to share

AI commands are optional. Deterministic `check` does not need a model. Use environment variables for provider keys. Do not store keys in `.archfit.yaml`.

Provider requests can include repository text. Do not run provider-backed commands when that text contains data you must keep private.
