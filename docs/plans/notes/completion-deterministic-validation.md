# Completion plan (deterministic) — acceptance sweep (2026-06-11)

Binary: `.bin/archfit` @ feat/completion-deterministic head. Each repo ran
`check --full --advisory --report --format json` TWICE; `det` is a byte
compare of the two outputs.

| repo      | exit | determinism    | verdict | metrics | findings | file_facts | agent_tasks |
| --------- | ---- | -------------- | ------- | ------- | -------- | ---------- | ----------- |
| ccgram    | 0/0  | byte-identical | fail    | 13      | 424      | 383        | 3           |
| pumba     | 0/0  | byte-identical | pass    | 13      | 8        | 0          | 0           |
| spotinfo  | 0/0  | byte-identical | pass    | 13      | 3        | 0          | 0           |
| codegraph | 0/0  | byte-identical | pass    | 13      | 12       | 0          | 0           |
| archfit   | 0/0  | byte-identical | pass    | 10\*    | 5        | 93         | 0           |

\* archfit's self-config explicitly disables risk_hub, architecture_fitness,
functional_candidates — the `metrics.<name>.enabled: false` fix is visibly
honored (was silently ignored before this plan).

Notes:

- No panics, no parse failures anywhere.
- `agent_tasks` fires exactly where active gate findings exist (ccgram: 3
  findings → 3 tasks with goals, files, and exact re-check commands).
- SARIF: ccgram output (424 results: 3 error / 421 warning) and the violating
  test fixture both validate clean against the official 2.1.0 schema
  (`uvx check-jsonschema --schemafile https://json.schemastore.org/sarif-2.1.0.json`).
- `file_facts` = 0 on pumba/spotinfo/codegraph because their configs do not
  enable `tools.scip` — facts require the symbol graph by design (absent ≠ 0).
- gitnexus enrichment verified separately on ccgram (coverage ok, 144 modules
  enriched) — see `gitnexus-adapter-decision.md`.
- change_locality verified in delta mode on archfit (`--base main`): "33
  cross-module edge(s) from 24 changed file(s); forward reach 45 file(s)".
