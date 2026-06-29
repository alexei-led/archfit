# LLM spike — result (Stage 1: structural-blind)

Date 2026-06-09. Graded mechanically against frozen `ground-truth.md` (pre-registered
before the classifier ran). Classifier = strong-Claude stand-in, firewalled from all
ccgram prose docs; fed archfit JSON evidence + `src/ccgram/` code only.

## Task A — subdomain (firm 10 modules)

| module           | target     | classifier | firm✓/✗ |
| ---------------- | ---------- | ---------- | ------- |
| handlers         | core       | core       | ✓       |
| providers        | core       | supporting | ✗       |
| miniapp          | core       | supporting | ✗       |
| hooks            | supporting | supporting | ✓       |
| session_state    | supporting | core       | ✗       |
| llm              | generic    | generic    | ✓       |
| whisper          | generic    | generic    | ✓       |
| tts              | generic    | generic    | ✓       |
| telegram_adapter | generic    | supporting | ✗       |
| tmux_adapter     | generic    | supporting | ✗       |

**Firm: 5/10 = 50%** (threshold ≥80%) → **MISS**.
**Telegram core-vs-generic split:** handlers=core ✓ but telegram_adapter=generic ✗ → **MISS**.
Ambiguous (5): window_state_ports=supporting ✓, window_state=core ✓, transcript=core ✗
(wanted {supporting,generic}), app_bootstrap=supporting ✓, domain_config=generic ✓ → 4/5 in-set.

Error pattern is systematic, not random: the classifier rates technical adapters
(`telegram_adapter`, `tmux_adapter`) as supporting where the stakeholder-confirmed truth
says generic, and rates state/transcript as core where the truth says supporting.
Several of these labels are individually defensible from code alone (is wrapping
python-telegram-bot "generic infra" or "supporting"? is provider-abstraction the
competitive differentiator?). core/supporting/generic depends on business/competitive
framing absent from the code+metrics — the firewall removed exactly that framing.

## Task B — coupling strength + risk (architect's 6 relationships)

| relationship                         | target strength/risk     | classifier                    | grade                                              |
| ------------------------------------ | ------------------------ | ----------------------------- | -------------------------------------------------- |
| Features ↔ window_state_store        | model/shared-state, high | model, unbalanced, high       | ✓ (and **corrected** archfit's "functional" prior) |
| Polling ↔ polling_state              | model/shared-state, med  | **not surfaced**              | ✗ recall miss                                      |
| Topic subflows ↔ directory_callbacks | functional, med          | **not surfaced**              | ✗ recall miss                                      |
| providers ↔ AgentProvider            | contract/balanced        | contract, balanced, low       | ✓                                                  |
| handlers ↔ TelegramClient            | contract/balanced        | contract, balanced, low       | ✓ (corrected archfit's "model" prior)              |
| decide ↔ TickContext                 | contract/balanced        | not surfaced (not misflagged) | —                                                  |

- **Recall on unbalanced: 1/3** (threshold 3/3) → **MISS**. Both misses are the
  **intra-`handlers` hubs** — predicted granularity gap: archfit groups all handlers
  into one config-module, so polling_state's mutable-singleton sharing and
  directory_callbacks's low cohesion don't surface from module-level + risk_hub-module
  evidence. This is an **evidence-granularity** failure, not an LLM-judgment failure.
- **Model-vs-functional: window_state_store corrected functional→model ✓** — the single
  highest-value capability Tranche 2 targets, and it worked where the classifier had
  cross-module visibility.
- **Precision on named contracts: 0/3 misflagged** → PASS. It even corrected archfit's
  blanket "model"/"functional" mislabels on the two real contract seams.
- **Extra finds:** `upgrade.py → main._restart_requested` (intrusive, genuine private-state
  reach — true positive, not in architect's 6). But `handlers → tmux_adapter` rated
  model/**high** — the architect explicitly accepts tmux centralization as intended,
  low-volatility (review line 572) → a **false-positive over-flag** on a generic hub.

## Verdict vs pre-registered thresholds: **FAIL**

Misses thresholds 1 (50%<80%), 2 (Telegram split), 3 (recall 1/3<3/3). Meets 4 (partial)
and 5 (precision). Per the frozen rule this is a FAIL — recorded as such; not softened.

## Diagnosis — two distinct, well-understood failure modes

1. **Task A (subdomain): plausibly firewall-induced.** core/supporting/generic needs
   business framing the firewall removed; many wrong labels are defensible. Implication
   for Tranche 2: `enrich` subdomain drafting must ingest README/business context, and
   the design's **human-review-the-draft** safeguard is load-bearing (LLM↔stakeholder
   agreement on subdomain is inherently low). The Stage-2 docs re-run tests this.
2. **Task B (recall): evidence-granularity, NOT firewall.** The two misses live inside
   one coarse config-module. **Docs cannot validly fix this** — the docs literally name
   polling*state/directory_callbacks as problems, so a docs re-run would \_leak the answer*
   and falsely inflate recall. The real fix is finer-grained evidence (sub-module /
   per-file symbol hubs in the package), independent of the LLM.

## Stage 2 — framing-allowed (firewalled BY DOCUMENT)

Fresh agent (no anchoring), firewall by document per the design fork: allowed only the
raw business framing the architect's interview was built from — `README.md`,
`docs/architecture.md`, `docs/ai-agents/architecture-map.md`, `pyproject.toml` — and
kept excluding `architecture-review/`, `modularity-review/`, `architecture-design/`,
`architecture-plan/`, `plans/`, `AGENTS.md`, `llm.txt`. (The Interview-context line 434
is itself the Task A answer key near-verbatim, so a blanket "docs allowed" would have
leaked both tasks.) Also added a prompt hint to chase high-fan-**out** source nodes.

**Task A firm with framing: 5/10 = 50% — unchanged from Stage 1.** Framing fixed
`providers` (→core ✓) but broke `hooks` (→core ✗, was correct) and left
`telegram_adapter`/`tmux_adapter` at supporting (target generic) and `session_state` at
core (target supporting). Telegram split: still MISS (`telegram_adapter`→supporting).

**Conclusion (per the pre-registered discriminator): the subdomain divergence does NOT
close with business framing → subdomain is GENUINELY CONTESTABLE, not a knowledge gap.**
Both agents land ~50% with defensible-but-different calls at the core/supporting/generic
boundaries (is a bespoke PTB adapter "generic" or "supporting"? is provider-abstraction
the competitive differentiator or supporting?). This directly validates the design's
**LLM-drafts → human-reviews → commits** pattern (§2): subdomain must never stand
unreviewed.

**Task B recall, re-confirmed as granularity (NOT prompt, NOT firewall):** the fan-out
hint made Stage 2 surface other real fan-out hubs (`app_bootstrap→handlers` eager
imports; `config`→108 deps singleton) — both genuine true positives — but it STILL missed
`polling_state` and `directory_callbacks`. Those two aren't fan-out hubs either; their
problem is intra-module (a mutable singleton shared among polling siblings; one 1086-line
file bundling 7 subflows). archfit emits no per-file cohesion / intra-module shared-state
signal, so the evidence package is blind to that whole class. Confirmed: deterministic
evidence-package gap, fixable without the LLM.

**Stage 2 also reinforced a precision concern:** the LLM over-flags intended
centralization as high-risk (Stage 1: `tmux_adapter`; Stage 2: `app_bootstrap`,
`config`) — it lacks the architect's "this hub is intended / low-volatility" judgment,
which itself depends on the (contestable) subdomain. So coupling refinement must also be
advisory, fed subdomain/volatility context.

---

## Final verdict

**Against the frozen pre-registered thresholds: FAIL (unchanged, not softened).**
Stage 1 missed thresholds 1, 2, 3; Stage 2 did not move them.

**The gate is SPLIT, not uniform.** The design rule was binary per capability — "broadly
matches → build; else → rethink." Read honestly, the two LLM capabilities land on
opposite sides:

**1. Coupling model-vs-functional refinement → PROCEED (validated).** This is the
capability §3–5 of the design exists for, and it earned its keep where the LLM had
cross-module visibility. Both agents independently corrected archfit's blanket priors:
`window_state_store` functional→model, `TelegramClient` model→contract; 0/3 named
contracts misflagged; and the LLM surfaced a real intrusive coupling the architect's own
review MISSED (`upgrade.py → main._restart_requested`) — value beyond the human review.
Build it, off-gate and advisory — but gated behind the Tranche-1.5 granularity fix below,
without which the LLM is blind to a whole class of intra-module hubs
(`polling_state`/`directory_callbacks`).

**2. Subdomain / volatility drafting → RETHINK / DESCOPE (does NOT broadly match).**
50% firm on a 3-way split is barely above the 33% chance line; it is framing-invariant
(Stage 2 didn't move it); and the pre-designated discriminator — the Telegram
core-vs-generic split — FAILED both stages. That is not "broadly matches." Do not launder
this through "the human reviews the draft": that excuse absolves any accuracy, including
zero, and is the rubber-stamp this spike existed to prevent.

The deeper, stronger finding: two capable judges disagree ~50% on subdomain, so
**subdomain-derived volatility is inherently unreliable regardless of who produces it** —
LLM, human, or stakeholder. The fix is not "review the LLM draft"; it is to NOT lean on
subdomain-derived volatility. Concretely: do not build LLM subdomain-drafting as designed.
At most a thin opt-in experiment whose output is NEVER weighted into a gate-adjacent
metric. `risk_hub` already treats volatility as neutral 1.0 absent explicit config — this
result argues for keeping volatility a purely human-authored, explicitly-owned config
field, not an LLM-drafted one.

### Recommended sequencing (revises the plan's "build Tranche 2 as designed")

- **Tranche 1.5 (deterministic, do first):** emit per-file / intra-module cohesion +
  shared-state hub signals in the evidence package. Closes the recall class the LLM cannot
  reach (`polling_state` singleton, `directory_callbacks` low-cohesion) and is a hard
  prerequisite for coupling refinement.
- **Tranche 2 (LLM, off-gate) — build ONLY the coupling-refinement + `explain` half:**
  provider interface + the model-vs-functional `Classify` over collected edges + `explain`
  narrative. Advisory, cached, never on `check`. Feed it subdomain/volatility CONTEXT
  (from human-authored config) to avoid over-flagging intended centralization
  (Stage 1 over-flagged `tmux_adapter`; Stage 2 over-flagged `app_bootstrap`, `config`).
- **DROP from Tranche 2 as designed:** LLM-drafted subdomain/volatility into `.archfit.yaml`.
  Keep subdomain/volatility a human-authored config field. Optionally a thin, clearly-labeled
  "suggestion" experiment later — never weighted.
- **Re-validate** the coupling-refinement output against this ccgram gold standard after
  Tranche 1.5 lands, plus one more repo once another architect review exists.

Caveats stand: ccgram-only (N=1); small denominators (10 firm modules, 6 relationships);
strong-Claude stand-in (a weaker provider like Ollama may underperform — validate per
provider). The coupling conclusion is the confident one; the subdomain conclusion is a
descope, not a defer.
