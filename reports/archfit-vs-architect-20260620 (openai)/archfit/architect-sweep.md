# Architect-skill quick sweep — archfit

Generated: 2026-06-20. Mode: read-only source quick sweep; report write only.

No final architecture scores assigned. This is hypothesis + evidence, not a full gated architecture review.

## 1. Scope, refs, dirty-state risk

- Scope: `/Users/alexei/Workspace/archfit`, latest working tree.
- HEAD: `8243a085427adaff5497fc67fb6248698f84a006`, tag `v0.4.1`.
- Delta baseline: `v0.4.0` (`git describe --tags --abbrev=0 HEAD^`), 2 commits behind HEAD.
- Tracked source state after sweep: clean. Untracked `reports/` artifacts are present, including this report.
- Run-summary risk: existing OpenAI archfit artifacts say repo dirty count was `2` and used `.archfit.full.yaml`; that config is not tracked, so those artifacts are not fully reproducible from tracked source alone. Evidence: `reports/archfit-vs-architect-20260620 (openai)/run-summary.json` archfit entry.
- Report target: `reports/archfit-vs-architect-20260620 (openai)/archfit/architect-sweep.md`.

## 2. Intent evidence with file refs

- Product intent: deterministic architecture-drift feedback for AI agents and CI across Go/TS/Python. Evidence: `README.md:12`, `docs/spec/arch-fitness-spec-v0.4.md:15`, `docs/spec/arch-fitness-spec-v0.4.md:41`.
- Agent contract: `agent_tasks` repair blocks and rerunnable checks. Evidence: `README.md:38-64`, `docs/spec/arch-fitness-spec-v0.4.md:136-152`.
- Methodology: Balanced Coupling vocabulary: strength, distance, volatility. Evidence: `README.md:147`, `docs/spec/arch-fitness-spec-v0.4.md:69-85`, `docs/design/arch-fitness-architecture-v0.2.md:10`.
- Gate model: structural rules gate; BC findings and metrics are advisory. Evidence: `docs/design/arch-fitness-architecture-v0.2.md:32`, `docs/design/arch-fitness-architecture-v0.2.md:72`, `docs/guide/dogfooding.md:10-44`.
- Internal invariants: core/model purity, subprocess boundary, LLM off-gate, release/runtime constraints. Evidence: `CLAUDE.md:28-56`, `internal/arch_test.go:81-179`, `cmd/archfit/main.go:32-42`, `cmd/archfit/doctor.go:46`.
- Current dogfood config: module/layer rules and optional full-tool metrics. Evidence: `.archfit.yaml:331-445`, `.archfit.full.yaml` referenced by artifact run-summary.

## 3. System map

- Language/package unit: Go module `github.com/alexei-led/archfit`, `go 1.26`; 56 Go packages from `go list ./...`.
- CLI/deploy unit: one binary, `cmd/archfit`; one runtime Docker image in `Dockerfile`.
- Entrypoint: `cmd/archfit/main.go` defines `check`, `scan`, `score`, `baseline`, `enrich`, `review`, `explain`, `doctor`, `install`, `init`, `update`, `calibrate`.
- Composition root: `cmd/archfit/pipeline.go:72` wires config, scope, extractors, metrics, adapters, labels, and calls `engine.Run`.
- Engine: `internal/engine/engine.go:97` runs extraction, pattern evidence, classification, rules, status, metrics, advisory collection, diagnostics.
- Ports: `internal/ports/ports.go:1`, `:30`, `:44`, `:58`, `:123` define extractor/pattern/symbol/renderer contracts.
- Core/domain packages: `internal/classify`, `internal/rules`, `internal/metrics`, `internal/status`, `internal/staleness`, `internal/facts`, `internal/score`, `internal/model/*`.
- Adapters: `internal/extract/*`, `internal/output/*`, `internal/history/git`, `internal/toolrun`, `internal/labels/labelsio`, `internal/initcfg`, `internal/fitness`, `internal/ownership`, `internal/llm`.
- External analyzers: `go list`, dependency-cruiser, ast-grep, grimp, SCIP, lizard, jscpd, gitnexus; subprocesses are intended to pass through `toolrun.Runner`.
- CI/release: `.github/workflows/ci.yaml` gates lint, govulncheck, tests, `TestArchImports`, and golden output; `.github/workflows/release.yaml` builds binaries and images on tags.

## 4. Tool coverage and commands run

Used:

```sh
git rev-parse HEAD
git describe --tags --always --dirty
git describe --tags --abbrev=0 HEAD^
git diff --stat v0.4.0..HEAD
git log --oneline --decorate v0.4.0..HEAD
go list ./...
python3 /tmp/archfit_summarize.py   # parses `go list -json ./...`
go test ./internal/ -run TestArchImports
go test ./internal/engine/ -run TestGolden
go test ./...
golangci-lint run -c .golangci.yaml ./...
actionlint .github/workflows/ci.yaml .github/workflows/release.yaml
hadolint Dockerfile
zizmor .github/workflows --format plain
GOWORK=off go list -m all
```

Existing artifact parsing:

```sh
python3 -c 'json.load(... full.json / delta.json)'
read reports/archfit-vs-architect-20260620 (openai)/archfit/{scan.md,delta-scorecard.md,doctor.txt,llm-review.md,scorecard.md}
read reports/archfit-vs-architect-20260620 (openai)/run-summary.json
```

GitNexus:

```text
gitnexus_list_repos
gitnexus_detect_changes(repo=archfit, scope=all)
gitnexus_context(runPipeline)
gitnexus_context(Run, file_path=internal/engine/engine.go)
gitnexus_impact(runPipeline, downstream, depth=2)
```

Failed / limited:

- `goda cycle ./...` failed: installed `goda` has no `cycle` subcommand. Import-cycle confidence comes from `go list`, `go test`, and successful full package load instead.
- `staticcheck ./...` failed: Staticcheck was built with older Go (`go1.24`) than module/toolchain (`go1.26`). Not evidence of clean staticcheck.
- `govulncheck ./...` failed: govulncheck source-processing package too old for Go 1.26. Not evidence of clean vulnerabilities.
- `hadolint Dockerfile` returned one warning (`DL3008` unpinned `apt-get install` packages).
- `zizmor` returned findings, not a tool failure: 48 findings, 30 high, mainly unpinned actions and template interpolation in release workflow.

## 5. Full-current architecture observations

### O1 — Core ring is real and currently green

Evidence:

- Intent: `CLAUDE.md:28-37`, `internal/arch_test.go:18-179`.
- Verification: `go test ./internal/ -run TestArchImports` passed; `go test ./internal/engine/ -run TestGolden` passed; `go test ./...` passed; `golangci-lint run -c .golangci.yaml ./...` reported `0 issues`.
- Archfit artifacts: `full.json` and `scan.md` report verdict `pass`, 0 gate findings, 0 warnings.

Judgment: boundary enforcement is stronger than prose-only architecture. The risk is coverage drift in what the ring test lists as adapters; see O4.

### O2 — Package graph is acyclic, but change impact is concentrated

Evidence from `go list -json` summary:

- 56 packages, 183 internal import edges.
- Top fan-out: `cmd/archfit` 40, `internal/engine` 17, `internal/metrics/modularity` 8.
- Top fan-in: `internal/model/diagnostic` 30, `internal/config` 16, `internal/model/graph` 15, `internal/model/finding` 13, `internal/toolrun` 13.
- Top forward reach: `cmd/archfit` reaches 52/56 packages; `internal/engine` reaches 24/56.
- Top reverse reach: `internal/model/graph` 39/56, `internal/model/finding` 35/56, `internal/model/diagnostic` 31/56, `internal/model/coupling` 28/56, `internal/config` 18/56.

Archfit agrees on the shape: `scan.md` reports 0 cycles, 5 blast-radius hubs, propagation cost 0.092, and the same hub family.

Judgment: this is expected for shared diagnostic/model DTOs and a CLI composition root. Keep those hubs stable; don't split them just to lower fan-in.

### O3 — The engine/adapter split is mostly hexagonal

Evidence:

- `internal/ports/ports.go:1` says ports separate engine from adapters; interfaces at `:30`, `:44`, `:58`, `:123`.
- `cmd/archfit/pipeline.go:72-214` wires concrete adapters and calls `engine.Run`.
- `internal/engine/engine.go:97-245` consumes ports, config views, models, metrics, and rules.
- GitNexus: `runPipeline` is called by check/baseline/enrich/explain/review and has downstream impact across 20 modules; `engine.Run` is called by `runPipeline` and heavily tested.

Judgment: `cmd/archfit` is correctly high fan-out. The design keeps concrete tool/process knowledge outside `engine`.

### O4 — Architecture map drifts from adapter vocabulary

Confirmed drift:

- `CLAUDE.md:34` calls `internal/extract/{go,ts,py}` out-of-process adapters.
- `internal/arch_test.go:81-87` includes `internal/extract/` in `adapterPrefixes`.
- `.archfit.yaml:331-337` and `.archfit.full.yaml:197-203` classify `internal/extract` as `layer: engine`, `subdomain: core`, `volatility: high`.

Impact: archfit uses the config as intent. If `internal/extract` is labelled engine/core, Balanced-Coupling distance and volatility are shaped by a map that conflicts with docs/tests. That can suppress or mis-rank advisories even while gate rules still pass.

Balancing move: align `.archfit.yaml` with the actual adapter boundary or document why extractors are intentionally promoted to engine/core.

### O5 — Fitness checks exist, but adapter coverage is incomplete

Evidence:

- Existing checks: `internal/arch_test.go`, `.github/workflows/ci.yaml:52-56`, `.archfit.yaml:350-420`.
- `internal/fitness/fitness.go` is filesystem adapter code (`os`, `filepath`) but `internal/arch_test.go:83-88` adapterPrefixes excludes `internal/fitness`, `internal/initcfg`, and `internal/ownership`.
- `.archfit.yaml:272-292` marks `internal/initcfg`, `internal/fitness`, and `internal/ownership` as adapter modules.

Judgment: this is a fitness gap, not a current import violation. A future core import from those adapter packages may be caught only by the warn-level layer rule, not by the strong ring test.

### O6 — Existing OpenAI archfit artifacts are useful, but have repeatability noise

Evidence:

- `run-summary.json` archfit entry: `scorecard.md` rc 3, bytes 29.
- `reports/.../archfit/scorecard.md` contains `archfit: unknown flag --full`.
- Current code confirms `ScoreCmd` has only `Config` and `Base`; no `Full` flag (`cmd/archfit/score.go:8-17`).
- Docs still show invalid command: `README.md:115`, `docs/guide/dogfooding.md:74` use `archfit score --config .archfit.yaml --full`.
- `full.json` has 140 `file_facts`, including Go build cache pseudo-modules such as `cmd/archfit.test` with `../../Library/Caches/go-build/...`; the Go package graph has 56 real packages.

Judgment: the archfit artifacts are high-value metrics, but their run harness and SCIP/file-fact filtering need cleanup before using the numbers as repeatable baselines.

### O7 — Operational/release architecture has supply-chain exposure not covered by archfit

Evidence:

- `actionlint` clean.
- `zizmor .github/workflows --format plain`: 48 findings, 30 high. High-confidence classes include unpinned action references and template-injection risks in `.github/workflows/release.yaml` at `github.ref_name` uses.
- `hadolint Dockerfile`: `DL3008` warning on unpinned apt packages at `Dockerfile:33`.

Judgment: this is outside archfit's current architecture metrics but inside runtime architecture risk. Release automation is part of the deploy unit.

## 6. Delta observations since baseline

Baseline `v0.4.0`; HEAD `v0.4.1`; commits:

- `e3d8744 ci: bump GitHub Actions to latest versions (#1)`
- `8243a08 docs: sharpen README for AI agents, bump to v0.4.0, add gap-closure notes`

Changed files only:

- `.github/workflows/ci.yaml`
- `.github/workflows/release.yaml`
- `README.md`
- `docs/guide/commands.md`
- `docs/guide/install.md`
- `docs/guide/release-notes.md`

Delta judgment:

- No production Go package changed; static package architecture is effectively unchanged since `v0.4.0`.
- CI/release coupling changed by action version bumps. `actionlint` is clean, but `zizmor` finds supply-chain findings in the changed workflows.
- Docs improved agent-task/post-verify language, but docs still include invalid `archfit score --full` examples while the CLI has no `--full` flag on `score`.
- Archfit delta artifact agrees: `delta.json` verdict pass, 0 gate findings, 6 changed files, 0 cross-module edges from changed files.

## 7. Balanced Coupling relationship records

| Relationship                                                                   | Strength                                                                                                                                                                                                          | Distance                                                                 | Volatility                                                               | Severity                                                                                | Balancing move                                                                                               |
| ------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `cmd/archfit` composition root → `internal/extract/*`, output, history, engine | Functional/concrete wiring. Evidence: `cmd/archfit/pipeline.go:72-214`, fan-out 40.                                                                                                                               | Medium: same binary/deploy, but crosses command-to-internal packages.    | Medium/high: extractors and pipeline evolve with tool support.           | Low. This is intentional composition-root coupling.                                     | Leave close to `cmd`; keep concrete adapters out of `engine`; gate engine→adapter imports.                   |
| `internal/engine` → `internal/ports` → adapters                                | Contract. Evidence: `internal/ports/ports.go:30`, `:44`, `:58`; engine consumes ports, adapters implement.                                                                                                        | Medium: engine and adapters are separate internal packages, same deploy. | Medium: extractor APIs evolve, but contracts are explicit.               | Low. Balanced: explicit contracts at distance.                                          | Keep ports narrow; add tests when changing `Extractor`, `PatternProvider`, `SymbolResolver`.                 |
| Core metrics/classify/rules → `internal/model/*`                               | Model coupling. Evidence: top cluster edges `internal/metrics -> internal/model` 21; fan-in hubs in model packages.                                                                                               | Low/medium: same repo, close internal packages.                          | Low/medium: DTO/model surfaces should be stable, coupling model is core. | Low to medium. High fan-in makes changes costly, but this is cohesive shared model use. | Stabilize model DTOs; split only when one model changes for different reasons than its consumers.            |
| Broad consumers → `internal/config`                                            | Model/contract coupling. Evidence: fan-in 16; `Config` passed via views in `cmd/archfit/pipeline.go`; `.archfit.yaml` drives all gates.                                                                           | Medium: support module imported across core/adapter/cmd.                 | Medium: config evolves with product surface.                             | Medium hypothesis. Config is a blast-radius hub.                                        | Prefer typed per-package views (`ForRules`, `ForExtract`, etc.); avoid passing whole config deeper.          |
| Architecture map → `internal/extract` implementation                           | Model/config coupling between declared intent and code. Evidence: `.archfit.yaml:331-337` says engine/core; `CLAUDE.md:34` and `arch_test.go:81-87` say adapter.                                                  | High conceptual distance: intent map conflicts with test/doc boundary.   | High: extractor modules change with language/tool support.               | Medium confirmed drift.                                                                 | Lower strength by aligning the map; or document and test the exception.                                      |
| Release workflow → external GitHub Actions and tag/ref inputs                  | Intrusive/functional operational coupling: shell scripts interpolate GitHub context and depend on action tags. Evidence: `.github/workflows/release.yaml`, `zizmor` template-injection and unpinned-use findings. | High: external CI marketplace/runtime.                                   | Medium/high: action tags and tag names are outside code ownership.       | High operational risk.                                                                  | Pin actions to SHAs; move `${{ github.ref_name }}` into env with shell-safe quoting; add `zizmor` gate/warn. |

## 8. Architect-skill blind spots vs archfit

- This quick sweep did not reproduce archfit's full SCIP/jscpd/lizard/GitNexus metrics from scratch; it parsed existing artifacts and ran Go/package/CI checks.
- No final scorecard: full review gates were not met and user asked for quick sweep hypotheses.
- No complete symbol-level LSP/reference pass over every package. Graph facts came from `go list`, GitNexus contexts, and existing archfit artifacts.
- No runtime execution beyond tests/linters; no Docker build, no release dry-run.
- No full churn/co-change mining beyond GitNexus freshness and existing archfit artifact data.

## 9. Archfit blind spots vs architect skill

- CLI/docs repeatability: archfit artifacts captured `scorecard.md` failure, but the tool does not point back to stale docs (`README.md:115`, `docs/guide/dogfooding.md:74`) or run-harness misuse.
- Intent drift: archfit trusts `.archfit.yaml`; it does not flag that `internal/extract` is configured as engine/core while docs/tests call it adapter.
- Fitness coverage drift: architecture_fitness detects signals, but not whether `arch_test.go` covers every module declared as adapter in `.archfit.yaml`.
- Operational supply-chain: archfit scan did not report `zizmor` findings or Dockerfile apt pinning risk.
- Artifact hygiene: `file_facts` includes Go build cache/test pseudo-modules, inflating module inventory from 56 Go packages to 140 artifact modules.
- Run reproducibility: existing artifacts depend on `.archfit.full.yaml`, which is not tracked; config hash alone is not enough to reconstruct intent.

## 10. Reliability/repeatability notes

- GitNexus index for `archfit` is fresh at HEAD (`lastCommit` matches `8243a085...`); `detect_changes` reported low risk.
- Go package/test evidence is strong: `go list`, `go test ./...`, `TestArchImports`, and `TestGolden` passed.
- Staticcheck and govulncheck coverage is missing due local tool/toolchain version mismatch. Rebuild tools with Go 1.26 before relying on those dimensions.
- Existing OpenAI archfit artifact `scorecard.md` is invalid because of `--full`; `delta-scorecard.md` is the usable scorecard artifact.
- Quality self-check: structure yes (12 requested sections); clarity yes (facts vs judgments separated); usefulness yes (next checks actionable); repeatability partial (commands included, but untracked `.archfit.full.yaml` blocks exact artifact replay); helpfulness yes (compares architect vs archfit blind spots).

## 11. Target architecture/design suggestions and executable fitness checks

Suggested target adjustments:

1. Treat `internal/extract/*`, `internal/fitness`, `internal/initcfg`, `internal/ownership`, `internal/history/*`, `internal/output/*`, `internal/toolrun`, and `internal/labels/labelsio` consistently as adapters unless there is a documented exception.
2. Keep `cmd/archfit` as the only concrete wiring point for adapters and LLM provider construction.
3. Keep `engine` operating on ports and pure inputs; do not let it import concrete extractors/history/output/labelsio.
4. Keep shared model hubs stable; split `internal/model/*` only around real change vectors, not fan-in aesthetics.
5. Treat release workflow as an architecture surface: it builds and publishes the deploy unit.

Executable checks to add or tighten:

- Extend `TestArchImports` adapter prefix coverage to all adapter-classified modules, including `internal/fitness`, `internal/initcfg`, and `internal/ownership`.
- Add a test or script that compares `.archfit.yaml` layer classifications against the ring-test adapter list. Fail on drift.
- Add a docs/CLI smoke check for README and dogfooding command examples, especially `archfit score` flags.
- Add artifact hygiene assertion: no `file_facts[].files[]` path outside repo root and no Go build-cache pseudo-modules.
- Add CI operational checks: `actionlint` plus `zizmor` (warn first if pinning policy is not approved), and `hadolint` for Dockerfile.
- Rebuild/pin `staticcheck` and `govulncheck` tool versions compatible with `go 1.26`; add them to local/CI tool setup.

## 12. Next checks

1. Re-run archfit with tracked config only:
   `archfit scan -c .archfit.yaml --full` and `archfit score -c .archfit.yaml`.
2. Re-run full-tool artifact generation with a checked-in or archived copy of `.archfit.full.yaml`.
3. Fix docs examples: replace `archfit score --config .archfit.yaml --full` with the current supported form.
4. Run `zizmor` triage on CI/release and decide whether SHA pinning is project policy.
5. Investigate SCIP/file-fact cache leakage; add a regression test if confirmed.
6. Rebuild `staticcheck` and `govulncheck` with Go 1.26 and rerun.
7. Align `.archfit.yaml` extractor layer/subdomain with actual adapter intent, then rerun archfit full + delta to check metric movement.
