package evaluation_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluationDoesNotImportRawRelationshipInternals(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		tree, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range tree.Imports {
			name := strings.Trim(imp.Path.Value, `"`)
			if strings.HasSuffix(name, "/internal/model/graph") || strings.Contains(name, "/internal/relationship/coupling") {
				t.Fatalf("assessment evaluation imports raw relationship internals: %s", name)
			}
		}
	}
}
