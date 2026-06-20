# Architecture Methodology Comparison — Research Notes

Generated: 2026-06-20

---

## 1. Vlad Khononov — Balanced Coupling (Book + coupling.dev)

### Beyond the Binary Blog Model

The 2024 book "Balancing Coupling in Software Design" extends the earlier binary blog
model into a continuous, quantified framework. Instead of "coupled / not coupled," every
dependency is scored across three dimensions, producing a numeric risk signal per
relationship that can be ranked, compared, and tracked over time.

Key additions over the blog model:
- Three-dimensional representation: integration strength, distance, volatility
- Composite score to prioritize design work and compare alternatives
- Continuous scoring (budgetable) rather than hard "coupled/decoupled" thresholds
- Integration with DDD subdomains, team topology / Conway's Law, and Wardley Maps

### Measuring the Three Dimensions

**Integration strength** — how much knowledge two components must share to interact.
Khononov collapses classical coupling varieties (data, control, temporal, content) into
a single "knowledge sharing" notion. Estimated on a discrete scale (~1–5) by structured
judgment:
- Very low: stable generic protocols (HTTP, S3 events, minimal schema), no shared domain concepts
- Mid: shared domain data schemas, reasonably stable API, consumer doesn't need internal state knowledge
- High: consumer must know internal workflow, state transitions, call ordering, or fine-grained ops
- Signals: call-ordering dependency → high; coarse-grained self-contained API → low

**Distance** — how hard it is for teams to coordinate and evolve the integration.
Factors:
- Same team / same repo / same deployment unit → low distance
- Same org, different team, separate services → medium
- Different orgs, vendors, cross-time-zone → high
Assessment is a checklist-based numeric scale, adaptable per team.

**Volatility** — business-driven likelihood of change over the relevant horizon.
Assessed via DDD subdomain classification and Wardley mapping:
- Core domain → high volatility (active innovation, frequent model changes)
- Supporting subdomain → medium (tools / internal processes evolve)
- Generic subdomain → low (commodity, stable interfaces)
- Wardley genesis/custom → high; product → moderate; commodity/utility → low

**Why NOT commit history for volatility:**
Commit history reflects past events, not future business intent. A module that changed
heavily may now be stable post-rewrite; a quiet but newly-strategic area may be about
to change. Commit frequency is also polluted by refactoring waves, reformatting, and
org changes unrelated to domain volatility. Volatility must be assessed from product
stakeholders and domain/strategy maps.

### The Formula

coupling.dev markets "one formula and three dimensions." The exact formula is not
published verbatim, but the described pattern is:
- Assign numeric scores (e.g., 1–5) to each of strength, distance, volatility
- Combine them into a composite coupling risk score (multiplicative or weighted sum)
- High-risk scenario: high strength × high distance × high volatility together
- Model is intentionally lightweight and adjustable, not a rigid academic metric

### Critiques / Limitations

- Semi-quantitative; relies on human workshop judgment — different teams may score same
  boundary differently; scores can drift without documented criteria
- No universal canonical formula — marketed as context-adaptable, not standardized
- Requires DDD + Wardley Maps literacy; without those, teams fall back to gut feeling
  or commit history (which Vlad explicitly warns against)
- Limited empirical validation in academic literature — more practical synthesis than
  a validated academic metric
- Risk of treating composite number as ground truth rather than a discussion starter

### Sources

- https://coupling.dev
- https://vladikk.com/page/books/
- https://www.oreilly.com/library/view/balancing-coupling-in/9780137353514/
- https://se-radio.net/2025/04/se-radio-662-vlad-khononov-on-balancing-coupling-in-software-design/
- https://www.infoq.com/podcasts/balancing-coupling-software-design/
- https://www.youtube.com/watch?v=_9vi70yFBDg

---

## 2. Architecture Fitness Functions (Ford, Parsons, Sadalage)

### Precise Definition

"An architectural fitness function provides an objective integrity assessment of some
architectural characteristic(s)." — Building Evolutionary Architectures

A fitness function is any mechanism (test, metric, monitoring, manual step) that
measures whether the system still satisfies an architectural concern. Key properties:
- Objective: measurable outcome (boolean, threshold, score, rate, count)
- Architectural focus: checks integrity of an architectural concern, not just unit behavior
- Any mechanism: tests, static analysis, monitoring alerts, even manual governance steps

### Categories

**Atomic vs Holistic**
- Atomic: assesses a single architectural dimension in isolation (e.g., "avg latency < 200ms for service X", "no cyclic deps between modules in layer Y")
- Holistic: runs against a shared context, exercises a combination of aspects (e.g., chaos test that checks resilience, timeout, and fallback across many services simultaneously)

**Triggered vs Continuous**
- Triggered: runs on events — commit, PR, nightly build, manual trigger (CI-integrated tests, static analysis, scheduled perf suites)
- Continuous: runs at short intervals or permanently in production (monitoring, dashboards, SLO alerts, ongoing security posture evaluation)

**Static vs Dynamic**
- Static: uses static artifacts — code, config, schemas; does not execute the system (AST analysis, linting, dependency graphs, import rules)
- Dynamic: requires running system; observes runtime behavior (perf tests, chaos experiments, latency p99, availability under zone failure)

### Encoding Architectural Intent as Executable Checks

1. Identify architectural dimensions: performance, security, reliability, coupling, compliance, operability, cost
2. Make intended state measurable: define thresholds, rules, or trends
   - "No synchronous call chains deeper than 3 services"
   - "All external calls must go through API gateway"
3. Implement using existing mechanisms: tests, static analysis, monitoring + alerts
4. Unify disparate checks under one conceptual framework for governance
5. Continuously execute and evolve as architecture and business goals change

Key analogy: "Test-driven development allows developers to design and verify as they
go; fitness function-driven architecture provides the same fast feedback, but on
architecture rather than design." — Neal Ford

### Relationship to CI Gates

- Triggered static fitness functions act as CI gates (build-breaking): failing layering
  rule, forbidden import, cyclomatic complexity threshold
- Triggered dynamic functions run at pipeline stages: merge-to-main perf suites
- Continuous dynamic functions act as production monitoring gates (alerting, auto-rollback)
- The net effect: distributed, automated architectural governance embedded in CI/CD,
  replacing central architecture review boards with executable policy

### Known Limitations

1. Not everything is automatable: some concerns are qualitative or context-specific
   (UX, strategic alignment); book explicitly allows manual processes for regulatory reasons
2. Tunnel vision / over-optimization: easy-to-measure dimensions get over-engineered;
   hard-to-measure ones get neglected
3. Complexity and maintenance: non-trivial to implement meaningfully; must evolve with
   architecture or become stale and misleading
4. False sense of security: passing fitness functions only covers selected dimensions,
   not architectural reality
5. Social challenges: teams must agree on dimensions and thresholds; culture shift required
   to move from review-board governance to automated gates
6. Granularity tradeoff: too many fine-grained → pipelines slow and brittle; too few
   coarse holistic → hard to diagnose which aspect failed
7. Not all tests are fitness functions: labeling too broadly dilutes the concept

### Sources

- https://nealford.com/books/buildingevolutionaryarchitectures.html
- https://nealford.com/downloads/Evolutionary_Architectures_by_Neal_Ford.pdf
- https://www.thoughtworks.com/en-us/insights/articles/fitness-function-driven-development
- https://www.infoq.com/articles/fitness-functions-architecture/
- https://aws.amazon.com/blogs/architecture/using-cloud-fitness-functions-to-drive-evolutionary-architecture/
- https://mikaelvesavuori.se/blog/2023-08-20_The-Up-and-Running-Guide-to-Architectural-Fitness-Function

---

## 3. Deterministic Static Analysis vs LLM-Based Architecture Review — Reliability

### Reproducibility and Determinism

Static architecture tools are deterministic: same codebase + rules → same result every
time. This is what makes them suitable for auditable CI gates.

LLMs are inherently stochastic: sampling + approximate training objective means even
temperature-0 runs can vary due to floating-point non-associativity, model updates, and
context window ordering effects. Model version upgrades can silently change past
"verdicts" — a layering violation judged OK before might be flagged now.

### False Positives and Hallucinations on Code-Structure Reasoning

From 2023–2026 evidence (none yet is a dedicated "architecture conformance by LLM"
benchmark, but adjacent evidence is consistent):
- Deterministic checks (exact entity/number matching) achieve near-zero false positives
  on the constraints they encode; LLM judges trade precision for semantic flexibility
- LLMs asking "are there dependency cycles between package A and B?" on large codebases
  are fragile: partial context, hallucinated missing links, missed existing ones
- EoG architecture paper (arXiv 2601.17915) explicitly notes "LLMs are weak at guaranteed
  exhaustive graph reasoning" — separates LLM local abductive reasoning from deterministic
  global graph traversal and belief propagation
- General LLM code review studies: LLMs can detect logic/semantic bugs but miss large
  fractions of issues found by static analyzers (null-safety, dataflow, concurrency);
  raise false positives by misreading API contracts

### Where LLMs Add Value

- Semantic / intent judgment: does this design align with high-level principles? Does
  this module mix concerns in a way that smells wrong? (Closer to NLU than to graph ops)
- Heuristic pattern recognition: noticing unnamed smells without explicit rules
- Naming and narrative: proposing meaningful module/layer names; generating ADRs,
  architectural overviews, and explanations for different audiences
- Subjective quality: "Is this modularization easy to understand?"; "Does this layering
  feel cohesive?"; "Is this design consistent with hexagonal architecture?"
- Post-processing static facts: explaining why a violation matters, suggesting refactors,
  clustering components into likely bounded contexts from naming/usage patterns

### Where Static Tools Win

- Exhaustive graph facts: complete dependency graphs, all cycles, all forbidden deps —
  static analyzers operate on full ASTs and type graphs, not chunked prompts
- Repeatable gates and policy enforcement: CI/CD must be predictable and auditable;
  stochastic LLM decisions are incompatible with hard build gates
- Regression testing: tracking metric trends across commits requires identical computation;
  LLM answers drift with model version, prompt, and context window

### Emerging Hybrid Pattern (2023–2026 consensus)

Consistently across hallucination-detection pipelines, graph-reasoning architectures, and
LLM-powered applications:
- Level 1 deterministic (cheap, precise) → Level 2 LLM semantic check → Level 3 human calibration
- EoG: LLMs handle local abductive reasoning; deterministic engines handle global graph
  consistency and belief propagation
- LLM application architectures: multiple deterministic protective layers (validation,
  filters, monitoring) surround models to "turn an unpredictable model into a reliable product"

For architecture review specifically:
1. Static analysis layer: build dependency graphs, detect cycles, measure coupling,
   enforce rules → produce structured facts/metrics/violations
2. LLM interpretation layer: ingest static facts + selected code snippets → explain
   issues in natural language, suggest refactorings, assess against principles
3. Deterministic gating: CI/CD fails only on deterministic violations; LLM output is
   advisory, reviewed by humans

### Sources

- https://dev.to/anshd_12/deterministic-vs-llm-evaluators-a-2026-technical-trade-off-study-11h
- https://arxiv.org/html/2601.17915v2 (EoG: LLM + deterministic graph traversal)
- https://www.craigrisi.com/post/the-architecture-of-llm-powered-applications-how-it-differs-from-conventional-software-architecture
- https://www.diplomacy.edu/blog/beyond-the-llm-hype-why-the-real-ai-breakthrough-is-deterministic-systems/
- https://blog.gopenai.com/llms-vs-deterministic-logic-overcoming-rule-based-evaluation-challenges-8c5fb7e8fe46
- https://magazine.sebastianraschka.com/p/llm-research-papers-2026-part1
- https://www.sciencedirect.com/science/article/pii/S089543562600096X

---

## 4. Deterministic Architecture-Rule Tools

### dependency-cruiser (JS/TS)
Enforces file/folder-level import rules, forbidden/allowed dependencies between modules
and layers, detects circular dependencies and unused modules. Rules encoded as declarative
JSON/JS config with path-pattern filters. Cannot express runtime/behavior constraints,
semantic domain policies, or cross-language coupling.
URL: https://github.com/sverweij/dependency-cruiser

### import-linter (Python)
Enforces import-based architectural contracts: layers, independence, forbidden imports.
Rules are declarative `.ini` contracts by Python package patterns. Cannot enforce
intra-module class-level constraints, runtime wiring, or multi-language architecture.
URL: https://github.com/seddonym/import-linter

### ArchUnit (Java/JVM)
Enforces architectural and coding rules from Java bytecode: layered architectures, package
dependencies, naming conventions, annotation usage, allowed component dependency directions.
Rules written as executable Java/Kotlin tests with a fluent API in the test suite. Cannot
see cross-language or non-JVM constraints, or dynamic DI wiring beyond bytecode visibility.
URL: https://www.archunit.org

### Structure101 (multi-language, commercial)
Enforces layering and dependency rules at package/module/component level via visual
architecture diagrams; flags violations and tangles. Rules encoded as structural models
in the UI. Cannot express intra-class rules, semantic domain policies, or constraints
beyond static structural dependencies.
URL: https://structure101.com

### NDepend (.NET)
Enforces .NET dependency rules, layering constraints, cyclomatic complexity thresholds,
and custom metrics via CQLinq (LINQ-like queries over a static code model). Powerful for
metrics-based and cross-layer enforcement. Cannot express deployment topology, runtime
config constraints, or behavior not in static metadata.
URL: https://www.ndepend.com

### Konsist (Kotlin)
Enforces structural/architectural rules for Kotlin projects: package/module boundaries,
naming and annotation conventions, folder structure, dependency directions. Rules written
as Kotlin tests with a fluent API. Cannot cover non-Kotlin parts of a system, dynamic
wiring, or semantic/business-level policies.
URL: https://github.com/Lasa-Konsist/konsist

### Reflexion Models (academic)
Conformance between conceptual and implemented architecture: map source entities to
high-level components, check dependency graph against intended architectural relations.
Detects convergences (unexpected), divergences (missing), and absences. Cannot express
intra-component fine-grained rules, temporal/runtime behavior, or non-structural properties.
Reference: Murphy, Notkin, Sullivan, "Software Reflexion Models: Bridging the Gap Between
Source and High-Level Models."

### Dependency Structure Matrix (DSM)
Represents dependencies as a matrix; enforces global layering (lower-triangular = no
cycles), clustering/cohesion constraints, cycle absence. Rules captured as allowed/forbidden
regions in the matrix. Cannot express detailed semantic constraints, intra-module rules,
or runtime behavior. Tools: Structure101, Lattix, IntelliJ's DSM view.
Reference: MacCormack et al., "A DSM Approach to Software Architecture."

---

## Sources Summary

All key URLs are embedded per section above. Adversarial note: Perplexity sources were
cross-checked across multiple distinct queries; the LLM-vs-deterministic findings are
convergent across an arXiv paper (2601.17915), a 2026 dev.to study, and engineering
practice from multiple LLM application architecture writeups. Khononov sources are
consistent across SE-Radio, InfoQ, and the book page. Fitness function sources come
directly from Ford/Parsons/Sadalage canonical materials (nealford.com, Thoughtworks,
AWS blog).
