# CI

Use `archfit check` as the CI gate. It is the command that should decide
whether a pipeline passes or fails.

## 1. The gate command

Run `archfit check` after checkout and tool setup:

```sh
archfit check -c .archfit.yaml
```

Keep the config path explicit in CI, even when the file lives at the default
path. That makes the job easier to read and copy.

`archfit check` exits with CI-friendly status codes:

| Exit code | Meaning                                                                                                                       | Typical CI action                               |
| --------- | ----------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| `0`       | `healthy`. Every dimension is measured, hard gates pass, and no diagnostic is active.                                         | Continue the job.                               |
| `1`       | `blocked`. A hard gate failed, or `--require-tools` turned a missing analyzer into a hard failure.                            | Fail the job.                                   |
| `2`       | `needs_attention`. Nothing blocking, but a diagnostic, partial/unmeasured dimension, or unevaluated required rule is present. | **Do not fail the job by default** — see below. |
| `3`       | Usage, config, or runtime/tool error.                                                                                         | Treat as CI infrastructure or config failure.   |

The exit code IS the architecture-state verdict — nothing else participates.

Exit `0` is reachable when all nine dimensions have complete evidence, hard
gates pass, and no diagnostic is active. Exit `2` is still common while adopting
the evidence contract: omitted supplied coverage leaves `testability` partial,
a missing or incomparable persisted baseline leaves `drift` unmeasured,
missing deploy corroboration or ownership statements leave `operations`
partial, and a fail-gated rule without enough producer evidence is listed under
`decision.unevaluated_required_rules`.
Treat that as an honest yellow result, not as a healthy zero. The recipe below
allows yellow and fails only blocked runs; require exit `0` instead after your CI
supplies every required fact.

## 2. GitHub Actions recipe

Minimal gate step:

```yaml
- name: Architecture check
  # Allow needs_attention (2) during adoption; fail blocked (1) and errors (3).
  run: archfit check -c .archfit.yaml || [ $? -eq 2 ]
```

Delta mode compares the current branch against a base ref. In GitHub Actions,
make sure the base ref exists in the local checkout first:

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0

- name: Architecture delta check
  run: archfit check -c .archfit.yaml --base origin/main
```

Use the plain gate for branch protection. Use delta mode on pull requests when
you want the check output to show before/after drift against `origin/main`.
Canonical JSON also adds `origin` to each current finding and repair task:

```yaml
- name: Architecture delta check (machine-readable)
  run: archfit check -c .archfit.yaml --base origin/main --json > archfit-state.json
```

```sh
jq '.agent_tasks[] | select(.origin == "introduced")' archfit-state.json
jq '.comparison.introduced_finding_ids' archfit-state.json
```

`introduced` names work the pull request brought in; `pre_existing` is older
debt. `unknown` means analyzer evidence differed between the two sides, so a
missing analyzer never manufactures a new finding. The classification is
report-only and changes neither the verdict nor the exit code. See
[Origin with `--base`](agent-feedback.md#origin-with---base).

`comparison.origin_status` is `comparable` when the analyzer evidence of both
runs pairs. Otherwise it is `unknown`. A difference in the measurement profile
does not make origins `unknown`. Both sides run one binary over one config
file, so a profile difference comes from the trees and is part of the change.
`comparison.origin_reasons` names each differing producer version and each
degraded analyzer. `comparison.status` is a separate answer: it covers the
fingerprints and the profile. The root `comparison` block describes this
explicit `--base` comparison. The persisted baseline used by the gate is a separate
`gate_reference` block; a base ref never becomes the gate reference.

**Known ceiling — gitignored generated code.** The base side is a checkout of
tracked files only, so a gitignored generated package (protoc, sqlc, wire, or
mockgen output) is absent there. Go imports inside that checkout can then fail to
load; the base `go/packages` row becomes partial and unmatched finding origins
become `unknown`. `comparison.origin_reasons` discloses the cause. If a build
requires generated Go packages, commit the generated package or treat the
origin as unknown. Tools that resolve by
walking up from the file, such as TypeScript through `node_modules`, are not
affected when the checkout remains inside the analyzed repository.

## 3. SARIF upload

Use SARIF when you want GitHub code scanning annotations:

```sh
archfit check --sarif > archfit.sarif
```

GitHub Actions example:

```yaml
- name: Generate Archfit SARIF
  run: archfit check --sarif -c .archfit.yaml > archfit.sarif

- name: Upload Archfit SARIF
  if: always()
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: archfit.sarif
```

Keep `if: always()` on the upload step. That way GitHub still receives findings
when `archfit check` exits `1` or `2`.

## 4. JSON output for scripting

Use JSON when another tool needs to read results:

```sh
archfit check --json -c .archfit.yaml | jq .
```

That is the right mode for CI wrappers, bots, and agent loops. The exit code
still matters. Parse the JSON, but also check the process status.

## 5. Strict tool presence

By default, missing analyzers are surfaced as coverage gaps and do not set the
repository hard gate to `fail`. The run can still be `needs_attention` (exit
`2`) when a dimension or required rule lacks evidence. If CI must fail when a
required tool is missing, turn on the strict gate:

```sh
archfit check --require-tools -c .archfit.yaml
```

Use this when the runner image is supposed to have the full analyzer toolchain
installed and any gap is a CI defect.

One carve-out: a gap whose tool is configured `gate: off` is still reported, but
`--require-tools` does not raise it. An explicit opt-out is not overruled by a
flag. Set that tool's gate to `warn` or `fail` if you want it required.

## 6. Baseline workflow

Commit `.archfit-baseline.json` to the repo. `archfit check` uses that file to
separate accepted current debt from new drift.

Normal CI flow:

1. Run `archfit check -c .archfit.yaml` on every branch and pull request.
2. Do not regenerate the baseline inside the same validation job.
3. Update the baseline only when you intentionally accept the current result.
4. Commit the new `.archfit-baseline.json` in its own reviewable change.

Capture and validation must use the same measurement environment: pin the
analyzer image and platform, toolchain versions, and build settings. A baseline
captured on macOS/arm64 can be non-comparable on Linux/amd64 because build
constraints change which Go files are analyzed. Different analyzer versions or
availability can also change the facts. Prefer a separate trusted CI workflow
that captures the candidate baseline with the same setup as the gate and opens
a reviewable baseline PR.

Update flow, inside that matching environment:

```sh
archfit baseline -c .archfit.yaml
archfit check -c .archfit.yaml
git add .archfit-baseline.json
git commit -m "Update archfit baseline"
```

Baseline capture reads an empty accepted set and accepts only the current tree.
Findings covered by temporary waivers, including expired waivers, are skipped
and the command prints the count; a temporary waiver is never converted into
permanent accepted debt. Review the complete baseline diff before committing it.
A baseline with an incompatible
measurement profile or missing seam snapshot leaves the gate reference
`non_comparable`; do not fix that by blindly re-running `archfit baseline`.
Review the findings and intentionally capture a new baseline only when the
owner accepts the resulting debt. Every baseline captured before v2.3.0 lacks
the measurement profile: accepted findings remain usable, but numerical drift
and seam comparisons abstain until an owner-approved capture supplies the new
identity. Check `gate_reference` explicitly; a non-comparable reference is not
evidence that the PR introduced no new coupling.

After an engine upgrade, the stored reference can become `non_comparable`.
Then use `archfit baseline --reanchor` instead of a full capture. It keeps only
the debt that the stored file accepted, and it prints every difference. New
findings stay `new`:

```sh
archfit baseline --reanchor -c .archfit.yaml
archfit check -c .archfit.yaml
```

If you automate this in CI, do it in a separate manual or scheduled workflow
that opens a pull request with the baseline diff. Do not let a PR job silently
rewrite its own gate input.

## 7. Agent repair loop

Use `archfit check --json` as the machine-readable gate in an automated repair
loop:

```text
archfit check --json -c .archfit.yaml
→ if exit 0 or 2, stop
→ if exit 1, read agent_tasks[]
→ repair the code
→ run archfit check --json -c .archfit.yaml again
```

Each `agent_tasks[]` item is the repair contract for one active gate finding.
Read its `repair_kind`, goal, constraints, files, and validation command.
`code_change` tasks are source repairs; `needs_owner_decision` tasks require a
policy or accepted-debt decision. A forbidden dependency or layer-direction task
never recommends the target module's public API, and a new cross-module
dependency task never uses baseline capture as its repair. The validation
command carries the effective `--base`, `--lang`, and `--require-tools` flags
from the original run. `--refresh` is intentionally not replayed because
cache-control must not change the validation result. Fix only that scope, then
run the command verbatim. The task is done when that run no longer lists its
`finding_id` and the verdict is not `blocked`; exit 2 can remain.

Exit 1 with an empty `agent_tasks[]` is a block no finding carries: a required
analyzer that did not run. A tripped metric ratchet is not one of those: it is a
task for the rule `metric/<name>`, with the values before and after. Fix the
regression, or have an owner review the new value and re-run `archfit baseline`.

If exit `2` includes `decision.unevaluated_required_rules`, resolve the named
producer evidence before claiming a required rule passed. A reason starting
with `selector matches nothing:` is a policy defect, not missing evidence: fix
the named selector, or mark an intentional guard with `guard: true`, in an
owner-approved config change. The array is structured and sorted by `rule_id`;
do not infer it from report prose. Run `archfit config lint` as a fast step
beside `check`: it exits `1` on the same dead selectors before any analyzer runs.
Two exceptions: lint leaves selectors a Rust crate could spell undecided (it
runs no `cargo metadata`), and `check --lang` turns on a language the config
switches off, so its source scope differs from the config lint reads.

## 8. Environment variables and `.env`

`archfit check` itself does not need an Anthropic key. If your pipeline also
runs Anthropic-backed agent or review steps, set `ANTHROPIC_API_KEY` as a real
CI secret:

```yaml
env:
  ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

At startup, archfit also best-effort loads `.env` files from:

- the current working directory;
- paths referenced by `--root`;
- the directory that contains `--config`.

Existing environment variables always win over `.env` values. In hosted CI,
prefer real job environment variables or secret stores over checked-in `.env`
files.
