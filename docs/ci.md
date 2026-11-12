# CI

With a version 3 policy, run `archfit check` as shown in
[Run the check without the Action](#run-the-check-without-the-action). It
returns `0` for `pass`, `1` for `fail`, and `2` for `unknown`, wrong usage, or
a tool error.

The rest of this page is for a version 2 policy (Archfit 3). There,
`archfit check` returns `0` for `healthy`, `1` for `blocked`, `2` for
`needs_attention`, and `3` for a command, config, or runtime error. Treat exit
`3` as a failure.

## Run the check without the Action

A pull request is checked against its target branch. `actions/checkout` fetches
one commit and creates no `origin/HEAD`, so give the check the history and the
target branch:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- name: archfit check
  run: archfit check --base "origin/${{ github.base_ref }}"
```

- `fetch-depth: 0` fetches the merge base. Without it the status is `unknown`,
  and the record says to fetch the history.
- `--base origin/<target branch>` names the target branch. Without it, and
  without `origin/HEAD`, the status is `unknown`, and the record says to pass
  `--base`. On a push to the default branch, use
  `--base "origin/${{ github.event.repository.default_branch }}"`: the change
  is then the last commit.
- Exit `2` (`unknown`) is not a pass. Fail the job on any exit code except `0`.
- With `--format sarif`, the output is a SARIF 2.1.0 log for code scanning
  (see [results](results.md)).

Without a workflow that uses the Archfit Action, every result ends with
`policy: not protected`. The check compares `.archfit.yaml` with the policy of
the target branch, but only the Action and a required check on the protected
branch make that comparison a boundary. The Action for a version 3 policy comes
with Archfit 4.

## Go coverage workflow

[The complete GitHub Actions example](../examples/ci-go.yaml) covers one Go
module. It installs Archfit v3.2.0, runs the repository's Go tests with
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
when the project requires complete evidence, and never read exit `2` alone as a pass. Review and commit baselines in a
separate change. Do not generate one in this workflow.

## Adopt the gate

During adoption, fail on exits `1` and `3` and on any change that is not `pass`.
Exit `2` alone is not a pass: an `unknown` change also exits `2`.

```yaml
- name: Architecture check
  run: |
    archfit check -c .archfit.yaml --format agent > archfit.json || [ "$?" -eq 2 ]
    jq -e '.change.status == "pass"' archfit.json
```

Exit `2` can report incomplete evidence or repository attention. Read the result and add
the missing evidence before you require a fully healthy run.

Use this strict gate when the pipeline requires complete evidence:

```yaml
- name: Architecture check
  run: archfit check -c .archfit.yaml
```

You can also use `archfit check --require-tools` to fail when a required
analyzer is missing. A tool with `gate: off` remains optional. See
[configuration](configuration.md) for tool gates.

## Pull request checks

`check` compares the change with the target branch by default (`--base auto`),
so fetch the full history. A shallow clone has no merge-base: the result is
`unknown` with `reason: scope_unresolved` and the exit code is `2`.

```yaml
- uses: actions/checkout@v7
  with:
    fetch-depth: 0

- name: Architecture check
  run: archfit check -c .archfit.yaml
```

If `origin/HEAD` is not set in CI, set `target_branch` in `.archfit.yaml` or
pass `--target`. Pass `--base <sha>` to compare with a fixed commit, for
example the target tip on a merge commit. `--base` classifies current findings
as introduced, pre-existing, or unknown. See [agent workflows](agents.md) for
scope handling.

## Concurrency

Run one archfit job per pull request. Give the workflow a concurrency group
that keys on the PR number:

```yaml
concurrency:
  group: archfit-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: ${{ github.event_name == 'pull_request' }}
```

A new push to a pull request cancels the older run. A push or a dispatched
baseline run on the same ref does not cancel another run, so it cannot drop
that run's upload.

## Cache

The fact cache lives under `.archfit-cache/facts`. Restoring it in CI is not
supported in this release. Do not add a cache step for it.

## Re-check when the target moves

The App can ask for a fresh run when the target branch moves and the change
overlaps the new commits. The setting is `integration.recheck: overlap` in
`.archfit-app.yaml`. The App owns that file and that setting. Configure it
there, not in the workflow.

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
  uses: github/codeql-action/upload-sarif@v4
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

## Complete engine image and App adoption

The v3.2 image bundles the Go, TypeScript/JavaScript, Python, and Rust producers and optional tools. Images after v3.2.3 add the .NET SDK and the Roslyn helper. It does not enable
all optional analysis for every project. Use the [locked tool environment](languages.md#install-analyzers)
and preserve target-specific limitations rather than treating missing evidence as zero.

The App runs the engine through the pinned composite Action, not inside the App service image.
Publishing an engine does not update installed workflows. Adopt the new Action commit, exact engine
version and per-platform digest in a reviewed workflow change after App consumer approval. Required
rules without usable evidence must remain `action_required` in App feedback.

The analyzer container gets no runner secrets and creates its upload token only after analysis.
It is not network-isolated by default. Target dependencies and optional build/indexing code can
use network access; do not forward private package credentials to repair incomplete evidence.
