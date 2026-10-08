package astgrep_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/astgrep"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/pattern"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestFactCache_ASTIgnoresBinaryBuildOutputs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "input.go"), []byte("package input"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "target"), 0o750); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(root, "target", "output.bin")) //nolint:gosec // isolated test fixture
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(2 << 30); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	scans := 0
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, true
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("ast-grep 0.30.0")}, nil
			}
			scans++
			return toolrun.Output{Stdout: []byte("[]")}, nil
		},
	}
	a := astgrep.New(runner)
	a.Cache = factcache.NewStore(t.TempDir())
	find := func() {
		t.Helper()
		if _, _, err := a.Find(context.Background(), scope.Scope{Root: root}, pattern.Config{{ID: "p", Lang: "go", Rule: "func $F"}}); err != nil {
			t.Fatal(err)
		}
	}
	find()
	if err := os.WriteFile(filepath.Join(root, "target", "output.bin"), []byte("rebuilt"), 0o600); err != nil {
		t.Fatal(err)
	}
	find()
	if scans != 1 {
		t.Fatalf("binary build output must not invalidate AST input cache: scans=%d", scans)
	}
	if err := os.WriteFile(filepath.Join(root, "target", "input.go"), []byte("package generated"), 0o600); err != nil {
		t.Fatal(err)
	}
	find()
	if scans != 2 {
		t.Fatalf("source in target must still invalidate AST cache: scans=%d", scans)
	}
}
