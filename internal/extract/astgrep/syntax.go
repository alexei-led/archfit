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

// SyntaxFact Kind constants for Go and TypeScript declarations and routes.
const (
	kindFunction   = "function"
	kindMethod     = "method"
	kindStruct     = "struct"
	kindInterface  = "interface"
	kindTypeAlias  = "type_alias"
	kindRoute      = "route"
	kindClass      = "class"
	kindEnum       = "enum"
	kindAnnotation = "annotation"
)

// Language identifier constants used as keys in embeddedRules and langRuleKinds.
const (
	langGo         = "go"
	langTypeScript = "typescript"
)

// kindInfo maps a ruleId to the SyntaxFact Kind and optional Framework.
type kindInfo struct {
	Kind      string
	Framework string
}

// goRuleKinds maps each go.yml ruleId to its Kind and Framework.
// Route rules carry a non-empty Framework; declaration rules do not.
var goRuleKinds = map[string]kindInfo{
	"go-func":                  {Kind: kindFunction},
	"go-method":                {Kind: kindMethod},
	"go-struct":                {Kind: kindStruct},
	"go-interface":             {Kind: kindInterface},
	"go-type-alias":            {Kind: kindTypeAlias},
	"go-route-net-http":        {Kind: kindRoute, Framework: "net/http"},
	"go-route-net-http-handle": {Kind: kindRoute, Framework: "net/http"},
	"go-route-gin":             {Kind: kindRoute, Framework: "gin"},
	"go-route-echo":            {Kind: kindRoute, Framework: "echo"},
	"go-route-chi":             {Kind: kindRoute, Framework: "chi"},
	"go-route-fiber":           {Kind: kindRoute, Framework: "fiber"},
	"go-route-gorilla":         {Kind: kindRoute, Framework: "gorilla/mux"},
}

// tsRuleKinds maps each typescript.yml ruleId to its Kind and Framework.
// Route rules carry a non-empty Framework; declaration and decorator rules do not.
// express/koa/fastify share the same call shape and are labelled "express" (see typescript.yml).
var tsRuleKinds = map[string]kindInfo{
	"ts-func":                  {Kind: kindFunction},
	"ts-class":                 {Kind: kindClass},
	"ts-interface":             {Kind: kindInterface},
	"ts-enum":                  {Kind: kindEnum},
	"ts-type-alias":            {Kind: kindTypeAlias},
	"ts-method":                {Kind: kindMethod},
	"ts-decorator":             {Kind: kindAnnotation},
	"ts-route-express":         {Kind: kindRoute, Framework: "express"},
	"ts-route-nest-controller": {Kind: kindRoute, Framework: "nest"},
	"ts-route-nest-method":     {Kind: kindRoute, Framework: "nest"},
}

// langRuleKinds maps a language identifier to its ruleId→kindInfo table.
// Add new language entries here when Tasks 5-6 add Python/Rust rules.
var langRuleKinds = map[string]map[string]kindInfo{
	langGo:         goRuleKinds,
	langTypeScript: tsRuleKinds,
}

// embeddedRules maps language identifiers (as passed in langs) to their embedded YAML.
// Add new entries here when Tasks 5-6 add Python/Rust rules.
var embeddedRules = map[string]string{
	langGo:         goRules,
	langTypeScript: tsRules,
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
		return nil, diagnostic.Coverage{Tool: toolName, Status: statusAbsent}, nil
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
		if len(out.Stdout) == 0 {
			continue
		}

		var raw []sgSyntaxMatch
		if err := json.Unmarshal(out.Stdout, &raw); err != nil {
			return nil, diagnostic.Coverage{}, fmt.Errorf("astgrep: parse syntax output for %q: %w", lang, err)
		}

		ruleKinds := langRuleKinds[lang]
		for _, m := range raw {
			ki, known := ruleKinds[m.RuleID]
			if !known {
				// Unknown ruleId — skip; future rules won't break existing facts.
				continue
			}

			name := nameFromMatch(m)
			if name == "" {
				continue
			}

			exported := isExported(lang, m.RuleID, name)

			facts = append(facts, diagnostic.SyntaxFact{
				Language:  lang,
				File:      m.File,
				Kind:      ki.Kind,
				Name:      name,
				Exported:  exported,
				StartLine: m.Range.Start.Line + 1, // 0-based → 1-based
				EndLine:   m.Range.End.Line + 1,
				Framework: ki.Framework,
			})
		}
	}

	diagnostic.SortSyntaxFacts(facts)
	cov := diagnostic.Coverage{Tool: toolName, Status: statusOK}
	return facts, cov, nil
}

// nameFromMatch extracts the declaration name from a syntax match.
// For most rules the name is in metaVariables.single["NAME"].text.
// For go-type-alias it is absent from metaVariables, so we fall back to
// parsing the match text ("type ID = int" → "ID").
func nameFromMatch(m sgSyntaxMatch) string {
	if nv, ok := m.MetaVariables.Single["NAME"]; ok && nv.Text != "" {
		return nv.Text
	}
	// Route rules: use $PATH as the name (the URL pattern string).
	if pv, ok := m.MetaVariables.Single["PATH"]; ok && pv.Text != "" {
		return strings.Trim(pv.Text, `"`+"`")
	}
	// Fallback for go-type-alias: extract from declaration text.
	if sub := goTypeAliasNameRe.FindStringSubmatch(m.Text); len(sub) == 2 {
		return sub[1]
	}
	return ""
}

// isExported returns true if the fact represents an exported identifier.
//
// Go: exported means the name starts with an uppercase letter; route rules are never exported.
// TypeScript: all declaration rules require inside:export_statement so they are always
// exported; route and annotation rules are never exported.
func isExported(lang, ruleID, name string) bool {
	switch lang {
	case langTypeScript:
		// Route and annotation ruleIds start with "ts-route-" or equal "ts-decorator".
		if strings.HasPrefix(ruleID, "ts-route-") || ruleID == "ts-decorator" {
			return false
		}
		return true // inside: export_statement guarantees export
	default: // go and future languages: uppercase-first convention
		if strings.HasPrefix(ruleID, "go-route-") {
			return false
		}
		return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
	}
}

// statusAbsent is the Coverage status for a missing tool.
const statusAbsent = "absent"

// statusOK is the Coverage status when the tool ran and produced output (or no matches).
const statusOK = "ok"

// Compile-time interface check.
var _ ports.SyntaxProvider = (*Adapter)(nil)
