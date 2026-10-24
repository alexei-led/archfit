package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestDoctorDiscoversOwnedAnalyzerPrograms(t *testing.T) {
	t.Chdir(t.TempDir())
	seen := map[string]bool{}
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			seen[name] = true
			return toolrun.ToolInfo{}, false
		},
	}
	var output bytes.Buffer
	if err := (&DoctorCmd{}).Run(&appDeps{Runner: runner, Stdout: &output}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ast-grep", "depcruise", "node"} {
		if !seen[name] {
			t.Errorf("doctor did not inspect %s", name)
		}
	}
	for _, name := range []string{"sg", "bunx", "npx"} {
		if seen[name] {
			t.Errorf("doctor inspected obsolete launcher %s", name)
		}
	}
}
