package evaluation

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/policy"
)

// toolGrimp is the Python primary analyzer's coverage name. Assessment is core
// ring: it names the tool, it never imports the extractor adapter that runs it.
const toolGrimp = "grimp"

// healthWarnings returns actionable hints, in emission order, when the assessed
// result looks suspicious. Each warning includes a next-command suggestion. They
// are disclosure only: no warning here moves a metric, a finding, or the verdict.
// scanRoot and configPath are used only in the command hints, and scanRoot is the
// caller's --root form so a copy-pasted hint reproduces the run.
//
// gaps is passed explicitly rather than read off diag: Score stamps
// diag.CoverageGaps AFTER Assess computes these warnings, so reading the field
// here would silently disable the coverage-gap hint.
func healthWarnings(diag result.Result, gaps []evidence.CoverageGap, topology policy.TopologyView, fileLOC map[string]int, scanRoot, configPath string) []string {
	var out []string
	warn := func(msg string) { out = append(out, msg) }
	if len(gaps) > 0 {
		warn("analyzer coverage gap — some edges may be unscored\n  → run: archfit doctor --fix")
	}

	if edges := diag.ClassifiedEdges; edges != nil {
		if edges.Total > 0 && edges.Scored == 0 {
			warn(fmt.Sprintf("0 of %d edges scored — coupling strength is unknown\n  → run: archfit config update -c %q", edges.Total, configPath))
		}
		if edges.Scored == 0 && edges.Abstained > 0 && edges.External == 0 {
			warn(fmt.Sprintf("all %d cross-module edges have unknown strength\n  → run: archfit config enrich abstained -c %q", edges.Abstained, configPath))
		}
		if pythonAllEdgesExternal(diag.ToolCoverage, edges) {
			warn(fmt.Sprintf("no internal edges found — module paths may not match source layout\n  → run: archfit config update -c %q", configPath))
		}
	}

	// Use the LOC walk's own file set rather than re-walking the source tree.
	//
	// This deliberately does NOT read diag.FileFacts: those are built from the
	// SCIP symbol graph, which is opt-in and off by default, so an empty
	// FileFacts means "no symbol indexer ran", not "no file matched a module
	// glob". Warning on it told every default-configured repository that its
	// module paths were wrong.
	if !anyFileMatchesAModule(fileLOC, topology.ModuleMap) && declaresModulePaths(topology.Modules) {
		// Reuse the repair-task command builder: it omits --root when the caller
		// gave none (empty ScanRoot means "the whole repository"), so the hint
		// never suggests `--root ""`.
		warn("no source files matched declared module paths — check --root and module globs\n  → run: " + validationCommand(configPath, scanRoot))
	}
	return out
}

// anyFileMatchesAModule reports whether at least one walked source file
// resolves to a declared module. An empty walk answers false: nothing matched,
// which is the condition the warning describes.
func anyFileMatchesAModule(fileLOC map[string]int, mm policy.ModuleMap) bool {
	for file := range fileLOC {
		if _, ok := mm.ModuleForFile(file); ok {
			return true
		}
	}
	return false
}

func pythonAllEdgesExternal(cov []evidence.Coverage, edges *result.ClassifiedEdgeSummary) bool {
	if edges == nil || edges.External == 0 || edges.Scored != 0 {
		return false
	}
	if !coverageStatusIs(cov, toolGrimp, evidence.StatusOK) {
		return false
	}
	crossModule := edges.Total - edges.SameModule
	return crossModule == edges.External
}

func coverageStatusIs(cov []evidence.Coverage, tool, status string) bool {
	for _, c := range cov {
		if c.Tool == tool && c.Status == status {
			return true
		}
	}
	return false
}

func declaresModulePaths(modules map[string]policy.ModuleDef) bool {
	for _, def := range modules {
		if len(def.Paths) > 0 {
			return true
		}
	}
	return false
}
