# Development evaluation

Use this workflow to evaluate archfit itself across supported languages.
Use [Getting started](getting-started.md) to analyze an ordinary target repository.

## Generated contracts

The committed configuration, state, and agent-result schemas must match the Go types.
After a config, report, or agent-result contract changes, run:

```sh
make schema
```

The target regenerates all three schemas and checks for drift. Review the schema diff and constraints before you finish.

The exported `internal/model` surface is pinned by `internal/testdata/model_surface.golden`.
After an intentional public contract change, run:

```sh
ARCHFIT_UPDATE_SURFACE=1 go test ./internal/ -run TestModelSurfaceNoDrift -count=1
go test ./internal/ -run TestModelSurfaceNoDrift -count=1
```

Review the golden diff and describe the contract change in the change review.
Do not update the golden to hide an accidental API change.

## Contract fixtures

The result contract has five reference cases: a new violation, an existing violation, an unmeasured ratchet from profile drift, a dead selector, and missing evidence. `internal/output/agentout/testdata/cases/` holds each one three ways: `<case>.json` (the agent result), `<case>.txt` (the `check` text), and `<case>.state.json` (the state report, `archfit.architecture-state.v1`, as `check --format json` prints it). A consumer of the state report, such as the App, ingests the `.state.json` files. The reportschema test validates them against `archfit.state.schema.json`. `new_violation` carries analyzer coverage rows with per-unit `details`, and `missing_evidence` carries a row without them. After an intended change, rewrite them with `ARCHFIT_UPDATE_AGENT_CASES=1 go test ./internal/output/agentout/` and review the diff.

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
The Go module path uses `/v3`. Keep the major suffix in `go.mod` and imports for v3 releases.

## Language contracts

Build the candidate and run the bounded real-tool fixtures:

```sh
make build
make language-contracts
```

The language contract harness exercises Go, TypeScript, JavaScript, Python, Rust, and .NET producers.
Read its output for missing tools and unverified cases.
Passing a fixture without its required producer is not evidence of language support.

`make language-contracts` builds the Roslyn helper first. The .NET case runs the real SDK and helper on a copy of `internal/testdata/dotnet`, using `ARCHFIT_DOTNET_ROSLYN` when set, else `.bin/dotnet-roslyn`. `--only dotnet` runs that case alone. Build and test the helper on its own with:

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
  --ai-repos '' --strict --max-workers 1 \
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

The evaluation skill owns the detailed review procedure:
[`skills/archfit-eval`](../skills/archfit-eval/SKILL.md).
Keep dated corpus measurements and review reports in the archive.
Write fresh output under `/tmp` or `.archfit-cache/`.

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
The image contract runs all supported languages and positive optional-tool cases under the default
and arbitrary numeric UID with readonly trusted inputs. The .NET case also analyzes the fixture directly from the readonly mount and checks its tree hash is unchanged. Known-bad compiler/config cases must not
produce valid empty evidence. Native amd64 and arm64 release jobs validate their own immutable
platform images before the manifest/version tags are assembled. Emulation is not native proof.

Run the SCIP reader tests with the provisioned interpreter; they do not bootstrap dependencies:

```sh
"$ARCHFIT_TOOL_PYTHON" -I internal/extract/scip/scip_reader_test.py
```

The release identity distinguishes the multiarch index from platform manifests. App/Action pins
must use the matching platform digest, and their strict consumer/capture tests need separate
adoption. Keep baseline migration owner-reviewed rather than editing a reference to pass CI.
