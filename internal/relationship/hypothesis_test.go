package relationship_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/relationship"
	"github.com/alexei-led/archfit/v3/internal/relationship/coupling"
	"github.com/alexei-led/archfit/v3/internal/relationship/scoring"
)

func band(s coupling.Strength, d coupling.Distance, v coupling.Volatility) relationship.Severity {
	return scoring.BookScorer{}.Score(coupling.Classification{Strength: s, Distance: d, Volatility: v}).Band
}

// Every flagged seam (band medium or worse) carries a move that re-scores to low
// or better, or a reason that needs no move. This sweeps the whole table across
// a module boundary, with and without a public surface and a cohesive source.
func TestBalancingHypothesis_EveryFlaggedSeamHasAnAnswer(t *testing.T) {
	strengths := []coupling.Strength{coupling.StrengthContract, coupling.StrengthModel, coupling.StrengthFunctional, coupling.StrengthSymmetric, coupling.StrengthIntrusive}
	vols := []coupling.Volatility{coupling.VolatilityFrozen, coupling.VolatilityLow, coupling.VolatilityMedium, coupling.VolatilityHigh, coupling.VolatilityUndeclared}
	for _, s := range strengths {
		for _, v := range vols {
			for _, public := range []bool{false, true} {
				for _, cohesive := range []bool{false, true} {
					b := band(s, coupling.DistanceCrossModule, v)
					h := relationship.BalancingHypothesis(relationship.HypothesisInput{Strength: s, Volatility: v, Band: b, TargetPublic: public, CohesiveRole: cohesive})
					if h == "" {
						t.Errorf("%s/%s band %q public=%t cohesive=%t: no hypothesis", s, v, b, public, cohesive)
						continue
					}
					flagged := b == coupling.SeverityMedium || b == coupling.SeverityHigh || b == coupling.SeverityCritical
					if !flagged {
						continue
					}
					switch h {
					case relationship.SeamHypothesisIntroduceContract:
						if got := band(coupling.StrengthContract, coupling.DistanceCrossModule, v); got != coupling.SeverityNone && got != coupling.SeverityLow {
							t.Errorf("%s/%s: introduce_contract re-scores to %q", s, v, got)
						}
					case relationship.SeamHypothesisMoveFunctionality:
						if got := band(s, coupling.DistanceSameModule, v); got != coupling.SeverityNone && got != coupling.SeverityLow {
							t.Errorf("%s/%s: move_functionality re-scores to %q", s, v, got)
						}
					case relationship.SeamHypothesisDeclareVolatility, relationship.SeamHypothesisExpectedByRole:
						// non-move answers
					default:
						t.Errorf("%s/%s band %q: hypothesis %q is not an answer for a flagged seam", s, v, b, h)
					}
				}
			}
		}
	}
}

func TestBalancingHypothesis_Rules(t *testing.T) {
	tests := []struct {
		name string
		in   relationship.HypothesisInput
		want relationship.SeamHypothesis
	}{
		{"weak strength, none band", relationship.HypothesisInput{Strength: relationship.StrengthContract, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityNone}, relationship.SeamHypothesisBalanced},
		{"weak strength, low band", relationship.HypothesisInput{Strength: relationship.StrengthModel, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityLow}, relationship.SeamHypothesisBalanced},
		{"strong into a low target", relationship.HypothesisInput{Strength: relationship.StrengthFunctional, Volatility: relationship.VolatilityLow, Band: relationship.SeverityLow}, relationship.SeamHypothesisAcceptLowVolatility},
		{"strong into a frozen target", relationship.HypothesisInput{Strength: relationship.StrengthIntrusive, Volatility: relationship.VolatilityFrozen, Band: relationship.SeverityNone}, relationship.SeamHypothesisAcceptLowVolatility},
		{"intrusive", relationship.HypothesisInput{Strength: relationship.StrengthIntrusive, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityCritical}, relationship.SeamHypothesisIntroduceContract},
		{"functional into a public target", relationship.HypothesisInput{Strength: relationship.StrengthFunctional, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityCritical, TargetPublic: true}, relationship.SeamHypothesisIntroduceContract},
		{"functional into a target without a public surface", relationship.HypothesisInput{Strength: relationship.StrengthFunctional, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityCritical}, relationship.SeamHypothesisMoveFunctionality},
		{"symmetric", relationship.HypothesisInput{Strength: relationship.StrengthSymmetric, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityCritical}, relationship.SeamHypothesisMoveFunctionality},
		{"clone fact", relationship.HypothesisInput{Strength: relationship.StrengthSymmetric, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityCritical, Clone: true}, relationship.SeamHypothesisMoveFunctionality},
		{"flagged only because volatility is undeclared", relationship.HypothesisInput{Strength: relationship.StrengthFunctional, Volatility: relationship.VolatilityUndeclared, Band: relationship.SeverityCritical}, relationship.SeamHypothesisDeclareVolatility},
		{"cohesive source, functional", relationship.HypothesisInput{Strength: relationship.StrengthFunctional, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityCritical, CohesiveRole: true}, relationship.SeamHypothesisExpectedByRole},
		{"cohesive source, intrusive is still a move", relationship.HypothesisInput{Strength: relationship.StrengthIntrusive, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityCritical, CohesiveRole: true}, relationship.SeamHypothesisIntroduceContract},
		{"clone fact ignores a cohesive source role", relationship.HypothesisInput{Strength: relationship.StrengthSymmetric, Volatility: relationship.VolatilityHigh, Band: relationship.SeverityCritical, Clone: true, CohesiveRole: true}, relationship.SeamHypothesisMoveFunctionality},
		{"abstained strength gives no answer", relationship.HypothesisInput{Strength: relationship.StrengthUnknown, Band: relationship.SeverityNone}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relationship.BalancingHypothesis(tt.in); got != tt.want {
				t.Errorf("hypothesis = %q, want %q", got, tt.want)
			}
		})
	}
}
