# Syntax-facts route false-positive fix — design v1.0

Branch: `tree-sitter-research`  
Date: 2026-06-25  
Status: APPROVED FOR IMPLEMENTATION

## Problem

Route rules match a call/decorator SHAPE without verifying that the matched
framework is actually present in the file. Three validated false-positive classes
from the cross-repo eval (5 repos, 2026-06-24):

| Repo            | Count | Root cause                                                                                                                                                      |
| --------------- | ----: | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| pumba (Go)      |     6 | `cntr.Delete(ctx, ...)` matched as chi/fiber route — `$PATH=ctx`, not a string                                                                                  |
| codegraph (TS)  |     1 | `https.get(url, cb)` matched as express route — `$PATH=url`, not a string                                                                                       |
| ccgram (Python) |   167 | `@pytest.fixture(autouse=True)`, `@win.event("window")`, `@topic_state.register("topic")` etc. matched — verbs are `fixture`/`event`/`register`, not HTTP verbs |

Also confirmed: `roleFromPath` substring bug — `"repo"` matches `update_report*` (12 false
`repository` roles on archfit self), `"domain"` matches `subdomain*`.

---

## FP-to-killer matrix

This matrix determines the priority ordering. All three layers together achieve
100% FP elimination on the eval set.

| FP case                             |  String-literal `$PATH`  |       HTTP verb-set        | Import gate  |
| ----------------------------------- | :----------------------: | :------------------------: | :----------: |
| pumba `ctx`                         | **kills** (ctx ≠ string) |     no (Delete ∈ set)      |    kills     |
| codegraph `url`                     | **kills** (url ≠ string) |       no (get ∈ set)       |    maybe     |
| ccgram `fixture`/`event`/`register` |  no (args are literals)  | **kills** (not HTTP verbs) | irrelevant\* |

\*ccgram imports aiohttp (same `@recv.verb(path)` family as fastapi). An
import-only gate would _pass_ for ccgram and all 167 FPs survive. The verb-set
constraint is the required fix for ccgram.

Layer 1 + Layer 2 together eliminate all three validated FP classes. Layer 3
(import gate) is the backstop for the broad unvalidated class and enables the
confidence policy.

---

## Layer 1 — String-literal `$PATH` constraint (YAML, highest priority)

**Fix**: add `constraints` to every route rule requiring `$PATH` to be a string
literal AST node. This is native ast-grep — zero adapter change, zero runtime cost.

### Per-language node kinds

| Language   | String literal kind(s)                                   |
| ---------- | -------------------------------------------------------- |
| Go         | `any: [interpreted_string_literal, raw_string_literal]`  |
| TypeScript | `string` (covers template literals separately if needed) |
| Python     | `string` (covers f-strings, raw strings)                 |
| Rust       | `string_literal`                                         |

### Go example

```yaml
id: go-route-gin
language: go
rule:
  any:
    - pattern: $RECV.GET($PATH, $$$)
    - pattern: $RECV.POST($PATH, $$$)
    # …
constraints:
  PATH:
    any:
      - kind: interpreted_string_literal
      - kind: raw_string_literal
```

Apply the same `constraints: {PATH: {kind: string}}` block to all TypeScript,
Python, and Rust route rules. For Go use the `any` form above.

**Kills**: pumba (ctx ≠ string), codegraph (url ≠ string).

**Does NOT require**: any adapter change.

**Ceiling**: does not filter by receiver type or framework import.

---

## Layer 2 — HTTP verb-set constraint on Python decorator rule (YAML)

**Fix**: add a regex constraint on `$VERB` in `py-route-decorator` requiring it to
be one of the HTTP verbs used by fastapi/flask/starlette/aiohttp.

```yaml
id: py-route-decorator
language: Python
rule:
  kind: decorator
  pattern: "@$RECV.$VERB($PATH)"
constraints:
  VERB:
    regex: "^(get|post|put|delete|patch|head|options|route|websocket|on_startup|on_shutdown)$"
  PATH:
    kind: string
```

The `$VERB` regex is the complete set of decorator method names used by the four
frameworks this rule covers. `route` covers Flask's `@app.route`. `websocket`,
`on_startup`, `on_shutdown` cover aiohttp/starlette event decorators that are
legitimately part of the framework surface.

**Kills**: all 167 ccgram FPs (`fixture`, `event`, `register`, `command`, `param`,
etc. are not in the set).

**Does NOT require**: any adapter change.

**Note on Go/TS/Rust verb sets**: Go and TS route rules already enumerate HTTP
verbs explicitly (`$RECV.GET(...)`, `$RECV.get(...)`), so they are already
verb-constrained by structure. Rust attribute rules use `#[get(...)]` etc. —
same. Only Python needed an explicit constraint.

---

## Layer 3 — Import-gate two-signal adapter (adapter, backstop)

**Purpose**: structural backstop for cases that Layers 1+2 miss (e.g. a
non-string metavariable that happens to be a string constant, or a new framework
whose shape resembles another), and the enabler for the confidence policy (§4).

**Principle**: a route fact is promoted to `framework-confirmed` only when the
same file also has a matching framework-import signal detected in a separate
ast-grep rule pass. If no import rule exists for the framework yet, the route is
kept (fail-open) to avoid silently dropping true positives.

### 3a — Framework groups and import-signal rules

Each framework group maps to one or more import signatures per language.

**Go** — one import-signal rule per framework group:

```yaml
id: go-import-gin
language: go
rule:
  pattern: $IMPORT
  kind: import_spec
  has:
    pattern: '"github.com/gin-gonic/gin"'
```

Signal rule IDs follow the convention `{lang}-import-{group}`. Add one per
entry in the table below. The adapter matches on prefix `{lang}-import-`.

| Group         | Go import paths (regex)                                           |
| ------------- | ----------------------------------------------------------------- |
| `gin`         | `gin-gonic/gin`                                                   |
| `echo`        | `labstack/echo`                                                   |
| `chi`         | `go-chi/chi` (v4 and `/v5`)                                       |
| `fiber`       | `gofiber/fiber`                                                   |
| `gorilla/mux` | `gorilla/mux`                                                     |
| `net/http`    | `net/http` (stdlib — always present; treated as always-confirmed) |

**TypeScript**:

| Group     | Import path regex |
| --------- | ----------------- |
| `express` | `^express$`       |
| `koa`     | `^koa$`           |
| `fastify` | `^fastify$`       |
| `nest`    | `^@nestjs/`       |

**Python**:

| Group     | Import token                                        |
| --------- | --------------------------------------------------- |
| `fastapi` | `fastapi`, `flask`, `starlette`, `aiohttp`, `sanic` |
| `django`  | `django`                                            |

**Rust**:

| Group   | `use` path            |
| ------- | --------------------- |
| `actix` | `actix_web`, `rocket` |
| `axum`  | `axum`, `warp`        |

### 3b — `kindInfo` extension: `IsSignal` field

Extend `kindInfo` in `syntax.go`:

```go
type kindInfo struct {
    Kind      string
    Framework string  // canonical group label (e.g. "gin", "express")
    IsSignal  bool    // true = import-signal rule, never emitted as a SyntaxFact
}
```

Add signal ruleId→kindInfo entries to each `*RuleKinds` map with `IsSignal: true`.
Example:

```go
"go-import-gin":     {Kind: kindRoute, Framework: "gin", IsSignal: true},
"go-import-chi":     {Kind: kindRoute, Framework: "chi", IsSignal: true},
```

### 3c — Integration point in `syntax.go`

The gate lives entirely in `Syntax()`, between raw parse and fact append. Two
passes over `raw` per language:

**Pass 1** — build the per-file confirmed-framework set:

```go
type fileFrameworkSet map[string]map[string]bool // file → set of confirmed group labels

confirmed := fileFrameworkSet{}
for _, m := range raw {
    ki, known := ruleKinds[m.RuleID]
    if !known || !ki.IsSignal {
        continue
    }
    if confirmed[m.File] == nil {
        confirmed[m.File] = map[string]bool{}
    }
    confirmed[m.File][ki.Framework] = true
}
```

**Pass 2** — emit facts, gate route facts:

```go
for _, m := range raw {
    ki, known := ruleKinds[m.RuleID]
    if !known || ki.IsSignal {
        continue  // signal rules are never emitted
    }
    // … name := nameFromMatch …

    // Gate: route fact requires import confirmation for the framework group.
    // Special case: net/http is stdlib — always confirmed.
    // Fail-open: if no signal rule exists for this group, emit anyway (unknown).
    if ki.Kind == kindRoute && ki.Framework != "" && ki.Framework != "net/http" {
        fileConfirmed := confirmed[m.File]
        if fileConfirmed != nil && !fileConfirmed[ki.Framework] {
            // File seen, import rules ran, no match → drop fact.
            continue
        }
        // fileConfirmed == nil → no signal rule ran yet → fail-open, keep fact.
    }

    facts = append(facts, diagnostic.SyntaxFact{ … })
}
```

**Note on `net/http`**: `net/http` is always-confirmed (stdlib); the gorilla/mux
and net/http route rules emit HandleFunc at the same site (identical patterns) —
the import gate resolves this by requiring a `gorilla/mux` import for the gorilla
rule. When only `net/http` is imported, only the `go-route-net-http` fact
survives.

### 3d — Fail-open vs fail-closed

**Fail-open**: when no import-signal rule exists for a framework group (new
framework added to YAML before its signal rule is written), the route fact is
kept. Adding a new framework does not silently suppress routes.

Implementers must add the signal rule at the same time as the route rule. The
extensibility pattern (§5) makes this a single co-located change.

---

## Confidence policy (§4)

| Signal state                                        | `role_confidence` | `roles.go` change?                                     |
| --------------------------------------------------- | ----------------- | ------------------------------------------------------ |
| Framework-confirmed (import gate matched)           | `high`            | No — current behavior preserved                        |
| Layer 1+2 only (string-literal + verb-set, no gate) | `medium`          | Yes — route facts not confirmed by import get `medium` |
| Unconfirmed route (neither layer 1+2 nor gate)      | drop              | Never emitted                                          |

**Implementation**: `roles.go` (`DeriveRoles`) is a core-ring package — it cannot
read the import-gate result. The adapter sets a new boolean field
`SyntaxFact.FrameworkConfirmed bool` (set to `true` when the import gate passed).
`deriveOne` in `roles.go` checks: if `f.Kind == "route"` and
`!f.FrameworkConfirmed`, assign `ConfMedium` instead of `ConfHigh`.

If the import gate (Layer 3) is not shipped in the first implementation, all
route facts surviving Layer 1+2 remain `high`. Confidence downgrade requires
Layer 3.

**Design rationale**: downstream gate rules (`forbidden_role_dependency`,
`public_api_max`) filter on confidence. Keeping false positives at `high`
pollutes those gates. `medium` routes are still surfaced in scan output — they
just don't trip high-confidence gates.

---

## `roleFromPath` word-boundary fix

### Current bug

`strings.Contains(lower, "repo")` matches:

- `update_report.go` → false `repository`
- `subdomain/` → false `domain` (via `"domain"` token)

### Fix: path-segment tokenizer

Replace `firstMatch(lower, ...)` with a segment-aware matcher in `roles.go`.
Tokenize on `/ \ - _ .` (path separators and identifier separators), then match
whole tokens. Plural forms must be included to avoid regressing common directory
names.

**Token table** (typed, replaces ad-hoc string lists):

```go
type roleToken struct {
    Token  string
    Plural string // empty = no plural
}

var handlerTokens  = []roleToken{{"handler","handlers"}, {"controller","controllers"}, {"route","routes"}, {"router","routers"}}
var repoTokens     = []roleToken{{"repository","repositories"}, {"repo","repos"}, {"storage","storages"}, {"persistence",""}}
var serviceTokens  = []roleToken{{"service","services"}, {"usecase","usecases"}, {"application","applications"}}
var domainTokens   = []roleToken{{"domain","domains"}, {"model","models"}, {"entity","entities"}}
```

`firstSegmentMatch` splits the lowercased path on `[/\\_\-.]`, then for each
token in order checks whether any path segment equals the token or its plural:

```go
func firstSegmentMatch(lower string, tokens []roleToken) (string, bool) {
    segs := strings.FieldsFunc(lower, func(r rune) bool {
        return r == '/' || r == '\\' || r == '_' || r == '-' || r == '.'
    })
    for _, seg := range segs {
        for _, t := range tokens {
            if seg == t.Token || (t.Plural != "" && seg == t.Plural) {
                return t.Token, true
            }
        }
    }
    return "", false
}
```

The precedence ordering (handler > repo > service > domain) is preserved by call
order, not map iteration.

**What this kills**: `update_report` → no match (`report` ≠ `repo`/`repos`),
`subdomain` → no match (`subdomain` ≠ `domain`/`domains`).

**No regression**: `handlers/`, `repositories/`, `services/`, `domain/`,
`models/`, `controller/`, `routes/`, `storage/`, `repo/` all still match.

---

## Extensibility: adding a language or framework

### Adding a new framework to an existing language

1. In the language `.yml`: add a route rule + an import-signal rule (same file,
   co-located comments).
2. In `syntax.go` `*RuleKinds` map: add two entries — the route `kindInfo` and
   the signal `kindInfo` with `IsSignal: true`.
3. Update the framework-group table in this doc.

That is the complete change set. No new types, no new registry, no adapter
structural change.

### Adding a new language

1. Create `rules/{lang}.yml` with declaration + route + import-signal rules.
2. Add `//go:embed rules/{lang}.yml` + `{lang}Rules` var in `syntax.go`.
3. Add `langRuleKinds[lang{Lang}]` map.
4. Add `embeddedRules[lang{Lang}]` entry.
5. Extend `isExported` switch case.

No other files change for declaration + route facts. `roles.go` is
language-agnostic and unchanged.

---

## ast-grep capability limits confirmed

- **No whole-file predicate**: ast-grep `inside`/`has`/`follows`/`precedes` are
  ancestor/descendant/sibling relations on the syntax tree. An import statement
  is not an ancestor of a route call — they are siblings under the file root.
  Therefore "file also imports express" cannot be expressed as a single rule
  constraint. The two-signal adapter (§3) is necessary.
- **`constraints:` with `kind`/`regex` per metavar IS supported** in sg 0.44.0.
  The `$PATH: {kind: ...}` and `$VERB: {regex: ...}` forms are valid.
- **Go string literal split**: Go has no single `string_literal` kind; use
  `any: [{kind: interpreted_string_literal}, {kind: raw_string_literal}]`.
- **net/http + gorilla duplicate**: both rules use `$RECV.HandleFunc(...)` — same
  pattern, two ruleIds. A file with only a `net/http` import currently emits two
  route facts per call (one chi, one fiber overlap for Go route rules too, for the
  same reason). Layer 3 (import gate) is the correct fix; Layers 1+2 do not
  address this.

---

## Implementation order

1. **Layer 1** — add `constraints: {PATH: {kind: …}}` to all route rules in all
   four YAML files. Verify pumba + codegraph FPs → 0. Golden regen + inspect diff.
2. **Layer 2** — add `constraints: {VERB: {regex: …}}` to `py-route-decorator`.
   Verify ccgram FPs → 0. Golden regen.
3. **roleFromPath** — replace `firstMatch` with segment-tokenizer. Unit-test all
   twelve confirmed FP path patterns + twelve TP patterns.
4. **Layer 3** (import gate) — extend `kindInfo`, add signal rules, add two-pass
   logic in `Syntax()`, add `SyntaxFact.FrameworkConfirmed`, update `deriveOne`
   confidence. Verify net/http+gorilla duplicate collapse.
5. **Acceptance check**: re-run archfit on pumba/ccgram/codegraph, confirm
   route-fact counts and role assignments against source; zero FP on eval set; no
   TP regression on archfit self.

Layers 1+2+roleFromPath are self-contained YAML + one-function changes. Layer 3
touches the adapter and model but has no coupling to any other dimension. They
can ship separately.
