# Deleted compatibility contracts

The former `internal/model/scan` and `internal/model/diagnostic` packages were
compatibility aliases, not independent domain models. Their production import
count was zero; tests now use `internal/model/report` and owner-specific
contracts directly. Both packages are deleted.

The former `internal/evidence/ports` facade was also an alias-only compatibility layer.
Evidence ports are owned by `internal/evidence/ports`; rendering is owned by
`internal/report/ports`. Tests use those owner packages directly, and the
facade and its generated mocks are deleted.

`internal/view` remains intentionally present. Production code still has 32
consumers, so it is explicit migration debt rather than a deleted contract.
The model-surface golden was regenerated for the resulting published model
surface. The report schema identifier remains `archfit.diagnostic.v2` for
wire compatibility; package ownership is the contract change.

The former `cmd/archfit/technical_compat.go` façade was also deleted. Configuration projection now lives in `internal/config`, coverage and volatility evidence in `internal/evidence/acquisition`, distance and coupling behavior in `internal/assessment/evaluation`, worktree mechanics in `internal/history/git`, and delta coordination in `internal/application`; command files retain only composition and exit/output wiring.
