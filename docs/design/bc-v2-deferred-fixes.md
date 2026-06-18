<!-- markdownlint-disable MD024 MD031 -->

# Deferred BC-v2 fixes — implementation design

Date: 2026-06-18. Status: **DONE** — both fixes implemented on branch
`docs/bc-measurement-v2`. Fix A (flat-name distance precedence): commit `febed17`.
Fix B (engine→labels split): commit `f2719fc`. The optional step B6 (`PairEvidence`
move) was intentionally left out to keep scope tight. Two defects from the
self-analysis vs expert-review comparison (`docs/bc-v2-self-vs-expert-comparison.md`)
were deferred because they are high-blast and shouldn't be rushed. This is the
concrete, implementable design for each (advisor + Perplexity grounded). The other 6
defects were already fixed on the same branch.

---

## Fix A — flat-name distance (composite-precedence redesign)

### Problem

`codeStructureDistance(fromMod, toMod)` returns `SameOwner` for two flat
single-segment module names (`depth=1, shared=0` satisfies `shared >= depth-1`),
systematically under-distancing repos that name modules flat (`core`, `api`,
`worker`). A naive `depth==1 → DiffOwner` guard alone is wrong because it interacts
with `isDegenerateOwnerMap`: an **explicit** all-same-owner config is indistinguishable
from the **git-author** single-author degenerate case, so it gets suppressed and the
code-structure default (now `DiffOwner`) overrides the user's explicit `owner:
same-team` via `maxDistance` (max takes the worst case).

Root cause: `maxDistance(code_structure, ownership, deploy)` treats the structural
_default_ as a peer of _explicit_ config. Explicit config must take **precedence**.

### Design

1. **Track explicit owners.** Add to `config.Config` (config.go):

   ```go
   explicitOwners map[string]bool // module → owner was hand-authored in YAML
   ```

   Populate it in `Load`, right after YAML decode and BEFORE any resolver fill:

   ```go
   c.explicitOwners = make(map[string]bool)
   for name, def := range c.Modules {
       if def.Owner != "" { c.explicitOwners[name] = true }
   }
   ```

   `FillMissingOwners` (the git-author/CODEOWNERS resolver, called later in the
   pipeline) must NOT write to this map — so anything in `explicitOwners` is
   hand-authored; anything with `Owner != ""` but absent from `explicitOwners` is
   resolver-filled.

2. **Thread it into classify.** Add `ExplicitOwners map[string]bool` to
   `ClassifyConfig`; set it in `ForClassify()` from `c.explicitOwners`. Pass it into
   `classifyDistance`.

3. **Precedence chain** — replace the `maxDistance(...)` call in `classifyDistance`
   (classify.go) with:

   ```go
   deploy := deployDistance(fromDef.DeployUnit, toDef.DeployUnit)
   if deploy == coupling.DistanceCrossDeployUnit {
       return deploy // a deploy boundary is absolute
   }
   // Explicit hand-authored ownership on EITHER endpoint is authoritative — the
   // user told us something, so don't drop it to the code-structure fallback.
   // One-sided: ownershipDistance compares the explicit owner against the other's
   // owner (explicit or resolver-filled) as usual.
   if explicitOwners[fromMod] || explicitOwners[toMod] {
       return maxDistance(ownershipDistance(fromDef.Owner, toDef.Owner), deploy)
   }
   // Real resolver ownership signal (>=2 distinct owners) is authoritative too.
   if !isDegenerateOwnerMap(owners) {
       return maxDistance(ownershipDistance(fromDef.Owner, toDef.Owner), deploy)
   }
   // Fallback: git-author degenerate or no ownership → code structure.
   return maxDistance(codeStructureDistance(fromMod, toMod), deploy)
   ```

4. **Flat-name guard** in `codeStructureDistance` (distance_structure.go) — now safe
   because explicit same-owner is handled before reaching it:
   ```go
   if len(fromParts) == 1 && len(toParts) == 1 {
       return coupling.DistanceCrossModuleDiffOwner
   }
   ```

### Test impact / the seam to solve first

Tests construct `config.Config{Modules: ...}` directly, **bypassing `Load`**, so
`explicitOwners` is nil and explicit-owner fixtures would misbehave. Before
implementing, add a test-friendly seam — pick one:

- a small exported constructor/setter `config.WithExplicitOwners(...)` used by tests, or
- have `ForClassify` treat any module whose `Owner != ""` as explicit **when
  `explicitOwners` is nil** (i.e. no resolver ran), which is exactly the test case.

**Prefer option 1** (`WithExplicitOwners`) — safer API contract, no silent
inference that could misclassify a resolver-filled config reaching classify via a
path that skipped `explicitOwners` init.

Then update: `classify_test.go` (the "cross-module same-owner" fixture should pass via
the explicit-owner branch; flat-name degenerate fixtures now expect `DiffOwner`) and
`distance_structure_test.go` (single-segment cases → `DiffOwner`). Verify
`TestArchImports` and `TestGolden` (archfit's own config is hierarchical, so golden
should be unaffected — confirm).

### Files

`internal/config/config.go` (field + `Load` + `ForClassify` + `ClassifyConfig`),
`internal/classify/classify.go` (`classifyDistance` precedence chain),
`internal/classify/distance_structure.go` (`codeStructureDistance` guard),
`internal/classify/classify_test.go`, `internal/classify/distance_structure_test.go`.

---

## Fix B — engine→labels boundary (package split)

### Problem

`internal/engine` imports `internal/labels` for the pure functions `Key`,
`Approved`, `HashItems` (and the `Label` type) — but `labels` also contains `Load`
(os + go-yaml), so the I/O dependency rides into the engine's import closure. The
arch test does not guard this boundary, so it is unenforced drift.

### Design

1. **Create `internal/labels/labelsio`** (an adapter package). Move `Load` there:

   ```go
   package labelsio
   import ( "errors"; "fmt"; "os"; "github.com/goccy/go-yaml"
            "github.com/alexei-led/archfit/internal/labels" )
   func Load(path string) ([]labels.Label, error) { /* body unchanged, but use
       labels.File / labels.Label / labels.ValidStrength / labels.Status* */ }
   ```

   (`Load`'s validation used the unexported `validStrengths` map — call the exported
   `labels.ValidStrength` instead.)

2. **Strip `labels.go`** to pure logic: remove `Load` and the `os`/`errors`/`fmt`/yaml
   imports. `Key`, `Approved`, `HashItems`, `ValidStrength`, and the `Label`/`File`
   types stay (struct yaml tags do not require importing yaml). Move the two `Load`
   tests from `labels_test.go` into `labelsio_test.go`.

3. **Point cmd at `labelsio`**: `cmd/archfit/pipeline.go` and `cmd/archfit/enrich.go`
   change `labels.Load(...)` → `labelsio.Load(...)` and import `labelsio`. The engine
   keeps importing the now-pure `internal/labels`.

4. **`go mod tidy`** — the split surfaced a `gopkg.in/yaml.v2` "implicitly required
   module" error during this session (the module graph shifts when `labels` stops
   importing goccy/go-yaml directly). Run `go mod tidy` after the move; this is the
   step that wants a clean fresh run (it is why the attempt was reverted).

5. **Enforce the boundary**: in `internal/arch_test.go` add
   `modulePrefix + "internal/labels/labelsio"` to `adapterPrefixes`, plus a subtest
   asserting the engine (and the core ring) never import `labelsio`. Then add the rule
   to archfit's own `.archfit.yaml` so the gate catches regressions:

   ```yaml
   - id: engine_no_labelsio
     type: forbidden_dependency
     from: internal/engine/**
     to: internal/labels/labelsio/**
     gate: fail
   ```

6. (Optional) `PairEvidence` (engine.go) is exported only for `cmd/enrich`'s benefit —
   an inverted dependency. It uses only `graph`, `config.ModuleMap`, and pure
   `labels` helpers, so it can move to `internal/labels` and stop being an engine
   export. Not required for the boundary fix; do it if cleaning F-TEST-1 too.

### Files

`internal/labels/labelsio/labelsio.go` (+ test), `internal/labels/labels.go`,
`internal/labels/labels_test.go`, `cmd/archfit/pipeline.go`, `cmd/archfit/enrich.go`,
`internal/arch_test.go`, `.archfit.yaml`. Finish with `go mod tidy` + `make all`.

---

## Verification (both fixes)

`make all` (fmt → lint → test → build), `go test -race ./...`,
`go test ./internal/ -run TestArchImports`, `go test ./internal/engine/ -run
TestGolden` (regenerate deliberately only if output legitimately changes), and the
determinism spot-check (`archfit check` twice → byte-identical). For Fix A, re-run
`archfit check` on a flat-named sample repo to confirm distance now discriminates.
