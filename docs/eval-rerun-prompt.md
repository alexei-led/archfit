# archfit eval rerun prompt

Paste the block below into a fresh Claude Code session (run from the archfit repo root) to
re-run a complete, high-quality evaluation of `archfit analyze` across the test corpus. It
encodes the methodology and pitfalls learned in the 2026-06-30 eval. Goal: judge archfit's
**evaluation quality and completeness** — does it gather every supporting metric properly,
score realistically, resolve ownership, and stay stable/usable across languages and scale.

---

## PROMPT

You are evaluating the `archfit analyze` CLI before release. Be empirical: run it, witness
live output, verify findings, and correct wrong conclusions. Use the advisor before
committing to an interpretation and before declaring done. Parallelize _analysis_ with
subagents, but run the archfit invocations yourself and capture exit code + walltime +
stderr. Consult `docs/test-corpus.md` for the repo list, paths, and delta bases.

### Setup

1. `make build` → `.bin/archfit`; run `.bin/archfit doctor` and record which analyzers are
   present (scip-go/ts/python, cargo+rust-analyzer+cargo-modules, jscpd, grimp/uv, ast-grep).
2. Run every repo **from the archfit dir** with `--root <canonical-path>` so `.env`
   auto-loads the LLM key (the `.env` is read from CWD only). On macOS pass **canonical-case**
   `--root` (`git -C <repo> rev-parse --show-toplevel`) — the delta path rejects case-variant
   roots.
3. Make a fresh `reports/eval-<YYYY-MM-DD>/` (gitignored) for all artifacts.

### Corpus & configs

- Local dogfood repos (archfit, spotinfo, pumba, ccgram, yazi, herdr, omni/scheduled-tasks)
  already ship `.archfit.yaml`. Use them as-is (back each up first).
- The cloned multi-author repos (prometheus, ruff, storybook, prefect, tokio) need a config.
  Write one per repo into the eval dir (keep their trees clean): declare modules per
  top-level package / crate / directory, enable analyzers (`scip`, `clones`, `syntax`;
  `cargo_modules` for single-crate Rust), set `coupling.volatility_cascade: true`, and
  **declare NO `owner:`** — leave owners blank so archfit resolves them from CODEOWNERS
  (prometheus/ruff/storybook/prefect) or git-history (tokio). This is the point of these
  repos: exercise socio-technical owner-distance.

### Run matrix (per repo)

Capture each to a file; record exit code (no `tee`/pipe masking — use direct redirect or
`${PIPESTATUS[0]}`) and walltime:

```
.bin/archfit analyze --full --root <root> --config <cfg> --advisory --json --quiet      > full.json
.bin/archfit analyze --full --root <root> --config <cfg> --advisory --llm --markdown     > full-llm.md
.bin/archfit analyze --base <prev-minor> --full --root <root> --config <cfg> --advisory   > delta.txt
```

Run heavy repos (prometheus, storybook, prefect, omni) **one at a time** for clean walltimes;
never run two heavy SCIP/packages.Load jobs concurrently. Witness a few runs live
(`--progress plain`) to judge the progress UX.

### Evaluation checklist (quality + completeness)

For each repo, from `full.json` + `full-llm.md` + `delta.txt`, assess:

1. **Metric completeness** — list every metric's band/display; count `n/a`. Is each `n/a`
   correct-by-design (e.g. `encapsulation` needs intrusive edges → Python-mostly) or a gap?
   Check `tool_coverage[]`: any `absent`/`disabled`/`partial` that should be `ok`?
2. **Edge classification** — `classified_edges`: scored vs abstained vs external vs
   same_module. High abstain% (unknown strength) = weak coverage; flag it and its cause.
3. **Ownership (headline for multi-author repos)** — does `by_distance` contain
   `cross_module_different_owner`? Is `owner_source` populated (codeowners | git | none)? On
   CODEOWNERS repos, do teams resolve? On tokio (no CODEOWNERS), does git-history yield ≥2
   real owners (not bot/vendor/name-variant noise)? On solo repos, does it correctly collapse
   to degenerate (code-structure distance)?
4. **Scoring realism** — is the 0–100 score and `coupling_balance` band defensible for the
   repo's real structure? Do scores discriminate across repos or compress into one band? Is
   the score sensitive mostly to config `volatility` labels?
5. **Advisory volume** — findings count by severity. Is it triageable or a flood (per-edge
   advisories on dense graphs)?
6. **Delta** — does the dimension table render meaningfully vs prev minor, or collapse to
   "no change"? Is the delta informative as a version diff?
7. **LLM** — grounded in real modules/LOC/fan-in? Did post-verification drop unsupported
   claims? Did it stay off-gate?
8. **Stability/usability** — exit codes, walltimes, crashes, partial failures; progress
   staging legibility; whether `--json` carries the score + coupling_balance + delta (or only
   text does).

### Regression re-checks (from the 2026-06-30 eval)

Confirm status of: **F1** config schema (`analyzers:` strict; do all corpus configs load? is
the `init` template valid? unknown-key handling loud-not-silent?), **F2** CODEOWNERS under
`--root` subtree (omni: do teams resolve and `different_owner` edges appear?), **F4** delta
case-variant `--root`, **F5** `--json` omits score/coupling_balance/delta. Note which are
fixed vs open.

### Deliverable

Write `reports/eval-<date>/00-FINDINGS.md`: a verdict (release-ready?), severity-ordered
findings with file:line evidence, a metric-applicability table (which metrics fire per
language), a per-repo results table (score, edges, findings, walltime, owner-distance
outcome), and a stability/capability/reliability/usability/realism assessment. Verify each
non-trivial finding empirically (re-run with a changed input) before writing it; if a check
contradicts a claim, correct the claim.

---

## Notes for the operator

- Prior eval: `reports/eval-2026-06-30/00-FINDINGS.md`; owner probe:
  `reports/eval-2026-06-30/owner-detection-probe.md`; runner: `reports/eval-2026-06-30/run_repo.sh`.
- The multi-author repos are the new signal — the local set is mostly single-owner and can't
  exercise cross-team owner-distance.
