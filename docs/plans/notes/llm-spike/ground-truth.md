# LLM spike — pre-registered ground truth + pass thresholds

**Frozen 2026-06-09, BEFORE running the blind classifier.** Do not edit after the
classifier runs. Source of truth: ccgram architect review
`/Users/alexei/Workspace/ccgram/docs/architecture-review/2026-05-23-ccgram-full.md`
(Interview context line 434; Coupling review 529–541; F1–F3 502–527).

Purpose: gate Tranche 2 (`enrich` = LLM drafts subdomain/volatility + refines
model-vs-functional). PASS → build as designed. FAIL → rethink before planning.

Scope/caveats (baked into verdict): **ccgram-only** (no other repo has an architect
review); small denominators (10 firm modules, 6 relationships); **strong-Claude
stand-in** (PASS = upper bound, not Ollama-validated); structural-blind firewall =
**harder than real `enrich`** (which reads docs), so a FAIL is partly firewall-attributable
→ staged docs re-run localizes cause.

Deterministic baseline the LLM must beat: archfit labels **419/461 edges "functional"**
(call-edge → "functional"); only 17 model, 7 contract, 2 intrusive. All 3 hubs are
deterministically "functional." The headline test is whether the LLM **corrects**
`window_state_store` and `polling_state` to model/shared-state while keeping
`directory_callbacks` functional.

---

## Task A — subdomain per config module (core / supporting / generic)

Target from Interview context: core {Telegram UX, provider abstraction,
polling/status detection, inter-agent messaging, Mini App}; supporting {session
monitoring, hooks, state persistence}; generic {tmux, Telegram API adaptation,
LLM/Whisper HTTP}.

**Firm (10) — counted in the pass denominator:**

| module           | target     | rationale                                            |
| ---------------- | ---------- | ---------------------------------------------------- |
| handlers         | core       | Telegram UX + polling/status + inter-agent messaging |
| providers        | core       | provider abstraction                                 |
| miniapp          | core       | Mini App                                             |
| hooks            | supporting | "hooks" named supporting                             |
| session_state    | supporting | session monitoring + state persistence               |
| llm              | generic    | LLM HTTP integration                                 |
| whisper          | generic    | Whisper HTTP integration                             |
| tts              | generic    | same class as LLM/Whisper HTTP                       |
| telegram_adapter | generic    | "Telegram API adaptation" = generic infra            |
| tmux_adapter     | generic    | tmux = generic infra                                 |

**Highlighted sub-check — the Telegram core-vs-generic split:** `handlers` = core
(Telegram _UX_) AND `telegram_adapter` = generic (Telegram _API adaptation_). Both
must be right. This is genuine expert judgment, not surface pattern-matching.

**Ambiguous (5) — reported, NOT in firm denominator; correct if within accepted set:**

| module             | accepted              | note                                             |
| ------------------ | --------------------- | ------------------------------------------------ |
| window_state_ports | {core, supporting}    | ports/contract for state                         |
| window_state       | {supporting, core}    | "state persistence" vs central domain state (F3) |
| transcript         | {supporting, generic} | inbound monitoring vs parsing infra              |
| app_bootstrap      | {generic, supporting} | factory/lifecycle wiring                         |
| domain_config      | {generic, supporting} | config/utils vs domain formatting                |

---

## Task B — coupling strength + risk on the 6 labeled relationships

**Unbalanced (must surface as risky; strength label as shown):**

| relationship                           | strength (target)         | risk   | note                                               |
| -------------------------------------- | ------------------------- | ------ | -------------------------------------------------- |
| Features ↔ `window_state_store`        | model / shared-state      | high   | det. baseline says "functional" → must correct     |
| Polling features ↔ `polling_state`     | model / shared-state      | medium | intra-`handlers` hub → needs symbol-level evidence |
| Topic subflows ↔ `directory_callbacks` | functional (low cohesion) | medium | intra-`handlers`; the one genuinely functional     |

**Balanced (must NOT flag as high-risk; strength = contract):**

- providers ↔ `AgentProvider`/`ProviderCapabilities` protocol — contract.
- handlers ↔ `TelegramClient` — contract.
- `window_tick/decide.py` ↔ `TickContext` — contract.

---

## Pass thresholds (pre-registered)

**PASS** iff ALL of:

1. Task A firm ≥ **80%** (≥ 8 / 10).
2. Telegram core-vs-generic split correct (`handlers`=core AND `telegram_adapter`=generic).
3. Task B recall **3/3** — all three unbalanced hubs in the classifier's risky set.
4. Task B model-vs-functional ≥ **2/3** correct — critically, ≥1 of
   {window_state_store, polling_state} corrected from "functional" to model/shared-state,
   AND directory_callbacks kept functional.
5. Task B precision — ≤ **1** of the 3 contract relationships misflagged as high-risk.

**BORDERLINE** — misses exactly one sub-threshold by a small margin → trigger the
staged "re-run with docs allowed" to localize cause (LLM judgment vs evidence-package
insufficiency).

**FAIL** otherwise → rethink Tranche 2 (prompt, evidence packaging, or whether
per-edge LLM labeling is worth building).
