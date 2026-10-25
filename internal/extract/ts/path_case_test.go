package ts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestOwnedProgramsRejectCaseAliasesOfTargetFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin", "node")
	if err := os.WriteFile(executable, []byte("decoy"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := strings.Replace(executable, string(filepath.Separator)+"target"+string(filepath.Separator), string(filepath.Separator)+"TARGET"+string(filepath.Separator), 1)
	if _, err := os.Stat(alias); err != nil {
		t.Skip("requires a case-insensitive filesystem")
	}
	runner := &toolrun.RunnerMock{DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
		return toolrun.ToolInfo{Name: name, Path: alias}, true
	}}
	extractor := &Extractor{runner: runner}
	if path, err := extractor.nodeInterpreterPath(context.Background(), root); err == nil {
		t.Errorf("accepted target-owned Node through case alias: %s", path)
	}
	if path, err := extractor.producerPath(context.Background(), root); err == nil {
		t.Errorf("accepted target-owned producer through case alias: %s", path)
	}
}
