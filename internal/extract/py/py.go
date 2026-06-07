package py

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	toolGrimp     = "grimp"
	langPython    = "python"
	statusOK      = "ok"
	statusPartial = "partial"
	statusAbsent  = "absent"
	runTimeout    = 5 * time.Minute
)

// Extractor is the Python import extractor using grimp via uv or python3.12.
// It satisfies the engine.Extractor interface structurally.
type Extractor struct {
	runner toolrun.Runner
	cfg    config.ExtractConfig
}

// New returns an Extractor configured with the given runner and config.
func New(runner toolrun.Runner, cfg config.ExtractConfig) *Extractor {
	return &Extractor{runner: runner, cfg: cfg}
}

// Name returns the language identifier for this extractor.
func (e *Extractor) Name() string {
	return langPython
}

// Extract detects uv or python3.12+grimp, writes the embedded helper to a temp
// file, runs it against the project root, parses the JSON output, and returns
// graph.Facts + diagnostic.Coverage.
//
// If mode is off, Extract returns empty Facts and an "absent" Coverage immediately.
// If mode is auto and no applicable tool or Python project is found,
// Extract returns empty Facts and an "absent" Coverage — never an error.
// If mode is on and the tool is absent, Extract returns an error.
func (e *Extractor) Extract(ctx context.Context, s scope.Scope) (graph.Facts, diagnostic.Coverage, error) {
	if e.cfg.Mode == config.ModeOff {
		return graph.Facts{}, absentCoverage(), nil
	}

	// Applicability: requires pyproject.toml, setup.py, or cfg.PyPackage directory.
	if !e.isApplicable(s.Root) {
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: no Python project marker found at %s", s.Root)
		}
		return graph.Facts{}, absentCoverage(), nil
	}

	// Detect uv (preferred) or python3.12.
	tool, version, found := e.detectTool(ctx)
	if !found {
		if e.cfg.Mode == config.ModeOn {
			return graph.Facts{}, diagnostic.Coverage{}, errors.New("extract/py: uv or python3.12 not found; install uv (https://docs.astral.sh/uv/) or Python 3.12")
		}
		return graph.Facts{}, absentCoverage(), nil
	}

	// Write embedded helper to a temp file.
	tmp, err := os.CreateTemp("", "grimp_helper_*.py")
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: create temp helper: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck

	if _, err := tmp.Write(grimpHelperSrc); err != nil {
		_ = tmp.Close()
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: write temp helper: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: close temp helper: %w", err)
	}

	pkgName := e.cfg.PyPackage
	if pkgName == "" {
		pkgName = filepath.Base(s.Root)
	}

	// Build the command.
	var cmd toolrun.ToolCmd
	if tool == "uv" {
		cmd = toolrun.ToolCmd{
			Name:    "uv",
			Args:    []string{"run", "--directory", s.Root, tmpName, "--package", pkgName},
			WorkDir: s.Root,
			Timeout: runTimeout,
		}
	} else {
		cmd = toolrun.ToolCmd{
			Name:    tool,
			Args:    []string{tmpName, "--package", pkgName, "--root", s.Root},
			WorkDir: s.Root,
			Timeout: runTimeout,
		}
	}

	out, err := e.runner.Run(ctx, cmd)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: run helper: %w", err)
	}
	if out.ExitCode != 0 {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: helper exited %d: %s", out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}

	facts, cov, err := e.parseAndNormalize(out.Stdout, version)
	if err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("extract/py: parse output: %w", err)
	}
	return facts, cov, nil
}

// isApplicable returns true if s.Root contains a Python project marker.
func (e *Extractor) isApplicable(root string) bool {
	for _, marker := range []string{"pyproject.toml", "setup.py"} {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			return true
		}
	}
	if e.cfg.PyPackage != "" {
		if _, err := os.Stat(filepath.Join(root, e.cfg.PyPackage)); err == nil {
			return true
		}
	}
	return false
}

// detectTool tries uv then python3.12. Returns (name, version, true) on success.
func (e *Extractor) detectTool(ctx context.Context) (string, string, bool) {
	if info, ok := e.runner.Detect(ctx, "uv"); ok {
		ver := e.toolVersion(ctx, info.Name, []string{"--version"})
		return "uv", ver, true
	}
	if info, ok := e.runner.Detect(ctx, "python3.12"); ok {
		ver := e.toolVersion(ctx, info.Name, []string{"--version"})
		return "python3.12", ver, true
	}
	return "", "", false
}

// toolVersion runs tool with versionArgs and returns the trimmed stdout.
// Returns empty string on any failure (non-fatal).
func (e *Extractor) toolVersion(ctx context.Context, tool string, args []string) string {
	out, err := e.runner.Run(ctx, toolrun.ToolCmd{
		Name:    tool,
		Args:    args,
		Timeout: 30 * time.Second,
	})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
}

// ---------------------------------------------------------------------------
// JSON parsing types for grimp_helper output.
// ---------------------------------------------------------------------------

type helperOutput struct {
	Edges      []helperEdge `json:"edges"`
	Unresolved int          `json:"unresolved"`
	Error      string       `json:"error,omitempty"`
}

type helperEdge struct {
	Importer string `json:"importer"`
	Imported string `json:"imported"`
	Line     int    `json:"line"`
}

// ---------------------------------------------------------------------------
// Parse + normalise.
// ---------------------------------------------------------------------------

// parseAndNormalize parses grimp_helper JSON and builds graph.Facts.
func (e *Extractor) parseAndNormalize(data []byte, version string) (graph.Facts, diagnostic.Coverage, error) {
	var h helperOutput
	if err := json.Unmarshal(data, &h); err != nil {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("unmarshal: %w", err)
	}
	if h.Error != "" {
		return graph.Facts{}, diagnostic.Coverage{}, fmt.Errorf("grimp: %s", h.Error)
	}

	var nodes []graph.Node
	var edges []graph.Edge
	seenNodes := make(map[string]struct{})

	emitNode := func(dotted string) {
		id := "module:" + dotted
		if _, ok := seenNodes[id]; !ok {
			seenNodes[id] = struct{}{}
			nodes = append(nodes, graph.Node{Kind: graph.NodeKindModule, Path: dotted})
		}
	}

	for _, he := range h.Edges {
		emitNode(he.Importer)
		emitNode(he.Imported)

		edgeKind := graph.EdgeKindImports
		if e.matchesInternal(he.Imported) {
			edgeKind = graph.EdgeKindUsesInternal
		}

		// Location file: dotted module name converted to path (dots → slashes).
		locFile := strings.ReplaceAll(he.Importer, ".", "/")

		edges = append(edges, graph.Edge{
			From:       "module:" + he.Importer,
			To:         "module:" + he.Imported,
			Kind:       edgeKind,
			Language:   langPython,
			Confidence: "high",
			Locations: []graph.Location{
				{File: locFile, Line: he.Line},
			},
		})
	}

	filesSeen := len(seenNodes)
	covStatus := statusOK
	if h.Unresolved > 0 {
		covStatus = statusPartial
	}
	cov := diagnostic.Coverage{
		Tool:            toolGrimp,
		Version:         version,
		FilesSeen:       filesSeen,
		FilesApplicable: filesSeen,
		Unresolved:      h.Unresolved,
		Status:          covStatus,
	}
	facts := graph.Facts{
		Nodes:      nodes,
		Edges:      edges,
		Language:   langPython,
		Unresolved: h.Unresolved,
	}
	return facts, cov, nil
}

// matchesInternal reports whether the dotted module name matches any internal glob.
func (e *Extractor) matchesInternal(dotted string) bool {
	for _, pattern := range e.cfg.Internal {
		if matched, _ := doublestar.Match(pattern, dotted); matched {
			return true
		}
	}
	return false
}

// absentCoverage returns a Coverage record indicating the tool was not found.
func absentCoverage() diagnostic.Coverage {
	return diagnostic.Coverage{
		Tool:   toolGrimp,
		Status: statusAbsent,
	}
}
