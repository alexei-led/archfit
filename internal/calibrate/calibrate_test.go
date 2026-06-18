package calibrate_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/calibrate"
	"github.com/alexei-led/archfit/internal/model/coupling"
)

// syntheticIndex builds a small coupling.Index covering key scoring quadrants.
// All four edges are cross-boundary (not same_module, not unknown distance) so
// all four are included in the calibration report.
//
//   - edge0: intrusive + cross_deploy_unit + high  → additive critical
//   - edge1: contract  + cross_module_same_owner + low → additive low/none (XOR: high strength, low distance)
//   - edge2: functional + cross_deploy_unit + low  → additive: 5+5-2=8→high; multi: XOR≈0, low floor → disagree likely
//   - edge3: contract  + cross_module_different_owner + high → additive: 0+3-0=3→low
func syntheticIndex() coupling.Index {
	return coupling.Index{
		"a\x00b\x00imports": {
			Strength:   coupling.StrengthIntrusive,
			Distance:   coupling.DistanceCrossDeployUnit,
			Volatility: coupling.VolatilityHigh,
		},
		"c\x00d\x00imports": {
			Strength:   coupling.StrengthContract,
			Distance:   coupling.DistanceCrossModuleSameOwner,
			Volatility: coupling.VolatilityLow,
		},
		"e\x00f\x00imports": {
			Strength:   coupling.StrengthFunctional,
			Distance:   coupling.DistanceCrossDeployUnit,
			Volatility: coupling.VolatilityLow,
		},
		"g\x00h\x00imports": {
			Strength:   coupling.StrengthContract,
			Distance:   coupling.DistanceCrossModuleDiffOwner,
			Volatility: coupling.VolatilityHigh,
		},
	}
}

// TestCompare_SyntheticIndex is a smoke test: no panic, non-nil Edges slice.
func TestCompare_SyntheticIndex(t *testing.T) {
	idx := syntheticIndex()
	r := calibrate.Compare(".", idx, coupling.AdditiveScorer{}, coupling.MultiplicativeScorer{})
	if len(r.Edges) == 0 {
		t.Fatal("Compare returned no scored edges for a non-empty index")
	}
}

func TestCompare_BasicProperties(t *testing.T) {
	idx := syntheticIndex()
	r := calibrate.Compare("/repo", idx, coupling.AdditiveScorer{}, coupling.MultiplicativeScorer{})

	if r.EdgeCount != 4 {
		t.Errorf("EdgeCount = %d, want 4", r.EdgeCount)
	}
	if r.RepoPath != "/repo" {
		t.Errorf("RepoPath = %q, want /repo", r.RepoPath)
	}

	rate := r.BandAgreementRate()
	if rate < 0.0 || rate > 1.0 {
		t.Errorf("BandAgreementRate = %f, want [0.0, 1.0]", rate)
	}
}

func TestCompare_ReasonLabels(t *testing.T) {
	idx := syntheticIndex()
	r := calibrate.Compare(".", idx, coupling.AdditiveScorer{}, coupling.MultiplicativeScorer{})

	for _, ea := range r.Edges {
		if ea.ScoreA.Reason != "additive" {
			t.Errorf("edge %q: ScoreA.Reason = %q, want additive", ea.EdgeKey, ea.ScoreA.Reason)
		}
		if ea.ScoreB.Reason != "multiplicative" {
			t.Errorf("edge %q: ScoreB.Reason = %q, want multiplicative", ea.EdgeKey, ea.ScoreB.Reason)
		}
	}
}

func TestCompare_IntrusiveCrossDeployUnitCritical(t *testing.T) {
	// edge0: intrusive + cross_deploy_unit + high
	// additive: 8+5-0=13→clamp→10 → critical
	idx := syntheticIndex()
	r := calibrate.Compare(".", idx, coupling.AdditiveScorer{}, coupling.MultiplicativeScorer{})

	found := false
	for _, ea := range r.Edges {
		if ea.Strength == coupling.StrengthIntrusive &&
			ea.Distance == coupling.DistanceCrossDeployUnit &&
			ea.Volatility == coupling.VolatilityHigh {
			found = true
			if ea.ScoreA.Band != coupling.SeverityCritical {
				t.Errorf("ScoreA.Band = %q, want critical for (intrusive, cross_deploy_unit, high)", ea.ScoreA.Band)
			}
		}
	}
	if !found {
		t.Error("expected edge (intrusive, cross_deploy_unit, high) not found in report")
	}
}

func TestCompare_XOREdgesNotCritical(t *testing.T) {
	// XOR edges: high strength + low distance OR low strength + high distance
	// Both scorers should not reach critical for these.
	//
	// edge1: contract(0) + cross_module_same_owner(1) + low → additive: 0+1-2=-1→0→none
	// edge3: contract(0) + cross_module_different_owner(3) + high → additive: 0+3-0=3→low
	idx := syntheticIndex()
	r := calibrate.Compare(".", idx, coupling.AdditiveScorer{}, coupling.MultiplicativeScorer{})

	for _, ea := range r.Edges {
		// contract edges are low-strength; both test XOR quadrants.
		if ea.Strength != coupling.StrengthContract {
			continue
		}
		if ea.ScoreA.Band == coupling.SeverityCritical {
			t.Errorf("XOR edge %q: ScoreA.Band is critical, expected none/low/medium/high", ea.EdgeKey)
		}
		if ea.ScoreB.Band == coupling.SeverityCritical {
			t.Errorf("XOR edge %q: ScoreB.Band is critical, expected none/low/medium/high", ea.EdgeKey)
		}
	}
}

func TestCompare_SkipsSameModuleAndUnknown(t *testing.T) {
	// Edges with same_module or unknown distance must be excluded.
	idx := coupling.Index{
		"a\x00b\x00imports": {
			Strength: coupling.StrengthIntrusive,
			Distance: coupling.DistanceSameModule,
		},
		"c\x00d\x00imports": {
			Strength: coupling.StrengthFunctional,
			Distance: coupling.DistanceUnknown,
		},
		"e\x00f\x00imports": {
			Strength:   coupling.StrengthContract,
			Distance:   coupling.DistanceCrossDeployUnit,
			Volatility: coupling.VolatilityLow,
		},
	}
	r := calibrate.Compare(".", idx, coupling.AdditiveScorer{}, coupling.MultiplicativeScorer{})
	if r.EdgeCount != 1 {
		t.Errorf("EdgeCount = %d, want 1 (same_module and unknown skipped)", r.EdgeCount)
	}
}

func TestBandAgreementRate_Empty(t *testing.T) {
	r := calibrate.Report{}
	if r.BandAgreementRate() != 0 {
		t.Errorf("BandAgreementRate of empty report = %f, want 0", r.BandAgreementRate())
	}
}
