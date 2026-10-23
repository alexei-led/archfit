# Migrate from v2 to v3

Version 3 changes the Go module path, baseline format, and report behavior. Keep a copy of your configuration and baseline before you upgrade.

## Install v3

Install the v3 module path:

```sh
go install github.com/alexei-led/archfit/v3/cmd/archfit@v3.0.1
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
