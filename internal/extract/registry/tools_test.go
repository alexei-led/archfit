package registry

import (
	"slices"
	"testing"
)

func names(tools []Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

// TestToolsPerLanguage pins the two groups of design 11.8: the check reads
// Go with its own parser, TypeScript and Python with ast-grep, Rust and C#
// with today's tools; ast-grep reads the literals of every language but Go.
// The rest serves the report only.
func TestToolsPerLanguage(t *testing.T) {
	t.Parallel()
	want := map[string]struct{ check, report []string }{
		"go":         {check: []string{}, report: []string{"go"}},
		"typescript": {check: []string{ToolAstGrep}, report: nil},
		"python":     {check: []string{ToolAstGrep}, report: nil},
		"rust":       {check: []string{ToolCargo, ToolAstGrep}, report: []string{ToolCargoModules}},
		"dotnet":     {check: []string{cmdDotNet, ToolAstGrep}, report: []string{"archfit-roslyn"}},
	}
	for _, lang := range All() {
		w, ok := want[lang.ID]
		if !ok {
			t.Errorf("language %s has no expected tool groups", lang.ID)
			continue
		}
		if got := names(lang.CheckTools); !slices.Equal(got, w.check) {
			t.Errorf("%s check tools = %v, want %v", lang.ID, got, w.check)
		}
		if got := names(lang.ReportTools); !slices.Equal(got, w.report) {
			t.Errorf("%s report tools = %v, want %v", lang.ID, got, w.report)
		}
	}
}

// TestToolGroups pins what doctor shows: git first, each tool once with every
// language that uses it.
func TestToolGroups(t *testing.T) {
	t.Parallel()
	check, report := ToolGroups()
	checkNames := make([]string, 0, len(check))
	for _, u := range check {
		checkNames = append(checkNames, u.Name)
	}
	if want := []string{"git", ToolAstGrep, ToolCargo, cmdDotNet}; !slices.Equal(checkNames, want) {
		t.Errorf("check group = %v, want %v", checkNames, want)
	}
	if len(check[0].Languages) != 0 {
		t.Errorf("git languages = %v, want none (every language)", check[0].Languages)
	}
	if got, want := check[1].Languages, []string{"typescript", "python", "rust", "dotnet"}; !slices.Equal(got, want) {
		t.Errorf("ast-grep languages = %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, u := range slices.Concat(check, report) {
		if seen[u.Name] {
			t.Errorf("tool %s is listed twice", u.Name)
		}
		seen[u.Name] = true
	}
}
