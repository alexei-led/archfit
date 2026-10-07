package evaluation_test

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	ruleUncovered  = "map/uncovered_path"
	uncoveredDir   = "tools/gen"
	uncoveredFile  = "tools/gen/main.go"
	ownedFile      = "billing/charge.go"
	ownedGlob      = "billing/**"
	moduleBilling  = "billing"
	matchedSubject = "subject"
	rustURLFile    = "crates/shared/src/url.rs"
	pyRunFile      = "src/acme/ops/run.py"
	rootMainGo     = "main.go"
)

func uncoveredPolicy(modules map[string]policy.ModuleDef) policy.AssessmentPolicy {
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}
	return policy.AssessmentPolicy{Topology: topology, Staleness: policy.StalenessPolicy{Enabled: true}}
}

func billingModules() map[string]policy.ModuleDef {
	return map[string]policy.ModuleDef{moduleBilling: {Paths: []string{ownedGlob}}}
}

func evaluateUncovered(ev evaluation.RuleEvidence, p policy.AssessmentPolicy, gate policy.GateMode, accepted acceptedSet) evaluation.Result {
	return evaluation.Evaluate(evaluation.Input{
		Evidence: ev, Policy: p, ModuleReview: gate, Accepted: accepted,
		IncludeAdvisories: true, Now: evaluatedAt,
	})
}

func uncoveredFindings(res evaluation.Result) []finding.Finding {
	var out []finding.Finding
	for _, f := range res.Findings {
		if f.RuleID == ruleUncovered {
			out = append(out, f)
		}
	}
	return out
}

func subjects(fs []finding.Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.MatchedBy[matchedSubject])
	}
	return out
}

func TestUncoveredSourceReportsProductionSourceOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		ev    evaluation.RuleEvidence
		mods  map[string]policy.ModuleDef
		wants []string
	}{
		{
			name:  "production file outside every module",
			ev:    evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{uncoveredFile: fileclass.Production, ownedFile: fileclass.Production}},
			mods:  billingModules(),
			wants: []string{uncoveredDir},
		},
		{
			name: "test generated and vendor files never count",
			ev: evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{
				"tools/gen/main_test.go": fileclass.Test, "tools/gen/z.pb.go": fileclass.Generated, "third/x.go": fileclass.Vendor,
			}},
			mods: billingModules(),
		},
		{
			name: "unanalysed source is not part of the map",
			ev: evaluation.RuleEvidence{
				FileClasses:     map[string]fileclass.FileClass{"web/ui/app.ts": fileclass.Production},
				UnanalysedFiles: map[string]struct{}{"web/ui/app.ts": {}},
			},
			mods: billingModules(),
		},
		{
			name: "unsupported extension is not source",
			ev:   evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{"docs/notes.txt": fileclass.Production}},
			mods: billingModules(),
		},
		{
			name: "go package dir glob owns the package files",
			ev:   evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{"internal/pay/pay.go": fileclass.Production}},
			mods: map[string]policy.ModuleDef{"pay": {Paths: []string{"internal/pay"}}},
		},
		{
			name: "python dotted glob owns the module file",
			ev: evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{
				"src/acme/billing/charge.py": fileclass.Production, pyRunFile: fileclass.Production,
			}},
			mods:  map[string]policy.ModuleDef{moduleBilling: {Paths: []string{"acme.billing.**"}}},
			wants: []string{"src/acme/ops"},
		},
		{
			name: "rust crate owner places a crate declared by its identifier",
			ev: evaluation.RuleEvidence{
				FileClasses:     map[string]fileclass.FileClass{rustURLFile: fileclass.Production},
				SourceSelectors: map[string]string{rustURLFile: "yazi-shared"},
				CrateOwners:     map[string]string{"yazi-shared": "shared"},
			},
			mods: map[string]policy.ModuleDef{"shared": {Paths: []string{"yazi_shared"}}},
		},
		{
			name: "rust file without crate metadata abstains",
			ev: evaluation.RuleEvidence{
				FileClasses:     map[string]fileclass.FileClass{rustURLFile: fileclass.Production},
				SourceSelectors: map[string]string{rustURLFile: ""},
			},
			mods: billingModules(),
		},
		{
			name:  "no declared module leaves every production dir unowned",
			ev:    evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{uncoveredFile: fileclass.Production, rootMainGo: fileclass.Production}},
			wants: []string{".", uncoveredDir},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := subjects(uncoveredFindings(evaluateUncovered(tc.ev, uncoveredPolicy(tc.mods), "", acceptedSet{})))
			if fmt.Sprint(got) != fmt.Sprint(tc.wants) {
				t.Fatalf("uncovered dirs = %v, want %v", got, tc.wants)
			}
		})
	}
}

func TestUncoveredSourceDisabledWithoutModuleReview(t *testing.T) {
	t.Parallel()
	p := uncoveredPolicy(billingModules())
	p.Staleness.Enabled = false
	ev := evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{uncoveredFile: fileclass.Production}}
	if got := uncoveredFindings(evaluateUncovered(ev, p, policy.GateFail, acceptedSet{})); len(got) != 0 {
		t.Fatalf("findings = %v, want none while module_review is off", got)
	}
}

func TestUncoveredSourceGateMode(t *testing.T) {
	t.Parallel()
	ev := evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{uncoveredFile: fileclass.Production}}
	cases := []struct {
		gate      policy.GateMode
		wantKind  string
		wantGates int
	}{
		{gate: "", wantKind: finding.KindAdvisory},
		{gate: policy.GateWarn, wantKind: finding.KindAdvisory},
		{gate: policy.GateFail, wantKind: finding.KindGate, wantGates: 1},
	}
	for _, tc := range cases {
		t.Run(string(tc.gate), func(t *testing.T) {
			t.Parallel()
			res := evaluateUncovered(ev, uncoveredPolicy(billingModules()), tc.gate, acceptedSet{})
			got := uncoveredFindings(res)
			if len(got) != 1 || got[0].Kind != tc.wantKind {
				t.Fatalf("findings = %+v, want one %s finding", got, tc.wantKind)
			}
			if res.GateFindings != tc.wantGates {
				t.Errorf("GateFindings = %d, want %d", res.GateFindings, tc.wantGates)
			}
		})
	}
}

func TestUncoveredSourceAcceptedFindingDoesNotBlock(t *testing.T) {
	t.Parallel()
	ev := evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{uncoveredFile: fileclass.Production}}
	p := uncoveredPolicy(billingModules())
	first := uncoveredFindings(evaluateUncovered(ev, p, policy.GateFail, acceptedSet{}))
	if len(first) != 1 {
		t.Fatalf("findings = %+v, want one", first)
	}
	accepted := acceptedSet{{Fingerprint: first[0].ID, RuleID: ruleUncovered}}
	res := evaluateUncovered(ev, p, policy.GateFail, accepted)
	got := uncoveredFindings(res)
	if len(got) != 1 || got[0].Status == finding.StatusNew {
		t.Fatalf("findings = %+v, want the accepted finding not new", got)
	}
	if res.GateFindings != 0 {
		t.Errorf("GateFindings = %d, want 0 for accepted debt", res.GateFindings)
	}
}

func TestUncoveredSourceIDIsStableAndBounded(t *testing.T) {
	t.Parallel()
	p := uncoveredPolicy(billingModules())
	small := evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{uncoveredFile: fileclass.Production}}
	grown := evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{}}
	for i := range 8 {
		grown.FileClasses[fmt.Sprintf("%s/f%d.go", uncoveredDir, i)] = fileclass.Production
	}
	grown.FileClasses[uncoveredFile] = fileclass.Production
	a := uncoveredFindings(evaluateUncovered(small, p, "", acceptedSet{}))
	b := uncoveredFindings(evaluateUncovered(grown, p, "", acceptedSet{}))
	if len(a) != 1 || len(b) != 1 || a[0].ID != b[0].ID || a[0].Why != b[0].Why {
		t.Fatalf("finding moved when files were added: %+v vs %+v", a, b)
	}
	if len(b[0].Locations) != 5 {
		t.Errorf("locations = %d, want the bound of 5", len(b[0].Locations))
	}
	if b[0].MatchedBy["uncovered_files"] != "9" {
		t.Errorf("uncovered_files = %q, want 9", b[0].MatchedBy["uncovered_files"])
	}
}

func TestUncoveredSourceCapsFindingsInPathOrder(t *testing.T) {
	t.Parallel()
	const dirs = 205
	ev := evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{}}
	for i := range dirs {
		ev.FileClasses[fmt.Sprintf("pkg%03d/x.go", i)] = fileclass.Production
	}
	got := uncoveredFindings(evaluateUncovered(ev, uncoveredPolicy(billingModules()), "", acceptedSet{}))
	if len(got) != 200 {
		t.Fatalf("findings = %d, want the cap of 200", len(got))
	}
	seen := make(map[string]bool, len(got))
	for _, f := range got {
		seen[f.MatchedBy[matchedSubject]] = true
		if f.MatchedBy["uncovered_dirs_total"] != strconv.Itoa(dirs) {
			t.Fatalf("uncovered_dirs_total = %q, want %d", f.MatchedBy["uncovered_dirs_total"], dirs)
		}
	}
	if !seen["pkg000"] || !seen["pkg199"] || seen["pkg200"] {
		t.Errorf("kept dirs are not the first 200 in path order")
	}
}

func TestModuleReviewFailBlocksOnlyUncoveredSource(t *testing.T) {
	t.Parallel()
	p := uncoveredPolicy(map[string]policy.ModuleDef{
		moduleBilling: {Paths: []string{ownedGlob}},
		"ghost":       {Paths: []string{"ghost/**"}},
	})
	ev := evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{uncoveredFile: fileclass.Production}}
	res := evaluateUncovered(ev, p, policy.GateFail, acceptedSet{})
	for _, f := range res.Findings {
		if f.RuleID != ruleUncovered && f.Kind != finding.KindAdvisory {
			t.Errorf("%s kind = %q, want advisory: only uncovered source blocks", f.RuleID, f.Kind)
		}
	}
	if res.GateFindings != 1 {
		t.Errorf("GateFindings = %d, want 1", res.GateFindings)
	}
}

func TestUncoveredSourceCapAppliesOnlyToNewDirectories(t *testing.T) {
	t.Parallel()
	p := uncoveredPolicy(billingModules())
	ev := evaluation.RuleEvidence{FileClasses: map[string]fileclass.FileClass{}}
	for i := range 200 {
		ev.FileClasses[fmt.Sprintf("pkg%03d/x.go", i)] = fileclass.Production
	}
	first := uncoveredFindings(evaluateUncovered(ev, p, policy.GateFail, acceptedSet{}))
	accepted := make(acceptedSet, 0, len(first))
	for _, f := range first {
		accepted = append(accepted, status.AcceptedEntry{Fingerprint: f.ID, RuleID: ruleUncovered})
	}
	ev.FileClasses["pkg200/x.go"] = fileclass.Production
	res := evaluateUncovered(ev, p, policy.GateFail, accepted)
	got := uncoveredFindings(res)
	for _, f := range got {
		if f.Status == finding.StatusFixed {
			t.Fatalf("accepted directory %s reads fixed although it is still unowned", f.MatchedBy[matchedSubject])
		}
	}
	if len(got) != 201 {
		t.Fatalf("findings = %d, want 200 accepted plus the new one", len(got))
	}
	if !slices.Contains(subjects(got), "pkg200") || res.GateFindings != 1 {
		t.Fatalf("new dir kept = %t, GateFindings = %d; want the new directory reported as the one blocker",
			slices.Contains(subjects(got), "pkg200"), res.GateFindings)
	}
}

// TestUncoveredSourceSuggestsAGlobThatOwnsTheDirectory pins the repair hint: the
// suggested paths: glob, added to a module, leaves the directory owned. A bare
// directory owns only a Go package, so TypeScript, Python, and root files each
// need their own spelling.
func TestUncoveredSourceSuggestsAGlobThatOwnsTheDirectory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		files     map[string]fileclass.FileClass
		selectors map[string]string
		want      string
	}{
		{name: "go package", files: map[string]fileclass.FileClass{uncoveredFile: fileclass.Production}, want: "tools/gen/**"},
		{name: "go root files", files: map[string]fileclass.FileClass{rootMainGo: fileclass.Production}, want: "*.go"},
		{name: "typescript directory", files: map[string]fileclass.FileClass{"web/src/a.ts": fileclass.Production, "web/src/b.ts": fileclass.Production}, want: "web/src/**"},
		{name: "python package modules", files: map[string]fileclass.FileClass{pyRunFile: fileclass.Production}, want: "acme.ops.**"},
		{name: "python package with init", files: map[string]fileclass.FileClass{
			"src/acme/ops/__init__.py": fileclass.Production, pyRunFile: fileclass.Production,
		}, want: "{acme.ops,acme.ops.**}"},
		{name: "python top-level module", files: map[string]fileclass.FileClass{"run.py": fileclass.Production}},
		{name: "mixed root languages", files: map[string]fileclass.FileClass{rootMainGo: fileclass.Production, "app.ts": fileclass.Production}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := evaluation.RuleEvidence{FileClasses: tc.files, SourceSelectors: tc.selectors}
			got := uncoveredFindings(evaluateUncovered(ev, uncoveredPolicy(billingModules()), "", acceptedSet{}))
			if len(got) != 1 {
				t.Fatalf("findings = %d, want 1", len(got))
			}
			glob := got[0].MatchedBy["suggested_path"]
			if glob != tc.want {
				t.Fatalf("suggested_path = %q, want %q", glob, tc.want)
			}
			if glob == "" {
				return
			}
			mods := billingModules()
			mods["owner"] = policy.ModuleDef{Paths: []string{glob}}
			if after := uncoveredFindings(evaluateUncovered(ev, uncoveredPolicy(mods), "", acceptedSet{})); len(after) != 0 {
				t.Errorf("after adding %q the directory is still unowned: %v", glob, subjects(after))
			}
		})
	}
}
