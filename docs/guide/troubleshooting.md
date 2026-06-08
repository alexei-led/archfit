# Troubleshooting

Run `archfit doctor` first when extraction looks incomplete.

Common fixes:

- Use `tools.<language>.enabled: off` to disable an adapter while calibrating.
- Use `--format json` when an AI agent or script needs structured output.
- Narrow module paths if generated config is noisy.
- Prefer an expiring exception over deleting a rule for intentional findings.
- Check that optional language tools are installed before enabling them.
- Re-run `archfit baseline --full` only after reviewing accepted findings.

If a command fails with exit code `3`, check config syntax, unknown YAML fields,
missing toolchain, and the exact error printed by `archfit`.
