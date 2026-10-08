package evaluation

import (
	"github.com/alexei-led/archfit/v3/internal/assessment/decision"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
)

// OriginInput is the flat, adapter-facing input to `--base` origin
// classification. It is flat on purpose: the stage adapter already holds every
// value, and assessment keeps the nested comparison contract private.
type OriginInput struct {
	BaseRef      string
	BaseFindings []decision.BaseFinding
	BaseSeams    []decision.ModulePair

	HeadCoverage []evidence.Coverage
	HeadGaps     []evidence.CoverageGap
	HeadProfile  *evidence.MeasurementProfile

	BaseCoverage []evidence.Coverage
	BaseGaps     []evidence.CoverageGap
	BaseProfile  *evidence.MeasurementProfile

	// PrimaryTools are the per-language dependency-graph analyzers this build
	// registers; the flags name the opt-in analyzers the config activated.
	PrimaryTools                                 []string
	Patterns, Syntax, SCIP, Clones, CargoModules bool
}

// AttachOrigins classifies every current finding by origin, copies each
// finding's origin onto its agent task, and records the introduced and
// resolved finding IDs on the comparison. The classification is report-only:
// it never changes the verdict or exit code.
func AttachOrigins(diag *result.Result, in OriginInput) {
	delta := decision.ClassifyOrigins(decision.OriginEvidence{
		Findings:     diag.Findings,
		BaseFindings: in.BaseFindings,
		BaseSeams:    in.BaseSeams,
		Head:         decision.AnalyzerEvidence{Coverage: in.HeadCoverage, Gaps: in.HeadGaps, Profile: in.HeadProfile},
		Base:         decision.AnalyzerEvidence{Coverage: in.BaseCoverage, Gaps: in.BaseGaps, Profile: in.BaseProfile},
		Families: decision.AnalyzerFamilies(decision.FamilyOptions{
			PrimaryTools: in.PrimaryTools, Patterns: in.Patterns, Syntax: in.Syntax,
			SCIP: in.SCIP, Clones: in.Clones, CargoModules: in.CargoModules,
		}),
	})

	for i := range diag.Findings {
		diag.Findings[i].Origin = delta.Origins[diag.Findings[i].ID]
	}
	for i := range diag.AgentTasks {
		diag.AgentTasks[i].Origin = delta.Origins[diag.AgentTasks[i].FindingID]
	}

	if diag.Comparison == nil {
		diag.Comparison = &result.StateComparison{BaseRef: in.BaseRef, Reasons: []string{}}
	}
	diag.Comparison.OriginStatus = delta.Status
	diag.Comparison.OriginReasons = delta.Reasons
	diag.Comparison.IntroducedFindingIDs = delta.Introduced
	diag.Comparison.ResolvedFindingIDs = delta.Resolved
}

// BaseSeams projects a base run's qualifying distributed-monolith seams to the
// module pairs that identify them.
func BaseSeams(seams []result.Seam) []decision.ModulePair {
	out := []decision.ModulePair{}
	for _, s := range seams {
		if s.DistributedMonolith {
			out = append(out, decision.ModulePair{From: s.FromModule, To: s.ToModule})
		}
	}
	return out
}
