package application

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/assessment/score"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

func TestProjectReportCycleFindingMatchesPublishedSchema(t *testing.T) {
	rs, err := rules.New(policy.RuleConfig{Rules: []policy.RuleDef{{ID: "no-cycles", Type: "cycle"}}})
	if err != nil {
		t.Fatal(err)
	}
	rels := relationship.Set{Edges: []relationship.Edge{
		{FromID: "file:" + baselineTestFrom, ToID: "file:" + baselineTestTo, FromPath: baselineTestFrom, ToPath: baselineTestTo, Kind: "imports"},
		{FromID: "file:" + baselineTestTo, ToID: "file:" + baselineTestFrom, FromPath: baselineTestTo, ToPath: baselineTestFrom, Kind: "imports"},
	}}
	diagnostic := result.New()
	diagnostic.Findings = rs[0].Check(rels, rules.Evidence{})
	if len(diagnostic.Findings) != 1 || len(diagnostic.Findings[0].Locations) != 0 {
		t.Fatalf("cycle findings = %+v, want one finding without file locations", diagnostic.Findings)
	}
	compiled, err := jsonschema.NewCompiler().Compile("../../archfit.state.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	accepted := baseline.Baseline{Accepted: []baseline.AcceptedFinding{{
		Fingerprint: diagnostic.Findings[0].ID, RuleID: diagnostic.Findings[0].RuleID, Kind: finding.KindGate,
	}}}
	removed := status.Assign(rs[0].Check(relationship.Set{}, rules.Evidence{}), accepted, policy.WaiverSet{}, time.Time{}, finding.KindGate)
	if len(removed) != 1 || removed[0].Status != finding.StatusFixed {
		t.Fatalf("removed cycle findings = %+v, want one fixed finding", removed)
	}
	for _, tc := range []struct {
		name     string
		findings []finding.Finding
	}{
		{name: "live cycle", findings: diagnostic.Findings},
		{name: "removed cycle", findings: removed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic := result.New()
			diagnostic.Findings = append(diagnostic.Findings, tc.findings...)
			diagnostic.Findings = append(diagnostic.Findings, finding.Finding{
				ID: "located", RuleID: "edge", Kind: finding.KindGate, Status: finding.StatusNew,
				Locations: []relationship.Location{{File: baselineTestFrom, Line: 7}},
			})
			document := ProjectReport(diagnostic, score.Scorecard{})
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
			if got := document.State.Findings[1].Locations; len(got) != 1 || got[0].File != baselineTestFrom || got[0].Line != 7 {
				t.Errorf("projected locations = %+v, want original file and line", got)
			}
		})
	}
}
