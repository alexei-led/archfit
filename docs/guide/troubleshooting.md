# Troubleshooting

Run `archfit doctor` first when extraction looks incomplete.

Common fixes:

- Use `languages.<language>.enabled: false` or `analyzers.<x>.enabled: false` to disable an adapter while calibrating.
- Use `--format json` when an AI agent or script needs structured output.
- Narrow module paths if generated config is noisy.
- Prefer an expiring waiver over deleting a rule for intentional findings.
- Check that optional analyzer tools are installed before enabling them.
- Re-run `archfit baseline` only after reviewing accepted findings.

For platform setup, package-manager choices, exact tool versions, home pages, and
PATH checks, see [Tooling reference](tooling.md).

## My old command stopped working

The CLI redesign removed the old compatibility flags. Current builds fail fast
with parser errors like these:

| Old flag / example       | Current error message                                  | New equivalent                                                               |
| ------------------------ | ------------------------------------------------------ | ---------------------------------------------------------------------------- |
| `archfit analyze --gate` | `archfit: unknown flag --gate, did you mean "--base"?` | `archfit check`                                                              |
| `--full`                 | `archfit: unknown flag --full`                         | Remove it. Full scan is now the default.                                     |
| `--advisory`             | `archfit: unknown flag --advisory`                     | Remove it. Advisories are on by default. Use `--no-advisories` to hide them. |
| `--no-cache`             | `archfit: unknown flag --no-cache`                     | `--refresh` — it re-runs the extractors and writes fresh results back.       |
| `archfit analyze --llm`  | `archfit: unknown flag --llm`                          | `archfit analyze --ai-summary`                                               |
| `--severity`             | `archfit: unknown flag --severity`                     | `--min-severity`                                                             |
| `--no-config`            | `archfit: unknown flag --no-config`                    | Initialize config first: `archfit config init --root .`                      |

## AI command fails, costs too much, or should not send repo text

Provider-backed commands are optional and off-gate. If `config init --ai-classify`,
`config update --ai-classify`, `config enrich ...`, `analyze --ai-summary`, or
`explain --ai-summary` fails, rerun the deterministic command without the AI
flag; `check` and CI do not need a model.

Common checks:

- Run `archfit doctor` to confirm the selected provider, model, API-key presence,
  and `.archfit-cache/` status.
- Set keys through `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`; do not put keys in
  `.archfit.yaml`. A local `.env` is best-effort loaded only when the real env var
  is unset.
- Use `--refresh` only for a fresh-provider control run. It bypasses cache reads
  and writes fresh extractor facts and AI responses, so repeated runs can spend
  tokens again.
- Keep `.archfit-cache/llm/` out of git unless cached provider responses are safe
  to share. Responses can quote repository text.
- Evidence packs skip obvious secret-like paths and cache/vendor directories, but
  they are not a secret scanner. Do not run provider-backed commands on docs,
  comments, or public APIs that contain secrets you would not send to the provider.

## Understanding warnings

These are active pipeline warnings. Stderr prefixes each with `warning:`. The
commands below match the warning text. Replace `.archfit.yaml` if your config
lives elsewhere.

| Warning                                                                                                                           | What triggers it                                                                                                                                                                           | Fix command                                                                |
| --------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------- |
| `analyzer coverage gap — some edges may be unscored`                                                                              | `coverage_gaps[]` is non-empty. One or more analyzers did not run, timed out, or are missing.                                                                                              | `archfit doctor --fix`                                                     |
| `0 of N edges scored — coupling strength is unknown`                                                                              | The run found classified edges, but none of them were scored. This usually means config/module metadata needs recalibration.                                                               | `archfit config update -c .archfit.yaml`                                   |
| `all N cross-module edges have unknown strength`                                                                                  | No edges were scored, some cross-module edges abstained, and none were classified as external. The graph is real, but archfit cannot classify edge strength yet.                           | `archfit config enrich abstained -c .archfit.yaml`                         |
| `no internal edges found — module paths may not match source layout`                                                              | Python only. `grimp` ran successfully, but every cross-module edge resolved as external, usually because module `paths:` globs do not match dotted Python module IDs.                      | `archfit config update -c .archfit.yaml`                                   |
| `no source files matched declared module paths — check --root and module globs`                                                   | Your config declares module `paths:`, but none of the files under the active scan root matched any declared module. Wrong `--root` or stale globs are the usual causes.                    | `archfit check --root "." -c .archfit.yaml`                                |
| `warning: grimp: N imports unresolved — check languages.python.package and src layout`                                            | Python `grimp` ran with partial coverage. Common causes are the wrong `languages.python.package`, a `src/` layout not importable in the active environment, or deps missing from the venv. | Set `languages.python.package`, fix package import paths, or install deps. |
| `warning: dependency-cruiser: N of M import specifiers unresolved (P%) — check tsconfig paths/baseUrl and installed dependencies` | TypeScript dependency-cruiser could not resolve import specifiers. Internal edges can fall into the external bucket when aliases or dependencies are missing.                              | Fix `tsconfig` `paths`/`baseUrl` and install `node_modules`.               |
| `SCIP analysis timed out — increase analyzers.scip.timeout or reduce the scope`                                                   | SCIP indexing exceeded the analyzer watchdog. Large Python repos can need more than the default 20m watchdog.                                                                              | Set `analyzers.scip.timeout`, for example `30m`.                           |

## Config update rejects `--json`

`--json` is report-only. Combining it with `--apply`, `--ai-classify`, or
`--refresh` prints `error: --json cannot be combined with ...` and exits `3`
before any discovery, analyzer call, or write. Run the JSON review first, then
run the mutating command separately:

```sh
archfit config update --json -c .archfit.yaml
archfit config update --apply -c .archfit.yaml
```

## Config update reports `no_known_issues` but the config still looks wrong

`no_known_issues` means the discovery diff, the module-field checks, and the
suggestion passes found nothing. It is not a completeness or health claim. The
checks cover structure drift, pending settings edits, `missing_owner`,
`missing_volatility_input`, and `missing_layer`. Wrong-but-present values, wrong
layer assignments, and wrong `paths:` globs are outside their reach — use
`archfit analyze -c .archfit.yaml` and its coverage warnings for those.

## Config update lists modules under `name_drift` or `removed_modules`

Module discovery derives its own key for each module (one per directory), and it
does not have to match the key your config uses — archfit's own config declares
capability modules that span several directories (`assessment-repair` over
`internal/assessment/**`). When a configured module and a discovered module own exactly the same
paths under different names, `config update` reports the pair under
`name_drift`, not as an add plus a remove.

Neither `name_drift` nor `removed_modules` is applied. Re-keying or deleting a
stanza discards its `owner`, `subdomain`, `volatility`, `layer`, and `public`
values, so both stay your decision. Neither raises the status to
`action_required`; a report with only these reads `review_available`.

A `name_drift` stanza IS checked for `missing_owner`,
`missing_volatility_input`, and `missing_layer` — only the key differs, the code
is there. A `removed_modules` stanza is not: discovery found no code for it, so
its fields describe nothing archfit can see. Every such module is listed in
`unchecked_modules` (with the reason) in the JSON document and counted in the
text status line, so an empty `issues` list is never a claim that every
configured module was examined.

## Config update cannot edit module paths

`archfit config update` can now replace flow-style module paths such as
`paths: [pkg/**]`; it rewrites them as a block-style list. Flow-style
`modules: {}` is still unsupported. Convert that section to block YAML first.

## Config compare reports `not_comparable` and no differences at once

These are two separate statements, and both can be true. `coverage evidence:`
grades the analyzer evidence the two runs rest on; the differences section
reports what the two configs actually measured. A duplicated or missing coverage
row, a timed-out analyzer, an analyzer that failed to finish, or an analyzer that
ran on only one side grades the evidence `not_comparable` even when both runs
produced identical findings and an identical score.

An analyzer that ran fully on both sides but left import specifiers unresolved
(the normal dependency-cruiser and grimp state) grades `comparable_with_gaps`
instead: the incompleteness is shared, so the comparison rests on it, and the
detail line names the analyzer so the loss is visible.

Read the coverage detail lines to see which analyzer caused it. A
`not_comparable` grade means "trust this difference less", not "the comparison
failed" — the command still exits `0`.

## Config compare says the candidate scores higher

A higher score is not a better config. `coupling_balance` is measured over the
edges a config declares, so a config that declares fewer modules measures fewer
edges and can score higher while seeing less of the same code. Check the
measurement-loss warnings before reading the score:

| Code                   | Meaning                                                                     |
| ---------------------- | ----------------------------------------------------------------------------- |
| `scored_fraction_fell` | A smaller share of cross-boundary edges got a concrete balance.               |
| `abstained_edges_rose` | More cross-boundary edges had unknown strength or distance.                   |
| `external_edges_rose`  | More edges fell outside the declared module set and left `coupling_balance` entirely. |

A `score_delta` of `null` means one side could not measure `coupling_balance` at
all. That is an unknown, not a tie.

## Installed but still reported missing

`archfit` finds tools through the current process `PATH`. A package manager can
install a tool successfully while your shell still cannot find its bin directory.
Check before reinstalling:

```sh
command -v <tool>
which -a <tool>
echo "$PATH" | tr ':' '\n'
```

Common bin directories:

- Homebrew: `$(brew --prefix)/bin` and `$(brew --prefix)/sbin`.
- npm globals: `$(npm config get prefix)/bin`.
- Go installs: `$(go env GOBIN)`, or `$(go env GOPATH)/bin` when `GOBIN` is empty.
- Rust installs: `${CARGO_HOME:-$HOME/.cargo}/bin`.
- uv tools: `uv tool dir --bin`.

If `which -a` shows multiple copies, the first one wins. Fix PATH order rather
than reinstalling the same tool through another package manager.

## Wrong tool on PATH

Some commands have name collisions:

- **`sg`** must be ast-grep. Verify with `sg --version`; it should print
  `ast-grep ...`. Linux systems may have util-linux `sg` at `/usr/bin/sg`, which
  is not usable by archfit.
- **`node`/`npm`** can come from Homebrew, a distro package, nvm/fnm, or another
  version manager. Use one Node source per shell profile and keep Node `22+` for
  the optional npm tools.
- **`cargo`/`rustc`/`rust-analyzer`** can come from a distro package, Homebrew, or
  rustup. Prefer one rustup-managed stable toolchain unless your CI deliberately
  pins distro Rust.

## Package manager version is too old

Distro packages are preferred on Linux only when they satisfy archfit's tool
constraints. If `apt` or `dnf` offers an old Go, Node, uv, or Rust version, use the
upstream install path from [Tooling reference](tooling.md) instead of mixing random
fallbacks.

For CI, pin exact npm/cargo/go/uv tool versions in setup commands, then run
`archfit doctor` for diagnostics and `archfit check --require-tools` when missing
analyzers should fail the build.

## Metrics show `n/a` / a "Coverage gaps" section

A dimension reading `n/a` (or a `## Coverage gaps` / `## Required tools missing`
section) means an analyzer did not run — archfit is refusing to score absence as
health, not failing. Each gap lists the tool, the metrics it unlocks, and an
install hint; `archfit doctor` lists the same tools. Install the tool to close the
gap. To make CI block on a missing tool instead, opt in with `--require-tools` or
`analyzers.<x>.gate: fail` (exit `1`, a policy violation distinct from exit `3`).

## Config-quality warnings ("N modules under-specified")

These appear as a `## Config warnings` section (md) and, under
`--format legacy-json`, a `config_warnings[]` block — the primary
`archfit.architecture-state.v1` JSON does not carry it. Most clear once modules declare `owner`, `subdomain`, and
`volatility` — draft them with `archfit config enrich owner`/`config enrich volatility` or
`archfit config init --ai-classify -o draft.yaml`, review, then apply. Filling them improves
ownership/volatility distance inputs and can move `coupling_balance` out of `n/a`;
`encapsulation` also needs explicit `public:` / `internal:` globs so edge kinds are
measurable.

## False-positive coupling advisories on a wiring/`cmd` package

If a composition-root or generated package is flagged for fanning out to many
modules, give it a `role:` (e.g. `composition_root`) so archfit reads the fan-out
as cohesion. See
[configuration-reference.md](configuration-reference.md#module-role).

## Reports change between runs / "output written inside analyzed root"

Write report artifacts **outside** the analyzed repo (or its excluded
directories). Built-in excludes already skip `reports/`, `.archfit-cache/`,
`.gitnexus/`, `vendor/`, `node_modules/`, and similar; a report written inside the
root is measured back into the scan and triggers a warning. Use `--root` to scan a
repo from a config that lives elsewhere.

## archfit analyze exits 0 even though I have violations

That is expected. `archfit analyze` is report-only by design. It runs the same
scan pipeline as `archfit check`, but a successful analysis still exits `0` even
when the rendered verdict is FAIL.

Use `archfit check` in CI, pre-push hooks, and agent repair loops. It is the gate
command. Current exit codes are:

- `0` — `healthy`
- `1` — `blocked` (a hard gate failed) — this is the one to fail CI on
- `2` — `needs_attention`. **Normal on a healthy repo in v1**: complexity,
  testability, and operations report `partial` by contract, and any partial
  dimension flags the verdict
- `3` — config or tool error

Use `archfit analyze` for local reports, markdown output, SARIF exports, scorecard
deltas, and `archfit analyze --ai-summary` after the deterministic check when you
want a narrative.

## --refresh vs deleting the cache

They are not the same.

- `--refresh` keeps `.archfit-cache/` in place, skips cache reads, re-runs all
  extractors, and writes fresh results back to the cache. Use it after installing
  or upgrading analyzer tools, or when you want to refresh entries without wiping
  the directory.
- `rm -rf .archfit-cache` deletes every cached fact and AI response. The next run
  is fully cold and repopulates the cache from empty.

Start with `--refresh` when you want fresh analysis output. Delete the cache when
you want a full reset or need to reclaim disk.

## Suspected stale cache

Extractor facts are cached under `.archfit-cache/` keyed by file content, config
slice, and tool version, so a stale result should be impossible by construction
(see [caching.md](caching.md)). To rule the cache out anyway: re-run with
`--refresh` and compare; that bypasses cache reads and writes fresh entries. To
reset everything, delete the directory — `rm -rf .archfit-cache` — and the next
run rebuilds it.

If a command fails with exit code `3`, check config syntax, unknown YAML fields,
missing toolchain, and the exact error printed by `archfit`. Exit `1` is a gate
result, and exit `2` is a warning result, not a config or tool error.
