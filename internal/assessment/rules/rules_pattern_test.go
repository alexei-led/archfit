package rules_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/pattern"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

const (
	typeForbiddenPattern = "forbidden_pattern"
	ruleIDDomainNoClock  = "domain_no_clock"
	patternClock         = "clock"
	kindPatternMatch     = "pattern_match"
	fileDomainSvc        = "internal/domain/service.go"
	fileDomainSvcTest    = "internal/domain/service_test.go"
	fileAppHandler       = "internal/app/handler.go"
	globDomainAll        = "internal/domain/**"
	textTimeNow          = "time.Now()"
	modDomain            = "domain"
	patternEnv           = "env"
	textOsEnviron        = "os.Environ()"
)

var clockPattern = pattern.Def{ID: patternClock, Lang: "go", Rule: textTimeNow}

func newForbiddenPatternRule(t *testing.T, def policy.RuleDef, others ...policy.RuleDef) rules.Rule {
	t.Helper()
	modules := map[string]policy.ModuleDef{
		modDomain: {Paths: []string{globDomainAll}},
		"app":     {Paths: []string{"internal/app/**"}},
		"pydom":   {Paths: []string{pkgPyDomain, globPyDomainAll}},
	}
	ruleSet, err := rules.New(policy.RuleConfig{
		Rules:     append([]policy.RuleDef{def}, others...),
		ModuleMap: policy.BuildModuleMap(modules),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ruleSet[0]
}

func match(file string, line int, text string) pattern.Match {
	return pattern.Match{File: file, Pattern: patternClock, Text: text, Line: line, Node: "call_expression"}
}

// inventory classifies the fixture files the way the LOC walk would.
func inventory() map[string]fileclass.FileClass {
	return map[string]fileclass.FileClass{
		fileDomainSvc:                      fileclass.Production,
		"internal/domain/clock.go":         fileclass.Production,
		fileDomainSvcTest:                  fileclass.Test,
		"internal/domain/clock_mock.go":    fileclass.Generated,
		fileAppHandler:                     fileclass.Production,
		"myapp/domain/service.py":          fileclass.Production,
		"internal/domain/internal_view.go": fileclass.Production,
	}
}

func TestForbiddenPattern_FiresOnProductionMatchesInScope(t *testing.T) {
	def := policy.RuleDef{ID: ruleIDDomainNoClock, Type: typeForbiddenPattern, From: globDomainAll, Patterns: []pattern.Def{clockPattern}}
	r := newForbiddenPatternRule(t, def)
	findings := r.Check(relationship.Set{}, rules.Evidence{
		PatternMatches: []pattern.Match{match(fileDomainSvc, 12, textTimeNow)},
		FileClasses:    inventory(),
	})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(findings), findings)
	}
	f := findings[0]
	if want := finding.NewKeyed(ruleIDDomainNoClock, kindPatternMatch, patternClock, fileDomainSvc, textTimeNow).ID; f.ID != want {
		t.Errorf("ID = %q, want keyed on (rule, kind, pattern, file, text) %q", f.ID, want)
	}
	if f.Edge.From != (finding.Endpoint{Module: modDomain, Path: fileDomainSvc}) || f.Edge.To != (finding.Endpoint{}) || f.Edge.Kind != kindPatternMatch {
		t.Errorf("edge = %+v, want the file in module domain and no target", f.Edge)
	}
	if len(f.Locations) != 1 || f.Locations[0] != (relationship.Location{File: fileDomainSvc, Line: 12}) {
		t.Errorf("locations = %+v, want %s:12", f.Locations, fileDomainSvc)
	}
	if f.Kind != kindGate || f.Severity != finding.SeverityHigh {
		t.Errorf("kind/severity = %s/%s, want gate/high", f.Kind, f.Severity)
	}
	if f.MatchedBy["pattern"] != patternClock || f.MatchedBy["file"] != fileDomainSvc || f.MatchedBy["locations_total"] != "1" {
		t.Errorf("matched_by = %v", f.MatchedBy)
	}
	if !strings.Contains(f.Why, fileDomainSvc+":12") || !strings.Contains(f.Why, `"clock"`) {
		t.Errorf("why = %q, want file:line and the pattern id", f.Why)
	}
}

func TestForbiddenPattern_GroupsByPatternFileAndNormalizedText(t *testing.T) {
	def := policy.RuleDef{ID: ruleIDDomainNoClock, Type: typeForbiddenPattern, From: globDomainAll, Patterns: []pattern.Def{clockPattern}}
	r := newForbiddenPatternRule(t, def)
	findings := r.Check(relationship.Set{}, rules.Evidence{
		PatternMatches: []pattern.Match{
			match(fileDomainSvc, 40, "time.Now()"),
			match(fileDomainSvc, 12, "time.Now( )"), // reformatted: same normalized text
			match(fileDomainSvc, 20, "time.Now().UTC()"),
		},
		FileClasses: inventory(),
	})
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2 (one per distinct normalized text): %+v", len(findings), findings)
	}
	var grouped finding.Finding
	for _, f := range findings {
		if len(f.Locations) == 2 {
			grouped = f
		}
	}
	if grouped.ID == "" {
		t.Fatalf("no finding grouped the two time.Now() sites: %+v", findings)
	}
	if grouped.Locations[0].Line != 12 || grouped.Locations[1].Line != 40 || grouped.MatchedBy["locations_total"] != "2" {
		t.Errorf("grouped locations = %+v total %s, want lines 12 and 40", grouped.Locations, grouped.MatchedBy["locations_total"])
	}
	if !strings.Contains(grouped.Why, ":12") || !strings.Contains(grouped.Why, "+1 more") {
		t.Errorf("why = %q, want the first site and the extra count", grouped.Why)
	}

	moved := r.Check(relationship.Set{}, rules.Evidence{
		PatternMatches: []pattern.Match{match(fileDomainSvc, 99, "time.Now()")},
		FileClasses:    inventory(),
	})
	if len(moved) != 1 || moved[0].ID != grouped.ID {
		t.Errorf("a moved match must keep its ID: got %+v, want %s", moved, grouped.ID)
	}
}

func TestForbiddenPattern_Skips(t *testing.T) {
	tests := []struct {
		name  string
		def   policy.RuleDef
		match pattern.Match
	}{
		{"test file", policy.RuleDef{From: globDomainAll}, match(fileDomainSvcTest, 5, textTimeNow)},
		{"generated file", policy.RuleDef{From: globDomainAll}, match("internal/domain/clock_mock.go", 5, textTimeNow)},
		{"file outside the source inventory", policy.RuleDef{From: globDomainAll}, match("internal/domain/testdata/x.go", 5, textTimeNow)},
		{"file outside from", policy.RuleDef{From: globDomainAll}, match(fileAppHandler, 5, textTimeNow)},
		{"another rule's pattern", policy.RuleDef{From: globDomainAll},
			pattern.Match{File: fileDomainSvc, Pattern: patternEnv, Text: textOsEnviron, Line: 5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def := tc.def
			def.ID, def.Type, def.Patterns = ruleIDDomainNoClock, typeForbiddenPattern, []pattern.Def{clockPattern}
			other := policy.RuleDef{ID: "other", Type: typeForbiddenPattern, Patterns: []pattern.Def{{ID: patternEnv, Lang: "go", Rule: textOsEnviron}}}
			r := newForbiddenPatternRule(t, def, other)
			findings := r.Check(relationship.Set{}, rules.Evidence{PatternMatches: []pattern.Match{tc.match}, FileClasses: inventory()})
			if len(findings) != 0 {
				t.Fatalf("got %d findings, want 0: %+v", len(findings), findings)
			}
		})
	}
}

func TestForbiddenPattern_ScopeMatchesPathOrConventionSelector(t *testing.T) {
	tests := []struct {
		name string
		from string
		file string
	}{
		{"empty from covers every production file", "", fileAppHandler},
		{"dotted python from matches the file's module selector", "myapp.domain.**", "myapp/domain/service.py"},
		{"slash from matches the path", globDomainAll, fileDomainSvc},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def := policy.RuleDef{ID: ruleIDDomainNoClock, Type: typeForbiddenPattern, From: tc.from, Patterns: []pattern.Def{clockPattern}}
			findings := newForbiddenPatternRule(t, def).Check(relationship.Set{}, rules.Evidence{
				PatternMatches: []pattern.Match{match(tc.file, 3, textTimeNow)},
				FileClasses:    inventory(),
			})
			if len(findings) != 1 {
				t.Fatalf("got %d findings, want 1", len(findings))
			}
		})
	}
}

// TestForbiddenPattern_NeverTransportsTheMatchedSource seeds a distinctive
// match text and asserts it reaches no published field: findings travel to the
// App, which carries no source snippets.
func TestForbiddenPattern_NeverTransportsTheMatchedSource(t *testing.T) {
	const seeded = `os.Setenv("API_TOKEN", "sk-seeded-0123456789")`
	def := policy.RuleDef{ID: ruleIDDomainNoClock, Type: typeForbiddenPattern, From: globDomainAll, Patterns: []pattern.Def{clockPattern}}
	findings := newForbiddenPatternRule(t, def).Check(relationship.Set{}, rules.Evidence{
		PatternMatches: []pattern.Match{match(fileDomainSvc, 7, seeded)},
		FileClasses:    inventory(),
	})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	raw, err := json.Marshal(findings[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"sk-seeded-0123456789", "API_TOKEN", "Setenv"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("finding JSON carries matched source %q: %s", leak, raw)
		}
	}
}

func TestForbiddenPattern_Validation(t *testing.T) {
	envPattern := pattern.Def{ID: patternEnv, Lang: "go", Rule: textOsEnviron}
	tests := []struct {
		name    string
		rules   []policy.RuleDef
		wantErr string
	}{
		{"no patterns", []policy.RuleDef{{ID: "p", Type: typeForbiddenPattern, From: globDomainAll}}, "at least one patterns entry"},
		{"to is not a pattern scope", []policy.RuleDef{{ID: "p", Type: typeForbiddenPattern, To: globDomainAll,
			Patterns: []pattern.Def{clockPattern}}}, "takes no to"},
		{"malformed from", []policy.RuleDef{{ID: "p", Type: typeForbiddenPattern, From: "internal/[",
			Patterns: []pattern.Def{clockPattern}}}, "malformed from glob"},
		{"pattern id shared with another rule type", []policy.RuleDef{
			{ID: "p", Type: typeForbiddenPattern, Patterns: []pattern.Def{clockPattern}},
			{ID: "dep", Type: typeForbiddenDependency, From: "a/**", To: "b/**", Patterns: []pattern.Def{clockPattern}},
		}, `pattern id "clock" is declared 2 times`},
		{"pattern id repeated inside the rule", []policy.RuleDef{
			{ID: "p", Type: typeForbiddenPattern, Patterns: []pattern.Def{clockPattern, clockPattern}},
		}, `pattern id "clock" is declared 2 times`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rules.New(policy.RuleConfig{Rules: tc.rules})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}

	// Existing configs keep loading: patterns repeated across rules that are
	// not forbidden_pattern stay a warning-level concern, never a load error.
	if _, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{
		{ID: "dep1", Type: typeForbiddenDependency, From: "a/**", To: "b/**", Patterns: []pattern.Def{envPattern}},
		{ID: "dep2", Type: typeForbiddenDependency, From: "c/**", To: "d/**", Patterns: []pattern.Def{envPattern}},
	}}); err != nil {
		t.Errorf("duplicate pattern ids outside forbidden_pattern must still load: %v", err)
	}
}

func TestForbiddenPattern_GateWarnIsAdvisory(t *testing.T) {
	def := policy.RuleDef{ID: ruleIDDomainNoClock, Type: typeForbiddenPattern, Gate: gateWarn, From: globDomainAll,
		Patterns: []pattern.Def{clockPattern}}
	findings := newForbiddenPatternRule(t, def).Check(relationship.Set{}, rules.Evidence{
		PatternMatches: []pattern.Match{match(fileDomainSvc, 3, textTimeNow)},
		FileClasses:    inventory(),
	})
	if len(findings) != 1 || findings[0].Kind != kindAdvisory {
		t.Fatalf("findings = %+v, want one advisory", findings)
	}
}
