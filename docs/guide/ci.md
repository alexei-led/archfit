# CI

Use `archfit check` like a linter for architecture drift: run it after normal
build/test setup, fail the job on new gate violations, and hand JSON
`agent_tasks[]` back to coding agents when they need to repair the change.

## Local Makefile loop

Keep the local and CI command identical. A simple target is enough:

```text
.PHONY: archfit
archfit: build
<TAB>.bin/archfit check --config .archfit.yaml --full
```

Then make the full local gate obvious:

```sh
make test
make archfit
```

This repository dogfoods that target in `make all` and in GitHub Actions:

```yaml
- name: CI gate — architecture drift
  run: make archfit
```

## Pull-request gate

Run the gate in CI after checkout and analyzer setup:

```sh
archfit check --config .archfit.yaml --full
```

For pull requests, compare to the base branch when available:

```sh
archfit check --config .archfit.yaml --base origin/main --format json
```

On exit `1`, a coding agent should read `agent_tasks[]`, apply the constrained
repair, and rerun the task's `validation` command. See
[agent-feedback.md](agent-feedback.md).

Store `archfit scan` output as an artifact when a full report is useful:

```sh
archfit scan --config .archfit.yaml > archfit-report.md
```

For GitHub code scanning (inline PR annotations), emit SARIF and upload:

```sh
archfit check --config .archfit.yaml --full --format sarif > archfit.sarif
# then upload with actions/upload-sarif
```

Calibrate locally before adding the check to required CI. Keep early rules narrow
and baseline accepted current findings before treating the check as a merge gate.

## Fail on missing analyzers (coverage gate)

By default a missing analyzer is **warn-loud, exit 0**: the dependent metrics drop
to `n/a` (never scored as healthy) and a coverage gap is surfaced in every format.
A CI job that must guarantee full coverage can opt in to blocking:

```sh
# fail the build if any required analyzer tool is missing
archfit check --config .archfit.yaml --full --require-tools
```

`--require-tools` raises every coverage gap to `fail` and exits `1` (a policy
violation, distinct from exit `3` tool errors). For per-tool control, set
`tools.<x>.gate: fail` in config instead — see
[configuration-reference.md](configuration-reference.md#toolsxgate-coverage-gate).
Install the missing tools (`archfit doctor` lists them) to close the gap rather
than disabling the gate.

## Scan a repo from an external config

When the policy config lives outside the analyzed repo (a central CI config), use
`--root` to point archfit at the repo without planting the config inside it:

```sh
archfit check --root "$GITHUB_WORKSPACE" --config /ci/policies/.archfit.yaml --full
```

Write any report artifacts **outside** the analyzed root — archfit warns when an
output path resolves inside it, and the built-in excludes skip `reports/`,
`.archfit-cache/`, and similar artifact directories to keep scans deterministic.

## Pin CI infrastructure

Keep CI reproducible:

- use explicit runner labels such as `ubuntu-24.04`, not `ubuntu-latest`;
- use `ubuntu-24.04-arm` for native arm64 jobs;
- pin GitHub Actions to released versions, or preferably full SHAs with a version
  comment;
- pin installed tools in CI commands instead of relying on package-manager
  defaults; use [Tooling reference](tooling.md) for current analyzer versions and
  install sources.
