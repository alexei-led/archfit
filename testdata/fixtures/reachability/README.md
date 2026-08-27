# HEALTHY reachability fixture

This is the committed source for the hermetic, Go-only reachability fixture.
`cmd/archfit/integration_reachability_test.go` is its only execution path. The
test copies these files to `t.TempDir()`, creates one deterministic git commit,
renders `.archfit.yaml`, and runs the real `analyze`, `baseline`, and `check`
commands there. Running Archfit against this committed directory would use the
parent repository's history and is invalid.

## File inventory and evidence purpose

| File | Dimension served | Contract fact supplied |
| --- | --- | --- |
| `go.mod` | structure, modularity, complexity, testability | Makes Go the sole applicable language and gives `go/packages` a local module with no downloaded dependencies. |
| `services/app/app.go` | structure, coupling, complexity, testability | Supplies production source and the only cross-module edge, through the API's public interface. |
| `services/api/api.go` | modularity, coupling, complexity, testability | Declares the public `Greeter` contract, giving the edge compiler-derived contract strength without an abstention, plus one local statement so a future Go coverprofile represents this module. |
| `services/app/Dockerfile` | operations, coupling | Independently corroborates the app deploy unit and makes deployment distance explicit. It is never built. |
| `services/api/Dockerfile` | operations, coupling | Independently corroborates the API deploy unit and makes deployment distance explicit. It is never built. |
| `CODEOWNERS` | operations, coupling | Gives each declared module deterministic CODEOWNERS provenance and an explicit owner for distance classification. |
| `.gitignore` | testability | Keeps the rendered config, coverage artifacts, cache, and the deliberately ignored covered source out of git status so freshness is proven from sidecar hashes rather than repository cleanliness. |
| `.archfit.yaml.tmpl` | intent and all declared-module dimensions | Declares two modules and public surfaces, high volatility, distinct deploy units, Go-only analyzer applicability, and one enabled forbidden-dependency rule in the direction the graph does not contain, so the hard gate passes. The test removes the coverage token for Task 3. |
| `README.md` | fixture auditability | Records why every committed file exists, the artifact lifecycle, the observed result, and the JSON envelope. |

The app-to-API edge has contract strength, cross-deploy distance, and high
volatility. That combination is fully scored in the no-advisory band, so the
fixture has coupling evidence without an active diagnostic.

## Artifact lifecycle

The committed directory intentionally has no `.git/`, `.archfit.yaml`,
`.archfit-baseline.json`, coverage artifact, sidecar, cache, or ref file. The
test owns all run artifacts:

1. `materializeFixture(t, false)` copies this directory, initializes git, and
   commits the copied source with fixed identity and timestamps.
2. It renders `.archfit.yaml` from the template with no `coverage:` block.
3. The integration test runs `analyze` twice, then runs `baseline` to persist a
   comparable reference, then re-runs `analyze` and `check`.
4. The `withCoverage=true` path uses only `go test -coverprofile`, writes
   tracked, untracked, and gitignored covered sources, and emits the version-1
   content-hash sidecar described by the evidence contract. Task 10 exercises
   matched, stale, unverified, and warm-cache freshness outcomes through this
   path; Archfit never executes the target tests itself.

## Recorded Task 3 outcome

The observed post-baseline result is **Outcome B-temporary**:

- `analyze` exits 0 because it is report-only.
- `check` exits 2.
- Verdict: `needs_attention`.
- Decision: hard gates `pass`, active blockers 0, unknown dimensions 3, active
  diagnostics 0.
- Measured: `intent`, `structure`, `modularity`, `coupling`,
  `change_locality`, and `drift`.
- Partial: `complexity`, `testability`, and `operations`.
- Unmeasured: none after the persisted baseline is loaded.

The blockers and Task 2 audit remedies are:

| Blocking dimension | Blocking symbol | Outcome class | Remedy class | Remedy |
| --- | --- | --- | --- | --- |
| `complexity` | `complexityDimension` | B-temporary | new collector required | Task 7 supplies complete module-graph depth, fan-in, and fan-out facts. |
| `testability` | `testabilityDimension` | B-temporary | new collector required | Tasks 8-10 ingest, attribute, and freshness-check supplied coverage. |
| `operations` | `operationsDimension` | B-temporary | new collector required | Task 6 retains deploy-unit and ownership provenance and reconciles it per module. |

No B-permanent blocker was observed.

## Recorded JSON envelope

The primary JSON contract is the architecture state **at the document root**.
There is no `architecture_state` wrapper. The observed top-level keys are:

```text
schema_version
verdict
decision
comparison
measurement
dimensions
coverage
findings
agent_tasks
seams
```

`dimensions` is a root child object with the nine named dimension keys. The
persisted baseline affects `dimensions.drift`; the independent root
`comparison.status` remains `not_requested` because this run does not use
`--base`.
