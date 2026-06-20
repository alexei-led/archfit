# Pumba architect-skill architecture quick sweep

Date: 2026-06-20  
Target repo: `/Users/alexei/Workspace/pumba`  
Depth: quick-sweep, read-only on source  
Output: `/Users/alexei/Workspace/archfit/reports/archfit-vs-architect-20260620 (openai)/pumba/architect-sweep.md`

No final architecture scores assigned. Full review gates were not run.

## 1. Scope, refs, dirty-state risk

- Reviewed latest working tree state of `/Users/alexei/Workspace/pumba`.
- Current branch/ref: `master`, `HEAD=69b78e6d286cc1da33c192df2a591b0335582bd7`, exact tag `1.1.7`.
- Delta baseline: previous tag `1.1.6` at `6b8d2ba`.
- Tracked source was clean before and after the sweep.
- Dirty-state risk: untracked local analysis/config dirs/files exist: `.archfit-cache/`, `.archfit.full.yaml`, `.archfit.yaml`, `.claude/`, `.codegraph/`. They can affect tools that do not restrict to `git ls-files`. `.gitnexus/` is ignored by `.git/info/exclude`, but archfit still reported `.gitnexus/run.cjs` as a complexity hit in `scan.md:64`.
- Source was not modified. Only this report was written outside the target repo.

## 2. Intent evidence with file refs

- Product purpose: Pumba is a chaos testing and network emulation tool for Docker, containerd, and Podman containers (`README.md:26`).
- Runtime intent: Docker, containerd, and Podman are first-class supported runtimes (`README.md:42-47`).
- Primary architecture intent is explicit in `CLAUDE.md:84-98`:
  - focused `pkg/container` sub-interfaces and request value objects (`CLAUDE.md:86`),
  - runtime factory injection, no `chaos.DockerClient` global (`CLAUDE.md:87`),
  - canonical fanout via `chaos.RunOnContainers` (`CLAUDE.md:88`),
  - generic CLI builder and CLI flag adapter (`CLAUDE.md:90-92`),
  - split Docker/containerd/Podman runtime adapters and sidecar lifecycle (`CLAUDE.md:93-98`).
- Prior modularity design history confirms the current target architecture is a post-refactor state: `docs/modularity-review/2026-04-25/modularity-review.md`, `docs/plans/completed/20260426-modularity-refactor.md`, `docs/plans/completed/20260427-modularity-followups.md`, `docs/plans/completed/20260427-modularity-followup-2.md`.
- Toolchain/dependencies: Go `1.26`; Docker SDK, containerd v2, `urfave/cli` v1, logrus, errgroup (`go.mod:5`, `go.mod:8`, `go.mod:13`, `go.mod:15-16`, `go.mod:79`).

## 3. System map

- **Language/package unit:** single Go module `github.com/alexei-led/pumba`; 18 internal Go packages from `go list ./...`.
- **Composition root:** `cmd/` builds the CLI, parses global flags, selects runtime, and wires chaos command constructors (`cmd/main.go`, `cmd/runtime.go`, `cmd/commands.go:15-77`).
- **Core chaos layer:** `pkg/chaos` owns common command execution, global params, recurring interval loop, and the list/random/parallel fanout helper (`pkg/chaos/command.go:23-100`, `pkg/chaos/runner.go:38-63`).
- **CLI command adapters:** `pkg/chaos/{lifecycle,netem,iptables,stress}/cmd` use `pkg/chaos/cmd.NewAction[P]` to convert flags into typed params and commands (`pkg/chaos/cmd/builder.go:22-51`).
- **Domain port/model:** `pkg/container` defines the `Client` aggregate and focused sub-interfaces for lister/lifecycle/exec/netem/iptables/stress (`pkg/container/client.go:20-59`), plus request/result value objects (`pkg/container/requests.go:11-79`).
- **Runtime adapters:** `pkg/runtime/docker`, `pkg/runtime/containerd`, and `pkg/runtime/podman` implement `container.Client`; Podman intentionally embeds the Docker delegate and overrides divergent paths (`pkg/runtime/podman/client.go:55-83`, `pkg/runtime/podman/doc.go`).
- **Utilities:** `pkg/util` holds shared parsing/validation, including interface-name validation used by both netem and iptables parsers (`pkg/chaos/netem/parse.go:22-55`, `pkg/chaos/iptables/parse.go:34-77`).
- **Deploy units:** release binary, scratch Docker image (`docker/Dockerfile`), nettools sidecar images (`docker/alpine-nettools.Dockerfile`, `docker/debian-nettools.Dockerfile`), Kubernetes/OpenShift DaemonSet examples (`deploy/*.yml`).
- **Test units:** Go unit tests, Bats runtime suites for Docker/containerd/Podman, Go integration tests under `tests/integration/`.

Dependency snapshot:

- Internal packages: 18.
- Internal package edges: 51.
- Top fan-in: `pkg/container` 14, `pkg/chaos` 10, `pkg/chaos/cliflags` 9, `pkg/chaos/cmd` 4.
- Top fan-out: `cmd` 10; each chaos `*/cmd` package 5.
- Largest production directory clusters excluding mocks/tests, grouped by first three path segments: `pkg/runtime/docker` 1503 LOC, `pkg/chaos/netem` 1334, `pkg/runtime/containerd` 1291, `pkg/runtime/podman` 993, `pkg/chaos/lifecycle` 889.

## 4. Tool coverage and commands run

Commands run from `/Users/alexei/Workspace/pumba` unless noted.

Used:

- `git status --short --branch`
- `git describe --tags --exact-match HEAD`
- `git describe --tags --abbrev=0 HEAD^`
- `git diff --stat 1.1.6..HEAD`
- `git diff --name-status 1.1.6..HEAD`
- `git diff --unified=80 1.1.6..HEAD -- pkg/runtime/containerd/client_test.go pkg/runtime/podman/stress_test.go`
- `go list ./...`
- `go list -json ./...` plus Python summary for package edge/fan-in/fan-out counts
- `goda graph -type edges ./...`
- `CGO_ENABLED=0 go test ./...` — passed all packages
- `gitnexus status` — index up-to-date for pumba at `69b78e6`
- GitNexus runtime tools: `gitnexus_list_repos`, `gitnexus_detect_changes(repo=pumba, scope=all)`, `gitnexus_query(repo=pumba, query="stress container runtime sidecar flow")`, `gitnexus_context(repo=pumba, RunOnContainers)`
- `actionlint .github/workflows/*.yml .github/workflows/*.yaml` — no output
- `kubeconform -strict -summary deploy/*.yml` — 3/3 valid schema
- `kube-linter lint deploy/*.yml` — 19 lint findings; some are intentional for a chaos daemon, one selector mismatch is real drift
- `hadolint docker/Dockerfile docker/alpine-nettools.Dockerfile docker/debian-nettools.Dockerfile` — package pinning and minor Dockerfile warnings
- Reads/greps of `README.md`, `CLAUDE.md`, `go.mod`, `Makefile`, `.github/workflows/*`, `.golangci.yaml`, `.pre-commit-config.yaml`, `deploy/*`, `docker/*`, key `cmd/`, `pkg/chaos/`, `pkg/container/`, and `pkg/runtime/` files.
- Reads of existing archfit artifacts in `/Users/alexei/Workspace/archfit/reports/archfit-vs-architect-20260620 (openai)/pumba/`.

Failed or limited:

- `goda cycle ./...` failed: installed goda does not expose a `cycle` subcommand. Used `go list`/`go test` and `goda graph -type edges` instead.
- `staticcheck ./...` failed because installed Staticcheck was built with Go 1.24 while this module requires Go 1.26. Coverage gap for local staticcheck.
- `govulncheck ./...` failed with `internal error: package "golang.org/x/sys/unix" without types was imported from "github.com/containerd/fifo"`. CI installs/runs govulncheck separately (`.github/workflows/build.yaml:40-49`).
- CLI `gitnexus detect-changes` failed without an explicit repo because multiple repos are indexed. Runtime tool with `repo=pumba` succeeded and reported no changes.

Skipped by scope:

- Full architecture scorecard.
- Bats integration suites against real Docker/containerd/Podman.
- Docker builds and Kubernetes deployment apply/dry-run.
- `golangci-lint run` local execution; CI wiring was inspected instead.

Confidence impact: good quick-sweep confidence for source structure, package dependencies, delta, and deploy-manifest schema. Lower confidence for static analysis/vulnerability dimensions due local tool failures.

## 5. Full-current architecture observations

1. **The current code largely matches the post-refactor intent.** The old critical issues from the 2026-04 modularity review are fixed in source: no `chaos.DockerClient` references were found; runtime creation is a closure/factory seam (`pkg/chaos/command.go:23`, `cmd/runtime.go:21-53`); `NetemRequest`, `IPTablesRequest`, `StressRequest`, `StressResult`, and `RemoveOpts` replaced wide positional interfaces (`pkg/container/requests.go:11-79`).

2. **The dependency graph is acyclic and expectedly centered on ports.** `go list ./...`, `goda graph -type edges ./...`, and `CGO_ENABLED=0 go test ./...` passed. The main hubs are intentional: `pkg/container` is the stable port/model package with fan-in 14; `pkg/chaos` is the core execution helper with fan-in 10; `cmd` has fan-out 10 as the composition root.

3. **`cmd` fan-out is composition-root coupling, not automatically a design smell.** `cmd/commands.go:15-77` wires command builders; `cmd/runtime.go:29-56` selects concrete runtime constructors. This is high fan-out but low runtime/deploy distance inside one binary and one owner (`.github/CODEOWNERS:1`). Archfit flags it as medium because module volatility is undeclared; architect review downgrades the immediate severity.

4. **Runtime adapters keep provider-specific details mostly local.** Docker, containerd, and Podman each expose `container.Client` implementations. Podman’s Docker-SDK vocabulary is explicitly documented as intentional, with an override invariant for future `ctr.Client` changes (`pkg/runtime/podman/doc.go`; `pkg/runtime/podman/client.go:55-83`). This is a strong design note, not proof forever; it needs a fitness check or review checklist when `container.Client` grows.

5. **Sidecar lifecycle is high-strength but low-distance.** Docker netem/iptables share `runSidecar`/`runSidecarExec`; cleanup uses `context.WithoutCancel` to avoid leaked sidecars/rules on cancellation (`pkg/runtime/docker/sidecar.go:29-34`, `pkg/runtime/docker/sidecar.go:71-158`). This coupling is cohesive and should stay close.

6. **Behavioral CI is strong; architecture-specific CI is weak.** Build CI runs lint, coverage tests, govulncheck, and Docker/containerd/Podman Bats integration on linux/amd64 and linux/arm64 (`.github/workflows/build.yaml:38-49`, `.github/workflows/build.yaml:79-189`). CodeQL and OSSF Scorecard also run (`.github/workflows/codeql-analysis.yml:41-54`, `.github/workflows/scorecard.yml:21`). None of the inspected CI files enforce module-boundary rules, allowed-dependency rules, or deploy-manifest lint gates.

7. **Operational examples have drift.** `kubeconform` validates schema, but `kube-linter` reports 19 findings. Some are inherent to Pumba’s purpose (`/var/run/docker.sock`, root privileges), but `deploy/pumba_openshift.yml` has selector `matchLabels.name=pumba` while pod labels are `app=pumba` (`deploy/pumba_openshift.yml:7-12`). That sample likely does not behave as intended. Old/unpinned images remain in deploy examples (`deploy/pumba_kube.yml:27`, `deploy/pumba_kube.yml:56`, `deploy/pumba_openshift.yml:16`).

8. **Dockerfile hygiene is not gated.** `hadolint` flags unpinned apt/apk packages in Dockerfiles (`docker/Dockerfile:7`, `docker/Dockerfile:66`, `docker/alpine-nettools.Dockerfile:8`, `docker/debian-nettools.Dockerfile:9`). This is supply-chain repeatability, not core modularity.

## 6. Delta observations since baseline

Baseline: `1.1.6` (`6b8d2ba`). Current: `1.1.7` (`69b78e6`).

- Delta contains one non-merge commit: `cefd5f8 test: harden stress result channel assertions`.
- Changed files: `pkg/runtime/containerd/client_test.go`, `pkg/runtime/podman/stress_test.go`.
- Diffstat: 93 insertions, 50 deletions.
- No production code changed between `1.1.6` and `1.1.7`.
- Delta improves test determinism for stress result channels by adding `requireStressOutput` / `requireStressError` helpers (`pkg/runtime/containerd/client_test.go:49-75`, `pkg/runtime/podman/stress_test.go:89`) and using them in stress sidecar tests (`pkg/runtime/containerd/client_test.go:1420-1473`, `pkg/runtime/podman/stress_test.go:334-430`).
- GitNexus `detect_changes(repo=pumba, scope=all)` reported no working-tree changes against the indexed commit.

Delta architecture risk: low. It changes test harness behavior around existing runtime stress flows, not module boundaries.

## 7. Balanced Coupling relationship records

These are quick-sweep records, not score inputs.

1. **`cmd` composition root → chaos CLI builders and runtime constructors**
   - Level: package/module within one Go binary.
   - Strength: contract/functional. `cmd` knows constructor names and runtime flag names, but uses `chaos.Runtime` and `container.Client` at the execution seam (`cmd/commands.go:15-77`, `cmd/runtime.go:29-56`).
   - Distance: code distance medium (cross package), ownership low (`.github/CODEOWNERS:1`), runtime/deploy distance low (same binary).
   - Volatility: supporting-domain composition logic; medium when runtimes/CLI flags change.
   - Severity: low today. Archfit’s medium rollups are plausible warnings, but same owner/same binary reduce distance.
   - Balancing move: leave wiring in `cmd`; document volatility/module intent. Do not add another abstraction just to reduce fan-out.

2. **Chaos actions → `pkg/container` ports and request model**
   - Level: package boundary.
   - Strength: contract. Focused interfaces plus request value objects hide runtime internals (`pkg/container/client.go:39-59`, `pkg/container/requests.go:20-79`).
   - Distance: package-level, same repo/binary, low runtime distance.
   - Volatility: chaos behavior is core/high; container model is stable/generic-supporting but high fan-in.
   - Severity: low-to-medium. High fan-in means mistakes are broad, but the contract is explicit and tested.
   - Balancing move: add a boundary fitness check so runtime packages do not leak into chaos actions.

3. **`pkg/runtime/podman` → Docker delegate / Docker SDK vocabulary**
   - Level: runtime-adapter package plus external provider API.
   - Strength: model, bordering intrusive via embedded delegate, but intentionally documented (`pkg/runtime/podman/client.go:55-83`, `pkg/runtime/podman/doc.go`).
   - Distance: code distance low inside adapter; provider/runtime distance medium-high because Podman compatibility behavior can drift.
   - Volatility: provider volatility medium; Podman rootless/cgroup behavior is known to vary.
   - Severity: medium watch item, not a rewrite trigger.
   - Balancing move: keep the embedding while override set is small; add contract tests/checklist for every `ctr.Client` method addition. Migrate to a libpod anti-corruption layer only if overridden methods outnumber inherited ones, as the doc already states.

4. **Runtime sidecar flows → host/container runtime privileges**
   - Level: runtime/deploy boundary.
   - Strength: intrusive by design: mounts runtime sockets and creates sidecars in target namespaces (`deploy/pumba_kube.yml:54-98`, `deploy/pumba_kube_stress.yml:49-59`, `pkg/runtime/docker/sidecar.go:71-158`).
   - Distance: high operational distance: host kernel, socket, Kubernetes/OpenShift manifests, sidecar images.
   - Volatility: operational/security volatility medium-high.
   - Severity: medium. The coupling is necessary for a chaos tool, but deploy docs/manifests must make the risk explicit and valid.
   - Balancing move: do not hide the privileged coupling; enforce manifest validation with documented kube-linter exceptions for intentional privileges and fail on accidental drift.

5. **Docker runtime netem/iptables/stress internals → shared sidecar helper**
   - Level: same package/internal helper.
   - Strength: functional; they share sidecar lifecycle and cleanup rules.
   - Distance: low; same package and concern cluster.
   - Volatility: medium due runtime/provider quirks.
   - Severity: low. High strength + low distance is cohesive.
   - Balancing move: leave co-located; keep unit tests around cancellation cleanup and exit-code handling.

6. **Deploy/OpenShift selector → pod template labels**
   - Level: deploy contract.
   - Strength: contract; selector must match pod labels.
   - Distance: deploy-time, outside Go compiler/test loop.
   - Volatility: low functional volatility, but high drift risk because deploy examples are ignored by main build workflow paths (`.github/workflows/build.yaml:1-5`).
   - Severity: medium for deploy sample correctness.
   - Balancing move: fix selector/labels and add `kubeconform` + kube-linter CI for `deploy/**` with explicit exceptions.

## 8. Architect-skill blind spots vs archfit

- This quick sweep did not run SCiP symbol extraction, jscpd clone analysis, lizard complexity metrics, or archfit’s weighted scorecard. Archfit has stronger quantitative coverage for hidden coupling, clone candidates, propagation cost, instability, and structural weight (`scan.md:62-68`, `scorecard.md:14-35`).
- Architect-skill did not assign final scores. Archfit produced scorecards, but this report treats them as comparison artifacts, not final judgment (`scorecard.md:7-43`).
- Local `staticcheck` and `govulncheck` runs failed, so this sweep has weaker local semantic/security evidence than CI or a fixed toolchain run.
- Git churn analysis here is coarse. Archfit/GitNexus have better change-locality and historical blast-radius metrics (`scan.md:61`, `scan.md:68`, `scan.md:91`).
- Manual source reading can miss low-level clone/hidden-coupling pairs that archfit enumerates mechanically.

## 9. Archfit blind spots vs architect skill

- Archfit classified `cmd` → `pkg_chaos` and `cmd` → `pkg_runtime` as medium unbalanced coupling because volatility was undeclared and owner/deploy distance were not reported (`scan.md:36-50`, `scan.md:75-79`). Source review sees `cmd` as the composition root in one binary/one owner; severity should be lower unless volatility or ownership changes.
- Archfit’s complexity metric picked up ignored tool code `.gitnexus/run.cjs:64` (`scan.md:64`). That is not Pumba source architecture.
- Archfit’s `architecture_fitness` is 0/100 because it counts architecture-specific checks only (`scorecard.md:37-39`). That is useful, but easy to misread: behavioral CI is strong (`.github/workflows/build.yaml:38-189`); missing part is boundary/dependency fitness.
- LLM review suggested extracting duplicated logic into shared util/contracts (`llm-review-nocache.md:17-19`). Source intent says residual CLI similarity is intentional around `NewAction[P]` and some high-strength/low-distance sidecar logic should stay together (`pkg/chaos/cmd/builder.go:1-3`, `pkg/runtime/docker/sidecar.go:71-158`). Generic extraction may worsen locality.
- Archfit did not surface deploy-manifest drift found by operational tools: OpenShift selector mismatch, unpinned/old deploy images, and Dockerfile package-pinning warnings.
- Archfit artifacts did not reconcile the older 2026-04 modularity review with current source. Several prior critical issues are already fixed.

## 10. Reliability/repeatability notes

- Source review was read-only. The only write was this report in the archfit reports directory.
- Tool versions observed: `go version go1.26.4 darwin/arm64`, actionlint `1.7.12`, hadolint `2.14.0`, kubeconform `0.8.0`, kube-linter `0.8.3`.
- `go test ./...` was run with `CGO_ENABLED=0`; this intentionally skips race detector/CGO paths. CI uses `make test-coverage` with `CGO_ENABLED=1` and integration runtime suites.
- Persistent GitNexus index for pumba is fresh at current commit. Runtime `detect_changes` reported no changes.
- Repeatability risk: untracked `.archfit*`, `.codegraph/`, `.claude/` and ignored `.gitnexus/` should be excluded or committed deliberately before comparing tool reports.
- Quality self-check:
  - Structure: yes — follows the 12 requested sections.
  - Clarity: yes — separates facts, hypotheses, and tool failures.
  - Usefulness: medium-high — enough evidence for follow-up, not a full score.
  - Repeatability: yes — commands and refs listed.
  - Helpfulness limit: no final score; staticcheck/govulncheck need fixed local toolchain.

## 11. Target architecture/design suggestions and executable fitness checks

Target architecture to keep:

- `cmd` remains the composition root.
- `pkg/chaos` remains the core action orchestration layer.
- `pkg/container` remains the stable port/model package.
- `pkg/runtime/{docker,containerd,podman}` remain provider adapters.
- `pkg/util` remains small shared parsing/validation utilities.
- Deploy examples remain explicit about privileged socket/kernel coupling instead of pretending it is safe generic Kubernetes workload code.

Suggested executable checks:

1. **Commit or formalize architecture config.** Promote reviewed `.archfit.yaml` or equivalent to tracked config, and exclude `.gitnexus/`, `.codegraph/`, generated mocks, and local caches from metrics.
2. **Allowed-dependency check.** Add a CI script using `go list -json ./...` or `goda graph -type edges ./...`:
   - only `cmd` may import `pkg/runtime/*`,
   - runtime packages may import `pkg/container` and external SDKs, not `pkg/chaos`,
   - chaos action packages may import `pkg/container`, `pkg/chaos`, `pkg/util`, and their cmd adapters, not runtime adapters.
3. **Regression grep/ast-grep checks.** Fail CI if `chaos.DockerClient` reappears, if `pkg/chaos/{netem,iptables}` define local interface-name regexes, or if new chaos action `Run()` methods bypass `chaos.RunOnContainers` / `RunOnContainersAll`.
4. **Podman delegate invariant check.** Add a small test or review script that compares methods in `container.Client` with documented Podman override set in `pkg/runtime/podman/client.go`; fail or flag when `container.Client` grows without Podman audit.
5. **Deploy fitness.** Add `kubeconform -strict -summary deploy/*.yml` and `kube-linter lint deploy/*.yml` to CI for `deploy/**`, with explicit exceptions for intended `/var/run/docker.sock` and root/privileged needs. Keep selector/image pinning as hard failures.
6. **Dockerfile/workflow fitness.** Add `hadolint docker/*.Dockerfile docker/Dockerfile` and `actionlint` to CI, with conscious ignores if package pinning is intentionally avoided.
7. **Archfit distance metadata.** Add owner/deploy-unit metadata so composition-root imports do not get over-classified as cross-owner/cross-deploy coupling.

## 12. Next checks

Smallest follow-ups to turn hypotheses into findings:

1. Fix local semantic tools, then rerun:
   - `staticcheck ./...`
   - `govulncheck ./...`
2. Run project CI-equivalent locally where possible:
   - `make lint`
   - `make test-coverage`
3. Run runtime integration suites in the intended environments:
   - `make integration-tests` or the Colima/Podman commands in `CLAUDE.md`.
4. Generate exact clone/hidden-coupling pairs from archfit JSON, then manually classify whether each is real duplication or intentional same-shape command boilerplate.
5. Validate deploy fixes after adjusting exceptions:
   - `kubeconform -strict -summary deploy/*.yml`
   - `kube-linter lint deploy/*.yml`
6. Re-run archfit with tracked/excluded analysis config, then compare whether `.gitnexus/run.cjs` disappears and whether cmd composition-root advisories change after owner/deploy metadata.
