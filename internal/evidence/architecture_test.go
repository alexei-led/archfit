package evidence

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestSnapshotContractHasNoAssessmentPolicyOrReportImports pins the neutral
// evidence contract: it records what the tools observed and never reaches into
// the packages that judge, configure, or render it. Every production file in
// the package is checked, so a new contract file cannot escape the rule.
func TestSnapshotContractHasNoAssessmentPolicyOrReportImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		tree, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range tree.Imports {
			pkg := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(pkg, "/assessment/") || strings.HasSuffix(pkg, "/policy") || strings.Contains(pkg, "/report") {
				t.Fatalf("%s imports forbidden contract package %s", name, pkg)
			}
		}
	}
}
