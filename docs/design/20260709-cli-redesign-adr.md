# ADR: CLI redesign for report vs gate workflows

Date: 2026-07-09  
Status: Accepted

## Context

`archfit` had one analysis command doing two jobs:

- local, report-only architecture review for humans;
- CI and AI-agent gating that should fail on findings.

That made the CLI muddy. Users had to learn a mode flag (`--gate`) instead of a command that matched intent. Other flags also drifted into an inconsistent surface: `--full` was redundant, `--advisory` used the opt-in direction even though advisories should be on by default, `--no-cache` did not describe the intended refresh workflow, and `--llm*` mixed terminology across commands.

The redesign was settled after three inputs:

1. an advisor recommended a clean break instead of a compatibility-heavy transition;
2. Perplexity raised the `clig.dev` rule of thumb that non-zero exit codes should mean program error, not findings;
3. a four-panelist fusion review converged on a linter-style split and made shared implementation a hard constraint.

The decision had to lock the CLI shape, exit-code contract, migration path, and warning behavior before code and docs moved.

## Decision

### 1. Split the command surface by intent

- `archfit analyze` is the report-only command.
- Bare `archfit` remains equivalent to `archfit analyze`.
- `archfit check` is the canonical CI and AI-agent gate command.

Exit behavior:

- `archfit analyze` exits `0` after any successful analysis, even when it reports findings.
- `archfit check` exits non-zero on gate findings:
  - `1` = violations
  - `2` = warnings
  - `3` = execution or usage error

### 2. Keep one pipeline implementation

Both commands must call one implementation path: `runPipeline` / `runScan` with a `gateMode bool` switch.

There will be no forked analyze/check logic. This is a hard design constraint from the fusion panel and is binding.

### 3. Prefer command split over a mode flag

`archfit` is a validator and linter-style tool for architecture drift feedback in CI and AI-agent workflows. Its CLI should match tools such as:

- `ruff check`
- `cargo check`
- `terraform validate`

Because the tool has two valid workflows with different exit semantics, those workflows map to two verbs better than to one verb plus `--gate`.

### 4. Make a clean break

No backward-compatibility layer will be kept.

- `--gate` is removed.
- No `--gate` alias is added to `check`.
- No compatibility shim is added for old command forms.

### 5. Final flag surface

| Old flag                                   | Decision                   | New surface                                            |
| ------------------------------------------ | -------------------------- | ------------------------------------------------------ |
| `--gate`                                   | Removed                    | Use `archfit check`                                    |
| `--full`                                   | Removed                    | Full scan is unconditional default                     |
| `--advisory`                               | Removed and inverted       | `--no-advisories`                                      |
| `--severity`                               | Renamed                    | `--min-severity`                                       |
| `--llm` on `analyze`                       | Renamed                    | `--ai-summary`                                         |
| `--llm` on `config init` / `config update` | Renamed                    | `--ai-classify`                                        |
| `--llm-provider`                           | Renamed                    | `--ai-provider`                                        |
| `--llm-model`                              | Renamed                    | `--ai-model`                                           |
| `--no-cache`                               | Renamed with new semantics | `--refresh`                                            |
| `--no-config`                              | Removed                    | Config is required; run `archfit config init --root .` |
| `--require-tools`                          | Moved                      | Check-only; not available on `analyze`                 |

`--refresh` means cache reads are bypassed, but successful fresh results are still written.

### 6. Add silent-failure warnings with next-step hints

The CLI will emit six stderr warnings with explicit remediation hints in the form `→ run: <next-command>`:

1. Missing analyzer tool → `archfit doctor --fix`
2. `0` scored edges when total edges are greater than `0` → `archfit config update -c <config>`
3. All edges abstained → `archfit config enrich abstained -c <config>`
4. All edges external because of Python `grimp` mismatch or equivalent path mismatch → `archfit config update -c <config>`
5. Cached partial result after tool change → `archfit check --refresh`
6. Root/module-path mismatch → `archfit check --root . -c <config>`

## Rationale

The advisor's recommendation was to do a clean break. That fits this CLI better than a soft migration because the old surface encoded the wrong mental model: gating looked like an optional mode layered onto reporting, instead of being a first-class command.

Perplexity's push-back was valid in the abstract. `clig.dev` is right for general-purpose reporting tools: non-zero exit codes usually mean the program failed, not that it found something interesting. That rule was overruled here because `archfit` is not a general reporting tool. It is a validator. Linter and gate tools conventionally return non-zero when they detect actionable findings. `archfit check` follows that convention. `archfit analyze` preserves the report-only workflow for humans and exploratory use.

The fusion panel resolved the tension cleanly:

- keep report-only analysis;
- add a dedicated gate command;
- remove `--gate`;
- force both verbs through the same pipeline so output, findings, score, and warning behavior stay aligned.

That shared-pipeline rule matters as much as the command names. It prevents semantic drift between local review and CI gating, cuts test duplication, and avoids maintaining two implementations that slowly disagree.

The flag cleanup follows the same principle: the surface should say what the user means.

- `--min-severity` reads as a threshold, not a vague concept.
- `--no-advisories` matches the default-on behavior.
- `--ai-*` uses one vocabulary across summary and classification features.
- `--refresh` describes the desired action better than `--no-cache` and matches the new read-bypass/write-through semantics.

The warning set exists to reduce false confidence. A successful process exit is not enough if the analysis was silently degraded by missing tools, abstentions, stale partial cache data, or path mismatches. Each warning therefore carries the next command to run.

## Consequences

### What changes

- CI, bots, and validation hooks move to `archfit check`.
- Local exploratory usage moves to `archfit analyze` or bare `archfit`.
- Old invocations using `--gate`, `--full`, `--advisory`, `--severity`, `--llm*`, `--no-cache`, or `--no-config` break and must be updated.
- Config becomes required.
- `--require-tools` becomes a gate-only concern.

### What improves

- The command name now matches the user's intent.
- Exit codes become predictable for automation.
- Human reporting and CI gating both stay available without overloading one command.
- One shared pipeline preserves consistent findings and output across both workflows.
- Silent degradation gets explicit operator guidance.
- The flag set is smaller and clearer.

### What breaks

This is a deliberate breaking change.

- Existing scripts using `archfit analyze --gate` or `archfit --gate` will fail until migrated.
- Existing references to removed or renamed flags will fail until migrated.
- Users relying on config-free scans must initialize config first.
- Any external docs, examples, or agent prompts using the old surface must be rewritten.

## Migration table

### Commands

| Old command              | New command       |
| ------------------------ | ----------------- |
| `archfit analyze --gate` | `archfit check`   |
| `archfit --gate`         | `archfit check`   |
| `archfit analyze`        | `archfit analyze` |
| `archfit`                | `archfit analyze` |

### Flags

| Old surface               | New surface                                                                    |
| ------------------------- | ------------------------------------------------------------------------------ |
| `--gate`                  | `archfit check`                                                                |
| `--full`                  | Remove it; full scan is default                                                |
| `--advisory`              | Remove it; advisories are on by default. Use `--no-advisories` to disable them |
| `--severity`              | `--min-severity`                                                               |
| `analyze --llm`           | `analyze --ai-summary`                                                         |
| `config init --llm`       | `config init --ai-classify`                                                    |
| `config update --llm`     | `config update --ai-classify`                                                  |
| `--llm-provider`          | `--ai-provider`                                                                |
| `--llm-model`             | `--ai-model`                                                                   |
| `--no-cache`              | `--refresh`                                                                    |
| `--no-config`             | Initialize config first with `archfit config init --root .`                    |
| `analyze --require-tools` | Use `check --require-tools`                                                    |

## Alternatives considered and rejected

### 1. Keep one command and use `--gate`

Rejected.

Why:

- it keeps the wrong default mental model;
- it hides the CI contract behind a flag;
- it clashes with linter-style command naming;
- it leaves the tool doing two jobs under one verb.

### 2. Follow `clig.dev` strictly and never return non-zero for findings

Rejected.

Why:

- it fits general reporting tools better than validators;
- it weakens CI and AI-agent ergonomics;
- it would force callers to parse output instead of trusting the exit code;
- it ignores established validator precedent.

### 3. Add `check` but keep `--gate` as an alias or compatibility shim

Rejected.

Why:

- the explicit direction was no backward compatibility;
- aliases prolong old docs, scripts, and support burden;
- the redesign is cleaner if there is one obvious gate path.

### 4. Implement separate analyze and check pipelines

Rejected.

Why:

- the fusion panel was unanimous against forked logic;
- duplicated pipelines would drift in findings or formatting;
- the extra code and tests would increase maintenance cost for no product gain.
