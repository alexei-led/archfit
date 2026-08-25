package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const languagesDocsURL = "https://github.com/alexei-led/archfit/blob/main/docs/guide/languages.md"

// Tool and metric constants used by the concrete Analyze/Check stage.
const (
	toolLoc           = "loc"
	toolAstGrep       = "ast-grep"
	toolAstGrepSyntax = "ast-grep/syntax"
	toolDeployUnit    = "deploy-unit"
	toolScip          = "scip"
	toolScipSymbols   = "scip-symbols"
	toolJscpd         = "jscpd"

	toolGoPackages   = registry.ToolGoPackages
	toolDepCruiser   = registry.ToolDepCruiser
	toolGrimp        = registry.ToolGrimp
	toolCargo        = registry.ToolCargo
	toolCargoModules = registry.ToolCargoModules

	metricCycle         = "cycle"
	metricBlastRadius   = "blast_radius"
	metricEncapsulation = "encapsulation"

	volatilityLow        = "low"
	volatilityMedium     = "medium"
	volatilityHigh       = "high"
	volatilityFrozen     = "frozen"
	volatilityLegacy     = "legacy"
	volatilityUndeclared = "undeclared"

	subdomainCore       = "core"
	subdomainSupporting = "supporting"
	subdomainGeneric    = "generic"
)

// LabelLoader is the application port for the optional approved-label file.
type LabelLoader interface {
	Load(path string) ([]labels.Label, error)
}

// Deps are the concrete process and IO adapters used by the analysis stage.
// They intentionally contain no cmd package types.
type Deps struct {
	Runner      toolrun.Runner
	LabelLoader LabelLoader
	LabelsPath  string
	Stderr      io.Writer

	Progress     func(stage string)
	WarnLabel    string
	Refresh      bool
	ResolvedRoot string
}

func (d *Deps) loadLabels(bundleDir string) ([]labels.Label, error) {
	if d == nil || d.LabelLoader == nil {
		return nil, nil
	}
	path := d.LabelsPath
	if path == "" {
		path = filepath.Join(bundleDir, ".archfit-labels.yaml")
	}
	return d.LabelLoader.Load(path)
}

func (d *Deps) warn(w string) {
	_, _ = fmt.Fprintln(d.stderr(), "warning: "+d.WarnLabel+w)
}

func (d *Deps) stderr() io.Writer {
	if d != nil && d.Stderr != nil {
		return d.Stderr
	}
	return os.Stderr
}

func (d *Deps) reportPhase(stage string) {
	if d != nil && d.Progress != nil {
		d.Progress(stage)
	}
}

// factsCacheDir returns the extractor fact-cache directory under baseDir.
func factsCacheDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "facts")
}

// baseWorktreesDir returns the --base worktree parent directory under baseDir.
func baseWorktreesDir(baseDir string) string {
	return filepath.Join(baseDir, ".archfit-cache", "worktrees")
}

// ExecutionError reports a controlled pipeline failure. The CLI owns process
// status translation for this error.
type ExecutionError struct{ Message string }

func (e *ExecutionError) Error() string { return e.Message }
