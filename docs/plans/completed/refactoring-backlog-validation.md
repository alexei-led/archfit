# Refactoring backlog — multi-repo validation sweep

Date: 2026-06-12. Binary: `.bin/archfit` built from `refactor/architecture-backlog`
@ 7788a53 (Tasks 1–7 complete). Runbook from
`docs/plans/20260612-archfit-refactoring-backlog.md` Task 9.

## Common checks (all four repos)

| Repo      | double-run byte-identical | exit 0 / clean stderr | SARIF 2.1.0 schema | baseline→check parity            |
| --------- | ------------------------- | --------------------- | ------------------ | -------------------------------- |
| archfit   | PASS                      | PASS (0 bytes stderr) | PASS               | PASS (pass verdict, zero deltas) |
| ccgram    | PASS                      | PASS (0 bytes stderr) | PASS               | PASS (pass verdict, zero deltas) |
| pumba     | PASS                      | PASS (0 bytes stderr) | PASS               | PASS (pass verdict, zero deltas) |
| codegraph | PASS                      | PASS (0 bytes stderr) | PASS               | PASS (pass verdict, zero deltas) |

SARIF validated with `uvx check-jsonschema --schemafile
https://json.schemastore.org/sarif-2.1.0.json` → "ok -- validation done" ×4.

## Repo-specific

### archfit (scip + complexity on)

- Verdict **pass**; arch ring tests green (`TestArchImports` — now also guards
  adapters→engine, core→baseline, and scope's process-freedom).
- **Baseline is EMPTY** after Task 7: zero accepted findings — all 5 historical
  engine→scope layer inversions resolved by the scope decoupling +
  reclassification, none replaced them.
- `Run` CCN: 28 → **17** (no longer in the complexity top list; repo max is
  `facts.Build` at 26, under the gocyclo 30 threshold).
- structural_weight: cmd/archfit 1395 → **1287 LOC** (collectors moved to
  `extract/loc` + `extract/complexity`). Still listed as a god-module (7× of
  176 median): the remaining bulk is `enrich`/`explain --llm` plumbing, which
  MUST stay in cmd — the arch test forbids `internal/llm` anywhere else. This
  is the honest floor, not an oversight.
- Facts block: **105 file-fact entries** (≥ 90 required).
- Delta mode `check --base main~5`: change_locality **82 cross-module edges
  from 116 changed files, forward reach 67 files** — sane, non-n/a, verdict pass.

### ccgram (scip on, Python)

- Facts: **383 entries** (meets the 383+ bar).
- 3 active gate findings (pre-existing `no-import-cycles`) → exactly **3
  `agent_tasks`**, each with the correct reproducible validation command
  (`archfit check -c <config> --full`).
- Labels-file checks: see Task 8 section of
  `tranche2-enrich-validation.md` (frontier enrich run).
- Baseline artifact created during parity testing was removed; repo left as
  found.

### pumba (Go; tools.scip enabled TEMPORARILY)

- scip-go indexed **424 files / 3012 symbols** (coverage ok); facts block
  populated with **38 entries**; verdict pass.
- Config restored byte-identical from backup after the run (diff clean);
  baseline artifact removed. No borrowed-config edits left behind.

### codegraph (TypeScript; tools.scip enabled TEMPORARILY)

- scip-typescript path **works**: **404 files / 5902 symbols** indexed; facts
  block **124 entries**; verdict pass. No coverage gap to record.
- Config restored byte-identical; baseline artifact removed.

## Suite + lint

- `go test -count=1 ./...` green and `make lint` 0 issues at every task
  boundary (Tasks 1–7) and re-verified at sweep time.
- Push: see plan Task 9 (exit-128 diagnosis + final push happen at merge).
