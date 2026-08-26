package acquisition

import (
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/acquire"
	"github.com/alexei-led/archfit/internal/extract/registry"
	"github.com/alexei-led/archfit/internal/model/pattern"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
)

// Analyzer coverage names produced outside the language registry. The registry
// owns the primary per-language tool names (registry.ToolGoPackages and peers);
// these are the auxiliary passes acquisition itself reports on.
const (
	toolLoc           = "loc"
	toolAstGrep       = "ast-grep"
	toolAstGrepSyntax = "ast-grep/syntax"
	toolDeployUnit    = "deploy-unit"
	toolScip          = "scip"
	toolScipSymbols   = "scip-symbols"
	toolJscpd         = "jscpd"

	metricCycle         = "cycle"
	metricBlastRadius   = "blast_radius"
	metricEncapsulation = "encapsulation"
)

// CoverageOptions is the narrow analyzer activation input built by the config
// adapter. ProjectPresent holds each language's own applicability probe — the
// same function its extractor calls — so a probe can never disagree with the
// extractor that produced the coverage row.
type CoverageOptions struct {
	Gates          map[string]string
	Modes          map[string]string
	ProjectPresent map[string]func(string) bool
}

// RunOptions is the narrow acquisition input built by the config adapter.
// Policy declarations are deliberately absent; they travel only in the
// policy snapshot.
type RunOptions struct {
	Exclusions  []string
	Scope       scope.Config
	Extractors  registry.Configs
	Acquisition acquire.Options
	Syntax      evidenceports.SyntaxConfig
	Patterns    pattern.Config
	// Lint reports the config-quality warnings for a module set. It is a
	// function, not a precomputed slice, because it must run against the run's
	// RESOLVED modules: a module whose owner CODEOWNERS filled is not missing an
	// owner, and a warning frozen at wiring time contradicts the owner_source
	// reported in the same document.
	Lint     func(map[string]policy.ModuleDef) []string
	Coverage CoverageOptions
}
