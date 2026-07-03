package agenttask_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/agenttask"
	"github.com/alexei-led/archfit/internal/model/finding"
)

const rulePublicAPIMax = "public_api_max"

// writeFixtureFile creates a real file at root/relPath so the integration
// assertion below can os.Stat it — filesFor's contract is "exists on disk",
// not merely "is a key in a map".
func writeFixtureFile(t *testing.T, root, relPath string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write %s: %v", full, err)
	}
}

// assertFilesExistOnDisk is the integration assertion helper: every
// agent_tasks[].files[] entry must os.Stat successfully against the fixture
// root — the field agents trust blindly must never point at nothing.
func assertFilesExistOnDisk(t *testing.T, root string, files []string) {
	t.Helper()
	if len(files) == 0 {
		t.Fatal("files is empty, want at least one resolved path")
	}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err != nil {
			t.Errorf("files[] entry %q does not exist on disk: %v", f, err)
		}
	}
}

// buildKnownFiles walks root and returns the set of repo-relative slash paths
// seen, mirroring what the real LOC walk's FileClassIndex would contain.
func buildKnownFiles(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	knownFiles := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		knownFiles[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixture root: %v", err)
	}
	return knownFiles
}

// gateFindingWithModuleEdge builds a gate finding shaped like the ones
// publicAPIMax/publicAPIChange/publicAPITypeLeak emit: Edge.From/To.Path set
// to the bare config module key, no Locations — the exact shape filesFor must
// not pass through verbatim.
func gateFindingWithModuleEdge(modKey string) finding.Finding {
	return finding.Finding{
		ID:     "f1",
		Kind:   finding.KindGate,
		RuleID: rulePublicAPIMax,
		Status: finding.StatusNew,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Path: modKey},
			To:   finding.Endpoint{Path: modKey},
		},
		MatchedBy: map[string]string{"module": modKey},
		Why:       "test",
	}
}

func TestFilesFor_PerLanguageResolution(t *testing.T) {
	t.Run("go_module_key_resolves_to_root_dir_listing", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "widget/foo.go")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil)
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("widget")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := []string{"widget"}; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want[0] {
			t.Errorf("files = %v, want %v (the module's real root dir, not a fabricated path)", tasks[0].Files, want)
		}
	})

	t.Run("python_dotted_module_resolves_via_file_probe", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "myapp/domain.py")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil)
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("myapp.domain")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "myapp/domain.py"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (probed via myapp/domain.py, not the dotted id verbatim)", tasks[0].Files, want)
		}
	})

	t.Run("python_dotted_module_resolves_via_init_probe", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "myapp/domain/__init__.py")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil)
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("myapp.domain")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "myapp/domain/__init__.py"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q]", tasks[0].Files, want)
		}
	})

	t.Run("rust_crate_mod_key_resolves_via_crate_root", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "crates/mycrate/src/lib.rs")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, map[string]string{"mycrate": "crates/mycrate"}, nil)
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("mycrate::mymod")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "crates/mycrate"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (the crate's src dir, not the crate::mod id verbatim)", tasks[0].Files, want)
		}
	})

	t.Run("typescript_real_path_passes_through", func(t *testing.T) {
		// Unlike the bare module-key shape rules_api emits, TS edge-based
		// findings (forbidden_dependency et al.) already carry real file
		// paths on Edge.From/To.Path — filesFor must trust them as-is.
		root := t.TempDir()
		writeFixtureFile(t, root, "src/components/Button.tsx")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil)
		f := finding.Finding{
			ID:     "f1",
			Kind:   finding.KindGate,
			RuleID: ruleTypeForbidden,
			Status: finding.StatusNew,
			Edge: finding.EdgeEvidence{
				To: finding.Endpoint{Path: "src/components/Button.tsx"},
			},
		}
		tasks := agenttask.Build(
			[]finding.Finding{f},
			map[string]string{ruleTypeForbidden: ruleTypeForbidden},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "src/components/Button.tsx"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] passed through unchanged", tasks[0].Files, want)
		}
	})

	t.Run("unresolvable_entries_are_dropped_not_fabricated", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "unrelated/file.go")
		knownFiles := buildKnownFiles(t, root)

		// "ghost.module" resolves to nothing: not a known file/dir, not a
		// crate::mod, and its dotted-probe candidates don't exist either. With
		// no module root dir configured, files[] must come back empty rather
		// than fabricate the bare module key.
		resolver := agenttask.NewPathResolver(knownFiles, nil, nil)
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("ghost.module")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		if len(tasks[0].Files) != 0 {
			t.Errorf("files = %v, want empty (never a fabricated bare module key)", tasks[0].Files)
		}
	})

	t.Run("empty_after_drop_falls_back_to_module_root_dir", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "legacy/pkg/file.go")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, map[string]string{"ghostmod": "legacy/pkg"})
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("ghostmod")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "legacy/pkg"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (module root dir fallback)", tasks[0].Files, want)
		}
	})
}
