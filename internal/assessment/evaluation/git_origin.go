package evaluation

import (
	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/model/evidence"
)

// GitOriginInput is the flat, adapter-facing input to `--base` origin
// classification. It is flat on purpose: the stage adapter already holds every
// value, and assessment keeps the nested comparison contract private.
type GitOriginInput struct {
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

// AttachGitOrigin classifies the current repair tasks by origin and attaches
// the report-only block. It never changes the verdict or the exit code.
func AttachGitOrigin(diag *result.Result, in GitOriginInput) {
	diag.GitFindingDelta = decision.BuildGitFindingDelta(decision.GitDeltaInput{
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
}

// BaseFindingIDs projects a base run's findings to the stable IDs that cross
// the worktree boundary. Fixed entries are dropped: a finding the base run
// reports as fixed was not observed there.
func BaseFindingIDs(findings []finding.Finding) []string {
	return decision.BaseFindingIDs(findings)
}
