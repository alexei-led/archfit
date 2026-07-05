# archfit book-alignment analysis prompt

Use this prompt from the archfit repo root. It is for updating the current book-alignment
analysis, not for creating a new report tree.

## Prompt

You are an expert software architect auditing **archfit** against Vlad Khononov's
**_Balancing Coupling in Software Design_**.

Your job is to update this existing report in place:

- `reports/book-alignment-review-2026-07-05/00-REVIEW.md`

Do **not** create more analysis documents. If you need scratch output, put only raw,
regenerable artifacts under the existing `reports/book-alignment-review-2026-07-05/artifacts/`
directory and cite them from the report. Keep the final output focused in the single report above.

## Core question

How complete is archfit as a practical tool for applying the book?

Answer by extracting every book metric, value, scale, formula, and useful adjacent metric, then
checking one by one whether archfit:

1. measures it deterministically from code facts,
2. can infer or improve it semantically through LLM-assisted review/configuration,
3. abstains honestly when it cannot know,
4. keeps the deterministic gate reproducible.

## Ground truth

Use the local book, not summaries or old reports:

- `.book/9780137353576.epub`

Extract and cite book evidence by chapter/section. Read at least:

- Ch6 — connascence and degrees of coupling
- Ch7 — integration strength
- Ch8 — distance
- Ch9 — volatility
- Ch10 — balancing formula and examples
- Ch11 — refactoring / cheapest move
- Ch13 — practical usage
- appendices — numeric scales, values, or summary tables

Old reports and plans are context only. Do not trust them without re-reading code and the book.

## Current implementation to inspect

Start with:

- `CLAUDE.md`
- `.archfit.yaml`
- `internal/model/coupling/`
- `internal/classify/`
- `internal/score/`
- `internal/metrics/`
- `internal/rules/`
- `internal/extract/`
- `cmd/archfit/`
- `internal/labels/`, `.archfit-labels.yaml` handling if present
- LLM/config surfaces: `config init --llm`, `config update --llm`, `config enrich labels`,
  `config enrich abstained`, `analyze --llm`, `review`, `explain`

Find exact implementation evidence as `file:line`.

## Required analysis shape

Update `00-REVIEW.md` so it contains these focused sections.

### 1. Verdict

Short verdict. Include two scores:

- book-conformance score
- practical-tool score

### 2. Book metric inventory

List every extracted book metric, value, formula, scale, or checkable method one by one.

For each row include:

- book reference
- name
- exact value / scale / formula / rule
- type: core BC metric, adjacent useful metric, method, or semantic judgment
- automation class:
  - deterministic/syntactic
  - semantic/LLM
  - mixed deterministic + LLM
  - not automatable / human-only
- current archfit status:
  - implemented verbatim
  - adapted soundly
  - partial
  - missing
  - out of scope
- archfit evidence (`file:line`) or explicit absence evidence
- gap / next action

Do not collapse distinct values. For example, strength levels, distance levels, volatility levels,
formula terms, connascence degrees, and Ch10 examples should be separate enough to audit.

### 3. Current metric and formula implementation

For each archfit metric/formula, list:

- metric name and output field
- purpose
- formula or scoring rule
- inputs/facts used
- whether it feeds `coupling_balance`, gates, scorecard only, or report-only output
- non-contamination proof for auxiliary metrics
- implementation files and tests
- known limitations

### 4. Per-language extraction matrix

Analyze each language separately: Go, TypeScript/JavaScript, Python, Rust.

For every book/core or adjacent metric, state:

- whether archfit can extract the needed facts for that language
- tool used today
- file paths in `internal/extract/` or related code
- confidence level
- what remains `n/a` or abstained
- whether a new tool would materially help

Prefer existing tools. Consider a new tool only if it provides deterministic, reproducible facts
with machine-readable output and fits CI/cache use. If recommending a new tool, state exactly what
book value it would measure and why existing tools cannot.

### 5. Deterministic vs semantic split

Separate facts that must stay deterministic from facts that need semantic judgment.

Deterministic/syntactic candidates include things like import edges, call/reference edges, public vs
internal access, DTO/data-shape detection, const/var/function use, cycles, fan-in/out, clone pairs,
complexity, and source-location evidence.

Semantic/LLM candidates include things like architectural intent, subdomain role, volatility from
business context, ownership meaning, ambiguous integration strength, module boundary naming, runtime
or lifecycle coupling meaning, and config quality.

The LLM may read:

- architecture docs
- ADR/ARD documents
- design docs
- README and guide docs
- comments and code as text
- existing config and labels

The LLM may propose config, labels, evidence packs, and explanations. It must not silently change a
gate result at analysis time. Any semantic result that affects scoring must become reviewable,
pinned, deterministic input first.

### 6. Gap list and next waves

Produce a prioritized gap list. Each gap must include:

- severity: P1/P2/P3
- book reference
- current archfit evidence
- deterministic extraction plan, semantic/LLM plan, or both
- expected output/config change
- validation command or test shape

## Required commands

Run and record:

```sh
make build
.bin/archfit doctor
make test
.bin/archfit analyze --full --json --config .archfit.yaml > reports/book-alignment-review-2026-07-05/artifacts/archfit-current.json
```

If time permits, run one representative repo per language from the existing corpus and update only
the empirical appendix in `00-REVIEW.md`. Do not create per-language report docs.

## Evidence rules

- Cite code as `file:line`.
- Cite the book by chapter/section.
- If you claim a metric is non-contaminating, prove it from code or by a controlled run.
- If a metric is missing, cite the searched code paths and why they do not implement it.
- If a tool is absent on the machine, mark coverage as a local tool gap, not an archfit defect.
- Distinguish book-core gaps from useful adjacent metrics.

## Output contract

Modify only:

- `reports/book-alignment-review-2026-07-05/00-REVIEW.md`
- optional regenerated raw artifacts under `reports/book-alignment-review-2026-07-05/artifacts/`

Do not create new report directories, new analysis markdown files, or broad research notes.

End with a short validation section listing commands run, pass/fail status, and any unverified gaps.
