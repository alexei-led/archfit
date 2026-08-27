package evaluation_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// internalPrefix is assembled rather than spelled out so a repository-wide
// search for a forbidden import path does not match this guard itself.
const internalPrefix = "github.com/alexei-led/archfit/" + "internal/"

// TestEvaluationImportsOnlyDomainContracts pins the evaluation boundary: the
// evaluator decides over the relationship contract and its own assessment
// values, never over the extractor graph, the classifier index, or the neutral
// evidence snapshot the relationship stage consumes.
func TestEvaluationImportsOnlyDomainContracts(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		internalPrefix + "model/graph",
		internalPrefix + "relationship/coupling",
		internalPrefix + "evidence",
		internalPrefix + "model/report",
	}
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
			for _, bad := range forbidden {
				if name == bad || strings.HasPrefix(name, bad+"/") {
					t.Errorf("%s imports %s: assessment decides over contracts, not over acquisition internals or report DTOs", path, name)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no production files were inspected — this rule checked nothing")
	}
}
