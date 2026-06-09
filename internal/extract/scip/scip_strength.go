package scip

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	statusOK      = "ok"
	statusPartial = "partial"
	statusAbsent  = "absent"
	indexTimeout  = 10 * time.Minute
	readerTimeout = 5 * time.Minute
	pyInitFile    = "__init__.py"
	pySrcDir      = "src"

	indexerPython = "scip-python"
	indexerGo     = "scip-go"
	indexerTS     = "scip-typescript"
	flagOutput    = "--output"
)

// readerOutput mirrors the JSON emitted by scip_reader.py.
type readerOutput struct {
	Edges []struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Strength string `json:"strength"`
	} `json:"edges"`
	Error string `json:"error,omitempty"`
}

// Strengths runs a SCIP indexer over the project, reads the index, and returns
// symbol-level integration strength per cross-module edge, keyed by
// "<fromModulePath>\x00<toModulePath>". A missing toolchain (no indexer, no uv) or
// any non-fatal failure yields an empty map with an absent/partial coverage record,
// never an error — strength enrichment is best-effort on top of the import graph.
func (a *Adapter) Strengths(ctx context.Context, s scope.Scope) (map[string]string, diagnostic.Coverage, error) {
	absent := diagnostic.Coverage{Tool: toolName, Status: statusAbsent}

	indexer, pkg, lang, ok := a.detectIndexer(ctx, s.Root)
	if !ok {
		return nil, absent, nil
	}
	// The reader runs via uv (PEP 723 inline deps: protobuf + grpcio-tools).
	if _, found := a.runner.Detect(ctx, "uv"); !found {
		return nil, absent, nil
	}

	tmp, err := os.MkdirTemp("", "archfit-scip-")
	if err != nil {
		return nil, absent, nil
	}
	defer os.RemoveAll(tmp) //nolint:errcheck

	protoPath := filepath.Join(tmp, "scip.proto")
	readerPath := filepath.Join(tmp, "scip_reader.py")
	indexPath := filepath.Join(tmp, "index.scip")
	if os.WriteFile(protoPath, scipProtoSrc, 0o600) != nil ||
		os.WriteFile(readerPath, scipReaderSrc, 0o600) != nil {
		return nil, absent, nil
	}

	partial := diagnostic.Coverage{Tool: toolName, Version: indexer, Status: statusPartial}

	// Index the project (the indexer runs in the project root, output to temp).
	idxOut, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    indexer,
		Args:    indexArgs(indexer, pkg, s.Root, indexPath),
		WorkDir: s.Root,
		Timeout: indexTimeout,
	})
	if err != nil || idxOut.ExitCode != 0 {
		return nil, partial, nil
	}
	if _, statErr := os.Stat(indexPath); statErr != nil {
		return nil, partial, nil
	}

	// Read: uv run scip_reader.py --proto <p> --index <i> --package <pkg> --lang <lang>
	rdOut, err := a.runner.Run(ctx, toolrun.ToolCmd{
		Name:    "uv",
		Args:    []string{"run", readerPath, "--proto", protoPath, "--index", indexPath, "--package", pkg, "--lang", lang},
		WorkDir: tmp,
		Timeout: readerTimeout,
	})
	if err != nil || rdOut.ExitCode != 0 {
		return nil, partial, nil
	}

	m, perr := parseReaderEdges(rdOut.Stdout)
	if perr != nil {
		return nil, partial, nil
	}
	return m, diagnostic.Coverage{
		Tool:            toolName,
		Version:         indexer,
		FilesSeen:       len(m),
		FilesApplicable: len(m),
		Status:          statusOK,
	}, nil
}

// parseReaderEdges parses scip_reader.py JSON into a strength map keyed by
// "<fromModulePath>\x00<toModulePath>" — the same key the engine builds from a
// graph edge's stripped from/to paths. A helper-reported error fails the parse.
func parseReaderEdges(stdout []byte) (map[string]string, error) {
	var ro readerOutput
	if err := json.Unmarshal(stdout, &ro); err != nil {
		return nil, err
	}
	if ro.Error != "" {
		return nil, errReader(ro.Error)
	}
	m := make(map[string]string, len(ro.Edges))
	for _, e := range ro.Edges {
		m[e.From+"\x00"+e.To] = e.Strength
	}
	return m, nil
}

// errReader is a helper error type carrying the reader's reported message.
type errReader string

func (e errReader) Error() string { return "scip reader: " + string(e) }

// detectIndexer picks a SCIP indexer for the project's language and the root
// package/module name used for internal-symbol filtering and the reader --lang.
// One reader handles all three (the .scip format is language-agnostic); only the
// indexer binary and the root identifier differ.
func (a *Adapter) detectIndexer(ctx context.Context, root string) (indexer, pkg, lang string, ok bool) {
	if fileExists(filepath.Join(root, "pyproject.toml")) || fileExists(filepath.Join(root, "setup.py")) {
		if _, found := a.runner.Detect(ctx, indexerPython); found {
			if p := detectPyPackage(root); p != "" {
				return indexerPython, p, "python", true
			}
		}
	}
	if fileExists(filepath.Join(root, "go.mod")) {
		if _, found := a.runner.Detect(ctx, indexerGo); found {
			if m := goModulePath(root); m != "" {
				return indexerGo, m, "go", true
			}
		}
	}
	if fileExists(filepath.Join(root, "package.json")) {
		if _, found := a.runner.Detect(ctx, indexerTS); found {
			if n := npmPackageName(root); n != "" {
				return indexerTS, n, "typescript", true
			}
		}
	}
	return "", "", "", false
}

// indexArgs returns the per-indexer command arguments to write an index to out.
func indexArgs(indexer, pkg, root, out string) []string {
	switch indexer {
	case indexerGo:
		// scip-go runs in the module root and auto-detects the module.
		return []string{flagOutput, out}
	case indexerTS:
		return []string{"index", flagOutput, out}
	default: // scip-python
		return []string{"index", "--project-name", pkg, "--cwd", root, flagOutput, out, "--quiet"}
	}
}

// goModulePath reads the module path from go.mod ("module github.com/x/y" → that path).
func goModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod")) //nolint:gosec
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, "module "); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// npmPackageName reads the "name" field from package.json.
func npmPackageName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json")) //nolint:gosec
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	return pkg.Name
}

// detectPyPackage returns the first top-level Python package directory name found
// at root or root/src (matching initcfg's discovery).
func detectPyPackage(root string) string {
	for _, dir := range []string{root, filepath.Join(root, pySrcDir)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
				continue
			}
			if fileExists(filepath.Join(dir, e.Name(), pyInitFile)) {
				return e.Name()
			}
		}
	}
	return ""
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
