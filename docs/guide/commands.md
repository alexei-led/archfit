# Commands

Common commands:

```sh
archfit doctor
archfit init --root . --output .archfit.yaml
archfit init --llm --root .
archfit update --config .archfit.yaml
archfit update --config .archfit.yaml --apply
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
archfit enrich --config .archfit.yaml
archfit enrich --owner --config .archfit.yaml
archfit enrich --volatility --config .archfit.yaml
archfit autopilot --root . --output .archfit-autopilot.yaml
archfit explain <finding-id-prefix> --llm
archfit analyze --gate --sarif > archfit.sarif
```

Use `analyze --gate` for CI gates. Use `analyze --markdown` for a human-readable
audit report. Bare `archfit` (no subcommand) runs `analyze` in report-only mode.

## Command summary

- `archfit doctor` — check available local toolchain.
- `archfit init` — generate a starter `.archfit.yaml`.
- `archfit update` — sync `.archfit.yaml` with the current project structure.
- `archfit analyze` — run architecture analysis (default command; also runs as
  bare `archfit`). Without `--gate` it is report-only (always exits `0` on
  success, `3` on config/tool error). With `--gate` it enforces rules and emits
  CI exit codes. See `analyze` below.
- `archfit baseline` — record accepted current findings.
- `archfit explain <id>` — explain one finding by fingerprint prefix
  (`--llm` appends an off-gate narrative; needs `ai:` configured).
- `archfit enrich` — draft LLM coupling-label refinements for human review
  (off-gate; writes `.archfit-labels.yaml` drafts). `--owner` / `--volatility`
  draft those module fields; `--pin` writes approved entries into the config. See
  [llm-enrich.md](llm-enrich.md).
- `archfit autopilot` — one-shot LLM drafter for a full `.archfit.yaml` (off-gate,
  review-only; never applies — writes `.archfit-autopilot.yaml`). See
  [llm-enrich.md](llm-enrich.md).
- `archfit install` — install or print commands for the common analyzer tools it
  can bootstrap; see [Tooling reference](tooling.md) for the full platform matrix.
- `archfit calibrate` — calibrate scoring thresholds against observed data.

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
- `1` — fail (active gate finding, **or** a missing required tool under
  `--require-tools` / `languages.<x>.gate: fail` / `analyzers.<x>.gate: fail` — a policy violation);
- `2` — warn;
- `3` — usage, config, or runtime error.

Exit `1` (policy) is deliberately distinct from exit `3` (tool/config error): a
missing required tool is a _gate_ decision you opted into, not a crash.

Balanced Coupling advisories are informational by default. Use them to prioritize
architecture review and refactoring, not as automatic pass/fail rules.

## Coverage gaps and required tools

When an analyzer is **absent** (tool not installed or not detected), archfit does
**not** silently score the repo as healthy. The dependent metrics drop to `n/a`
(no evidence — never `strong`), and a machine-readable **coverage gap** lists the
tool, the metrics its absence leaves unmeasured, and a one-line install hint.

When an analyzer is **disabled by config** (`enabled: false`), it is simply
skipped — no coverage gap is emitted and no install prompt appears.
Disabled-by-config is a deliberate opt-out, not a gap to resolve.

Coverage gaps for absent tools appear in every format:

- markdown (`--markdown` / `--format markdown`) — a `## Coverage gaps (N)` section before findings;
- scorecard (`--format scorecard`) — a `## Required tools missing (N)` section;
- `--format json` — a `coverage_gaps[]` array (`tool`, `install_cmd`,
  `affected_metrics`, `gate`) plus `config_warnings[]`;
- stderr — one warn-loud line per gap.

Default posture is **warn-loud, exit 0**. To make CI block on a missing tool, opt
in with `--require-tools` (raises every gap to `fail` for that run) or
`languages.<x>.gate: fail` / `analyzers.<x>.gate: fail` per tool (see
[configuration-reference.md](configuration-reference.md#analyzersx-gate-coverage-gate)).

Config-quality warnings (e.g. "N modules under-specified") now reach md/json too,
as a `## Config warnings` section and the `config_warnings[]` JSON field — they
are no longer stderr-only.

### n/a coverage semantics

Two distinct `n/a` reasons appear in coverage and metric output:

- **`n/a (no history)`** — the subtree (after `--root` scoping) has fewer than 2
  commits visible to `git log`, or `--root` points at a non-git directory.
  Delta mode (`--base`) requires git history; without it, the before/after
  comparison is skipped. For shallow clones: add `git fetch --unshallow` in CI
  or use a full clone. For non-git trees: all other metrics still run.
- **`n/a (timed out)`** — a per-analyzer watchdog fired before the subprocess
  finished. The tool result is dropped cleanly; the run continues and exits with
  the verdict from the remaining analyzers. Increase the per-tool timeout or
  reduce the scope via `languages.go.modules` / `--root`. See
  [configuration-reference.md → analyzers.&lt;x&gt;.timeout](configuration-reference.md#analyzersx-timeout).

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
  in the text/markdown report. JSON/SARIF stay the normal HEAD diagnostic.
- `--gate` — enable CI exit codes (0/1/2/3); without it the run is report-only
  (always exits `0` on success, `3` on config or tool error).
- `--json` — output JSON (shorthand for `--format json`).
- `--markdown` — output Markdown audit report (shorthand for `--format markdown`).
- `--sarif` — output SARIF 2.1.0 (shorthand for `--format sarif`).
- `--format <fmt>` — repeatable: `text` (default), `json`, `markdown`/`md`,
  `sarif`, `scorecard`. Shorthands and `--format` are mutually exclusive.
- `--llm` — append an off-gate LLM advisory interpretation after the
  deterministic output (needs `ai:` configured in `.archfit.yaml`).
- `--full` — scan all files (default true).
- `--advisory` — include Balanced Coupling advisories (default true).
- `--severity`, `--lang`, `--no-config`, `--require-tools` — same as the
  former `check` command.
- `--progress auto|plain|none` — progress output mode (default `auto`).
- `--quiet` / `-q` — suppress progress and non-essential output.

## --root: analysis boundary

`--root` (on `analyze` and `enrich`) sets the **analysis
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
**off-gate**: it never changes the exit code.

## LLM narrative (analyze --llm)

`archfit analyze --llm` runs the full deterministic pipeline, synthesizes the
scorecard, and feeds **both** to the LLM for a holistic narrative appended after
the deterministic output. It is **off-gate**: the narrative is advisory only and
never affects the gate verdict (enforced by the LLM-off-gate invariant in
`internal/arch_test.go`).

```sh
archfit analyze --llm --config .archfit.yaml
archfit analyze --llm --no-cache --config .archfit.yaml
```

The model is constrained by a Balanced-Coupling-grounded system prompt and a
strict JSON schema. It may only:

- narrate, prioritize, and contextualize findings **already present** in the
  evidence;
- classify volatility / subdomain for modules that appear in the evidence;
- propose dimension bands for dimensions named in the evidence.

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

The model **cannot** invent gate violations, module names, or band labels.

Dynamic/lazy imports (detected by TypeScript and Python extractors as
`dynamic_imports`) are included in the review prompt as a hidden-coupling risk
section so the narrative can flag coupling the static dependency graph misses.

**Layer intent:** when layers are declared, `forbidden_layer_direction` gates
deterministically. When they are not, `archfit enrich` can propose a layer
structure; see [configuration-reference.md → layers](configuration-reference.md#layers).

Requirements:

- `ai:` configured (provider + model) and the provider's API key set.
  Without it, `--llm` exits `3` with an actionable message and touches nothing.
  See [LLM enrichment](llm-enrich.md) and `archfit doctor`.

Flags (in addition to the standard `analyze` flags):

- `--no-cache` — bypass the LLM response cache at `.archfit-cache/llm/`.

## archfit autopilot

`archfit autopilot` is a one-shot LLM drafter for a whole `.archfit.yaml`. It
discovers project structure, classifies every module (subdomain, volatility,
layer, and `role`), drafts an owner per module from CODEOWNERS context, and
renders the entire config in **plan mode** — every suggestion is a commented YAML
line, nothing is applied.

```sh
archfit autopilot --root . --output .archfit-autopilot.yaml
archfit autopilot --root . -o -      # stream the draft to stdout
```

It is **off-gate and review-only**: the draft lands in a separate file
(`.archfit-autopilot.yaml` by default) and autopilot **refuses** to write
`.archfit.yaml` directly (exit 3). Review the draft, then move approved fields
into the live config deliberately. Needs `ai:` configured (provider +
model) and the provider's API key — see [LLM enrichment](llm-enrich.md).

Flags:

- `--root` / `-r` — project root (default: `.`).
- `--config` / `-c` — existing config to read `ai:` from (default:
  `.archfit.yaml`).
- `--output` / `-o` — draft output file; `-` for stdout. Never `.archfit.yaml`.
- `--llm-provider`, `--llm-model`, `--no-cache` — same as `init --llm`.

## archfit init --llm

`archfit init` discovers project structure and writes a starter `.archfit.yaml`.
With `--llm` it adds an off-gate classification pass that suggests `subdomain`,
`volatility`, `layer`, and a module-name improvement for each discovered module.

```sh
# plan mode (default): suggestions are commented-inert in the output
archfit init --llm --root .

# apply mode: LLM classifications written live into the file
archfit init --llm --apply --root .

# stream to stdout (no file written) — useful for inspection
archfit init --llm -o -
```

Mode behaviour:

- `--llm` (plan): classification lines are emitted as YAML comments
  (`# subdomain: core  # llm-suggested — review and uncomment`). Uncommenting
  activates them; the file is safe to use as a gate without reviewing them.
- `--llm --apply`: `subdomain`, `volatility`, and `layer` are written as live
  fields. `layer` is written only when the value is in `layers:`; otherwise it
  stays a comment. Module keys are never renamed automatically.
- `--apply` without `--llm` is an error.

Flags:

- `--llm-provider` — override provider (`anthropic`, `openai`, `ollama`).
  Default: `anthropic`.
- `--llm-model` — override model. Default: `claude-opus-4-8`.
- `--no-cache` — bypass the LLM response cache at `.archfit-cache/llm/`.

See [LLM enrichment](llm-enrich.md) for provider and API key setup. `archfit doctor`
shows key and cache status.

## archfit update

`archfit update` keeps `.archfit.yaml` in sync as the codebase evolves. It re-runs
discovery, compares the results to the existing config, and reports or applies the
diff.

```sh
# plan mode (default): prints a drift report, writes nothing
archfit update --config .archfit.yaml

# apply mode: writes structural changes live
archfit update --config .archfit.yaml --apply

# with LLM: adds classification of unclassified modules to the report
archfit update --config .archfit.yaml --llm

# with LLM + apply: structural + classification written live
archfit update --config .archfit.yaml --llm --apply
```

Mode matrix:

| Command                | Effect                                             |
| ---------------------- | -------------------------------------------------- |
| `update`               | Drift report only; writes nothing.                 |
| `update --apply`       | Structural drift written live (add/path/comment).  |
| `update --llm`         | Drift report + LLM classification of unclassified. |
| `update --llm --apply` | Structural + LLM classification written live.      |

What "structural drift" means:

- **Added modules** — modules discovered but absent from the config are added as
  new stanzas.
- **Path drift** — modules whose discovered paths differ from config paths get
  their `paths:` block replaced with the discovered paths.
- **Removed modules** — modules in the config with no discovered paths are
  commented out with a marker (e.g. `# archfit: removed module "foo" — verify
before deleting`).

Guardrails:

- Plan mode (`update` without `--apply`) never writes `.archfit.yaml`.
- `--apply` backs up the existing file before writing (`.archfit.yaml.bak` or
  timestamped if a backup already exists).
- Existing field values (`subdomain`, `volatility`, `layer`) are never
  overwritten — `--llm --apply` fills only absent fields.
- `layer` from LLM is written live only when the value is present in `layers:`.
- Module keys are never auto-renamed.
- If the config has not changed since it was read, `--apply` aborts rather than
  overwriting concurrent edits.

Flags:

- `--config` / `-c` — config file path (default: `.archfit.yaml`).
- `--root` / `-r` — project root for discovery (default: directory of `--config`).
- `--llm`, `--llm-provider`, `--llm-model`, `--no-cache` — same as `init --llm`.

## Scorecard delta (analyze --base)

`archfit analyze --base <ref>` scores a git ref in addition to HEAD and shows a
before/after scorecard delta. It checks `<ref>` out into a clean detached
worktree, scores it with the same pipeline, and adds a **CHANGE VS BASE** section
to the text report (a "Change vs base" block under `--markdown`). The HEAD side is
a normal full analysis, so `--json`/`--sarif` are the standard HEAD diagnostic
(no separate delta schema) and `--gate` / `--require-tools` apply exactly as
without `--base`.

```sh
archfit analyze --base main                            # decision + before/after delta
archfit analyze --gate --base origin/main              # gate HEAD and show the delta
archfit analyze --base v1.0.0 --root ./services/api --markdown
```

**Both sides use the current `--config`** — this isolates code drift from config
drift; the base ref may predate the config file. A non-git directory or an
unknown ref exits `3`. Use it to answer "did this branch improve or regress the
architecture scorecard?"
