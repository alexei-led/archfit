# Wave 2: Config & Rules Correctness — trustworthy onboarding

## Overview

Wave 2 of 7 from `docs/archived/reports/eval-2026-07-02-v1.1.2/00-FINDINGS.md`. Assumes Wave 1 (gate integrity) is merged — verdict tests exist and per-metric gating works.

Three verified defects make archfit's out-of-box experience wrong, which matters doubly when an AI agent generates the config unaided:

- **V4 — `config init` emits a rule that can never fire.** `internal/initcfg/initcfg.go:352-370` (`Render`/`inferLayerRules`) emits `type: forbidden_dependency` with `from_layer:`/`to_layer:` keys — for both the inferred-rules branch and the `no-forbidden-deps` placeholder. But `forbiddenDependency.Check` (`internal/rules/rules_dependency.go:27-34`) reads `def.From`/`def.To` path globs; those are empty, and `doublestar.Match("", path)` is always false — the rule matches zero edges, ever. The generated header even says `# TODO: review and promote gate: warn to gate: fail`, steering users into a permanently vacuous **blocking** gate. Verified repro: two-layer repo with a genuine back-edge → `pass`, 0 findings; changing only `type:` to `forbidden_layer_direction` → `fail`, 1 finding. Note `forbiddenLayerDirection.Check` (`rules_dependency.go:108-138`) ignores `FromLayer`/`ToLayer` too (derives layers from the module map globally), so the generator's `from_layer/to_layer` values are vestigial even for the correct type.
- **V5 — public_api_only mislabels same-module access.** `publicAPIOnly.Check` (`rules_dependency.go:63-92`) has no module-map check; it fires on any `EdgeKindUsesInternal` edge, which Go's extractor assigns lexically (`internal/extract/golang/golang.go:349-352`: import path contains `/internal/`). A single module `domain` importing its own `domain/internal` — idiomatic Go and the pattern in the tool's own docs (`docs/guide/configuration-reference.md:60-70`) — fires a finding whose JSON shows `from.module == to.module == "domain"` yet says `"Cross-module access to internal path …"`, under a default **blocking** gate (`fail` is the type's default; not in `defaultGateForType`'s warn list).
- **Ownership case bug (new, high).** On omni, a lowercase `--root` silently drops `SubtreePrefix` → `owner_source` flips `codeowners→none` → coupling_balance flips from the true 40/poor ("distributed-monolith risk") to a false 58/mixed, zero warnings. `classified_edges.total` is identical between runs, isolating the bug to ownership resolution. Task 25's `snapScanRoot` (`internal/scope/scope.go`, `os.SameFile` device+inode) fixed scope resolution but not the CODEOWNERS subtree-prefix derivation. This is the only place in the tool that violates abstain-not-fake: it degrades silently instead of disclosing.

## Context

- `internal/initcfg/initcfg.go` (generator), `cmd/archfit` config group commands.
- `internal/rules/rules_dependency.go` (forbiddenDependency :27-34, publicAPIOnly :63-92, forbiddenLayerDirection :108-138), `internal/rules/rules.go` (`defaultGateForType` :107-113).
- `internal/scope/scope.go` (`snapScanRoot`, `SubtreePrefix`), CODEOWNERS matching (subtree handling shipped in v1.0 — find via `git log --grep CODEOWNERS`).
- Fixtures: create `internal/initcfg/testdata/` mini-projects per language (see Task 1).

## Development Approach

- Tests-first: reproduce each defect in a failing test before fixing.
- One behavior change per commit; goldens regenerated deliberately per commit.
- Branch `fix/wave2-config-rules`; `make test && make lint && make archfit` green between tasks; PR at end.

## Implementation Steps

### Task 1: Four-language init fixtures

- [x] create minimal fixture projects under `internal/initcfg/testdata/`: `gofixture/` (two packages with a layer back-edge), `tsfixture/` (two dirs + tsconfig), `pyfixture/` (small package, dotted imports), `rustfixture/` (single crate, two modules) — each just large enough for `config init` to infer modules
- [x] add `TestConfigInit_PerLanguage` that runs init against each fixture and asserts: generated YAML parses under strict config; every generated rule has a `type:` recognized by `internal/rules`
- [x] the Go fixture test additionally runs the full analyze gate over the fixture with the generated config and asserts the layer rule CAN fire when the back-edge exists (this is the V4 repro — expect it to fail until Task 2)
- [x] run `make test-fast` — commit harness (V4 case marked TODO(wave2) per partial-implementation rule)

### Task 2: Fix config init rule generation (V4)

- [x] `initcfg.go:352-370`: emit `type: forbidden_layer_direction`; drop the vestigial `from_layer:`/`to_layer:` keys (the checker derives layers from the module map)
- [x] fix the `no-forbidden-deps` placeholder branch the same way; keep `gate: warn` as the shipped default, and keep the promote-to-fail TODO comment — it is now honest
- [x] flip the TODO(wave2) fixture assertion: generated config on the Go fixture now fails the gate when promoted to `fail`, passes when the back-edge is removed
- [x] add a migration note: existing users with generated-vacuous rules get a `doctor` check (extend `doctor` to flag `forbidden_dependency` rules that have neither `from:` nor `to:` — they are dead by construction)
- [x] run `make test && make lint && make archfit`; commit

### Task 3: public_api_only consults the module map (V5)

- [x] failing test first: fixture module `domain` with nested `domain/internal`, `domain.go` importing it, docs-example config — currently produces a blocking finding; assert zero findings expected
- [x] `rules_dependency.go:63-92`: resolve both endpoints through the module map; skip when `from.module == to.module`; keep firing for genuine cross-module internal access (add a positive-case test: `moduleA` importing `moduleB/internal/...` must still fire)
- [x] make the `Why` text conditional — never claim "Cross-module" for an edge the module map calls same-module
- [x] language check: the rule keys off `EdgeKindUsesInternal`, which only Go's extractor assigns lexically — add per-language tests documenting behavior for TS/Python/Rust edges (no `EdgeKindUsesInternal` → rule inert there; assert no spurious findings on the Wave 2 Task 1 fixtures)
- [x] run `make test && make lint && make archfit`; regen goldens if output changed; commit

### Task 4: Case-safe CODEOWNERS subtree + owner degradation warning

- [x] failing test first: temp git repo with CODEOWNERS, analyze with a case-variant `--root` (guard: skip unless the filesystem is case-insensitive — probe by creating `a`/`A` in t.TempDir()); assert `owner_source` stays `codeowners` and SubtreePrefix is correct
- [x] apply the `os.SameFile` canonicalization used by `snapScanRoot` to the path used for `SubtreePrefix`/CODEOWNERS resolution in `internal/scope/scope.go`
- [x] disclosure rule: whenever owner resolution degrades (`codeowners→git→none`), emit a stderr warning naming the cause, and surface `owner_source` in the JSON evidence — silent degradation is the defect, not just the wrong prefix
- [x] test the warning path (force a degradation, assert stderr + JSON field)
- [x] run `make test && make lint && make archfit`; commit

### Task 5: Corpus verification (four languages)

- [x] Go — omni repro from `docs/archived/reports/eval-2026-07-02-v1.1.2/corpus-experiments.md`: run the exact lowercase `--root` invocation; score must equal the correct-case run (40/poor), `owner_source: codeowners` — verified byte-identical JSON between correct-case and lowercase `--root` runs (using `reports/eval-2026-06-30-corpus/configs/omni.yaml` against `server/services/scheduled-tasks`); both `owner_source: codeowners`, `40/poor`
- [x] Go — a throwaway `config init` on a fresh clone (e.g. `~/workspace/spotinfo` copy in scratch): generated rules parse, layer rule fires on an injected back-edge — verified in `/tmp/spotinfo-scratch` (git clone): generated `forbidden_layer_direction` rule, injected a real core→cmd import edge (`go build` clean), gate promoted to `fail` correctly fires (exit 1) with an accurate `matched_by.from_layer/to_layer`
- [x] TypeScript — `config init` smoke on `~/workspace/storybook` subdir copy: parses strict, analyze exit 0 — verified on a scratch copy of `code/core`; generated config parsed strict, `analyze --gate --full` exit 0
- [x] Python — `~/workspace/ccgram`: no same-module false positives from public_api_only; analyze exit unchanged vs pre-wave — 0 `public_api_only` findings (rule is inert for Python — no `EdgeKindUsesInternal` edges, as documented in Task 3), exit 2 (warn) matches the pre-wave baseline in `docs/archived/reports/eval-2026-07-02-v1.1.2/corpus-experiments.md`
- [x] Rust — `~/workspace/herdr`: analyze exit unchanged; no new findings from this wave's rules — exit 0/pass, score 25/poor matches pre-wave baseline; only `bc/imbalanced_coupling` findings present (config has no `rules:` block, so V4/V5 rule types don't apply)
- [x] leave every corpus repo clean (`git status --porcelain` unchanged); run `make all` — all 5 corpus repos (omni, spotinfo, storybook, ccgram, herdr) confirmed unchanged pre/post; scratch clones used under `/tmp` for the throwaway config-init tests, removed after use; `make all` green (0 blocking, 82 advisory, same baseline as prior commits). PR deferred to end of Task 6 per plan structure.

### Task 6: [Final] Documentation

- [x] `docs/guide/configuration-reference.md`: correct the layer-rule example (`configuration-reference.md:60-70` area) and document the doctor dead-rule check
- [x] mark V4/V5 + omni case bug fixed in the findings backlog with commit refs

## Technical Details

- The doctor dead-rule check is deliberately conservative: flag only `forbidden_dependency` with both globs empty — that combination is provably inert.
- Case-insensitivity probe, not `runtime.GOOS == "darwin"`: Linux can mount case-insensitive volumes and macOS can be case-sensitive.

## Post-Completion

- Ask existing users (or regen self/corpus configs) to re-run `config init`-derived configs through `doctor` to surface pre-existing vacuous rules.
