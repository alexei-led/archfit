# archfit book-alignment analysis prompt

Use this prompt from the archfit repo root.

It is self-contained. Do not rely on old reports, old verdicts, or archived artifacts.
Treat any prior review as stale unless you re-derive the claim from the book and the current code.

## Prompt

You are an expert software architect auditing **archfit** against Vlad Khononov's
**_Balancing Coupling in Software Design_**.

Your job is to produce **one self-contained markdown review** of the current repo.

Do **not** assume any previous `00-REVIEW.md`, archived report tree, or old analysis note is correct.
Do **not** update or depend on old report files.
Do **not** create a report directory.
If you need scratch data, use temporary local files outside the repo or ephemeral command output.

## Core question

How complete is archfit as a practical tool for applying the book today?

Answer by extracting the book's metrics, values, scales, formulas, and useful adjacent
methods, then checking one by one whether current archfit:

1. measures it deterministically from code facts,
2. can infer or improve it through reviewable LLM-assisted configuration,
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

## Current implementation to inspect

Start with:

- `README.md`
- `CLAUDE.md`
- `.archfit.yaml`
- `cmd/archfit/`
- `internal/model/coupling/`
- `internal/classify/`
- `internal/score/`
- `internal/metrics/`
- `internal/rules/`
- `internal/extract/`
- `internal/labels/`
- `.archfit-labels.yaml` handling if present
- LLM/config surfaces:
  - `config init --llm`
  - `config update --llm`
  - `config enrich labels`
  - `config enrich abstained`
  - `config enrich owner|subdomain|volatility`
  - `analyze --llm`
  - `explain`

Find exact implementation evidence as `file:line`.

## Required analysis shape

Produce one markdown review with these sections.

### 1. Verdict

Short verdict. Include two scores:

- book-conformance score
- practical-tool score

### 2. Book metric inventory

List every extracted book metric, value, formula, scale, or checkable method.

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

Do not collapse distinct values.

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

Analyze Go, TypeScript/JavaScript, Python, and Rust separately.

For each core or adjacent metric, state:

- whether archfit can extract the needed facts for that language
- tool used today
- implementation path in `internal/extract/` or related code
- confidence level
- what remains `n/a` or abstained
- whether a new tool would materially help

Prefer existing tools. Recommend a new tool only when it adds deterministic,
reproducible, machine-readable evidence the current stack cannot provide.

### 5. Deterministic vs semantic split

Separate facts that must stay deterministic from facts that need semantic judgment.

Any semantic result that affects scoring must become reviewable, pinned,
deterministic input first.

### 6. Gap list and next waves

Each gap must include:

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
.bin/archfit analyze --full --json --config .archfit.yaml
.bin/archfit analyze --gate --full --config .archfit.yaml
```

If time permits, run one representative repo per language from the existing corpus.
Keep any extra findings inside the same final review.
Do not create per-language report docs.

## Evidence rules

- Cite code as `file:line`.
- Cite the book by chapter/section.
- If you claim a metric is non-contaminating, prove it from code or by a controlled run.
- If a metric is missing, cite the searched code paths and why they do not implement it.
- If a tool is absent on the machine, mark coverage as a local tool gap, not an archfit defect.
- Distinguish book-core gaps from useful adjacent metrics.

## Output contract

Return one self-contained markdown review in your final response.

Do not depend on old report paths.
Do not write under `docs/archived/reports/`.
Do not create new analysis directories.

End with a short validation section listing commands run, pass/fail status, and any unverified gaps.
