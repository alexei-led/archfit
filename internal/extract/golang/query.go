package golang

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"golang.org/x/mod/modfile"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
)

// QueryEdge builds the facts `archfit policy can-import` judges: one import
// of target by the Go file from, spelled exactly as the extractor spells it —
// a file node to a ScanRoot-relative package node, with the same edge kind and
// the loaded members — so the rule pass keys its findings as it does on the
// extracted edge. target is a ScanRoot-relative package dir or an import path;
// an import path under a member module is placed on its dir, any other is
// kept as given (an external package, or a dir).
//
// It reads files and places the target with the decisions ParseImports makes
// (fileReason, buildSelects, placeImport), so it answers for the files the
// check reads. It wraps evidenceports.ErrNotExtracted where the extractor
// emits no edge: a file the go command never reads (a _test.go file, a file
// under testdata, vendor or a "." or "_" dir, a file of no loaded member), a
// file the build context excludes, or an excluded file or target. It wraps
// evidenceports.ErrNotDecidable when whether the load reads from depends on
// cgo and CGO_ENABLED is not set. It reads go.mod files and the importing
// file's build constraints; it runs no Go tool.
func QueryEdge(scanRoot string, cfg evidenceports.ExtractConfig, from, target string) (graph.Facts, error) {
	if !strings.HasSuffix(from, ".go") {
		return graph.Facts{}, fmt.Errorf("go: %q is not a .go file", from)
	}
	entries, err := memberModules(scanRoot, cfg)
	if err != nil {
		return graph.Facts{}, err
	}
	target = strings.TrimSuffix(target, "/")
	to, firstParty := placeImport(entries, target)
	if !firstParty {
		to = target
	}
	reason := fileReason(scanRoot, cfg.Exclusions, entries, from)
	if reason == "" && excluded(cfg.Exclusions, to) {
		reason = to + " matches an exclude: glob"
	}
	if reason != "" {
		return graph.Facts{}, fmt.Errorf("go: %s: %w", reason, evidenceports.ErrNotExtracted)
	}
	switch sel, reason, _ := buildSelects(scanRoot, cfg.BuildFlags, from, importsC(filepath.Join(scanRoot, filepath.FromSlash(from)))); sel {
	case selectNo:
		return graph.Facts{}, fmt.Errorf("go: %s: %w", reason, evidenceports.ErrNotExtracted)
	case selectUndecided:
		return graph.Facts{}, fmt.Errorf("go: %s: %w", reason, evidenceports.ErrNotDecidable)
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

// importsC reports whether the Go file at p imports "C". A file that does not
// exist yet or does not parse imports nothing.
func importsC(p string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
	if err != nil {
		return false
	}
	for _, imp := range file.Imports {
		if imp.Path.Value == `"C"` {
			return true
		}
	}
	return false
}

// goEnv reads a go environment variable as the go command does: the process
// environment, else the go env file (GOENV, default <user config>/go/env).
func goEnv(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	file := os.Getenv("GOENV")
	if file == "off" {
		return ""
	}
	if file == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		file = filepath.Join(dir, "go", "env")
	}
	data, err := os.ReadFile(file) //nolint:gosec // the go env file the go command reads
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), key+"="); ok {
			return value
		}
	}
	return ""
}

// memberModules reads the module path of every member the extractor loads,
// with its ScanRoot-relative dir, sorted by sortModEntries for placeImport.
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
