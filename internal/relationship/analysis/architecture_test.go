package analysis_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalysisDoesNotImportAssessmentOrReport(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		checked++
		tree, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range tree.Imports {
			name := strings.Trim(imp.Path.Value, `"`)
			reportImport := "/internal/" + "model/" + "report"
			if strings.Contains(name, "/internal/assessment/") || strings.Contains(name, "/internal/report") || strings.Contains(name, reportImport) {
				t.Fatalf("relationship analysis imports assessment/report: %s", name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no production files were inspected — this rule checked nothing")
	}
}
