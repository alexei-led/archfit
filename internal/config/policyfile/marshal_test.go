package policyfile_test

import (
	"reflect"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
)

// TestMarshalRoundTrip writes a policy with every custom-decoded form, the
// switch values, one name and a list of names, a date, and pointers, and
// reads the same policy back.
func TestMarshalRoundTrip(t *testing.T) {
	const web, model = "web", "model"
	minBalance, maxBytes, maxFacts := 6, int64(1024), 10
	want := policyfile.File{
		Version: policyfile.Version,
		Owners:  []string{"alexei-led"},
		Gate:    policyfile.GateRequired,
		Layers:  []string{"inner", "outer"},
		Modules: map[string]policyfile.Module{
			web: {Paths: []string{"web/**"}, Public: []string{"web"}, Layer: "outer", DependsOn: []string{model},
				Volatility: policyfile.VolatilityLow, Subdomain: policyfile.SubdomainGeneric},
			model: {Paths: []string{"model/**"}, Layer: "inner"},
		},
		Builtin: policyfile.Builtin{Graph: policyfile.ModeDeny, Duplication: policyfile.ModeOff, MinBalance: &minBalance},
		Rules: []policyfile.Rule{
			{ID: "one-name", Deny: &policyfile.Deny{From: policyfile.Names{web}, To: policyfile.Names{"model/db/**"}}, Why: "w", Instead: "i"},
			{ID: "two-names", Deny: &policyfile.Deny{From: policyfile.Names{web, "web/x/**"}, To: policyfile.Names{model}}, Why: "w", Instead: "i"},
			{ID: "only", Only: &policyfile.Only{Use: "network", In: policyfile.Names{web}}, Why: "w"},
			{ID: "note", Note: "yes", Why: "a note that reads like a boolean"},
		},
		Exceptions: []policyfile.Exception{
			{Rule: "cycle", From: model, To: web, Expires: "2026-12-31", Why: "w"},
			{Rule: "only", From: model, Why: "w"},
		},
		Generated: []string{"gen/**"},
		Tests:     []string{"**/*_test.go"},
		Exclude:   []string{"vendor/**"},
		Languages: policyfile.Languages{
			Go:         policyfile.GoLanguage{Enabled: policyfile.SwitchOn, Modules: policyfile.GoModules{Include: []string{"a"}}},
			TypeScript: policyfile.Language{Enabled: policyfile.SwitchOff},
			Python:     policyfile.PythonLanguage{Enabled: policyfile.SwitchAuto, Package: "shop"},
			Rust:       policyfile.RustLanguage{Enabled: policyfile.SwitchOn, Features: []string{"x"}, IncludeDevDeps: true},
			DotNet:     policyfile.DotNetLanguage{Enabled: policyfile.SwitchOff, Projects: []string{"a.sln"}},
		},
		Analyzers: policyfile.Analyzers{
			Syntax:       policyfile.Analyzer{Enabled: policyfile.SwitchOn},
			CargoModules: policyfile.Analyzer{Enabled: policyfile.SwitchOff},
			Roslyn:       policyfile.TimedAnalyzer{Enabled: policyfile.SwitchAuto, Timeout: "5m"},
		},
		Coverage: policyfile.Coverage{Enabled: true, Sources: []policyfile.CoverageSource{
			{Path: "c.out", Format: "go-coverprofile", MaxBytes: &maxBytes, MaxFacts: &maxFacts},
		}},
	}
	data, err := yaml.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := policyfile.Decode(data)
	if err != nil {
		t.Fatalf("Decode:\n%v\n%s", err, data)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the policy:\ngot  %+v\nwant %+v\n%s", got, want, data)
	}
}
