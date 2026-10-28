package application

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/v3/internal/evidence"
	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/scope"
)

const (
	headScanRoot = "/repo"
	baseScanRoot = "/repo/.archfit-cache/worktrees/abc/wt"
	toolPrimary  = "go/packages"
)

type fixedWorktree struct{}

type noPreparation struct{}

func (noPreparation) Prepare(context.Context) error { return nil }

func (fixedWorktree) Checkout(context.Context, string, string, string) (string, func(), error) {
	return baseScanRoot, func() {}, nil
}

// coverageStage acquires one primary coverage row and records its request.
type coverageStage struct {
	root string
	row  modevidence.Coverage
	req  AnalysisRequest
}

func (s *coverageStage) Acquire(_ context.Context, req AnalysisRequest) (Acquired, error) {
	s.req = req
	return Acquired{
		Facts:        evidence.Facts{FileLOC: map[string]int{baselineTestFrom: 1}},
		Observations: evaluation.Observations{FileLOC: map[string]int{baselineTestFrom: 1}},
		Context: AnalysisContext{
			Scope:                 scope.Scope{Root: s.root},
			Policy:                policy.New(policy.TopologyView{}, policy.RelationshipPolicy{}, policy.AssessmentPolicy{}, policy.GatePolicy{}, nil, nil),
			PrimaryExtractorTools: []string{toolPrimary},
			MarkedCoverage:        []modevidence.Coverage{s.row},
		},
	}, nil
}

// TestBaseComparison_BaseRunReadsTheWorkingTreeAndLeaksNoBasePath: the base
// sub-run learns which working tree its checkout stands in for, and the base
// row's reason reaches the origin reason without the checkout's path.
func TestBaseComparison_BaseRunReadsTheWorkingTreeAndLeaksNoBasePath(t *testing.T) {
	head := &coverageStage{root: headScanRoot, row: modevidence.Coverage{Tool: toolPrimary, Status: modevidence.StatusOK}}
	base := &coverageStage{root: baseScanRoot, row: modevidence.Coverage{
		Tool: toolPrimary, Status: modevidence.StatusPartial, Reason: baseScanRoot + "/pkg/a.go: cannot load; fix " + baseScanRoot,
	}}
	stages := StageExecutor{
		Preparer: noPreparation{}, Evidence: head, Stderr: io.Discard, Worktree: fixedWorktree{},
		NewBaseEvidence: func(string) EvidenceStage { return base },
	}
	out, err := stages.Execute(t.Context(), AnalysisRequest{BaseRef: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if base.req.Root != baseScanRoot || base.req.DependencyRoot != headScanRoot {
		t.Errorf("base request root = %q, dependency root = %q; want %q, %q", base.req.Root, base.req.DependencyRoot, baseScanRoot, headScanRoot)
	}
	reasons := out.Diagnostic.Comparison.OriginReasons
	want := toolPrimary + ": head ok, base partial — base: pkg/a.go: cannot load; fix ."
	if len(reasons) != 1 || reasons[0] != want {
		t.Errorf("origin reasons = %q, want [%q]", reasons, want)
	}
	for _, r := range reasons {
		if strings.Contains(r, baseScanRoot) {
			t.Errorf("origin reason leaks the base checkout path: %q", r)
		}
	}
}
