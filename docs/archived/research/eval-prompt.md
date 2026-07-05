# archfit evaluation prompt

Paste the block below into a **fresh Claude Code session, run from the archfit repo root**, to
run a complete, high-quality evaluation of archfit across the test corpus — **full + delta +
LLM**. The eval judges archfit's **usability, stability, usefulness, and correctness**, the
**real architectural value of its findings** for each analyzed project, and its **conformance
to the Balanced Coupling book** (methodology, terminology, formula, behavior). The eval driver
acts as the **simulated project architect** for every repo it analyzes.

> archfit implements Vlad Khononov's **Balanced Coupling** model — book: _Balancing Coupling in
> Software Design_. Its score (`ScoreVersion = "bc_score.v3"`, formula `balance = max(|S−D|,
10−V) + 1`) and anchors are adopted from the book verbatim. Hold archfit's output to the
> book, not to a similar-but-different variant.

---

## PROMPT

You are evaluating archfit before a release **and** acting as the architect of each project it
analyzes. Be empirical: build it, run it yourself, witness live output, verify findings, and
correct wrong conclusions. Capture exit code + walltime + stderr for every invocation (no
`tee`/pipe masking — direct redirect or `${PIPESTATUS[0]}`). Use the **advisor** before
committing to an interpretation and before declaring done. Parallelize _analysis_ with
subagents, but run the archfit invocations yourself. Consult `docs/test-corpus.md` for the repo
list, paths, and delta bases.

archfit follows the **Balanced Coupling** model. Internalize it before judging output:

- **Integration strength** (likelihood of cascading change) — `intrusive` > `functional` >
  `model` > `contract`.
- **Distance** (cost of cascading change) — socio-technical: code structure **+** org/ownership
  **+** runtime coupling.
- **Volatility** (probability of change at all) — evaluated from **DDD subdomains**
  (core=high, supporting=low, generic=low functional volatility), **not commit history**.
- **Balance**: modularity = strength XOR distance; complexity = both high (tight coupling /
  distributed monolith) **or** both low (low cohesion / big ball of mud). Low volatility
  _neutralizes_ unbalanced coupling.

### 1. Setup

1. `make build` → `.bin/archfit`. Run `.bin/archfit doctor` and record which analyzers are
   present (scip-go/ts/python, cargo + rust-analyzer + cargo-modules, jscpd, grimp/uv,
   ast-grep). To install missing toolchains, use `.bin/archfit doctor --fix [--lang go,ts,py,rust]`
   (dry-run first: `doctor --fix --dry-run`).
2. Run every repo **from the archfit dir** with `--root <canonical-path>` so `.env`
   auto-loads the LLM key (the `.env` is read from CWD only). On macOS pass **canonical-case**
   `--root` (`git -C <repo> rev-parse --show-toplevel`) — the delta path rejects case-variant
   roots.
3. Make a fresh `reports/eval-<YYYY-MM-DD>/` (gitignored) for all artifacts, including your
   per-repo architect notes (below).

### 2. Corpus bootstrap — clone what's missing

Read `docs/test-corpus.md` for the repo list, local paths (`~/workspace/<name>`), and prev-minor
delta bases. For each repo:

- If `~/workspace/<name>` is missing, **clone it** with full history (needed for git-log
  ownership and tag-based delta selection):
  `git clone --filter=blob:none https://github.com/<owner>/<name> ~/workspace/<name>`
  (derive `<owner>/<name>` from the corpus tables — e.g. `prometheus/prometheus`, `astral-sh/ruff`,
  `storybookjs/storybook`, `PrefectHQ/prefect`, `tokio-rs/tokio`; the local dogfood repos —
  archfit, spotinfo, pumba, ccgram, yazi, herdr, omni — are the operator's own).
- Confirm each clone has tags (`git -C <repo> tag | head`) so the delta base can be derived.

### 3. Config as architect — create missing, update existing

Before analyzing each repo, ensure a good `.archfit.yaml` exists, **authored with a human
architect's understanding of the project** — not a blind scaffold:

1. **Understand the project.** Read its README/docs and top-level layout. Identify the real
   modules (top-level packages / crates / workspace members / directories), what each does, and
   the business/technical domain.
2. **Classify per the book.** For each module decide its **subdomain** — _core_ (the project's
   competitive advantage / "interesting problems"; high volatility), _supporting_ (needed but
   undifferentiated; low volatility), or _generic_ (solved/off-the-shelf; low functional
   volatility) — and the resulting volatility. Reason from the **domain**, not commit frequency
   (the book is explicit: volatility is _essential_, evaluated via subdomains).
3. **Author the config.**
   - **Local dogfood repos** already ship `.archfit.yaml`. Back it up, then
     `.bin/archfit config update --root <repo> --config <cfg>` to sync structure. Review and
     refine as architect; never discard human-authored module mapping.
   - **Cloned multi-author repos** need a config — write it into the **eval dir** (keep their
     trees clean): declare modules per top-level package/crate/dir; enable analyzers (`scip`,
     `clones`, `syntax`; `cargo_modules` for single-crate Rust); set
     `coupling.volatility_cascade: true`; set each module's `subdomain`/`volatility` from your
     architect classification; and leave **`owner:` BLANK** so archfit resolves owners from
     CODEOWNERS (prometheus/ruff/storybook/prefect) or git-history (tokio). This owner-less
     setup is the point — it exercises socio-technical owner-distance.
   - You may **seed** with `.bin/archfit config init --root <repo> -o <eval-cfg>` (deterministic
     scaffold) or `config init --llm` (archfit's own LLM classification). Use
     `.bin/archfit config enrich {owner|volatility|subdomain}` to draft a single dimension;
     review the draft before `--apply`.
4. **Record your architect model** of each repo (modules, subdomains, volatility, expected
   hot/fragile seams) in the eval dir — you will use it to judge findings (Dimension C) and to
   measure agreement (Cross-cutting).

> **Eval signal:** if you use archfit's `config init --llm` / `config enrich`, record whether
> archfit's LLM classification **matches your independent architect judgment**. Divergence is a
> finding — does archfit's classifier reason like an architect?

### 4. Delta base selection (per repo, at run time)

- List tags newest-first: `git -C <repo> tag --sort=-creatordate`.
- Pick the **previous minor** release before HEAD/latest (e.g. latest `v3.12.0` → base
  `v3.11.x`). If minors are sparse, pick a **significant version change** instead, and say why.
  Respect per-crate tag schemes (e.g. `tokio-1.52.x` → `tokio-1.51.x`). Confirm the base
  resolves (`git -C <repo> rev-parse <tag>`). The corpus doc lists starting points; refine to
  the exact prev-minor patch here.

### 5. Run matrix (per repo)

Capture each to a file; record exit code and walltime:

```
.bin/archfit analyze --full --root <root> --config <cfg> --advisory --json --quiet        > full.json
.bin/archfit analyze --full --root <root> --config <cfg> --advisory --llm --markdown       > full-llm.md
.bin/archfit analyze --base <prev-minor> --full --root <root> --config <cfg> --advisory     > delta.txt
```

Run heavy repos (prometheus, storybook, prefect, omni) **one at a time** for clean walltimes;
never run two heavy SCIP/`packages.Load` jobs concurrently. Witness a few runs live
(`--progress plain`) to judge the progress UX.

### 6. Evaluation dimensions

For each repo, cross-reference `full.json` + `full-llm.md` + `delta.txt` **and your architect
model**:

**A. Usability** — command ergonomics on the current surface (`analyze` is the default command;
config authoring is `config init`/`config update`/`config enrich`; tools are
`doctor [--fix]`). Help clarity, flag consistency, progress staging. Does `--json` carry the
score + `coupling_balance` + delta, or only text? How much friction in the config-authoring
loop?

**B. Stability** — exit codes, walltimes, crashes, partial failures, analyzer timeouts. Check
`tool_coverage[]`: any `absent`/`disabled`/`partial` that should be `ok`?

**C. Usefulness / finding quality (for the project)** — using your architect understanding of
the repo: are the flagged unbalanced couplings **real design smells** the maintainers would
recognize? Are advisories actionable and **triageable**, or a per-edge flood on dense graphs?
Do `agent_tasks[]` describe a real fix path (goal, constraints, files, validation)? **Would you,
as this project's architect, act on the top findings?** Note false positives and missed-but-real
seams.

**D. Correctness** —

1. _Metric completeness_ — list every metric's band/display; count `n/a`. Is each `n/a`
   correct-by-design (e.g. `encapsulation` needs intrusive edges → mostly Python) or a gap?
2. _Edge classification_ — `classified_edges`: scored vs abstained vs external vs same_module.
   High abstain % (unknown strength) = weak coverage; flag it and its cause.
3. _Ownership_ — does `by_distance` contain `cross_module_different_owner`? Is `owner_source`
   populated (`codeowners` | `git` | `none`)? Do teams resolve on CODEOWNERS repos? Does tokio
   (no CODEOWNERS) yield ≥2 real git-history owners (not bot/vendor/name-variant noise)? Do
   solo repos correctly collapse to degenerate code-structure distance?
4. _Scoring realism_ — is the 0–100 score and `coupling_balance` band defensible for the
   repo's real structure? Do scores discriminate across repos or compress into one band? Is
   the score over-sensitive to config `volatility` labels?
5. _Delta_ — does the dimension table render meaningfully vs the prev minor, or collapse to
   "no change"? Is it informative as a version diff?

**E. Book conformance (Balanced Coupling — the headline new axis)** — verify archfit _follows
the book_, not a homegrown approximation:

- **Three dimensions present and correct**: integration strength named per book
  (`intrusive` > `functional` > `model` > `contract`); distance socio-technical (code + owner
  - runtime); volatility from subdomains, **not** churn.
- **Formula**: spot-check a scored edge — pull its S/D/V and confirm the reported balance
  equals `max(|S−D|, 10−V) + 1`. Does the math match the book?
- **Balance semantics**: does archfit treat tight coupling (high S + high D) **and** low
  cohesion (low S + low D) as the problems, and balanced (S XOR D) as healthy? Does **low
  volatility neutralize** unbalanced coupling (pragmatic balance)?
- **Abstain-not-fake**: when strength or distance is unknown, is the edge unscored (never
  invented ordinals)? This is book-faithful — don't fabricate knowledge.
- **Volatility source**: subdomain/config-driven; `runtime_async` stays **report-only** (never
  feeds distance or balance) — confirm.
- **Terminology**: output and docs use the book's vocabulary without drift.
- **Deviations**: flag any place archfit departs from the book (formula, anchors, terminology,
  methodology) as a **high-priority** finding — archfit's stated contract is to adopt the book
  verbatim.

### 7. Cross-cutting

- **Architect agreement** — compare archfit's classification (subdomain / volatility / strength)
  to your independent architect judgment per repo. Quantify agreement; explain divergences.
- **Regression re-checks** — config schema strict-load (do all corpus configs load? is the
  `config init` template valid? unknown keys loud, not silent?); CODEOWNERS under `--root`
  subtree (omni: do teams resolve and `different_owner` edges appear?); delta case-variant
  `--root`; whether `--json` carries score + `coupling_balance` + delta. Note fixed vs open.

### 8. Deliverable

Write `reports/eval-<date>/00-FINDINGS.md`:

- A **verdict** (release-ready?).
- **Severity-ordered findings** with file:line / output evidence.
- A **book-conformance section** (formula / terminology / methodology checks; deviations called
  out explicitly).
- A **metric-applicability table** (which metrics fire per language).
- A **per-repo results table** (score, edges, findings, walltime, owner-distance outcome,
  architect-agreement).
- A **usability / stability / usefulness / correctness** assessment.

Verify each non-trivial finding empirically (re-run with a changed input) before writing it; if
a check contradicts a claim, correct the claim.

---

## Notes for the operator

- Prior eval artifacts: `reports/eval-2026-06-30/00-FINDINGS.md`; owner probe:
  `reports/eval-2026-06-30/owner-detection-probe.md`.
- The multi-author repos are the cross-team owner-distance signal; the local set is mostly
  single-owner and can't exercise it.
- Book reference for conformance checks: Vlad Khononov, _Balancing Coupling in Software Design_.
  archfit's scorer design notes live in `CLAUDE.md` ("Coupling scorer — key design facts").
