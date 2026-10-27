package evaluation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	csDomainFile  = "src/Domain/Entities/TodoItem.cs"
	csDomainGlob  = "src/Domain/**"
	toolMSBuild   = "dotnet-msbuild"
	efCoreTarget  = "Microsoft.EntityFrameworkCore*"
	frameworksBan = "{Microsoft.EntityFrameworkCore*,Microsoft.AspNetCore*}"
)

// dotnetFixture is a .NET solution with a Domain and an Infrastructure
// project. The dotnet-msbuild primary producer has the given status.
func dotnetFixture(status string) (*result.Result, evaluation.StateInput) {
	diag, in := dimensionsFixture()
	diag.PrimaryExtractorTools = append(diag.PrimaryExtractorTools, toolDepCruiser, assessGrimp, toolCargo, toolMSBuild)
	diag.ToolCoverage = []modevidence.Coverage{{Tool: toolMSBuild, Status: status}}
	in.Facts = evaluation.Observations{FileLOC: map[string]int{
		csDomainFile: 10, "src/Infrastructure/Data/ApplicationDbContext.cs": 10,
	}}
	withModules(&in, map[string]policy.ModuleDef{
		"Domain":         {Paths: []string{csDomainGlob}},
		"Infrastructure": {Paths: []string{"src/Infrastructure/**"}},
	})
	return diag, in
}

// TestExternalNameTargetsAreEvaluated pins forbidden_dependency targets that
// name an external package by a dotted name: a .NET assembly (Roslyn emits
// external:<assembly> nodes), a brace glob of them, a third-party Python
// module, and a Go import path whose last element carries a dot. A dotted
// suffix is a name segment, not a file extension, so each rule is scoped over
// its source language: evaluated with a complete producer, unevaluated naming
// the producer when it is partial. A slash path ending in an unsupported file
// type still cannot be scoped, and its reason says so.
func TestExternalNameTargetsAreEvaluated(t *testing.T) {
	const unsupportedReason = "selector names a file type no dependency producer analyses: to src/Legacy/**/*.vb"
	for _, tc := range []struct {
		name   string
		python bool
		golang bool
		rule   policy.RuleDef
		// producer is the tool a partial run must name (Go: the fixture's Go
		// producer); "" means the rule scope is unknown whatever its state.
		producer string
	}{
		{name: "dotted assembly glob", rule: policy.RuleDef{From: csDomainGlob, To: efCoreTarget}, producer: toolMSBuild},
		{name: "dotted assembly name", rule: policy.RuleDef{From: csDomainGlob, To: "MediatR.Contracts"}, producer: toolMSBuild},
		{name: "brace glob of assembly names", rule: policy.RuleDef{From: csDomainGlob, To: frameworksBan}, producer: toolMSBuild},
		{name: "assembly name with a wildcard segment", rule: policy.RuleDef{From: csDomainGlob, To: "Microsoft.*.Abstractions"}, producer: toolMSBuild},
		{name: "undotted assembly glob", rule: policy.RuleDef{From: csDomainGlob, To: "MediatR*"}, producer: toolMSBuild},
		{name: "third-party python module", python: true, rule: policy.RuleDef{From: "app.core**", To: "requests.adapters"}, producer: assessGrimp},
		{name: "go import path with a dotted element", golang: true, rule: policy.RuleDef{From: selShipping, To: "gopkg.in/yaml.v3"}},
		{name: "unsupported file type target", rule: policy.RuleDef{From: csDomainGlob, To: "src/Legacy/**/*.vb"}},
	} {
		for _, status := range []string{modevidence.StatusOK, modevidence.StatusPartial} {
			t.Run(tc.name+"/"+status, func(t *testing.T) {
				diag, in := dotnetFixture(status)
				producer := tc.producer
				switch {
				case tc.python:
					diag.ToolCoverage = []modevidence.Coverage{{Tool: assessGrimp, Status: status}}
					in.Facts = evaluation.Observations{FileLOC: map[string]int{pyCoreFile: 10}}
					withModules(&in, map[string]policy.ModuleDef{assessCore: {Paths: []string{"app.core.**"}}})
				case tc.golang:
					producer = diag.PrimaryExtractorTools[0]
					diag.ToolCoverage = []modevidence.Coverage{{Tool: producer, Status: status}}
					in.Facts = evaluation.Observations{FileLOC: map[string]int{"internal/shipping/ship.go": 10}}
					withModules(&in, map[string]policy.ModuleDef{"shipping": {Paths: []string{selShipping}}})
				}
				tc.rule.Type = ruleForbidden
				withRule(&in, tc.rule)
				reason, unevaluated := unevaluatedReasons(diag, in)[ruleIDScoped]
				switch {
				case producer == "":
					if reason != unsupportedReason {
						t.Fatalf("reason = %q, want %q", reason, unsupportedReason)
					}
				case status == modevidence.StatusOK && unevaluated:
					t.Fatalf("rule unevaluated with a complete producer: %q", reason)
				case status != modevidence.StatusOK && !strings.Contains(reason, producer):
					t.Fatalf("reason = %q, want one naming the partial %s producer", reason, producer)
				}
			})
		}
	}
}

// TestBraceGlobOfExternalNamesIsNotADeadSelector pins config lint to the rule
// scope above: a brace glob whose every alternative names an external package
// is an external ban, and one with a misspelt first-party alternative is still
// judged first-party.
func TestBraceGlobOfExternalNamesIsNotADeadSelector(t *testing.T) {
	modules := map[string]policy.ModuleDef{"Domain": {Paths: []string{csDomainGlob}}}
	rules := []policy.RuleDef{
		{ID: "frameworks", Type: ruleForbidden, Gate: gateFail, From: csDomainGlob, To: frameworksBan},
		{ID: "efcore", Type: ruleForbidden, Gate: gateFail, From: csDomainGlob, To: efCoreTarget},
		{ID: "misspelt", Type: ruleForbidden, Gate: gateFail, From: csDomainGlob, To: "{src/Domian/**,src/Nope/**}"},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	snapshot := policy.New(topology, policy.RelationshipPolicy{}, policy.AssessmentPolicy{},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: rules}}, nil, nil)
	facts := evaluation.Observations{FileClassIndex: map[string]fileclass.FileClass{csDomainFile: fileclass.Production}}
	var dead []string
	for _, d := range evaluation.LintPolicy(snapshot, facts) {
		if d.Code == evaluation.LintDeadSelector {
			dead = append(dead, d.Path)
		}
	}
	if want := []string{"rules[misspelt].to"}; !slices.Equal(dead, want) {
		t.Fatalf("dead selectors = %v, want %v", dead, want)
	}
}
