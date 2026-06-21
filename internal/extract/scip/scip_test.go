package scip_test

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/scip"
	"github.com/alexei-led/archfit/internal/ports"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// absentRunner returns a RunnerMock where Detect always reports tools as missing.
func absentRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
}

// presentRunner returns a RunnerMock where Detect reports scip-typescript as present.
func presentRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == "scip-typescript" {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/scip-typescript"}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 0}, nil
		},
	}
}

func TestAdapter_Name(t *testing.T) {
	a := scip.New(absentRunner())
	if a.Name() != "scip" {
		t.Errorf("Name() = %q, want %q", a.Name(), "scip")
	}
}

// TestResolve_AbsentTool_IdentityResolver asserts that when no SCIP indexer is
// found on PATH, Resolve returns toPath unchanged with confidence "low"
// (identity, no resolution capability).
func TestResolve_AbsentTool_IdentityResolver(t *testing.T) {
	a := scip.New(absentRunner())
	ctx := context.Background()

	cases := []struct {
		fromFile string
		toPath   string
	}{
		{"src/index.ts", "src/components/index.ts"},
		{"pkg/main.go", "pkg/utils"},
		{"app/main.py", "app/models/__init__.py"},
	}

	for _, tc := range cases {
		realPath, confidence := a.Resolve(ctx, tc.fromFile, tc.toPath)
		if realPath != tc.toPath {
			t.Errorf("Resolve(%q, %q): realPath = %q, want %q (identity)", tc.fromFile, tc.toPath, realPath, tc.toPath)
		}
		if confidence != "low" {
			t.Errorf("Resolve(%q, %q): confidence = %q, want %q", tc.fromFile, tc.toPath, confidence, "low")
		}
	}
}

// TestResolve_PresentTool_IdentityResolver asserts that even when a SCIP indexer
// is detected, Resolve returns toPath unchanged with confidence "medium" (full
// .scip parsing pending importable Go bindings).
func TestResolve_PresentTool_IdentityResolver(t *testing.T) {
	a := scip.New(presentRunner())
	ctx := context.Background()

	barrelPath := "src/components/index.ts"
	realPath, confidence := a.Resolve(ctx, "src/app.ts", barrelPath)
	if realPath != barrelPath {
		t.Errorf("realPath = %q, want %q", realPath, barrelPath)
	}
	if confidence != "medium" {
		t.Errorf("confidence = %q, want %q", confidence, "medium")
	}
}

// TestResolve_DetectCalledOnce asserts that tool detection runs only once
// regardless of how many times Resolve is called.
func TestResolve_DetectCalledOnce(t *testing.T) {
	detectCount := 0
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			detectCount++
			return toolrun.ToolInfo{}, false
		},
	}

	a := scip.New(runner)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a.Resolve(ctx, "from.ts", "to.ts")
	}

	// Detect is called once per known tool (4 tools checked: scip-typescript,
	// scip-python, scip-go, rust-analyzer), all absent so all 4 calls happen on
	// first Resolve, then sync.Once prevents re-detection.
	if detectCount > 4 {
		t.Errorf("Detect called %d times across 5 Resolve calls; want ≤4 (sync.Once)", detectCount)
	}
}

// Compile-time interface check.
var _ ports.SymbolResolver = (*scip.Adapter)(nil)
