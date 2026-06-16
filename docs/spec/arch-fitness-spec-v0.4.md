<!-- markdownlint-configure-file { "MD013": { "line_length": 180, "code_blocks": false, "tables": false } } -->

# Archfit CLI — Build Spec

**Version:** 0.4 draft
**Date:** 2026-06-06
**Product and binary name:** `archfit`
**Primary goal:** deterministic architecture-drift feedback with legible coupling metrics and agent-repairable diagnostics
**First languages:** Go, TypeScript, Python

---

## 1. Product definition

`archfit` is a local CLI for deterministic architecture-drift feedback.

The name is short for **architecture fitness**: executable checks that verify whether code still fits the declared architecture under change. It signals health and fit, not a generic
quality score.

It protects the part of code quality that tests and linters do not protect: whether new code keeps the intended architecture intact. AI agents can generate large amounts of locally
correct code that quietly adds cross-boundary dependencies, bypasses public APIs, duplicates domain logic, or widens change blast radius. That drift raises the cost of every future
change, including agent token burn, context size, and repair iterations.

AI models are primary consumers of the output, but not the architecture judge. `archfit` provides deterministic facts, policy decisions, metric deltas, and repair constraints. Agents
use that output to fix code; humans define architecture intent, approve exceptions, and refresh the architecture map.

It stays architecture-specific. It runs or consumes existing tools when their output helps architecture review, metric confidence, or agent repair.

The core workflow mirrors tests and linters:

```text
agent edits code
  -> archfit check
  -> deterministic finding or metric delta
  -> agent repairs within stated constraints
  -> archfit check passes
```

One-line version:

> A deterministic architecture-drift CLI that gates declared boundary violations, reports legible coupling metrics, and returns repairable diagnostics for AI agents.

---

## 2. Scope

### In scope for v1

- Local CLI.
- Config file: `archfit.yaml`.
- Deterministic dependency extraction for Go, TypeScript, and Python.
- Normalized architecture graph.
- Explicit architecture rules.
- Baseline support.
- Accepted exceptions.
- Legible architecture metrics: encapsulation ratio, unbalanced-edge counts, cycles, change-locality indicators, and coverage/confidence.
- Balanced Coupling classification for findings: strength, distance, volatility, and explicitness.
- Agent-first JSON output with evidence and repair constraints.
- Console and Markdown output for humans.
- SARIF output for CI/code-scanning compatibility.
- Small extension points for future dimensions and languages.

### V1 product boundary

V1 is a local deterministic engine. It should be easy to run inside an AI agent loop, developer shell, pre-commit/pre-push hook, or CI job.

The design keeps the first release focused on four outcomes:

- extract architecture facts from code;
- enforce explicit architecture intent;
- report small, legible architecture metrics and deltas;
- produce repairable diagnostics for AI agents.

Later product layers can build on this engine: shared ledgers, dashboards, GitHub integration, richer reports, and additional tool adapters. The local feedback loop comes first
because agents and CI need deterministic results while preparing commits and pull requests.

---

## 3. Methodology and operating model

The methodology follows Vlad Khononov's Balanced Coupling model. The CLI makes parts of that model executable. It does not replace architectural judgment.

### Methodology terms

Use the same review dimensions everywhere:

- **Integration strength** — how much knowledge crosses a boundary: `contract`, `model`, `functional`, `intrusive`.
- **Distance** — the cost of co-evolving coupled components: code distance, ownership distance, deploy/runtime distance when known.
- **Volatility** — the likelihood a component will need meaningful change, primarily from DDD subdomain classification: `core`, `supporting`, `generic`, `unknown`.
- **Explicitness** — whether the shared knowledge is intentional and visible, or implicit and fragile.
- **Balance** — coupling is acceptable when strength and distance counterbalance, or when volatility is low enough that the imbalance is not painful.

The tool should report findings in this vocabulary so Vlad, architects, humans, and agents speak the same language.

### Gate channel

The gate channel is deterministic pass/fail.

Only explicit architecture rules can fail the gate:

- forbidden dependency;
- public API only;
- internal API access;
- forbidden layer direction;
- new cross-module dependency if configured as fail;
- cycles if configured as fail;
- expired exception.

### Metric channel

Metrics are deterministic, narrow, and legible. They summarize architecture drift from explicit graph facts, config, baselines, and exceptions.

Initial metrics:

- encapsulation ratio;
- unbalanced-edge count by severity;
- cycle count;
- changed-module and blast-radius indicators;
- coverage/confidence for the facts behind those metrics;
- map staleness.

Avoid one blended architecture score in v1. Balanced Coupling is classification and severity language for findings, not one opaque number.

Default policy:

```text
gate findings fail CI
metric regressions warn by default
```

### Agent loop contract

The command agents should call is:

```bash
archfit check --base <ref> --format json
```

Deterministic guarantee:

```text
same repo + same config + same base/head + same tool versions
=> same gate findings and deterministic metrics
```

Agent behavior:

- read `findings[]` and `agent_tasks[]` from JSON;
- preserve the stated architecture constraint;
- prefer allowed public APIs or configured contracts;
- do not create or approve exceptions;
- rerun `archfit check` after changes.

Success condition:

```text
no gate findings
no configured metric regression
```

A green `archfit check` means the change conforms to configured architecture checks. It does not mean the code is correct, secure, performant, simple, or well tested.

### AI agent feedback

The CLI output must be self-contained enough for an agent to fix the problem without guessing architecture intent. A finding should explain:

- what changed;
- which rule, metric, or invariant was affected;
- exact evidence: file, line, edge, source module, target module;
- integration strength, distance, volatility, and explicitness when applicable;
- why the edge is forbidden or risky;
- what constraint must be preserved;
- what paths, modules, or APIs are allowed instead;
- how to verify the fix.

Do not emit cryptic messages such as `ARCH001 failed`. Codes are useful only when paired with clear, structured context.

Human-friendly reports can be generated from the same JSON, or explained later by an LLM. The CLI itself should optimize first for precise agent instructions.

---

## 4. Minimal CLI

```bash
archfit init
archfit doctor
archfit baseline
archfit check                 # the analysis command (gate + report, controlled by flags)
archfit scan                  # alias: check --full --advisory --report --format markdown
archfit explain <finding-id>
```

`check` is the only analysis command. There is one engine; delta-vs-full, advisory, and report behavior are flags
on `check`, following the linter convention (`golangci-lint run --new-from-rev`, `ruff check --diff`, `semgrep
--baseline-commit`). `scan` is a preset alias for the human-audit use case, not a separate code path.

### `archfit init`

Creates a starter `archfit.yaml` from repository structure.

It may infer modules from:

- Go packages/modules;
- npm/pnpm/yarn workspaces;
- Python packages;
- common folders such as `services/*`, `apps/*`, `packages/*`, `libs/*`.

Inference is a starting point, not truth.

### `archfit doctor`

Shows available extractors and external tools.

It reports install hints and leaves tool installation to the user or CI setup.

### `archfit baseline`

Stores accepted current architecture state in `.archfit/baseline.json`.

Baseline prevents legacy debt from blocking adoption. `archfit check` should report new drift by default.

### `archfit check`

Main command for the agent and CI loop, and the only analysis command.

Runs extraction, rules, metric calculation, baseline filtering, exception filtering, and outputs diagnostics. By default it emphasizes architecture facts changed since `--base` or
the accepted baseline.

Flags select the run shape (no separate analysis subcommand):

```text
--base <ref>              delta scope: emphasize findings new since <ref> (gate default for CI/agents)
--full                    full-repo scope: full inventory (baseline debt, exceptions, staleness)
--advisory                enable the semantic advisory channel (advisory only; never gates)
--report                  report-only: do not fail on metric regressions (gate rules still set exit 1)
--format json|markdown|sarif   output renderer(s)
```

Typical use:

```bash
archfit check --base main --format json
```

Exit codes:

```text
0 pass
1 gate failed
2 warnings/metric regressions outside threshold, non-blocking by default
3 tool/config error
```

### `archfit scan`

Preset alias for the human-audit use case (onboarding, calibration, consulting audits, architecture map refreshes):

```bash
archfit scan  ==  archfit check --full --advisory --report --format markdown
```

It shows full-repo metrics, baseline debt, accepted exceptions, semantic advisories, stale map areas, and coverage,
and does not imply CI failure unless explicit rules fail. `scan` is sugar over the same engine — it resolves to the
flag set above before the engine runs; the engine never learns which verb was typed. If `scan` ever needs something
`check` cannot express as a flag, that is a design smell (the modes have forked); the fix is a new flag, not a new
command.

### `archfit explain <finding-id>`

Prints rationale, violated rule, graph evidence, and repair constraints.

---

## 5. Implementation language and tool stack

Build the core CLI in Go.

Go is the best fit for the core because `archfit check` must be fast, reliable, easy to install in CI, and easy for AI agents to run repeatedly. TypeScript and Python remain
first-class analysis targets, but their ecosystem-native tools should be used through adapters instead of making Node or Python the core runtime.

### Core language decision

Use Go for:

- CLI command routing;
- config loading and validation;
- graph normalization;
- rule execution;
- metric calculation;
- baseline and exception matching;
- subprocess orchestration for external adapters;
- JSON, SARIF, Markdown, and console output;
- release packaging.

Use TypeScript for:

- TypeScript/JavaScript extractor adapters when dependency-cruiser or the TypeScript compiler API provides better facts than the native scanner;
- future UI/dashboard code if the product grows beyond the CLI.

Use Python for:

- Python extractor adapters when import-linter/grimp provides better facts than the native scanner;
- metric experiments and calibration scripts if they help Vlad and Alexei tune formulas faster.

### Go libraries for the core

Recommended v1 dependencies:

- CLI: `github.com/alecthomas/kong` for a small typed command surface. Use Cobra only if command/plugin complexity grows.
- Config: `gopkg.in/yaml.v3` initially. Consider `github.com/goccy/go-yaml` only if line-aware config diagnostics become important.
- Globs: `github.com/bmatcuk/doublestar/v4` for predictable `**` matching.
- Go extraction: `golang.org/x/tools/go/packages`.
- Git: shell out to `git`; do not embed a Git implementation in v1.
- Graph: custom adjacency maps first. Add `gonum/graph` only if algorithms outgrow simple cycle/reachability code.
- JSON: stdlib `encoding/json`; keep schemas versioned with tests.
- SARIF: internal structs first; add a library only if compatibility becomes painful.
- Markdown: simple templates first.
- Release: GoReleaser.

Keep the binary boring. Avoid TUI frameworks, graph databases, embedded scripting runtimes, and broad linter bundles in v1.

### Tool tiers

#### Tier 0 — required runtime tools

These are expected for normal `archfit check`:

- `archfit` Go binary;
- `git` for changed files, base/head comparison, baseline support, and optional churn measurements.

#### Tier 1 — built-in v1 extractors

These should work without extra tool installation:

- Go: `go/packages` package/import extraction.
- TypeScript: conservative static import scanner with `tsconfig` path alias support.
- Python: conservative static import scanner for `import` and `from ... import ...` statements.

Built-in TS/Python scanners may produce lower confidence than ecosystem-native adapters. That is acceptable if the coverage output is honest.

#### Tier 2 — useful optional adapters now

These tools are worth supporting early because they directly improve architecture facts:

- TypeScript/JavaScript: `dependency-cruiser` for dependency graphs, path aliases, cycles, and existing boundary rules.
- Python: `import-linter` / `grimp` for import graphs and Python architecture contracts.
- Structural patterns: `ast-grep` for targeted rules such as direct database access, framework leakage, provider DTO leakage, or forbidden imports when simple path rules are not
  enough.
- Git history: `git log` first; GitNexus adapter later if available and useful for co-change/change-locality calibration.

Optional adapters must never be silent. If applicable but missing, record a coverage gap and lower confidence for metrics that depend on that evidence.

#### Tier 3 — useful for calibration, not v1 runtime dependencies

Use these while developing and validating the tool, or expose them later through adapters:

- LSPs: `gopls`, `pyright`, and TypeScript language server for validating extractor accuracy and future symbol-level facts.
- Tree-sitter: useful fallback for precise syntax extraction across TS/Python/Go, but avoid making it mandatory until native scanners or adapters prove insufficient.
- SCIP: useful future symbol graph format when file/package-level import facts are not enough.
- codegraph: useful for architecture-review calibration and richer semantic graph comparison, not required for the local v1 feedback loop.
- GitNexus: useful for change-coupling, churn, historical blast radius, and volatility calibration, but not required for deterministic boundary gates.

### Tool selection rule

Add a tool only when it improves one of the current product primitives:

- architecture graph facts;
- explicit rule enforcement;
- architecture metrics;
- Balanced Coupling classification;
- coverage/confidence;
- agent repair diagnostics.

Defer tools that only provide generic style, security, formatting, dead-code, or dependency hygiene findings unless a specific architecture rule needs them.

---

## 6. Configuration

`archfit.yaml` is executable architecture intent.

Minimal example:

```yaml
version: 1

modules:
  checkout:
    paths: ["services/checkout/**"]
    layer: application
    subdomain: supporting
    volatility: medium
    owner: team-checkout
    deploy_unit: web-api
    reviewed_at: "2026-06-01"
    reviewed_by: "@architect"

  pricing:
    paths: ["services/pricing/**"]
    public: ["services/pricing/contracts/**"]
    internal: ["services/pricing/internal/**"]
    layer: domain
    subdomain: core
    volatility: high
    owner: team-pricing
    deploy_unit: pricing-service
    reviewed_at: "2026-06-01"
    reviewed_by: "@architect"

layers:
  - domain
  - application
  - infrastructure

rules:
  - id: checkout-no-pricing-internals
    type: public_api_only
    from: checkout
    to: pricing
    gate: fail

  - id: domain-no-infra
    type: forbidden_layer_direction
    from_layer: domain
    to_layer: infrastructure
    gate: fail

exclusions:
  - "**/generated/**"
  - "**/*.pb.go"

tools:
  git:
    enabled: true
  dependency_cruiser:
    enabled: auto
  import_linter:
    enabled: auto
  ast_grep:
    enabled: auto

metrics:
  encapsulation:
    enabled: true
    gate: warn
    min_delta: 0

  unbalanced_edges:
    enabled: true
    gate: warn
    max_new_high: 0

  cycles:
    enabled: true
    gate: fail
    max_new: 0

  change_locality:
    enabled: true
    gate: warn

  coverage:
    enabled: true
    gate: warn

map_review:
  stale_after: 90d
  gate: warn

exceptions:
  - rule: checkout-no-pricing-internals
    from: "services/checkout/legacy/**"
    to: "services/pricing/internal/**"
    reason: "migration"
    approved_by: "@alexei"
    expires: "2026-12-01"

outputs:
  json: true
  markdown: true
  sarif: true
```

Keep config small. Add fields only when a rule or metric uses them. Tool setting `auto` means: run the adapter only when the tool is installed and the repository markers make it
applicable; otherwise record a coverage gap when the missing evidence matters.

---

## 7. Architecture graph

All extractors normalize into one graph.

### Nodes

```text
repo
module
package
file
symbol optional later
external_dependency
```

### Edges

```text
belongs_to
imports
depends_on
exposes
uses_internal
changed_with optional later
```

MVP can work at file/package/module level. Symbol-level precision is optional and should come later through SCIP or language-specific tooling.

Minimal edge shape:

```json
{
  "from": "file:services/checkout/discounts.ts",
  "to": "file:services/pricing/internal/rules.ts",
  "kind": "imports",
  "language": "typescript",
  "confidence": "high",
  "locations": [{ "file": "services/checkout/discounts.ts", "line": 42 }]
}
```

---

## 8. Language extraction

### Go

First-class native extractor.

Use `go/packages` to resolve packages and imports.

Initial facts:

- package imports;
- file-to-package mapping;
- module path;
- Go `internal/` package access;
- test package distinction.

### TypeScript

Start conservative.

Use one of:

- native import scanner with `tsconfig` path alias support;
- dependency-cruiser adapter if installed.

Initial facts:

- static imports;
- workspace package boundaries;
- path aliases;
- `index.ts` / configured public API convention;
- internal folders.

### Python

Start conservative.

Use one of:

- native AST import scanner;
- import-linter/grimp adapter if installed.

Initial facts:

- static imports;
- relative imports;
- package roots;
- `_private.py` and `internal/` conventions;
- lower confidence for unresolved dynamic imports.

Unknown imports lower confidence. They should not become false gate failures.

---

## 9. Rules

Rules are explicit architecture invariants.

MVP rule types:

```text
forbidden_dependency
public_api_only
internal_api_access
forbidden_layer_direction
new_cross_module_dependency
cycle
```

Each rule has:

```yaml
id: stable-id
type: rule-type
gate: off | warn | fail
```

Rules generated by `archfit init` should default to `warn` until a human promotes them to `fail`.

Rules operate on the normalized graph, not raw tool output.

Example finding:

```json
{
  "id": "archfit-001",
  "kind": "gate",
  "status": "new",
  "rule_id": "checkout-no-pricing-internals",
  "severity": "high",
  "confidence": "high",
  "edge": {
    "from": { "module": "checkout", "path": "services/checkout/discounts.ts" },
    "to": { "module": "pricing", "path": "services/pricing/internal/rules.ts" },
    "kind": "imports"
  },
  "locations": [{ "file": "services/checkout/discounts.ts", "line": 42 }],
  "matched_by": {
    "from_module_glob": "services/checkout/**",
    "to_internal_glob": "services/pricing/internal/**"
  },
  "why": "checkout imports pricing internals",
  "constraint": "Use services/pricing/contracts/** instead."
}
```

---

## 10. Measurement and metrics

Metrics must be deterministic, legible, and honest about their inputs.

A metric is valid only if it reports:

- metric version;
- exact operational definition;
- inputs used;
- coverage/confidence;
- findings that affected it;
- whether it came from delta mode (`check --base`) or full mode (`check --full`, aliased `scan`).

Avoid one blended architecture score in v1. A composite number can hide too much: detected edges, human-declared domain volatility, inferred distance, missing coverage, and
accepted exceptions. Report small metrics that measure one thing each.

Metric output:

```json
{
  "name": "encapsulation",
  "value": 0.86,
  "display": "8.6/10",
  "band": "serviceable",
  "confidence": "high",
  "metric_version": "encapsulation.v1",
  "mode": "delta",
  "definition": "contract / (contract + intrusive) cross-boundary edges (functional, model, unknown excluded)",
  "delta": -0.03
}
```

### 10.1 Bands and thresholds

Use bands for human-facing output and gates. Keep exact values for delta comparison and machine output.

Default bands:

```text
strong       9.0-10.0
serviceable 7.0-8.9
mixed       5.0-6.9
poor        3.0-4.9
critical    0.0-2.9
```

Rules:

- Prefer delta gates first: metric did not regress, no new high-severity unbalanced edges, no new cycles.
- Use absolute thresholds only after calibration on real repositories.
- Do not imply precision the metric does not have. For noisy metrics, report a band and confidence rather than pretending 7.3 is meaningfully different from 7.4.
- If confidence is low, cap the highest reportable band.

### 10.2 Primitive measurements

Measure relationships first. Metrics summarize relationship facts.

#### Boundary measurement

What it measures:

- source module;
- target module;
- public/internal classification;
- layer direction;
- owning path glob;
- whether the edge is new, baseline, excepted, expired, or fixed.

How to measure:

- map files/packages to modules from `archfit.yaml`;
- classify target path against `public`, `internal`, and exclusions;
- compare edge against configured rules and baseline.

Used by:

- gate findings;
- encapsulation metric;
- unbalanced-edge metric;
- agent repair constraints.

#### Integration strength measurement

What it measures: how much knowledge crosses a boundary.

Strength levels:

```text
contract < model < functional < intrusive
```

V1 deterministic proxies:

```text
contract
  dependency targets configured public API, facade, contract, DTO,
  or published interface

model
  dependency targets shared domain model, schema, DTO package, or exported
  business entity used by multiple modules

functional
  dependency shares business-rule knowledge, duplicated validation, required
  call order, or config-assisted domain concept coupling

intrusive
  dependency targets internal/private path, private package, storage schema,
  generated provider model, or implementation detail
```

V1 confidence rule:

- `contract` and `intrusive` can be high confidence from paths and exports;
- `model` is medium confidence from configured shared-model paths;
- `functional` starts low or medium confidence unless configured explicitly or found by an optional semantic pass.

Used by:

- unbalanced-edge severity;
- encapsulation metric;
- agent repair hints.

#### Explicitness measurement

What it measures: whether shared knowledge is visible and intentional.

How to measure:

- explicit: configured public API, contract package, facade, published schema;
- implicit: internal access, direct storage access, magic values, positional data, duplicated rules, hidden ordering requirements.

Used by:

- encapsulation metric;
- severity and confidence;
- semantic advisory candidates.

#### Connascence measurement

What it measures: finer-grained knowledge sharing inside a strength level.

Static degrees, weakest to strongest:

```text
name < type < meaning < algorithm < position
```

Dynamic degrees, weakest to strongest:

```text
execution < timing < value < identity
```

V1 approach:

- use static signals only when easy to detect;
- treat dynamic signals as future, config-assisted, or semantic-advisory evidence;
- use connascence to explain severity, not as a hard gate by itself.

Examples:

- primitive status codes across modules -> meaning;
- parallel hashing/encryption logic -> algorithm;
- positional arrays/tuples as integration payloads -> position;
- required call order -> execution/temporal coupling.

#### Distance measurement

What it measures: cost of changing coupled components together.

V1 levels:

```text
same_package < same_module < cross_module_same_owner < cross_module_different_owner < cross_deploy_unit
```

V1 inputs:

- module path;
- package/workspace boundary;
- `owner` from `archfit.yaml`;
- `deploy_unit` from `archfit.yaml`.

Future inputs:

- repository boundary;
- runtime sync/async boundary;
- shared CI/test/release lifecycle.

#### Volatility measurement

What it measures: probability that a component will need meaningful change.

Primary input is human-defined architecture intent:

```text
core       -> high volatility
supporting -> low or medium volatility
generic    -> low functional volatility, configurable implementation volatility
unknown    -> low risk, lower confidence
```

Supporting inputs:

- explicit `volatility` in `archfit.yaml`;
- git churn as supporting evidence only;
- roadmap or known migration tags later.

Churn alone must not decide business volatility. It can indicate accidental volatility caused by poor design, or accidental involatility caused by fear of change.

#### Direction and centrality measurement

What it measures:

- afferent coupling: incoming dependencies;
- efferent coupling: outgoing dependencies;
- fan-in/fan-out;
- instability: `Ce / (Ca + Ce)`.

How to use:

- report as graph context;
- increase attention on volatile hubs;
- do not judge design quality from counts alone.

A single intrusive outgoing dependency can matter more than many stable contract consumers.

#### Change locality measurement

What it measures: how predictable the change scope appears.

V1 inputs:

- changed files and modules;
- new dependency edges;
- cycles;
- graph reach from changed nodes.

Future inputs:

- git co-change;
- repeated multi-module edits;
- historical blast radius;
- agent token/iteration cost for comparable tasks.

This is the economic bridge to AI-agent cost. Track it as calibration data, not as a v1 gate.

#### Runtime and lifecycle measurement

What it measures:

- synchronous runtime dependency across boundaries;
- asynchronous integration;
- shared deploy/test/release lifecycle;
- resilience patterns around cross-service calls.

V1 approach:

- config-assisted or adapter-provided only;
- report as distance evidence when available;
- do not require runtime analysis for core gates.

### 10.3 Balanced Coupling classification

For every cross-boundary relationship, classify the relationship using Vlad's dimensions:

```text
relationship = source component + target component + abstraction level
strength     = contract | model | functional | intrusive
distance     = same_package | same_module | cross_module_same_owner | cross_module_different_owner | cross_deploy_unit
volatility   = low | medium | high | unknown
explicitness = explicit | implicit
```

Severity mapping:

```text
critical: high strength + high distance + high volatility
medium:   high strength + high distance + low/unknown volatility
medium:   low strength + low distance + high volatility
low:      high strength + low distance, usually high cohesion
low:      low strength + high distance, usually loose coupling
```

This is the executable form of the review method. It should produce finding classifications and counts, not a single Balanced Coupling score.

### 10.4 V1 metrics

#### Encapsulation ratio

Headline metric for v1.

Definition:

```text
encapsulation = contract_cross_boundary_edges / (contract + intrusive)_cross_boundary_edges
```

The denominator counts only edges that take a stance on boundary respect:
`contract` (goes through a published interface) and `intrusive` (reaches into
internals). `functional` (calling a public function) and `model` (using a public
data type) are normal public coupling — neither a contract nor a leak — and
`unknown` is absence of evidence; all three are excluded from the denominator
rather than counted against the score. Counting functional/model would crush the
ratio for any codebase that mostly calls public functions (the common case) and
manufacture a false `critical` once symbol-level strength (SCIP) is available.

Degenerate cases:

```text
no cross-boundary edges at all          → value 1.0 (vacuously encapsulated)
cross-boundary edges, none classifiable → n/a (indeterminate; band "n/a", not critical)
```

The second case is common for projects that declare no public/internal API
surface (e.g. a Python repo without `public:`/`internal:` globs and before
language-level strength inference applies). Reporting a high-confidence 0/critical
there is a false alarm; `n/a` is the honest result. Confidence scales with the
classified fraction of cross-boundary edges, so the band cap prevents over-claiming
a good band on a thin classified sample.

Why it matters: it is the closest coverage-style metric. It has a clear numerator and denominator. It tells whether cross-boundary coupling goes through explicit contracts instead
of leaking through internal details.

Gate shape:

```text
warn if encapsulation decreases in check mode
warn or fail only after human calibration of absolute thresholds
```

#### Unbalanced-edge count

Definition:

```text
unbalanced_edge = high_strength + high_distance + high_or_declared_volatile
```

Report counts by severity and status:

```text
new_high
new_medium
baseline_high
excepted_high
expired_exception
```

Gate shape:

```text
fail or warn on new high-severity unbalanced edges, depending on policy
```

Balanced Coupling is most useful here: it explains why a relationship is risky and how to rebalance it. It is classification and severity, not a single opaque score.

#### Cycle count

Definition:

```text
cycles among modules or configured layers
```

Gate shape:

```text
fail on new cycles when configured
```

#### Change locality indicator

Definition:

```text
changed_modules, new_cross_module_edges, graph_reach_from_changed_nodes
```

Gate shape:

```text
warn on expansion of change blast radius
```

This is a predictor of future agent token burn and human change cost. It needs calibration against real tasks.

#### Coverage and confidence

Definition:

```text
coverage = extracted_files_and_edges / applicable_files_and_edges
confidence = evidence quality after missing, failed, or partial tools
```

This metric does not judge architecture. It tells agents and humans how much to trust the findings.

#### Map staleness

Definition:

```text
now - reviewed_at for modules, rules, subdomains, and exceptions
```

Why it matters: `archfit.yaml` is a social artifact. The domain can drift even when code conforms to the old map. Stale architecture intent should be visible.

Gate shape:

```text
warn when map_review.stale_after is exceeded
fail only for expired exceptions or explicitly required reviews
```

### 10.5 Semantic advisory research track

Static dependency graphs miss high-value implicit coupling: duplicated business rules, parallel validation, shared semantics with no import edge, and accidental reimplementation by
AI agents.

V1 should not depend on an LLM for gates or deterministic metrics. A later semantic advisory track may use an LLM or another semantic analyzer to propose findings for
functional/model coupling. It must follow these rules:

- never produce gate failures;
- never change deterministic metric values unless the finding is converted into explicit config or a confirmed rule;
- include evidence snippets and confidence;
- label output as `semantic_advisory`;
- produce repair tasks only when the architectural constraint is clear;
- be usable in `scan` and CI, not required for deterministic `check`.

This preserves the deterministic core while leaving room to find coupling that static import graphs cannot see.

### 10.6 Later metric candidates

Promote these only after primitive measurements are reliable:

- **Semantic/model coupling:** shared domain terms, shared models, primitive meaning coupling.
- **Temporal coupling:** required execution order and timing constraints.
- **Runtime/lifecycle coupling:** sync calls, deploy coupling, resilience gaps.
- **Centrality/instability:** afferent/efferent graph context for volatile hubs.
- **Architecture fitness coverage:** how much intended architecture is enforced by executable rules.
- **Agent cost correlation:** token usage, context size, repair iterations, and task failure rate versus graph risk.

---

## 11. Baseline and exceptions

### Baseline

`.archfit/baseline.json` stores accepted current graph state and findings.

Default `check` behavior:

```text
review new or changed architecture facts since baseline/base ref
```

This avoids flooding teams with legacy issues.

Finding status values:

```text
new
baseline
excepted
expired_exception
fixed
```

### Exceptions

Exceptions are explicit accepted risk, not a quiet ignore list. They should add deliberate human friction where an agent would otherwise optimize for the fastest green check.

Required fields:

```yaml
rule: rule-id
from: path-or-module
to: path-or-module
reason: text
approved_by: user
expires: date
```

Rules:

- exceptions require a reason, approver, and expiry;
- expired exceptions fail if the matching rule is `gate: fail`;
- `scan` reports exception inventory and age;
- agents may suggest an exception only as a last resort, never create or approve one silently.

### Architecture map review

`archfit.yaml` can drift from domain reality. A green check only means code conforms to the current map, not that the map is still correct.

Use `reviewed_at`, `reviewed_by`, and `map_review.stale_after` to surface stale modules, subdomains, volatility tags, and rules. Staleness is usually a warning, but it is a strong
signal for consulting, architecture review, or map-refresh work.

---

## 12. Output formats

### Rich JSON

Source of truth for agents and integrations.

This output is an agent contract, not a log dump. It must be stable, structured, and explicit. Agents should not need to scrape console text.

Top-level shape:

```json
{
  "schema_version": "archfit.diagnostic.v1",
  "verdict": "fail",
  "base": "abc123",
  "head": "def456",
  "metrics": [],
  "findings": [],
  "agent_tasks": [],
  "tool_coverage": [],
  "summary": {
    "gate_findings": 1,
    "warnings": 2,
    "exceptions_used": 1
  }
}
```

Every finding that affects the gate or a metric should include:

- stable id;
- finding kind: `gate` or `advisory`;
- status: `new`, `baseline`, `excepted`, `expired_exception`, or `fixed`;
- rule or metric id;
- severity and confidence;
- graph edge evidence;
- module/path matching evidence;
- source locations;
- plain-language reason;
- constraint to preserve;
- allowed alternatives when known;
- repair task;
- validation commands.

### Console

Short local summary. Console output may be terse, but it must point to the JSON file or `archfit explain <finding-id>` for full agent-ready detail.

### Markdown

Human report for `archfit scan` and CI artifacts.

### SARIF

Compatibility output. SARIF is not the source of truth because it cannot carry all architecture-specific context cleanly.

---

## 13. Agent repair task

Every gate finding should include repair constraints. Important advisory findings should include them when a safe local improvement is known.

The repair task is not an auto-fix. It is a precise prompt contract that an AI coding agent can use for a follow-up edit.

Example:

```json
{
  "repair_task": {
    "goal": "Remove checkout dependency on pricing internals.",
    "constraints": [
      "Do not import services/pricing/internal/** from services/checkout/**.",
      "Use services/pricing/contracts/** or add a public contract there.",
      "Preserve current behavior."
    ],
    "files": [
      "services/checkout/discounts.ts",
      "services/pricing/internal/rules.ts"
    ],
    "validation": ["archfit check", "project tests"]
  }
}
```

Do not require an LLM to produce this. The CLI can generate useful repair tasks from rule metadata and graph evidence.

A good repair task avoids architecture interpretation by the agent. It states the allowed design boundary and the validation loop. The agent decides the code edit, then reruns
`archfit check` like it would rerun tests.

---

## 14. Extensibility

Keep extensibility boring.

### Built-in first

The first version should compile built-in extractors, rules, and metric calculators into the binary.

### External tools second

The CLI may run or consume existing tools when available:

- Go: `go-arch-lint` later;
- TypeScript: `dependency-cruiser`;
- Python: `import-linter` / `grimp`.

External tool support is an adapter layer, not the core.

### Process plugins later

After real adapters prove the contract, support process plugins.

Contract:

```text
stdin:  archfit plugin request JSON
stdout: archfit plugin result JSON
```

Plugin kinds:

```text
extractor
rule
metric
output
```

This keeps extension language-agnostic while the built-in core stays simple.

---

## 15. Internal package shape

Go implementation should stay simple.

Suggested packages:

```text
cmd/archfit
internal/config
internal/graph
internal/extract/goextract
internal/extract/tsimport
internal/extract/pyimport
internal/adapters/depcruise
internal/adapters/importlinter
internal/adapters/astgrep
internal/rules
internal/metrics
internal/baseline
internal/output
internal/doctor
internal/tools
```

Guidelines:

- keep graph types concrete;
- keep rule interfaces small;
- pass `context.Context` through tool execution;
- prefer stdlib;
- shell out only for optional external adapters;
- make output schemas versioned;
- test rules and metrics with table-driven tests.

Minimal interfaces:

```go
type Extractor interface {
    Extract(ctx context.Context, repo Repo, cfg Config) (graph.Graph, error)
}

type Rule interface {
    Check(ctx context.Context, g graph.Graph, cfg Config) ([]Finding, error)
}

type Metric interface {
    Calculate(
        ctx context.Context,
        g graph.Graph,
        findings []Finding,
        cfg Config,
    ) (MetricResult, error)
}
```

These are internal interfaces first. Do not freeze them as public plugin API until the core model survives real repos.

---

## 16. MVP build plan

Start with the deterministic core. Calibration runs in parallel; it should improve thresholds and confidence, not block initial development.

### Phase 1 — deterministic core (three languages)

Deliverable: useful local CLI for **Go, TypeScript, and Python** repos. The tool is not useful single-language, so all three
import extractors ship together in v1. Investment priority: **TypeScript and Python first** (the primary audience); Go last
(the simplest, native path).

- `archfit.yaml` parser.
- import extractors for all three languages, behind one `Extractor` contract:
  - **Go** — native, in-process via `go/packages`.
  - **TypeScript** — `dependency-cruiser` adapter (`depcruise --output-type json --ts-config tsconfig.json`), run via `bunx` (preferred) or `npx`.
  - **Python 3.12+** — `grimp` via a bundled PEP 723 helper script, run via `uv run` (ephemeral, no pre-install) or `python3.12` + installed `grimp`.
- module matcher.
- rules: public API only, forbidden dependency, layer direction (one rule model across all three languages).
- baseline.
- exceptions.
- encapsulation metric.
- unbalanced-edge count.
- cycle count.
- coverage/confidence metric (per-language; unresolved imports lower confidence, never fail).
- tool detection (`doctor`): detect each toolchain (go; node + dependency-cruiser; python3.12 + uv/grimp; git), print availability, versions, and install hints.
- JSON and console output.
- exit codes.
- table-driven tests for module matching, rules, baselines, exceptions, metrics, and JSON schema; per-language extractor tests against real fixtures.

Done when:

- a repo can define modules and rules;
- `archfit check` fails on a real boundary violation **in any of Go, TS, or Python**, and passes after it is fixed;
- the same rule model works across all three languages;
- a missing optional toolchain (node/dependency-cruiser, python/grimp/uv) yields a coverage record + install hint and lowered confidence — never a false failure or a silent empty result;
- metric deltas are explainable;
- an agent can read a gate finding and apply the repair constraints.

### Phase 2 — Balanced Coupling classification and scan report

- strength/distance/volatility classifier v1.
- explicitness classification.
- relationship-level findings using Vlad's severity mapping.
- map staleness and exception inventory.
- Markdown report.

Done when:

- high-risk coupling examples are surfaced;
- high-cohesion same-module relationships are not treated as risk;
- every metric explains its inputs;
- `scan` is useful as an architecture audit artifact.

### Phase 3 — semantic fidelity and pattern evidence

(TypeScript and Python moved into Phase 1.) Raise extraction fidelity and add structural pattern evidence, behind the
same contracts:

- **ast-grep** as a `PatternProvider` (structural rule evidence across languages; v1's `Evidence` is empty).
- **real-target / re-export resolution** (build guardrail, §17) via a semantic index: SCIP indexers (`scip-go`,
  `scip-typescript`, `scip-python`) and/or live LSP (gopls, tsgo/typescript, pyright) — the "native → tree-sitter → SCIP"
  fidelity ladder. Defeats re-export shims and barrel files at symbol level.
- **in-process native fallback** via `tree-sitter` (official `go-tree-sitter` bindings): low-fidelity import extraction
  with no external toolchain, so a bare Go binary still produces partial graphs for TS/Python when adapters are absent.

Done when:

- re-export shims and barrel files no longer hide forbidden coupling;
- pattern-based rules can attach evidence to findings;
- the native fallback degrades gracefully (lower confidence, recorded in coverage) when toolchains are missing.

### Phase 4 — agent and CI integration

- SARIF output.
- repair-task JSON.
- GitHub Action wrapper.
- MCP server only if the JSON contract is stable.

Done when:

- an AI agent can read a gate finding, fix the violation, and rerun `archfit check`;
- CI can gate deterministic findings and publish metrics without human interpretation.

### Parallel calibration track

Calibration is practical feedback for thresholds and documentation.

Track:

- deliberate boundary violations and fixes;
- known good module boundaries;
- known legacy exceptions;
- intrusive cross-module dependencies;
- public-contract dependencies;
- partial TS/Python extraction cases;
- optional agent-cost data: changed modules, graph reach, context size, token usage, and repair iterations.

Use calibration to tune bands, severity defaults, and confidence caps. Do not make strong ROI claims until the data supports them.

### Later semantic advisory spike

Functional and model coupling are important in Vlad's methodology and often invisible to import graphs. Keep this as a dedicated later spike, not a hidden dependency of v1.

Evaluate:

- duplicated business rules;
- shared domain model concepts without direct import edges;
- provider/domain models leaking through indirect wrappers;
- false-positive rate on common generic validation or utility logic.

Semantic advisories remain advisory until a human confirms them or turns them into explicit config/rules.

---

## 17. Build guardrails

Keep the first release useful by optimizing for the local feedback loop.

V1 implementation choices:

- collect deterministic graph facts from code;
- make reviewed `archfit.yaml` the source of architecture intent;
- version every metric formula;
- separate delta feedback (`check --base`) from full-repo review (`check --full`, aliased `scan`), as flag-derived modes of one engine;
- preserve baseline findings without blocking new work;
- require expiry and rationale for exceptions;
- keep exception approval as a deliberate human action;
- resolve real dependency targets when possible, so re-export shims and allowed-path wrappers do not hide the same forbidden coupling;
- generate agent-readable diagnostics from rule metadata and graph evidence;
- keep extension seams internal until real adapters stabilize the contract.

**Toolchain, environment, and distribution (multi-language v1):**

- External analyzers run behind the `ToolRunner` choke point and are **auto-detected and optional**: TypeScript needs
  Node + `dependency-cruiser` (run via `bunx`/`npx`); Python needs `python3.12` + `grimp` (run via `uv run`, which
  provisions `grimp` ephemerally, or a system/venv Python with `grimp` installed). Go needs no external tool (native
  `go/packages`).
- **Minimal environment for boundary analysis:** archfit analyses _internal_ module boundaries, so it does **not**
  require installing the target project's third-party dependencies. dependency-cruiser resolves intra-repo and
  tsconfig-aliased imports without a full install (only bare/workspace specifiers need `node_modules`); grimp resolves
  intra-package imports from source on `sys.path` with external packages excluded.
- **Distribution:** the archfit binary is pure Go; a `scratch`/static image runs the Go path out of the box but cannot
  execute Node or Python. Full TS/Python analysis requires those toolchains in the environment (developer host, CI, or a
  fatter published image that bundles them). A missing toolchain produces a coverage record + install hint and lowers
  confidence — it never fails silently.

LLMs can explain or repair findings. Optional semantic passes can propose advisory coupling findings. Deterministic code and policy decide gate verdicts and deterministic metrics.

A green `archfit check` means the change conforms to the configured architecture checks. It does not mean the code is correct, secure, performant, simple, or well tested.

---

## 18. Validation checklist and open questions

Use this section to track what is known enough to implement and what must be answered later.

### V1 validation checklist

The v1 deterministic core is useful when:

- `archfit check` fails on a deliberate configured boundary violation;
- `archfit check` passes after the violation is fixed;
- encapsulation delta is explained by concrete edge changes;
- unbalanced-edge findings include strength, distance, volatility, and explicitness;
- high-cohesion same-module relationships are not reported as drift;
- unresolved imports reduce confidence instead of creating false failures;
- missing optional adapters show install hints and confidence impact;
- an AI agent can use the JSON finding to repair a violation and rerun the check without extra architecture interpretation.

### Initial implementation decisions

- `check` is the only analysis command; delta-first by default (`--base`).
- full-repo review is `check --full`, aliased as `scan` (not a separate command).
- Go uses native `go/packages` first.
- TypeScript starts with static imports and path aliases, with dependency-cruiser as the first adapter candidate.
- Python starts with static AST imports, with import-linter/grimp as the first adapter candidate.
- `contract` and `intrusive` strength detection are v1 reliable signals.
- `model` and `functional` strength detection start as config-assisted or lower-confidence signals.
- `owner` and `deploy_unit` are v1 config fields because they materially affect distance.
- Final product and binary name is `archfit`; `archfit.yaml` and `.archfit/` are the default local files.

### Open questions

Track these explicitly instead of hiding them in the implementation:

- Exact severity defaults for unbalanced edges after Vlad reviews real findings.
- Whether `same owner but different module` should be medium or low distance by default.
- Whether `different deploy_unit but same owner` should be high distance by default.
- How to model generic subdomains with low functional volatility but high provider-switching volatility.
- How much TypeScript path resolution to implement natively before relying on dependency-cruiser.
- Whether Python v1 should prefer native AST extraction or grimp/import-linter when available.
- How to detect allowed-path wrappers or re-export shims reliably without full symbol resolution.
- Whether semantic advisory should be LLM-based, pattern-based, or omitted from the first public release.
- How to measure agent token/context cost without making it a v1 product claim.

---

## 19. References

Use these references to calibrate concepts, metric formulas, tool adapters, and agent output. Keep them as source material, not as runtime dependencies.

### Methodology

- Vlad Khononov, [**Balancing Coupling in Software Design**][bc-book] — primary methodology source for integration strength, distance, volatility, balance, and rebalancing.
- Vlad Khononov, [**Learning Domain-Driven Design**][lddd-book] — reference for subdomains, bounded contexts, integration patterns, and volatility classification.
- `https://coupling.dev/posts/core-concepts/` — concept index for complexity, modularity, coupling, and balance.
- `https://coupling.dev/posts/dimensions-of-coupling/integration-strength/` — strength levels: intrusive, functional, model, contract.
- `https://coupling.dev/posts/dimensions-of-coupling/distance/` — technical, socio-technical, lifecycle, and runtime distance.
- `https://coupling.dev/posts/dimensions-of-coupling/volatility/` — DDD subdomains and essential vs accidental volatility.
- `https://coupling.dev/posts/related-topics/connascence/` — static/dynamic connascence detail for future strength refinements.
- `https://coupling.dev/posts/related-topics/afferent-and-efferent-coupling/` — fan-in/fan-out context, with the warning that counts alone miss dependency meaning.

### Local methodology and architecture-review references

- `https://github.com/vladikk/modularity` / `/Users/alexei/Workspace/modularity` — Claude Code plugin and reference implementation of the review/design workflow.
- `/Users/alexei/Workspace/modularity/skills/balanced-coupling/SKILL.md` — local Balanced Coupling summary and severity mapping.
- `/Users/alexei/Workspace/modularity/skills/balanced-coupling/details.md` — implicit/explicit coupling, connascence, and DDD pattern mappings.
- `/Users/alexei/Workspace/modularity/skills/review/SKILL.md` — human/LLM architecture-review workflow this CLI should make more deterministic.
- `/Users/alexei/Workspace/architect/src/skills/architecture-review/SKILL.md` — evidence-first architecture-review procedure.
- `/Users/alexei/Workspace/architect/src/skills/methodology-balanced-coupling/SKILL.md` — Balanced Coupling review rubric used by Alexei's architect skills.
- `/Users/alexei/Workspace/architect/src/skills/methodology-architecture-fitness/SKILL.md` — executable architecture-fitness framing.
- `/Users/alexei/Workspace/architect/src/templates/scorecard.yaml` — scorecard conventions and confidence discipline.
- `/Users/alexei/Workspace/architect/docs/tools.md` — tool coverage states and confidence impact.

### Tool references for v1 and calibration

- `golang.org/x/tools/go/packages` — Go package/import extraction for the built-in Go extractor.
- `dependency-cruiser` — TypeScript/JavaScript dependency graph and boundary-rule adapter candidate.
- `import-linter` and `grimp` — Python import graph and architecture-contract adapter candidates.
- `ast-grep` — targeted structural pattern checks when path/import rules need syntax evidence.
- `git` / `git log` — baseline diff, changed files, churn, and simple change-locality evidence.
- GitNexus — optional calibration adapter for co-change, churn hotspots, and historical blast radius.
- Tree-sitter — later syntax fallback if native scanners and adapters do not provide enough structural evidence.
- SCIP — later symbol graph format when file/package-level dependency facts are insufficient.
- codegraph — calibration/reference tool for richer semantic graph evidence, not a v1 runtime requirement.

### Reference-use rule

References should improve one of the product primitives: architecture graph facts, rule enforcement, metrics, confidence, or agent repair diagnostics. Do not add references for
generic code quality unless they directly support an architecture rule or metric.

[bc-book]: https://www.informit.com/store/balancing-coupling-in-software-design-universal-design-9780137353484
[lddd-book]: https://www.oreilly.com/library/view/learning-domain-driven-design/9781098100124/

---

## 20. Summary

`archfit` starts as a deterministic architecture feedback loop.

It tells an agent:

```text
Your change crossed this architecture boundary.
Here is the exact evidence.
Here is the metric impact.
Here is the constraint to repair it.
Rerun this check after the fix.
```

The extension path grows from the same core contracts: architecture graph, explicit rules, versioned metrics, baselines, exceptions, and agent-first architecture diagnostics.
