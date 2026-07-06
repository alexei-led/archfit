# Commands

Common commands:

```sh
archfit doctor
archfit config init --root . --output .archfit.yaml
archfit config init --llm --root .
archfit config update --config .archfit.yaml
archfit config update --config .archfit.yaml --llm
archfit config update --config .archfit.yaml --apply
archfit config update --config .archfit.yaml --llm --apply  # structural apply only; LLM proposals stay review-only
archfit                                                      # report-only, default text output
archfit analyze --gate --config .archfit.yaml --full         # CI gate
archfit analyze --gate --config .archfit.yaml --base main    # PR delta gate
archfit analyze --gate --config .archfit.yaml --full --require-tools
archfit analyze --gate --root ../repo --config .archfit.yaml --full
archfit analyze --gate --config .archfit.yaml --json
archfit analyze --format scorecard --config .archfit.yaml --full
archfit analyze --markdown --config .archfit.yaml > archfit-report.md
archfit analyze --llm --config .archfit.yaml
archfit baseline --full --config .archfit.yaml
archfit explain <finding-id-prefix> --config .archfit.yaml
archfit config enrich labels --config .archfit.yaml
archfit config enrich abstained --config .archfit.yaml
archfit config enrich owner --config .archfit.yaml
archfit config enrich volatility --config .archfit.yaml
archfit config enrich subdomain --config .archfit.yaml
archfit config enrich owner --config .archfit.yaml --apply --reviewed-by @you
archfit config init --llm --root . -o .archfit-init-llm.yaml
archfit config init --llm --apply --root .   # writes LLM judgments live; review before using as a gate
archfit explain <finding-id-prefix> --llm
archfit analyze --gate --sarif > archfit.sarif
```

Use `analyze --gate` for CI gates. Use `analyze --markdown` for a human-readable
audit report. Bare `archfit` (no subcommand) runs `analyze` in report-only mode.

## Command summary

- `archfit doctor` — check available local toolchain; a config that fails to
  load is reported with the load error (a `forbidden_dependency` rule with no
  `from:`/`to:` glob is such an error — it would be dead by construction).
  `--fix` installs missing tools (`--dry-run` previews without installing).
- `archfit config init` — generate a starter `.archfit.yaml`; `--llm` adds an
  off-gate classification pass (subdomain, volatility, layer, role, and owner per
  module). Default LLM plan mode comments suggestions; `--llm --apply` writes the
  model judgments live, so review before using the file as a gate.
- `archfit config update` — sync `.archfit.yaml` with the current project structure;
  `--llm` adds review-only module and rule proposals to the drift report. Even with
  `--apply`, only structural drift is written live; LLM proposals stay review-only.
- `archfit analyze` — run architecture analysis (default command; also runs as
  bare `archfit`). Without `--gate` it is report-only (always exits `0` on
  success, `3` on config/tool error). With `--gate` it enforces rules and emits
  CI exit codes. See `analyze` below.
- `archfit baseline` — record accepted current findings.
- `archfit explain <id>` — explain one finding by fingerprint prefix
  (`--llm` appends an off-gate narrative; needs `ai:` configured).
- `archfit config enrich` — draft LLM refinements for human review (off-gate).
  Subcommands: `labels` (coupling-label drafts → `.archfit-labels.yaml`),
  `abstained` (labels for unknown-strength cross-module edges with snippets),
  `owner`, `volatility`, `subdomain` (module-field drafts → separate draft files);
  `--apply` writes approved module-field entries into the config. See
  [llm-enrich.md](llm-enrich.md).

Output formats for `analyze`: `text` (default), `json`, `markdown`/`md`, `sarif`
(SARIF 2.1.0 for CI code-scanning annotations), `scorecard` (the banded
coupling_balance scorecard). Shorthands: `--json`, `--markdown`, `--sarif` (mutually
exclusive). `--format` is repeatable for multiple outputs.

For wiring archfit into an AI coding agent's loop (`agent_tasks`, SARIF),
see [agent-feedback.md](agent-feedback.md).

## Finding status

Findings have a lifecycle status:

- `new` — active finding not present in the baseline;
- `baseline` — accepted finding already recorded;
- `fixed` — previously baselined finding that is no longer detected;
- `waived` — active finding covered by an approved waiver;
- `expired_waiver` — active finding whose waiver has expired.

## Exit codes

- `0` — pass;
- `1` — fail (active gate finding, a metric delta that worsens past its
  `metrics.<name>` threshold with `gate` fail/unset, a tripped
  [`coupling.gate`](configuration-reference.md#couplinggate), **or** a missing
  required tool under `--require-tools` / `languages.<x>.gate: fail` /
  `analyzers.<x>.gate: fail` — a policy violation);
- `2` — warn;
- `3` — usage, config, or runtime error.

Exit `1` (policy) is deliberately distinct from exit `3` (tool/config error): a
missing required tool is a _gate_ decision you opted into, not a crash.

Balanced Coupling advisories are informational by default. Use them to prioritize
architecture review and refactoring, not as automatic pass/fail rules. The
synthesised `coupling_balance` score built from those advisories _can_ fail the
build, but only through the opt-in `coupling.gate` block.

## Coverage gaps and required tools

A missing analyzer drops dependent metrics to `n/a` (never a false green) and
emits a machine-readable coverage gap in every output format. A disabled analyzer
(`enabled: false`) is a deliberate opt-out — no gap is emitted.

Default posture is warn-loud, exit 0. To gate on a missing tool use `--require-tools`
or `languages.<x>.gate: fail` / `analyzers.<x>.gate: fail`. For the full gap format,
`n/a` semantics, and timeout behavior see
[configuration reference → analyzers](configuration-reference.md#analyzersx-gate-coverage-gate).

## analyze

`archfit analyze` is the single analysis command. It is also the **default
command** — bare `archfit` (with no subcommand) runs it.

```sh
# report-only: always exits 0 on success; decision band printed to terminal
archfit
archfit analyze --config .archfit.yaml

# CI gate: exits 0/1/2/3 per exit-code table below
archfit analyze --gate --config .archfit.yaml --full

# PR delta: compare to base branch
archfit analyze --gate --config .archfit.yaml --base origin/main --json

# Markdown audit report
archfit analyze --markdown --config .archfit.yaml > archfit-report.md

# Scorecard view
archfit analyze --format scorecard --config .archfit.yaml

# LLM holistic narrative appended (off-gate; needs ai: configured)
archfit analyze --llm --config .archfit.yaml

# SARIF for GitHub code-scanning
archfit analyze --gate --sarif > archfit.sarif
```

The default text output leads with a **Decision** block:

```
ARCHFIT RESULT

Decision   HEALTHY
Gate       PASS  ·  0 blocking
Warnings   0 advisory
Score      82 / 100  serviceable

RECOMMENDATIONS

  MUST FIX
    none
  SHOULD FIX
    none
  WATCH
    ...
```

Live progress lines (`[1/5] extracting Go packages …`) print to **stderr** while
analyzing. They are TTY-gated: suppressed when stdout is piped, under CI
environments, or with `--quiet`. This keeps `archfit --json | jq` clean.

### analyze flags

- `-c` / `--config` — config file (default `.archfit.yaml`).
- `--root` — analysis root (default: config directory).
- `--base <ref>` — also score a git ref and show a before/after scorecard delta
  in the text/markdown report. JSON also includes `score_delta`; SARIF stays the
  normal HEAD diagnostic.
- `--gate` — enable CI exit codes (0/1/2/3); without it the run is report-only
  (always exits `0` on success, `3` on config or tool error).
- `--json` — output JSON (shorthand for `--format json`).
- `--markdown` — output Markdown audit report (shorthand for `--format markdown`).
- `--sarif` — output SARIF 2.1.0 (shorthand for `--format sarif`).
- `--format <fmt>` — repeatable: `text` (default), `json`, `markdown`/`md`,
  `sarif`, `scorecard`. Shorthands and `--format` are mutually exclusive.
- `--llm` — add an off-gate LLM advisory interpretation after the
  deterministic output (needs `ai:` configured in `.archfit.yaml`). For `json`,
  `sarif`, and `scorecard` formats the review is written to stderr so stdout
  stays parseable.
- `--full` — scan all files (default true).
- `--advisory` — include Balanced Coupling advisories (default true).
- `--no-cache` — bypass archfit caches: extractor facts (and LLM responses with
  `--llm`). Reads and writes — a `--no-cache` run neither uses nor refreshes
  entries. See [caching.md](caching.md).
- `--severity`, `--lang`, `--no-config`, `--require-tools` — standard analyze flags.
- `--progress auto|plain|none` — progress output mode (default `auto`).
- `--quiet` / `-q` — suppress progress and non-essential output.

## --root: analysis boundary

`--root` (on `analyze` and `config enrich`) sets the **analysis
boundary** — the directory tree that extractors walk, coverage counts against, and
file-based metrics scope to. All tool calls (go/packages, dependency-cruiser,
grimp, loc, clones, fitness) operate inside this tree.

`--root` is decoupled from the git topmost directory. archfit resolves `git
rev-parse --show-toplevel` separately (stored as the internal `GitRoot`); git
history and changed-file diffs run there and are then re-based to the subtree.
This lets a config in a CI repo point at a subdir of a monorepo:

```sh
archfit analyze --gate --root ./server/shared --config ./policies/.archfit.yaml --full
```

or let you analyze a monorepo subproject without touching any config:

```sh
archfit analyze --gate --root ~/workspace/omni/server/shared --full
```

When `--root` is absent, the scan root equals `GitRoot` (or the config directory
for a non-git tree), so no existing invocations change.

**File-count scoping:** `FilesSeen` in coverage and loc counts only files inside
`--root`. Running `--root <subdir>` restricts those counts to the subtree; running
at the repo root includes everything.

**Baseline, labels, and config-hash** stay config-adjacent — only the scanned
tree moves.

**macOS APFS case-insensitive volumes:** `--root` paths whose case differs from
the on-disk canonical form (e.g. `/Users/alice/workspace/myrepo` vs
`/Users/alice/Workspace/MyRepo`) are resolved to the canonical path via
`os.SameFile` (device+inode comparison), so subtree scoping works correctly on
case-insensitive APFS. `filepath.EvalSymlinks` still handles symlinks; the
`os.SameFile` pass handles case mismatches that `EvalSymlinks` cannot fix.

## Scorecard view

The banded **coupling_balance scorecard** is one of `analyze`'s output formats:

```sh
# whole-repo scorecard
archfit analyze --format scorecard --config .archfit.yaml

# with a base ref: shows a delta section
archfit analyze --format scorecard --config .archfit.yaml --base main
```

The scorecard shows one scored dimension: `coupling_balance`. It carries a 0–100
value, a band (critical / poor / mixed / serviceable / strong), a confidence, and
evidence references. The overall is the coupling_balance value. This is not a
composite of multiple dimensions — coupling_balance is the single architecture
fitness measure; structural rules (forbidden deps, layering, cycles, encapsulation)
are pass/fail gates reported separately.

Output is deterministic — byte-identical across a double-run. The scorecard is
report-only by default; the opt-in
[`coupling.gate`](configuration-reference.md#couplinggate) block makes the
synthesised score fail the run (exit 1) on a band floor or score drop.

## LLM narrative (analyze --llm)

`archfit analyze --llm` runs the full deterministic pipeline, synthesizes the
scorecard, builds the shared repository evidence pack, and feeds those cited
facts to the LLM for an architect review after the deterministic output. Text and
Markdown runs append the review to stdout; `json`, `sarif`, and `scorecard` runs
write it to stderr so machine stdout stays parseable. It is **off-gate**: the
review is advisory only and never affects the gate verdict, findings, metrics, or
scorecard (enforced by the LLM-off-gate invariant in `internal/arch_test.go`).

```sh
archfit analyze --llm --config .archfit.yaml
archfit analyze --llm --no-cache --config .archfit.yaml
```

The model is constrained by a Balanced-Coupling-grounded system prompt and a
strict JSON schema. Each dimension, risk, and suggestion carries `claim_type`
(`deterministic_fact`, `semantic_interpretation`, `recommendation`, or
`unknown`) plus citations via `finding_ids`, `metric_ids`, or `evidence_refs`.
It may only:

- narrate, prioritize, and contextualize findings **already present** in the
  evidence;
- classify volatility / subdomain for modules that appear in the evidence;
- propose dimension bands for dimensions named in the evidence;
- cite only supplied finding IDs, metric IDs, and repository evidence IDs.

A post-verify pass enforces the rubric vocabulary and drops fabricated values
(dropped counts logged to stderr):

- **Overall band** — blanked (shown as "unrated") if outside the five-band rubric
  (`critical` / `poor` / `mixed` / `serviceable` / `strong`).
- **Dimension entries** — dropped if the dimension name is unknown **or** the band
  is outside the rubric vocabulary.
- **Subdomain suggestions** — dropped if the suggested subdomain is not in the
  fixed DDD subdomain set (`core`, `supporting`, `generic`).
- **Module/risk references** — dropped if the module name does not appear in the
  deterministic evidence. Dynamic/lazy-import modules are valid evidence even when
  they carry no static finding.
- **Claim metadata** — dropped if `claim_type` is outside the fixed vocabulary or
  if a recommendation lacks at least one supported `finding_id`, `metric_id`, or
  `evidence_ref`.

The model **cannot** invent gate violations, module names, band labels, finding
IDs, metric IDs, or evidence refs.

Dynamic/lazy imports (detected by TypeScript and Python extractors as
`dynamic_imports`) are included in the review prompt as a hidden-coupling risk
section so the narrative can flag coupling the static dependency graph misses.
The same is true for `connascence.roadmap` and `runtime_async_edges`: the review
may explain them or propose follow-up config/docs, but it cannot convert them into
scored facts, gate findings, or baseline changes.

**Layer intent:** when layers are declared, `forbidden_layer_direction` gates
deterministically. When they are not, `archfit config enrich` can propose a layer
structure; see [configuration-reference.md → layers](configuration-reference.md#layers).

Requirements:

- `ai:` configured (provider + model) and the provider's API key set.
  Without it, `--llm` exits `3` with an actionable message and touches nothing.
  See [LLM enrichment](llm-enrich.md) and `archfit doctor`.

Flags (in addition to the standard `analyze` flags):

- `--no-cache` — bypass archfit caches: extractor facts and the LLM response
  cache at `.archfit-cache/llm/`. See [caching.md](caching.md).

## archfit config init --llm (full draft or direct apply)

`archfit config init --llm` is a one-shot LLM drafter for a whole `.archfit.yaml`. It
discovers project structure, classifies every module (subdomain, volatility,
layer, and `role`), drafts an owner per module from CODEOWNERS context, and
renders the entire config in **plan mode** — every suggestion is a commented YAML
line, nothing is applied.

```sh
archfit config init --llm --root . -o .archfit-init-llm.yaml
archfit config init --llm --root . -o -   # stream the draft to stdout
```

Direct it to a side file with `-o` to keep it review-only: review the draft, then
move approved fields into the live config deliberately. `--apply` skips that review
step and writes the LLM classifications live into the generated config, so inspect
and edit the file before using it as a gate. Needs `ai:` configured (provider +
model) and the provider's API key — see [LLM enrichment](llm-enrich.md).

Flags:

- `--root` / `-r` — project root (default: `.`).
- `--output` / `-o` — draft output file; `-` for stdout.
- `--llm-provider`, `--llm-model`, `--no-cache` — provider/cache controls.

When `--llm` is used, `config init` reads `ai:` from the target output config
when that file already exists; otherwise use `--llm-provider` and `--llm-model`.

## archfit config init

`archfit config init` discovers project structure and writes a starter `.archfit.yaml`.
With `--llm` it adds an off-gate classification pass that suggests `subdomain`,
`volatility`, `layer`, and a module-name improvement for each discovered module.

The output path is resolved against `--root`, so `archfit config init --root <dir>` writes
`<dir>/.archfit.yaml` (not the current directory). **By default `config init` will not
overwrite an existing config** — if a valid `.archfit.yaml` is already present it
leaves it untouched (architect-authored module mapping is not clobbered) and exits 0
with a note. Pass `--force` to overwrite it; a timestamped backup is kept.

```sh
# plan mode (default): suggestions are commented-inert in the output
archfit config init --llm --root .

# apply mode: LLM classifications written live into the file
archfit config init --llm --apply --root .

# stream to stdout (no file written) — useful for inspection
archfit config init --llm -o -
```

Mode behaviour:

- `--llm` (plan): classification lines are emitted as YAML comments
  (`# subdomain: core  # llm-suggested — review and uncomment`). Uncommenting
  activates them; the file is safe to use as a gate without reviewing them.
- `--llm --apply`: `subdomain`, `volatility`, `owner`, and `layer` are written
  as live fields from the model response. `layer` is written only when the value
  is in `layers:`; otherwise it stays a comment. Module keys are never renamed
  automatically. Treat the output as unreviewed until a human checks the cited
  rationale.
- `--apply` without `--llm` is an error.

Flags:

- `--llm-provider` — override provider (`anthropic`, `openai`, `ollama`).
  Default: `anthropic`.
- `--llm-model` — override model. Default: `claude-opus-4-8`.
- `--no-cache` — bypass the LLM response cache at `.archfit-cache/llm/`.

See [LLM enrichment](llm-enrich.md) for provider and API key setup. `archfit doctor`
shows key and cache status.

## archfit config update

`archfit config update` keeps `.archfit.yaml` in sync as the codebase evolves. It re-runs
discovery, compares the results to the existing config, and reports or applies the
diff.

```sh
# plan mode (default): prints a drift report, writes nothing
archfit config update --config .archfit.yaml

# apply mode: writes structural changes live
archfit config update --config .archfit.yaml --apply

# with LLM: adds review-only role/volatility proposals to the report
archfit config update --config .archfit.yaml --llm

# with LLM + apply: structural changes written live; LLM proposals stay review-only
archfit config update --config .archfit.yaml --llm --apply
```

Mode matrix:

| Command                       | Effect                                                                          |
| ----------------------------- | ------------------------------------------------------------------------------- |
| `config update`               | Drift report only; writes nothing.                                              |
| `config update --apply`       | Structural drift written live (add/path/comment).                               |
| `config update --llm`         | Drift report plus review-only subdomain, volatility, layer, and role proposals. |
| `config update --llm --apply` | Structural drift written live; LLM semantic proposals remain review-only.       |

What "structural drift" means:

- **Added modules** — modules discovered but absent from the config are added as
  new stanzas.
- **Path drift** — modules whose discovered paths differ from config paths get
  their `paths:` block replaced with the discovered paths.
- **Removed modules** — modules in the config with no discovered paths are
  commented out with a marker (e.g. `# archfit: removed module "foo" — verify
before deleting`).

Guardrails:

- Plan mode (`config update` without `--apply`) never writes `.archfit.yaml`.
- `--apply` backs up the existing file before writing (`.archfit.yaml.bak` or
  timestamped if a backup already exists).
- Existing field values are never overwritten.
- LLM subdomain, volatility, layer, and role proposals are report-only. Review
  and copy accepted values into `.archfit.yaml` deliberately.
- Module keys are never auto-renamed.
- If the config has not changed since it was read, `--apply` aborts rather than
  overwriting concurrent edits.

Flags:

- `--config` / `-c` — config file path (default: `.archfit.yaml`).
- `--root` / `-r` — project root for discovery (default: directory of `--config`).
- `--llm`, `--llm-provider`, `--llm-model`, `--no-cache` — same as `config init --llm`.

## Scorecard delta (analyze --base)

`archfit analyze --base <ref>` scores a git ref in addition to HEAD and shows a
before/after scorecard delta. It checks `<ref>` out into a clean detached
worktree, scores it with the same pipeline, and adds a **CHANGE VS BASE** section
to the text report (a "Change vs base" block under `--markdown`). The HEAD side is
a normal full analysis. JSON includes `score_delta`:
`base_overall`, `head_overall`, `overall_delta`, and per-dimension
`base`/`head`/`delta`; SARIF stays the standard HEAD diagnostic. `--gate` /
`--require-tools` apply exactly as without `--base`. Base-side extractor facts are cached by commit SHA, so
repeat runs against the same ref skip all base-side subprocess work — see
[caching.md](caching.md).

```sh
archfit analyze --base main                            # decision + before/after delta
archfit analyze --gate --base origin/main              # gate HEAD and show the delta
archfit analyze --base v1.0.0 --root ./services/api --markdown
```

**Both sides use the current `--config`** — this isolates code drift from config
drift; the base ref may predate the config file. A non-git directory or an
unknown ref exits `3`. Use it to answer "did this branch improve or regress the
architecture scorecard?"
