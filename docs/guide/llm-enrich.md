# LLM enrichment (off-gate): enrich, labels, explain --llm

archfit's verdict is deterministic — `check` never calls a model. The LLM
layer refines the one thing deterministic analysis cannot judge: whether a
cross-module dependency's integration strength is really `functional`
(invokes behavior), `model` (types cross the boundary), `contract`
(published stable interface), or `intrusive` (reaches into internals). The
deterministic heuristic blanket-labels most call edges `functional`; the
spike that validated this design measured ~91% of edges landing there.

The workflow is **draft → review → pin**:

```text
archfit enrich        # LLM drafts → .archfit-labels.yaml (status: draft)
$EDITOR .archfit-labels.yaml   # review: approve or delete each draft
archfit check         # consumes APPROVED labels only — still LLM-free
```

## Configuration

```yaml
tools:
  scip:
    enabled: "on" # enrich needs the symbol-level strength hints
  llm:
    provider: anthropic # anthropic | openai | ollama
    model: claude-opus-4-8
    # base_url: http://localhost:11434/v1   # ollama only
```

API keys come from the standard env vars only — `ANTHROPIC_API_KEY` /
`OPENAI_API_KEY` — never from config. `archfit doctor` shows provider, key
presence, and cache status.

## The labels file (.archfit-labels.yaml)

```yaml
version: 1
labels:
  - from: handlers
    to: window_state
    strength: model
    rationale: "WindowState dataclasses cross the boundary"
    evidence_hash: 4f1c… # written by enrich; verified by check
    status: draft # ← flip to approved to pin
```

- Only `status: approved` entries affect classification; drafts are inert.
- Precedence: config `public`/`internal` globs > approved labels > SCIP hint.
- A label pins all edges of the ordered module pair (`from` → `to`).
- `evidence_hash` fingerprints the pair's import-graph edges at enrich time.
  On full runs, `check` recomputes it: a mismatch means the dependency surface
  changed since review — the label is ignored and a `labels/stale` advisory
  tells you to re-run enrich. Hand-authored labels may omit the hash.
- A malformed labels file fails `check` loudly (exit 3) — a half-read file
  must never silently alter the gate.

## Determinism

The gate stays reproducible: labels are plain YAML read deterministically, and
the arch ring test (`TestArchImports/llm_ring_unreachable_from_internal`)
proves at CI time that no internal package can even import the LLM layer.
Enrich itself is replayable through the content-addressed response cache at
`.archfit-cache/llm/` (ignored by git by default; commit it if you want
byte-identical enrich replay across machines). `--no-cache` forces fresh
calls.

## explain --llm

`archfit explain <fingerprint> --llm` appends a Balanced Coupling narrative
(why the finding matters, the risk, a repair sketch) after the deterministic
explain output, using the same provider and cache. Without `--llm`, explain
is fully offline.

## Scope guard

Enrich refines coupling strength labels only. Subdomain and volatility remain
human-authored config — the validation spike showed capable judges disagree at
chance level on subdomain splits, so no tool (or human draft-launderer) should
generate them.
