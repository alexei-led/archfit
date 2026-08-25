package signal

import (
	"testing"

	"github.com/alexei-led/archfit/internal/relationship"
)

func TestCommonInputCarriesNarrowAssessmentViews(t *testing.T) {
	const changedFile = "a.go"
	relationships := relationship.Set{Edges: []relationship.Edge{{FromPath: changedFile, ToPath: "b.go", Kind: "imports"}}}
	coverage := CoverageView{{Tool: "scip", Status: "partial", FilesSeen: 10, FilesApplicable: 12, Unresolved: 2}}
	input := CommonInput{Relationships: relationships, Coverage: coverage, ChangedFiles: []string{changedFile}}

	if len(input.Relationships.Edges) != 1 || input.Relationships.Edges[0].Kind != "imports" {
		t.Fatalf("relationship input lost contract data: %+v", input.Relationships)
	}
	if len(input.Coverage) != 1 || input.Coverage[0].Tool != "scip" || input.Coverage[0].Status != "partial" || input.Coverage[0].Unresolved != 2 {
		t.Fatalf("coverage view lost assessment data: %+v", input.Coverage)
	}
	if len(input.ChangedFiles) != 1 || input.ChangedFiles[0] != changedFile {
		t.Fatalf("changed files = %v", input.ChangedFiles)
	}
}
