# CI

Use `archfit check` as the CI gate. It returns `0` for `healthy`, `1` for
`blocked`, `2` for `needs_attention`, and `3` for a command, config, or runtime
error. Treat exit `3` as a failure.

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
