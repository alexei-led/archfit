# Configuration basics

Configuration lives in `.archfit.yaml`. Generate a starter file:

```sh
archfit config init --root . --output .archfit.yaml
```

Then review the generated modules, layers, and rules before using it as a gate.
Start with narrow rules and baseline accepted current findings while calibrating.
Keep `gate` values aligned with the intended CI policy.

LLM authoring commands are draft-first. `config update --llm` prints cited module
and rule proposals without changing `.archfit.yaml`; `config enrich owner`,
`volatility`, and `subdomain` write separate draft files. Review the `rationale`,
`evidence_refs`, and `basis` fields, approve keepers, then apply or copy them
into the live config deliberately.

Key top-level sections: `layers`, `modules`, `rules`, `waivers`, `languages`,
`analyzers`, `metrics`. See [Configuration reference](configuration-reference.md)
for all fields, examples, and defaults.

Next: [Language support](languages.md) · [Configuration reference](configuration-reference.md).
