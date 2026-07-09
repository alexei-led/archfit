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

| Exit code | Meaning                                                                                                           | Typical CI action                                |
| --------- | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ |
| `0`       | Pass. No blocking findings.                                                                                       | Continue the job.                                |
| `1`       | Policy failure. Blocking findings were found, or `--require-tools` turned a missing analyzer into a hard failure. | Fail the job.                                    |
| `2`       | Warning verdict. No blocking findings, but the run is still non-green.                                            | Fail or mark unstable, depending on your policy. |
| `3`       | Usage, config, or runtime/tool error.                                                                             | Treat as CI infrastructure or config failure.    |

In short: `1` means the architecture policy rejected the change. `3` means the
job itself is misconfigured or could not run correctly.

## 2. GitHub Actions recipe

Minimal gate step:

```yaml
- name: Architecture check
  run: archfit check -c .archfit.yaml
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

By default, missing analyzers are surfaced as coverage gaps instead of hard job
failures. If CI must fail when a required tool is missing, turn on the strict
gate:

```sh
archfit check --require-tools -c .archfit.yaml
```

Use this when the runner image is supposed to have the full analyzer toolchain
installed and any gap is a CI defect.

## 6. Baseline workflow

Commit `.archfit-baseline.json` to the repo. `archfit check` uses that file to
separate accepted current debt from new drift.

Normal CI flow:

1. Run `archfit check -c .archfit.yaml` on every branch and pull request.
2. Do not regenerate the baseline inside the same validation job.
3. Update the baseline only when you intentionally accept the current result.
4. Commit the new `.archfit-baseline.json` in its own reviewable change.

Update flow:

```sh
archfit baseline -c .archfit.yaml
archfit check -c .archfit.yaml
git add .archfit-baseline.json
git commit -m "Update archfit baseline"
```

If you automate this in CI, do it in a separate manual or scheduled workflow
that opens a pull request with the baseline diff. Do not let a PR job silently
rewrite its own gate input.

## 7. Agent repair loop

Use `archfit check --json` as the machine-readable gate in an automated repair
loop:

```text
archfit check --json -c .archfit.yaml
→ if exit 0, stop
→ if exit 1, read agent_tasks[]
→ repair the code
→ run archfit check --json -c .archfit.yaml again
```

Each `agent_tasks[]` item is the repair contract for one active gate finding.
Read its goal, constraints, files, and validation command. Fix only that scope,
then re-run the check.

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
