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

The language contract harness exercises Go, TypeScript, JavaScript, Python, and Rust producers.
Read its output for missing tools and unverified cases.
Passing a fixture without its required producer is not evidence of language support.

Run the evaluation harness unit tests separately:

```sh
uv run --with jsonschema python -m unittest discover -s scripts/eval -p '*_test.py'
```

## Corpus sweeps

Use disposable checkouts selected by `--workspace-root`.
External analyzers can write caches and build artifacts in target working trees.
Preserve the original repositories and their policy files.
Prepare version-2 configurations in the disposable checkouts before the sweep.

The supported corpus labels and defaults live in `scripts/eval/corpus_sweep.py`.
Use its help for the current options:

```sh
uv run --with jsonschema python scripts/eval/corpus_sweep.py --help
```

Example with one representative per primary language:

```sh
uv run --with jsonschema python scripts/eval/corpus_sweep.py \
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
