package decision_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/decision"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/score"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeScorecard(overall int, dims []score.Dimension) score.Scorecard {
	return score.Scorecard{
		RubricVersion: 1,
		Overall:       overall,
		OverallBand:   bandForValue(overall),
		Dimensions:    dims,
	}
}

// bandForValue mirrors score.bandFor (exported equivalent not available; reproduce
// the same mapping so test helpers stay self-contained).
func bandForValue(v int) score.Band {
	switch {
	case v <= 20:
		return score.BandCritical
	case v <= 40:
		return score.BandPoor
	case v <= 60:
		return score.BandMixed
	case v <= 80:
		return score.BandServiceable
	default:
		return score.BandStrong
	}
}

func makeDim(name string, value int, conf score.Confidence, meta bool) score.Dimension {
	return score.Dimension{
		Name:       name,
		Value:      value,
		Band:       bandForValue(value),
		Confidence: conf,
		Evidence:   []string{"evidence for " + name},
		Summary:    "summary of " + name,
		Meta:       meta,
	}
}

func healthyDims() []score.Dimension {
	return []score.Dimension{
		makeDim(score.DimBoundaryIntegrity, 75, score.ConfidenceHigh, false),
		makeDim(score.DimCouplingBalance, 70, score.ConfidenceHigh, false),
		makeDim(score.DimDependencyGraphHealth, 72, score.ConfidenceHigh, false),
		makeDim(score.DimCohesionModularity, 68, score.ConfidenceHigh, false),
		makeDim(score.DimChangeLocality, 80, score.ConfidenceHigh, false),
		makeDim(score.DimArchitectureFitness, 75, score.ConfidenceHigh, false),
		makeDim(score.DimAnalysisConfidence, 90, score.ConfidenceHigh, true),
	}
}

func makeDiag(verdict diagnostic.Verdict, gateFindings, warnings int) diagnostic.Diagnostic {
	d := diagnostic.New()
	d.Verdict = verdict
	d.Summary = diagnostic.Summary{
		GateFindings: gateFindings,
		Warnings:     warnings,
	}
	return d
}

func gateFinding(ruleID string, status finding.Status) finding.Finding {
	return finding.Finding{
		ID:       ruleID + "-id",
		Kind:     "gate",
		RuleID:   ruleID,
		Status:   status,
		Severity: finding.SeverityCritical,
		Why:      "because " + ruleID,
	}
}

func advisoryFinding(ruleID string, sev finding.Severity) finding.Finding {
	return finding.Finding{
		ID:       ruleID + "-id",
		Kind:     "advisory",
		RuleID:   ruleID,
		Status:   finding.StatusNew,
		Severity: sev,
		Why:      "advisory: " + ruleID,
	}
}

// ---------------------------------------------------------------------------
// Band logic — all four bands
// ---------------------------------------------------------------------------

func TestBuild_BandFail_VerdictFail(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictFail, 2, 0)
	sc := makeScorecard(75, healthyDims())
	r := decision.Build(diag, sc, nil, false)
	if r.Band != decision.BandFail {
		t.Errorf("expected FAIL, got %q", r.Band)
	}
	if r.Headline == "" {
		t.Error("Headline must not be empty")
	}
}

func TestBuild_BandFail_HardGate(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	sc := makeScorecard(85, healthyDims())
	r := decision.Build(diag, sc, nil, true) // hardGate=true
	if r.Band != decision.BandFail {
		t.Errorf("expected FAIL from hardGate, got %q", r.Band)
	}
}

func TestBuild_CriticalHighDim_DoesNotEscalate(t *testing.T) {
	// A single critical-at-high-confidence dimension must NOT escalate the
	// decision past the overall band — soft/advisory criticals (e.g. cohesion
	// clone-scoring) are calibration-sensitive. Mirrors archfit's own shape
	// (mixed overall, 0 blockers, a critical advisory dim) → ACCEPTABLE, per the brief.
	dims := healthyDims()
	dims[0] = makeDim(score.DimBoundaryIntegrity, 10, score.ConfidenceHigh, false)
	sc := makeScorecard(58, dims) // mixed overall
	diag := makeDiag(diagnostic.VerdictPass, 0, 12)
	r := decision.Build(diag, sc, nil, false)
	if r.Band != decision.BandAcceptable {
		t.Errorf("critical dim with mixed overall must stay ACCEPTABLE, got %q", r.Band)
	}
}

func TestBuild_BandNeedsAttention_OverallPoor(t *testing.T) {
	dims := make([]score.Dimension, len(healthyDims()))
	copy(dims, healthyDims())
	sc := makeScorecard(30, dims) // overall 30 → poor
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	r := decision.Build(diag, sc, nil, false)
	if r.Band != decision.BandNeedsAttention {
		t.Errorf("expected NEEDS_ATTENTION for poor overall, got %q", r.Band)
	}
}

func TestBuild_BandNeedsAttention_CriticalLowConfidence_NotTriggered(t *testing.T) {
	// Critical dim but low confidence → should NOT trigger NEEDS_ATTENTION from
	// the critical-high-dim rule; may still fall back to ACCEPTABLE.
	dims := healthyDims()
	dims[0] = makeDim(score.DimBoundaryIntegrity, 10, score.ConfidenceLow, false)
	sc := makeScorecard(65, dims) // serviceable overall, warnings=0, pass verdict
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	r := decision.Build(diag, sc, nil, false)
	// NEEDS_ATTENTION from critical+high not fired; overall is serviceable but
	// there IS a critical dim, so it won't be HEALTHY either — expect ACCEPTABLE.
	if r.Band == decision.BandNeedsAttention {
		t.Errorf("critical dim with low confidence must not trigger NEEDS_ATTENTION; got %q", r.Band)
	}
}

func TestBuild_BandHealthy(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0) // no warnings, pass
	sc := makeScorecard(82, healthyDims())         // strong overall
	r := decision.Build(diag, sc, nil, false)
	if r.Band != decision.BandHealthy {
		t.Errorf("expected HEALTHY, got %q", r.Band)
	}
}

func TestBuild_BandHealthy_Serviceable(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	sc := makeScorecard(70, healthyDims()) // serviceable
	r := decision.Build(diag, sc, nil, false)
	if r.Band != decision.BandHealthy {
		t.Errorf("expected HEALTHY for serviceable+no warnings+pass, got %q", r.Band)
	}
}

func TestBuild_BandAcceptable_WarnVerdict(t *testing.T) {
	// serviceable overall, but verdict=warn → not HEALTHY → ACCEPTABLE
	diag := makeDiag(diagnostic.VerdictWarn, 0, 2)
	sc := makeScorecard(75, healthyDims())
	r := decision.Build(diag, sc, nil, false)
	if r.Band != decision.BandAcceptable {
		t.Errorf("expected ACCEPTABLE_WITH_WATCH_ITEMS for warn verdict, got %q", r.Band)
	}
}

func TestBuild_BandAcceptable_WithAdvisories(t *testing.T) {
	// serviceable but has advisory warnings
	diag := makeDiag(diagnostic.VerdictPass, 0, 3)
	sc := makeScorecard(75, healthyDims())
	r := decision.Build(diag, sc, nil, false)
	if r.Band != decision.BandAcceptable {
		t.Errorf("expected ACCEPTABLE_WITH_WATCH_ITEMS for warnings>0, got %q", r.Band)
	}
}

func TestBuild_BandAcceptable_MixedOverall(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	sc := makeScorecard(55, healthyDims()) // mixed
	r := decision.Build(diag, sc, nil, false)
	if r.Band != decision.BandAcceptable {
		t.Errorf("expected ACCEPTABLE_WITH_WATCH_ITEMS for mixed overall, got %q", r.Band)
	}
}

// ---------------------------------------------------------------------------
// Exhaustiveness assertion
// ---------------------------------------------------------------------------

// TestBuild_NeverEmptyBand asserts Build always returns a non-empty Band by
// sweeping the key input axes.
func TestBuild_NeverEmptyBand(t *testing.T) {
	verdicts := []diagnostic.Verdict{
		diagnostic.VerdictPass,
		diagnostic.VerdictFail,
		diagnostic.VerdictWarn,
	}
	overalls := []int{10, 30, 55, 75, 90}
	warningCounts := []int{0, 1, 5}
	hardGates := []bool{false, true}
	hasCriticalHighDimOptions := []bool{false, true}

	for _, verdict := range verdicts {
		for _, overall := range overalls {
			for _, warnings := range warningCounts {
				for _, hg := range hardGates {
					for _, withCritical := range hasCriticalHighDimOptions {
						dims := healthyDims()
						if withCritical {
							dims[0] = makeDim(score.DimBoundaryIntegrity, 10, score.ConfidenceHigh, false)
						}
						sc := makeScorecard(overall, dims)
						diag := makeDiag(verdict, 0, warnings)
						r := decision.Build(diag, sc, nil, hg)
						if r.Band == "" {
							t.Errorf(
								"Build returned empty Band for verdict=%s overall=%d warnings=%d hardGate=%v criticalDim=%v",
								verdict, overall, warnings, hg, withCritical,
							)
						}
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Field propagation
// ---------------------------------------------------------------------------

func TestBuild_FieldPropagation(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 3, 2)
	sc := makeScorecard(70, healthyDims())
	r := decision.Build(diag, sc, nil, false)

	if r.Blocking != 3 {
		t.Errorf("Blocking: want 3, got %d", r.Blocking)
	}
	if r.Advisory != 2 {
		t.Errorf("Advisory: want 2, got %d", r.Advisory)
	}
	if r.Overall != 70 {
		t.Errorf("Overall: want 70, got %d", r.Overall)
	}
	if r.OverallBand != score.BandServiceable {
		t.Errorf("OverallBand: want serviceable, got %q", r.OverallBand)
	}
	if len(r.Dimensions) != len(healthyDims()) {
		t.Errorf("Dimensions: want %d, got %d", len(healthyDims()), len(r.Dimensions))
	}
}

// ---------------------------------------------------------------------------
// Dimension rendering
// ---------------------------------------------------------------------------

func TestBuild_DimReportWhy(t *testing.T) {
	dims := healthyDims()
	sc := makeScorecard(70, dims)
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	r := decision.Build(diag, sc, nil, false)

	for _, dr := range r.Dimensions {
		if dr.Why == "" {
			t.Errorf("DimReport.Why must not be empty for dim %q", dr.Name)
		}
	}
}

func TestBuild_DimReportWhatMoves(t *testing.T) {
	dims := healthyDims()
	sc := makeScorecard(70, dims)
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	r := decision.Build(diag, sc, nil, false)

	for _, dr := range r.Dimensions {
		if dr.Meta {
			if dr.WhatMoves != "" {
				t.Errorf("meta dim %q must have empty WhatMoves, got %q", dr.Name, dr.WhatMoves)
			}
		} else {
			if dr.WhatMoves == "" {
				t.Errorf("non-meta dim %q must have non-empty WhatMoves", dr.Name)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Recommendation bucketing
// ---------------------------------------------------------------------------

func TestBuild_Recommendations_MustFix(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictFail, 2, 0)
	diag.Findings = []finding.Finding{
		gateFinding("forbidden_dep", finding.StatusNew),
		gateFinding("layer_violation", finding.StatusExpiredWaiver),
		gateFinding("already_fixed", finding.StatusFixed),  // must NOT appear
		gateFinding("excepted_rule", finding.StatusWaived), // must NOT appear
	}
	sc := makeScorecard(30, healthyDims())
	r := decision.Build(diag, sc, nil, false)

	if len(r.Recommendations.MustFix) != 2 {
		t.Errorf("MustFix: want 2 (new+expired_waiver), got %d", len(r.Recommendations.MustFix))
	}
	ruleIDs := make(map[string]bool)
	for _, rec := range r.Recommendations.MustFix {
		ruleIDs[rec.RuleID] = true
	}
	for _, want := range []string{"forbidden_dep", "layer_violation"} {
		if !ruleIDs[want] {
			t.Errorf("MustFix missing rule %q", want)
		}
	}
}

func TestBuild_Recommendations_ShouldFix(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictWarn, 0, 2)
	diag.Findings = []finding.Finding{
		advisoryFinding("high_risk_rule", finding.SeverityHigh),
		advisoryFinding("critical_rule", finding.SeverityCritical),
		advisoryFinding("medium_rule", finding.SeverityMedium), // goes to Watch
	}
	sc := makeScorecard(65, healthyDims())
	r := decision.Build(diag, sc, nil, false)

	if len(r.Recommendations.ShouldFix) != 2 {
		t.Errorf("ShouldFix: want 2 (critical+high), got %d", len(r.Recommendations.ShouldFix))
	}
	if len(r.Recommendations.Watch) != 1 {
		t.Errorf("Watch: want 1 (medium), got %d", len(r.Recommendations.Watch))
	}
}

func TestBuild_Recommendations_Watch_LowSeverity(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictWarn, 0, 1)
	diag.Findings = []finding.Finding{
		advisoryFinding("low_risk", finding.SeverityLow),
	}
	sc := makeScorecard(65, healthyDims())
	r := decision.Build(diag, sc, nil, false)

	if len(r.Recommendations.Watch) != 1 {
		t.Errorf("Watch: want 1 low-severity advisory, got %d", len(r.Recommendations.Watch))
	}
}

func TestBuild_Recommendations_DedupeByRuleID(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictFail, 3, 0)
	diag.Findings = []finding.Finding{
		gateFinding("same_rule", finding.StatusNew),
		gateFinding("same_rule", finding.StatusNew), // duplicate ruleID → one Rec
		gateFinding("other_rule", finding.StatusNew),
	}
	sc := makeScorecard(20, healthyDims())
	r := decision.Build(diag, sc, nil, false)

	if len(r.Recommendations.MustFix) != 2 {
		t.Errorf("MustFix after dedup: want 2, got %d", len(r.Recommendations.MustFix))
	}
}

func TestBuild_Recommendations_SortedByRuleID(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictFail, 3, 0)
	diag.Findings = []finding.Finding{
		gateFinding("zzz_rule", finding.StatusNew),
		gateFinding("aaa_rule", finding.StatusNew),
		gateFinding("mmm_rule", finding.StatusNew),
	}
	sc := makeScorecard(20, healthyDims())
	r := decision.Build(diag, sc, nil, false)

	want := []string{"aaa_rule", "mmm_rule", "zzz_rule"}
	for i, rec := range r.Recommendations.MustFix {
		if rec.RuleID != want[i] {
			t.Errorf("MustFix[%d].RuleID = %q, want %q", i, rec.RuleID, want[i])
		}
	}
}

func TestBuild_Recommendations_EmptyCalibrateIgnore(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	sc := makeScorecard(70, healthyDims())
	r := decision.Build(diag, sc, nil, false)

	if r.Recommendations.Calibrate == nil {
		t.Error("Calibrate must be non-nil empty slice")
	}
	if len(r.Recommendations.Calibrate) != 0 {
		t.Errorf("Calibrate: want 0, got %d", len(r.Recommendations.Calibrate))
	}
	if r.Recommendations.Ignore == nil {
		t.Error("Ignore must be non-nil empty slice")
	}
	if len(r.Recommendations.Ignore) != 0 {
		t.Errorf("Ignore: want 0, got %d", len(r.Recommendations.Ignore))
	}
}

// ---------------------------------------------------------------------------
// Delta computation
// ---------------------------------------------------------------------------

func TestBuild_Delta_NilWhenNoBase(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	sc := makeScorecard(70, healthyDims())
	r := decision.Build(diag, sc, nil, false)
	if r.Delta != nil {
		t.Error("Delta must be nil when no base scorecard provided")
	}
}

func TestBuild_Delta_OverallChange(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	base := makeScorecard(60, healthyDims())
	current := makeScorecard(75, healthyDims())
	r := decision.Build(diag, current, &base, false)
	if r.Delta == nil {
		t.Fatal("Delta must be non-nil when base provided")
	}
	if r.Delta.Overall != 15 {
		t.Errorf("Delta.Overall: want 15, got %d", r.Delta.Overall)
	}
}

func TestBuild_Delta_NegativeChange(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	baseDims := healthyDims()
	curDims := healthyDims()
	curDims[0] = makeDim(score.DimBoundaryIntegrity, 40, score.ConfidenceHigh, false) // dropped from 75 to 40
	base := makeScorecard(75, baseDims)
	current := makeScorecard(60, curDims)
	r := decision.Build(diag, current, &base, false)
	if r.Delta == nil {
		t.Fatal("Delta must be non-nil when base provided")
	}
	if r.Delta.Overall != -15 {
		t.Errorf("Delta.Overall: want -15, got %d", r.Delta.Overall)
	}
	// Find the boundary_integrity dim delta.
	var found bool
	for _, dd := range r.Delta.Dimensions {
		if dd.Name == score.DimBoundaryIntegrity {
			found = true
			if dd.Before != 75 {
				t.Errorf("DimDelta.Before: want 75, got %d", dd.Before)
			}
			if dd.After != 40 {
				t.Errorf("DimDelta.After: want 40, got %d", dd.After)
			}
			if dd.Change != -35 {
				t.Errorf("DimDelta.Change: want -35, got %d", dd.Change)
			}
		}
	}
	if !found {
		t.Error("DimDelta for boundary_integrity not found")
	}
}

func TestBuild_Delta_ZeroChange(t *testing.T) {
	diag := makeDiag(diagnostic.VerdictPass, 0, 0)
	dims := healthyDims()
	base := makeScorecard(70, dims)
	current := makeScorecard(70, dims)
	r := decision.Build(diag, current, &base, false)
	if r.Delta.Overall != 0 {
		t.Errorf("Delta.Overall: want 0, got %d", r.Delta.Overall)
	}
}
