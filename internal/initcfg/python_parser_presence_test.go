package initcfg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestDiscoverPythonMetadataWithoutPackagesRemainsAuto(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for name, body := range map[string]string{"pyproject.toml": "[project]\n", "app.py": "import os\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	presence := ProbePresence(root)
	if presence.Python {
		t.Fatal("packaging metadata and loose scripts are not a selected Python package")
	}
	cfg, err := Discover(context.Background(), root, &toolrun.RunnerMock{}, presence)
	if err != nil {
		t.Fatal(err)
	}
	if output := Render(cfg, nil, false); !strings.Contains(output, "  python:\n    enabled: auto\n") {
		t.Fatalf("Python imports do not apply to the tree, want auto:\n%s", output)
	}
}
