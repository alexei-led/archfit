# Development evaluation

Use this workflow to evaluate archfit itself across supported languages.
Use [Getting started](getting-started.md) to analyze an ordinary target repository.

## Generated contracts

The committed configuration, report, and result schemas must match the Go types.
After a config, report, or result contract changes, run:

```sh
make schema
```

The target regenerates the schemas and checks for drift. Review the schema diff and constraints before you finish.

The exported `internal/model` surface is pinned by `internal/testdata/model_surface.golden`.
After an intentional public contract change, run:

```sh
ARCHFIT_UPDATE_SURFACE=1 go test ./internal/ -run TestModelSurfaceNoDrift -count=1
go test ./internal/ -run TestModelSurfaceNoDrift -count=1
```

Review the golden diff and describe the contract change in the change review.
Do not update the golden to hide an accidental API change.

## Contract fixtures

`internal/model/result/testdata/` holds the check and can JSON examples; `internal/output/record/testdata/` holds their text records. The reportschema tests validate the result examples against `archfit.result.schema.json`.

`internal/reportschema/testdata/` retains five report JSON cases: a new violation, an existing violation, an unmeasured ratchet from profile drift, a dead selector, and missing evidence. They validate against `archfit.report.schema.json`; `new_violation` carries analyzer coverage rows with per-unit `details`, and `missing_evidence` carries a row without them. These exercise the retained report contract, not the check result.

Agents finish with `archfit check` and read the default text record. Exit `0` is `pass`, `1` is `fail`, and `2` is `unknown`, wrong usage, or a tool error. Follow `next:`; exit `2` is not a pass.

## Release

The release workflow starts when a version tag is pushed. It builds the binaries and image.
Then it creates the GitHub release.
Prepare and review the release notes before creating the tag.
The workflow uses the tag annotation as the release notes.
Create an annotated `vX.Y.Z` tag from that file, then push the tag:

```sh
git tag -a vX.Y.Z -F /tmp/archfit-release-notes.md
git push origin vX.Y.Z
```

Do not create the GitHub release by hand. The workflow owns that step.

A release candidate uses a prerelease tag `vX.Y.Z-rc.N`, created and pushed the same way:

```sh
git tag -a v4.0.0-rc.1 -F /tmp/archfit-release-notes.md
git push origin v4.0.0-rc.1
```

The first job runs `scripts/release-channel.sh` on the tag. It gives `final` for `vX.Y.Z` and `prerelease` for
`vX.Y.Z-rc.N`. Each publishing step uses this one answer:

| Effect | `vX.Y.Z` | `vX.Y.Z-rc.N` |
| --- | --- | --- |
| GitHub release | Latest release | Prerelease, not the latest release |
| Image tags | `vX.Y.Z` and `latest` | `vX.Y.Z-rc.N` only |
| Homebrew formula | Updated | Skipped |

The workflow creates no moving major tag for either form.
A tag of a different form stops the run in the first job, before anything is published.
`make workflows` runs `scripts/tests/release_channel_test.sh`, the test of this rule.
The Go module path uses `/v3`. Keep the major suffix in `go.mod` and imports for v3 releases.

## Language contracts

Build the candidate and run the bounded real-tool fixtures:

```sh
make build
make language-contracts
```

The language contract harness runs `archfit check` on Go, TypeScript, JavaScript, Python, Rust, and .NET fixtures.
Each fixture has a schema 3 policy with one `deny` rule. Its `main` branch holds a clean tree, and its change branch adds the forbidden dependency.
The check must report the rule's error and exit 1. An active exception (judged with `--policy`) must pass, and an expired one must fail.
When the change is undone, the check must pass with exit 0.
Go, TypeScript, JavaScript, and Python need only `git` and `ast-grep`. Rust also needs `cargo`. .NET needs the SDK and the Roslyn helper.
The preflight fails on a missing tool. Passing a fixture without its required producer is not evidence of language support.
`--only LANGUAGE` runs one case and checks only the tools it needs.

`make language-contracts` builds the Roslyn helper first. The .NET case runs the real SDK and helper on a copy of `internal/testdata/dotnet`, using `ARCHFIT_DOTNET_ROSLYN` when set, else `.bin/dotnet-roslyn`. It asserts the helper's project statuses and edges. Then it runs the check with a new `ProjectReference` as the forbidden dependency, because the check reads .NET at the project-reference level. The policy lists the four projects that evaluate. With every project, the failed `Broken` project must appear in the record, and the result is never a pass. `--only dotnet` runs that case alone. Build and test the helper on its own with:

```sh
make dotnet-roslyn
make dotnet-roslyn-test
```

`make dotnet-roslyn` publishes `tools/dotnet/archfit-roslyn` to `.bin/dotnet-roslyn`, restoring in locked mode. Set `ARCHFIT_DOTNET_ROSLYN` to that directory. `make dotnet-roslyn-test` runs the helper's xUnit tests against `internal/testdata/dotnet`; the `gates` CI job builds the helper and runs that suite, because the helper ships inside the image. Update `tools/dotnet/archfit-roslyn/src/packages.lock.json` when the helper's dependencies change.

The helper's own project makes archfit's self-scan detect .NET. Without `ARCHFIT_DOTNET_ROSLYN`, `make archfit` reports a `roslyn` coverage gap, raised to fail by `--require-tools`. `.bin/dotnet-roslyn` lies inside the repository, which the extractor rejects, so copy it outside the checkout before you point the variable at it.

Run the evaluation harness unit tests separately:

```sh
uv run --with jsonschema==4.26.0 python -m unittest discover -s scripts/eval -p '*_test.py'
```

## Corpus sweeps

Use disposable checkouts selected by `--workspace-root`.
External analyzers can write caches and build artifacts in target working trees.
Preserve the original repositories and their policy files.
Prepare version-2 configurations in the disposable checkouts before the sweep.

The supported corpus labels and defaults live in `scripts/eval/corpus_sweep.py`.
Use its help for the current options:

```sh
uv run --with jsonschema==4.26.0 python scripts/eval/corpus_sweep.py --help
```

Example with one representative per primary language:

```sh
uv run --with jsonschema==4.26.0 python scripts/eval/corpus_sweep.py \
  --workspace-root /tmp/archfit-corpus-workspace \
  --repos spotinfo,storybook,ccgram,herdr \
  --repeat-repos spotinfo,storybook,ccgram,herdr \
  --format-repos spotinfo,storybook,ccgram,herdr \
  --strict --max-workers 1 \
  --output-dir /tmp/archfit-corpus-eval \
  --summary-file /tmp/archfit-corpus-results.json
```

The harness validates state JSON, command exits, requested format parity, and repeated-run determinism.
Compare repeated state JSON byte for byte.
Do not remove fields to make repeated output equal.

Strict mode fails on unverified cases unless an explicit `--allow-unverified label=reason` accepts that gap.
An accepted gap remains `accepted_unverified`. It is not a pass.
Missing checkouts and toolchains must remain visible.

Keep AI runs optional and separate from deterministic checks.
Use overlay copies for provider-backed configuration review.
Do not let AI output change the candidate policy used for comparison.

## Extractor equivalence

`scripts/eval/extractors` compares today's import extractors with the parser-based ones planned for Archfit 4.
It reports file edges, module pairs, and `unknown` imports for one language and repository.
Run it on a disposable clone, because today's extractors can write caches in the target.

```sh
go build -o /tmp/gofacts ./scripts/eval/extractors/gofacts
PYTHONPATH=scripts/eval uv run python -m extractors.equivalence \
  --lang typescript --root /tmp/clone --config /tmp/clone/.archfit.yaml \
  --gofacts /tmp/gofacts --out /tmp/equivalence
```

## Classify results

| Result | Required response |
| --- | --- |
| Product defect | Record a minimal reproduction and add a regression test. |
| Policy migration defect | Show the lost, changed, or invented setting. |
| Target architecture finding | Record the finding without changing the target to hide it. |
| Tool or environment gap | Name the missing evidence and mark the case unverified. |
| Documentation defect | Correct the explanation or example against current behavior. |

## Review coupling claims

Use the formula and examples in [Coupling](coupling.md) as the current implementation contract.
Use chapter or section references for the book. Do not copy book text into reports.
Separate book-derived measures, implementation policy, report-only evidence, and unsupported claims.
Explain each mismatch with source evidence and a reproducible case.

Keep dated corpus measurements and review reports in the archive.
Write fresh output under `/tmp` or `.archfit-cache/`.

## Trap benchmark

The trap benchmark measures what coding agents do when the shortest path breaks the architecture policy.
It runs trap tasks, setups (arms), and models in disposable clones.
Traps, arms, and default model IDs live in `scripts/eval/traps/definitions.py`.

Each run clones the pinned trap commit and commits the schema 3 policy on its default branch.
The task branch starts from that policy commit. Setup changes are excluded from the measured diff.
The `prose` arm keeps the prose graph and denies `archfit`.
The other arms use the five-line block. The `tool` arms remove the prose graph.
Hook arms use `archfit hook install`, the same hook writer used by `doctor --fix`.
This avoids installing host-wide toolchains during benchmark setup.
A headless `claude -p` agent works on the task. Then `archfit check` judges the result.

| Measure | Meaning |
| --- | --- |
| Forbidden change | A production import that the pinned policy denies |
| Policy edit | A changed `.archfit*` file |
| Unnoticed copy | An identical rule literal in another module with a `pass` |
| Copy unknown | An identical rule literal with no valid check verdict |
| Request calls | `archfit request` commands in the agent tool transcript |
| Text request | The final message asks the owner to decide. This is a text heuristic. |
| Check | The exit code and text status of `archfit check` |

Install and log in to the `claude` CLI. Put `archfit` on `PATH`.
The run needs network access to clone the trap repository.
Each agent run costs money. `--max-budget-usd` caps one run.

```sh
python3 scripts/eval/traps/bench.py --out /tmp/archfit-traps --reps 2 --jobs 3
```

Use `--setup-only` to prepare one clone per arm and save its setup diff without calling a model.
`--out` must be a new directory outside the repository.
It receives `meta.json`, `results.jsonl`, `summary.md`, and the raw logs of each run.
Select runs with `--arms`, `--models name=model-id,...`, and `--reps`.
The agent gets an allowlisted environment, no user settings, and no MCP servers.
The logs replace the values of secret-like variables with `[redacted]`.

## Locked tool and final-image contracts

`tools/manifest.json` owns exact analyzer/SDK versions, including the .NET SDK pin. `make tool-locks` validates it. CI provisions the SDK from the same pin; there is no separate CI version. Node and Python transitive dependencies
are locked and native downloads are checksum-verified. Do not float one CI or image pin separately.

```sh
make tool-locks
make language-contracts
python3 scripts/image_contracts.py --image <immutable-local-image> --docker-context <context> --output-dir <new-output-directory>
```

CI provisioning is Linux-only, requires GitHub Actions environment files, and installs into the
runner-owned prefix named by `ARCHFIT_PREFIX`. The CI job restores that prefix from the Actions
cache keyed on the lock files below and rebuilds it only on a miss, so a pin change re-provisions and a
code change reuses the verified tree. The CI job calls `scripts/ci-provision-tools.sh`; it is not
a general host installer.
The image contract runs `archfit check` on the language-contract fixtures under the default
and arbitrary numeric UID with readonly trusted inputs: each forbidden dependency must give its error and exit 1. The .NET case also analyzes the fixture directly from the readonly mount and checks its tree hash is unchanged. Poisoned target config, `sitecustomize.py` and virtual environments must not run, and the real error must stay. Native amd64 and arm64 release jobs validate their own immutable
platform images before the manifest/version tags are assembled. Emulation is not native proof.

Run the SCIP reader tests with the provisioned interpreter; they do not bootstrap dependencies:

```sh
"$ARCHFIT_TOOL_PYTHON" -I internal/extract/scip/scip_reader_test.py
```

CI runs the architecture check (`make arch-lint`) inside the image built from the change, in the `Complete image (linux/amd64)` job. The check compares the merge base and head under one policy. It reads no stored architecture-state reference and has no re-anchor workflow. Only exit `0` is a pass. The local Make target accepts exit `2` only for the named `policy: differs from origin/main` record, because a policy edit needs owner approval. Other unknown results and tool errors fail the target.

The release identity distinguishes the multiarch index from platform manifests. App/Action pins
must use the matching platform digest, and their strict consumer/capture tests need separate
adoption. Keep policy conversion owner-reviewed rather than changing the policy to pass CI.
