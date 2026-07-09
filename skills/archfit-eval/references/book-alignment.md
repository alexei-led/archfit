# Archfit book-alignment review

Use this when the task is to judge how well archfit applies Vlad Khononov's
_Balancing Coupling in Software Design_ as a practical tool.

## Core question

How complete is archfit as a practical, deterministic, honest tool for applying
the book today?

Check whether current archfit:

1. measures book-defined facts deterministically from code,
2. uses reviewable AI/LLM help only off-gate,
3. abstains honestly when evidence is missing,
4. keeps the deterministic gate reproducible,
5. stays usable across supported languages and real repos.

## Ground truth

Use the local book, not summaries:

- `.book/9780137353576.epub`

Read and cite at least:

- Ch6 — connascence and degrees of coupling
- Ch7 — integration strength
- Ch8 — distance
- Ch9 — volatility
- Ch10 — balancing formula and examples
- Ch11 — cheapest move / refactoring guidance
- Ch13 — practical usage
- appendices — scales, numeric values, summary tables

## Self-check commands

Run and record:

```sh
make build
.bin/archfit doctor
make test
.bin/archfit analyze --json --config .archfit.yaml
.bin/archfit check --config .archfit.yaml
```

If a command fails, quote the exact error and treat it as evidence.

## Implementation areas to inspect

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
- AI/config surfaces:
  - `config init --ai-classify`
  - `config update --ai-classify`
  - `config enrich labels`
  - `config enrich abstained`
  - `config enrich owner|subdomain|volatility`
  - `analyze --ai-summary`
  - `explain --ai-summary`

Find exact implementation evidence as `file:line`.

## Required review shape

Produce one self-contained markdown review with these sections.

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

Separate facts that must stay deterministic from facts that need semantic
judgment.

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

## Extra evaluation rules

- Repeat at least one representative repo run with the same temp config and
  compare parsed JSON. If it changes, call that out explicitly.
- Treat temp-config prep failures as product findings. They are part of config UX.
- Do not call a semantic-only issue an archfit miss unless the current config and
  deterministic model should reasonably catch it.
- Separate missing-tool local environment gaps from archfit design gaps.
- Keep one final report. Do not create archived report trees or per-repo report
  directories unless the task explicitly asks for them.

## Evidence rules

- Cite code as `file:line`.
- Cite the book by chapter/section.
- If you claim a metric is non-contaminating, prove it from code or by a
  controlled run.
- If a metric is missing, cite the searched code paths and why they do not
  implement it.
- If a tool is absent on the machine, mark coverage as a local tool gap, not an
  archfit defect.
- Distinguish book-core gaps from useful adjacent metrics.
