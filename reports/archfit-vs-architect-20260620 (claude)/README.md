# archfit (deterministic + LLM) vs the Architect skill

## A Balanced-Coupling design-review comparison across 5 repositories

**Date:** 2026-06-20 · **archfit:** v0.4.0-2-g8243a08 · **LLM:** anthropic/claude-opus-4-8 (off-gate)
**Author:** Claude (Opus 4.8), multi-agent run · **Methodology anchor:** Vlad Khononov, _Balancing Coupling in Software Design_

---

## TL;DR (one-paragraph summary)

> Two tools, two different jobs. **archfit** is the more _repeatable, automatable, ready-for-automated-checks_ tool, and it fits Balanced Coupling's goal of **measuring** the design. The **architect skill** is the more _trustworthy independent reviewer_ of whether the design is actually sound, and it fits Balanced Coupling's **intent** (judging meaning, business change-rate, and design quality). Neither is enough on its own. The best review combines them: **archfit produces the hard facts → an independent AI reviewer judges them → a human confirms how likely each part is to change → archfit re-measures and enforces the rules automatically.**

**Which is better / more reliable?** archfit wins on _repeatability, automation, completeness of evidence, machine-readable output, and fitness for automated checks_. It is **not** automatically "more reliable" in the sense of _trustworthy judgement_: a tool that shows a healthy score when it did not actually have the evidence to judge is giving false reassurance (see "false-green" in the key below). The architect skill wins on _judging change-rate, working even when archfit is misconfigured, and giving steady overall scores_ — but it cannot be reproduced exactly and is slow.

**Most aligned with Balanced Coupling?** Split: the architect skill is closer to the _intent_ of the method, archfit is closer to its _measurement form_. archfit only pulls level once two things exist: (1) it refuses to score where it lacks evidence, and (2) each module is labelled with its business change-rate.

**Can they be combined?** Yes — that is the main recommendation (section 8.3).

---

## How to read this report (plain-language key)

New to these terms? Read this first. Everything in the report is defined here.

### Score notation

- **`62/100 (serviceable)`** — a number from 0–100 plus a one-word **band**. archfit bands, worst to best: **critical → poor → mixed → serviceable → strong**. The architect skill uses a 0–10 scale with bands **Poor → Adequate → Good → Excellent**.
- **Table shorthand** like `60 mixed/lo` means: score **60**, band **mixed**, confidence **low**. Confidence: `lo` = low, `med` = medium, `hi` = high. Band shorthand: `serv` = serviceable, `crit` = critical.
- **`file:line`** (e.g. `main.go:159`) — a place in the code: the file name, then the line number.
- **`1H/60M`** — counts of findings by severity: 1 High + 60 Medium.
- **`I>0.7`** — an "instability" measure above 0.7: a module that depends on many others and is easily affected by their changes.

### Tools and systems named

- **archfit** — the tool being reviewed; a command-line program that checks a codebase against architecture rules and prints a scorecard.
- **architect skill** — an AI agent (an LLM) that reviews architecture by reading the code itself and writing up findings.
- **LLM** — Large Language Model (an AI such as Claude or GPT).
- **scip** — a precise code indexer (compiler-grade). It records exactly which code symbol uses which other symbol. archfit needs it to classify how modules are connected.
- **gitnexus** — a tool that builds a graph from git history: which files tend to change together over time.
- **lizard** — measures code complexity and the size of functions/files.
- **jscpd** — detects copy-pasted / duplicated code (called "clones").
- **dependency-cruiser, ast-grep, `go list`** — tools that extract the import/dependency graph from source code.
- **SARIF** — a standard JSON format for reporting code-analysis findings; understood by CI systems and editors.
- **`agent_tasks`** — machine-readable repair instructions that archfit emits for AI coding agents to act on.
- **CI** — Continuous Integration: automated checks that run on every code change.
- **gate** — an automated check that makes a build pass or fail.

### Balanced Coupling concepts (Vlad Khononov's method)

- **Coupling** — when two parts of a system depend on each other. Not inherently bad; the point is to keep it _balanced_.
- **Integration strength** — how much one part must "know" about another. From strongest to weakest: **intrusive → functional → model → contract**. Higher strength means a change in one part is more likely to force a change in the other.
- **Distance** — how far apart two parts are: same function → same file → different package → different service. Greater distance makes changing both together more expensive.
- **Volatility** — how likely a part is to change at all. Khononov says to judge this from the part's **business role** (its subdomain), **not** from how often it changed in git history.
- **Subdomain** — the business role of a component: **core** (your competitive advantage; changes often), **supporting** (needed but not special; changes rarely), **generic** (a solved problem you could buy off the shelf).
- **Balance rule** — coupling is healthy when strength and distance are _opposite_ (one high, one low), **or** when the part rarely changes.
- **Cohesion** — how well the things inside one module belong together. High cohesion is good.
- **god file / god module** — a single file or module that is far too large and does too many unrelated things.
- **import cycle** — module A depends on B and B depends on A (a loop). It makes both hard to change independently.
- **layer inversion** — a lower layer (e.g. the database code) wrongly depends on a higher layer (e.g. business logic). Dependencies should point one way.
- **distributed monolith** — a system split into separate services that are still so tightly coupled they must be changed and deployed together: the worst of both worlds.
- **fitness function** — an automated, repeatable check that confirms an architecture rule still holds (for example: "no inner layer may import an outer layer"). archfit _is_ this kind of check.
- **hexagonal / ports-and-adapters** — a design style where the core logic talks to the outside world only through defined interfaces called "ports".

### Words used to describe scoring behaviour

- **harsher / stricter / more critical** — gives a **lower (worse)** score.
- **lenient / more forgiving** — gives a **higher (better)** score.
- **false-green** — a score that _looks_ healthy ("green") but is wrong, because the tool did not actually have the evidence to make that judgement. It gives false reassurance.
- **coverage** — how much of the code the tools actually analysed. Low coverage means the tool "could not see" much.
- **deterministic** — the same input always produces the same output (fully repeatable).
- **off-gate** — the LLM features run on the side and never decide whether the automated check passes or fails.
- **version delta** — the change in scores between an older version and the latest version.
- **under-tooled vs full-tools** — run with some analysis tools switched off, versus run with all of them switched on.

---

## Contents

1. [What this is and how it was done](#1-what-this-is-and-how-it-was-done)
2. [The two approaches](#2-the-two-approaches)
3. [Scoring method (Balanced Coupling + 8 criteria)](#3-scoring-method)
4. [Results](#4-results)
5. [The fairness experiment (before and after enabling all tools)](#5-the-fairness-experiment)
6. [Scored criteria comparison](#6-scored-criteria-comparison)
7. [Findings](#7-findings)
8. [Recommendations](#8-recommendations) — improve archfit · improve the architect skill · combine both
9. [Direct answers](#9-direct-answers)
10. [Limitations](#10-limitations)
11. [Evidence index](#11-evidence-index)
12. [Appendix: independent second opinions](#12-appendix-independent-second-opinions)

---

## 1. What this is and how it was done

Goal: compare two ways of producing an architecture/design review — **archfit with its LLM layer** versus the **architect skill** — on the same real codebases; judge which is better and which fits Balanced Coupling more closely; and decide whether to combine them.

**Repositories reviewed** (under `~/Workspace`, chosen to cover several languages):

| repo      | language   | size      | what it does                    |
| --------- | ---------- | --------- | ------------------------------- |
| pumba     | Go         | 159 files | chaos-testing CLI for Docker    |
| spotinfo  | Go         | 22 files  | AWS spot-price CLI + MCP server |
| ccgram    | Python     | 382 files | larger application              |
| codegraph | TypeScript | 196 files | code-graph indexer (uses SCIP)  |
| archfit   | Go (self)  | 214 files | the tool being tested itself    |

**Procedure:**

1. **archfit side** — ran `scan` (full report), `score` (the 7-dimension scorecard), `score --base=<old-version>` (version delta), `check --format json` (machine-readable output), and `review` (the off-gate LLM write-up) on all 5 repos.
2. **architect side** — five independent `architecture:architect` AI agents, each working **without seeing** archfit's output, each gathering its own evidence and producing a cited review plus a scorecard.
3. **Research** — read Khononov's book and blog, the fitness-functions literature (Ford / Parsons), and 2023–2026 studies comparing rule-based tools with LLMs.
4. **Stress test** — an independent **Codex** review (a different AI model) and an adversarial **Claude advisor** attacked the conclusion to check for bias toward archfit.

> ### Fairness correction (important)
>
> The first archfit run used whatever each repo's `.archfit.yaml` happened to enable. For the Go and TypeScript repos, **`scip` was switched off** (so archfit could not classify how modules connect), and `gitnexus` / `lizard` / `jscpd` were not installed — an unfair handicap. The **entire archfit analysis was therefore re-run with every tool switched on and every index rebuilt**. **All headline numbers in this report come from that full-tools run.** The difference between the two runs is itself a finding (section 5).

---

## 2. The two approaches

|                           | **A — archfit (rule-based tool + LLM)**                                                                                                                                                                     | **B — architect skill (AI reviewer)**                       |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| What it is                | A command-line program that runs external analysis tools                                                                                                                                                    | An AI agent that gathers its own evidence                   |
| Evidence used             | `go list` / scip / dependency-cruiser / ast-grep / gitnexus / lizard / jscpd → a classified dependency graph, metrics, and change history                                                                   | Text search (`grep`), language tools, git log, reading code |
| Output                    | A 7-dimension scorecard + findings + **SARIF** + **`agent_tasks`** + a markdown report; plus **version deltas**                                                                                             | A written, cited review + a scorecard + a plan              |
| Role of the LLM           | **Off-gate**: `review` re-states archfit's own measurements in plain Balanced-Coupling language; `enrich --subdomains` proposes change-rate labels for a human to confirm. The LLM never decides pass/fail. | The LLM **is** the whole analysis                           |
| Repeatable?               | **Yes** — same input gives the same output                                                                                                                                                                  | No — results vary between runs                              |
| Is it a fitness function? | **Yes** — it can pass or fail an automated check                                                                                                                                                            | No — it only _recommends_ writing such checks               |

**The key difference:** archfit _measures first, then (optionally) describes its own measurements_; the architect skill _investigates first, then judges_. archfit's `review` does **not** gather new evidence — it re-words archfit's own numbers. This single difference explains every disagreement in section 4.

---

## 3. Scoring method

Balanced Coupling (Khononov) says every dependency between two parts has three properties:

- **Integration strength** — how much they must know about each other (intrusive → functional → model → contract). This drives the _chance_ that changing one forces changing the other.
- **Distance** — how far apart they are (same method → same module → different service). This drives the _cost_ of changing both together.
- **Volatility** — how likely the part is to change at all, judged from its **business role**, **not** from git commit counts.

Balance rule (using `XOR` = "exactly one of the two is true"): a dependency is healthy when strength and distance are opposite — `MODULARITY = STRENGTH XOR DISTANCE` — or, more practically, `BALANCE = (STRENGTH XOR DISTANCE) OR (the part rarely changes)`. The highest risk is when **all three are high at once**: strong, distant, and frequently changing.

Both tools were scored on 8 criteria: (1) faithfulness to the three dimensions, (2) handling of change-rate (volatility), (3) repeatability, (4) evidence quality, (5) coverage, (6) score calibration, (7) actionability, (8) fitness for automated checks.

---

## 4. Results

### 4.1 archfit scorecard — full tools (0–100, with band/confidence)

| repo      | **overall**  | boundary_integrity | coupling_balance | dependency_graph_health          | cohesion_modularity | change_locality | architecture_fitness | findings    |
| --------- | ------------ | ------------------ | ---------------- | -------------------------------- | ------------------- | --------------- | -------------------- | ----------- |
| pumba     | **51 mixed** | 60 mixed/lo        | 50 mixed/hi      | 69 serv/hi                       | 33 poor/hi          | 96 strong/hi    | 0 crit/hi            | 2 med       |
| spotinfo  | **69 serv**  | 60 mixed/lo        | 90 strong/med    | 77 serv/hi                       | 95 strong/hi        | 92 strong/hi    | 0 crit/hi            | 0           |
| ccgram    | **43 mixed** | 33 poor/lo         | 50 mixed/hi      | 18 crit/hi                       | 11 crit/hi          | 80 serv/hi      | 67 serv/hi           | 61 (1H/60M) |
| codegraph | **50 mixed** | 60 mixed/lo        | 90 strong/med    | 43 mixed/hi _(import cycles: 2)_ | 5 crit/hi           | 70 serv/hi      | 33 poor/hi           | 0           |
| archfit   | **64 serv**  | 60 mixed/lo        | 90 strong/med    | 63 serv/hi                       | 9 crit/hi           | 94 strong/hi    | 67 serv/hi           | 0           |

Reading the cells: `60 mixed/lo` = score 60, band "mixed", confidence "low" (`serv` = serviceable, `crit` = critical). The 7 dimensions are: boundary integrity (are module boundaries respected), coupling balance, dependency-graph health (cycles, hubs), cohesion/modularity (god files, duplication), change locality (do recent changes stay inside boundaries), and architecture fitness (are the rules actually enforced by automated checks). All five repos ran with `scip + gitnexus + lizard + jscpd` present (confidence 100). Source: `evidence/archfit-scorecards-fulltools.txt`.

### 4.2 architect skill — overall band + main findings (with `file:line`)

| repo      | overall          | main findings                                                                                                                                                                                                                                                                                                              |
| --------- | ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| pumba     | **Good ~6.0/10** | test "mock" files placed in the production package `pkg/container/mock_Client.go`; `pkg/runtime/podman/client.go:10,43` depends on its sibling `docker`; an oversized `container.Client` interface passed via `pkg/chaos/cmd/builder.go:23`; the rule check at `.archfit.yaml:13` only warns, never fails                  |
| spotinfo  | **Adequate 5–6** | the `spotClient` interface is defined twice (`main.go:159` + `mcp/server.go:26`); a **silent fallback to fake data** in production at `score.go:75`; a 712-line god file `main.go`                                                                                                                                         |
| ccgram    | **Good 5.9/10**  | god modules `hook.py` (1234 lines), `directory_callbacks` (1076); an A↔B loop between `bot` and `main`; `providers→tmux_manager` is unbalanced; the most-changed code path depends on a single global object                                                                                                               |
| codegraph | **Poor 4.3/10**  | a strong **two-way loop between `extraction` and `resolution`** (`src/extraction/index.ts:24` ↔ `resolution/frameworks/drupal.ts:50`); the database file `db/queries.ts:21` wrongly depends on business logic (layer inversion); a concrete `QueryBuilder` injected with no interface; a 3375-line god file `mcp/tools.ts` |
| archfit   | **Good 8.0/10**  | the `Runner` interface sits in `toolrun` instead of the shared `ports` package; `config` does two jobs (parses YAML and touches the OS) inside the inner "core" ring; a flat 5174-line `cmd/archfit`; **and it confirmed archfit's own rule-check is genuine** (it traverses the real import graph)                        |

Source: `evidence/architect-reviews/`.

### 4.3 Side-by-side overall scores — and why they do not closely match

To compare, both are put on a 0–100 scale (the architect's 0–10 score ×10). "Gap" is archfit minus architect; a negative gap means **archfit gave the lower (stricter) score**.

| repo      | archfit /100 | architect /100 | gap                                   |
| --------- | ------------ | -------------- | ------------------------------------- |
| pumba     | 51           | 60             | −9 (archfit stricter / lower score)   |
| spotinfo  | 69           | 55             | +14 (archfit more forgiving / higher) |
| ccgram    | 43           | 59             | −16 (archfit stricter / lower score)  |
| codegraph | 50           | 43             | +7 (archfit more forgiving / higher)  |
| archfit   | 64           | 80             | −16 (archfit stricter / lower score)  |

**What this means:** the two tools give **similar averages (about 55 vs 59) but they disagree on individual repos.** With all tools enabled, archfit tends to give **lower (stricter)** scores — its cohesion score drops to "critical" wherever `jscpd`/`lizard` find duplicated code or oversized files, and its "architecture fitness" score gives a flat 0 to any rule that only warns instead of failing. On a small, clean repo (spotinfo) archfit is **more forgiving** (coupling 90). The architect's scores are steadier and more holistic. **They are two different lenses on the same code, not the same measurement** — which is exactly why combining them adds value instead of repeating the same answer.

### 4.4 Version delta (a capability only archfit has)

`score --base=<old-version>` produces a repeatable quality trend across releases that the architect skill cannot produce on its own:

| repo               | overall: old → latest | change_locality: old → latest |
| ------------------ | --------------------- | ----------------------------- |
| pumba (1.1.3)      | 51 → 51 (no change)   | 96 → 96 (stable)              |
| spotinfo (2.0.1)   | 66 → 69               | 72 → 92 (improved)            |
| ccgram (v3.3.3)    | 35 → 43               | 30 → 80 (large improvement)   |
| codegraph (v0.9.4) | 42 → 50               | 20 → 70 (large improvement)   |
| archfit (v0.2.0)   | 56 → 64               | 44 → 94 (large improvement)   |

This is a genuine, repeatable trend you can plot in CI — a clear advantage for archfit.

### 4.5 Where the two tools disagree (the most informative part)

- **codegraph — the clearest test case.** The architect rated it Poor (4.3) and named the two-way `extraction`↔`resolution` loop as the number-one problem. archfit (full tools) gave overall **50/mixed**, correctly flagged the god file (`cohesion 5/critical` ✅) and **did detect the loop** (`dependency_graph_health 43` says _"import cycles: 2"_ ✅) — **but** `coupling_balance` still shows **90/strong** ❌ and `findings=0`. So archfit _sees_ the loop (under "dependency-graph health") yet the dimension literally named "coupling" does not reflect it, and no separate finding is produced.
- **pumba — the false-green was caused by a missing tool, and got fixed.** When `scip` was off, `coupling_balance` was a false **90**. With `scip` on, it dropped to **50/mixed** and 2 findings appeared. A real fix from real coverage.
- **spotinfo — archfit is more forgiving, possibly correctly.** `coupling_balance 90`, `cohesion 95`, even with all tools on, while the architect found a duplicated interface, a silent fallback to fake data, and a 712-line god file. spotinfo is small and clean, so there genuinely are very few unbalanced dependencies. archfit's 90 may be defensible — but it cannot tell the difference between "truly balanced" and "I found nothing", and its god-module rule (which measures whole packages) misses a single oversized _file_.
- **ccgram — same coverage, archfit stricter.** Both tools find the god modules and the loops. archfit's `cohesion 11/critical` versus the architect's `modularity 6/Good` — archfit punishes the bad parts harder, while the architect gives credit for the large clean majority.
- **archfit (reviewing itself) — archfit under-rates itself.** archfit gave itself 64/serviceable with `cohesion 9/critical` (driven by jscpd duplication detection). The architect gave 8.0/Good with `cohesion 8`, after _confirming archfit's own rule-check genuinely works_. The mechanical metric and the human-style judgement disagree sharply.

---

## 5. The fairness experiment

This compares archfit run **under-tooled** (some tools off, the first run) against **full tools** (all on, the corrected run).

| repo      | archfit overall — under-tooled | archfit overall — full tools | what changed when tools were enabled                                           |
| --------- | ------------------------------ | ---------------------------- | ------------------------------------------------------------------------------ |
| pumba     | 62                             | **51**                       | `coupling_balance` 90 → **50** (false-green fixed by `scip`); cohesion 58 → 33 |
| spotinfo  | 70                             | **69**                       | coupling and cohesion stayed high (a metric limit, not a coverage gap)         |
| ccgram    | 47                             | **43**                       | cohesion 36 → **11 critical**; confidence 90 → 100                             |
| codegraph | 54                             | **50**                       | cohesion 30 → **5 critical** (god file detected); loops surfaced               |
| archfit   | 68                             | **64**                       | cohesion 34 → **9 critical** (jscpd duplication)                               |

**What this proves:**

- archfit's accuracy **depends on which tools are switched on**. Turning on `scip` fixed pumba's false-green; turning on `lizard`/`jscpd` correctly dropped cohesion to "critical" wherever real god files or duplication exist. **Most of the earlier "archfit is blind" disagreements were caused by configuration, and are now resolved.**
- One problem did **not** go away with tooling: on codegraph, `coupling_balance` stayed at 90 even though there is a real loop. That leads to the one precise criticism that remains →

> **archfit's headline scores do not drop when the evidence behind them is thin, and they do not reflect a problem that is counted under a different dimension.** On data with no classified dependencies, `boundary_integrity` honestly reports `encapsulation: n/a (no classified edges)`, but on the _same_ data `coupling_balance` reports `90 strong`. Coverage, and problems counted elsewhere, must lower the score (sections 7 and 8.1).

---

## 6. Scored criteria comparison

| #   | Criterion (plain meaning)               | archfit                                                                                                               | architect                                                                                   | Winner                                                     |
| --- | --------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| 1   | Faithful to the 3 dimensions            | Measures all 3 as metrics; encodes the balance rule                                                                   | Judges all 3 per dependency; catches couplings the metrics do not model                     | roughly tied (measurement vs judgement)                    |
| 2   | Handling of change-rate (volatility)    | Uses human-set labels (correct) **only if configured**; otherwise leans on git history (which Khononov warns against) | Infers change-rate from business role, by reasoning                                         | **architect**                                              |
| 3   | Repeatability                           | Fully repeatable                                                                                                      | Varies between runs                                                                         | **archfit**                                                |
| 4   | Evidence quality                        | Complete, compiler-grade dependency graph; exact `file:line`; cannot hallucinate                                      | Exact `file:line` but only sampled; can miss things or invent them                          | **archfit** (when tools are on)                            |
| 5   | Coverage                                | Whole graph, many languages — **when tools are enabled**; silent when not                                             | Works regardless of configuration; investigates until it finds something; states its limits | **architect** for robustness; **archfit** for completeness |
| 6   | Score calibration                       | Overall band is sound; individual dimensions are unstable; **score does not drop when evidence is missing**           | Holistic and steady per dimension; states confidence; not repeatable                        | **architect** today; archfit after the fix in 8.1          |
| 7   | Actionability (can a machine act on it) | SARIF + `agent_tasks` + version deltas → consumable by tools and AI agents                                            | A written report and a plan                                                                 | **archfit**                                                |
| 8   | Fitness for automated checks            | Fast, repeatable, can pass/fail a build; _is_ a fitness function                                                      | Slow, costly, not repeatable, can be gamed                                                  | **archfit**                                                |

**Weighting matters more than the count.** For an **automated check or AI agent loop**, archfit's wins (3, 4, 7, 8) are decisive. For a **one-off, trustworthy design judgement**, the architect's wins (2, 5, 6) are decisive.

---

## 7. Findings

**F1 — `coupling_balance` has a name broader than what it measures.** It only counts "unbalanced dependencies that scip could classify". It needs `scip` to mean anything (pumba 90 → 50 once `scip` was on); it does not include import cycles or layer inversions (codegraph's loop is counted under `dependency_graph_health`, while coupling stays 90); and when it is green you cannot tell "truly balanced" from "I could not see". _Evidence: codegraph and pumba scorecards._

**F2 — the score does not drop when the evidence is missing (a design defect).** On data with no classified dependencies, `boundary_integrity` correctly says `encapsulation: n/a`, but `coupling_balance` says `90 strong`. A green score with no evidence behind it is false reassurance. _Confirmed by the independent Codex review and by section 5._

**F3 — individual dimensions are unstable; the overall band is reliable.** Cohesion swings between 5 and 95 depending on whether `jscpd`/`lizard` are on and how "god module" is defined; coupling swings between 50 and 90 depending on coverage. The overall band stays sensible and roughly tracks the architect. Always read the **band plus the dimensions together**, never one dimension alone.

**F4 — both tools agree that rule-enforcement is the weakest area everywhere.** `architecture_fitness` is 0 (pumba, spotinfo), 33 (codegraph), 67 (ccgram, archfit). The architect independently flagged warn-only checks or missing architecture tests on every repo. archfit-self is the only repo with a real, verified rule-check (its import-ring test).

**F5 — with equal coverage, the two tools agree on substance.** On ccgram (both fully tooled) both find the same god modules and loops; archfit is more detailed (61 separate findings), the architect more contextual. They measure the same reality; only the format differs.

**F6 — archfit's LLM `review` fails on repos with many findings.** ccgram and full-tools codegraph both returned _"model response is not the required JSON: unexpected end of JSON input"_ — the evidence sent to the model was too large. archfit's LLM write-up could only be assessed on 3 of the 5 repos.

**F7 — the architect catches meaning-level problems the metrics do not model**, e.g. spotinfo's silent fallback to fake data (`score.go:75`, a correctness risk) and the duplicated interface — none of which count as "unbalanced coupling" to archfit. In the other direction, archfit catches every cycle, hub, and instability completely, where the architect only samples.

**F8 — archfit's version delta is a unique, repeatable capability** (section 4.4): it tracks how scores change across releases, and that trend can itself be an automated check. The architect skill has no equivalent.

---

## 8. Recommendations

### 8.1 How to improve **archfit**

1. **Make missing evidence lower the score (most important).** When archfit lacks the data to judge `coupling_balance` or `cohesion_modularity`, it should report `n/a` / low confidence instead of a green score — exactly as it already does for `encapsulation`. Until then, comparing scores across repos is unsafe. _(Fixes F2.)_
2. **Either count cycles under coupling, or rename the dimension.** Fold import-cycle and layer-inversion signals into `coupling_balance`, or rename it to something like "classified-dependency balance", so a 90 next to "import cycles: 2" stops misleading readers. _(Fixes F1.)_
3. **Emit a separate finding for graph-level problems.** codegraph showed `findings=0` while two cycles existed — those cycles should appear as findings and `agent_tasks` so tools and agents can act on them.
4. **Calibrate cohesion.** Turning on `jscpd`/`lizard` pushes cohesion to "critical" (archfit-self 9 vs architect 8). Tune the thresholds and weight problems by how much of the module they affect, so a few oversized files do not drag down an otherwise-clean module.
5. **Split the `review` evidence into smaller pieces.** Summarise or chunk the evidence before sending it to the model so repos with many findings do not overflow. _(Fixes F6.)_
6. **Make change-rate labels part of setup.** Most repos leave `subdomain`/`volatility` unset, so the change-rate dimension falls back to defaults. Build `enrich --subdomains --pin` into the initial setup, and lower confidence while labels are missing. Prefer the human-set labels over git-history signals (per Khononov). _(Improves criterion 2.)_

### 8.2 How to improve the **architect skill**

1. **Reduce score variation.** A single AI run cannot be reproduced (it rated archfit 8.0, possibly too generously). Use a fixed scoring rubric, average several runs, or add a self-check that argues the opposite before settling on a score.
2. **Build on archfit's hard facts instead of tracing by hand.** The codegraph reviewer traced imports manually with text search and admitted it skipped a full dependency-cruiser pass. Giving it archfit's complete graph (cycles, hubs, instability) removes that blind spot while keeping its meaning-level judgement.
3. **Produce machine-readable output too.** Add a structured findings block (in SARIF or `agent_tasks` shape) alongside the prose, so the review can be consumed by CI and AI agents, not only read by people.
4. **Add a version-delta mode.** Compare two runs (or consume archfit's `--base` delta) so it can describe a _trend_, not only the current state.
5. **Export its change-rate judgement per module**, so its (strong) volatility reasoning can be **fed back into archfit** as confirmed labels — turning a one-off opinion into a durable, enforceable setting.

### 8.3 How to **combine both** (the highest-value setup)

The 2023–2026 consensus is a hybrid: **a rule-based tool for the complete, exact graph + an LLM for meaning-level judgement.** archfit already provides about 80% of this; the missing piece is an _independent_ reviewer, not a model that re-words archfit's own numbers.

```text
1. archfit (rule-based core)  → classified dependencies, metrics, cycles, change history,
                                a coverage report, agent_tasks, version deltas        [HARD FACTS]
2. independent AI review      → reads the facts AND the code; adds meaning-level judgement;
   (architect-style)            catches what the metrics do not model;
                                says where coverage is too thin to trust              [JUDGEMENT]
3. human confirms change-rate → archfit `enrich --subdomains` proposes core/supporting/
                                generic; a person approves them → `--pin`             [BUSINESS TRUTH]
4. archfit re-measures        → now with correct change-rate + evidence-gated scores  [CALIBRATED MEASURE]
5. archfit `check` in CI      → an automated check fails the build if the rules drift [ENFORCEMENT]
```

- Step 2 is the architect skill's strength — but it should _use_ archfit's facts as a starting point while still gathering its **own** evidence. That independence is what catches archfit's blind spots (the true severity of codegraph's loop; spotinfo's silent fallback).
- **What to avoid (raised by Codex):** feeding archfit's own `review` (a model re-wording archfit's numbers) into the architect (another model) just passes one model's opinion through another and adds nothing new. The value comes from **hard facts + an independent reviewer + human change-rate labels + an automated check** — not from chaining two models together.
- `enrich --subdomains` is the natural connector: the model proposes the change-rate labels that archfit cannot derive on its own (and that Khononov says must come from the business); a human approves them; archfit then scores correctly. The right division of labour is: **the tool measures, the model proposes, the human decides, the automated check enforces.**

---

## 9. Direct answers

- **Which is better?** Neither in every case. For **automated / repeatable / AI-agent** review → archfit. For a **one-off, trustworthy design judgement** → the architect skill. Best of all → both together (section 8.3).
- **Which is more reliable?** archfit is more **repeatable and automatable**. It is **not** more _trustworthy_ until missing evidence lowers its score — today it can show a green score on thin or wrongly-counted evidence. The architect skill cannot be reproduced exactly, but it does not produce false-greens.
- **Which is more aligned with Balanced Coupling?** Split. The **architect** is closer to the _intent_ of the method (judging meaning, distance, and business change-rate). **archfit** is closer to the _measurement form_, and it _is_ a fitness function — and it draws level once it refuses to score without evidence and once change-rate labels are set.
- **Can they be combined for more value?** Yes — that is the main recommendation (8.3), and archfit already provides the connector (`enrich --subdomains`).

---

## 10. Limitations

- **archfit-self is the only fully-labelled config** — all 24 of its modules carry human-confirmed `subdomain`/`volatility` labels, so its change-rate dimension used the intended path. The other four repos' `.archfit.yaml` files lack those labels, so their change-rate fell back to defaults; labelling them could raise their scores.
- **Tooling honesty note:** during the batched full-tools run, archfit's gitnexus index _refresh_ timed out (the machine was busy running 5 AI agents at once) and that run fell back to the existing index. It was rebuilt afterwards in isolation (finished cleanly in seconds), and archfit-self was re-scored on the fresh index — **the result was identical (64/serviceable)**, so the reported numbers stand. All five repos scored with `scip + gitnexus + lizard + jscpd` present (confidence 100).
- The architect side is a single AI run per repo — not reproducible; a second run could shift the scores.
- The two tools use different dimension names and scales (archfit 0–100 / 7 dimensions; architect ~0–10 / 7 dimensions); comparing overall bands is safe, but matching individual dimensions one-to-one is approximate.
- archfit's LLM `review` failed on 2 repos (F6), so that feature was judged on 3 of 5.
- "architect skill" here means the `architecture:architect` agent available in this environment, running on Claude Opus 4.8; the results are specific to that setup.

---

## 11. Evidence index

- `evidence/archfit-scorecards-fulltools.txt` — the full-tools scorecards and deltas (source for 4.1 and 4.4)
- `evidence/archfit-scorecards-undertooled.txt` — the first, partly-disabled run (source for section 5)
- `evidence/archfit-fulltools/` — per-repo `*-score.txt`, `*-full.md` (the scan report), `*-delta.md`, `*-review.md` (the LLM write-up)
- `evidence/archfit-undertooled/` — per-repo scorecards from before all tools were enabled
- `evidence/architect-reviews/` — the 5 full, cited architect reviews
- `evidence/research-notes.md` — research on Khononov, fitness functions, and rule-based-vs-LLM analysis, with sources
- `evidence/working-notes.md` — the raw data log and how the conclusion evolved

---

## 12. Appendix: independent second opinions

**Codex (a different AI model) — verdict: the first draft OVERCLAIMED; corrections applied.** Key points, all now reflected above: (a) "more reliable" was softened to "more repeatable / automatable, not more trustworthy"; (b) the Balanced-Coupling comparison was too generous to archfit — the architect fits the _intent_ better; (c) "combine them" must keep separate roles, not chain two models; (d) **missing evidence must lower the headline score — a green score on a disabled tool is a defect, and makes cross-repo comparison unsafe until missing data is marked n/a.** This review is why sections 1, 5, 7, and 8.1 are worded as they are.

**Adversarial Claude advisor** — was told to look for bias toward archfit (archfit being the author's own tool). Its delivery channel dropped its final text, but its task (attack the conclusion, check the Balanced-Coupling reasoning, test the "combine" idea for vagueness) overlapped with Codex's, which was captured in full.

**Net effect of the stress-testing:** the conclusion was pulled back from an early, archfit-favouring "more reliable" claim to the role-separated, evidence-gated conclusion in section 1 — and the fairness re-run (section 5) was added specifically so archfit would not be judged while under-tooled.
