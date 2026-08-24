package main

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/scan"
)

// emitHealthWarnings writes actionable hints to stderr when the pipeline
// result looks suspicious. Each warning includes a next-command suggestion.
// cfg must be the same config used for the analysis run (already loaded; no
// second disk read). root and configPath are used only in the command hints.
func emitHealthWarnings(deps *appDeps, diag scan.Diagnostic, cfg config.Config, root, configPath string) {
	if deps == nil {
		return
	}
	if len(diag.CoverageGaps) > 0 {
		deps.warn("analyzer coverage gap — some edges may be unscored\n  → run: archfit doctor --fix")
	}

	if edges := diag.ClassifiedEdges; edges != nil {
		if edges.Total > 0 && edges.Scored == 0 {
			deps.warn(fmt.Sprintf("0 of %d edges scored — coupling strength is unknown\n  → run: archfit config update -c %q", edges.Total, configPath))
		}
		if edges.Scored == 0 && edges.Abstained > 0 && edges.External == 0 {
			deps.warn(fmt.Sprintf("all %d cross-module edges have unknown strength\n  → run: archfit config enrich abstained -c %q", edges.Abstained, configPath))
		}
		if pythonAllEdgesExternal(diag.ToolCoverage, edges) {
			deps.warn(fmt.Sprintf("no internal edges found — module paths may not match source layout\n  → run: archfit config update -c %q", configPath))
		}
	}

	// Use the already-matched FileFacts from the pipeline run rather than
	// re-walking the source tree. Empty FileFacts with declared module paths
	// means no source files matched any module glob.
	if len(diag.FileFacts) == 0 && declaresModulePaths(cfg) {
		deps.warn(fmt.Sprintf("no source files matched declared module paths — check --root and module globs\n  → run: archfit check --root %q -c %q", root, configPath))
	}
}

func pythonAllEdgesExternal(cov []diagnostic.Coverage, edges *diagnostic.ClassifiedEdgeSummary) bool {
	if edges == nil || edges.External == 0 || edges.Scored != 0 {
		return false
	}
	if !coverageStatusIs(cov, toolGrimp, diagnostic.StatusOK) {
		return false
	}
	crossModule := edges.Total - edges.SameModule
	return crossModule == edges.External
}

func coverageStatusIs(cov []diagnostic.Coverage, tool, status string) bool {
	for _, c := range cov {
		if c.Tool == tool && c.Status == status {
			return true
		}
	}
	return false
}

func declaresModulePaths(cfg config.Config) bool {
	for _, def := range cfg.Modules {
		if len(def.Paths) > 0 {
			return true
		}
	}
	return false
}
