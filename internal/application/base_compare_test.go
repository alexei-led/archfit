package application

import (
	"context"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

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
	out, err := stages.Execute(t.Context(), AnalysisRequest{BaseRef: blockBaseRef})
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

// TestBaseComparison_LongOriginReasonIsDisclosedInFull: an origin reason names
// every project behind a degraded base side and can run far past the report's
// one-line bound. The projected line stays bounded; stderr carries it whole.
func TestBaseComparison_LongOriginReasonIsDisclosedInFull(t *testing.T) {
	long := "cannot load: " + strings.Repeat("src/Some/Project.csproj, ", 30) + "src/Last/Project.csproj"
	head := &coverageStage{root: headScanRoot, row: modevidence.Coverage{Tool: toolPrimary, Status: modevidence.StatusOK}}
	base := &coverageStage{root: baseScanRoot, row: modevidence.Coverage{Tool: toolPrimary, Status: modevidence.StatusPartial, Reason: long}}
	var stderr strings.Builder
	stages := StageExecutor{
		Preparer: noPreparation{}, Evidence: head, Stderr: &stderr, Worktree: fixedWorktree{},
		NewBaseEvidence: func(string) EvidenceStage { return base },
	}
	out, err := stages.Execute(t.Context(), AnalysisRequest{BaseRef: blockBaseRef})
	if err != nil {
		t.Fatal(err)
	}
	want := toolPrimary + ": head ok, base partial — base: " + long
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr does not carry the whole origin reason:\n%s", stderr.String())
	}
	reasons := ProjectReport(out.Diagnostic, out.Score).State.Comparison.OriginReasons
	if len(reasons) != 1 {
		t.Fatalf("projected origin reasons = %q, want one", reasons)
	}
	if n := utf8.RuneCountInString(reasons[0]); n > maxReportTextRunes || !strings.HasSuffix(reasons[0], "…") {
		t.Errorf("projected origin reason is %d runes, %q; want a bounded line ending in an ellipsis", n, reasons[0])
	}
}

// TestBaseComparison_BaseDegradationIsDisclosedOnStderr: the head run prints
// each analyzer's full reason from Execute; the base sub-run bypasses Execute,
// so a base-side partial row was only visible as the comparison's bounded reason.
// It now gets the same disclosure, labelled with the base prefix.
func TestBaseComparison_BaseDegradationIsDisclosedOnStderr(t *testing.T) {
	long := "13 complete, 54 partial, 18 failed: " + strings.Repeat("src/Some/Project.csproj partial: no reference pack; ", 20)
	head := &coverageStage{root: headScanRoot, row: modevidence.Coverage{Tool: toolPrimary, Status: modevidence.StatusOK}}
	base := &coverageStage{root: baseScanRoot, row: modevidence.Coverage{Tool: toolPrimary, Status: modevidence.StatusPartial, Reason: long}}
	var stderr strings.Builder
	stages := StageExecutor{
		Preparer: noPreparation{}, Evidence: head, Stderr: &stderr, Worktree: fixedWorktree{},
		NewBaseEvidence: func(string) EvidenceStage { return base },
	}
	if _, err := stages.Execute(t.Context(), AnalysisRequest{BaseRef: blockBaseRef}); err != nil {
		t.Fatal(err)
	}
	got := stderr.String()
	if !strings.Contains(got, "warning: [base] "+toolPrimary+" is partial") {
		t.Errorf("stderr does not carry a base-labelled coverage warning:\n%s", got)
	}
	if !strings.Contains(got, long) {
		t.Errorf("stderr does not carry the base analyzer's full reason:\n%s", got)
	}
	if strings.Contains(got, "warning: "+toolPrimary) {
		t.Errorf("the ok head row was disclosed as degraded:\n%s", got)
	}
}

// TestBaseComparison_ShortOriginReasonAddsNoStderr keeps stderr quiet for the
// ordinary case: a reason within the bound is published as is.
func TestBaseComparison_ShortOriginReasonAddsNoStderr(t *testing.T) {
	head := &coverageStage{root: headScanRoot, row: modevidence.Coverage{Tool: toolPrimary, Status: modevidence.StatusOK}}
	base := &coverageStage{root: baseScanRoot, row: modevidence.Coverage{Tool: toolPrimary, Status: modevidence.StatusPartial, Reason: "cannot load: one package"}}
	var stderr strings.Builder
	stages := StageExecutor{
		Preparer: noPreparation{}, Evidence: head, Stderr: &stderr, Worktree: fixedWorktree{},
		NewBaseEvidence: func(string) EvidenceStage { return base },
	}
	if _, err := stages.Execute(t.Context(), AnalysisRequest{BaseRef: blockBaseRef}); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Errorf("a report-safe origin reason was disclosed on stderr: %q", stderr.String())
	}
}
