package evidence

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotContractHasNoAssessmentPolicyOrReportImports(t *testing.T) {
	for _, name := range []string{"snapshot.go", "signals.go"} {
		path := filepath.Join(".", name)
		tree, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
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
