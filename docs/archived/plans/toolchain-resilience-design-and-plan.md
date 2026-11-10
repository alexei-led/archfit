# Toolchain resilience: design and remaining plan

**Status:** reviewed recommendation. Future implementation requires owner approval.
**Archived:** 2026-11-10. The current plan is the [Archfit 4 design](../../plans/20261110-archfit-4-architecture-change-control.md); its section 11.8 decides the tools.
**Reconciled:** 2026-10-25, after engine v3.2.0 and Action v2.2.0 adoption.
**Scope:** analyzer execution, compatibility, maintenance, and review integration.
**Audience:** engine, Action, and review-service maintainers.

## Recommendation

Keep **one complete analysis image as the long-term default for App-managed reviews**.
Keep the existing architecture. Do not add a toolchain service, resolver, plugin system,
or image-flavor selector.

The App backend remains analyzer-free. Its GitHub workflow runs a pinned image containing
all supported analyzers and their required runtimes. Policy selects required evidence.
Enabling supported evidence must not require installing another analyzer during review.

The repair release and connected adoption are complete. The next development increment
is compatibility automation, not another rebuild of the execution architecture.

A complete image does not guarantee support for unknown future syntax or supply every
project dependency. Unsupported or incomplete required evidence must remain explicit.
A process exit, tool version, or nonempty graph alone does not prove correct analysis.

Reports remain **customer-attested**. Provenance is necessary, not sufficient.
Matching a claimed approved image does not independently verify execution or report contents.

## 1. What has shipped

| Component | Identity | Established status |
| --- | --- | --- |
| Engine v3.2.0 | `faa4eb4e513859567fd6c380230c91ea22fdaab6` | Published. Source CI and native image contracts passed. |
| Linux amd64 image | `sha256:5f2d4a5fff1ace10d7b08937429179631ae0c1d0e1f9e6f37c72b30954cf00db` | Published platform manifest. |
| Linux arm64 image | `sha256:725c3a074358df8ea41881296eda06b07daed7934d7a6d40bb1ecfc3c8a8128d` | Published platform manifest. |
| Action v2.2.0 | `cadf35c0d4885a49a3a1482b876f2a456d20de06` | Published. Same Git tree as reviewed and CI-tested `9e94db2`. |
| App adoption | Release/operator evidence retained separately | Deployed with the new producer approvals. Historical report readability remains separate from approval. |
| Connected workflow adoption | Public engine workflow plus a Python repository | Pin updates merged. Fresh reports reached the App. |

Sources: [engine release](https://github.com/alexei-led/archfit/releases/tag/v3.2.0),
[release workflow](https://github.com/alexei-led/archfit/actions/runs/40172977752),
[image identities](https://github.com/alexei-led/archfit/releases/download/v3.2.0/image-identity.json),
[Action release](https://github.com/alexei-led/archfit-action/releases/tag/v2.2.0),
[Action CI](https://github.com/alexei-led/archfit-action/actions/runs/40175778074).

Release records report 133/133 assertions on each native engine image and 31/31 on each
native Action smoke run. These are bounded fixture results, not universal certification.

### Connected results are not interchangeable

- **Go:** the [fresh main-branch run](https://github.com/alexei-led/archfit/actions/runs/40184059258)
  measured 41/41 declared rules with no active blockers. The App still reports
  `action_required`: four metric ratchets lack a comparable old reference.
- **Python:** the fresh connected report measured all six required rules with no blockers
  or required-rule gaps. The advisory state is `neutral`, with 88 advisories.
  The optional cycle rule remains unmeasured. This is not complete coverage of every capability.
- **Baseline:** no reference was reset to make either result pass. The Go reference migration
  remains an architecture-owner decision.
- **UI:** a separate owner is fixing unavailable-result diagnostics and provenance.
  That work is not deployed or part of this plan's implementation.

The original design and raw experiment records are preserved byte-for-byte in an
operator-owned archive, outside this public repository.
[Public evidence notes](toolchain-resilience-evidence/README.md) retain the method,
normalized findings, and limitations without private service code, customer reports,
deployment identifiers, or machine-specific paths.

## 2. Findings: fixed, remaining, and unproven

| Classification | Evidence | Consequence / disposition |
| --- | --- | --- |
| Fixed: TS tool substitution and missing compiler | Owned Node/depcruise, effective compiler probe, input census, poisoned-target fixtures | Reuse the repaired adapter. Do not add another tool resolver. |
| Fixed: image-time Python provisioning | Existing manifest, hash locks, owned Python environment | No gate-time analyzer installation on the complete-image path. Standalone CLI fallback is a separate behavior. |
| Fixed: Rust attribution and unsupported-target status | Release fixtures and normalized target/package joins | Do not reschedule the old candidate's three failed image assertions. |
| Fixed: standalone Action evidence gap | v2.2.0 guard rejects unmeasured hard gates and required-rule gaps | Optional-only attention remains distinct. |
| Fixed: fresh published measurements | Action report, capture, reanchor, and self-check use `--refresh` | Do not replace the existing fresh external bundle with checkout cache reuse. |
| Fixed: dedicated Rust reader test wiring | Released `scip_reader_test.py` invokes `scip_reader_rust_test.py` | The earlier review looked only at Makefile/CI entry points. That finding is closed. |
| Remaining: local cache trust | A forged cached edge set removed a known violation; refresh restored it | Content hashes do not authenticate cached facts. This was not a demonstrated App gate bypass. |
| Remaining investigation: cache input identity | Go driver and SCIP build/workspace inputs are not fully represented in the reviewed keys | Reproduce cold/warm/refresh differences, then key or bypass the affected cache. |
| Remaining investigation: Rust scope/target completeness | Module extraction selects a library or first binary; nested member scope needs explicit tests | Define the claimed scope before advertising complete workspace/target evidence. |
| Known execution boundary | A target Cargo wrapper ran inside the constrained experiment container | Deep analysis can execute target code. Read-only tools and no credentials are not a sandbox guarantee. |
| Not established: arbitrary grammar detection | Invalid TS retained the known imports without surfacing all parser diagnostics | No missed forbidden import was proved in that case. Do not infer universal completeness. |
| Future improvement: compatibility canaries | Release tests cover fixed fixtures, not every upcoming language feature | Reuse those fixtures in scheduled maintenance CI. No catalog or new service is needed. |

Public source anchors at the reviewed release:

- [TS producer selection and census](https://github.com/alexei-led/archfit/blob/faa4eb4e513859567fd6c380230c91ea22fdaab6/internal/extract/ts/producer.go).
- [Go loading/cache identity](https://github.com/alexei-led/archfit/blob/faa4eb4e513859567fd6c380230c91ea22fdaab6/internal/extract/golang/cache.go).
- [SCIP execution/cache](https://github.com/alexei-led/archfit/blob/faa4eb4e513859567fd6c380230c91ea22fdaab6/internal/extract/scip/scip_strength.go).
- [Rust module targets](https://github.com/alexei-led/archfit/blob/faa4eb4e513859567fd6c380230c91ea22fdaab6/internal/extract/rust/modules.go).
- [Cache replay](https://github.com/alexei-led/archfit/blob/faa4eb4e513859567fd6c380230c91ea22fdaab6/internal/factcache/runner.go).
- [Reader test entry point](https://github.com/alexei-led/archfit/blob/faa4eb4e513859567fd6c380230c91ea22fdaab6/internal/extract/scip/scip_reader_test.py).
- [Action invocation](https://github.com/alexei-led/archfit-action/blob/cadf35c0d4885a49a3a1482b876f2a456d20de06/scripts/run.sh).

## 3. Packaging choice

| Option | Benefit and cost | Recommendation |
| --- | --- | --- |
| A. Repaired full image plus compatibility checks and automation | One environment for mixed-language repositories. Existing adapters and locks remain authoritative. | Long-term default. |
| B. Core Go/Python/TS plus full adding Rust | Possible smaller non-Rust downloads. Requires flavor selection, parity checks, and four platform/flavor identities instead of two. | Not planned without a concrete operational requirement and measured benefit. |
| C. Existing full image plus targeted checks and one reused canary job | Smallest implementation of A. No new runtime subsystem or schema. | Next implementation approach. |

The historical candidate occupied 2.66 GB in Docker's local disk accounting.
Its Rust directory occupied 632,912 KiB. Five existing-image version launches took
0.1485–0.4630 seconds. These are not compressed transfer savings or cold-pull measurements.
No core image comparison was performed.

Use existing build stages and layer caching before creating another supported flavor.
Do not remove compilers or system libraries solely because tiny fixtures do not need them.
Cgo and Rust dependencies can require them.

### Keep / change / delete

| Original proposal | Decision |
| --- | --- |
| Immutable tools, capability distinctions, source accountability | Keep. Existing locks/adapters/contracts cover these responsibilities. |
| Core/full as a likely follow-up | Change. Complete image is the default, not a temporary choice. |
| Generic requirement preflight | Change. Add adapter-local checks only for slow or unclear observed failures. |
| Four permanent compatibility lanes | Simplify to one workflow reusing supported and new-feature cases. |
| New bundle/compatibility catalog | Delete unless existing fixtures and locks cannot express a demonstrated need. |
| Slots, plugins, runtime resolver, independent tool services | Delete from the plan. No measured bottleneck requires them. |
| New report fields or global measurement epoch | Delete from the initial work. Existing reasons and required-rule fields carry the observed failures. |
| Universal dependency-preparation service | Defer. Design one concrete private-dependency case first. |
| Independent heartbeat service and fixed freshness SLA | Defer until an owner approves the operational promise. |

### Component budget

| Component | Observed need and reuse | If omitted | Smallest value test | Maintenance obligation |
| --- | --- | --- | --- | --- |
| Complete locked image, shipped | Tool substitution/provisioning defects; existing build and ecosystem locks | Producer selection can drift | Known forbidden edge with poisoned local tools | Security/tool updates and native platform gates |
| Adapter diagnostics/accounting | Missing compiler, partial files, selected Rust targets | Successful-looking subsets can hide dependencies | One omitted required input among valid inputs | Per-tool scope and coverage semantics |
| Fresh gate execution, shipped | Local cache forgery; existing Action flags and external bundle | Untrusted cache reuse can change facts | Poisoned cache cannot affect a fresh gate | Analysis time; measure before optimizing |
| Targeted local cache repair | Missing effective inputs in source | Warm results can differ from fresh ones | Change one build/driver input on an unchanged tree | Explicit key inputs or documented bypass |
| Scheduled compatibility job | No proactive future-feature result | First user run discovers incompatibility | New-feature fixture has its exact edge or an explicit gap | Fixture updates and named triage owner |
| Existing approval/feedback contracts | Producer adoption and missing evidence | Reports can be rejected or gaps hidden | Exact tuple accepted, retired tuple rejected, required gap actionable | Coordinated release and consumer tests |
| Separate dependency preparation, conditional | Online/offline dependency behavior differs | Private/unavailable inputs remain gaps | One approved dependency case works offline | Credentials, artifact identity, platform compatibility |

## 4. Responsibilities and trust boundaries

```text
MAINTENANCE
existing locks -> image build -> known-answer native contracts -> reviewed release
upstream change -> candidate fixture/result -> maintainer triage (not auto-approval)

REVIEW
protected workflow/policy -> Action -> approved complete image
untrusted source/config/dependencies -> adapters -> facts plus explicit gaps
                                                -> assessment -> report/envelope
                                                -> App admission/protected-input checks
                                                -> published customer-attested result
```

- **Adapters:** own applicability, language requirements, source scope, normalization, and diagnostics.
- **Registry/Runner:** reuse existing construction and process boundaries. Runner is not a security sandbox.
- **Acquisition:** collects facts and producer identity. Cache hits do not increase trust.
- **Assessment:** decides rule-specific sufficiency and comparison. No package installs or LLM decisions.
- **Action:** materializes trusted inputs, runs the approved image, and transports the report.
- **App:** admits claimed identities, checks protected inputs, and publishes outcomes. It does not independently verify facts.
- **Maintenance CI:** proposes tested candidates. It cannot silently replace production pins or accept a baseline.

Keep tightly coupled tools together: Node/depcruise/TypeScript, Python/Grimp,
Rust/rust-src/analyzers, and the engine's linked Go parser/x/tools with its external SDK.
Adding an analyzer needs an adapter, locked dependencies, and fixtures—not another service.

### Language and dependency limits

- **Go:** minimum `go` and suggested `toolchain` differ. The image fixes
  `GOTOOLCHAIN=local`. Replacing an external SDK does not update the linked parser/x/tools.
  Keep import-complete/type-precision-degraded distinctions.
- **Python:** application `requires-python` is not necessarily the static parser requirement.
  Import extraction and SCIP/Pyright support are separate capabilities.
- **TS/JS:** `tsconfig.target` is not the compiler version. Subject resolution settings
  are data. Target executable analyzer config and compiler substitution are not trusted inputs.
- **Rust:** edition, MSRV, lock format, target kinds, metadata, module facts, and semantics differ.
  Metadata success does not establish module or semantic coverage.
- **Mixed workspaces:** name unsupported roots and retained scope. Do not silently select
  per-root tool versions or merge independently produced graphs.
- **Dependencies:** installing analyzers does not acquire all target dependencies.
  Exact public dependency preparation worked offline in one fixture, not every ecosystem.
- **Execution:** build scripts, wrappers, plugins, and macros are untrusted code.
  Keep credentials out of analyzer containers. Network access can permit source exfiltration.
- **Coverage:** account for relevant inputs, not a percentage threshold. One missing file
  can hide a forbidden dependency. Zero edges and legitimate empty scope are valid cases.

## 5. Failure and comparison contracts

Missing required evidence can produce engine `needs_attention`, exit 2,
`hard_gates: unmeasured`, and `unevaluated_required_rules`.
It need not produce engine `blocked` to prevent a passing App gate.
Action v2.2.0 also rejects this case without an App endpoint.

Optional failure, not-applicable scope, empty scope, and an unevaluated required rule
must remain distinct. A completed empty producer does not make a stale required selector valid.

The App's existing verdict maps missing required evidence to `action_required`.
A successful upload job is not a passing architecture result.
Advisory `neutral` with passing required rules is not the same as skipping unavailable required evidence.

Baseline comparison already checks classification/model/labels/rubric fingerprints,
settings, producer membership/status, partial basis, semantics, exact tool versions,
supported profile format, and unknowns.
See [measurement comparison](https://github.com/alexei-led/archfit/blob/faa4eb4e513859567fd6c380230c91ea22fdaab6/internal/assessment/decision/measurement.go)
and [persisted comparison](https://github.com/alexei-led/archfit/blob/faa4eb4e513859567fd6c380230c91ea22fdaab6/internal/assessment/decision/state_comparison.go).

The wire/profile format version is not the entire compatibility decision.
Base-tree comparison and persisted references remain separate paths.
Historical report readability does not approve a historical producer.

Do not invent a second compatibility system. Add missing fact-affecting inputs to
existing identities when demonstrated. Do not hash the whole image and invalidate
unrelated language measurements after an unrelated tool or OS update.

## 6. What remains to do

### First future horizon: one compatibility CI increment

No production implementation is authorized by this document.

1. **Engine:** extend existing language/image contracts with one genuine upcoming feature
   per selected language change and paired positive/negative controls.
   Reuse existing scope discovery and fixture definitions.
2. **Engine CI:** schedule the released image against those fixtures.
   Resolve candidate tool versions to exact locks before candidate execution.
   Save image/source/tool identities, expected versus observed facts, and failure causes.
3. **Operations:** assign a triage owner and cadence. Keep one result artifact and one
   deduplicated failure issue. Distinguish tool incompatibility from missing CI results.
4. **Action/App:** reuse the released contracts. No code or wire change unless an actual
   fixture demonstrates a lost diagnostic or incorrect published outcome.
5. **Stop and review:** demonstrate a deliberate compatibility failure and recovery before
   adding a larger matrix, customer-source polling, or a freshness service.

Acceptance: exact known edges, honest required gaps, no runtime analyzer installation,
no silent pin change, and no missing result labeled supported.

### Separate work, not prerequisites to the canary

| Work | Owner / status |
| --- | --- |
| Four Go metric ratchets with an old reference | Architecture owner reviews reanchor/capture after fresh measurement. Not waived here. |
| Unavailable-result UI diagnostics/provenance | Separate App writer. Keep invalid/stale/unapproved reports hidden while exposing safe actionable context. |
| Local cache input/trust regressions | Engine maintainer reproduces specific cases before changing cache behavior. |
| Rust workspace/target coverage and SCIP partial scope | Adapter maintainer defines capability-specific fixtures and supported scope. |
| Private dependency preparation | Separate owner-approved design for one concrete case. |
| Core/full or independently released producers | No implementation planned without measured need. |

The UI work is not permission to weaken `action_required` or render an unapproved report
as trusted findings. Its deployment and acceptance are tracked separately.

## 7. Verification plan

| Layer | Positive controls | Negative and regression controls |
| --- | --- | --- |
| Adapter | Exact edge/identity; valid zero-edge project | Missing compiler, omitted file, malformed output, timeout |
| Language | Supported grammar and resolution | Higher minimum, unknown edition, one bad file, mixed workspace |
| Execution | Image-owned tool/runtime | Local shadowing, executable config, preload/driver/wrapper injection |
| Dependencies | Prepared inputs retain expected facts offline | Missing dependency, wrong platform/source, unavailable registry |
| Cache | Cold/warm/refresh meaningful facts agree | Changed effective input misses or bypasses; forged cache cannot affect gate |
| Comparison | Same measurement remains comparable | Changed version/semantics/settings, unknown producer, stale reference |
| Image | All advertised capabilities on each native platform | Missing tools and unavailable optional capabilities |
| Integration | Same actual report survives Action and App publication | Required gaps, retired identity, wrong protected digest, absent report |
| Maintenance | Exact candidate and result identity | Deliberate failure and missing/stale run cannot appear supported |

Use focused tests while implementing. Before an approved release, run the repository's
required full checks, real-image contracts, and exact-head integration gates.
Emulated amd64 is not native certification.
Finite fixtures establish their stated capabilities, not universal correctness.

## 8. Migration, rollback, and operational ownership

Compare old and candidate tools on the same protected source before interpreting tool-induced deltas.
Use reanchor only where its existing rules allow it. Otherwise require an owner-reviewed capture.
Keep image approval and debt acceptance separate. Never accept new findings automatically.
Never interpret an old-tool base against a new-tool head as a code-only change.

Rollback needs a coherent image, Action, App admission/decoder, workflow pin, and baseline set.
An older image is not guaranteed to parse upgraded source.
If no safe compatible set exists, fail closed with a recovery message.
Retained artifacts do not automatically retain approval.

| Owner | Ongoing responsibility |
| --- | --- |
| Engine maintainer | Locks, adapter limits, capability fixtures, compatibility triage |
| Action maintainer | Protected invocation, fresh cache boundary, transport and standalone outcome |
| App maintainer | Admission, protected-input checks, actionable publication and trust wording |
| Release coordinator | Exact artifacts, native gates, consumer adoption, rollback evidence |
| Repository architecture owner | Scope, dependency preparation, baseline debt decisions |

A single person can hold several roles. A scheduled failure still needs a named responder.
No model output belongs on the pass/fail path.

## 9. Owner decisions

- Approve the bounded compatibility-CI increment, supported capabilities/platforms, and cadence.
- Decide the deep-analysis target-code execution/network policy.
- Decide the promised local-cache trust boundary and private-dependency preparation requirements.
- Review the outstanding reference migration separately from shipped producer adoption.
- Keep UI diagnostics under its existing owner and separate deployment approval.
- Revisit packaging only for measured operational need without reducing supported tool availability.

**No additional pushes, publication, deployments, customer changes, or baseline acceptance
are authorized by this plan.**
