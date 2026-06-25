// Package astgrep implements ports.PatternProvider and ports.SyntaxProvider
// using the "sg" (ast-grep) binary. Syntax() runs embedded per-language rules
// and maps ruleId→Kind+Framework into SyntaxFacts sorted by (File, StartLine, Kind, Name).
package astgrep

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

//go:embed rules/go.yml
var goRules string

//go:embed rules/typescript.yml
var tsRules string

//go:embed rules/python.yml
var pyRules string

//go:embed rules/rust.yml
var rustRules string

// SyntaxFact Kind constants for Go and TypeScript declarations and routes.
const (
	kindFunction    = "function"
	kindMethod      = "method"
	kindStruct      = "struct"
	kindInterface   = "interface"
	kindTypeAlias   = "type_alias"
	kindRoute       = "route"
	kindClass       = "class"
	kindEnum        = "enum"
	kindAnnotation  = "annotation"
	kindTestImport  = "test_import"
	kindUnsafeOp    = "unsafe_op"
	kindStructField = "struct_field"
	kindPanicOp     = "panic_op"
	kindGlobalState = "global_state"
	kindTypeLeak    = "type_leak"
	kindLazyImport  = "lazy_import"
	kindTestFn      = "test_fn"
)

// Language identifier constants used as keys in embeddedRules and langRuleKinds.
const (
	langGo         = "go"
	langTypeScript = "typescript"
	langPython     = "python"
	langRust       = "rust"
)

// kindInfo maps a ruleId to the SyntaxFact Kind and optional Framework.
// IsSignal=true marks import-signal rules: they are never emitted as SyntaxFacts;
// instead they populate the per-file framework-confirmed map used by the two-pass
// import-gating logic. One signal rule per framework group is sufficient.
//
// Extensibility: to add a new framework, add one route rule + one signal rule in
// the YAML + one entry in the *RuleKinds map with IsSignal:true. No other changes.
type kindInfo struct {
	Kind      string
	Framework string
	IsSignal  bool // true = import-signal rule; never emitted as a SyntaxFact
}

// Framework group constants: each route rule belongs to one group; the matching
// signal rule confirms that the file imports that group. Groups are strings so
// new frameworks need no code change beyond the YAML + kindInfo entry.
const (
	fwGroupNetHTTP = "net/http"    // net/http + gorilla/mux (gorilla always imports net/http too)
	fwGroupGin     = "gin"         // gin
	fwGroupEcho    = "echo"        // echo
	fwGroupChi     = "chi"         // chi
	fwGroupFiber   = "fiber"       // fiber
	fwGroupGorilla = "gorilla/mux" // gorilla/mux (confirmed via go-import-gorilla)
	fwGroupExpress = "express"     // express/koa/fastify
	fwGroupNest    = "nest"        // NestJS
	fwGroupFastAPI = "fastapi"     // fastapi/flask/starlette/aiohttp/sanic
	fwGroupDjango  = "django"      // django
	fwGroupActix   = "actix"       // actix-web/rocket
	fwGroupAxum    = "axum"        // axum/warp
	// Test-framework groups (used by test_import rules).
	fwGroupTestifyMock   = "testify/mock"
	fwGroupTestifyAssert = "testify/assert"
	fwGroupTestifyRequir = "testify/require"
	fwGroupTestifySuite  = "testify/suite"
	fwGroupGomockUber    = "gomock/uber"
	fwGroupGomockGolang  = "gomock/golang"
	fwGroupSQLMock       = "go-sqlmock"
	fwGroupJest          = "jest"
	fwGroupVitest        = "vitest"
	fwGroupMocha         = "mocha"
	fwGroupSinon         = "sinon"
	fwGroupPytest        = "pytest"
	fwGroupUnittest      = "unittest"
	fwGroupMock          = "mock"
	fwGroupMockall       = "mockall"
	fwGroupProptest      = "proptest"
)

// goRuleKinds maps each go.yml ruleId to its Kind and Framework.
// Route rules carry a non-empty Framework; declaration rules do not.
// Signal rules have IsSignal=true and are never emitted as SyntaxFacts.
var goRuleKinds = map[string]kindInfo{
	"go-func":                  {Kind: kindFunction},
	"go-method":                {Kind: kindMethod},
	"go-struct":                {Kind: kindStruct},
	"go-interface":             {Kind: kindInterface},
	"go-type-alias":            {Kind: kindTypeAlias},
	"go-route-net-http":        {Kind: kindRoute, Framework: fwGroupNetHTTP},
	"go-route-net-http-handle": {Kind: kindRoute, Framework: fwGroupNetHTTP},
	"go-route-gin":             {Kind: kindRoute, Framework: fwGroupGin},
	"go-route-echo":            {Kind: kindRoute, Framework: fwGroupEcho},
	"go-route-chi":             {Kind: kindRoute, Framework: fwGroupChi},
	"go-route-fiber":           {Kind: kindRoute, Framework: fwGroupFiber},
	"go-route-gorilla":         {Kind: kindRoute, Framework: fwGroupGorilla},
	// Import-signal rules (IsSignal=true): confirm framework import per file.
	"go-import-net-http": {Framework: fwGroupNetHTTP, IsSignal: true},
	"go-import-gin":      {Framework: fwGroupGin, IsSignal: true},
	"go-import-echo":     {Framework: fwGroupEcho, IsSignal: true},
	"go-import-chi":      {Framework: fwGroupChi, IsSignal: true},
	"go-import-fiber":    {Framework: fwGroupFiber, IsSignal: true},
	"go-import-gorilla":  {Framework: fwGroupGorilla, IsSignal: true},
	// Struct-field rules: one fact per exported struct; Count = estimated field count.
	"go-struct-field": {Kind: kindStructField},
	// Panic-operation rules: report-only, never gates.
	"go-panic": {Kind: kindPanicOp},
	// Type-leak rules: exported struct fields with qualified external-package types.
	// report-only (Cat 5); consumed by public_api_type_leak rule.
	"go-type-leak": {Kind: kindTypeLeak},
	// Test-function detection: emit test_fn facts for Go test functions (^Test prefix).
	"go-test-fn": {Kind: kindTestFn},
	// Test-import rules: emit test_import facts for production Go files.
	"go-test-import-testify-mock":    {Kind: kindTestImport, Framework: fwGroupTestifyMock},
	"go-test-import-testify-assert":  {Kind: kindTestImport, Framework: fwGroupTestifyAssert},
	"go-test-import-testify-require": {Kind: kindTestImport, Framework: fwGroupTestifyRequir},
	"go-test-import-testify-suite":   {Kind: kindTestImport, Framework: fwGroupTestifySuite},
	"go-test-import-gomock-uber":     {Kind: kindTestImport, Framework: fwGroupGomockUber},
	"go-test-import-gomock-golang":   {Kind: kindTestImport, Framework: fwGroupGomockGolang},
	"go-test-import-sqlmock":         {Kind: kindTestImport, Framework: fwGroupSQLMock},
}

// tsRuleKinds maps each typescript.yml ruleId to its Kind and Framework.
// Route rules carry a non-empty Framework; declaration and decorator rules do not.
// express/koa/fastify share the same call shape and are labelled "express" (see typescript.yml).
// Signal rules have IsSignal=true and are never emitted as SyntaxFacts.
var tsRuleKinds = map[string]kindInfo{
	"ts-func":                  {Kind: kindFunction},
	"ts-class":                 {Kind: kindClass},
	"ts-interface":             {Kind: kindInterface},
	"ts-enum":                  {Kind: kindEnum},
	"ts-type-alias":            {Kind: kindTypeAlias},
	"ts-method":                {Kind: kindMethod},
	"ts-decorator":             {Kind: kindAnnotation},
	"ts-route-express":         {Kind: kindRoute, Framework: fwGroupExpress},
	"ts-route-nest-controller": {Kind: kindRoute, Framework: fwGroupNest},
	"ts-route-nest-method":     {Kind: kindRoute, Framework: fwGroupNest},
	// Import-signal rules (IsSignal=true): confirm framework import per file.
	"ts-import-express": {Framework: fwGroupExpress, IsSignal: true},
	"ts-import-nest":    {Framework: fwGroupNest, IsSignal: true},
	// Test-import rules: emit test_import facts for production TS files.
	"ts-test-import-jest":   {Kind: kindTestImport, Framework: fwGroupJest},
	"ts-test-import-vitest": {Kind: kindTestImport, Framework: fwGroupVitest},
	"ts-test-import-mocha":  {Kind: kindTestImport, Framework: fwGroupMocha},
	"ts-test-import-sinon":  {Kind: kindTestImport, Framework: fwGroupSinon},
}

// pyRuleKinds maps each python.yml ruleId to its Kind and Framework.
// Route rules carry a non-empty Framework; declaration and decorator rules do not.
// fastapi/flask/starlette/aiohttp share the same @recv.verb(path) shape and are
// labelled "fastapi" as the canonical representative (see python.yml).
// Signal rules have IsSignal=true and are never emitted as SyntaxFacts.
var pyRuleKinds = map[string]kindInfo{
	"py-func":                {Kind: kindFunction},
	"py-class":               {Kind: kindClass},
	"py-decorator":           {Kind: kindAnnotation},
	"py-route-decorator":     {Kind: kindRoute, Framework: fwGroupFastAPI},
	"py-route-django-path":   {Kind: kindRoute, Framework: fwGroupDjango},
	"py-route-django-repath": {Kind: kindRoute, Framework: fwGroupDjango},
	// Import-signal rules (IsSignal=true): confirm framework import per file.
	"py-import-fastapi": {Framework: fwGroupFastAPI, IsSignal: true},
	"py-import-django":  {Framework: fwGroupDjango, IsSignal: true},
	// Test-function detection: emit test_fn facts for Python test functions (^test_ prefix).
	"py-test-fn": {Kind: kindTestFn},
	// Test-import rules: emit test_import facts for production Python files.
	"py-test-import-pytest":   {Kind: kindTestImport, Framework: fwGroupPytest},
	"py-test-import-unittest": {Kind: kindTestImport, Framework: fwGroupUnittest},
	"py-test-import-mock":     {Kind: kindTestImport, Framework: fwGroupMock},
	// Lazy-import rules: emit lazy_import facts for in-function import statements.
	// py-lazy-import-module: `import X` inside a function body.
	// py-lazy-import-from:   `from X import Y` inside a function body.
	"py-lazy-import-module": {Kind: kindLazyImport},
	"py-lazy-import-from":   {Kind: kindLazyImport},
}

// rustRuleKinds maps each rust.yml ruleId to its Kind and Framework.
// Route rules carry a non-empty Framework; declaration and attribute rules do not.
// actix/rocket share the same #[verb(path)] attribute shape and are labelled "actix".
// axum/warp share the same .route(path, handler) builder shape and are labelled "axum".
// Signal rules have IsSignal=true and are never emitted as SyntaxFacts.
var rustRuleKinds = map[string]kindInfo{
	"rs-func":               {Kind: kindFunction},
	"rs-struct":             {Kind: kindStruct},
	"rs-enum":               {Kind: kindEnum},
	"rs-trait":              {Kind: kindInterface},
	"rs-impl":               {Kind: kindMethod},
	"rs-mod":                {Kind: kindStruct},
	"rs-attribute":          {Kind: kindAnnotation},
	"rs-route-actix-rocket": {Kind: kindRoute, Framework: fwGroupActix},
	"rs-route-axum-warp":    {Kind: kindRoute, Framework: fwGroupAxum},
	// Import-signal rules (IsSignal=true): confirm framework import per file.
	"rs-import-actix": {Framework: fwGroupActix, IsSignal: true},
	"rs-import-axum":  {Framework: fwGroupAxum, IsSignal: true},
	// Test-function detection: emit test_fn facts for Rust test functions (^test_ name prefix).
	// Ceiling: misses #[test] fn check_foo() where name doesn't start with test_.
	// Practical proxy for test density; use SCIP for semantic precision.
	"rs-test-fn": {Kind: kindTestFn},
	// Test-import rules: emit test_import facts for production Rust files.
	"rs-test-import-mockall":  {Kind: kindTestImport, Framework: fwGroupMockall},
	"rs-test-import-proptest": {Kind: kindTestImport, Framework: fwGroupProptest},
	// Safety-operation rules: report-only, never gates.
	"rs-unsafe-block": {Kind: kindUnsafeOp},
	"rs-unsafe-cell":  {Kind: kindUnsafeOp},
	"rs-raw-cast":     {Kind: kindUnsafeOp},
	"rs-transmute":    {Kind: kindUnsafeOp},
	// Struct-field rules: one fact per pub struct with named fields; Count = estimated field count.
	"rs-struct-field": {Kind: kindStructField},
	// Panic-operation rules: report-only, never gates.
	"rs-unwrap": {Kind: kindPanicOp},
	"rs-expect": {Kind: kindPanicOp},
	"rs-panic":  {Kind: kindPanicOp},
	// Global-state rules: static mut, Atomic*, OnceLock — report-only, never gates.
	// AtomicU32/Bool/etc. ID-generators are idiomatic Rust; these are info signal only.
	"rs-static-mut":      {Kind: kindGlobalState},
	"rs-static-atomic":   {Kind: kindGlobalState},
	"rs-static-oncelock": {Kind: kindGlobalState},
}

// langRuleKinds maps a language identifier to its ruleId→kindInfo table.
var langRuleKinds = map[string]map[string]kindInfo{
	langGo:         goRuleKinds,
	langTypeScript: tsRuleKinds,
	langPython:     pyRuleKinds,
	langRust:       rustRuleKinds,
}

// embeddedRules maps language identifiers (as passed in langs) to their embedded YAML.
var embeddedRules = map[string]string{
	langGo:         goRules,
	langTypeScript: tsRules,
	langPython:     pyRules,
	langRust:       rustRules,
}

// sgSyntaxMatch is the JSON shape ast-grep emits for --json=compact scan output.
// It extends sgMatch with the end line and metaVariables needed for SyntaxFacts.
type sgSyntaxMatch struct {
	Text  string `json:"text"`
	Range struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
		End struct {
			Line int `json:"line"`
		} `json:"end"`
	} `json:"range"`
	File          string `json:"file"`
	RuleID        string `json:"ruleId"`
	MetaVariables struct {
		Single map[string]struct {
			Text string `json:"text"`
		} `json:"single"`
	} `json:"metaVariables"`
}

// goTypeAliasNameRe extracts the name from a type-alias declaration text
// ("type ID = int" → "ID"). Used when $NAME metavar is unavailable.
var goTypeAliasNameRe = regexp.MustCompile(`^type\s+(\w+)`)

// Syntax runs embedded ast-grep rules for each requested language that has rules
// defined, collects SyntaxFacts, and returns them sorted by (File, StartLine,
// Kind, Name). A missing "sg" binary returns empty facts with status "absent" —
// never an error. Languages with no embedded rules are silently skipped.
func (a *Adapter) Syntax(ctx context.Context, s scope.Scope, langs []string) ([]diagnostic.SyntaxFact, diagnostic.Coverage, error) {
	_, ok := a.runner.Detect(ctx, "sg")
	if !ok {
		return nil, diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusAbsent}, nil
	}

	var facts []diagnostic.SyntaxFact

	for _, lang := range langs {
		rules, hasRules := embeddedRules[lang]
		if !hasRules {
			// No rules for this language yet — skip without error.
			continue
		}

		out, err := a.runner.Run(ctx, toolrun.ToolCmd{
			Name:    "sg",
			Args:    []string{"scan", "--inline-rules", rules, "--json=compact", "."},
			WorkDir: s.Root,
		})
		if err != nil {
			return nil, diagnostic.Coverage{}, fmt.Errorf("astgrep: syntax scan for %q: %w", lang, err)
		}
		// sg present but exited non-zero with empty stdout → rule file was rejected
		// (e.g. YAML parse error). Silently continuing would produce zero facts and
		// report status=ok — a false green. Surface it as a partial/degraded result.
		if out.ExitCode != 0 && len(out.Stdout) == 0 {
			reason := fmt.Sprintf("sg rejected rule file for %q (exit %d): %s", lang, out.ExitCode, strings.TrimSpace(string(out.Stderr)))
			return nil, diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusPartial, Reason: reason}, nil
		}
		if len(out.Stdout) == 0 {
			continue
		}

		var raw []sgSyntaxMatch
		if err := json.Unmarshal(out.Stdout, &raw); err != nil {
			return nil, diagnostic.Coverage{}, fmt.Errorf("astgrep: parse syntax output for %q: %w", lang, err)
		}

		ruleKinds := langRuleKinds[lang]

		// Pass 1: collect import-signal matches to build the confirmed map.
		// confirmed[file][frameworkGroup] = true when that file imports the group's framework.
		confirmed := make(map[string]map[string]bool)
		for _, m := range raw {
			ki, known := ruleKinds[m.RuleID]
			if !known || !ki.IsSignal {
				continue
			}
			if confirmed[m.File] == nil {
				confirmed[m.File] = make(map[string]bool)
			}
			confirmed[m.File][ki.Framework] = true
		}

		// Pass 2: emit declaration facts unconditionally; emit route facts only
		// when the file's framework import was confirmed in pass 1.
		for _, m := range raw {
			ki, known := ruleKinds[m.RuleID]
			if !known || ki.IsSignal {
				// Signal rules are never emitted as facts; unknown ruleIds are skipped.
				continue
			}

			name := nameFromMatch(m)
			// test_import rules use pattern forms (e.g. bare import, $$$ multi-metavar,
			// regex) that don't populate $NAME/$PATH. Use the framework label as the
			// name — it is non-empty for all test_import entries in kindInfo maps.
			if name == "" && ki.Kind == kindTestImport {
				name = ki.Framework
			}
			// unsafe_op rules have no $NAME metavar. Use the ruleId (minus the "rs-"
			// prefix) as the name so the fact is identifiable in displays and tests.
			if name == "" && ki.Kind == kindUnsafeOp {
				name = strings.TrimPrefix(m.RuleID, "rs-")
			}
			// panic_op rules have no $NAME metavar. Use the ruleId (minus the
			// language prefix) as the name so the fact is identifiable in displays.
			if name == "" && ki.Kind == kindPanicOp {
				name = strings.TrimPrefix(strings.TrimPrefix(m.RuleID, "rs-"), "go-")
			}
			if name == "" {
				continue
			}

			// Import-gating: route facts require a confirmed framework import.
			// If no signal rule fired for this framework in this file, drop the fact.
			if ki.Kind == kindRoute && ki.Framework != "" {
				if !confirmed[m.File][ki.Framework] {
					continue // no import confirmed → drop (not downgrade)
				}
			}

			exported := isExported(lang, m.RuleID, name)

			// For struct_field facts, estimate the field count from the line range.
			// count = endLine - startLine - 1 (opening and closing brace lines excluded).
			// Negative results (single-line structs) clamp to 0.
			count := 0
			if ki.Kind == kindStructField {
				count = m.Range.End.Line - m.Range.Start.Line - 1
				if count < 0 {
					count = 0
				}
			}

			facts = append(facts, diagnostic.SyntaxFact{
				Language:           lang,
				File:               m.File,
				Kind:               ki.Kind,
				Name:               name,
				Exported:           exported,
				StartLine:          m.Range.Start.Line + 1, // 0-based → 1-based
				EndLine:            m.Range.End.Line + 1,
				Framework:          ki.Framework,
				FrameworkConfirmed: ki.Kind == kindRoute && ki.Framework != "",
				Count:              count,
			})
		}
	}

	diagnostic.SortSyntaxFacts(facts)

	// Count distinct files that produced at least one fact.
	// FilesSeen=0 is reserved for "ran but nothing matched" — callers use it to
	// distinguish zero-match runs from untooled runs (StatusAbsent/StatusPartial).
	seen := make(map[string]struct{}, len(facts))
	for i := range facts {
		seen[facts[i].File] = struct{}{}
	}
	cov := diagnostic.Coverage{Tool: toolName, Status: diagnostic.StatusOK, FilesSeen: len(seen)}
	return facts, cov, nil
}

// nameFromMatch extracts the declaration name from a syntax match.
// For most rules the name is in metaVariables.single["NAME"].text.
// For go-type-alias it is absent from metaVariables, so we fall back to
// parsing the match text ("type ID = int" → "ID").
// For go-type-leak the name is built from $PKG and $TYPE ("pkg.Type").
func nameFromMatch(m sgSyntaxMatch) string {
	if nv, ok := m.MetaVariables.Single["NAME"]; ok && nv.Text != "" {
		return nv.Text
	}
	// Route rules: use $PATH as the name (the URL pattern string).
	if pv, ok := m.MetaVariables.Single["PATH"]; ok && pv.Text != "" {
		return strings.Trim(pv.Text, `"`+"`")
	}
	// go-type-leak: build "pkg.Type" from $PKG and $TYPE metavars.
	pkg := m.MetaVariables.Single["PKG"].Text
	typ := m.MetaVariables.Single["TYPE"].Text
	if pkg != "" && typ != "" {
		return pkg + "." + typ
	}
	// Fallback for go-type-alias: extract from declaration text.
	if sub := goTypeAliasNameRe.FindStringSubmatch(m.Text); len(sub) == 2 {
		return sub[1]
	}
	return ""
}

// isNonExportedGoRule returns true for Go ruleIds that produce signal/structural
// facts rather than exported API surface entries.
func isNonExportedGoRule(ruleID string) bool {
	return strings.HasPrefix(ruleID, "go-route-") || strings.HasPrefix(ruleID, "go-import-") ||
		strings.HasPrefix(ruleID, "go-test-import-") || ruleID == "go-struct-field" ||
		ruleID == "go-type-leak" || ruleID == "go-test-fn"
}

// isNonExportedTSRule returns true for TypeScript ruleIds that are never exported.
func isNonExportedTSRule(ruleID string) bool {
	return strings.HasPrefix(ruleID, "ts-route-") || strings.HasPrefix(ruleID, "ts-import-") ||
		ruleID == "ts-decorator" || strings.HasPrefix(ruleID, "ts-test-import-")
}

// isNonExportedPyRule returns true for Python ruleIds that are never exported.
func isNonExportedPyRule(ruleID string) bool {
	return strings.HasPrefix(ruleID, "py-route-") || strings.HasPrefix(ruleID, "py-import-") ||
		ruleID == "py-decorator" || strings.HasPrefix(ruleID, "py-test-import-") ||
		strings.HasPrefix(ruleID, "py-lazy-import-") || ruleID == "py-test-fn"
}

// isNonExportedRustRule returns true for Rust ruleIds that are never exported.
// struct-field facts represent whole structs with field counts, not API surface.
// global-state facts are module-level statics; visibility is not the relevant signal.
func isNonExportedRustRule(ruleID string) bool {
	return strings.HasPrefix(ruleID, "rs-route-") || strings.HasPrefix(ruleID, "rs-import-") ||
		ruleID == "rs-attribute" || strings.HasPrefix(ruleID, "rs-test-import-") ||
		strings.HasPrefix(ruleID, "rs-unsafe-") || ruleID == "rs-raw-cast" || ruleID == "rs-transmute" ||
		ruleID == "rs-struct-field" || strings.HasPrefix(ruleID, "rs-static-") || ruleID == "rs-test-fn"
}

// isExported returns true if the fact represents an exported identifier.
//
// Go: exported means the name starts with an uppercase letter; route and signal rules are never exported.
// TypeScript: all declaration rules require inside:export_statement so they are always
// exported; route, annotation, and signal rules are never exported.
// Python: public means the name does not start with underscore; route, decorator, and signal rules
// are never exported.
// Rust: pub = has visibility_modifier (the YAML rule already filters for it); route, attribute,
// and signal rules are never exported.
func isExported(lang, ruleID, name string) bool {
	switch lang {
	case langTypeScript:
		if isNonExportedTSRule(ruleID) {
			return false
		}
		return true // inside: export_statement guarantees export
	case langPython:
		if isNonExportedPyRule(ruleID) {
			return false
		}
		// Public = name does not start with underscore.
		return len(name) > 0 && name[0] != '_'
	case langRust:
		if isNonExportedRustRule(ruleID) {
			return false
		}
		// The YAML rule already requires visibility_modifier, so any match is pub.
		return true
	default: // go and future languages: uppercase-first convention
		if isNonExportedGoRule(ruleID) {
			return false
		}
		return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
	}
}

// Compile-time interface check.
var _ ports.SyntaxProvider = (*Adapter)(nil)
