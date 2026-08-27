package finding_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	nodeFromFile     = "file:pkg/a/a.go"
	nodeToFile       = "file:pkg/b/internal/impl.go"
	pathFrom         = "pkg/a/a.go"
	testUsesInternal = "uses_internal"
)

func TestNew_StableID(t *testing.T) {
	edge := relationship.Edge{
		FromID: nodeFromFile,
		ToID:   nodeToFile,
		Kind:   testUsesInternal,
	}
	locs := []relationship.Location{{File: pathFrom, Line: 5}}

	f1 := finding.New("public_api_only", edge, locs)
	f2 := finding.New("public_api_only", edge, locs)

	if f1.ID != f2.ID {
		t.Errorf("same inputs produced different IDs: %q vs %q", f1.ID, f2.ID)
	}
}

func TestNew_IDLength(t *testing.T) {
	edge := relationship.Edge{
		FromID: nodeFromFile,
		ToID:   nodeToFile,
		Kind:   testUsesInternal,
	}
	f := finding.New("public_api_only", edge, nil)

	if len(f.ID) != 32 {
		t.Errorf("expected ID length 32, got %d: %q", len(f.ID), f.ID)
	}
}

func TestNew_LineNumbersDoNotAffectID(t *testing.T) {
	edge := relationship.Edge{
		FromID: nodeFromFile,
		ToID:   nodeToFile,
		Kind:   testUsesInternal,
	}

	locsA := []relationship.Location{{File: pathFrom, Line: 5}}
	locsB := []relationship.Location{{File: pathFrom, Line: 99}}

	fA := finding.New("public_api_only", edge, locsA)
	fB := finding.New("public_api_only", edge, locsB)

	if fA.ID != fB.ID {
		t.Errorf("different line numbers produced different IDs: %q vs %q", fA.ID, fB.ID)
	}
}

func TestNew_DifferentRuleProducesDifferentID(t *testing.T) {
	edge := relationship.Edge{
		FromID: nodeFromFile,
		ToID:   nodeToFile,
		Kind:   testUsesInternal,
	}

	f1 := finding.New("public_api_only", edge, nil)
	f2 := finding.New("forbidden_dependency", edge, nil)

	if f1.ID == f2.ID {
		t.Errorf("different ruleIDs should produce different IDs, got same: %q", f1.ID)
	}
}

func TestNew_DifferentEdgeProducesDifferentID(t *testing.T) {
	e1 := relationship.Edge{FromID: nodeFromFile, ToID: "file:pkg/b/b.go", Kind: "imports"}
	e2 := relationship.Edge{FromID: nodeFromFile, ToID: "file:pkg/c/c.go", Kind: "imports"}

	f1 := finding.New("forbidden_dependency", e1, nil)
	f2 := finding.New("forbidden_dependency", e2, nil)

	if f1.ID == f2.ID {
		t.Errorf("different edges should produce different IDs, got same: %q", f1.ID)
	}
}

func TestNew_Defaults(t *testing.T) {
	edge := relationship.Edge{
		FromID: "module:svc/a",
		ToID:   "module:svc/b",
		Kind:   "depends_on",
	}
	f := finding.New("forbidden_dependency", edge, nil)

	if f.Kind != "gate" {
		t.Errorf("Kind: got %q, want %q", f.Kind, "gate")
	}
	if f.Status != finding.StatusNew {
		t.Errorf("Status: got %q, want %q", f.Status, finding.StatusNew)
	}
	if f.RuleID != "forbidden_dependency" {
		t.Errorf("RuleID: got %q, want %q", f.RuleID, "forbidden_dependency")
	}
}

func TestNew_EdgeEvidencePaths(t *testing.T) {
	edge := relationship.Edge{
		FromID: nodeFromFile,
		ToID:   "package:pkg/b/internal",
		Kind:   testUsesInternal,
	}
	f := finding.New("public_api_only", edge, nil)

	if f.Edge.From.Path != pathFrom {
		t.Errorf("Edge.From.Path: got %q, want %q", f.Edge.From.Path, pathFrom)
	}
	if f.Edge.To.Path != "pkg/b/internal" {
		t.Errorf("Edge.To.Path: got %q, want %q", f.Edge.To.Path, "pkg/b/internal")
	}
	if f.Edge.Kind != testUsesInternal {
		t.Errorf("Edge.Kind: got %q, want %q", f.Edge.Kind, testUsesInternal)
	}
	// Module labels are empty until engine assembly
	if f.Edge.From.Module != "" {
		t.Errorf("Edge.From.Module should be empty, got %q", f.Edge.From.Module)
	}
	if f.Edge.To.Module != "" {
		t.Errorf("Edge.To.Module should be empty, got %q", f.Edge.To.Module)
	}
}

func TestNew_GoldenID(t *testing.T) {
	// Pin a known-good ID so any formula change is immediately visible.
	edge := relationship.Edge{
		FromID: nodeFromFile,
		ToID:   nodeToFile,
		Kind:   testUsesInternal,
	}
	f := finding.New("public_api_only", edge, nil)

	// Computed once: sha256("public_api_only\x00file:pkg/a/a.go\x00file:pkg/b/internal/impl.go\x00uses_internal")[:16] hex
	const want = "4afe8eb98ff923287da178292a38eec7"
	if f.ID != want {
		t.Errorf("golden ID mismatch: got %q, want %q", f.ID, want)
	}
}
