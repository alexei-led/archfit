# Configuration basics

Configuration lives in `.archfit.yaml`.

Use this page for the first config. Use
[Configuration reference](configuration-reference.md) for all fields and examples.

Generate a starter file:

```sh
archfit init --root . --output .archfit.yaml
```

Then review the generated modules, layers, and rules before using it as a gate.
Start with narrow rules and baseline accepted current findings while calibrating.
Keep `gate` values aligned with the intended CI policy.

Important sections:

- `layers` — architecture layers, ordered from inner to outer.
- `modules` — owned path groups with optional public APIs and metadata.
- `rules` — executable constraints such as dependencies or layer direction.
- `waivers` — approved temporary deviations with reasons and expiry dates.
- `languages` — language adapter enablement: `auto`, `true`, or `false`.
- `analyzers` — opt-in deeper analysis backends.
- `metrics` — metric gates and delta thresholds.

Small example:

```yaml
version: 1
layers: [domain, application, adapter]
modules:
  domain:
    paths: [internal/domain/**]
    public: [internal/domain]
    layer: domain
    subdomain: core
  adapter:
    paths: [internal/http/**]
    public: [internal/http]
    layer: adapter
    subdomain: supporting
rules:
  - id: domain_no_http
    type: forbidden_dependency
    from: internal/domain/**
    to: internal/http/**
    gate: fail
  - id: layer_direction
    type: forbidden_layer_direction
    gate: fail
languages:
  go:
    enabled: auto
  typescript:
    enabled: auto
  python:
    enabled: auto
  rust:
    enabled: auto
```

Start with explicit, high-value rules. Avoid encoding every preference on day one.

Next:

- [Language support](languages.md) for supported-language examples.
- [Configuration reference](configuration-reference.md) for field details.
