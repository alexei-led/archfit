# Upgrade configuration and baselines

Use this reference when an upgrade changes the configuration schema or baseline
format. The current configuration schema is version 2. The current baseline
schema is version 3.

## Configuration

Archfit does not migrate older configuration schemas. Back up the file, create
a current draft with `archfit config init`, then copy reviewed policy into the
draft. Do not change only the `version` field.

```sh
cp .archfit.yaml .archfit.yaml.bak
archfit config init --root . --output /tmp/.archfit.yaml
```

Validate the draft with `archfit analyze --config /tmp/.archfit.yaml --root .`.
The `--root .` flag analyzes the repository that owns the active config.
Review the output before replacing the active file.

## Baselines

Do not edit a baseline's `schema_version`. The v3 engine cannot compare an old
baseline directly. Re-anchor accepted findings with the current config:

```sh
archfit baseline --reanchor -c .archfit.yaml
```

Review every `not accepted` line. Re-anchoring does not accept new findings.
Fix each new finding, or run `archfit baseline` after owner review. Then run
`archfit check -c .archfit.yaml`.

The state JSON keeps the identifier `archfit.architecture-state.v1`. Its
contract can change between engine releases. See the Results and Migration
pages in the project docs:
<https://github.com/alexei-led/archfit/tree/main/docs>.
