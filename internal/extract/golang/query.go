package golang

import (
	"fmt"
	"go/build"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/mod/modfile"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// QueryEdge builds the facts `archfit policy can-import` judges: one import
// of target by the Go file from, spelled exactly as the extractor spells it —
// a file node to a ScanRoot-relative package node, with the same edge kind and
// the loaded members — so the rule pass keys its findings as it does on the
// extracted edge. target is a ScanRoot-relative package dir or an import path;
// an import path under a member module is stripped to its dir, any other is
// kept (an external package).
//
// It wraps evidenceports.ErrNotExtracted where the extractor emits no edge: a
// _test.go file (the load does not include tests), a file the host build
// context excludes, a file of no loaded member, or an excluded file or target.
// It reads go.mod files and the importing file's build constraints; it runs no
// Go tool.
func QueryEdge(scanRoot string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
	if !strings.HasSuffix(from, ".go") {
		return graph.Facts{}, fmt.Errorf("go: %q is not a .go file", from)
	}
	entries, err := memberModules(scanRoot, cfg)
	if err != nil {
		return graph.Facts{}, err
	}
	to := stripModulePath(entries, strings.TrimSuffix(target, "/"))
	if reason := notExtracted(scanRoot, cfg, entries, from, to); reason != "" {
		return graph.Facts{}, fmt.Errorf("go: %s: %w", reason, evidenceports.ErrNotExtracted)
	}
	modules := make([]graph.GoModule, 0, len(entries))
	for _, m := range entries {
		modules = append(modules, graph.GoModule{Path: m.path, RelDir: m.relDir})
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Path < modules[j].Path })
	return graph.Facts{
		Language:  graph.LangGo,
		GoModules: modules,
		Nodes: []graph.Node{
			{Kind: graph.NodeKindFile, Path: from, Language: graph.LangGo},
			{Kind: graph.NodeKindPackage, Path: to, Language: graph.LangGo},
		},
		Edges: []graph.Edge{{
			From:       graph.Node{Kind: graph.NodeKindFile, Path: from}.ID(),
			To:         graph.Node{Kind: graph.NodeKindPackage, Path: to}.ID(),
			Kind:       ImportEdgeKind(to),
			Language:   graph.LangGo,
			Confidence: "high",
			Locations:  []graph.Location{{File: from}},
		}},
	}, nil
}

// notExtracted names why the extractor would emit no edge from from to the
// stripped target to, or "" when it would.
func notExtracted(scanRoot string, cfg evidenceports.ExtractConfig, entries []modEntry, from, to string) string {
	switch {
	case strings.HasSuffix(from, "_test.go"):
		return from + " is a test file, which the load does not read"
	case excluded(cfg.Exclusions, from):
		return from + " matches an exclude: glob"
	case excluded(cfg.Exclusions, to):
		return to + " matches an exclude: glob"
	case !inLoadedMember(scanRoot, entries, from):
		return from + " belongs to no loaded Go module"
	case !buildMatches(scanRoot, cfg.BuildFlags, from):
		return from + " is excluded by the host build constraints"
	}
	return ""
}

// excluded reports whether an exclusion glob matches p. It is the extractor's
// one exclusion test, for files and stripped import paths alike.
func excluded(exclusions []string, p string) bool {
	for _, pattern := range exclusions {
		if matched, _ := doublestar.Match(pattern, p); matched {
			return true
		}
	}
	return false
}

// inLoadedMember reports whether the nearest go.mod above from is a loaded
// member: a file of a nested module the run does not load yields no edge.
func inLoadedMember(scanRoot string, entries []modEntry, from string) bool {
	for dir := path.Dir(from); ; dir = path.Dir(dir) {
		if _, err := os.Stat(filepath.Join(scanRoot, filepath.FromSlash(dir), "go.mod")); err == nil {
			for _, m := range entries {
				if m.relDir == dir {
					return true
				}
			}
			return false
		}
		if dir == "." || dir == "/" {
			return false
		}
	}
}

// buildMatches reports whether the go toolchain the run starts includes from:
// the host GOOS/GOARCH (or their environment overrides), the run's -tags and
// any -tags in GOFLAGS, and the CGO_ENABLED it inherits. With CGO_ENABLED
// unset, cgo depends on a C compiler this binary cannot see, so a file that
// either setting includes counts as included. A file that does not exist yet
// is judged by its name.
func buildMatches(scanRoot string, buildFlags []string, from string) bool {
	tags := append(buildTags(buildFlags), buildTags(strings.Fields(os.Getenv("GOFLAGS")))...)
	cgo := []bool{true, false}
	switch os.Getenv("CGO_ENABLED") {
	case "1":
		cgo = []bool{true}
	case "0":
		cgo = []bool{false}
	}
	for _, enabled := range cgo {
		ctxt := build.Default
		ctxt.CgoEnabled = enabled
		ctxt.BuildTags = append(slices.Clone(ctxt.BuildTags), tags...)
		ctxt.OpenFile = func(p string) (io.ReadCloser, error) {
			f, err := os.Open(p) // #nosec G304 -- the queried source file under the scan root
			if os.IsNotExist(err) {
				return io.NopCloser(strings.NewReader("package p\n")), nil
			}
			return f, err
		}
		dir := filepath.Join(scanRoot, filepath.FromSlash(path.Dir(from)))
		if match, err := ctxt.MatchFile(dir, path.Base(from)); err != nil || match {
			return true
		}
	}
	return false
}

// buildTags reads the tags of a -tags flag in a list of go build flags.
func buildTags(flags []string) []string {
	for i, flag := range flags {
		value, ok := strings.CutPrefix(flag, "-tags=")
		if !ok && flag == "-tags" && i+1 < len(flags) {
			value, ok = flags[i+1], true
		}
		if ok {
			return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
		}
	}
	return nil
}

// memberModules reads the module path of every member the extractor loads,
// with its ScanRoot-relative dir, sorted for stripModulePath.
func memberModules(scanRoot string, cfg evidenceports.ExtractConfig) ([]modEntry, error) {
	members, err := AnalysableMembers(scanRoot, cfg.Exclusions, cfg.GoModuleInclude, cfg.GoModuleExclude)
	if err != nil {
		return nil, err
	}
	root, err := canonicalDir(scanRoot)
	if err != nil {
		return nil, err
	}
	var entries []modEntry
	for _, dir := range members.Dirs {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod")) // #nosec G304 -- member dir comes from go.mod discovery
		if err != nil {
			continue
		}
		modPath := modfile.ModulePath(data)
		member, err := canonicalDir(dir)
		if modPath == "" || err != nil {
			continue
		}
		rel, err := filepath.Rel(root, member)
		if err != nil {
			continue
		}
		entries = append(entries, modEntry{path: modPath, relDir: filepath.ToSlash(rel)})
	}
	sortModEntries(entries)
	return entries, nil
}

// canonicalDir resolves dir to an absolute path without symlinks, so a member
// found under a symlinked spelling of the scan root still gets its relative dir.
func canonicalDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
