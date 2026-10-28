package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/status"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/policy"
)

const (
	ruleUncoveredChange = "map/uncovered_change"
	genSkipFile         = "gen/skip.go"
	webAppFile          = "web/app.ts"
)

func uncoveredChangeFindings(res evaluation.Result) map[string]finding.Finding {
	out := map[string]finding.Finding{}
	for _, f := range res.Findings {
		if f.RuleID == ruleUncoveredChange {
			out[f.MatchedBy["subject"]] = f
		}
	}
	return out
}

func changeEvidence(added ...string) evaluation.RuleEvidence {
	ev := evaluation.RuleEvidence{
		FileClasses: map[string]fileclass.FileClass{ownedFile: fileclass.Production},
		AddedFiles:  map[string]struct{}{},
	}
	for _, f := range added {
		ev.AddedFiles[f] = struct{}{}
	}
	return ev
}

// TestUncoveredChangeNamesEachWayACodeIsUnread: a file the change adds is
// uncovered when no module owns it, an exclude: glob matches it, its language is
// switched off, or no producer reads it. Each says which.
func TestUncoveredChangeNamesEachWayACodeIsUnread(t *testing.T) {
	t.Parallel()
	ev := changeEvidence(ownedFile, uncoveredFile, genSkipFile, webAppFile, "docs/readme.md", "tools/gen/main_test.go")
	ev.FileClasses[uncoveredFile] = fileclass.Production
	ev.FileClasses["tools/gen/main_test.go"] = fileclass.Test
	ev.FileClasses["docs/readme.md"] = fileclass.Production
	// Out of scope: declared by the acquisition, with the cause it recorded.
	ev.OutOfScopeFiles = map[string]struct{}{genSkipFile: {}, webAppFile: {}}
	ev.UncoveredCauses = map[string]string{genSkipFile: "exclude: gen/**", webAppFile: "languages.typescript.enabled: false"}

	got := uncoveredChangeFindings(evaluateUncovered(ev, uncoveredPolicy(billingModules()), "", nil))
	want := map[string]struct{ reason, cause string }{
		uncoveredFile: {"unowned", ""},
		genSkipFile:   {"excluded", "exclude: gen/**"},
		webAppFile:    {"language_disabled", "languages.typescript.enabled: false"},
	}
	if len(got) != len(want) {
		t.Fatalf("findings for %d files %v, want %d: %v", len(got), keys(got), len(want), keys(want))
	}
	for file, w := range want {
		f := got[file]
		if f.MatchedBy["reason"] != w.reason || f.MatchedBy["cause"] != w.cause {
			t.Errorf("%s: reason %q cause %q, want %q %q", file, f.MatchedBy["reason"], f.MatchedBy["cause"], w.reason, w.cause)
		}
		if f.Kind != finding.KindAdvisory {
			t.Errorf("%s: kind %q, want an advisory: it is unknown, not a repair", file, f.Kind)
		}
	}
	if glob := got[uncoveredFile].MatchedBy["suggested_path"]; glob == "" {
		t.Error("an unowned file carries the glob that would own it")
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestUncoveredChangeIgnoresFilesTheChangeDidNotAdd: files that were always
// unowned are map/uncovered_path's. Without a base nothing is added.
func TestUncoveredChangeIgnoresFilesTheChangeDidNotAdd(t *testing.T) {
	t.Parallel()
	ev := changeEvidence() // no added files
	ev.FileClasses[uncoveredFile] = fileclass.Production
	if got := uncoveredChangeFindings(evaluateUncovered(ev, uncoveredPolicy(billingModules()), "", nil)); len(got) != 0 {
		t.Errorf("findings %v without added files, want none", keys(got))
	}
}

// TestUncoveredChangeCanBeSwitchedOff: change_gate: off drops the advisories.
func TestUncoveredChangeCanBeSwitchedOff(t *testing.T) {
	t.Parallel()
	ev := changeEvidence(uncoveredFile)
	ev.FileClasses[uncoveredFile] = fileclass.Production
	res := evaluation.Evaluate(evaluation.Input{
		Evidence: ev, Policy: uncoveredPolicy(billingModules()), UncoveredChange: policy.GateOff, Accepted: status.Empty{},
		IncludeAdvisories: true, Now: evaluatedAt,
	})
	if got := uncoveredChangeFindings(res); len(got) != 0 {
		t.Errorf("findings %v under change_gate: off, want none", keys(got))
	}
}
