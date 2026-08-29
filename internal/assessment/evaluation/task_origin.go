package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

// TaskOriginInput is the flat, adapter-facing input to `--base` origin
// classification. It is flat on purpose: the stage adapter already holds every
// value, and assessment keeps the nested comparison contract private.
type TaskOriginInput struct {
	BaseRef        string
	BaseFindingIDs []string

	HeadCoverage   []evidence.Coverage
	HeadGaps       []evidence.CoverageGap
	HeadConfigHash string

	BaseCoverage   []evidence.Coverage
	BaseGaps       []evidence.CoverageGap
	BaseConfigHash string

	// PrimaryTools are the per-language dependency-graph analyzers this build
	// registers; the flags name the opt-in analyzers the config activated.
	PrimaryTools                                 []string
	Patterns, Syntax, SCIP, Clones, CargoModules bool
}

// AttachTaskOrigins classifies current repair tasks by origin. The classification
// is report-only: it never changes the verdict or exit code.
func AttachTaskOrigins(diag *result.Result, in TaskOriginInput) {
	delta := decision.ClassifyTaskOrigins(decision.TaskOriginEvidence{
		BaseRef:        in.BaseRef,
		Tasks:          diag.AgentTasks,
		BaseFindingIDs: in.BaseFindingIDs,
		Head:           decision.AnalyzerEvidence{Coverage: in.HeadCoverage, Gaps: in.HeadGaps, Hash: in.HeadConfigHash},
		Base:           decision.AnalyzerEvidence{Coverage: in.BaseCoverage, Gaps: in.BaseGaps, Hash: in.BaseConfigHash},
		Families: decision.AnalyzerFamilies(decision.FamilyOptions{
			PrimaryTools: in.PrimaryTools, Patterns: in.Patterns, Syntax: in.Syntax,
			SCIP: in.SCIP, Clones: in.Clones, CargoModules: in.CargoModules,
		}),
	})

	originByID := make(map[string]result.TaskOrigin, len(diag.AgentTasks))
	for _, id := range delta.IntroducedFindingIDs {
		originByID[id] = result.TaskOriginIntroduced
	}
	for _, id := range delta.PreExistingFindingIDs {
		originByID[id] = result.TaskOriginPreExisting
	}
	for _, id := range delta.UnknownOriginFindingIDs {
		originByID[id] = result.TaskOriginUnknown
	}
	for i := range diag.AgentTasks {
		diag.AgentTasks[i].Origin = originByID[diag.AgentTasks[i].FindingID]
	}

	if diag.Comparison == nil {
		diag.Comparison = &result.StateComparison{BaseRef: in.BaseRef, Reasons: []string{}}
	}
	diag.Comparison.TaskOriginStatus = delta.ComparisonStatus
	diag.Comparison.TaskOriginReasons = append([]string{}, delta.ComparisonReasons...)
}

// BaseFindingIDs projects a base run's findings to the stable IDs that cross
// the worktree boundary. Fixed entries are dropped: a finding the base run
// reports as fixed was not observed there.
func BaseFindingIDs(findings []finding.Finding) []string {
	return decision.BaseFindingIDs(findings)
}
