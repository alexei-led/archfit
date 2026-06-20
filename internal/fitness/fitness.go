// Package fitness detects architecture-intent ENFORCEMENT signals in a repository.
//
// Detect performs a deterministic filesystem scan and returns a signal.Signals struct
// describing which enforcement mechanisms are present. It reads only the
// filesystem under root; no network access, no subprocesses.
package fitness

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/model/signal"
)

// category constants used as keys in signal.EvidenceMap.
const (
	catArchTestFiles  = "arch_test_files"
	catImportLinter   = "import_linter_config"
	catArchLinterInCI = "arch_linter_in_ci"
)

// archTestImports is the set of import path prefixes that indicate an
// architecture-test library is in use.
var archTestImports = []string{
	"github.com/alexei-led/archfit",
	"archfit",
	"import-linter",
	"importlinter",
	"github.com/mibrahimali/archtest",
	"github.com/fdaines/arch-go",
}

// ciArchTools is the set of tool names that indicate architecture enforcement
// when referenced in a CI workflow file.
var ciArchTools = []string{
	"archfit",
	"import-linter",
	"importlinter",
	"deptry",
	"dependency-cruiser",
	"depcruise",
	"goda",
}

// Detect scans the directory tree rooted at root for architecture-intent
// enforcement signals and returns a populated signal.Signals struct.
//
// Absent or unreadable files are silently skipped. Detect never returns an
// error; missing signals are represented as false booleans and empty evidence.
func Detect(root string) signal.Signals {
	evidence := signal.EvidenceMap{
		catArchTestFiles:  nil,
		catImportLinter:   nil,
		catArchLinterInCI: nil,
	}

	detectArchTestFiles(root, evidence)
	detectImportLinterConfig(root, evidence)
	detectArchLinterInCI(root, evidence)

	return signal.Signals{
		ArchTestFiles:      len(evidence[catArchTestFiles]) > 0,
		ImportLinterConfig: len(evidence[catImportLinter]) > 0,
		ArchLinterInCI:     len(evidence[catArchLinterInCI]) > 0,
		EvidencePaths:      evidence,
	}
}

// detectArchTestFiles walks the tree looking for test files whose names suggest
// architecture rules, or whose content imports a known arch-test library.
func detectArchTestFiles(root string, evidence signal.EvidenceMap) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if skip := skipVendorAndHidden(d); skip != nil {
				return skip
			}
			return skipModuleCacheAndTestdata(root, path, d)
		}
		name := d.Name()
		if !isTestFile(name) {
			return nil
		}
		if archTestFileName(name) || fileContainsArchTestImport(path) {
			rel := relOrAbs(root, path)
			evidence[catArchTestFiles] = append(evidence[catArchTestFiles], rel)
		}
		return nil
	})
}

// isTestFile reports whether name looks like a test source file.
func isTestFile(name string) bool {
	switch {
	case strings.HasSuffix(name, "_test.go"):
		return true
	case strings.HasSuffix(name, "_test.py"):
		return true
	case strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py"):
		return true
	case strings.HasSuffix(name, ".test.ts"), strings.HasSuffix(name, ".spec.ts"):
		return true
	case strings.HasSuffix(name, ".test.js"), strings.HasSuffix(name, ".spec.js"):
		return true
	default:
		return false
	}
}

// archTestFileName reports whether the file name contains keywords that suggest
// it encodes architecture rules.
func archTestFileName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "arch") ||
		strings.Contains(lower, "import_cycle") ||
		strings.Contains(lower, "importcycle") ||
		strings.Contains(lower, "import-cycle") ||
		strings.Contains(lower, "layer") ||
		strings.Contains(lower, "dependency") ||
		strings.Contains(lower, "coupling")
}

// fileContainsArchTestImport reports whether any line in the file contains an
// import path from a known architecture-test library.
func fileContainsArchTestImport(path string) bool {
	f, err := os.Open(path) //nolint:gosec // intentional: scanning repo files under a scanned root
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for _, imp := range archTestImports {
			if strings.Contains(line, imp) {
				return true
			}
		}
	}
	return false
}

// detectImportLinterConfig checks for the three standard import-linter
// configuration locations.
func detectImportLinterConfig(root string, evidence signal.EvidenceMap) {
	// 1. Standalone .importlinter file anywhere in the tree.
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return skipVendorAndHidden(d)
		}
		if d.Name() == ".importlinter" {
			rel := relOrAbs(root, path)
			evidence[catImportLinter] = append(evidence[catImportLinter], rel)
			return nil
		}
		// 2. setup.cfg with [importlinter] section.
		if d.Name() == "setup.cfg" && fileContainsSection(path, "[importlinter]") {
			rel := relOrAbs(root, path)
			evidence[catImportLinter] = append(evidence[catImportLinter], rel)
			return nil
		}
		// 3. pyproject.toml with [tool.importlinter] section.
		if d.Name() == "pyproject.toml" && fileContainsSection(path, "[tool.importlinter]") {
			rel := relOrAbs(root, path)
			evidence[catImportLinter] = append(evidence[catImportLinter], rel)
		}
		return nil
	})
}

// detectArchLinterInCI scans .github/workflows/*.yml and *.yaml for references
// to known architecture-linting tools.
func detectArchLinterInCI(root string, evidence signal.EvidenceMap) {
	workflowDirs := []string{
		filepath.Join(root, ".github", "workflows"),
		filepath.Join(root, ".gitlab-ci.yml"),
		filepath.Join(root, ".circleci"),
	}

	// Primary: .github/workflows directory
	wfDir := workflowDirs[0]
	entries, err := os.ReadDir(wfDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
				continue
			}
			path := filepath.Join(wfDir, name)
			if fileContainsCITool(path) {
				rel := relOrAbs(root, path)
				evidence[catArchLinterInCI] = append(evidence[catArchLinterInCI], rel)
			}
		}
	}

	// Secondary: .gitlab-ci.yml at root
	if fileContainsCITool(workflowDirs[1]) {
		rel := relOrAbs(root, workflowDirs[1])
		evidence[catArchLinterInCI] = append(evidence[catArchLinterInCI], rel)
	}

	// Secondary: .circleci/config.yml
	circleConfig := filepath.Join(workflowDirs[2], "config.yml")
	if fileContainsCITool(circleConfig) {
		rel := relOrAbs(root, circleConfig)
		evidence[catArchLinterInCI] = append(evidence[catArchLinterInCI], rel)
	}
}

// fileContainsCITool reports whether any line in the file references a known
// architecture-linting tool.
func fileContainsCITool(path string) bool {
	f, err := os.Open(path) //nolint:gosec // intentional: scanning repo files under a scanned root
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for _, tool := range ciArchTools {
			if strings.Contains(line, tool) {
				return true
			}
		}
	}
	return false
}

// fileContainsSection reports whether the file contains a line that equals
// (after trimming whitespace) the given section header.
func fileContainsSection(path, section string) bool {
	f, err := os.Open(path) //nolint:gosec // intentional: scanning repo files under a scanned root
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == section {
			return true
		}
	}
	return false
}

// skipVendorAndHidden returns fs.SkipDir for vendor and hidden directories,
// and nil otherwise, to be used as the walk error handler for directories.
func skipVendorAndHidden(d fs.DirEntry) error {
	if d != nil && d.IsDir() {
		name := d.Name()
		if name == "vendor" || name == "node_modules" || (len(name) > 1 && name[0] == '.') {
			return fs.SkipDir
		}
	}
	return nil
}

// skipModuleCacheAndTestdata returns fs.SkipDir for the Go module cache
// (<root>/pkg/mod) and any testdata directory. Both hold third-party or fixture
// _test.go files (e.g. golang.org/x/tools archive_test.go) that must not be
// counted as first-party architecture tests.
func skipModuleCacheAndTestdata(root, path string, d fs.DirEntry) error {
	if d == nil || !d.IsDir() {
		return nil
	}
	if d.Name() == "testdata" {
		return fs.SkipDir
	}
	if rel, err := filepath.Rel(root, path); err == nil && rel == filepath.Join("pkg", "mod") {
		return fs.SkipDir
	}
	return nil
}

// relOrAbs returns path relative to root, or path itself if the relative
// computation fails.
func relOrAbs(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
