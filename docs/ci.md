# CI

Use `archfit check` as the CI gate. It returns `0` for `healthy`, `1` for
`blocked`, `2` for `needs_attention`, and `3` for a command, config, or runtime
error. Treat exit `3` as a failure.

## Go coverage workflow

[The complete GitHub Actions example](../examples/ci-go.yaml) covers one Go
module. It installs Archfit v3.1.0, runs the repository's Go tests with
coverage, writes a freshness sidecar, then runs the gate. It does not run
repository tests through Archfit.

The example assumes one declared Archfit module, named `app`, spanning that Go module.
Keep existing architecture boundaries when adapting it. Do not flatten the module map to fit this example.

Before using it, configure the matching module name and coverage artifact in
`.archfit.yaml`:

```yaml
modules:
  app:
    paths: ["**"]
coverage:
  enabled: true
  gate: warn
  sources:
    - path: coverage.out
      format: go-coverprofile
```

Change `ARCHFIT_COVERAGE_MODULE` in the workflow if the declared module has a
different name. Keep the test scope and module paths aligned. This example
attests one module only. It does not represent other modules in a monorepo.
For multiple Go modules, make one coverage artifact and module entry per module
or use a workflow that explicitly combines their source evidence.

The sidecar lists covered files and hashes their current bytes. The example
fails if a profile path cannot map to a file inside the checkout. `source_ref`
records the commit. Archfit checks file hashes for freshness. The sidecar is
unsigned and does not prove who produced the report.

The example installs only Archfit and the Go SDK. Add each optional analyzer
tool required by your `.archfit.yaml`, or disable that analyzer. Keep the tool
versions and build settings stable when you create or reanchor a baseline.
The example allows exit `2` while adopting the gate. Remove that allowance
when the project requires complete evidence. Review and commit baselines in a
separate change. Do not generate one in this workflow.

## Adopt the gate

During adoption, allow exit `2` and fail on exits `1` and `3`:

```yaml
- name: Architecture check
  run: archfit check -c .archfit.yaml || [ $? -eq 2 ]
```

Exit `2` reports incomplete evidence or active diagnostics. It does not report a
healthy result. Read the result and add the missing evidence before you require
a fully healthy run.

Use this strict gate when the pipeline requires complete evidence:

```yaml
- name: Architecture check
  run: archfit check -c .archfit.yaml
```

You can also use `archfit check --require-tools` to fail when a required
analyzer is missing. A tool with `gate: off` remains optional. See
[configuration](configuration.md) for tool gates.

## Pull request checks

Fetch the base branch before you use `--base`:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0

- name: Architecture delta check
  run: archfit check -c .archfit.yaml --base origin/main
```

Use the ordinary check for branch protection. Use `--base` to classify current
findings as introduced, pre-existing, or unknown. This comparison does not
change the verdict or exit code. See [agent workflows](agents.md) for scope
handling.

## Upload SARIF

Upload SARIF so GitHub Code Scanning can show findings on changed lines:

```yaml
permissions:
  contents: read
  security-events: write
```

```yaml
- name: Generate Archfit SARIF
  if: always()
  run: archfit analyze --sarif -c .archfit.yaml > archfit.sarif

- name: Upload Archfit SARIF
  if: always()
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: archfit.sarif
```

Both steps run after a failed gate. The upload step needs the SARIF file, so
check that the generation step ran before the job was cancelled.
Use the separate `check` step to enforce policy. Successful SARIF generation exits `0` even when the report contains blockers.

## Keep a baseline stable

Commit `.archfit-baseline.json` and let CI read it. Do not regenerate it in the
same job that checks a change.

Capture or reanchor a baseline in a separate reviewed change. Use the same
platform, analyzer versions, toolchain, and build settings as the gate. A
different measurement environment can make the baseline non-comparable. See
[migration](migration.md) for baseline version changes.
