package ts

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestNativeSnapshotRuntimeFallbackPreservesGraphWithoutIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, snapshotCJSConfig), []byte("module.exports = {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	ex := &Extractor{runner: &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		calls++
		if i := slices.Index(cmd.Args, "--config"); i >= 0 {
			if err := os.WriteFile(filepath.Join(filepath.Dir(cmd.Args[i+1]), "snapshot.json"), []byte(`{"fallback":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			return toolrun.Output{ExitCode: 1}, nil
		}
		return toolrun.Output{Stdout: []byte("observed facts")}, nil
	}}}
	out, hash, _, err := ex.runWithConfigSnapshot(context.Background(), scope.Scope{Root: root}, "17.4.3", "", root, toolrun.ToolCmd{Name: "launcher", Args: []string{"depcruise"}})
	if err != nil || string(out.Stdout) != "observed facts" || hash != "" || calls != 2 {
		t.Fatalf("fallback output=%s hash=%s calls=%d error=%v", out.Stdout, hash, calls, err)
	}
}
