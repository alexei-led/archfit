// Contract test for the values the application hands each stage: the run
// context resolved once during acquisition must reach assessment unchanged.
package application

import (
	"context"
	"io"
	"testing"

	"github.com/alexei-led/archfit/internal/evidence"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	acquiredConfigHash  = "hash-from-acquire"
	acquiredOwnerSource = "codeowners"
)

// contextCarryingStage acquires a context whose identity fields are all
// distinguishable, so anything the downstream stages re-derive instead of
// reading shows up as a mismatch in the diagnostic.
type contextCarryingStage struct{ prepared bool }

func (f *contextCarryingStage) Prepare(context.Context) error {
	f.prepared = true
	return nil
}

func (f *contextCarryingStage) Acquire(context.Context, AnalysisRequest) (Acquired, error) {
	return Acquired{
		Facts: evidence.Facts{FileLOC: map[string]int{"a.go": 1}},
		Context: AnalysisContext{
			ConfigHash: acquiredConfigHash, OwnerSource: acquiredOwnerSource,
			Policy:         policy.New(policy.TopologyView{}, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil),
			ConfigWarnings: []string{"acquisition disclosed this"},
			CoverageGaps:   []modevidence.CoverageGap{{Tool: "grimp", Gate: string(OutcomeWarn)}},
		},
	}, nil
}

func TestStageExecutorPassesExplicitStageValues(t *testing.T) {
	stage := &contextCarryingStage{}
	out, err := (StageExecutor{Preparer: stage, Evidence: stage, Stderr: io.Discard}).
		Execute(t.Context(), AnalysisRequest{BaseRef: ""})
	if err != nil {
		t.Fatal(err)
	}
	if !stage.prepared {
		t.Fatal("policy preparation did not run before analysis")
	}
	// Assessment receives the acquisition-time context verbatim: the ownership
	// and config identity resolved once during Acquire must not be re-derived.
	if out.Diagnostic.ConfigHash != acquiredConfigHash || out.Diagnostic.OwnerSource != acquiredOwnerSource {
		t.Fatalf("assessment did not receive the acquisition context: hash=%q owner=%q", out.Diagnostic.ConfigHash, out.Diagnostic.OwnerSource)
	}
	// The coverage evidence and config warnings acquisition resolved are
	// attached, not rebuilt: assessment cannot probe the source tree.
	if len(out.Diagnostic.CoverageGaps) != 1 || out.Diagnostic.CoverageGaps[0].Tool != "grimp" {
		t.Fatalf("coverage gaps = %+v, want the acquisition-resolved block", out.Diagnostic.CoverageGaps)
	}
	if len(out.Diagnostic.ConfigWarnings) != 1 {
		t.Fatalf("config warnings = %+v, want the acquisition-resolved block", out.Diagnostic.ConfigWarnings)
	}
}
