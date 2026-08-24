package diagnostic

import (
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/model/scan"
)

// Coverage is retained as a compatibility alias for evidence.Coverage.
type Coverage = evidence.Coverage

// DeprecatedDep is retained as a compatibility alias for evidence.DeprecatedDep.
type DeprecatedDep = evidence.DeprecatedDep

// DynamicImport is retained as a compatibility alias for evidence.DynamicImport.
type DynamicImport = evidence.DynamicImport

// DynamicImportSite is retained as a compatibility alias for evidence.DynamicImportSite.
type DynamicImportSite = evidence.DynamicImportSite

// SyntaxFact is retained as a compatibility alias for evidence.SyntaxFact.
type SyntaxFact = evidence.SyntaxFact

// SemanticStrengthOverlay is retained as a compatibility alias.
type SemanticStrengthOverlay = evidence.SemanticStrengthOverlay

// SemanticStrengthOverlayStats is retained as a compatibility alias.
type SemanticStrengthOverlayStats = evidence.SemanticStrengthOverlayStats

// FileFact is retained as a compatibility alias for evidence.FileFact.
type FileFact = evidence.FileFact

// DynamicConnascenceSignals is retained as a compatibility alias.
type DynamicConnascenceSignals = evidence.DynamicConnascenceSignals

// DynamicConnascenceSignal is retained as a compatibility alias.
type DynamicConnascenceSignal = evidence.DynamicConnascenceSignal

// DynamicConnascenceSite is retained as a compatibility alias.
type DynamicConnascenceSite = evidence.DynamicConnascenceSite

// RuntimeAsyncSite is retained as a compatibility alias.
type RuntimeAsyncSite = evidence.RuntimeAsyncSite

// RuntimeAsyncModule is retained as a compatibility alias.
type RuntimeAsyncModule = evidence.RuntimeAsyncModule

// RuntimeAsyncEdge is retained as a compatibility alias.
type RuntimeAsyncEdge = evidence.RuntimeAsyncEdge

// CoverageGap is retained as a compatibility alias.
type CoverageGap = evidence.CoverageGap

// DistanceConfigCandidate is retained as a compatibility alias.
type DistanceConfigCandidate = evidence.DistanceConfigCandidate

// DistanceConfigEvidenceSite is retained as a compatibility alias.
type DistanceConfigEvidenceSite = evidence.DistanceConfigEvidenceSite

// VolatilityCorroboration is retained as a compatibility alias.
type VolatilityCorroboration = evidence.VolatilityCorroboration

// VolatilityTouch is retained as a compatibility alias.
type VolatilityTouch = evidence.VolatilityTouch

// ConnascenceRoadmapItem is retained as a compatibility alias.
type ConnascenceRoadmapItem = evidence.ConnascenceRoadmapItem

// ConnascenceReport is retained as a compatibility alias.
type ConnascenceReport = evidence.ConnascenceReport

// DistanceContext is retained as a compatibility alias.
type DistanceContext = evidence.DistanceContext

// LocalCouplingModule is retained as a compatibility alias.
type LocalCouplingModule = evidence.LocalCouplingModule

// LocalCouplingEdge is retained as a compatibility alias.
type LocalCouplingEdge = evidence.LocalCouplingEdge

// SortSyntaxFacts preserves the old diagnostic package entry point.
func SortSyntaxFacts(facts []SyntaxFact) { evidence.SortSyntaxFacts(facts) }

// Verdict is retained as a compatibility alias for report.Verdict.
type Verdict = report.Verdict

// Direction is retained as a compatibility alias for report.Direction.
type Direction = report.Direction

// MetricResult is retained as a compatibility alias for report.MetricResult.
type MetricResult = report.MetricResult

// MetricSnapshot is retained as a compatibility alias for report.MetricSnapshot.
type MetricSnapshot = report.MetricSnapshot

// Summary is retained as a compatibility alias for report.Summary.
type Summary = report.Summary

// DeltaReport is retained as a compatibility alias for report.DeltaReport.
type DeltaReport = report.DeltaReport

// ClassifiedEdgeSummary is retained as a compatibility alias.
type ClassifiedEdgeSummary = report.ClassifiedEdgeSummary

// CouplingTailRiskSummary is retained as a compatibility alias.
type CouplingTailRiskSummary = report.CouplingTailRiskSummary

// DistanceCompressionSummary is retained as a compatibility alias.
type DistanceCompressionSummary = report.DistanceCompressionSummary

// DistanceCount is retained as a compatibility alias.
type DistanceCount = report.DistanceCount

// DistanceOmittedRungReason is retained as a compatibility alias.
type DistanceOmittedRungReason = report.DistanceOmittedRungReason

// VolatilityProvenance is retained as a compatibility alias.
type VolatilityProvenance = report.VolatilityProvenance

// GitFindingDelta is retained as a compatibility alias.
type GitFindingDelta = report.GitFindingDelta

// AgentTask is retained as a compatibility alias for scan.AgentTask.
type AgentTask = scan.AgentTask

// AdvisoryTask is retained as a compatibility alias for scan.AdvisoryTask.
type AdvisoryTask = scan.AdvisoryTask

// Diagnostic is retained as a compatibility alias for scan.Diagnostic.
type Diagnostic = scan.Diagnostic

// SchemaVersion preserves the old diagnostic package entry point.
const SchemaVersion = scan.SchemaVersion

// New preserves the old diagnostic package entry point.
func New() Diagnostic { return scan.New() }

// Verdict and direction constants preserve the old package vocabulary.
const (
	VerdictPass             = report.VerdictPass
	VerdictFail             = report.VerdictFail
	VerdictWarn             = report.VerdictWarn
	DirectionHigherIsBetter = report.DirectionHigherIsBetter
	DirectionHigherIsWorse  = report.DirectionHigherIsWorse
)

// Coverage and dynamic-import constants preserve the old package vocabulary.
const (
	StatusOK       = evidence.StatusOK
	StatusPartial  = evidence.StatusPartial
	StatusAbsent   = evidence.StatusAbsent
	StatusDisabled = evidence.StatusDisabled
	StatusTimedOut = evidence.StatusTimedOut

	DynamicImportKindLazyImport    = evidence.DynamicImportKindLazyImport
	DynamicImportKindImportlib     = evidence.DynamicImportKindImportlib
	DynamicImportKindRequire       = evidence.DynamicImportKindRequire
	DynamicImportKindDynamicImport = evidence.DynamicImportKindDynamicImport
)

// Git-origin comparison statuses for GitFindingDelta.ComparisonStatus.
const (
	// GitComparisonComparable means every current repair task was placed in a
	// definite origin bucket (introduced or pre_existing).
	GitComparisonComparable = "comparable"
	// GitComparisonUnknown means at least one repair task could not be placed,
	// so the delta must not be read as a complete list of new work.
	GitComparisonUnknown = "unknown"
)
