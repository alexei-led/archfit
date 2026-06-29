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
`volatility` — draft them with `archfit enrich --owner`/`--volatility` or
`archfit autopilot`, review, then pin. Filling them also makes `encapsulation` measurable and lets `coupling_balance`
move out of `n/a`.

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

If a command fails with exit code `3`, check config syntax, unknown YAML fields,
missing toolchain, and the exact error printed by `archfit`. Exit `1` is a gate
result (a finding or an opted-in missing required tool), not an error.
