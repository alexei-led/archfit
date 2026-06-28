package coupling

import "testing"

// TestScoreBand_Severity verifies the book-formula severity bands.
// Severity is now derived entirely from ScoreBand(BookScorer.Score().Balance).
// Cases are annotated with (S ordinal, D ordinal, V ordinal) → balance → expected band.
func TestScoreBand_Severity(t *testing.T) {
	scoreClassification := func(c Classification) Severity {
		s := BookScorer{}.Score(c)
		if !s.Scored {
			return SeverityNone
		}
		return s.Band
	}

	tests := []struct {
		name     string
		c        Classification
		expected Severity
	}{
		// --- Symmetric balanced quadrant (low strength + low distance) ---
		{
			// S=1,D=4,V=3 → |1-4|=3, 10-3=7 → max(3,7)+1=8 → low
			name: "contract cross-module-same-owner low_vol returns low",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			expected: SeverityLow,
		},
		{
			// S=1,D=4,V=10 → |1-4|=3, 10-10=0 → max(3,0)+1=4 → high
			name: "contract cross-module-same-owner high_vol returns high",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityHigh,
		},

		// --- XOR modular quadrants ---
		{
			// S=1,D=9,V=3 → |1-9|=8, 10-3=7 → max(8,7)+1=9 → none
			name: "contract cross-deploy low_vol returns none (loose quadrant)",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			expected: SeverityNone,
		},
		{
			// S=1,D=9,V=10 → |1-9|=8, 10-10=0 → max(8,0)+1=9 → none
			name: "contract cross-deploy high_vol returns none (loose quadrant)",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityNone,
		},
		{
			// S=8,D=4,V=10 → |8-4|=4, 10-10=0 → max(4,0)+1=5 → medium
			name: "functional cross-module-same-owner high_vol returns medium (cohesive)",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityMedium,
		},
		{
			// S=8,D=4,V=3 → |8-4|=4, 10-3=7 → max(4,7)+1=8 → low
			name: "functional cross-module-same-owner low_vol returns low",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			expected: SeverityLow,
		},

		// --- Symmetric unbalanced quadrant (high strength + high distance) ---
		{
			// S=8,D=9,V=3 → |8-9|=1, 10-3=7 → max(1,7)+1=8 → low
			name: "functional cross-deploy low_vol returns low",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			expected: SeverityLow,
		},
		{
			// S=8,D=9,V=10 → |8-9|=1, 10-10=0 → max(1,0)+1=2 → critical
			name: "functional cross-deploy high_vol returns critical",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityCritical,
		},
		{
			// S=8,D=9,V=10 undeclared → |8-9|=1, 10-10=0 → max(1,0)+1=2 → critical
			name: "functional cross-deploy undeclared returns critical (conservative)",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityUndeclared,
			},
			expected: SeverityCritical,
		},

		// --- P3 divergence Case A: StrengthSymmetric + SameOwner (was None, now Medium) ---
		// S=9,D=4,V=10 → |9-4|=5, 10-10=0 → max(5,0)+1=6 → medium
		// BalanceResult returned None (XOR cohesive quadrant). Book formula: Medium.
		{
			name: "symmetric same-owner high_vol returns medium (P3 case A fix)",
			c: Classification{
				Strength:   StrengthSymmetric,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityMedium,
		},

		// --- P3 divergence Case B: StrengthSymmetric + DiffOwner (was Critical, now High) ---
		// S=9,D=7,V=10 → |9-7|=2, 10-10=0 → max(2,0)+1=3 → high
		// BalanceResult returned Critical. Book formula: High.
		{
			name: "symmetric diff-owner high_vol returns high (P3 case B fix)",
			c: Classification{
				Strength:   StrengthSymmetric,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityHigh,
		},

		// --- Intrusive paths ---
		{
			// S=10,D=7,V=3 → |10-7|=3, 10-3=7 → max(3,7)+1=8 → low
			name: "intrusive cross-module-diff-owner low_vol returns low",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityLow,
			},
			expected: SeverityLow,
		},
		{
			// S=10,D=7,V=10 → |10-7|=3, 10-10=0 → max(3,0)+1=4 → high
			name: "intrusive cross-module-diff-owner high_vol returns high",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityHigh,
		},
		{
			// S=10,D=9,V=10 → |10-9|=1, 10-10=0 → max(1,0)+1=2 → critical
			name: "intrusive cross-deploy high_vol returns critical",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityCritical,
		},
		{
			// S=10,D=4,V=10 → |10-4|=6, 10-10=0 → max(6,0)+1=7 → low
			name: "intrusive same-owner high_vol returns low",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityLow,
		},

		// --- Abstention: unknown strength/distance → SeverityNone ---
		{
			name: "unknown strength abstains → none",
			c: Classification{
				Strength:   StrengthUnknown,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreClassification(tt.c)
			if got != tt.expected {
				s := BookScorer{}.Score(tt.c)
				t.Errorf("BookScorer.Score(%+v).Band = %q, want %q (balance=%d scored=%v)",
					tt.c, got, tt.expected, s.Balance, s.Scored)
			}
		})
	}
}
