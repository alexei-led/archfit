# Troubleshooting

Run `archfit doctor` first when extraction looks incomplete.

Common fixes:

- Use `languages.<language>.enabled: false` or `analyzers.<x>.enabled: false` to disable an adapter while calibrating.
- Use `--format json` when an AI agent or script needs structured output.
- Narrow module paths if generated config is noisy.
- Prefer an expiring waiver over deleting a rule for intentional findings.
- Check that optional analyzer tools are installed before enabling them.
- Re-run `archfit baseline --full` only after reviewing accepted findings.

For platform setup, package-manager choices, exact tool versions, home pages, and
PATH checks, see [Tooling reference](tooling.md).

## LLM command fails, costs too much, or should not send repo text

Provider-backed commands are optional and off-gate. If `config init --llm`,
`config update --llm`, `config enrich ...`, `analyze --llm`, or `explain --llm`
fails, rerun the deterministic command without `--llm`; `analyze --gate` and CI do
not need a model.

Common checks:

- Run `archfit doctor` to confirm the selected provider, model, API-key presence,
  and `.archfit-cache/` status.
- Set keys through `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`; do not put keys in
  `.archfit.yaml`. A local `.env` is best-effort loaded only when the real env var
  is unset.
- Use `--no-cache` only for a fresh-provider control run. It bypasses extractor
  facts and LLM response reads/writes, so repeated runs can spend tokens again.
- Keep `.archfit-cache/llm/` out of git unless cached provider responses are safe
  to share. Responses can quote repository text.
- Evidence packs skip obvious secret-like paths and cache/vendor directories, but
  they are not a secret scanner. Do not run provider-backed commands on docs,
  comments, or public APIs that contain secrets you would not send to the provider.

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
`archfit doctor` for diagnostics and `archfit analyze --gate --require-tools`
when missing analyzers should fail the build.

## Metrics show `n/a` / a "Coverage gaps" section

A dimension reading `n/a` (or a `## Coverage gaps` / `## Required tools missing`
section) means an analyzer did not run — archfit is refusing to score absence as
health, not failing. Each gap lists the tool, the metrics it unlocks, and an
install hint; `archfit doctor` lists the same tools. Install the tool to close the
gap. To make CI block on a missing tool instead, opt in with `--require-tools` or
`analyzers.<x>.gate: fail` (exit `1`, a policy violation distinct from exit `3`).

## Config-quality warnings ("N modules under-specified")

These now appear as a `## Config warnings` section (md) and `config_warnings[]`
(json), not just stderr. Most clear once modules declare `owner`, `subdomain`, and
`volatility` — draft them with `archfit config enrich owner`/`config enrich volatility` or
`archfit config init --llm -o draft.yaml`, review, then apply. Filling them improves
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

## Suspected stale cache

Extractor facts are cached under `.archfit-cache/` keyed by file content, config
slice, and tool version, so a stale result should be impossible by construction
(see [caching.md](caching.md)). To rule the cache out anyway: re-run with
`--no-cache` (a control run that neither reads nor writes cache entries) and
compare; to reset, delete the directory — `rm -rf .archfit-cache` — and the next
run rebuilds it.

If a command fails with exit code `3`, check config syntax, unknown YAML fields,
missing toolchain, and the exact error printed by `archfit`. Exit `1` is a gate
result (a finding or an opted-in missing required tool), not an error.
