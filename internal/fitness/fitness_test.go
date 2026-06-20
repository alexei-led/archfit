package fitness_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/fitness"
)

// writeFile creates a file at path (relative to dir) with the given content,
// creating any necessary parent directories.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", full, err)
	}
}

// containsPath reports whether paths contains target.
func containsPath(paths []string, target string) bool {
	return slices.Contains(paths, target)
}

func TestDetect_NonePresent(t *testing.T) {
	root := t.TempDir()
	// Plant a harmless Go file that is not a test and not an arch tool reference.
	writeFile(t, root, "internal/core/core.go", "package core\n")

	got := fitness.Detect(root)

	if got.ArchTestFiles {
		t.Errorf("ArchTestFiles: want false, got true; evidence=%v", got.EvidencePaths["arch_test_files"])
	}
	if got.ImportLinterConfig {
		t.Errorf("ImportLinterConfig: want false, got true; evidence=%v", got.EvidencePaths["import_linter_config"])
	}
	if got.ArchLinterInCI {
		t.Errorf("ArchLinterInCI: want false, got true; evidence=%v", got.EvidencePaths["arch_linter_in_ci"])
	}
	if len(got.EvidencePaths["arch_test_files"]) != 0 {
		t.Errorf("arch_test_files evidence: want empty, got %v", got.EvidencePaths["arch_test_files"])
	}
	if len(got.EvidencePaths["import_linter_config"]) != 0 {
		t.Errorf("import_linter_config evidence: want empty, got %v", got.EvidencePaths["import_linter_config"])
	}
	if len(got.EvidencePaths["arch_linter_in_ci"]) != 0 {
		t.Errorf("arch_linter_in_ci evidence: want empty, got %v", got.EvidencePaths["arch_linter_in_ci"])
	}
}

func TestDetect_AllPresent(t *testing.T) {
	root := t.TempDir()

	// Arch test file: name contains "arch".
	writeFile(t, root, "internal/core/arch_test.go",
		"package core_test\nimport \"testing\"\nfunc TestArch(t *testing.T) {}\n")

	// Import-linter config: standalone .importlinter file.
	writeFile(t, root, ".importlinter",
		"[importlinter]\nroot_package = myapp\n")

	// Arch-linter in CI: GitHub Actions workflow referencing archfit.
	writeFile(t, root, ".github/workflows/ci.yml",
		"name: CI\nsteps:\n  - run: archfit check\n")

	got := fitness.Detect(root)

	if !got.ArchTestFiles {
		t.Errorf("ArchTestFiles: want true, got false")
	}
	if !got.ImportLinterConfig {
		t.Errorf("ImportLinterConfig: want true, got false")
	}
	if !got.ArchLinterInCI {
		t.Errorf("ArchLinterInCI: want true, got false")
	}

	// Evidence paths must contain the created files (relative to root).
	archEvidence := got.EvidencePaths["arch_test_files"]
	if !containsPath(archEvidence, filepath.Join("internal", "core", "arch_test.go")) {
		t.Errorf("arch_test_files evidence: want internal/core/arch_test.go, got %v", archEvidence)
	}

	linterEvidence := got.EvidencePaths["import_linter_config"]
	if !containsPath(linterEvidence, ".importlinter") {
		t.Errorf("import_linter_config evidence: want .importlinter, got %v", linterEvidence)
	}

	ciEvidence := got.EvidencePaths["arch_linter_in_ci"]
	if !containsPath(ciEvidence, filepath.Join(".github", "workflows", "ci.yml")) {
		t.Errorf("arch_linter_in_ci evidence: want .github/workflows/ci.yml, got %v", ciEvidence)
	}
}

// TestDetect_Partial covers multiple partial combinations in a table.
func TestDetect_Partial(t *testing.T) {
	tests := []struct {
		name               string
		setup              func(root string, t *testing.T)
		wantArchTest       bool
		wantImportLinter   bool
		wantArchLinterInCI bool
		wantArchEvidence   []string // relative paths expected in arch_test_files
		wantLinterEvidence []string // relative paths expected in import_linter_config
		wantCIEvidence     []string // relative paths expected in arch_linter_in_ci
	}{
		{
			name: "only_arch_test_file_by_name",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, "tests/arch_dependency_test.go",
					"package tests\n")
			},
			wantArchTest:     true,
			wantArchEvidence: []string{filepath.Join("tests", "arch_dependency_test.go")},
		},
		{
			name: "only_arch_test_file_by_import",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, "tests/rules_test.go",
					"package tests\nimport \"github.com/alexei-led/archfit/check\"\n")
			},
			wantArchTest:     true,
			wantArchEvidence: []string{filepath.Join("tests", "rules_test.go")},
		},
		{
			name: "only_importlinter_standalone",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, ".importlinter", "[importlinter]\n")
			},
			wantImportLinter:   true,
			wantLinterEvidence: []string{".importlinter"},
		},
		{
			name: "only_setup_cfg_with_importlinter_section",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, "setup.cfg",
					"[metadata]\nname = myapp\n\n[importlinter]\nroot_package = myapp\n")
			},
			wantImportLinter:   true,
			wantLinterEvidence: []string{"setup.cfg"},
		},
		{
			name: "only_pyproject_toml_with_tool_importlinter",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, "pyproject.toml",
					"[tool.poetry]\nname = \"myapp\"\n\n[tool.importlinter]\nroot_package = \"myapp\"\n")
			},
			wantImportLinter:   true,
			wantLinterEvidence: []string{"pyproject.toml"},
		},
		{
			name: "only_ci_dependency_cruiser",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, ".github/workflows/arch.yml",
					"name: arch\nsteps:\n  - run: npx dependency-cruiser src\n")
			},
			wantArchLinterInCI: true,
			wantCIEvidence:     []string{filepath.Join(".github", "workflows", "arch.yml")},
		},
		{
			name: "only_ci_import_linter",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, ".github/workflows/lint.yml",
					"name: lint\nsteps:\n  - run: lint-imports\n    env:\n      TOOL: import-linter\n")
			},
			wantArchLinterInCI: true,
			wantCIEvidence:     []string{filepath.Join(".github", "workflows", "lint.yml")},
		},
		{
			name: "arch_test_plus_ci_no_linter_config",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, "tests/arch_layer_test.go",
					"package tests\n")
				writeFile(t, root, ".github/workflows/ci.yml",
					"jobs:\n  check:\n    steps:\n      - run: archfit check\n")
			},
			wantArchTest:       true,
			wantArchLinterInCI: true,
			wantArchEvidence:   []string{filepath.Join("tests", "arch_layer_test.go")},
			wantCIEvidence:     []string{filepath.Join(".github", "workflows", "ci.yml")},
		},
		{
			name: "linter_config_plus_ci_no_arch_tests",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, ".importlinter", "[importlinter]\n")
				writeFile(t, root, ".github/workflows/quality.yml",
					"steps:\n  - run: deptry .\n")
			},
			wantImportLinter:   true,
			wantArchLinterInCI: true,
			wantLinterEvidence: []string{".importlinter"},
			wantCIEvidence:     []string{filepath.Join(".github", "workflows", "quality.yml")},
		},
		{
			name: "plain_test_file_no_arch_keywords",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, "internal/core/core_test.go",
					"package core\nimport \"testing\"\nfunc TestFoo(t *testing.T){}\n")
			},
			wantArchTest: false,
		},
		{
			name: "setup_cfg_without_importlinter_section",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, "setup.cfg", "[metadata]\nname = myapp\n")
			},
			wantImportLinter: false,
		},
		{
			name: "ci_workflow_no_arch_tools",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, ".github/workflows/build.yml",
					"steps:\n  - run: go build ./...\n")
			},
			wantArchLinterInCI: false,
		},
		{
			name: "ci_goda_reference",
			setup: func(root string, t *testing.T) {
				writeFile(t, root, ".github/workflows/graph.yml",
					"steps:\n  - run: goda graph ./...\n")
			},
			wantArchLinterInCI: true,
			wantCIEvidence:     []string{filepath.Join(".github", "workflows", "graph.yml")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(root, t)

			got := fitness.Detect(root)

			if got.ArchTestFiles != tc.wantArchTest {
				t.Errorf("ArchTestFiles: want %v, got %v; evidence=%v",
					tc.wantArchTest, got.ArchTestFiles, got.EvidencePaths["arch_test_files"])
			}
			if got.ImportLinterConfig != tc.wantImportLinter {
				t.Errorf("ImportLinterConfig: want %v, got %v; evidence=%v",
					tc.wantImportLinter, got.ImportLinterConfig, got.EvidencePaths["import_linter_config"])
			}
			if got.ArchLinterInCI != tc.wantArchLinterInCI {
				t.Errorf("ArchLinterInCI: want %v, got %v; evidence=%v",
					tc.wantArchLinterInCI, got.ArchLinterInCI, got.EvidencePaths["arch_linter_in_ci"])
			}

			// Evidence path assertions.
			for _, want := range tc.wantArchEvidence {
				if !containsPath(got.EvidencePaths["arch_test_files"], want) {
					t.Errorf("arch_test_files: want %q in evidence, got %v", want, got.EvidencePaths["arch_test_files"])
				}
			}
			for _, want := range tc.wantLinterEvidence {
				if !containsPath(got.EvidencePaths["import_linter_config"], want) {
					t.Errorf("import_linter_config: want %q in evidence, got %v", want, got.EvidencePaths["import_linter_config"])
				}
			}
			for _, want := range tc.wantCIEvidence {
				if !containsPath(got.EvidencePaths["arch_linter_in_ci"], want) {
					t.Errorf("arch_linter_in_ci: want %q in evidence, got %v", want, got.EvidencePaths["arch_linter_in_ci"])
				}
			}
		})
	}
}

func TestDetect_EmptyRoot(t *testing.T) {
	root := t.TempDir()

	got := fitness.Detect(root)

	if got.ArchTestFiles || got.ImportLinterConfig || got.ArchLinterInCI {
		t.Errorf("empty root: all signals must be false, got %+v", got)
	}
}

func TestDetect_VendorDirIgnored(t *testing.T) {
	root := t.TempDir()
	// Arch test file inside vendor — must be skipped.
	writeFile(t, root, "vendor/somelib/arch_test.go",
		"package somelib\n")
	// Import-linter config inside vendor — must be skipped.
	writeFile(t, root, "vendor/tool/.importlinter", "[importlinter]\n")

	got := fitness.Detect(root)

	if got.ArchTestFiles {
		t.Errorf("ArchTestFiles: vendor arch test must be ignored, got evidence=%v",
			got.EvidencePaths["arch_test_files"])
	}
	if got.ImportLinterConfig {
		t.Errorf("ImportLinterConfig: vendor .importlinter must be ignored, got evidence=%v",
			got.EvidencePaths["import_linter_config"])
	}
}

func TestDetect_ModuleCacheAndTestdataIgnored(t *testing.T) {
	root := t.TempDir()
	// Go module cache under <root>/pkg/mod: a third-party arch-named test file
	// (golang.org/x/tools archive_test.go — "archive" matches "arch") must not be
	// counted as a first-party architecture test.
	writeFile(t, root, "pkg/mod/golang.org/x/tools@v0.1.0/archive_test.go",
		"package archive\nimport \"testing\"\nfunc TestArch(t *testing.T) {}\n")
	// testdata fixtures hold synthetic test files that are not real arch tests.
	writeFile(t, root, "internal/foo/testdata/arch_test.go",
		"package foo\n")

	got := fitness.Detect(root)

	if got.ArchTestFiles {
		t.Errorf("ArchTestFiles: module-cache/testdata files must be ignored, got evidence=%v",
			got.EvidencePaths["arch_test_files"])
	}
}

func TestDetect_PkgDirNotModCache(t *testing.T) {
	root := t.TempDir()
	// A real pkg/ source dir (not the module cache) must still be scanned: only
	// the exact <root>/pkg/mod subtree is skipped, not every "pkg" directory.
	writeFile(t, root, "pkg/core/arch_test.go", "package core\n")

	got := fitness.Detect(root)

	if !got.ArchTestFiles {
		t.Errorf("ArchTestFiles: pkg/core/arch_test.go should be detected, got evidence=%v",
			got.EvidencePaths["arch_test_files"])
	}
}

func TestDetect_MultipleWorkflowFiles(t *testing.T) {
	root := t.TempDir()
	// Two workflow files: one with arch tool, one without.
	writeFile(t, root, ".github/workflows/build.yml",
		"steps:\n  - run: go build ./...\n")
	writeFile(t, root, ".github/workflows/arch.yml",
		"steps:\n  - run: archfit check\n")

	got := fitness.Detect(root)

	if !got.ArchLinterInCI {
		t.Errorf("ArchLinterInCI: want true, got false")
	}
	ciEvidence := got.EvidencePaths["arch_linter_in_ci"]
	if !containsPath(ciEvidence, filepath.Join(".github", "workflows", "arch.yml")) {
		t.Errorf("expected arch.yml in CI evidence, got %v", ciEvidence)
	}
	if containsPath(ciEvidence, filepath.Join(".github", "workflows", "build.yml")) {
		t.Errorf("build.yml must not be in CI evidence (no arch tool), got %v", ciEvidence)
	}
}
