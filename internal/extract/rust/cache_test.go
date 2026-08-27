package rust_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"

	"github.com/alexei-led/archfit/internal/extract/rust"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// rustCacheRunner fakes cargo + cargo-modules: --version calls return pinned
// versions, `cargo metadata` returns a single-member workspace rooted at
// root, `cargo-modules` returns a minimal DOT graph. The two counters count
// only the real scans, not version probes.
func rustCacheRunner(root string, metadataCalls, modulesCalls *int) *toolrun.RunnerMock {
	metadata := fmt.Sprintf(`{
		"packages": [{
			"id": "pkg-a 0.1.0",
			"name": "pkg-a",
			"manifest_path": %q,
			"dependencies": [],
			"targets": [{"name": "pkg-a", "kind": ["lib"]}]
		}],
		"workspace_members": ["pkg-a 0.1.0"],
		"workspace_root": %q
	}`, filepath.Join(root, "Cargo.toml"), root)
	dot := `digraph {
"pkg_a" [label="crate|pkg_a"]; // "crate" node
"pkg_a::api" [label="pub mod|api"]; // "mod" node
}`
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, true
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			switch {
			case slices.Contains(cmd.Args, "--version"):
				return toolrun.Output{Stdout: []byte(cmd.Name + " 1.99.0\n")}, nil
			case cmd.Name == testTool && slices.Contains(cmd.Args, "metadata"):
				*metadataCalls++
				return toolrun.Output{Stdout: []byte(metadata)}, nil
			case strings.Contains(cmd.Name, "cargo-modules"):
				*modulesCalls++
				return toolrun.Output{Stdout: []byte(dot)}, nil
			default:
				return toolrun.Output{}, nil
			}
		},
	}
}

func writeRustFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{
		"Cargo.toml": "[package]\nname = \"pkg-a\"\nversion = \"0.1.0\"\n",
		"src/lib.rs": "pub mod api;\n",
		"src/api.rs": "pub fn hello() {}\n",
	} {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestFactCache_CargoMetadataAndModules pins the Rust cache contract:
//   - warm run: neither cargo metadata nor cargo-modules re-runs;
//   - .rs edit: cargo-modules re-runs (it reads source) but cargo metadata
//     stays cached (its key is manifests-only);
//   - Cargo.toml edit: both re-run.
func TestFactCache_CargoMetadataAndModules(t *testing.T) {
	t.Parallel()
	root := writeRustFixture(t)
	metadataCalls, modulesCalls := 0, 0
	runner := rustCacheRunner(root, &metadataCalls, &modulesCalls)
	ex := rust.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, ModuleGraph: true})
	ex.Cache = factcache.NewStore(t.TempDir())
	ctx := context.Background()
	s := scope.Scope{Root: root}

	extract := func() {
		t.Helper()
		if _, _, err := ex.Extract(ctx, s); err != nil {
			t.Fatalf("Extract: %v", err)
		}
	}

	extract()
	if metadataCalls != 1 || modulesCalls != 1 {
		t.Fatalf("cold run: want 1 metadata + 1 modules call, got %d/%d", metadataCalls, modulesCalls)
	}
	extract()
	if metadataCalls != 1 || modulesCalls != 1 {
		t.Fatalf("warm run: want full cache hit, got %d/%d", metadataCalls, modulesCalls)
	}

	// .rs edit: module graph invalidates, metadata does not.
	if err := os.WriteFile(filepath.Join(root, "src", "api.rs"), []byte("pub fn hello2() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	extract()
	if metadataCalls != 1 {
		t.Errorf("after .rs edit: cargo metadata must stay cached, got %d calls", metadataCalls)
	}
	if modulesCalls != 2 {
		t.Errorf("after .rs edit: cargo-modules must re-run, got %d calls", modulesCalls)
	}

	// Cargo.toml edit: everything invalidates.
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"),
		[]byte("[package]\nname = \"pkg-a\"\nversion = \"0.2.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	extract()
	if metadataCalls != 2 {
		t.Errorf("after Cargo.toml edit: want metadata re-run, got %d calls", metadataCalls)
	}
	if modulesCalls != 3 {
		t.Errorf("after Cargo.toml edit: want cargo-modules re-run, got %d calls", modulesCalls)
	}
}
