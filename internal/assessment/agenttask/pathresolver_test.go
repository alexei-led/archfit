package agenttask_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/agenttask"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/relationship"
)

const rulePublicAPIMax = "public_api_max"

// Shared fixture paths hoisted for goconst.
const (
	fixtureCrateDir = "crates/mycrate"
	fixtureRealGo   = "pkg/real.go"
	fixtureCrate    = "mycrate"
	fixturePkgAGo   = "pkg/a.go"
	fixturePkgBGo   = "pkg/b.go"
	fixturePkgDir   = "pkg"
)

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

// TestNewPathResolver_ZeroKnownFilesTrustsEverything mirrors the zero-value
// PathResolver{} tests (agenttask_test.go) but goes through the constructor:
// a nil knownFiles map disables resolution, so any non-empty, non-escaping
// candidate passes through unchanged.
func TestNewPathResolver_ZeroKnownFilesTrustsEverything(t *testing.T) {
	resolver := agenttask.NewPathResolver(nil, nil, nil, nil)
	f := finding.Finding{
		ID:     "f1",
		Kind:   finding.KindGate,
		RuleID: ruleTypeForbidden,
		Status: finding.StatusNew,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Path: fixturePkgAGo},
			To:   finding.Endpoint{Path: fixturePkgBGo},
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
	want := []string{fixturePkgAGo, fixturePkgBGo}
	if len(tasks[0].Files) != len(want) || tasks[0].Files[0] != want[0] || tasks[0].Files[1] != want[1] {
		t.Errorf("files = %v, want %v (candidates pass through unchanged)", tasks[0].Files, want)
	}
}

// TestNewPathResolver_SharedDirectoryAncestorDedup pins the ancestor-dedup
// break in NewPathResolver's knownDirs build: two files under the same
// directory make the second file's walk hit an already-recorded ancestor and
// break early. Both files and the shared directory must still resolve.
func TestNewPathResolver_SharedDirectoryAncestorDedup(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, fixturePkgAGo)
	writeFixtureFile(t, root, fixturePkgBGo)
	knownFiles := buildKnownFiles(t, root)

	resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
	f := finding.Finding{
		ID:        "f1",
		Kind:      finding.KindGate,
		RuleID:    ruleTypeForbidden,
		Status:    finding.StatusNew,
		Locations: []relationship.Location{{File: fixturePkgAGo}, {File: fixturePkgBGo}, {File: fixturePkgDir}},
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
	want := []string{fixturePkgDir, fixturePkgAGo, fixturePkgBGo}
	if len(tasks[0].Files) != len(want) {
		t.Fatalf("files = %v, want %v", tasks[0].Files, want)
	}
	for i, w := range want {
		if tasks[0].Files[i] != w {
			t.Errorf("files = %v, want %v", tasks[0].Files, want)
		}
	}
}

func TestFilesFor_PerLanguageResolution(t *testing.T) {
	t.Run("go_module_key_resolves_to_root_dir_listing", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "widget/foo.go")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
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

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
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

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
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

	t.Run("python_dotted_module_resolves_under_src_layout", func(t *testing.T) {
		// pythonFileToModuleKey strips a leading "src/" before dotting a file
		// into a module key, so the forward direction (module -> candidate
		// files) must offer the "src/"-prefixed form back, or every src-layout
		// repo (ccgram, prefect) drops legitimate resolutions.
		root := t.TempDir()
		writeFixtureFile(t, root, "src/myapp/domain.py")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
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
		if want := "src/myapp/domain.py"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (probed via src/myapp/domain.py)", tasks[0].Files, want)
		}
	})

	t.Run("rust_crate_mod_key_resolves_via_crate_root", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "crates/mycrate/src/lib.rs")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, map[string]string{fixtureCrate: fixtureCrateDir}, nil, nil)
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
		if want := fixtureCrateDir; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
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

		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
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
		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
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

		resolver := agenttask.NewPathResolver(knownFiles, nil, map[string]string{"ghostmod": "legacy/pkg"}, nil)
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

// TestFilesFor_RustCrateResolution covers the crate::mod shapes beyond the
// basic dir fallback: module-file precision when the file is indexed, and the
// root-crate layout (CrateRoot.Dir "" — a single-crate repo, the common case)
// which previously could never resolve at all.
func TestFilesFor_RustCrateResolution(t *testing.T) {
	build := func(root string, crateDirs map[string]string, modKey string) []result.AgentTask {
		knownFiles := buildKnownFiles(t, root)
		resolver := agenttask.NewPathResolver(knownFiles, crateDirs, nil, nil)
		return agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge(modKey)},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
	}

	t.Run("module_file_beats_crate_dir_when_indexed", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "crates/mycrate/src/mymod.rs")
		tasks := build(root, map[string]string{fixtureCrate: fixtureCrateDir}, "mycrate::mymod")
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "crates/mycrate/src/mymod.rs"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (the module's own file, not just the crate dir)", tasks[0].Files, want)
		}
	})

	t.Run("mod_rs_beats_crate_dir_when_flat_file_absent", func(t *testing.T) {
		// Only the mod.rs shape exists (no flat mymod.rs) — the second candidate
		// in the resolve loop (internal/agenttask/agenttask.go:143-149).
		root := t.TempDir()
		writeFixtureFile(t, root, "crates/mycrate/src/mymod/mod.rs")
		tasks := build(root, map[string]string{fixtureCrate: fixtureCrateDir}, "mycrate::mymod")
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "crates/mycrate/src/mymod/mod.rs"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (mod.rs shape, not just the crate dir)", tasks[0].Files, want)
		}
	})

	t.Run("root_crate_resolves_module_file_under_src", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "src/mymod.rs")
		tasks := build(root, map[string]string{fixtureCrate: ""}, "mycrate::mymod")
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "src/mymod.rs"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (root crate: Dir \"\" must still resolve)", tasks[0].Files, want)
		}
	})

	t.Run("root_crate_falls_back_to_src_dir", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "src/lib.rs")
		tasks := build(root, map[string]string{fixtureCrate: ""}, "mycrate::other")
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "src"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (root crate src dir, never the repo root or a drop)", tasks[0].Files, want)
		}
	})
}

// TestFilesFor_OnDiskFallback pins the "exists on disk" contract: the LOC
// walk's index skips directories (mocks/, target/, venv/) the extractor
// exclusions do not, so a real file there is index-invisible. The onDisk
// callback must rescue it; without the callback it is dropped.
func TestFilesFor_OnDiskFallback(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, root, fixtureRealGo)
	writeFixtureFile(t, root, "mocks/fake.go")
	// Simulate the LOC walk: mocks/ is skipped, so only pkg/real.go is indexed.
	knownFiles := map[string]struct{}{fixtureRealGo: {}}
	onDisk := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}

	f := finding.Finding{
		ID:     "f1",
		Kind:   finding.KindGate,
		RuleID: ruleTypeForbidden,
		Status: finding.StatusNew,
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Path: "mocks/fake.go"},
			To:   finding.Endpoint{Path: fixtureRealGo},
		},
	}

	t.Run("with_callback_the_on_disk_file_survives", func(t *testing.T) {
		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, onDisk)
		tasks := agenttask.Build([]finding.Finding{f}, map[string]string{ruleTypeForbidden: ruleTypeForbidden}, nil, nil, nil, resolver)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if len(tasks[0].Files) != 2 {
			t.Errorf("files = %v, want both endpoints (mocks/fake.go exists on disk)", tasks[0].Files)
		}
	})

	t.Run("without_callback_the_index_miss_is_dropped", func(t *testing.T) {
		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
		tasks := agenttask.Build([]finding.Finding{f}, map[string]string{ruleTypeForbidden: ruleTypeForbidden}, nil, nil, nil, resolver)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		if want := fixtureRealGo; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] only (index miss, no disk check available)", tasks[0].Files, want)
		}
	})
}

// TestFilesFor_DottedModuleRootFallback pins the Python shape of the module
// root fallback: policy.ModuleRootDirs returns a dotted module-ID prefix
// ("myapp.domain") for a dotted glob, and filesFor must route it through the
// full resolver — the dotted-candidate probe — rather than a bare dir check
// that a dotted string can never pass.
func TestFilesFor_DottedModuleRootFallback(t *testing.T) {
	t.Run("dotted_root_resolves_via_file_probe", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "myapp/domain.py")
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, map[string]string{"domain": "myapp.domain"}, nil)
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("domain")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "myapp/domain.py"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (dotted module root resolved to its real file)", tasks[0].Files, want)
		}
	})

	t.Run("dotted_root_resolves_to_package_dir_without_init", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "myapp/domain/handlers.py") // namespace package: no __init__.py
		knownFiles := buildKnownFiles(t, root)

		resolver := agenttask.NewPathResolver(knownFiles, nil, map[string]string{"domain": "myapp.domain"}, nil)
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("domain")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "myapp/domain"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (dots-to-slashes package dir)", tasks[0].Files, want)
		}
	})
}

// TestFilesFor_ModuleKeyCollisionSkipped pins the false-evidence guard: once a
// finding's Locations resolved to real files, the module-key edge endpoints
// are skipped — a module named "docs" owning src/domain/** must not drag an
// unrelated real docs/ directory into files[]. When no Location resolves, the
// key remains the best-effort probe.
func TestFilesFor_ModuleKeyCollisionSkipped(t *testing.T) {
	t.Run("resolved_location_suppresses_colliding_module_key", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "src/domain/api.go")
		writeFixtureFile(t, root, "docs/readme.md") // unrelated dir named like the module key
		knownFiles := buildKnownFiles(t, root)

		f := gateFindingWithModuleEdge("docs")
		f.Locations = []relationship.Location{{File: "src/domain/api.go"}}
		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
		tasks := agenttask.Build(
			[]finding.Finding{f},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "src/domain/api.go"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] only (colliding module key must not leak in)", tasks[0].Files, want)
		}
	})

	t.Run("unresolvable_locations_keep_module_key_probe", func(t *testing.T) {
		root := t.TempDir()
		writeFixtureFile(t, root, "widget/foo.go")
		knownFiles := buildKnownFiles(t, root)

		f := gateFindingWithModuleEdge("widget")
		f.Locations = []relationship.Location{{File: "ghost/nope.go"}}
		resolver := agenttask.NewPathResolver(knownFiles, nil, nil, nil)
		tasks := agenttask.Build(
			[]finding.Finding{f},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		assertFilesExistOnDisk(t, root, tasks[0].Files)
		if want := "widget"; len(tasks[0].Files) != 1 || tasks[0].Files[0] != want {
			t.Errorf("files = %v, want [%q] (no resolved location — the key probe must still rescue)", tasks[0].Files, want)
		}
	})
}

// TestFilesFor_EscapingCandidatesDropped pins the scan-root boundary: a
// candidate that escapes upward (a module Paths glob like "../outside/**"
// feeding the policy.ModuleRootDirs fallback) or is absolute must be dropped even
// when the target really exists on disk — files[] never points outside the
// analyzed tree.
func TestFilesFor_EscapingCandidatesDropped(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	writeFixtureFile(t, root, fixtureRealGo)
	writeFixtureFile(t, base, "outside-repo/secret.txt") // sibling of the scan root
	knownFiles := buildKnownFiles(t, root)
	onDisk := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}

	cases := []struct {
		name, moduleRoot string
	}{
		{"parent_escape", "../outside-repo"},
		{"nested_parent_escape", "sub/../../outside-repo"},
		{"absolute_path", "/etc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := agenttask.NewPathResolver(knownFiles, nil, map[string]string{"leaky": tc.moduleRoot}, onDisk)
			tasks := agenttask.Build(
				[]finding.Finding{gateFindingWithModuleEdge("leaky")},
				map[string]string{rulePublicAPIMax: rulePublicAPIMax},
				nil, nil, nil,
				resolver,
			)
			if len(tasks) != 1 {
				t.Fatalf("tasks = %d, want 1", len(tasks))
			}
			if len(tasks[0].Files) != 0 {
				t.Errorf("files = %v, want empty (escaping module root must be dropped, not emitted)", tasks[0].Files)
			}
		})
	}

	// Derived crate::mod probes are built from crateRootDirs AFTER resolve's
	// raw-candidate guard ran, so the escape check inside exists is the only
	// line of defense for a ".."-bearing crate root.
	t.Run("escaping_crate_root", func(t *testing.T) {
		writeFixtureFile(t, base, "outside-repo/src/mymod.rs")
		resolver := agenttask.NewPathResolver(knownFiles, map[string]string{"leaky": "../outside-repo"}, nil, onDisk)
		tasks := agenttask.Build(
			[]finding.Finding{gateFindingWithModuleEdge("leaky::mymod")},
			map[string]string{rulePublicAPIMax: rulePublicAPIMax},
			nil, nil, nil,
			resolver,
		)
		if len(tasks) != 1 {
			t.Fatalf("tasks = %d, want 1", len(tasks))
		}
		if len(tasks[0].Files) != 0 {
			t.Errorf("files = %v, want empty (escaping crate root must be dropped, not emitted)", tasks[0].Files)
		}
	})
}
