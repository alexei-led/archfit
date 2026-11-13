# Contributing

Use Go 1.27.x, Git, Python 3.12 or later, `uv`, and `pre-commit`.
Language analysis needs the tools listed in [Languages](docs/languages.md).

Analyzer tests need the Go version declared in `go.mod`.
If your default compiler is newer, use `GOTOOLCHAIN=go1.27.0` for these checks.
New compiler export formats can exceed the analyzer's supported format.

## Build and run

```sh
git clone https://github.com/alexei-led/archfit.git
cd archfit
make setup-tools
pre-commit install --install-hooks
make build
.bin/archfit doctor
```

`go.work` is not committed. If you keep one, run `go work edit -go=1.27.0` to match `go.mod`.

`make build` writes a static binary to `.bin/archfit`.
Use `go run ./cmd/archfit --help` while developing a command.

## Check a change

```sh
make fmt
make lint
make test
make arch-lint
```

`make test` runs Go tests with the race detector and coverage.
It also runs the SCIP reader tests, evaluation harness tests, and shell exit-contract tests.
`make test-fast` skips slow integration tests for the inner development loop.

`make test` uses analyzers installed on the host.
Their versions come from the [tool manifest](tools/manifest.json):

| Tool | Required version | How the tests find it | When absent |
| --- | --- | --- | --- |
| dependency-cruiser | 18.7.0 with TypeScript 6.0.3 installed beside it. Version 18.7.0 accepts TypeScript `>=2.0.0 <7.0.0`. | `depcruise` and `node` on `PATH` | Its tests skip. |
| `grpcio-tools` and `protobuf` | 1.84.1 and 7.36.2 | Imported by `ARCHFIT_TOOL_PYTHON` | `make test` runs the SCIP reader tests in an isolated `uv` Python 3.14. |
| `ARCHFIT_TOOL_PYTHON` | Python 3.14 or later with `grimp` 3.17, `grpcio-tools` 1.84.1, and `protobuf` 7.36.2 | Environment variable with an absolute interpreter path outside the checkout | Go tests provision through `uv`, or skip without it. |

When `ARCHFIT_TOOL_PYTHON` is set, the Go and SCIP reader tests use that interpreter and do not fall back.
`make host-tools` uses `npm` and `uv` to install the locked Node and Python environments.
It installs under `~/.local/share/archfit-tools`. Set `HOST_TOOLS_DIR` to choose another directory.
It prints the `depcruise` path and the `ARCHFIT_TOOL_PYTHON` value. It does not edit `PATH` or shell profiles.

Use focused checks during development:

| Change | Check |
| --- | --- |
| One Go package | `go test ./internal/<package>/` |
| Import boundaries | `go test ./internal/ -run TestArchImports` |
| Module ownership | `go test ./internal/ -run TestSelfModel` |
| Model API | `go test ./internal/ -run TestModelSurfaceNoDrift` |
| Report output | `go test ./internal/application/ -run TestGolden` |
| State invariants | `go test ./internal/ ./cmd/archfit/ -run TestErosion_` |
| Published schemas | `go test ./internal/configschema/ ./internal/reportschema/` |
| Portable skill | `go test ./skills/ ./cmd/archfit/ -run 'Skill\|Help'` |
| Workflow files | `make workflows` |

`make arch-lint` runs `archfit check` and accepts exit `0` only.
Before a new dependency between modules, run `archfit can <file> <target>`.
Before finishing, run `archfit check` and follow each record's `next` line.

Add behavior tests for changed logic and regressions.
Mock system boundaries rather than internal decisions.
Review changed golden files before accepting them.

## Keep contracts current

Read [Architecture](docs/architecture.md) before changing package boundaries.
Ask `archfit policy can-import <file> <target>` before adding a cross-module import.
Update the declared module paths when a package moves.

Do not weaken policy, add waivers, change labels, or replace a baseline to make a check pass.
Keep deterministic analysis independent of AI providers.
The [evidence contract](docs/evidence.md) defines completeness and comparison limits.

Run `make mock` after changing interfaces that use generated fakes.
Run `make schema` after changing configuration, report, agent-result, or result contracts.
This command rewrites the published schemas and checks for drift.
Review those changes with their source types.

## Documentation style

Update the topic owner when a command, configuration field, metric, or output changes.
Keep active human documentation directly under `docs/`.
Keep one owner for each fact and link from other pages.
Do not add folder indexes.

- Write one instruction per sentence, with at most 20 words.
- Use at most 25 words in descriptive sentences.
- Use active voice, short paragraphs, and established programming terms.
- Keep identifiers, commands, paths, and quoted output exact.
- Avoid filler, contractions, semicolons, and claims without evidence.
- Use tables for fields, defaults, and comparisons.
- Keep the README focused on the problem and first useful run.
- Keep release history in [CHANGELOG.md](CHANGELOG.md).

Use small Mermaid diagrams when they explain a flow or boundary faster than text.
Use `flowchart LR` for short flows and `flowchart TD` for decisions.
Use these fill, border, and text colors consistently:

| Meaning | Fill | Border | Text |
| --- | --- | --- | --- |
| Neutral or unknown | `#f1f5f9` | `#64748b` | `#0f172a` |
| Process | `#e0f2fe` | `#0284c7` | `#082f49` |
| Healthy | `#dcfce7` | `#16a34a` | `#14532d` |
| Needs attention | `#fef3c7` | `#d97706` | `#451a03` |
| Blocked | `#fee2e2` | `#dc2626` | `#7f1d1d` |

Name the outcome in the node text so color is not the only signal.
Render each changed diagram and inspect its layout.
Check relative links and anchors after moving files.
Check examples against CLI help, source, tests, and the published schemas.

Install the documentation tools with Node.js 24 and npm:

```sh
npm ci --prefix scripts/eval/docs
make docs
```

`make docs` runs checker tests, Markdown lint, local link and anchor checks, and YAML/JSON example checks.
It also renders the active Mermaid diagrams under `.archfit-cache/docs/`.
Inspect those diagrams after a layout change. A successful render does not prove that a diagram is clear.
The first tool install downloads Puppeteer's browser. Later checks use the installed browser with its normal sandbox.

The documentation CI workflow runs the same command.
Archives, release history, and fixture documents do not enter this gate.
Check behavior claims against source and tests. This gate checks document structure and examples, not product behavior.

Archive replaced designs, completed plans, and dated reports under `docs/archived/`.
Preserve unresolved tasks in [Current follow-ups](docs/plans/current.md) before archiving their source.
Keep generated reports and evaluation output outside tracked documentation.

## Evaluation and release

Use [Development evaluation](docs/development.md) for language contracts, corpus sweeps, and coupling reviews.
Run `pre-commit run --all-files` before submitting a change when hooks are installed.
Pin tool versions in repeatable CI examples.

Maintainers release by pushing a version tag.
The release workflow builds Linux and macOS binaries and publishes the multi-platform container image.
The workflow creates the GitHub release. Do not create a second release for the same tag.
