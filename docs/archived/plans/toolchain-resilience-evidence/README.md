# Toolchain resilience: public evidence notes

These notes support the [design and remaining plan](../toolchain-resilience-design-and-plan.md).
They contain normalized findings and public engine/Action identities.
Private service test code, deployment records, raw customer reports, logs, and
machine-specific paths are deliberately excluded.

The original 1,425-line design and all raw review files were copied byte-for-byte to
an operator-owned archive before public-worktree sanitization. File hashes were verified.
The original design SHA-256 is
`60582ede9bfbc2b49791e8b69105985f11cdb22c157651be069e0d23cb8d2352`.
Historical captures were not relabeled as results from a newer release.

## 1. Distinguish the experiments from the release

| Evidence | Source / image | Meaning |
| --- | --- | --- |
| Local experiments, 2026-10-24 | Engine `11e84d1152d96ca724ac27326932ff8a689a6439`; local arm64 image `sha256:c206455444ebae08fad47b2b3a4ff503e8b466f615bc4cf8a2a3c6d954302714` | Historical candidate, not an approved registry pin. |
| Published engine v3.2.0 | Source `faa4eb4e513859567fd6c380230c91ea22fdaab6` | Later fixes and successful native release gates. |
| Published Action v2.2.0 | Source `cadf35c0d4885a49a3a1482b876f2a456d20de06` | Required-evidence guard and fresh scans shipped. |
| Connected adoption | Public engine run and separately retained Python acceptance evidence | New producer works on those scopes. Baseline and optional-evidence limits remain explicit. |

The [release identity artifact](https://github.com/alexei-led/archfit/releases/download/v3.2.0/image-identity.json)
lists the published platform manifests. The historical local image is not one of them.

The [engine publisher](https://github.com/alexei-led/archfit/actions/runs/40172977752)
succeeded on the released source. Release records report 133/133 assertions on each native platform.
[Action CI](https://github.com/alexei-led/archfit-action/actions/runs/40175778074)
passed all five jobs, including both native image smokes and actual composite invocation.
Its tested tree equals the published Action tree.

These release results supersede old candidate gate failures.
They do not change what the earlier experiments observed.

## 2. Local experiment method

The [historical engine-only harness](experiments.py) reuses fixture definitions from
the frozen public engine source. It checks the hashes of its imported fixture/helper/manifest files.
It does not import private App code or change a customer repository.

Runtime: existing Colima aarch64 VM, 4 CPUs, about 6.2 GB memory, Docker 29.5.2.
The disconnected implicit Docker socket was not used. Commands selected the existing
`colima` context explicitly. No settings, unrelated containers, or image tags were changed.

Each fixture ran with:

- The exact local image ID and `--pull=never`.
- Non-root UID/GID 27182, 2 CPUs, 2 GiB memory, 256 PIDs.
- Read-only container root, no capabilities, no-new-privileges.
- A 768 MiB disposable `/tmp`.
- Read-only fixture input copied into disposable scratch, plus a dedicated output mount.
- No Docker socket, credentials, or customer source mounts.
- Network disabled except explicit public dependency preparation.
- A 120-second per-fixture deadline and cleanup limited to the owned container.

Thirty observations covered 28 unique cases. A successful probe process is not a successful
hypothesis. Cache poisoning and wrapper execution were observed failures of the desired boundary.

Effective candidate versions, obtained from executed probes:

| Tool closure | Versions |
| --- | --- |
| Engine / external Go | Go 1.27.3, linked x/tools v0.52.0, external Go 1.27.3 |
| TS/JS | Node 24.22.1, dependency-cruiser 18.7.0, TypeScript 6.0.3, Acorn 8.19.0 |
| Python | Python 3.14.9, Grimp 3.17, protobuf 7.36.2, grpcio-tools 1.84.1 |
| Rust | Rust/Cargo/rust-analyzer 1.99.1, cargo-modules 0.28.0 |
| Semantic tools | scip-go 0.2.9, scip-typescript 0.5.2, scip-python 0.6.8 |
| Shared tools | ast-grep 0.51.0, jscpd 5.4.2, uv 0.13.5 |

These are historical effective identities, not claims about the latest upstream releases.

## 3. Decision-focused results

| Hypothesis / decision | Expected behavior | Observed result | Interpretation and limit |
| --- | --- | --- | --- |
| Existing full image supplies primary evidence | Exact forbidden core-to-adapter dependency | All five Go/TS/JS/Python/Rust fixtures found it | Useful primary evidence, not proof of all semantic capabilities. |
| Newer supported grammar works | Known edge survives Go new(expression), Python t-string, Rust2024 let chains | Edge retained; Rust module coverage OK | Concrete features, not unknown next-release grammar. |
| Missing TS compiler is a gap | Required rule unevaluated, not a valid empty scan | Exit 2, needs_attention, unmeasured hard gates, compiler reason | Already repaired in candidate and retained in release. |
| JS does not require optional TS compiler | Same JS forbidden edge | Edge retained with TypeScript package hidden | Analyzer and target-language capabilities differ. |
| Target cannot select TS producer | No marker from local Node/compiler/config | Local Node refused; local compiler/config ignored; edge retained | Does not establish every tool's isolation. |
| One bad Python file remains visible | Partial imports and required-rule gap | Observed | Valid files do not conceal that tested failure. |
| Invalid TS is not universally diagnosed | Explicit syntax limitation | Invalid expression retained both known imports without parser warning; malformed import became partial | No missed forbidden import proved. Do not infer full grammar coverage. |
| Legitimate empty and zero-edge scopes differ | Producer emptiness is allowed without inventing facts | Empty producer OK; stale required selector unevaluated. Two valid zero-edge files had passing hard gates | Empty scope does not validate an obsolete policy selector. |
| Missing manifest is not completed analysis | Required scope remains unmeasured | Explicitly enabled TS without package.json reported a gap | Auto applicability/orphan coverage needs separate fixtures. |
| Python target runtime is not parser runtime | Static parsing can continue | Target declared less than 3.14; Python 3.14 retained the edge | Does not execute or certify the application on 3.14. |
| Go minimum mismatch is already fast | No toolchain download, useful failure | Synthetic Go1.28 minimum on1.27.3 failed in about0.13s; mixed workspace also failed | Not a genuine upcoming-release grammar fixture. |
| Cache reuse preserves facts | Cold/warm/refresh agree | Untampered facts agreed; forged cache removed the forbidden edge; refresh restored it | Proven local false hard-gate pass, not demonstrated App bypass. |
| Analyzer installation is not target dependency preparation | Dependency failure stays explicit | Unprepared offline Go dependency was partial; known internal violation retained | Missing type/dependency inputs do not erase observed violations. |
| Prepared dependency works offline | Same known edge in separate network-disabled container | Exact public module cache produced Go coverage OK and the edge | One pure-Go dependency, not universal offline builds. |
| Deep Rust is not non-executing | Target configuration cannot run under that claim | Cargo wrapper wrote a benign output marker with cargo-modules enabled | Pinned/read-only tools are not a no-target-code boundary. |
| Unsupported edition is not clean evidence | Explicit required gap | Synthetic edition2099 rejected | Rejection test, not future edition support. |

The expected forbidden rule was `core_no_adapter`.
Assertions examined that finding and required-rule/coverage fields, not only graph size or exit code.

### Cache probe detail

The probe retained all module entries and deleted only dependency arrays from cached depcruise stdout.
Sources, policy, and the cache filename/key did not change.

- Cold, warm, and refreshed decision/coverage/findings/seams had the same normalized hash:
  `23e4e0b25173e87a438235e6066f4729a19d9af624db88a5e0ceaf8163772f2c`.
- Tampered replay changed hard gates from fail to pass and removed the forbidden finding.
- Overall verdict still needed attention for unrelated unmeasured dimensions.

The Action already kept its bundle outside the checkout. Its published v2.2.0 also
requests `--refresh` for report/capture/reanchor/self-check.
Do not turn a local cache observation into an unproved published-gate exploit.

### Dependency probe detail

The public dependency was `github.com/google/uuid v1.6.0`.
Online preparation recorded go.sum and the module cache.
A separate container mounted those inputs read-only and ran with no network,
`GOPROXY=off GOSUMDB=off`, and `--refresh`.
The known internal forbidden edge remained and Go coverage was OK.

An earlier subcommand only disabled the proxy in a networked container.
The later separate-container probe resolved that limitation.

## 4. Cross-repository evidence

The old Action accepted a no-endpoint `needs_attention` report even with an unevaluated
required rule. A report-stage replay confirmed exit 0.
The released v2.2.0 guard fixes that path and retains optional-only attention.

A local App composition test used the real missing-compiler report:

- An unapproved candidate identity produced `action_required/report_invalid`.
- A deliberately substituted approved identity isolated report semantics and produced
  `action_required/missing_required_coverage`.

The second case did not approve the candidate or verify its execution.
Private test implementation and logs are not included here.

After release, actual connected producer reports reached the App:

- The [engine's main-branch workflow](https://github.com/alexei-led/archfit/actions/runs/40184059258)
  measured 41/41 declared rules. The remaining required gap was four metric ratchets
  against a non-comparable old reference. No active blocker was reported.
- A connected Python report measured all six required rules with no blockers or required
  gaps. The App's advisory state was neutral. Optional cycle evidence remained unmeasured.

These are distinct from approval of a baseline migration or completeness of every optional capability.
No baseline was accepted by this review.

## 5. Cost observations

| Measurement on historical candidate | Result | Limit |
| --- | --- | --- |
| Docker local disk accounting | 2.66 GB | Not registry transfer size. |
| Docker inspect Size field | 592,415,288 bytes | Not a timed pull. |
| Rust directory | 632,912 KiB | Removal opportunity, not measured core-image savings. |
| Existing-image version launch, five samples | 0.4630, 0.1749, 0.1485, 0.1608, 0.1698 seconds | Warm local startup, not analysis or cold pull. |

No core image was built. No compressed transfer, cold registry pull, native amd64 local
experiment, or build-time comparison was measured.
Release CI later exercised both native platforms, which does not retroactively change this local experiment.

## 6. Reproducing historical probes

The harness needs the already-prepared historical image. It does not pull or build it.
That image ID is local evidence, not a public release artifact.

From an engine checkout containing the frozen commit:

```sh
cache="$HOME/.cache/archfit-design-review/replay"
mkdir -p "$cache/source"
git archive 11e84d1152d96ca724ac27326932ff8a689a6439 | tar -x -C "$cache/source"
uv run --offline --no-project --python 3.14 \
  docs/plans/toolchain-resilience-evidence/experiments.py \
  --source "$cache/source" --output "$cache/probes"
```

Use a new output directory. The harness never clears an existing one.
A full run includes one bounded public dependency fetch.
Use `--case typescript` or another named case for an offline subset.

For release acceptance, use the current public engine's existing
`scripts/eval/language_contracts.py` and `scripts/image_contracts.py` instead.
Do not promote this historical probe script into a second production test framework.

## 7. Failed attempts and verification limits

The private archive preserves failures rather than replacing them with later success:

- Initial cache mutation used `Stdout` instead of the real lowercase `stdout` field.
  It failed before mutation. The corrected probe completed.
- Initial standalone report-stage replay omitted its runner directory.
  The corrected setup completed.
- Old Action full suite with the negative specimen: 473 passed, four failed because
  the default fixture was expected to be blocked, not needs_attention.
- Retried old Action suite with the blocked specimen hit a 180-second outer timeout.
  No completed-suite pass was claimed. The later released Action CI passed independently.
- Initial local docs tooling lacked `github-slugger`. Final documentation validation
  reused prepared dependencies without a global installation.
- The old host architecture check returned ask_owner and an incompatible measurement profile.
  No baseline was changed to make it pass.

No future grammar guarantee, private-dependency design, arbitrary workspace coverage,
or independently verified report content follows from these experiments.
The final plan separates shipped repairs, owner-held reference decisions, and future compatibility work.
