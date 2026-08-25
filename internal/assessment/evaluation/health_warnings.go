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
func healthWarnings(diag result.Result, gaps []evidence.CoverageGap, modules map[string]policy.ModuleDef, scanRoot, configPath string) []string {
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

	// Use the already-matched FileFacts from the pipeline run rather than
	// re-walking the source tree. Empty FileFacts with declared module paths
	// means no source files matched any module glob.
	if len(diag.FileFacts) == 0 && declaresModulePaths(modules) {
		warn(fmt.Sprintf("no source files matched declared module paths — check --root and module globs\n  → run: archfit check --root %q -c %q", scanRoot, configPath))
	}
	return out
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
