# archfit review profile

## What this is

- archfit: a Go CLI that turns language-analyzer facts (go/packages, dependency-cruiser,
  grimp, cargo, ast-grep, SCIP) plus a declared `.archfit.yaml` into an architecture state:
  verdict, nine dimensions, findings, a Balanced-Coupling seam ledger, agent repair tasks.
- It is a CI gate. `check` exit code IS the verdict: 0 healthy, 2 needs_attention,
  1 blocked, 3 error. A private GitHub App decodes its JSON with a strict decoder.
- Invariants live in `CLAUDE.md` (binding); `internal/arch_test.go`, `internal/selfmodel_test.go`,
  the erosion gates and the goldens enforce many of them.

## What a real defect looks like here

- A false pass: a rule that cannot fire but counts as evaluated, an absent analyzer read as a
  measured zero, unevaluated evidence promoted to `measured`, a guardrail bypass not caught.
- A false block: a gate that fires on a no-op change (comment edit, path spelling, unused
  language), a finding on code outside the declared scope.
- Non-determinism: cold vs warm or run-to-run output differs (map order, wall clock, absolute
  paths, PIDs). Byte-identical JSON is a contract.
- Contract drift: a new key in `archfit.architecture-state.v1`, baseline v2, or the comparison
  fingerprints without a deliberate version change (the App rejects unknown keys); a finding ID
  that re-keys for unchanged code; unbounded strings or lists the App caps reject.
- Invariant breaks: core-ring packages importing `os`/`os/exec`/YAML/adapters; stages importing
  `internal/config`; a subprocess outside `toolrun.Runner`; a coverage name shared by two
  analyzers; applicability decided by a marker list instead of the extractor's own function;
  exclusions merged twice; the LLM on the gate path; a repository scalar deciding anything.
- Agent misdirection: an `agent_tasks` goal, constraint or `files[]` entry that routes an agent to
  a forbidden target, a non-existent path, or a fix the gate still rejects.
- Book fidelity: the Ch10 equation and anchors are frozen; abstain (unknown) instead of
  inventing strength, distance or volatility.

## Blast radius

- Every repository gated by archfit in CI, every App pilot, and every AI agent following
  `agent_tasks`. A false block stops merges; a false pass silently lets erosion through.

## Reporting bar

- Report: correctness, false pass/block, determinism, contract/schema, invariant, security
  (path traversal, injection into subprocess args), missing tests for new branches or failure
  paths, docs that contradict the code (users and agents act on them).
- Do not report: style the linters own (golangci: revive, gocritic, goconst, gosec, prealloc,
  gofmt/goimports), naming taste, speculative refactors, or wording polish in docs.

## Deliberate conventions (not defects)

- Abstain-not-fake: `n/a` or `unevaluated` instead of a number when evidence is missing.
- No `--no-cache` flag; `--refresh` re-runs extractors. Fact cache stores facts, never scores.
- Go strength from `go/types` beats SCIP; TS/Python/Rust take SCIP's.
- Same-module edges are report-only (`local_coupling`); runtime async evidence is report-only.
- `config init`/`update` never destroy a configured module stanza; `Removed` is review-only.
- Comments explain why; tests are table-driven and mock only system boundaries.
- Leftover git worktrees under `.claude/` are tool state; test walkers skip dot directories.
