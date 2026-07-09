package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/loc"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

func emitHealthWarnings(deps *appDeps, diag diagnostic.Diagnostic, configPath string, refresh bool) {
	if deps == nil {
		return
	}
	if len(diag.CoverageGaps) > 0 {
		deps.warn("analyzer coverage gap — some edges may be unscored\n  → run: archfit doctor --fix")
	}

	if edges := diag.ClassifiedEdges; edges != nil {
		if edges.Total > 0 && edges.Scored == 0 {
			deps.warn(fmt.Sprintf("0 of %d edges scored — coupling strength is unknown\n  → run: archfit config update -c %s", edges.Total, configPath))
		}
		if edges.Scored == 0 && edges.Abstained > 0 && edges.External == 0 {
			deps.warn(fmt.Sprintf("all %d cross-module edges have unknown strength\n  → run: archfit config enrich abstained -c %s", edges.Abstained, configPath))
		}
		if pythonAllEdgesExternal(diag.ToolCoverage, edges) {
			deps.warn("no internal edges found — module paths may not match source layout\n  → run: archfit config update -c " + configPath)
		}
	}

	if noModuleSourceFilesMatched(deps.scanRoot, configPath) {
		deps.warn("no source files matched declared module paths — check --root and module globs\n  → run: archfit check --root . -c " + configPath)
	}

	// TODO: Use refresh + fact-cache metadata to warn when cached partial tool
	// output may hide a newly installed or updated analyzer. That metadata is not
	// plumbed through the diagnostic yet, so refresh is intentionally unused here.
	_ = refresh
}

func pythonAllEdgesExternal(cov []diagnostic.Coverage, edges *diagnostic.ClassifiedEdgeSummary) bool {
	if edges == nil || edges.External == 0 || edges.Scored != 0 {
		return false
	}
	if !coverageStatusIs(cov, toolGrimp, diagnostic.StatusOK) {
		return false
	}
	return crossModuleEdgeTotal(edges) == edges.External
}

func crossModuleEdgeTotal(edges *diagnostic.ClassifiedEdgeSummary) int {
	if edges == nil {
		return 0
	}
	return edges.Total - edges.SameModule
}

func coverageStatusIs(cov []diagnostic.Coverage, tool, status string) bool {
	for _, c := range cov {
		if c.Tool == tool && c.Status == status {
			return true
		}
	}
	return false
}

func noModuleSourceFilesMatched(scanRoot, configPath string) bool {
	cfg, err := loadConfig(context.Background(), configPath)
	if err != nil || !declaresModulePaths(cfg) {
		return false
	}
	if scanRoot == "" {
		scanRoot = filepath.Dir(configPath)
	}
	_, classes, _, err := loc.RunWithConfig(scanRoot, cfg.ForFileClass())
	if err != nil {
		return false
	}
	mm := cfg.ModuleMapView()
	for file := range classes {
		if _, ok := mm.ModuleForFile(file); ok {
			return false
		}
	}
	return true
}

func declaresModulePaths(cfg config.Config) bool {
	for _, def := range cfg.Modules {
		if len(def.Paths) > 0 {
			return true
		}
	}
	return false
}
