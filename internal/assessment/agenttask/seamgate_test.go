package agenttask_test

import (
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

const (
	seamFrom      = "web"
	seamTo        = "billing"
	seamGoFile    = "web/handler.go"
	seamGoPackage = "billing/ledger"
	seamTSFile    = "billing/api.ts"
	seamTSSource  = "web/app.ts"
	seamRustFile  = "crates/billing/src/ledger.rs"
)

// couplingGateFinding is shaped like the finding the seam gate emits: a module
// pair and nothing else — no paths, no import sites.
func couplingGateFinding() finding.Finding {
	return finding.Finding{
		ID: "coupling-gate/seam", Kind: finding.KindGate, RuleID: finding.RuleIDCouplingGate,
		Status: finding.StatusNew,
		Edge:   finding.EdgeEvidence{From: finding.Endpoint{Module: seamFrom}, To: finding.Endpoint{Module: seamTo}},
		Why:    "newly introduced distributed-monolith seam: web -> billing",
	}
}

// TestBuild_CouplingGateTaskNamesSeamFiles pins the seam gate's files[]: a
// blocking seam names the files behind its qualifying edges, resolved on disk
// per language, and falls back to the source module's root rather than ship a
// file-less task. Only the seam the finding names contributes.
func TestBuild_CouplingGateTaskNamesSeamFiles(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{
		seamGoFile, seamGoPackage + "/ledger.go", seamTSSource, seamTSFile,
		"crates/web/src/handler.rs", seamRustFile,
	} {
		writeFixtureFile(t, root, f)
	}
	resolver := agenttask.NewPathResolver(buildKnownFiles(t, root),
		map[string]string{seamFrom: "crates/web", seamTo: "crates/billing"},
		map[string]string{seamFrom: seamFrom, seamTo: seamTo}, nil)

	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{"go import site and target package", []string{seamGoFile, seamGoPackage, seamGoFile}, []string{seamGoPackage, seamGoFile}},
		{"typescript edges without import sites", []string{seamTSSource, seamTSFile}, []string{seamTSFile, seamTSSource}},
		{"rust module keys", []string{"web::handler", "billing::ledger"}, []string{seamRustFile, "crates/web/src/handler.rs"}},
		{"unresolvable evidence is dropped", []string{"web/ghost.go", seamGoPackage}, []string{seamGoPackage}},
		{"nothing resolves falls back to the source module root", []string{"ghost/x.go"}, []string{seamFrom}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seams := []result.Seam{
				{FromModule: seamFrom, ToModule: seamTo, QualifyingPaths: tc.paths},
				{FromModule: seamTo, ToModule: seamFrom, QualifyingPaths: []string{seamRustFile}},
			}
			tasks := agenttask.Build([]finding.Finding{couplingGateFinding()}, nil, nil, nil, nil, seams, resolver)
			if len(tasks) != 1 {
				t.Fatalf("tasks = %d, want 1", len(tasks))
			}
			if !reflect.DeepEqual(tasks[0].Files, tc.want) {
				t.Errorf("files = %v, want %v", tasks[0].Files, tc.want)
			}
			assertFilesExistOnDisk(t, root, tasks[0].Files)
		})
	}
}

// TestBuild_CouplingGateTaskCarriesNoDeclarations pins the size bound: a seam
// task's files are evidence across two modules, up to forty of them, so their
// declarations stay out of the task.
func TestBuild_CouplingGateTaskCarriesNoDeclarations(t *testing.T) {
	seams := []result.Seam{{FromModule: seamFrom, ToModule: seamTo, QualifyingPaths: []string{seamGoFile}}}
	facts := []evidence.SyntaxFact{{File: seamGoFile, Kind: kindFunction, Name: "Handle", StartLine: 3}}
	tasks := agenttask.Build([]finding.Finding{couplingGateFinding()}, nil, nil, nil, facts, seams, agenttask.PathResolver{})
	if len(tasks) != 1 || len(tasks[0].Files) != 1 {
		t.Fatalf("tasks = %+v, want one task naming the seam file", tasks)
	}
	if tasks[0].Declarations != nil {
		t.Errorf("declarations = %+v, want none on a seam-gate task", tasks[0].Declarations)
	}
}
