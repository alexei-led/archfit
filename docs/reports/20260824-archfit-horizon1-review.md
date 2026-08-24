# Horizon 1 architecture re-review

Date: 2026-08-24  
Scope: report projection and diagnostic compatibility migration on PR #33

## Decision

**GO for Horizon 2.** No blocking regression found.

## Completed boundaries

- `report.Document` is a concrete report contract in
  `internal/model/report/document.go`.
- Renderer ports and output adapters consume `report.Document`.
- `internal/output/**` and `internal/ports/**` have zero imports of
  `internal/model/scan`.
- Production imports of `internal/model/diagnostic` fell from 19 to zero.
- `internal/model/scan` is now a compatibility alias layer.
- Model purity checks discover all `internal/model/**` packages automatically.
- Output, CLI byte-identical, architecture, lint, test, build, and vet checks
  pass.

## Compatibility evidence

The old `scan.Diagnostic` and new `report.Document` fields were compared by
name and JSON tag:

- 31 fields before.
- 31 fields after.
- Field order identical.
- JSON tags identical.
- `SchemaVersion` remains `archfit.diagnostic.v2`.

## Deterministic verification

- `make all`: pass.
- `go vet ./...`: pass.
- CLI contract suite: 24 passed, 0 failed.
- Archfit check: exit 2 advisory result, 0 blockers, 86 warnings.
- `coupling_balance`: 53/100, mixed.
- Critical edges: 14.
- Fixed-config delta against `main`: 55 → 53.
- Module cycles: 0.
- `git diff --check`: pass.
- CodeGraph refreshed and current at the working tree.

The score fell from 54 to 53 because the migration exposes honest report and
contract edges. This is acceptable. No `.archfit.yaml` labels or baseline rules
were changed.

## Remaining low-risk migration debt

- Production still uses `internal/model/scan` in application and engine code.
  Horizon 2 must add a non-increasing scan-import ratchet before dissolving the
  compatibility package.
- Compatibility aliases remain in `internal/model/diagnostic` and
  `internal/model/scan` for tests and application migration.
- The generated report's module pair list still reflects the provisional
  type-centric map. Do not update target labels until physical context moves
  land.

## Horizon 2 gate

Proceed with application use-case extraction and stage separation. Keep the
current volatility ledger and module map fixed while measuring source changes.
Re-run this review after CLI/engine migration before moving relationship and
assessment code.
