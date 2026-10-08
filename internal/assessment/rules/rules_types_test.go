package rules_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/model/pattern"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

// TestRuleTypesMatchNew holds the published rule type vocabulary
// (policy.RuleTypes, enumerated in archfit.schema.json) equal to what rules.New
// accepts: every listed type loads, and every type the New switch handles is
// listed. Either drift lets the schema and the engine disagree about a config.
func TestRuleTypesMatchNew(t *testing.T) {
	listed := policy.RuleTypes()
	for _, typ := range listed {
		def := minimalRuleDef(typ)
		if _, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{def}}); err != nil {
			t.Errorf("policy.RuleTypes lists %q, but rules.New rejects a minimal rule of it: %v", typ, err)
		}
	}

	handled := newSwitchCases(t)
	slices.Sort(handled)
	sortedListed := slices.Sorted(slices.Values(listed))
	if !slices.Equal(handled, sortedListed) {
		t.Errorf("rules.New handles %v, policy.RuleTypes lists %v", handled, sortedListed)
	}
	if !slices.IsSorted(listed) {
		t.Errorf("policy.RuleTypes = %v, want sorted (it is the schema enum order)", listed)
	}
}

// minimalRuleDef returns the smallest rule of typ that rules.New's validators accept.
func minimalRuleDef(typ string) policy.RuleDef {
	def := policy.RuleDef{ID: "r-" + typ, Type: typ}
	switch typ {
	case typeForbiddenDependency:
		def.From, def.To = "app/**", "lib/**"
	case typePublicAPIMax:
		zero := 0
		def.Max = &zero
	case typeForbiddenPattern:
		def.Patterns = []pattern.Def{{ID: "p-1", Lang: "go", Rule: "panic($$$)"}}
	}
	return def
}

// newSwitchCases parses rules.go and returns the string-literal case values of
// the `switch def.Type` statement inside New.
func newSwitchCases(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "rules.go", nil, 0)
	if err != nil {
		t.Fatalf("parse rules.go: %v", err)
	}
	var cases []string
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "New" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok || !isDefType(sw.Tag) {
				return true
			}
			found = true
			for _, stmt := range sw.Body.List {
				for _, expr := range stmt.(*ast.CaseClause).List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Fatalf("rules.New: case %T is not a string literal; update this test", expr)
					}
					v, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("unquote %s: %v", lit.Value, err)
					}
					cases = append(cases, v)
				}
			}
			return false
		})
	}
	if !found {
		t.Fatal("rules.go: no `switch def.Type` in func New; update this test to read the type dispatch")
	}
	return cases
}

func isDefType(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Type" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "def"
}
