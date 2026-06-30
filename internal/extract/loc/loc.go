// Package loc counts source-file line counts for FileLOC facts (used by SCIP
// file-facts and the coverage metric). It walks the repository root, skips
// test files, generated/dependency directories, and dot-dirs, and returns a
// repo-relative slash-normalised map of path → line count together with a
// diagnostic coverage record.
//
// Run also produces a FileClassIndex — a parallel map[string]FileClass covering
// ALL source files (including test and generated) so downstream metrics can
// filter correctly without re-reading files. The LOC map excludes test/generated
// files (unchanged behaviour); the FileClassIndex includes them.
package loc

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/syntax"
)

const toolName = "loc"

// headerReadSize is the number of bytes read from each file to detect the
// generated-code header marker. 512 bytes covers the longest standard header
// across Go, Python, and TypeScript generators.
const headerReadSize = 512

// sourceExts are the file extensions counted for FileLOC facts.
var sourceExts = map[string]bool{
	".go": true, ".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".rs": true,
}

const langTypeScript = "typescript"

// extLang maps source file extensions to the language key used by the
// classifier so IsTestFile and ClassifyFile get the right language.
var extLang = map[string]string{
	".go":  "go",
	".py":  "python",
	".ts":  langTypeScript,
	".tsx": langTypeScript,
	".js":  langTypeScript, // close enough for test/generated detection
	".jsx": langTypeScript,
	".rs":  "rust",
}

// skipDirs are directory names never walked for source LOC. "target" is Cargo's
// build-output directory (the Rust analogue of node_modules/dist).
var skipDirs = map[string]bool{
	"node_modules": true, "dist": true, "vendor": true, "venv": true,
	".venv": true, "build": true, "testdata": true, "mocks": true, "target": true,
}

// Run walks root and returns:
//   - locMap: repo-relative path→LOC (production files only; test/generated excluded)
//   - classes: repo-relative path→FileClass for ALL visited source files
//   - cov: coverage record for the loc tool
//   - err: always nil (walk is best-effort)
//
// The walk is best-effort: unreadable files contribute nothing.
// FilesSeen and FilesApplicable equal the locMap length; status is always ok
// because the walk is a pure filesystem operation with no external tool.
func Run(root string) (locMap map[string]int, classes map[string]fileclass.FileClass, cov diagnostic.Coverage, err error) {
	return RunWithConfig(root, syntax.FileClassConfig{})
}

// RunWithConfig is like Run but accepts a FileClassConfig for config-driven
// pattern overrides (custom mock frameworks, project-specific generated globs).
// The pipeline calls this variant; Run is a convenience wrapper for callers
// that have no config (tests, doctor, etc.).
func RunWithConfig(root string, cfg syntax.FileClassConfig) (map[string]int, map[string]fileclass.FileClass, diagnostic.Coverage, error) {
	locMap := make(map[string]int)
	classes := make(map[string]fileclass.FileClass)

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return filepath.SkipDir
			}
			// Skip the Go module cache (<root>/pkg/mod): third-party deps, not
			// first-party source. Mirrors fitness.skipModuleCacheAndTestdata; the
			// exact rel-path match never collides with pkg/model or pkg/modules.
			if rel, relErr := filepath.Rel(root, path); relErr == nil && rel == filepath.Join("pkg", "mod") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(name)
		if !sourceExts[ext] {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		lang := extLang[ext]
		header := readHeader(path)
		fc := syntax.ClassifyFile(lang, rel, header, cfg)
		classes[rel] = fc

		// LOC map: only production files (test/generated excluded).
		if fc == fileclass.Production {
			if n := countLines(path); n > 0 {
				locMap[rel] = n
			}
		}
		return nil
	})

	cov := diagnostic.Coverage{
		Tool:            toolName,
		FilesSeen:       len(locMap),
		FilesApplicable: len(locMap),
		Status:          diagnostic.StatusOK,
	}
	return locMap, classes, cov, nil
}

// readHeader reads the first headerReadSize bytes of a file for the generated-
// code marker sniff. Returns nil on any error (header sniff is then skipped).
func readHeader(path string) []byte {
	f, err := os.Open(path) // #nosec G304 -- path comes from a directory walk of the project
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, headerReadSize)
	n, _ := f.Read(buf)
	if n == 0 {
		return nil
	}
	return buf[:n]
}

// countLines returns the number of lines in a file, or 0 on error.
func countLines(path string) int {
	f, err := os.Open(path) // #nosec G304 -- path comes from a directory walk of the project
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		n++
	}
	return n
}
