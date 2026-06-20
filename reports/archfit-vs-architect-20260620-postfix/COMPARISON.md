# archfit post-fix verification — old vs new (2026-06-20)

Five-repo sweep with the reliability-hardened binary (`archfit-reliability-llm-gate`)
and refreshed per-repo configs. Captures live under `<repo>/` and `negative-case/`.

- **Binary**: `v0.4.1-15-g7dcddf1` + Task 1–10 fixes (coverage/encapsulation/go-extractor
  false-green elimination, opt-in gate, coverage-gaps, role-aware modules, `--root`).
- **Tools present** (`doctor`): go, git, node, scip-go/-python/-typescript, ast-grep,
  lizard, jscpd, gitnexus, uv, python3. Absent on these runs: dependency-cruiser, grimp
  (where the language is off), lizard/jscpd (opt-in, off by default).
- **Determinism**: double-run `check --full --format json` is byte-identical
  (`same_hash=true`) for archfit, codegraph, ccgram.

## Headline: the documented false-greens are gone

| Old defect (study + Context table)                                | New behaviour (this sweep)                                                                         |
| ----------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| unanalysed repo → coverage **100% / strong / high** (defect #1)   | coverage **n/a** when no structural extractor contributes; zero-applicable → n/a, not 100%         |
| `coupling_balance` **90 / strong** on empty edges (defect #2)     | empty/low-base → **50 / low** "coupling unmeasured"; a real 90 reads "no unbalanced among N edges" |
| `analysis_confidence` **100/100** while 3 dims n/a (defect #4)    | coverage n/a drives analysis_confidence to **0 / critical** (negative case)                        |
| `encapsulation` 10/10 strong on a thin/empty repo                 | empty graph / nil graph → **n/a**; compiler-boundary repos → n/a (boundary_integrity 50/low)       |
| `architecture_fitness` counts module-cache `_test.go` (defect #8) | `pkg/mod/**` + `**/testdata/**` skipped; spotinfo/pumba 0/critical is the _correct_ "no signals"   |
| `score --full` rejected (defect #9)                               | parses, rc 0 on all repos                                                                          |
| missing tools invisible                                           | **coverage-gaps** block in md+json with install hints; opt-in `--require-tools` → exit 1           |

No repo reports **strong on thin evidence**: the only low-confidence dimension
(`boundary_integrity`, encapsulation n/a for compiler-enforced Go/TS boundaries) reads
50/100 mixed/low everywhere — never strong. Every `strong` dimension below is a
high/medium-confidence _measured_ value (change_locality, cohesion, coupling on clean
graphs, the analysis_confidence meta when extractors actually ran).

## New scorecards (overall + gate verdict)

| repo      | overall        | gate verdict | why                                                                                             |
| --------- | -------------- | ------------ | ----------------------------------------------------------------------------------------------- |
| archfit   | 66 serviceable | **pass** (0) | self-scan green; boundary_integrity 50/low (encapsulation n/a)                                  |
| spotinfo  | 57 mixed       | **pass** (0) | coupling_balance 20/critical (real unbalanced seams); arch_fitness 0 = no enforcement (correct) |
| pumba     | 49 mixed       | **pass** (0) | same shape as spotinfo; change_locality 96/strong                                               |
| ccgram    | 47 mixed       | **fail** (1) | **3 import cycles** (`no-import-cycles: fail`)                                                  |
| codegraph | 36 poor        | **fail** (1) | **2 import cycles** (`no-import-cycles: fail`)                                                  |

## The four required assertions

1. **No strong-on-thin-evidence** — confirmed. The encapsulation-driven dimension is
   50/low for every Go/TS repo; thin coverage drives analysis_confidence down, not up.
2. **Coverage-gaps surfaced for genuinely-missing tools** — every repo lists its absent
   analyzers with install hints (dependency-cruiser, grimp, lizard, jscpd; go/packages in
   the negative case). Gated `warn` by default, so they inform without failing.
3. **ccgram fails on cycles** — `verdict: fail`, exit 1, `no-import-cycles: 3` (exactly the
   3 cycles the study predicted). codegraph likewise fails on its 2 real cycles.
4. **Exit codes meaningful** — pass→0, policy fail→1 (cycles, or `--require-tools` on a
   missing required tool), tool/config error→3 (distinct). Verified in `_summary.txt`.

## Negative case (`negative-case/`)

Fixed binary on a git repo with **no source files** (`--lang go`, no config):

- `coverage`: **n/a** (no extractor contributed) — was 100%/strong.
- `encapsulation`: **n/a** — was 10/10 strong.
- `coupling_balance`: 50/100 mixed/low ("coupling unmeasured").
- `analysis_confidence`: 0/100 critical ("file extraction coverage: n/a").
- Overall: 48/100 mixed — **no dimension reads strong**.
- coverage-gaps lists **go/packages** + install hint plus the other absent analyzers.
- Exit: **0** in auto mode; **1** with `--require-tools`.

## Config refresh applied (per-repo)

- **spotinfo / pumba**: owners + volatility + `role: composition_root` on the `cmd`
  module; `forbidden_dependency` + `forbidden_layer_direction` promoted to `fail` (stay
  green). pumba `mocks` → `role: generated`.
- **codegraph**: owners + volatility; added `no-import-cycles: fail` (surfaces its real
  cycles) + `forbidden_dependency: fail`.
- **ccgram**: `python_package: ccgram` (already set); `no-import-cycles` and
  `forbidden_dependency` promoted to `fail` → now fails honestly on its 3 cycles.
- **archfit**: unchanged from Task 9 (owners on all 24 modules, layer-direction `fail`).

## Residual false-greens fixed during this verification

The negative-case acceptance run surfaced two false-greens that survived Tasks 1–9; both
fixed here (with tests) because Task 11 verifies "no false-green path remains":

1. **Coverage zero-applicable → 100%** (`internal/metrics/boundary/coverage.go`): a
   record with zero applicable files (e.g. `loc`/`ast-grep` reporting `ok` over a
   no-source repo, or `go/packages` on a non-Go dir) is no longer scored 1.0 — it reads
   n/a. The Go extractor (`internal/extract/golang/golang.go`) now reports `absent` when
   it finds zero Go files instead of `partial`. Supersedes Task 1's
   `zero-applicable → 1.0` sub-decision; real repos (applicable > 0) are unaffected, so
   golden + self-scan stay byte-identical.
2. **Encapsulation on empty/nil graph → 10/10** (`internal/metrics/boundary/encapsulation.go`):
   an empty graph (no dependency edges) or nil graph now reads n/a, matching the package's
   "absent inputs yield n/a" contract. A graph with edges but no cross-boundary coupling
   still reads 1.0 (a genuine analysed result).

## Notes / minor

- An explicitly-disabled tool (e.g. `go` off in ccgram) still appears in coverage-gaps as
  `go/packages` (gated warn). Cosmetic; not a correctness issue. Tracked as possible
  follow-on: skip gaps for tools the config turns off.
- `_summary.txt` `doctor` lines were regenerated: the sweep script passed `-c` to
  `doctor`, which takes no config flag; the per-repo `doctor.txt` files are correct (rc 0).
