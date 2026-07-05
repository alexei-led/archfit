# Wave 1 deterministic baseline

Baseline source: `reports/book-alignment-review-2026-07-05/00-REVIEW.md`, especially the deterministic gaps in review §6 and the short-verdict gap list.

## Characterized behavior

- Clone-only duplicated knowledge is currently advisory-only.
  - `internal/classify/clone_only_test.go` pins that a cross-module clone pair with no import edge is returned by `CloneOnlyPairs`, but `classify.Run` does not invent a classified graph edge for it.
  - The same test now pins label suppression for approved labels and LLM labels.
  - `internal/engine/duplicated_knowledge_test.go` pins that clone-only evidence emits one `bc/duplicated_knowledge` advisory, emits no `bc/imbalanced_coupling` advisory, and leaves `classified_edges` at zero.
- Tiny fully scored graphs can currently read as high confidence.
  - `internal/score/score_test.go` pins that one or two fully scored cross-boundary edges with mean book balance 10/10 synthesize as `strong` with `high` confidence under the current 100% scored-fraction policy.
- Golden output guard remains byte-identical.
  - No golden fixture update was needed; the task adds characterization tests only.

## Current self/corpus scores

`ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib` produced:

| repo      | score |  band | scored | abstained | external |
| --------- | ----- | ----: | -----: | --------: | -------: |
| archfit   | 42    | mixed |    337 |         0 |      572 |
| ccgram    | 55    | mixed |    497 |        18 |        0 |
| herdr     | 29    |  poor |    686 |        18 |       24 |
| storybook | 48    | mixed |    310 |         0 |      181 |

Skipped corpus repos: none. All expected corpus inputs were present under `${ATTRIB_REPOS_DIR:-$HOME/Workspace}`.

Self gate snapshot from `make archfit`:

- Gate: PASS, 0 blocking.
- Decision: ACCEPTABLE WITH WATCH ITEMS.
- Warnings: 80 advisory.
- Score: 42 / 100, mixed.
- coupling_balance: 42/100, mixed.

## Impact snapshot

- `gitnexus impact clone_only.go --direction upstream --depth 3`: LOW risk, 11 impacted files, 4 direct import dependents.
- `gitnexus impact score_boundary_coupling.go --direction upstream --depth 3`: MEDIUM risk, 5 direct import dependents.
- `gitnexus detect-changes --scope all`: LOW risk, 3 changed files, no affected processes reported.

## Verification results

- `go test ./internal/classify/... ./internal/engine/... ./internal/score/...`: PASS.
  - `internal/classify`: ok.
  - `internal/engine`: ok.
  - `internal/score`: ok.
- `make build`: PASS.
  - Built `.bin/archfit` with `CGO_ENABLED=0 go build`.
- `ATTRIB_REPOS_DIR=${ATTRIB_REPOS_DIR:-$HOME/Workspace} make corpus-attrib`: PASS.
  - Produced the attribution table above.
- Additional local checks:
  - `make lint`: PASS, 0 issues.
  - `make archfit`: PASS, 0 blocking.
