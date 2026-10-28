# Migrate from v2 to v3

Version 3 changes the Go module path, baseline format, and report behavior. Keep a copy of your configuration and baseline before you upgrade.

## Install v3

Install the v3 module path:

```sh
go install github.com/alexei-led/archfit/v3/cmd/archfit@v3.2.0
```

Update imports if your Go code imports archfit packages. The `internal/` packages are not a public API.

The configuration schema remains version `2`. Review the full [configuration reference](configuration.md) after upgrade.

## Review configuration changes

- Narrow `public:` globs to the published interfaces. A whole-module public glob can hide coupling inside the module.
- Add `volatility:` or `subdomain:` to modules that need a rated volatility. Missing values can leave coupling evidence partial.
- Replace the distance token `cross_module_same_owner` with `cross_module` in scripts or consumers. Owner and deploy unit no longer change coupling severity.
- Review `role:` values. Roles no longer lower distance or severity.
- Do not use the `metric/` rule ID prefix. It is reserved for metric findings.

The `config init` and `config update` commands no longer generate `public:` globs for Go or TypeScript modules.

## Rebuild an older configuration

Archfit does not migrate a configuration schema older than version 2, or one that still sets the retired `coupling.gate.min_band` or `max_drop`. Do not change only the `version` field. Back up the file, draft a current one, and copy the reviewed policy into the draft:

```sh
cp .archfit.yaml .archfit.yaml.bak
archfit config init --root . --output /tmp/.archfit.yaml
archfit analyze --config /tmp/.archfit.yaml --root .
```

Review the output before you replace the active file.

## Re-anchor a baseline

The v3 engine cannot compare a v2 baseline. Re-anchor it to carry forward matching accepted findings:

```sh
archfit baseline --reanchor -c .archfit.yaml
```

Review every `not accepted` line. Re-anchoring does not accept new findings. Fix each new finding or run a full `archfit baseline` after review.

Then run:

```sh
archfit check -c .archfit.yaml
```

The baseline must be comparable before metric ratchets can evaluate against it. See [Results](results.md#baseline-comparison).

Captures store measured metric references only. An `n/a` placeholder is not a
measured zero and is omitted. Re-anchoring names each stored numeric reference
that it cannot carry because the current metric is unmeasured. Review these
omissions alongside changed metric values. Re-anchoring does not create missing
measurement evidence.

## Update integrations

If a tool consumes JSON, SARIF, or agent output, update it for the v3 contract. The state schema keeps the identifier `archfit.architecture-state.v1`, but some fields changed. See [Results](results.md#json-contracts).

Update the archfit GitHub App or Action to a v3-compatible release. Use the release history for version-specific changes in [CHANGELOG.md](../CHANGELOG.md).

## Python producer semantics

The `grimp.v3` producer records static import literals independently of installed
project dependencies. Every partial grimp result is non-comparable. Older
`grimp.v2` baseline profiles, including profiles where grimp was absent, cannot
be compared with this producer. An architecture owner must review a
`archfit baseline --reanchor` change before stored comparisons resume.
Reanchoring never clears missing producer evidence. See [Python](languages.md#python)
for runtime requirements and extraction limits.

## v3.2 analyzer environment

The complete image adds the Rust and optional analyzer tools. Executing those tools remains
configuration-controlled; generated Rust deep-analysis defaults can now run instead of reporting
missing binaries. Use the same platform, tool environment and settings for capture and checking.

Project-local dependency-cruiser executables and `.dependency-cruiser.*` configurations no longer
control extraction. Move intended architecture scope into reviewed Archfit policy. Keep supported
resolution aliases in data-only `tsconfig` files. Type-only TypeScript imports are now measured.

Corrected producers use `dependency-cruiser.v2`, `ast-grep/syntax.v2`, `scip.v3` and
`scip-symbols.v2` and `cargo-modules.v2`. The profile format stays `archfit.measurement.v2`; equal format versions do not
make producer versions/settings comparable. Preserve the stored baseline and review a reanchor
with the new image. Inspect retained/dropped findings and metric-reference changes. Reanchoring
must not accept new debt or conceal missing evidence.

App/Action adoption is separate from publishing the engine. Update the reviewed workflow's Action
commit, engine version and matching platform digest together after the App approves that tuple.
Stale tuples must remain actionable, not silently neutral or skipped.
