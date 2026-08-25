package evaluation_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
)

// modulesWithPaths is the precondition for the "no source files matched" hint:
// the config declares at least one module glob.
func modulesWithPaths() map[string]policy.ModuleDef {
	return map[string]policy.ModuleDef{assessCore: {Paths: []string{"internal/**"}}}
}

// fileFacts satisfies the FileFacts-non-empty branch so a case that is not
// testing the module-glob hint does not trip it by accident.
func fileFacts() []evidence.FileFact {
	return []evidence.FileFact{{Module: assessCore, Files: []string{"internal/a.go"}}}
}

// TestHealthWarnings covers every branch. The coverage-gap case is the reason
// gaps is a parameter: Score stamps diag.CoverageGaps only AFTER Assess builds
// these warnings, so reading the field would leave that hint permanently dead.
func TestHealthWarnings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		diag    result.Result
		gaps    []evidence.CoverageGap
		modules map[string]policy.ModuleDef
		// wantCount is the number of warnings; want lists substrings that must
		// appear across them. Several branches fire together on the same input,
		// so the count is asserted separately from the text.
		scanRoot  string
		wantCount int
		want      []string
		deny      []string
	}{
		{
			name:    "clean run warns about nothing",
			diag:    result.Result{FileFacts: fileFacts(), ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 10, Scored: 10}},
			modules: modulesWithPaths(),
		},
		{
			name:      "coverage gap suggests doctor",
			diag:      result.Result{FileFacts: fileFacts(), ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 10, Scored: 10}},
			gaps:      []evidence.CoverageGap{{Tool: assessGrimp}},
			modules:   modulesWithPaths(),
			wantCount: 1,
			want:      []string{"analyzer coverage gap"},
		},
		{
			name:      "nothing scored suggests config update",
			diag:      result.Result{FileFacts: fileFacts(), ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 7, Scored: 0, External: 7}},
			modules:   modulesWithPaths(),
			wantCount: 1,
			want:      []string{"0 of 7 edges scored", "archfit config update"},
		},
		{
			name:      "all-abstained suggests enrich abstained",
			diag:      result.Result{FileFacts: fileFacts(), ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 4, Scored: 0, Abstained: 4}},
			modules:   modulesWithPaths(),
			wantCount: 2,
			want:      []string{"0 of 4 edges scored", "all 4 cross-module edges have unknown strength", "archfit config enrich abstained"},
		},
		{
			name: "python all-external suggests config update",
			diag: result.Result{
				FileFacts:       fileFacts(),
				ToolCoverage:    []evidence.Coverage{{Tool: assessGrimp, Status: evidence.StatusOK}},
				ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 5, Scored: 0, External: 5},
			},
			modules:   modulesWithPaths(),
			wantCount: 2,
			want:      []string{"0 of 5 edges scored", "no internal edges found"},
		},
		{
			name:      "no matched files echoes the caller's root",
			diag:      result.Result{ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 10, Scored: 10}},
			modules:   modulesWithPaths(),
			scanRoot:  assessRoot,
			wantCount: 1,
			want:      []string{"no source files matched declared module paths", "--root " + assessRoot},
		},
		{
			// Empty ScanRoot is "the whole repository". The hint must omit the
			// flag rather than suggest an unusable `--root ""`.
			name:      "no matched files and no --root omits the flag",
			diag:      result.Result{ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 10, Scored: 10}},
			modules:   modulesWithPaths(),
			wantCount: 1,
			want:      []string{"no source files matched declared module paths"},
			// The prose legitimately names --root; the RUN command must not.
			deny: []string{assessCfgPath + " --root"},
		},
		{
			name: "no declared module paths stays silent",
			diag: result.Result{ClassifiedEdges: &result.ClassifiedEdgeSummary{Total: 10, Scored: 10}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := evaluation.HealthWarnings(tt.diag, tt.gaps, tt.modules, tt.scanRoot, assessCfgPath)
			if len(got) != tt.wantCount {
				t.Fatalf("got %d warning(s) %q, want %d", len(got), got, tt.wantCount)
			}
			joined := strings.Join(got, "\n")
			for _, want := range tt.want {
				if !strings.Contains(joined, want) {
					t.Errorf("warning %q missing from:\n%s", want, joined)
				}
			}
			for _, deny := range tt.deny {
				if strings.Contains(joined, deny) {
					t.Errorf("warning must not mention %q:\n%s", deny, joined)
				}
			}
		})
	}
}
