package astgrep_test

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/astgrep"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	langCSharpStr = "dotnet"
	statusPartial = "partial"
	pathJobID     = "/jobs/{id}"
)

type routeWant struct {
	name string
	line int
}

type functionWant struct {
	kind  string
	name  string
	start int
	end   int
}

func csharpFixture(t *testing.T, dir string) scope.Scope {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return scope.Scope{Root: filepath.Join(filepath.Dir(thisFile), "testdata", "csharp", dir)}
}

func TestSyntaxIntegration_CSharp(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration test — skipped with -short (use make test-fast)")
	}
	sgSkipGuard(t)

	tests := []struct {
		name          string
		dir           string
		exclusions    []string
		wantRoutes    []routeWant
		wantFunctions []functionWant
		wantStatus    string
		wantReason    string
	}{
		{
			name: "every route form",
			dir:  "routes",
			wantRoutes: []routeWant{
				{"api/orders", 3},
				{"{id}", 9},
				{"create", 12},
				{"{id}", 15},
				{"{ref}", 16},
				{"/status", 24},
				{"/jobs", 25},
				{pathJobID, 26},
				{pathJobID, 27},
				{pathItems, 28},
				{pathJobID, 42},
				{"/jobs", 43},
				// Class prefix of PrefixOnlyController: the only route its
				// [HttpGet("")] and [HttpPost("")] actions address.
				{"v2/{organizationId}/groups", 57},
			},
			wantStatus: "ok",
		},
		{
			name: "methods, constructors, and local functions with a body",
			dir:  "functions",
			wantFunctions: []functionWant{
				{kindMethodStr, "Base", 10, 13},
				{kindMethodStr, "Sum", 17, 25},
				{kindFunctionStr, "Twice", 19, 22},
				{kindMethodStr, "Arrow", 27, 27},
			},
			wantStatus: "ok",
		},
		{
			name:       "C# 14 extension block is partial and keeps routes elsewhere in the file",
			dir:        "extension",
			wantRoutes: []routeWant{{"kept", 11}},
			wantStatus: statusPartial,
			wantReason: "Extensions.cs:",
		},
		{
			// An excluded fixture's parse error hides no analysed route; its
			// facts are unchanged, as for every other language.
			name:       "a parse error in an excluded file leaves the row complete",
			dir:        "extension",
			exclusions: []string{"**/Extensions.cs"},
			wantRoutes: []routeWant{{"kept", 11}},
			wantStatus: "ok",
		},
		{
			// #if inside a fluent chain splits one statement: tree-sitter sees
			// the directive mid-expression and reports ERROR. Preprocessing
			// would need the build's symbols, so the file is named instead.
			name:       "#if splitting a statement is partial and keeps routes in other files",
			dir:        "preprocessor",
			wantRoutes: []routeWant{{"api/kept", 1}, {"/before", 9}, {"/after", 15}},
			wantStatus: statusPartial,
			wantReason: "syntax errors in 1 file(s), routes there may be missing: Program.cs:",
		},
		{
			name: "plain class has no route",
			dir:  "plain",
			wantFunctions: []functionWant{
				{kindMethodStr, "Add", 3, 3},
				{kindMethodStr, "Old", 5, 6},
			},
			wantStatus: "ok",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := astgrep.New(toolrun.New())
			a.Exclusions = tt.exclusions
			facts, cov, err := a.Syntax(context.Background(), csharpFixture(t, tt.dir), []string{langCSharpStr})
			if err != nil {
				t.Fatalf("Syntax: %v", err)
			}
			if cov.Status != tt.wantStatus {
				t.Errorf("cov.Status = %q (reason %q), want %q", cov.Status, cov.Reason, tt.wantStatus)
			}
			if tt.wantReason != "" && !strings.Contains(cov.Reason, tt.wantReason) {
				t.Errorf("cov.Reason = %q, want it to contain %q", cov.Reason, tt.wantReason)
			}
			var got []routeWant
			var gotFunctions []functionWant
			for _, f := range facts {
				if f.Language != langCSharpStr || f.Exported {
					t.Errorf("fact %+v: want a non-exported csharp fact", f)
				}
				if f.Kind == kindRouteStr {
					got = append(got, routeWant{f.Name, f.StartLine})
				} else {
					gotFunctions = append(gotFunctions, functionWant{f.Kind, f.Name, f.StartLine, f.EndLine})
				}
			}
			if !slices.Equal(got, tt.wantRoutes) {
				t.Errorf("routes = %v, want %v", got, tt.wantRoutes)
			}
			if tt.wantFunctions != nil && !slices.Equal(gotFunctions, tt.wantFunctions) {
				t.Errorf("functions = %v, want %v", gotFunctions, tt.wantFunctions)
			}
		})
	}
}
