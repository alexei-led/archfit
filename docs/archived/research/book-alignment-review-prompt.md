# archfit — Book-Alignment Review Prompt

Paste the block below into a **fresh session of a capable model, run from the archfit repo root**
(`/Users/alexei/Workspace/archfit`), on branch **`fix/wave1-gate-integrity`** (ScoreVersion
`bc_score.v4`). It reviews how well archfit implements the **Balanced Coupling** book and returns
a scored, prioritized report. Read-only: the reviewer must not modify source.

---

## PROMPT

You are an expert software-architecture reviewer auditing the Go CLI **archfit** for fidelity to
the book **_Balancing Coupling in Software Design_** by Vlad Khononov. Produce a **scored,
well-organized report** (format specified at the end). Be empirical and skeptical: build the tool,
run it, read the code and the book yourself, and verify every claim — do NOT trust plan checkboxes,
commit messages, or code comments. Cite evidence as `file:line` for code and `chapter/section` for
the book. If you have subagents/parallel tools, use them for breadth; if not, work sequentially.

### archfit's goal (the lens for this review)

archfit aims to be **the** automated tool for applying the book's Balanced Coupling method to a real
codebase, on two legs:

1. **Syntactic / deterministic** — external analysis tools + metrics compute integration strength,
   distance, volatility, and the balance score from code facts (Go `go list`/packages, TS
   dependency-cruiser, Python grimp, Rust cargo/SCIP, ast-grep syntax facts, jscpd clones).
2. **Semantic / LLM** — an LLM understands the code the way a human architect does: classifying
   ambiguous coupling, and **generating/improving configuration by reading design docs, ADRs, and
   code** to infer module boundaries, subdomain roles (core/supporting/generic → volatility), and
   ownership. Deterministic output must stay reproducible; LLM judgments enter only through
   reviewable, pinned config/labels and never silently change a score at gate time.

Judge archfit against BOTH legs: is it a faithful, reproducible fitness function for the book, AND is
the semantic layer real and useful?

### Internalize the model before judging (from the book)

- **Integration strength** (likelihood of cascading change), strongest→weakest: **intrusive >
  functional > model > contract**. Ch7. Ordinals in archfit's Ch10 numeric model.
- **Distance** (cost of cascading change) — socio-technical: code structure + org/ownership +
  runtime coupling. Ch8.
- **Volatility** (probability of change at all) — from **DDD subdomains** (core=high volatility,
  supporting=low, generic=low functional volatility), **not commit history**. Ch9.
- **Balance** (Ch10): `balance = max(|S − D|, 10 − V) + 1`. Modularity = strength XOR distance;
  complexity = both high (tight coupling / distributed monolith) OR both low (low cohesion / big ball
  of mud); low volatility neutralizes unbalanced coupling. Replay the book's Ch10 worked examples
  through archfit's scorer and confirm the numbers match.
- **Connascence** (Ch6) refines degrees within strength levels. **Fractal** — the model applies at
  every abstraction level; the highest distance is the boundary of the level under review.

### Inputs

- **Book (ground truth):** `.book/9780137353576.epub` (preferred — unzip and read the
  `OEBPS/xhtml/ch*.xhtml` chapters as text; cheaper than the PDF `.book/9780137353538.pdf`).
  Chapters that carry the checkable model: Ch6 connascence, Ch7 integration strength, Ch8 distance,
  Ch9 volatility, Ch10 balancing (THE numeric model), Ch11 (refactoring / cheapest move), Ch13
  (Balanced Coupling in practice). Appendices carry numeric scales.
- **Code:** the repo on `fix/wave1-gate-integrity`. Start at `CLAUDE.md` ("Coupling scorer — key
  design facts", "Invariants"), then `internal/model/coupling/` (scorer + ordinals),
  `internal/classify/` (edge strength/distance classification), `internal/score/`,
  `internal/metrics/` (boundary + modularity families), `internal/rules/`, and the LLM/semantic
  surface (`cmd/archfit/enrich*.go`, `config init/update --llm`, `.archfit-labels.yaml` handling,
  `internal/llm`).
- **Prior baseline to diff against (optional but recommended):**
  `reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md` — the pre-fix audit. This branch landed 7 fix
  waves on top of it (bc_score.v4). Report what improved / regressed / is still open.
- **Corpus for empirical runs (`~/workspace/`):** Go — `pumba`, `spotinfo`, `prometheus`; Python —
  `ccgram`, `prefect`; Rust — `yazi`, `herdr`; TS — `storybook`. Plus archfit itself. Use the
  repo's own `.archfit.yaml` where present.

### Method

1. `make build` → `.bin/archfit`; `.bin/archfit doctor` to record available analyzers.
2. Read the book chapters above. Extract every **checkable** claim (formula, ordinal scale, level
   definition, decision rule, methodology step) — skip pure narrative.
3. For each claim, find how archfit implements it (verify in code, file:line). Classify (see Lane A).
4. Run `.bin/archfit analyze --full --json` on ≥1 repo per language (canonical-case `--root`);
   spot-check that reported edge S/D/V and balance match the book formula; check `coupling_balance`
   band, `classified_edges` (scored/abstained/external/same_module), n/a honesty.
5. Evaluate the complementary metrics (Lane B) and the semantic/LLM layer (Lane C).

### Lane A — coupling CORE (must be book-verbatim)

Applies to `coupling_balance`, S/D/V dimensions, the balance formula, and coupling terminology.
Classify each checkable claim: **verbatim** | **adapted** (same intent, different mechanics) |
**deviation** (contradicts the book) | **missing** (checkable, absent) | **out_of_scope**
(documented deliberate omission). A core **deviation is high priority**. Specifically verify:
integration-strength ordinals and their assignment (incl. the DTO→contract and const/var→model
edge cases); distance ladder incl. a top rung for external/vendor integration and the
socio-technical ownership factor; volatility from subdomain role not churn (and the opt-in
volatility cascade); the exact formula; abstain-not-fake (unknown S or D ⇒ edge unscored, no
invented ordinals); `n/a` when unmeasured (never a fabricated mid-band number); fractal/level
handling (same-module vs cross-module).

### Lane B — complementary metrics (NOT judged as book deviations)

archfit may ship metrics outside the book that are well-known and genuinely useful for a Balanced
Coupling design review (e.g. cyclomatic complexity, dependency cycles, blast radius, cohesion/LCOM,
instability/abstractness, code clones, god-struct/large-type, global mutable state, unsafe/panic
density, test density). For each such metric judge, on a DIFFERENT axis:

1. **Non-contamination (load-bearing):** prove it does NOT feed `coupling_balance` or the S/D/V
   ordinals — the coupling score must be identical with the metric on vs off. Leakage into the book
   score is a high-priority finding.
2. **Separation & honesty:** clearly labeled auxiliary/report-only/opt-in; abstain-not-fake; real
   file:line evidence; non-blocking by default.
3. **Relevance & value:** does it add real signal a BC review wants, or is it noise/redundant?
   Verdict per metric: **keep** | **refine** | **drop**. Also note book-relevant signals that are
   MISSING and would strengthen the review.

### Lane C — semantic / LLM layer (the second leg of the goal)

Assess whether the LLM leg is real and sound: (a) does `config enrich` / abstained-edge labeling
raise scored coverage without contaminating determinism (LLM labels reviewable, pinned, provenance
lowers confidence, never override compiler-grade facts)? (b) can archfit generate/improve a config
from design docs / ADRs / code (module boundaries, subdomain-role→volatility, ownership)? How good,
how much human review needed? (c) is the gate always deterministic (LLM runs at enrich time, never
at gate time)? Score maturity and call out gaps.

### Deliverable — a single well-organized report

Write it to `reports/book-alignment-review-<date>/00-REVIEW.md`. Structure:

1. **Verdict** (2–4 sentences): how close is archfit to being the book's tool, on both legs?
2. **Scorecard table** — one row per area, each scored **0–5** (0 absent · 1 wrong · 2 partial ·
   3 adapted-sound · 4 close · 5 book-verbatim/excellent) with a one-line justification + key
   evidence. Rows:
   - Integration Strength (Ch7) · Distance (Ch8) · Volatility (Ch9) · Balance formula & bands (Ch10)
     · Connascence degrees (Ch6) · Abstain-not-fake & n/a honesty · Fractal/level handling ·
     Terminology fidelity · Gate/fitness-function (fast, trustworthy, agent-actionable) ·
     Complementary metrics (Lane B overall) · Semantic/LLM layer (Lane C) · Multi-language parity
     (Go/TS/Py/Rust).
   - Add an **overall book-conformance score** (weighted; core Lanes A dominate) and an **overall
     tool-goal score**.
3. **What's implemented & how good** — per area, verbatim/adapted with evidence; note what improved
   vs the 2026-07-02 baseline.
4. **What's missing or wrong** — deviations + missing items, severity-ranked, each with file:line
   and the book section it violates, and a concrete fix.
5. **Complementary-metrics table** — metric · keep/refine/drop · non-contamination proof · value.
6. **Prioritized recommendations** — P1/P2/P3, each tied to evidence, framed as the next "waves."
7. **Empirical appendix** — per-repo run results (score, bands, scored/abstained edges, n/a, walltime)
   and the Ch10 worked-example replay (book numbers vs archfit numbers).

Verify each non-trivial finding by re-running or re-reading before writing it. Prefer being specific
and evidence-backed over exhaustive.
