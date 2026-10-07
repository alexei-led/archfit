package scoring

import (
	"testing"

	"github.com/alexei-led/archfit/internal/relationship/coupling"
)

// TestScoreBand_Severity pins the v7 severity table across a module boundary
// (D=9, spec "Severity across modules"). The high band cannot occur across
// modules: an edge is critical exactly when it is strong (functional,
// symmetric, intrusive) and V=10.
func TestScoreBand_Severity(t *testing.T) {
	vols := []struct {
		name string
		v    coupling.Volatility
	}{
		{"frozen", coupling.VolatilityFrozen},
		{"low", coupling.VolatilityLow},
		{"medium", coupling.VolatilityMedium},
		{"high", coupling.VolatilityHigh},
		{"undeclared", coupling.VolatilityUndeclared},
	}
	// balance per volatility, in the order above.
	rows := []struct {
		strength coupling.Strength
		balance  []int
		band     []coupling.Severity
	}{
		{coupling.StrengthContract, []int{10, 9, 9, 9, 9}, []coupling.Severity{"none", "none", "none", "none", "none"}},
		{coupling.StrengthModel, []int{10, 8, 7, 7, 7}, []coupling.Severity{"none", "low", "low", "low", "low"}},
		{coupling.StrengthFunctional, []int{10, 8, 5, 2, 2}, []coupling.Severity{"none", "low", "medium", "critical", "critical"}},
		{coupling.StrengthSymmetric, []int{10, 8, 5, 1, 1}, []coupling.Severity{"none", "low", "medium", "critical", "critical"}},
		{coupling.StrengthIntrusive, []int{10, 8, 5, 2, 2}, []coupling.Severity{"none", "low", "medium", "critical", "critical"}},
	}
	distances := []coupling.Distance{coupling.DistanceCrossModule, coupling.DistanceCrossModuleDiffOwner, coupling.DistanceCrossDeployUnit}
	for _, row := range rows {
		for i, vol := range vols {
			for _, d := range distances {
				t.Run(string(row.strength)+"/"+string(d)+"/"+vol.name, func(t *testing.T) {
					s := BookScorer{}.Score(coupling.Classification{Strength: row.strength, Distance: d, Volatility: vol.v})
					if !s.Scored {
						t.Fatal("not scored")
					}
					if s.Balance != row.balance[i] {
						t.Errorf("balance = %d, want %d", s.Balance, row.balance[i])
					}
					want := row.band[i]
					if want == "none" {
						want = coupling.SeverityNone
					}
					if s.Band != want {
						t.Errorf("band = %q, want %q", s.Band, want)
					}
				})
			}
		}
	}
}

// The high band cannot occur at any module boundary, and owner or deploy unit
// never move the score: only strength and volatility do.
func TestScoreBand_BoundaryTokensScoreAlike(t *testing.T) {
	for _, st := range []coupling.Strength{coupling.StrengthContract, coupling.StrengthModel, coupling.StrengthFunctional, coupling.StrengthSymmetric, coupling.StrengthIntrusive} {
		for _, v := range []coupling.Volatility{coupling.VolatilityFrozen, coupling.VolatilityLow, coupling.VolatilityMedium, coupling.VolatilityHigh} {
			base := BookScorer{}.Score(coupling.Classification{Strength: st, Distance: coupling.DistanceCrossModule, Volatility: v})
			if base.Band == coupling.SeverityHigh {
				t.Errorf("%s/%s scored the high band at a module boundary", st, v)
			}
			for _, d := range []coupling.Distance{coupling.DistanceCrossModuleDiffOwner, coupling.DistanceCrossDeployUnit} {
				got := BookScorer{}.Score(coupling.Classification{Strength: st, Distance: d, Volatility: v})
				if got.Balance != base.Balance {
					t.Errorf("%s/%s: %s balance %d != cross_module balance %d", st, v, d, got.Balance, base.Balance)
				}
			}
		}
	}
}

func TestScoreBand_UnknownStrengthAbstains(t *testing.T) {
	s := BookScorer{}.Score(coupling.Classification{Strength: coupling.StrengthUnknown, Distance: coupling.DistanceCrossModuleDiffOwner, Volatility: coupling.VolatilityHigh})
	if s.Scored {
		t.Errorf("unknown strength must abstain, got %+v", s)
	}
}

// DistanceIsHigh is true for every module boundary and for a declared external
// system; same_module and unknown are not boundaries.
func TestDistanceIsHigh(t *testing.T) {
	tests := []struct {
		d    coupling.Distance
		want bool
	}{
		{coupling.DistanceSameModule, false},
		{coupling.DistanceCrossModule, true},
		{coupling.DistanceCrossModuleDiffOwner, true},
		{coupling.DistanceCrossDeployUnit, true},
		{coupling.DistanceExternal, true},
		{coupling.DistanceUnknown, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.d), func(t *testing.T) {
			if got := coupling.DistanceIsHigh(tt.d); got != tt.want {
				t.Errorf("DistanceIsHigh(%q) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}
