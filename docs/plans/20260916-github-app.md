# archfit as a GitHub App — design proposal

Status: proposal (2026-09-16). Not a decision.

Produced from a peer design session (Claude + Codex) over the shipped CLI. Every
claim about current behaviour is cited to source. Where the session found a
latent defect or an unclosable gap, it is stated rather than smoothed over.

## The ask

Connect archfit to a repo or org and have architecture guardrails checked on
every PR, the way Aikido checks security and Codecov checks coverage. Most PRs
are authored by AI agents; the app must stop those agents from eroding an
approved architecture, must support the legitimate refactor that *does* change
architecture, and must speak to both humans and agents. Config per-repo or
centrally managed.

## Product in one paragraph

Install the App on an org. It discovers each repo server-side, proposes
boundaries in small reviewable groups, and opens an onboarding PR. Repos start
advisory; owners activate reviewed boundaries incrementally. From then on every
PR gets two checks — a narrow required gate and an advisory architecture-state
report — plus one sticky comment that is readable by a human in five seconds and
by an agent verbatim. The App owns identity, approved policy, approved
measurement identity, baselines, and the feedback surfaces. Analysis runs in the
customer's CI on images the App pins. The engine, its config contract and its
`archfit.architecture-state.v1` output stay as they are, with one engine change
requested (Decision 5).

**The promise, worded exactly:** *archfit enforces your approved policy over
disclosed evidence.* Never "your architecture is correct", never "human-reviewed
means architecturally sound". No UI can prove a reviewer paid attention.

---

# Part I — Trust model

## Decision 1 — compute lives in the customer's runner

archfit's facts come from `packages.Load`, dependency-cruiser, grimp,
`cargo metadata`, ast-grep, jscpd, SCIP. Hosted analysis would need every repo's
build toolchain and private-dependency credentials. So the App does not analyze
by default: the Action runs `archfit check --json`, the App consumes the report.

Two corrections from the session. This is a **toolchain-and-credential-locality**
argument, not primarily a sandboxing one — `cargo metadata` reads manifests and
does not run `build.rs`; provisioning dependencies and extracting facts are
distinct operations. And BYO compute does not remove the sandboxing question, it
relocates it (Decision 2).

## Decision 2 — the hard problem is gate-input integrity

Three verified ways enforcement disappears with no red verdict, all reachable by
an ordinary agent doing an ordinary refactor:

1. **Policy flip.** `languages.<x>.gate: off` is an explicit opt-out that
   `--require-tools` does not overrule
   (`internal/assessment/evaluation/tool_gate.go:10-11`).
2. **Comparability drift.** The seam gate blocks only against a comparable
   reference; one moved fingerprint makes the run abstain.
3. **Input rewrite.** `.archfit.yaml`, `.archfit-labels.yaml` and
   `.archfit-baseline.json` live in the tree the PR is editing, and `config_hash`
   is the sha256 of the config file's raw bytes
   (`internal/evidence/acquisition/service.go:378`) — so the report faithfully
   publishes the *weakened* policy's hash.

> **Principle: gate inputs come only from a trusted source, and the required
> check fails closed.**

The App never evaluates the head tree's policy. It resolves the effective
policy itself from a protected ref, renders a snapshot, issues its digest, and
rejects any report whose `comparison.config_hash` does not match. Labels,
baseline and measurement identity are bound independently, the same way.

This closes vectors 1 and 3 outright. **Vector 2 is only half closable**, and
the proposal says so: `config_hash` and `labels_hash` can be server-sourced, but
`model_hash` covers the *resolved* module map — CODEOWNERS-filled owners and
detected deploy units come from the head tree, a settled decision pinned by
`policy.TestModelHashCoversResolvedTopology` (hashing only the declared map
would let an owner flip re-qualify an existing seam). So a legitimate rename or
CODEOWNERS edit makes the stored baseline non-comparable and the run exits 2.
Fail-closed and no-op refactors conflict at exactly this point; the exit mapping
in Part III is the resolution, and the cost is accepted, not hidden.

### What provenance proves, and what it does not

OIDC proves **which workflow definition** ran (`job_workflow_ref`, `sha`,
`run_attempt`), so trust is a property of the workflow definition rather than of
the runner or the transport, and a report from a head-authored job definition can
never satisfy enforcement. But provenance is **necessary and insufficient**: a
trusted workflow can faithfully run a PR's install script that overwrites
`archfit-state.json` with a clean report carrying the expected digests, and the
legitimate uploader then authenticates the forgery. Fail-closed catches *missing*
evidence; fabricated evidence looks complete.

Strong enforcement needs **runtime separation**: target build code must not be
able to write the analyzer, the policy snapshot, the report path or the auth
environment, and trusted extraction must independently derive the facts it
certifies. Build-generated files and supplied coverage remain untrusted inputs
even after a clean uploader signs their digest.

Pinning the image by digest does **not** deliver that. Docker adds a writable
container layer, so the digest freezes the *starting artifact*, not the runtime
trust boundary; permissions, mounts, subprocess isolation and a protected output
path are separate controls that must be designed explicitly.

Mechanics: `workflow_run` runs the **default-branch** definition and its
`GITHUB_SHA` is that context, so the envelope binds PR number, head SHA, base
SHA and merge-base explicitly rather than inheriting the job's own. The envelope
covers repository, analyzed tree, base, workflow revision, run attempt,
measurement profile, image digest and report digest.

## Decision 3 — provenance per fact, not a trust tier

An earlier draft had a ladder: "customer-attested CI" now, "adversary-resistant"
later. The ladder conflates two independent properties:

- **Measurement trust** — could target-authored code influence this fact?
- **Evidence completeness** — did the required facts actually get observed?

No-target-execution simplifies protected measurement; it does not guarantee
parser safety, and richer properly-isolated measurement is not inherently less
trustworthy. So the App carries **provenance per fact or rule** — the same shape
the engine already uses: nine dimensions, each with its own status, confidence
and coverage, and `state.Promote` refusing to infer completion from a high
metric value. The Check Run states the measurement trust of what it enforced,
and never prints a blanket "verified" badge. archfit already ships this
epistemics: the coverage sidecar is unsigned producer attestation that
cross-binds what it can and discloses what it cannot prove.

## Decision 4 — independently measured boundaries, attested rich evidence

Where the App can observe a fact server-side with no install or build
(source-level import scan, ast-grep patterns, module mapping), it should
**enforce that rule directly** under approved policy, not merely use the
observation as a checksum on CI. That yields a hybrid the ladder could not
express: independently measured boundaries plus customer-attested richer
evidence, each labelled with its own provenance.

The same machinery gives forgery **detection** within attested facts, under a
strict contract:

- verifies *named facts*, never the report as a whole;
- a disagreement is an **independent-observation discrepancy**, not proof of
  tampering — stale checkout, analyzer bugs, build variants and resolution
  differences produce the same symptom;
- requires an explicit comparability contract first: identical source SHA,
  approved policy snapshot, file classification, module mapping, import
  semantics. A source-only import graph and a compiler-resolved graph are not
  interchangeable; compare a declared common subset and report the rest as
  **not checked**;
- compares normalized identities and source witnesses, **not aggregates** — seam
  rows carry `Edges`/`ScoredEdges`/`CriticalEdges` counts, not edge identities
  (`internal/model/report/state.go:313`), and equal counts can conceal different
  edges;
- a definite contradiction inside the common subset fails the gate as an
  **integrity failure**, reported separately from architecture violations;
  agreement says "these observations matched" and promotes nothing outside the
  subset.

An attacker tailoring a forgery to the checked subset does not invalidate that
narrow guarantee. Presenting it as report-wide verification would.

## Decision 5 — measurement identity (engine change requested)

**The defect the App would introduce.** If the App ships the analyzer as images,
analyzer versions become a service-side variable. But the four comparability
fingerprints are `config_hash`, `model_hash`, `labels_hash`, `rubric_version`
only (`internal/assessment/decision/state_comparison.go:13-26`; the stored
`baseline.StateSnapshot` keeps just those four), while
`factcache.Key(analyzer, toolVersion, configSliceHash, inputTreeHash)`
(`internal/factcache/key.go:18`) is direct evidence that tool version changes
the facts. Bump dependency-cruiser in our image and findings move between last
week's baseline and today's PR while `CompareFingerprints` still returns
`comparable` — the delta is attributed to the developer's code. That is exactly
the misattribution that function exists to prevent.

**Both layers are needed, and they are different layers:**

| Layer | Owns | Why it cannot be the other layer |
|---|---|---|
| **Engine** — versioned *measurement-profile fingerprint* added to the comparison contract and the stored baseline | archfit implementation identity, applicable extractor/tool identities, rule-bundle identity, relevant target settings | An App-only digest check leaves host-CLI comparisons unprotected |
| **App** — approved image digest + platform, checked against that release's locked manifest | deployment identity | Using the image digest as the engine fingerprint would invalidate baselines on unrelated image rebuilds |

Two constraints on the fingerprint: `tool_versions` is an **input** to it, not
automatically its complete definition; and **missing identity must not compare
equal merely because both sides say "unknown"**.

**Decided 2026-09-16: this ships as its own engine PR, ahead of any App work,
and it ships in two steps.**

*Step 1 (v2.2.0 — disclosure).* Analyzer identity is captured and published;
the profile is NOT wired into `CompareFingerprints`. Rationale: enforcing it is
a breaking change to comparison semantics and would invalidate every stored
baseline on first run, which a minor release must not do; and the App needs the
FIELD, not the CLI blocking on it, because Part III already routes a profile
mismatch to its own `action_required` path. Honest ceiling, stated rather than
implied: a tool bump is now *visible* in the report but still does not make the
run `non_comparable`, so the original misattribution is detectable, not fixed.

Found while implementing, all three real defects rather than test noise:
`sg --version` prints a multi-line deprecation banner that was going into the
fact-cache key as the "version"; the jscpd version probe sat outside the
per-analyzer watchdog; and SCIP stamped the indexer NAME (`scip-go`) where a
version belongs. `measurement.tool_versions` went from two bogus entries to six
real ones.

A tool version is machine state, not tree state, so `go version` is reduced to
its toolchain token (`go1.27.1`, not `go1.27.1 darwin/arm64` — the platform
belongs to the App layer that pins the image) and the byte-identical baselines
record `<VERSION>` placeholders while pinning the analyzer KEYS. Without that,
a golden generated on a laptop can never pass on a runner, since CI pins
jscpd 5.0.11 / ast-grep 0.44.0.

*Step 2 (follow-up, breaking).* Wire the profile into `CompareFingerprints` as
the fifth fingerprint, bump the baseline schema, and decide the incompleteness
rule and the cross-platform question — a baseline captured on a linux runner
and re-read on a developer's mac has a genuinely different measurement, and
whether that abstains or is tolerated is a product decision, not a mechanical
one. Own PR, own version bump, with the App's `action_required` semantics
designed first.

**Upgrades are a migration, not a refresh.** On a profile bump the App replays
the same approved source tree under old and new profile, exposes the
measurement-induced difference, and requires an explicit **baseline migration**
approval. Otherwise an upgrade silently accepts newly exposed violations — or
silently discards accepted debt. Never blanket re-baseline.

## Decision 6 — measurement authority and gate eligibility

"Who may this run's evidence come from?" is **not one enum**. An earlier draft
collapsed it into `vendor_workflow` / `org_declared_runner` / `untrusted_fork`,
but those name three different axes — producer control, execution environment,
and source origin. Each is recorded separately on the run:

| Fact | Example | Notes |
|---|---|---|
| approving authority | org admin team, authenticated | never a run's self-reported flag |
| approved producer / workflow identity | `job_workflow_ref` on a protected ref | scoped to installation + repository |
| runner environment | GitHub-hosted, or an approved self-hosted runner group | a runner *label* is not proof |
| source trust | base-owned tree vs fork-authored | independent of the other three |
| measurement profile | see Decision 5 | |

**Eligibility is derived, not declared** — from the org's protected evidence
policy over those five facts. A run never asserts its own eligibility.

**An org may declare its own self-hosted runners trusted**, and such a run may
satisfy the required gate. The check then says **"customer-attested
measurement" prominently, not in small print.** The cross-tenant invariant is
*"this commit satisfies the organization's approved architecture and evidence
requirements"* — not *"every customer has identical infrastructure assurance"*.
An org's declaration can never upgrade a result to vendor-verified or protected
measurement.

**Fork PRs.** A fork's job has neither our credentials nor `id-token: write`, and
a fork-authored report is a head-authored producer that cannot satisfy
enforcement. But "enforcement on forks is impossible before server-side
measurement" is **wrong**: a **base-owned dispatcher** can arrange a fresh scan
through an approved workflow on isolated customer-side compute, with reporting
credentials kept separate. That is additional acquisition work, not a hosted
analysis requirement. Two things are forbidden outright: executing fork code in
a privileged reporter, and treating "we approved this fork workflow to run" as
authentication of its output. Until the approved acquisition path exists, a fork
artifact is displayed as **advisory evidence** and the required gate stays
blocked with *"no eligible measurement"* — recovery is a fresh approved
measurement or the explicit GitHub ruleset bypass, never an automatic
`neutral`/`success`. **Fork execution on persistent self-hosted runners defaults
to unsupported**, because fork code can compromise those runners.

## Decision 7 — one repository, one gate

Splitting a monorepo into a check per `--root` looks attractive for ownership,
but `--root` *is* the evidence boundary: "All extractors walk this directory"
(`internal/scope/scope.go:132-134`). Seams and clone analysis are
repository-wide by construction, so edges between subtrees exist in no scoped
run, and the distributed-monolith gate goes blind to exactly the connectivity it
was written for. Concatenating per-scope reports cannot recover it — the same
fact-composition problem as merging per-language reports (Part II).

**v1: one logical repository-wide assessment, one approved baseline version, one
authoritative gate.** Team addressability comes from routing findings by
owner/CODEOWNERS *inside* that single check — a view narrows presentation, never
the evidence boundary.

"One assessment" is not forever "one process": parallel extraction may later
assemble compatible facts into one complete graph and run global
classification, seams and clone analysis once. That is a performance design, not
a scope change.

`--root` keeps its job — a repository holding several independent products, or
an explicitly narrower *"within this scope"* claim that names its excluded
territory. A green scoped run never substitutes for the global gate.

**If scoped results are ever to satisfy a repository-wide gate**, that needs a
separate, architecture-owner-approved **partition contract**: every in-scope
source belongs to the declared partitioning, the relevant analyzers can resolve
cross-partition relationships, and a global assessment verifies the required
cross-partition invariants on the same revision and measurement profile. An
unresolved target or a missing analyzer means *"independence unestablished"* —
never zero edges. Absence of import edges also says nothing about shared clone
evidence, declared external-system relationships, or every repository-wide
metric, so the claim stays limited to the rules and relationship types actually
checked; static analysis cannot certify runtime independence. Invariants are
verified per examined revision by an eligible producer — not by a team's
assertion and not by a historical clean scan. A later partitioned mode keeps a
mandatory cross-partition gate, or narrows the product's claim explicitly
through governed policy approval.

---

# Part II — Tool images

One image per whole-repository run, built from **one locked release manifest**
that fixes every tool version. The manifest ID is what the measurement profile
references; the variants are packaging choices over the same manifest, with
shared extractor versions.

| Tag | Contents | Notes |
|---|---|---|
| `archfit:<ver>-base` | archfit, git | no language analyzers; server-side/no-install profile |
| `archfit:<ver>-go` | + Go SDK | `GOTOOLCHAIN=local` pins the toolchain |
| `archfit:<ver>-ts` | + Node, dependency-cruiser, ast-grep, jscpd | |
| `archfit:<ver>-py` | + python3, uv, **baked grimp** | see defect below |
| `archfit:<ver>-rust` | + cargo, rust-analyzer (SCIP) | today's image omits these, so Rust reports `n/a` |
| `archfit:<ver>-polyglot` | all of the above | for mixed repositories |

Base is `debian:bookworm-slim` (glibc — musl broke ast-grep), non-root, and
`sg` must resolve to ast-grep rather than util-linux (`archfit doctor` checks
this).

**Defect found in the current image:** `Dockerfile:56-58` states archfit runs
`uv run --with grimp`, *resolving grimp at runtime*. So an identical image digest
can acquire different Python analyzer code, and analysis needs network egress
mid-run. Bake and lock grimp before any digest pinning claim is made. `go list`
resolving modules through a proxy is a second, narrower egress dependency worth
documenting rather than denying.

**Mixed repos use `polyglot`, not several images merged.** Running per-language
images and merging their reports would need a separate fact-composition design
to preserve repository-wide seams and clone analysis. Until that design exists,
one image per run, and initial comparability across variants stays conservative
until equivalence is demonstrated.

Workflow shape (pinned by digest, cache wired, fail-closed left to the App):

```yaml
name: archfit
on: [pull_request, push]
jobs:
  architecture:
    runs-on: ubuntu-latest
    permissions: { contents: read, id-token: write }
    container:
      image: ghcr.io/alexei-led/archfit@sha256:<digest-from-approved-policy>
      options: --user root
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/cache@v4
        with:
          path: .archfit-cache
          key: archfit-${{ env.ARCHFIT_PROFILE }}-${{ hashFiles('go.sum','package-lock.json','Cargo.lock') }}
      - run: archfit check --json -c "$ARCHFIT_POLICY" > archfit-state.json || true
      - uses: alexei-led/archfit-action@v1     # uploads report + envelope via OIDC
```

---

# Part III — Reporting

## Two check names, deliberately separated

`neutral` and `skipped` do **not** block a required check, so an advisory
conclusion would make "break the upload" the cheapest bypass.

| Check | Required | Domain |
|---|---|---|
| `archfit/gate` | yes | blocking findings under approved policy · input-integrity failure · unverifiable evidence. Never `neutral`. Always posts a conclusion. |
| `archfit/state` | never | nine dimensions, coverage split, seam ledger, drift, `agent_tasks[]`, deltas. |

## Exit code to conclusion

| Run | `archfit/gate` |
|---|---|
| `healthy` (0) | `success` |
| `blocked` (1) | `failure` — blocking findings, `agent_tasks[]` attached |
| (2) `comparison.status: non_comparable` | `action_required` — the drifted fingerprint is already named in `comparison.reasons`; routed to owner review: "module model or ownership changed; seam ledger needs review; migrate baseline after merge" |
| (2) coverage gap on a present language | `action_required` once enforcing — an unobserved boundary is not a passed boundary. Advisory mode is what covers adoption, not a green check |
| `error` (3) / no report | `action_required` |
| integrity failure (Decision 4) | `failure`, reported separately from architecture violations |

**How `action_required` is cleared.** It is not an open-ended block. A
non-comparable run or a policy-change run is cleared by approval from the
authority that owns what moved — architecture owners for a floor or boundary
change, module owners for a verified relocation — **bound to the exact head SHA
and policy snapshot**. A re-push re-arms it. A coverage-gap `action_required` is
cleared by supplying the named missing fact, never by approving around it; an
integrity failure is never cleared by approval at all.

**Erosion-only adoption is the baseline's job, not origin filtering.** Filtering
`agent_tasks[].origin == "introduced"` would break enforcement: the synthetic
`bc/coupling_gate` task is per-run trip state and its origin is *always*
`unknown` (`docs/guide/agent-feedback.md:117`), so origin filtering would make
the entire coupling gate unenforceable. The gate preserves actual gate failures
against the stored baseline whose fingerprints the App verified; `unknown` origin
never exempts a blocking finding. Origin only sorts the comment.

## The sticky comment (one per PR, edited not appended)

````text
## archfit — architecture gate

BLOCKED — this PR introduces 1 boundary violation

### Introduced here
no_internal_access · pkg/a/a.go:12 → pkg/b/internal/impl.go
  Use only the public API of module b: pkg/b/api/**
  Fix and re-run: archfit check -c .archfit.yaml

### Already accepted (not blocking)
3 accepted findings unchanged · 1 diagnostic

### What moved
coupling      measured → measured   +2 seams (orders→billing, billing→orders)
modularity    measured → measured   unchanged

### Not measured — and why
rust          analyzer absent in this image variant → facts n/a
testability   partial — no coverage sidecar supplied

<details><summary>machine-readable · archfit.architecture-state.v1</summary>

```json
{ "verdict": "...", "agent_tasks": [ ... ], "dimensions": { ... } }
```
</details>

policy org/policy@a1b2c3 · profile mp.v1:9d41… · image ghcr.io/…@sha256:7b2e…
baseline merge-base 9f8e7d · evidence: 4 independently measured, 5 attested
````

Design rules: verdict and the single next action above the fold; *introduced*
separated from *accepted*; only dimensions that moved are listed; the
not-measured section is mandatory, never omitted when empty-looking; the JSON is
marker-delimited and verbatim so any coding agent gets goal, constraints,
resolved `files[]` and the exact validation command with zero integration; the
footer is the provenance line — policy ref, measurement profile, image digest,
baseline ref, evidence split. File/line annotations come from the existing SARIF
locations, with no Advanced Security dependency.

**Prevention beats detection.** The App also maintains a generated
architecture-constraints section in `AGENTS.md`/`CLAUDE.md` by PR, so the agent
knows the boundary before writing code. An MCP endpoint ("what is the public
surface of module b?") is the natural next step.

---

# Part IV — Governance: who owns the guardrails

## Roles

| Role | Authority | Bound by |
|---|---|---|
| **Architecture owners** (org team) | the org floor: rules, gates, layer directions, what counts as a boundary | CODEOWNERS on the policy path in a repo with branch protection on the ref the App reads |
| **Module owners** | their module's stanza, its public surface, mechanical relocations | CODEOWNERS on the module subtree |
| **Repo maintainers** | repo-level additions that only strengthen | App-side composition checks |
| **Waiver approver** | must be the level that owns the waived obligation | expiry is necessary but not sufficient governance |

## The org floor is obligations, not a YAML merge

An unrestricted merge of org policy and repo config cannot express an
unweakenable floor: repo exclusions, module remapping, labels, baseline updates
or waivers can all *indirectly* suppress an org rule without lowering a single
`gate:`. So the floor is implemented as **independently enforced obligations plus
repo additions**, and the App validates that no repo-side input suppresses a
floor obligation. A repo may strengthen, never weaken.

The App still hands the runner one rendered file, so `internal/config` keeps its
single-file parse, strict unknown-key rejection and byte-hash identity: no
`extends:` in the schema, no core change.

## Architecture change vs code refactoring

The distinction is not visible in a diff, and my first two attempts at it were
both wrong. What survives review:

**Authorizing check — verified namespace relocation (structural).** A policy
edit rides along with code automatically only when every moved file provably
keeps its logical module, its public/internal classification, its layer, its
ownership and deploy semantics, and its applicable rules; and every
path-sensitive selector undergoes the *same bounded relocation* without
broadening its meaning for future paths. "Same file set" is **not** sufficient —
swapping files between modules preserves the set while changing boundaries. And
"`public:`/`internal:`/rule bodies must not change" is **too strict**, because
those fields contain paths that must move too: allow the *verified substitution*,
not arbitrary edits.

**Corroborating evidence — `archfit config compare` (never proof).** The shipped
command compares two policies over one tree, so after a package move the
approved config is **stale for that tree** and its buckets cannot distinguish a
corrected path from a relaxed policy. It is disclosure in the PR comment and a
reason to escalate — never a proof of no new permission. A future
relocation-aware check would compare the *mechanically transported* approved
policy against the candidate, preserving finding identity where possible.

**If equivalence cannot be established, it is not mechanical** — route to
architecture-owner review. Unexplained differences or inadequate evidence route
the same way.

**Separate approval authority, not separate PRs.** The earlier "policy first,
then code" rule is dropped as a universal: code and policy may land atomically in
one PR when architecture-owner approval is bound to the **exact head SHA and
policy snapshot**. Re-push invalidates the approval. What matters is who
authorized it, not how many PRs it took.

**Accepted debt survives by explicit baseline migration**, never by blanket
re-baseline — and "present today" never implies "allowed forever": existing
dependencies may be accepted debt.

### Worked examples

| Scenario | Classified as | Outcome |
|---|---|---|
| Agent adds `pkg/a` → `pkg/b/internal/impl.go` | code change | `failure`, `agent_tasks[]` names goal, files, validation |
| Agent also sets `no_internal_access: {gate: off}` | gate-input mismatch | `failure` — "policy differs from approved; this is a policy change, needs architecture-owner authority" |
| `internal/orders/**` moved to `internal/sales/orders/**`, `paths:` updated in the same commit | verified namespace relocation | rides along, module-owner approval, `config compare` shown as disclosure |
| New module `billing`, boundary `orders → billing` removed | boundary change | architecture-owner approval bound to head SHA; policy-cost report across affected repos; baseline migration after merge |
| Module renamed, or repo CODEOWNERS changes a module's resolved owner | legitimate model change | `action_required` (`model_hash` drift) → module-owner approval bound to head SHA + baseline migration, **not** "architecture broke" |
| CODEOWNERS changed **on the policy path** | floor change — who may authorize architecture | boundary review by architecture owners; never an owner-review fast path |
| Image/profile bump by us | measurement change | replay one tree old vs new profile, show measurement-induced delta, explicit baseline migration |

---

# Part V — Onboarding

Day 0 requires no CI change: the App discovers server-side (source-only,
sandboxed, nothing persisted).

1. **Install** on the org, pick repos.
2. **Discover** — languages via the extractors' own applicability probes;
   `config init` server-side produces a draft.
3. **Review boundaries in small groups**, not one forty-module wall: each group
   ships with its evidence, unmatched and overlapping paths, allowed and
   forbidden dependency examples, and the effect of each proposed rule.
   Uncertain classifications stay explicit; generation provenance is preserved
   alongside human approval.
4. **Onboarding PR** — approved groups become `.archfit.yaml`, plus the workflow,
   the cache wiring and the `AGENTS.md` constraints section.
5. **Advisory period** — `archfit/state` only. Weekly digest: "what would have
   blocked".
6. **Validate before enforcing** — the App confirms that representative
   forbidden changes actually fail and representative legitimate changes actually
   pass. A gate nobody has watched fire is a gate nobody knows works. (Same
   reasoning as the repo's own six erosion gates, each with a paired fixture.)
7. **Enable** — owners activate reviewed boundaries incrementally; baseline
   captured by a trusted default-branch run; `archfit/gate` becomes required.

---

# Part VI — Service

## Components

| Component | Shape | Owns |
|---|---|---|
| **Reporter** | tiny stateless Go service, its own deploy, multi-region | verify envelope → post check + comment. No dependency on billing, dashboards or LLMs |
| **Control plane** | Go API + workers | policy resolution and snapshot issuance, baseline store, migrations, entitlements |
| **Sandbox pool** | ephemeral, no egress, nothing persisted | server-side discovery and no-install measurement |
| **Storage** | Postgres (row-level isolation by installation) + object store | facts, reports, baselines — **never source** |
| **Policy repo** | customer-side, protected ref | the approved architecture |

## Multi-tenancy

Tenant = GitHub installation. Isolation by `installation_id` at the row level;
per-tenant encryption for stored reports; policy read only from the tenant's own
protected ref, recorded by ref and commit. Source code touches only ephemeral
sandboxes. Reports are size-capped, held in object storage, with a summary index
in Postgres.

## Reliability — and the cost we cannot design away

Fail-closed makes us a merge-blocking dependency: our backend holds the App
private key, so posting any check conclusion depends on our availability at PR
time. **There is no signed-permit shortcut.** Pre-issued repository-scoped
installation tokens expire after one hour — they can bridge a short outage but
cannot remove the dependency, and revoking after a merge cannot restore a
pre-merge guarantee. A label-based break-glass is worse than useless: the App
must process the label, and the App is what is down.

What actually works:

- **Keep the reporting path small, redundant and independent** of billing,
  dashboards and LLM features.
- **Emergency recovery is a GitHub ruleset bypass**, preconfigured and narrowly
  authorized, with archfit's requirement in **its own ruleset** so bypassing it
  preserves every other protection. Recorded as an override — never as
  fabricated success.
- **Customer-operated reporter/App** offered as a different deployment model for
  orgs that will not accept vendor availability in their merge path.
- **Billing outages must never become architecture failures**: cached
  entitlements, documented grace period.

Performance: envelope verification and check posting p95 under a couple of
seconds after upload; the customer's run cost is bounded by the Actions-cached
fact store, whose keys already include tool version and input tree.

## Subscription

Familiar and simple: **org subscription with active-repo bands**, plus an
explicit large-estate enterprise tier (SSO, audit log, self-hosted reporter,
registry mirror, SLA). Public repos free; private repos free in advisory mode.

- **Never per analysis run** — charging per run punishes the frequent-PR
  behaviour the product wants.
- **Never per module.** Module-count billing makes better architecture modelling
  increase the bill and rewards collapsing modules. A monorepo carrying five
  hundred modules is not gaming a repo-shaped unit: customer-run analysis means
  those modules do not cost us five hundred times more.
- Validate real costs and willingness to pay before adding dimensions.

## LLM use (service-side, allowed)

Permitted: onboarding draft config and boundary-group explanations, PR narrative,
finding explanation, proposed relocation patches, onboarding PR bodies.
Forbidden: anything on the gate path — structurally enforced, since
`arch_test.go` bars every `internal/*` package from importing `internal/llm`.

Human approval establishes **chosen policy**, not the truth of every inferred
architectural fact. So: reviewable groups over walls of YAML; uncertain
classifications explicit; generation provenance retained next to human approval
(the engine already lowers coupling confidence for `provenance: llm` labels below
high confidence); and validation that forbidden changes fail before enforcement
is switched on.

---

# Part VII — Repository layout and contract delivery

**Three repositories.**

| Repo | Owns | Visibility |
|---|---|---|
| `archfit` (this one) | engine, CLI, images, **wire contracts** | OSS |
| `archfit-action` | thin execution/upload adapter | public |
| `archfit-app` | tenancy, governance, subscriptions, reporting | private |

**The boundary is a tenant-bearing service versus a customer-executed analyzer,
with a deliberately narrow protocol between them.** The self-model test walking
every Go package (`internal/selfmodel_test.go`) and the six-rank layer stack are
good *evidence* that this repository models one product, but adaptation cost
should not be what decides a product boundary — corrected during review, since
that was the argument the first draft led with.

**No public Go DTO package.** Promoting `internal/model/report` would add
import-path, Go-type and compatibility obligations that a JSON-consuming service
does not need. The model-surface golden checks *internal exported symbols*;
calling the model kernel a pinned published contract does not make those
packages an existing external Go API.

**New deliverable, blocking App integration: a versioned output schema.**
`archfit.schema.json` describes the **configuration** — verified: its `$defs`
are `AIConfig` and friends. There is no published schema for
`archfit.architecture-state.v1` at all, only prose in `docs/design`. So before
any App integration the engine repo must publish:

1. a versioned JSON Schema for `archfit.architecture-state.v1`;
2. representative conformance fixtures covering **healthy, blocked, partial,
   unknown, and incompatible-profile** runs.

The App generates its *transport* types from that schema; its domain and storage
types stay separate.

**Integration CI replaces the monorepo's atomicity.** A monorepo's advantages
(atomic source change, easier integration tests) are real, but separate repos do
not have to discover incompatibility after release: trusted integration CI tests
**immutable candidate engine artifacts** against the App's consumer tests before
release or promotion. Test schema conformance **and decision semantics** —
generated types catch neither semantic drift nor incorrect acceptance.

**Staged rollout with a supported-version matrix**, because installed Actions,
pinned images and stored reports never upgrade atomically — monorepo or not.

One earlier claim corrected: an Action *can* live in a repository subdirectory
at a ref or SHA. A separate Action repo buys clean distribution and independent
releases; it is not a platform requirement.

# What not to build

- No LLM in the gate.
- No repository score anywhere, dashboard included — the cross-format parity
  invariant extends to the web UI.
- No hosted analysis as the default path, and no "lite" tier sold as equivalent
  coverage.
- No `origin == "introduced"` filtering as the enforcement rule.
- No blanket re-baseline on upgrade.
- No "verified report" badge on attested evidence.
- No per-run or per-module billing.
- No merging of per-language reports until a fact-composition design exists.

# Phases

0. **Engine: measurement-profile fingerprint** (separate PR, before the App —
   decided). Added to the comparison contract and the stored baseline.
   Prerequisite for any honest delta, and it fixes a CLI-visible misattribution
   on its own.
0b. **Engine: published output schema + conformance fixtures** for
   `archfit.architecture-state.v1` (Part VII). Blocks App integration; useful to
   every external consumer on its own.
1. **Action + App shell.** Advisory `archfit/state`, sticky comment with
   `agent_tasks[]`, image variants from a locked manifest, baked grimp, cache
   wiring. Adoption value, no enforcement claim.
2. **Trusted inputs + gate.** Server-rendered policy snapshot with issued
   digests, approved image digest, server-stored baselines keyed by merge-base,
   OIDC envelope, `archfit/gate` required and fail-closed, ruleset-bypass
   break-glass, own-ruleset isolation.
   **What this phase does not deliver:** the gate conclusion carries provenance
   `attested`, and the fabricated-report gap from Decision 2 is open here and
   disclosed in the comment footer. This phase enforces approved policy over
   evidence the customer's CI attests to — it does not yet enforce over evidence
   we measured ourselves. **Decided 2026-09-16: ship the required check here
   with provenance `attested` and the gap named, rather than waiting for
   server-side measurement.** Ordinary agent erosion is the threat being bought
   down; an adversarial install script is not, and the report says which.
3. **Governance.** Org floor as obligations, verified-namespace-relocation
   check, approval bound to head SHA and policy snapshot, baseline migrations,
   waiver authority, cross-repo policy-cost preview.
4. **Independent measurement.** Server-side no-install observation: direct
   enforcement of what we measure ourselves, plus the comparability-contracted
   cross-check with explicit not-checked reporting.
5. **Agent channel.** MCP endpoint; boundary queries before the edit.

# Owner-supplied inputs

None of this blocks design, planning or code. Secrets gate **deployment**, not
development: phases 0-4 are built and tested against a locally registered test
App and recorded fixtures. Each item is a step in its own phase with a
placeholder until the owner fills it.

| Input | Placeholder | Needed at | Blocks |
|---|---|---|---|
| GitHub App registration | `ARCHFIT_APP_ID`, `ARCHFIT_APP_PRIVATE_KEY`, `ARCHFIT_WEBHOOK_SECRET`, app slug | phase 1, first deploy | posting real checks; not the reporter's code or tests |
| Hosting target + public base URL | `ARCHFIT_BASE_URL` | phase 1, first deploy | webhook delivery only |
| Image registry namespace | reuse existing `ghcr.io/alexei-led/archfit` | phase 1 | publishing variants; local builds need nothing |
| OIDC audience | `ARCHFIT_OIDC_AUDIENCE` | phase 2 | envelope verification against a live org; fixtures cover the logic |
| Pilot org / repo | — | phase 1 deploy; **blocks enabling the phase 2 gate anywhere** | the Part V step 6 rehearsal ("forbidden change fails, legitimate change passes") needs a real repo with real CI, not fixtures |
| Billing provider | Marketplace listing or `STRIPE_*` | GA | nothing before GA; entitlements are cached and default-open in dev |
| Price points and bands | — | GA | nothing technical |

# Open questions

- The fork acquisition path (Decision 6): what is the minimum base-owned
  dispatcher that produces an eligible measurement of a fork head without
  running fork code under any credential?
- What is the minimum comparable subset for the independent cross-check that is
  worth shipping, and what tests pin it?
- Exactly which identities belong in the measurement-profile fingerprint, and
  how is "unknown" represented so it never compares equal to "unknown"?
- Is the runtime trust boundary (mounts, permissions, subprocess isolation,
  protected output) specifiable well enough to narrow the attested gap in the
  customer's runner, or does that only ever close with server-side measurement?
- GitLab/Bitbucket: the forge adapter is a seam from phase 1 (check posting,
  comment, policy ref, provenance are the only forge-shaped surfaces), with a
  second implementation deferred until a customer asks. Open only in the sense
  that nothing validates the seam until there is a second forge.
