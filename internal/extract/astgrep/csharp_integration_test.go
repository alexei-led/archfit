package astgrep_test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/astgrep"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const langCSharpStr = "dotnet"

type routeWant struct {
	name string
	line int
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
		name       string
		dir        string
		exclusions []string
		wantRoutes []routeWant
		wantStatus string
		wantReason string
	}{
		{
			name: "every route form",
			dir:  "routes",
			wantRoutes: []routeWant{
				{"api/orders", 3},
				{"HttpGet", 6},
				{"{id}", 9},
				{"create", 12},
				{"{id}", 15},
				{"{ref}", 16},
				{"/status", 24},
				{"/jobs", 25},
				{"/jobs/{id}", 26},
				{"/jobs/{id}", 27},
				{pathItems, 28},
				{"HttpGet", 34},
			},
			wantStatus: "ok",
		},
		{
			name:       "C# 14 extension block is partial and keeps routes elsewhere in the file",
			dir:        "extension",
			wantRoutes: []routeWant{{"kept", 11}},
			wantStatus: "partial",
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
			name:       "plain class has no route",
			dir:        "plain",
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
			for _, f := range facts {
				if f.Kind != kindRouteStr || f.Language != langCSharpStr || f.Exported {
					t.Errorf("fact %+v: want a non-exported csharp route", f)
				}
				got = append(got, routeWant{f.Name, f.StartLine})
			}
			if len(got) != len(tt.wantRoutes) {
				t.Fatalf("routes = %v, want %v", got, tt.wantRoutes)
			}
			for i := range got {
				if got[i] != tt.wantRoutes[i] {
					t.Errorf("routes[%d] = %v, want %v", i, got[i], tt.wantRoutes[i])
				}
			}
		})
	}
}
