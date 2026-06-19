# Starter `.archfit.yaml` templates

A flat, minimal config is the most common way to get noise out of `archfit`:
without owners and subdomains, distance falls back to code structure and Balanced
Coupling advisories flood. These templates are **fully specified** — every module
declares an `owner` and a `subdomain` (or explicit `volatility`) — so distance
and volatility classify cleanly from the first run.

Pick the one closest to your repo, copy it to `.archfit.yaml`, replace the paths
and owners, then run:

```sh
cp examples/go-monolith.archfit.yaml .archfit.yaml
$EDITOR .archfit.yaml
archfit check --config .archfit.yaml --full
archfit score --config .archfit.yaml --full
```

| Template                                      | Shape                                                 |
| --------------------------------------------- | ----------------------------------------------------- |
| [go-monolith](go-monolith.archfit.yaml)       | Single Go deployable, layered packages.               |
| [python-package](python-package.archfit.yaml) | Python `src/` layout, dotted module names.            |
| [ts-monorepo](ts-monorepo.archfit.yaml)       | TypeScript workspace packages, file-path globs.       |
| [ddd](ddd.archfit.yaml)                       | DDD bounded contexts; subdomain-driven volatility.    |
| [microservices](microservices.archfit.yaml)   | Multiple services; `deploy_unit` distance boundaries. |

## Why declare owner and subdomain

`archfit` resolves Balanced Coupling distance by a precedence chain (see the
[configuration reference](../docs/guide/configuration-reference.md#modules)):
an explicit `owner` decides distance before the structural fallback, and a
`subdomain` (`core`/`supporting`/`generic`) or explicit `volatility` sets how
volatile a target is. Omit them and you get the structural fallback plus
`undeclared` volatility — which is exactly what produces advisory floods on flat
configs.

Run `archfit init` first to discover your real structure, then borrow the
metadata patterns from the closest template here. See
[Language support](../docs/guide/languages.md) for per-language path and tool
details.
