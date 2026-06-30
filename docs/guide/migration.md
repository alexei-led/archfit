# Migrating a config to archfit v1.0

archfit v1.0 finalizes the `.archfit.yaml` schema. A few keys from pre-1.0 configs
were renamed or removed. archfit fails loudly (exit 3) with a hint pointing here
rather than silently ignoring stale keys.

## Renamed

| pre-1.0 key          | v1.0 key     | notes                                         |
| -------------------- | ------------ | --------------------------------------------- |
| `tools:` (top level) | `analyzers:` | same nested shape, minus `complexity` (below) |

Rename the top-level `tools:` map to `analyzers:`:

```yaml
# before
tools:
  scip: { enabled: true }
  clones: { enabled: true }

# after
analyzers:
  scip: { enabled: true }
  clones: { enabled: true }
```

## Removed

| removed key                                     | reason                                                                | action           |
| ----------------------------------------------- | --------------------------------------------------------------------- | ---------------- |
| `analyzers.complexity` (was `tools.complexity`) | gocyclo/lizard backends dropped; complexity is no longer a gate input | delete the block |
| `metrics.risk_hub`                              | metric removed                                                        | delete the block |
| `metrics.functional_candidates`                 | metric removed                                                        | delete the block |
| `gitnexus:` / `metrics.gitnexus`                | gitnexus integration dropped                                          | delete the block |

## Current schema

Top-level keys: `version`, `coupling`, `languages`, `analyzers`, `ai`, `layers`,
`modules`, `rules`, `metrics`, `module_review`, `file_class`.

`analyzers`: `syntax`, `scip` (+`timeout`), `clones` (+`timeout`), `cargo_modules`.

`metrics` (known keys): `encapsulation`, `unbalanced_edge`, `cycle`, `coverage`,
`blast_radius`. An unknown metric key is a config error.

Regenerate a current starter config any time with `archfit init` (it will not
overwrite an existing config unless you pass `--force`).
