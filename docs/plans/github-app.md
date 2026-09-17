# archfit as a GitHub App — moved

The design plan now lives with the work: **`alexei-led/archfit-app`**, at
`docs/plans/20260916-github-app.md` (private).

It moved because this `docs/plans/` should carry only work this repository does.
The App is a separate product in a separate repository, and a plan filed here
would become a backlog nobody can tell the owner of.

## What stayed here

Phase 0 of that plan was engine work and shipped from this repository:

- **v2.2.0** — analyzer identity published in `measurement.tool_versions`, and
  `archfit.state.schema.json`: a machine-readable contract for the
  `archfit.architecture-state.v1` output, generated from the report structs by
  `internal/reportschema` and regenerated with `make schema`.
- **v2.2.1** — rule conformance stopped depending on which analyzers ran, so the
  `intent` dimension can reach `measured`.

The obligations this engine carries for the App are invariants, not plans, and
live in `CLAUDE.md` — the state contract, the four comparability fingerprints,
the coverage-row semantics, selector vocabulary, and the published schemas.

Still open in the engine, recorded in
[`20260826-post-migration-followups.md`](20260826-post-migration-followups.md):
wiring the measurement profile into `CompareFingerprints` as a fifth
fingerprint. It is a breaking change to the comparison contract and the baseline
schema, so it needs its own release, and the App's `action_required` semantics
should be designed before it lands.

## The three repositories

| Repo | Owns | License |
| --- | --- | --- |
| `archfit` (this one) | engine, CLI, images, wire contracts | Apache-2.0 |
| [`archfit-action`](https://github.com/alexei-led/archfit-action) | execution/upload adapter for CI | Apache-2.0 |
| `archfit-app` | tenancy, approved policy, baselines, PR feedback | proprietary |
