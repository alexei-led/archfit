# Commands

Common commands:

```sh
archfit doctor
archfit init --root . --output .archfit.yaml
archfit init --llm --root .
archfit update --config .archfit.yaml
archfit update --config .archfit.yaml --apply
archfit check --config .archfit.yaml --full
archfit check --config .archfit.yaml --base main
archfit check --config .archfit.yaml --format json
archfit check --config .archfit.yaml --format scorecard
archfit score --config .archfit.yaml
archfit score --config .archfit.yaml --base main
archfit review --config .archfit.yaml
archfit scan --config .archfit.yaml > archfit-report.md
archfit baseline --full --config .archfit.yaml
archfit explain <finding-id-prefix> --config .archfit.yaml
archfit enrich --config .archfit.yaml
archfit explain <finding-id-prefix> --llm
archfit check --format sarif > archfit.sarif
```

Use `check` for gates. Use `scan` for a human-readable audit report.

## Command summary

- `archfit doctor` — check available local toolchain.
- `archfit init` — generate a starter `.archfit.yaml`.
- `archfit update` — sync `.archfit.yaml` with the current project structure.
- `archfit check` — run architecture gates and metrics.
- `archfit score` — emit the banded 7-dimension scorecard (off-gate; see below).
- `archfit review` — off-gate LLM holistic narrative over the deterministic
  evidence (needs `tools.llm`; see below).
- `archfit scan` — produce a full Markdown audit report.
- `archfit baseline` — record accepted current findings.
- `archfit explain <id>` — explain one finding by fingerprint prefix
  (`--llm` appends an off-gate narrative; needs `tools.llm`).
- `archfit enrich` — draft LLM coupling-label refinements for human review
  (off-gate; writes `.archfit-labels.yaml` drafts — see
  [llm-enrich.md](llm-enrich.md)).
- `archfit install` — install or print commands for optional language tools.

Output formats for `check`: `text`, `json`, `markdown`/`md`, `sarif`
(SARIF 2.1.0 for CI code-scanning annotations), `scorecard` (the banded
7-dimension synthesis — same output as `archfit score`).

For wiring archfit into an AI coding agent's loop (`agent_tasks`, SARIF,
`change_locality`), see [agent-feedback.md](agent-feedback.md).

## Finding status

Findings have a lifecycle status:

- `new` — active finding not present in the baseline;
- `baseline` — accepted finding already recorded;
- `fixed` — previously baselined finding that is no longer detected;
- `excepted` — active finding covered by an approved exception;
- `expired_exception` — active finding whose exception has expired.

## Exit codes

- `0` — pass;
- `1` — fail;
- `2` — warn;
- `3` — usage, config, or runtime error.

Balanced Coupling advisories are informational by default. Use them to prioritize
architecture review and refactoring, not as automatic pass/fail rules.

## archfit score

`archfit score` emits the banded **7-dimension scorecard** aligned to the
architect rubric: `boundary_integrity`, `coupling_balance`,
`dependency_graph_health`, `cohesion_modularity`, `change_locality`,
`architecture_fitness`, and the meta dimension `analysis_confidence`. Each
dimension carries a 0–100 value, a band (critical / poor / mixed / serviceable /
strong), a confidence, and evidence refs; the overall is the mean of the six
non-meta dimensions.

```sh
# whole-repo scorecard
archfit score --config .archfit.yaml

# with a base ref: enables the change_locality dimension (delta mode)
archfit score --config .archfit.yaml --base main
```

`score` is a convenience wrapper over `check`: always advisory (so
`coupling_balance` sees the Balanced-Coupling edges), always report-only (a
scorecard informs, it never gates), and renders the `scorecard` format. The same
output is available as `archfit check --format scorecard`. Output is
deterministic — byte-identical across a double-run. The scorecard is **off-gate**:
it never changes the `check` verdict or exit code.

Flags:

- `--config` / `-c` — config file path (default: `.archfit.yaml`).
- `--base` — git ref to compare against; enables the `change_locality` dimension.

## archfit review

`archfit review` runs the full deterministic pipeline, synthesizes the scorecard,
and feeds **both** to the LLM for a holistic narrative. It is **off-gate**: the
narrative is advisory only and never affects `check` (enforced by the LLM-off-gate
invariant in `internal/arch_test.go`).

```sh
archfit review --config .archfit.yaml
archfit review --config .archfit.yaml --no-cache
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

Requirements:

- `tools.llm` configured (provider + model) and the provider's API key set.
  Without it, `review` exits `3` with an actionable message and touches nothing.
  See [LLM enrichment](llm-enrich.md) and `archfit doctor`.

Flags:

- `--config` / `-c` — config file path (default: `.archfit.yaml`).
- `--no-cache` — bypass the LLM response cache at `.archfit-cache/llm/`.

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
