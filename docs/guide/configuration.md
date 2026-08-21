# Configuration basics

Configuration lives in `.archfit.yaml`. Generate a starter file:

```sh
archfit config init --root . --output .archfit.yaml
```

Then review the generated modules, layers, and rules before using `archfit
check` in CI. Start with narrow rules and baseline accepted current findings
while calibrating. Keep `gate` values aligned with the intended CI policy.

Use `archfit analyze --config .archfit.yaml` for local review and `archfit check
--config .archfit.yaml` for CI validation.

To see what the config still needs, run `archfit config update -c .archfit.yaml`.
It reports structure drift, every change `--apply` would write, and per-module
gaps such as a missing `owner:`, a module with neither `subdomain:` nor
`volatility:`, and a missing `layer:` while a `forbidden_layer_direction` rule is
active. `archfit config update --json -c .archfit.yaml` emits the same review as
the `archfit.config-review.v1` document for scripts and agents. Its status line
reads `action_required`, `review_available`, or `no_known_issues`; the last one
means these checks found nothing, not that the config is complete.

LLM authoring commands are off-gate. `config init --ai-classify` emits commented
suggestions by default; `config init --ai-classify --apply` writes those model judgments
live into the generated file, so review the result before using it as a gate.
`config update --ai-classify` prints cited module and rule proposals without changing
`.archfit.yaml`; even `config update --ai-classify --apply` only applies structural drift.
`config enrich owner`, `volatility`, and `subdomain` write separate draft files.
Review the `rationale`, `evidence_refs`, and `basis` fields, change keepers to
`status: approved`, then apply or copy them into the live config deliberately.

Key top-level sections: `layers`, `modules`, `rules`, `waivers`, `languages`,
`analyzers`, `metrics`. See [Configuration reference](configuration-reference.md)
for all fields, examples, and defaults.

Next: [Language support](languages.md) · [Configuration reference](configuration-reference.md).
