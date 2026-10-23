# Corpus sweep

Use the current sweep procedure in [`docs/development.md`](../../../docs/development.md).
That guide owns supported corpus labels, commands, workspace isolation, and
result handling. This reference lists the evaluation checks.

## Before the sweep

- Build the current branch binary and run `.bin/archfit doctor`.
- Use disposable checkouts selected by `--workspace-root`.
- Keep target configuration and policy files unchanged.
- Prepare version-2 candidate configs in the disposable checkouts.
- Record missing checkouts, analyzers, and toolchains as unverified.

## Review each result

Check state JSON, verdict and exit agreement, dimension coverage, requested
format parity, and repeated-run determinism. Keep AI summaries separate from
deterministic results. An accepted unverified gap is not a pass.

Classify each anomaly as a product defect, policy migration defect, target
architecture finding, tool or environment gap, or documentation defect. Do not
edit target policies to improve results. Cite the exact command and evidence for
each finding.
