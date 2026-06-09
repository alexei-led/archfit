# risk_hub validation — 4-repo acceptance gate

Date: 2026-06-09. Branch: feat/hybrid-tranche1.

## Purpose

Verify the `risk_hub` metric surfaces the genuinely risky symbol-level hubs that
the module-level `blast_radius` metric misses — without overfitting to one repo.
Two candidate designs were compared on all 4 reference repos (3 languages) with
SCIP enabled:

- **Transitive-reachability** (original Task 5 design): per-symbol transitive
  reverse-reachability over symbol refs, SCC-condensed — the symbol-level analog
  of `blast_radius`.
- **Surface-breadth** (accepted): per module, the count of that module's own
  symbols referenced by at least one symbol from a _different_ module — how wide
  a module's externally-coupled surface is.

Gold standard: ccgram's architect review
(`/Users/alexei/Workspace/ccgram/docs/architecture-review/2026-05-23-ccgram-full.md`)
names `window_state_store` as its #1 critical finding (F3): a high-fan-in,
high-churn state hub with a broad mutable surface.

## Results (top-5 per design)

| Repo (lang)        | Transitive-reachability                                                                          | Surface-breadth                                                                                     | blast_radius (existing)                                 |
| ------------------ | ------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| **ccgram** (Py)    | `__init__`(7942), utils(4264), config(3943), topic_state_registry(3668), thread_router(3578)     | **window_state_store**(73), callback_data(72), providers/base(71), telegram_client(69), config(47)  | utils(68%), config(63%), providers/base(59%)            |
| **codegraph** (TS) | types(4207), errors(3235), go-module(3074), path-aliases(3074), workspace-packages(3074)         | types(135), tree-sitter-types(56), queries(46), index(45)                                           | tree-sitter-helpers(31%)                                |
| **pumba** (Go)     | pkg/container(1122), chaos/cliflags(823), pkg/chaos(756), pkg/util(176), chaos/cmd(173)          | pkg/container(139), runtime/containerd(68), runtime/docker(67), runtime/podman(61), chaos/netem(42) | pkg/container(82%), chaos/cliflags(65%), pkg/chaos(59%) |
| **spotinfo** (Go)  | internal/spot(267), internal/mcp(86), cmd `TestMain…`(7), `test/main()`(0), `mcp/test init()`(0) | internal/spot(72), internal/mcp(62), cmd/spotinfo(21)                                               | internal/spot(100%), internal/mcp(50%)                  |

## Findings

1. **Transitive-reachability reproduces `blast_radius`** (ccgram: utils/config;
   pumba: container/cliflags/chaos — identical to blast_radius's own top hubs). It
   adds no distinct signal over a metric archfit already ships.
2. **Transitive is polluted by artifacts**: Python `__init__` re-export symbols
   (ccgram impact 7942), TS barrel/namespace symbols (codegraph), and **test
   symbols + impact-0 entries** (spotinfo). SCIP refs are doc-scoped (indexers do
   not populate `enclosing_range`), which makes transitive reach noisy/inflated.
3. **Transitive fails the gold standard**: `window_state_store` is not in ccgram's
   top 5.
4. **Surface-breadth is distinct, clean, and consistent**: it is the only design
   that ranks `window_state_store` #1 (matching architect F3), carries no test/zero
   noise, and stays sensible across Go/TS/Python (e.g. pumba surfaces
   `pkg/container` + its runtime backends; spotinfo surfaces the two real internal
   packages).

## Verdict: PASS (surface-breadth)

The `risk_hub` metric uses **cross-module surface-breadth**. It surfaces the
known risky hub on the gold-standard repo and produces distinct, clean,
language-consistent results on the other three — not an overfit to ccgram.

Method note: each repo was scanned with both binaries (built from the committed
transitive design and the working-tree surface design); scip was temporarily
enabled in codegraph/pumba/spotinfo configs for the runs and restored afterward.
