# Troubleshooting

Run `archfit doctor` first when extraction looks incomplete.

Common fixes:

- Use `tools.<language>.enabled: off` to disable an adapter while calibrating.
- Use `--format json` when an AI agent or script needs structured output.
- Narrow module paths if generated config is noisy.
- Prefer an expiring exception over deleting a rule for intentional findings.
- Check that optional analyzer tools are installed before enabling them.
- Re-run `archfit baseline --full` only after reviewing accepted findings.

## Metrics show `n/a` / a "Coverage gaps" section

A dimension reading `n/a` (or a `## Coverage gaps` / `## Required tools missing`
section) means an analyzer did not run — archfit is refusing to score absence as
health, not failing. Each gap lists the tool, the metrics it unlocks, and an
install hint; `archfit doctor` lists the same tools. Install the tool to close the
gap. To make CI block on a missing tool instead, opt in with `--require-tools` or
`tools.<x>.gate: fail` (exit `1`, a policy violation distinct from exit `3`).

## Config-quality warnings ("N modules under-specified")

These now appear as a `## Config warnings` section (md) and `config_warnings[]`
(json), not just stderr. Most clear once modules declare `owner`, `subdomain`, and
`volatility` — draft them with `archfit enrich --owner`/`--volatility` or
`archfit autopilot`, review, then pin. Filling them also makes `encapsulation`
measurable, so `boundary_integrity` and `coupling_balance` leave `n/a`.

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
