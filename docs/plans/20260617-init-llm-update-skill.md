# archfit: LLM-assisted init/update with --apply live-write, and a portable skill

## Overview

Two new capabilities plus a skill cleanup, built so the LLM can either _suggest_
(safe default = "plan") or _act as the architect_ (opt-in `--apply` = "apply").

1. **`archfit init --llm`** — off-gate, opt-in LLM pass that enriches the
   generated `.archfit.yaml` with `subdomain`, `volatility`, `layer`, and a
   module-name _suggestion_ static heuristics cannot infer.
   - default (plan): suggestions are **commented-inert** YAML lines (review then
     uncomment) — the `init`-time analog of `status: draft`.
   - `--apply`: the LLM's classification is written **live** as real fields.
     Scope: classification only (`subdomain`, `volatility`, `layer`). Coupling
     **rules stay with `enrich`** (they need a dependency graph init lacks).

2. **`archfit update`** — keeps `.archfit.yaml` in sync as the codebase evolves.
   - default (plan): prints a **drift report** + paste-ready stanzas; writes
     nothing to `.archfit.yaml`.
   - `--apply`: applies structural drift **live** (add modules, update paths,
     comment removed) via an AST-located source patcher that leaves hand-authored
     sections (`rules`, `exceptions`, comments) intact.
   - `--llm`: adds LLM classification of unclassified modules to the report; with
     `--apply`, those classifications are written live too — even when the config
     is structurally in sync.

3. **Portable skill** — bring `skills/archfit/SKILL.md` to agentskills.io
   compliance and document the new command modes.

### Mode matrix (the contract)

| Command                | Effect                                                               |
| ---------------------- | -------------------------------------------------------------------- |
| `init`                 | structural scaffold (unchanged)                                      |
| `init --llm`           | + commented-inert subdomain/volatility/layer/name suggestions        |
| `init --llm --apply`   | + subdomain/volatility/layer written **live** (name stays a comment) |
| `update`               | drift report, no write                                               |
| `update --apply`       | structural drift written **live** (add/path/comment-removed)         |
| `update --llm`         | drift report incl. LLM classification of unclassified modules        |
| `update --llm --apply` | structural + LLM classification written **live**                     |

`init --apply` without `--llm` is an error. `--apply` prints a one-line stdout
notice of what it wrote and always uses the strict write protocol (Technical Details).

## Context (from discovery, verified)

- `internal/initcfg/initcfg.go` — `Discover`, `Render` (manual `strings.Builder`),
  `ModuleDef` (discovery-only), `DiscoveredConfig.Layers`. Module names can collide
  across Go/Python/TS; `Render` emits one YAML key per `ModuleDef`.
- Discovered `paths` are heterogeneous: Go `internal/x/**`, TS `src/x/**`, Python
  **dotted** import names (`ccgram.handlers`) — not filesystem globs.
- `cmd/archfit/init.go` — `InitCmd.Output` supports `-` → print to `deps.Stdout` and return.
- `cmd/archfit/main.go` — `appDeps{ Runner toolrun.Runner; Stdout io.Writer }` (no Stderr).
- `cmd/archfit/pipeline.go` — `loadConfig(ctx, path, noConfig)`.
- `internal/config/config.go` — `Load(ctx, path string)` (path-based);
  `config.ModuleDef` has Subdomain/Volatility/Owner/DeployUnit; `DisallowUnknownField()`;
  `layerRank` returns -1 for a layer absent from `layers`; goccy rejects duplicate map keys.
- `cmd/archfit/enrich.go` — off-gate LLM pattern (provider, cache, batch, strict JSON,
  atomic temp+rename, notices to `deps.Stdout`).
- `go.mod` — `github.com/goccy/go-yaml v1.19.2`.

## Non-negotiable invariants (enforced by internal/arch_test.go + CLAUDE.md)

- LLM stays **off-gate**: `check` never calls an LLM. `internal/initcfg` imports neither
  `internal/llm` nor `internal/config` in production code; the `config` projection lives in
  `cmd/archfit/*` (only `initcfg` tests may use `config.Load`).
- All emitted YAML uses only known config keys and round-trips through `config.Load`.
- `--apply` is the only path that writes LLM judgment into gate-feeding fields; always
  explicit + opt-in. `update` plan/default modes leave an existing `.archfit.yaml` untouched;
  `init` writes the generated config (unless `-o -`, which streams to stdout).
- `internal/model/*` stdlib-only; core ring imports no adapters; every subprocess goes
  through `toolrun.Runner`.
- Golden gate (`go test ./internal/engine/ -run TestGolden`) must stay green.

## Development Approach

- **Testing approach**: Regular (code first, then tests); every task ships its tests
  before the next; all tests pass before moving on.
- `init` with no flags stays byte-identical to today (golden-guarded in Task 2).
- Small, focused changes; preserve backward compatibility.
- Update this plan file if scope changes during implementation.

## Testing Strategy

- **Unit tests** required per task; table-driven for input/output matrices.
- Mock only boundaries: fake `llm.Provider`; fake `toolrun.Runner`; temp-dir fixtures
  for filesystem and patcher tests.
- Commands: `make test`, `make lint`, `make build`; ring gate
  `go test ./internal/ -run TestArchImports`; golden
  `go test ./internal/engine/ -run TestGolden`; `make all`.

## Progress Tracking

- Mark `[x]` immediately when done; ➕ for new tasks; ⚠️ for blockers.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, tests, in-repo docs.
- **Post-Completion** (no checkboxes): manual key-bearing LLM smoke runs, release.

---

## Implementation Steps

## Topic 0 — de-risk goccy before building on it

### Task 0: goccy capability spike (characterization + decision)

- [x] add `internal/initcfg/yamledit_probe_test.go`: pin goccy v1.19.2 behaviors — `parser.ParseBytes`
      with `parser.ParseComments` preserves untouched comments on `ast.File.String()`; mapping nodes expose
      source token positions (line/column) for a module's line range, its `paths:` block, and the top-level
      `layers:` list
- [x] add a characterization test showing `ast.File.String()` is not byte-identical and node replacement
      drops comments inside the replaced subtree (justifies line-splice approach)
- [x] record the decision in Technical Details: hybrid AST-located source patcher
- [x] run `go test ./internal/initcfg/...` — must pass before next task

## Topic 1 — `init` LLM modes (plan suggestions + `--apply` live-write)

### Task 1: unique module names in discovery (two-pass)

- [ ] in `Discover`, detect names shared by more than one module; disambiguate colliders with a slug of the
      first path glob (strip trailing `/**`; replace `/` and `.` with `_`)
- [ ] **second pass**: re-check the full name set; if any slug now collides with another module (slug or
      original), append a deterministic numeric suffix (`_2`, `_3`, …) by sorted order until unique
- [ ] non-colliding names are unchanged; document the exact scheme in a `Discover` doc comment
- [ ] write tests: no-collision fixture → names + `Render` byte-identical to today; Go+Python same base name
      → distinct deterministic slugs; slug-equals-existing-module-name → numeric-suffix fallback; stable ordering
- [ ] run `go test ./internal/initcfg/...` and `go test ./internal/engine/ -run TestGolden` — must pass before next task

### Task 2: stanza text helper + ModuleAnnotation + dual-mode Render

- [ ] extract `writeModuleStanza(b *strings.Builder, indent, name string, m ModuleDef, allowedLayers []string, ann *ModuleAnnotation, apply bool)`
      from `Render` — `allowedLayers` is the authority for layer validity; quote any scalar that needs it
- [ ] add `sanitizeComment(s string) string` (shared) and run **every** dynamic string rendered into a comment
      (SuggestedName, out-of-set layer value, any note) through it: strip/replace CR/LF and other control chars,
      trim, cap length — a single newline must never let an LLM value escape its comment into live YAML
- [ ] add `ModuleAnnotation struct{ Subdomain, Volatility, Layer, SuggestedName string }` — `Layer` holds the
      raw LLM suggestion; validity is decided in the helper, not the classifier
- [ ] change `Render(cfg DiscoveredConfig)` → `Render(cfg DiscoveredConfig, ann map[string]ModuleAnnotation, apply bool) string`
      (passing `cfg.Layers` as `allowedLayers`); `ann == nil` → byte-identical to today
- [ ] layer rule (both modes): the resolved live layer is `ann.Layer` if it is in `allowedLayers`, else
      `m.Layer`; write it live **only if that resolved value is in `allowedLayers`**, otherwise render it as a
      comment. (For `init`, `cfg.Layers` is derived to include every module's heuristic `m.Layer`, so the gate
      is a no-op there and `Render(cfg, nil, false)` stays byte-identical; the gate only changes `update`
      `AddModule`, where the config's `layers:` may omit a discovered layer.) `apply == false`: emit commented
      `# subdomain:`/`# volatility:` and a layer-suggestion comment (note `(not in layers:)` when out of set) +
      a `# llm: consider renaming` comment when SuggestedName differs. `apply == true`: write live
      `subdomain:`/`volatility:` and the gated layer; never rename the key
- [ ] update the caller in `cmd/archfit/init.go` to pass `(cfg, nil, false)`
- [ ] write golden tests: `nil` (unchanged), `apply=false` (round-trips), `apply=true` (round-trips),
      out-of-set layer → comment in both modes (never live), heuristic `m.Layer` absent from `allowedLayers`
      → commented not live; and an injection test: `SuggestedName = "x\"\n  volatility: high"` stays fully
      inert (sanitized, no live YAML), output still round-trips through `config.Load`
- [ ] run `go test ./internal/initcfg/...` and `go test ./internal/ -run TestArchImports` — must pass before next task

### Task 3: BuildClassifyTargets (multi-language, explicit degrade)

- [ ] add `ClassifyTarget struct{ Name string; Paths, Files []string }` and
      `func BuildClassifyTargets(root string, mods []ModuleDef) []ClassifyTarget`
- [ ] resolve up to 20 first-level file basenames for filesystem-glob paths (Go/TS); for Python dotted paths
      resolve the package dir under `root` when cheaply findable, else leave `Files` empty
- [ ] write tests: Go subdir-only package (empty Files, no crash); TS glob with files; Python dotted; cap at 20;
      deterministic order
- [ ] run `go test ./internal/initcfg/...` — must pass before next task

### Task 4: reusable LLM classifier in cmd (off-gate, batched, strict-JSON)

- [ ] add `initClassifySystemPrompt` in `cmd/archfit/init.go`: domain-modeler role; `subdomain` =
      core|supporting|generic, `volatility` = low|medium|high, `layer` chosen from a provided allowed set;
      response is a JSON **array** whose entries each include a `module` field — no prose/fences
- [ ] add `classifyBatchSize = 25` + a user-prompt builder (target name, paths, files, allowed layers)
- [ ] add `classifyModules(ctx, p llm.Provider, targets []initcfg.ClassifyTarget, layers []string) (map[string]initcfg.ModuleAnnotation, error)`:
      batch; parse strictly (tolerate fences, skip entries with unknown `module` / invalid subdomain|volatility
      enum, malformed body is a hard error). **Carry** the raw `layer` into `ann.Layer` even if out of set —
      the stanza helper / patcher decide validity
- [ ] write tests with a fake `llm.Provider`: valid, unknown-module skipped, invalid-enum skipped, out-of-set
      layer carried, malformed-body errors, batch boundary (26 → 2 batches)
- [ ] run `go test ./cmd/archfit/...` and `go test ./internal/ -run TestArchImports` — must pass before next task

### Task 5: wire init flags + shared safeWriteConfig (preserve `-o -`)

- [ ] add flags: `LLM bool`, `Apply bool`, `LLMProvider string` (default `anthropic`),
      `LLMModel string` (default `claude-opus-4-8`), `NoCache bool`; reject `--apply` without `--llm`
- [ ] add shared `safeWriteConfig(ctx, deps, path string, edited, original []byte) error` in `cmd/archfit`
      (strict protocol, Technical Details); `original == nil` means the target must not exist — abort if it
      appeared since read; notices to `deps.Stdout`
- [ ] **preserve stdout mode**: when `Output == "-"`, print rendered YAML to `deps.Stdout` and return — no
      `safeWriteConfig`, no backup (LLM cache may still write unless `--no-cache`)
- [ ] when `--llm`: best-effort read of an existing `.archfit.yaml` `tools.llm` (tolerate an invalid config —
      fall back to flags); `buildProvider` + `llm.NewCache` unless `--no-cache`
- [ ] build targets via `BuildClassifyTargets`; `classifyModules` to completion; `Render(cfg, ann, c.Apply)`;
      for a file path, write via `safeWriteConfig`
- [ ] write tests: no-`--llm` unchanged (golden); `init --llm` → commented; `init --llm --apply` → live;
      `init --apply` alone → error; `init -o -` → stdout, no file/backup; invalid existing config → runs from flags
- [ ] run `make test` and `make lint` — must pass before next task

### Task 6: verify Topic 1 acceptance

- [ ] all four init modes per the matrix; no key rename; out-of-set layer never live; names unique; `-o -` streams
- [ ] ring gate green (`initcfg` imports neither `internal/llm` nor `internal/config`); golden gate green
- [ ] `make all` passes; coverage ≥ project standard

## Topic 2 — AST-located source patcher (foundation for `update --apply`)

### Task 7: comment-preserving source patcher with a strict contract

- [ ] add `internal/initcfg/yamledit.go`: `func ApplyEdits(src []byte, edits []Edit) ([]byte, error)`; parse with
      goccy (`parser.ParseComments`) to locate+validate and to read the top-level `layers:` set; mutate via line
      splices on the original bytes
- [ ] `Edit` variants (closed set): `AddModule(ModuleDef, *ModuleAnnotation)` (renders via `writeModuleStanza`
      with the parsed `layers:` as `allowedLayers`), `SetModuleFields(module string, fields map[ModuleField]string)`
      with `ModuleField` enum {Subdomain, Volatility, Layer} — inserts one coalesced block of the absent fields
      in **canonical order (subdomain, volatility, layer)** for stable diffs/tests, YAML-quoting values, and
      **skips a Layer value not in the parsed `layers:`**;
      `UpdateModulePaths(module string, paths []string)`; `CommentModule(module, note string)`
- [ ] `UpdateModulePaths`: replace the `paths:` line range if present; if the module exists but has no `paths:`,
      insert a block-style `paths:` as the first stanza key; reject inline flow-style `paths: [...]`
- [ ] `CommentModule` runs `note` through `sanitizeComment` (no CR/LF/control chars) and prefixes the module's
      source range (incl. head comment) with a marker (Technical Details);
      no-op if the marker already exists. `AddModule` is a no-op if a live `module:` key already exists
- [ ] contract: compute all line ranges first; reject overlapping/conflicting edits; apply bottom-up; a missing
      target module (Set/UpdatePaths/Comment) is an error; create a block-style `modules:` at the defined
      location when absent; reject flow-style `modules: {}`
- [ ] preserve everything not targeted: comments (incl. head comments), key order, `rules`, `exceptions`, formatting
- [ ] import neither `internal/llm` nor `internal/config`; operate on raw bytes + plain inputs
- [ ] write tests: add module (comments/rules intact; into a no-`modules:` config; insertion-location cases:
      layers-present / tools-present / rules-only / append-EOF); `SetModuleFields` coalesces multiple absent
      fields into one block (no overlap error); Layer-out-of-set skipped; `UpdateModulePaths` into a stanza with
      no `paths:`; flow-style rejection; comment incl. head comment; re-apply CommentModule/AddModule → no-op;
      bottom-up ordering; round-trips through `config.Load`
- [ ] run `go test ./internal/initcfg/...` and `go test ./internal/ -run TestArchImports` — must pass before next task

## Topic 3 — `archfit update`

### Task 8: module-diff logic (pure, dependency-free, normalized)

- [ ] add to `internal/initcfg/update.go`:
      `type ExistingModule struct{ Name string; Paths []string; HasSubdomain, HasVolatility, HasLayer bool }`,
      `type PathDelta struct{ Name string; ConfigPaths, DiscoveredPaths []string }`,
      `type UpdateReport struct{ Added []ModuleDef; Removed []ExistingModule; PathDrift []PathDelta; Unclassified []string; StructuralInSync bool }`
- [ ] `func DiffModules(existing []ExistingModule, fresh []ModuleDef) UpdateReport` — compare paths as normalized
      sets (trim empties, dedupe, sort); preserve discovered order for output; `StructuralInSync` when no
      add/remove/drift; `Unclassified` = non-removed existing modules missing **any** of subdomain/volatility/layer
      (uses `HasSubdomain`/`HasVolatility`/`HasLayer`), so a module with subdomain+volatility but no layer is a candidate
- [ ] no `config` import
- [ ] write table-driven tests incl. path-reorder → no drift; removed-and-unclassified (removed excluded);
      subdomain+volatility present but no layer → still in `Unclassified`
- [ ] run `go test ./internal/initcfg/...` — must pass before next task

### Task 9: report rendering (default plan mode)

- [ ] `func RenderUpdateReport(r UpdateReport, ann map[string]ModuleAnnotation, allowedLayers []string) string`:
      added (paste-ready stanzas via `writeModuleStanza` with `allowedLayers`, incl. annotation), removed
      (`verify or remove`), path-drift, unclassified (with classification when `ann` has it, else suggest `--llm`);
      a "structurally in sync" line when `StructuralInSync`
- [ ] path-drift section states explicitly that `--apply` replaces module paths with discovered paths (backup written)
- [ ] write tests: report per case, with/without `ann`; out-of-set layer rendered as comment; added-stanza
      round-trips via `config.Load`
- [ ] run `go test ./internal/initcfg/...` — must pass before next task

### Task 10: UpdateCmd wiring (plan default + `--apply` + `--llm`)

- [ ] add `cmd/archfit/update.go`: `UpdateCmd{ Config; Root; LLM, Apply, NoCache bool; LLMProvider, LLMModel string }`;
      register in `cmd/archfit/main.go`; when `--root` is unset, default discovery root to the directory of `--config`
- [ ] `Run`: `loadConfig` existing → `[]initcfg.ExistingModule` (incl. `HasLayer`); `Discover`; `DiffModules`
- [ ] `--llm`: `BuildClassifyTargets` from `Added` (fresh) **and** `Unclassified` existing modules; `classifyModules`;
      pass `ann` + the config's `layers` to the report
- [ ] compute `HasActionableEdits` = structural edits (add/path/remove) **or** at least one field-fill that
      survives filtering — a field-fill counts only when the field is absent on an existing (not newly added)
      module, the annotation value is non-empty, and (for layer) the value is in `layers`. An LLM reply with
      only an out-of-set layer for an existing module yields no actionable edit. Default (no `--apply`) prints
      the report, leaves `.archfit.yaml` untouched (LLM cache may write unless `--no-cache`)
- [ ] `--apply`: if `!HasActionableEdits`, print the in-sync line and write nothing; else build edits —
      `AddModule(def, ann)` for added (classification in the stanza), `UpdateModulePaths` for drift, `CommentModule`
      for removed, and `SetModuleFields` for existing modules only for **absent** fields (use `HasSubdomain/
HasVolatility/HasLayer` — never overwrite an existing field). Never emit both `AddModule` and `SetModuleFields`
      for the same module. `ApplyEdits` → `safeWriteConfig`; notice incl. that paths were replaced
- [ ] write tests vs a divergent fixture: plan mode byte-unchanged; `--apply` adds/updates/comments, preserves
      rules+comments; `--llm --apply` on a structurally in-sync config writes only absent fields and no
      `SetModuleFields` for newly added modules; existing layer never overwritten; an existing module needing
      only a layer gets it filled when valid; an LLM reply of only an out-of-set layer writes nothing;
      backup created; changed-since-read aborts
- [ ] run `make test` and `make lint` — must pass before next task

### Task 11: verify Topic 3 acceptance

- [ ] all four update modes per the matrix; plan mode never writes `.archfit.yaml`; `--apply` preserves untargeted
      content and is idempotent on re-run; existing fields never overwritten; round-trips via `config.Load`
- [ ] `make all` passes; ring + golden gates green

## Topic 4 — portable skill (agentskills.io)

### Task 12: SKILL.md compliance edits

- [ ] add a `metadata:` frontmatter block (`author`, `tags`); **omit `version`**
- [ ] add a "Do not use when" paragraph (generic architecture advice unrelated to the tool; adoption → web research)
- [ ] add a portability line: outside the archfit source repo, use the public links in
      `references/archfit-docs.md` exclusively; do not assume local paths exist
- [ ] verify required frontmatter (`name` == directory, `description`); body < 500 lines
- [ ] run `skills-ref validate ./skills/archfit` if available, else a manual frontmatter check

### Task 13: document new command modes in the skill + references

- [ ] document the mode matrix + plan→apply safety model; note guardrails (live `layer` only if in `layers`;
      keys never auto-renamed; `--apply` replaces paths with discovered + backup; existing fields never overwritten;
      plan mode leaves the config untouched)
- [ ] ensure `references/archfit-docs.md` links are present/current; add `llm-enrich.md` if missing
- [ ] confirm the body stays consistent and < 500 lines

### Task 14: Verify acceptance criteria (whole plan)

- [ ] every Overview requirement and the full mode matrix implemented
- [ ] `make all` passes; ring + golden green; coverage ≥ standard; no new lint findings

### Task 15: Update documentation

- [ ] update `docs/guide` (init / config-reference) for `--llm`, `--apply`, `update`
- [ ] update `README.md` command list if it enumerates commands
- [ ] ensure kong `--help` strings on the new flags are clear (esp. the `--apply` safety note)

## Technical Details

### goccy decision (from Task 0)

Hybrid **AST-located source patcher**: goccy v1.19.2 parses with `parser.ParseComments` to locate node source
positions and read the top-level `layers:` set, and to validate structure; all mutations are line splices on the
original bytes. `ast.File.String()` is not byte-preserving and node replacement loses targeted comments.

### Layer-validity plumbing

`writeModuleStanza(..., allowedLayers, ann, apply)` is the single authority for live-vs-comment layer. `Render`
passes `cfg.Layers`; `RenderUpdateReport` passes the config's `layers`; `ApplyEdits` parses the file's `layers:`
and passes them for `AddModule`, and skips a `SetModuleFields` Layer value not in that set. The **resolved** live
layer (`ann.Layer` if valid, else `m.Layer`) is written live only when it is itself in `allowedLayers` — so an
`AddModule` whose heuristic `m.Layer` is absent from the target config's `layers:` renders the layer as a comment
rather than emitting a live value that `layerRank` would treat as -1 and silently exclude from layer rules. For
`init`, `cfg.Layers` already contains every `m.Layer`, so the gate is a no-op and the `ann == nil` golden output
is unchanged.

### Comment sanitization (injection guard)

Every dynamic string rendered into a YAML comment — `SuggestedName`, an out-of-set layer value, a `CommentModule`
note — passes through `sanitizeComment`: CR/LF and other control characters are stripped/replaced, the value is
trimmed and length-capped. Without this, an LLM-produced `SuggestedName` containing a newline could close the
comment and inject live YAML (e.g. a fake `volatility:` line) into a gate-feeding file. Tested with
`"x\"\n  volatility: high"` staying fully inert.

### Removed-module marker (`CommentModule` idempotency)

`CommentModule("foo", note)` prefixes the module's source range (incl. head comment) and prepends a marker:

```yaml
# archfit: removed module "foo" — verify before deleting (<note>)
# foo:
#   paths:
#     - "foo/**"
```

Re-applying is a no-op when `# archfit: removed module "foo"` already exists in `src`.

### Missing `modules:` insertion location (`AddModule`)

Create a block-style section, in priority order: (1) after `layers:`, (2) else after `tools:`, (3) else before
`rules:`, (4) else append before EOF.

### Strict `--apply` write protocol (shared `safeWriteConfig` in `cmd/archfit`)

1. LLM classification completes fully before any write (no partial-batch writes).
2. Write edited bytes to a temp file in the target's directory.
3. Validate by `config.Load(ctx, tempPath)`; abort + remove temp on error (`config.Load` is path-based).
4. Concurrency guard: if `original != nil`, compare a hash of the bytes read against a fresh re-read and abort if
   changed; if `original == nil` (target must not exist), abort if the target now exists.
5. Back up the original to a non-clobbering path (`.archfit.yaml.bak`, timestamped / `O_EXCL` if it exists).
6. `os.Rename` the temp over the target (atomic). `init` (file path) uses this too, replacing `os.WriteFile`;
   `init -o -` bypasses it and streams to stdout.

Notices print to `deps.Stdout`. A residual TOCTOU window remains between the step-4 re-read and the step-6 rename;
accepted as the single-user-CLI ceiling (no lock file). **Non-goal**: repairing a config that already has duplicate
module keys — `loadConfig` rejects it before `update` can diff.

### LLM classify response (strict JSON array)

`[{"module":"<name>","subdomain":"core|supporting|generic","volatility":"low|medium|high","layer":"<string>","name":"<suggested>","rationale":"<one sentence>"}]`

### Commented-inert init output (default `--llm`)

```yaml
classify:
  paths: ["internal/classify/**"]
  layer: core
  # subdomain: core      # llm-suggested — review and uncomment
  # volatility: low       # llm-suggested — review and uncomment
  # llm layer: domain     # not in layers: — review
  # llm: consider renaming to "classification"
```

### Boundaries

`Discover`, `DiffModules`, `ApplyEdits`, `BuildClassifyTargets`, `ClassifyTarget`, `ModuleAnnotation`,
`writeModuleStanza` live in `internal/initcfg` and import neither `config` nor `llm`. `cmd/archfit` owns: provider
bootstrap, cache, the `config.Config` ↔ `[]ExistingModule`/`[]ClassifyTarget` projections, `HasActionableEdits`,
and `safeWriteConfig`.

## Post-Completion

_Manual / external — informational only._

**Manual verification:**

- One real `init --llm --apply` and one `update --llm --apply` smoke run on a small multi-language repo with a live
  `ANTHROPIC_API_KEY` (from `op`: Doit/Reflex Keys/anthropic-team-key); confirm the written config passes
  `archfit check` and the classification is sensible.

**Release:**

- Tag-triggered only (`git tag -a vX.Y.Z && git push origin vX.Y.Z`); never release manually.

## Open follow-ons (out of scope, noted for symmetry)

- `enrich --apply` (auto-approve LLM coupling labels) to complete the "trust the LLM" story for rules.
- Explicit module **rename** support: path-based old↔new matching + reference rewrites across
  `rules`/`exceptions`/labels (the safe version of the dropped live key-rename).
- `update --prune` to delete (not comment) removed modules.
