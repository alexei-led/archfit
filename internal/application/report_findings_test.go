package application

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/assessment/rules"
	"github.com/alexei-led/archfit/v3/internal/assessment/score"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

const cycleTestFrom = "a.go"
const cycleTestTo = "b.go"

func TestProjectReportCycleFindingMatchesPublishedSchema(t *testing.T) {
	rs, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{{ID: "no-cycles", Type: "cycle"}}})
	if err != nil {
		t.Fatal(err)
	}
	rels := relationship.Set{Edges: []relationship.Edge{
		{FromID: "file:" + cycleTestFrom, ToID: "file:" + cycleTestTo, FromPath: cycleTestFrom, ToPath: cycleTestTo, Kind: "imports"},
		{FromID: "file:" + cycleTestTo, ToID: "file:" + cycleTestFrom, FromPath: cycleTestTo, ToPath: cycleTestFrom, Kind: "imports"},
	}}
	diagnostic := result.New()
	diagnostic.Findings = rs[0].Check(rels, rules.Evidence{})
	if len(diagnostic.Findings) != 1 || len(diagnostic.Findings[0].Locations) != 0 {
		t.Fatalf("cycle findings = %+v, want one finding without file locations", diagnostic.Findings)
	}
	compiled, err := jsonschema.NewCompiler().Compile("../../archfit.report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		findings []finding.Finding
	}{
		{name: "live cycle", findings: diagnostic.Findings},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic := result.New()
			diagnostic.Findings = append(diagnostic.Findings, tc.findings...)
			diagnostic.Findings = append(diagnostic.Findings, finding.Finding{
				ID: "located", RuleID: "edge", Kind: finding.KindGate, Status: finding.StatusNew,
				Locations: []relationship.Location{{File: cycleTestFrom, Line: 7}},
			})
			document := projectReport(diagnostic, score.Scorecard{})
			raw, err := json.Marshal(document.State)
			if err != nil {
				t.Fatal(err)
			}
			var instance any
			if err := json.Unmarshal(raw, &instance); err != nil {
				t.Fatal(err)
			}
			if err := compiled.Validate(instance); err != nil {
				t.Errorf("projected findings violate the published schema: %v", err)
			}
			if got := document.State.Findings[1].Locations; len(got) != 1 || got[0].File != cycleTestFrom || got[0].Line != 7 {
				t.Errorf("projected locations = %+v, want original file and line", got)
			}
		})
	}
}
