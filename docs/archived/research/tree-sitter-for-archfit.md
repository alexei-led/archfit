# Research: Tree-sitter as archfit's syntax-facts layer

Status: research complete · 2026-06-23 · branch `tree-sitter-research`
Input: the "Tree-sitter as ArchFit's universal syntax-facts layer" brief.

## TL;DR

The **product direction is sound and additive**: a cross-language syntax-facts
layer (declarations, public API surface, framework annotations, role hints,
changed-file summaries, compact agent context) gives archfit value its
dependency-only extractors do not produce today.

The **mechanism in the brief is wrong for archfit**. The brief assumes an
in-process tree-sitter Go library (`internal/extract/syntax/` + `.scm` query
files). Every maintained Go tree-sitter binding is **CGO-mandatory** (proven
below). Adopting it means dropping `CGO_ENABLED=0`, migrating the 4-target
cross-compile release to a CGO-capable cross toolchain, and reworking the
multi-arch Docker build — a large, permanent infrastructure cost.

That cost buys facts archfit can already produce: **ast-grep is already shipped
and wired** (`internal/extract/astgrep`, `@ast-grep/cli` in the Docker image),
and ast-grep _is tree-sitter under the hood_. Experiments confirm it extracts
named declarations, decorators/attributes, exported symbols, and exact
locations across Go/Python/TS/Rust via JSON.

**Recommendation:** keep the product direction. Deliver the syntax-facts layer
through the **already-bundled ast-grep subprocess** (behind `toolrun.Runner`),
emitted as a neutral `SyntaxFacts` block (the `FileFacts` precedent), off-gate.
Zero new binary/build/release cost. Decline in-process tree-sitter bindings.

---

## The constraint that decides it: build & release model, not arch_test

archfit's hard invariant is the **build and release model**, enforced by the
Makefile / `release.yaml` / `Dockerfile`, not by `internal/arch_test.go`:

- `make build`, `make release`, and the Docker build all use `CGO_ENABLED=0`.
- `release` cross-compiles to **linux/amd64, linux/arm64, darwin/amd64,
  darwin/arm64** with `CGO_ENABLED=0 GOOS=… GOARCH=…` (`Makefile:83-104`).
- The Docker image builds a fully static binary (`Dockerfile:20-23`).

Note what is _not_ the blocker: the hexagonal `arch_test` permits in-process
libraries. The Go extractor already runs `go/packages` in-process, and a linked
library is not a subprocess, so it would not violate the "every subprocess goes
through `toolrun.Runner`" rule either. The blocker is purely that a CGO binary
cannot be produced by the current `CGO_ENABLED=0` cross-compile pipeline.

---

## Spike: is in-process tree-sitter CGO-free? (empirical)

Built an isolated module (not added to archfit's `go.mod`) against the official
binding `github.com/tree-sitter/go-tree-sitter v0.25.0` + grammars
(`tree-sitter-go v0.25.0`, `-python`, `-rust v0.23.2`, `-typescript v0.23.2`;
indirect `mattn/go-pointer v0.0.1`).

**Why CGO is mandatory** — the grammar binding compiles C parser source:

```go
// tree-sitter-go@v0.25.0/bindings/go/binding.go
package tree_sitter_go
// #cgo CFLAGS: -std=c11 -fPIC
// #include "../../src/parser.c"
```

With `CGO_ENABLED=0` the package has _no buildable Go files_ (the cgo build
constraint excludes them), so it fails to compile.

**Build matrix** (go 1.26.4, macOS arm64 host):

| Build                                                       | Result                                               |
| ----------------------------------------------------------- | ---------------------------------------------------- |
| `CGO_ENABLED=1` native (darwin/arm64)                       | ✅ builds, parses correctly                          |
| `CGO_ENABLED=0` native                                      | ❌ exit 1 — "build constraints exclude all Go files" |
| `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` (a release target)  | ❌ exit 1 — same                                     |
| `CGO_ENABLED=1 GOOS=linux GOARCH=arm64` (plain macOS clang) | ❌ `runtime/cgo` needs a Linux cross C compiler      |

The last row is the cost, not an impossibility: `zig cc` is the standard way Go
projects cross-compile CGO to all four targets. The honest framing — **adopting
in-process tree-sitter requires dropping `CGO_ENABLED=0`, switching the release
matrix to a CGO cross toolchain (e.g. `zig cc`) for linux/darwin × amd64/arm64,
and reworking the multi-arch Docker build.** Permanent infra cost.

**Binary footprint** (stripped `-s -w -trimpath`):

| Binary                          | Size    |
| ------------------------------- | ------- |
| archfit today (`CGO_ENABLED=0`) | 25.7 MB |
| 1 grammar linked in             | +2.1 MB |
| 4 grammars (go+py+rust+ts)      | +6.2 MB |

Size is the minor cost. The blocker is the toolchain migration.

---

## Tree-sitter Go binding landscape (verified, mid-2026)

Confirmed via Perplexity + the spike:

- **`github.com/tree-sitter/go-tree-sitter`** (official) — actively maintained,
  the right target for new work; core binding only, grammars consumed per-language
  from `tree-sitter-<lang>/bindings/go`. **CGO.**
- **`github.com/smacker/go-tree-sitter`** — legacy/superseded; bundles many
  grammars in one module. **CGO.**
- **No maintained pure-Go / `CGO_ENABLED=0` binding exists.** No
  ecosystem-standard wazero/WASM tree-sitter binding covering Go/Py/TS/Rust with
  a stable Go API and published benchmarks. A WASM-via-wazero path would be
  custom engineering and is expected to be materially slower than native.
- The only CGO-free-host option is **purego + per-arch shared libraries**
  (`.so`/`.dylib` for each of the 4 targets × N grammars) — it removes CGO from
  the Go build but reintroduces a native-artifact matrix, which defeats the
  single-static-binary distribution model. Rejected.

---

## The discriminator: ast-grep is already tree-sitter, already shipped

archfit already integrates a tree-sitter-based structural engine:

- `internal/extract/astgrep/astgrep.go` — `Adapter` implements
  `ports.PatternProvider`, shells `sg` through `toolrun.Runner`, parses JSON to
  `pattern.Match`. Absent-tool → `Coverage{Status: "absent"}`, never an error.
- `Dockerfile:50-53` installs `@ast-grep/cli` in the runtime image.
- ast-grep is a Rust CLI that **embeds tree-sitter**.

So the brief's "add tree-sitter" is, mechanically, _already done_ — behind a CLI
rather than a linked library. The real question the brief must answer is: **what
does an in-process tree-sitter library give that the ast-grep subprocess does
not?** For the syntax-facts use cases: nothing that justifies the CGO cost.

### What ast-grep extracts today (experiments)

Run against sample Go/Python/TS files with `sg --lang … --json`:

- **Named declarations** — pattern `func $NAME($$$) $$$` on Go captured
  `NewService`, `helper`. JSON returns `metaVariables.single.NAME.text` **and its
  range** (line/column/byteOffset), plus `file`, `language`, matched `text`.
- **Decorators / attributes / annotations** — Python `@router.$METHOD($$$)`
  captured `METHOD=get`; TS `@$DEC($$$)` captured `@Controller("/users")`.
- **Exported symbols** — TS `export class $NAME { $$$ }` captured `Foo`.
- **Kind-based enumeration** — a `sg scan` YAML rule with `kind:
class_declaration` matched _every_ class regardless of decorator/export
  wrapper (`class UserService`). Robust where pattern shapes are brittle.
- **Coverage** — ast-grep supports Go, Python, TypeScript, Rust (and more).

Two technique notes (don't overclaim either):

- **`pattern` rules** give clean `metaVariables.NAME` capture but are
  **per-shape** — a method (`func (r R) $NAME(...)`) is a different shape from a
  free function; decorated/exported wrappers change the shape. You write one
  pattern per shape.
- **`kind` rules** enumerate by tree-sitter node kind robustly, but you
  post-process the matched `text` to pull out the name (no clean metavariable).

Either way, this is the full syntax-facts vocabulary the brief asks for. What
ast-grep does **not** give — type resolution, import resolution, interface/trait
implementation, real call graph — the brief explicitly says tree-sitter
shouldn't do either; archfit already has SCIP for symbol-level strength.

(Considered and rejected: the raw `tree-sitter` CLI as a subprocess — lower
level, not bundled, no advantage over the already-present higher-level ast-grep.)

---

## The pivot, stated plainly

```
Brief proposed:  internal/extract/syntax/ + queries/*.scm  (in-process tree-sitter lib)
Research found:  that mechanism is CGO-mandatory → breaks CGO_ENABLED=0 + the
                 4-target cross-compile + multi-arch Docker build
Therefore:       deliver the SAME SyntaxFact model via the already-shipped
                 ast-grep subprocess (tree-sitter under the hood)
Result:          product direction survives; only the extraction mechanism changes
```

---

## Recommended design (additive, off-gate)

**Facts shape — Option B (neutral block), following the `FileFacts` precedent.**
`internal/facts` already emits a neutral, score-free facts block on
`Diagnostic`. Add a sibling:

```go
// Diagnostic gains:
SyntaxFacts []SyntaxFact `json:"syntax_facts,omitempty"`

type SyntaxFact struct {
    Language  string `json:"language"`
    File      string `json:"file"`
    Kind      string `json:"kind"`              // function|method|class|struct|interface|trait|enum|type_alias|annotation|route
    Name      string `json:"name"`
    Exported  bool   `json:"exported,omitempty"`
    StartLine int    `json:"start_line"`
    EndLine   int    `json:"end_line"`
    Role      string `json:"role,omitempty"`    // handler|service|repository|domain|… (heuristic)
    RoleConf  string `json:"role_confidence,omitempty"`
}
```

**Producer — reuse the subprocess path.** Either extend the existing
`astgrep.Adapter` or add a sibling `SyntaxProvider` port that runs `sg scan`
with a bundled set of per-language `kind`/`pattern` rules (the rules replace the
brief's `.scm` files; they live in archfit, versioned, not in `go.mod`).

**Wiring (from the codebase map):**

1. Add `tools.syntax` opt-in in config (`internal/config/tools.go` — mirror
   `ToolCargoModules`/`ToolScip`); default `auto`.
2. Run the provider in the engine; populate `SyntaxFacts` after `engine.Run`
   like Rust's module-graph coverage is appended in `pipeline.go`.
3. Add a `Coverage` row for the tool in `ToolCoverage`. Absent `sg` →
   `Status: "absent"`, never a failure (existing convention).
4. `jsonout` serializes the new field automatically.

**Phasing** (matches the brief):

- **Phase 1** — collect syntax facts (Go + TS first), emit the neutral block.
- **Phase 2** — surface in `scan`/`review` output; add the public-API-surface
  report; add Python/Rust.
- **Phase 3** — enrich `agent_tasks` evidence (symbol + role + location).
- **Phase 4 (optional, flavor C)** — attach role hints to graph nodes (e.g.
  `file … role: handler`). The brief leaned this way; defer until Phase 1 facts
  prove out. Keep it report-only first — never gate on heuristic roles.

---

## Tradeoffs accepted

- **Extends the external-tool surface.** ast-grep is present in the Docker image
  but **not guaranteed in standalone-binary installs**. So syntax facts are
  _absent-when-`sg`-absent_ — exactly like the existing pattern provider. This is
  consistent with archfit's no-false-green philosophy: missing tool → `n/a`,
  never invented facts. It is the honest cost of the subprocess path versus a
  self-contained binary.
- **Syntactic, not semantic.** Roles are heuristic (name/path/decorator/call
  shape) with explicit confidence. Keep them off-gate and report-only.
- **Per-shape rule authoring.** Some constructs need multiple `pattern` rules or
  a `kind` rule + text post-processing. Bounded, language-by-language work.

## Rejected alternatives

| Option                                                  | Why rejected                                                                                                                                        |
| ------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| In-process tree-sitter Go bindings (`smacker`/official) | CGO-mandatory; breaks `CGO_ENABLED=0` + 4-target cross-compile + multi-arch Docker. Large permanent infra cost for facts ast-grep already produces. |
| CGO + `zig cc` cross toolchain                          | Viable but rewrites the entire release/Docker pipeline for marginal benefit over the bundled subprocess.                                            |
| purego + per-arch shared libraries                      | Removes CGO from the Go build but reintroduces a native-artifact matrix (4 targets × N grammars), defeating single-static-binary distribution.      |
| wazero / pure-Go tree-sitter                            | No maintained, multi-language, benchmarked binding exists (mid-2026). Custom engineering; expected materially slower.                               |
| Raw `tree-sitter` CLI subprocess                        | Lower-level, not bundled, no advantage over the already-present ast-grep.                                                                           |

---

## Appendix: reproduce the spike

```bash
# isolated module — do NOT add to archfit go.mod
mkdir ts-spike && cd ts-spike && go mod init tsspike
go get github.com/tree-sitter/go-tree-sitter@latest \
       github.com/tree-sitter/tree-sitter-go/bindings/go@latest
# proves CGO requirement:
CGO_ENABLED=0 go build ./...            # -> exit 1 (build constraints exclude all Go files)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...   # -> exit 1 (release target)
CGO_ENABLED=1 go build ./...            # -> ok

# ast-grep already extracts the same facts (sg = @ast-grep/cli, in the Docker image):
sg --lang go   -p 'func $NAME($$$) $$$ { $$$ }' --json=compact file.go   # captures NAME + range
sg --lang python -p '@router.$METHOD($$$)'      --json=compact file.py   # captures decorator
echo 'id: c
language: typescript
rule: {kind: class_declaration}' > r.yml
sg scan -r r.yml --json=compact file.ts                                  # enumerates all classes
```
