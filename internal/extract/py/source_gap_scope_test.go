package py_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/py"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestPythonSourceGapsRespectExactFileExclusions(t *testing.T) {
	for _, tc := range []struct {
		name               string
		exclusions         []string
		total, want, files int
	}{
		{"included sources", nil, 3, 3, 3},
		{"one excluded source", []string{testSourceA}, 3, 2, 2},
		{"anonymous graph failure retained", []string{testSourceA, "myapp/b.py"}, 3, 1, 1},
		{"all located gaps excluded", []string{testSourceA, "myapp/b.py"}, 2, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"modules":["myapp","myapp.a","myapp.b"],"sources":[
{"file":"myapp/__init__.py","module":"myapp","parsed":true},
{"file":"myapp/a.py","module":"myapp.a","parsed":false},
{"file":"myapp/b.py","module":"myapp.b","parsed":false}],
"edges":[{"importer":"myapp.a","imported":"myapp.b","line":1}],
"external_imports":[{"importer":"myapp.a","imported":"absent_external","line":2}],
"unresolved":%d,"unresolved_imports":[
{"file":"myapp/a.py","importer":"myapp.a","reason":"SyntaxError under Python 3.13"},
{"file":"myapp/b.py","importer":"myapp.b","reason":"unreadable"}]}`, tc.total)
			runner := &toolrun.RunnerMock{
				DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{Name: "uv"}, true },
				RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
					return toolrun.Output{Stdout: []byte(body)}, nil
				},
			}
			ex := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, PyPackage: testPkgName, Exclusions: tc.exclusions})
			facts, coverage, err := ex.Extract(t.Context(), scope.Scope{Root: testRoot})
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Unresolved != tc.want || facts.Unresolved != tc.want || coverage.UnresolvedInputsMissing != tc.want || coverage.FilesApplicable != tc.files {
				t.Fatalf("gap identity or scoped file counts lost: coverage=%+v facts=%+v", coverage, facts)
			}
			if (coverage.Status == evidence.StatusOK) != (tc.want == 0) {
				t.Fatalf("coverage=%+v", coverage)
			}
			if len(tc.exclusions) > 0 && (len(facts.Edges) != 0 || strings.Contains(coverage.Reason, testSourceA)) {
				t.Fatalf("excluded source still affects the result: edges=%+v reason=%q", facts.Edges, coverage.Reason)
			}
		})
	}
}

func TestPythonPackageFailureCannotBeClearedByExcludingInitializer(t *testing.T) {
	body := `{"modules":["myapp"],"sources":[{"file":"myapp/__init__.py","module":"myapp","parsed":false}],"unresolved":1,"unresolved_imports":[{"importer":"myapp","imported":"myapp","reason":"directory walk failed"}]}`
	runner := &toolrun.RunnerMock{
		DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{Name: "uv"}, true },
		RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(body)}, nil
		},
	}
	_, coverage, err := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, PyPackage: testPkgName, Exclusions: []string{"myapp/__init__.py"}}).Extract(t.Context(), scope.Scope{Root: testRoot})
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Status != evidence.StatusPartial || coverage.Unresolved != 1 {
		t.Fatalf("unlocated package failure was waived: %+v", coverage)
	}
}
