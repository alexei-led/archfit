package coupling

import "testing"

// TestBalanceResult covers the severity table from the Balanced Coupling model (Khononov).
func TestBalanceResult(t *testing.T) {
	tests := []struct {
		name     string
		c        Classification
		expected Severity
	}{
		// --- Symmetric balanced quadrant (low strength + low distance) ---
		{
			// low+low+low_vol → none (balanced).
			// cross_module_same_owner is the realistic low-distance input
			// (same_module edges are excluded by the classifier before BalanceResult).
			name: "low+low low_vol returns none",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			expected: SeverityNone,
		},
		{
			// low+low+high_vol → medium (over-decoupled volatile seam).
			name: "low+low high_vol returns medium",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityMedium,
		},

		// --- XOR modular quadrants (Balanced Coupling §3): SeverityNone ---
		{
			// low strength + high distance → loose coupling across a large boundary.
			// BC-modular: no finding regardless of volatility.
			name: "low+high low_vol returns none (XOR loose quadrant)",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			expected: SeverityNone,
		},
		{
			// low strength + high distance, volatile: still BC-modular, no finding.
			name: "low+high high_vol returns none (XOR loose quadrant)",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityNone,
		},
		{
			// high strength + low distance → cohesive (co-located tight coupling).
			// BC-modular: no finding regardless of volatility.
			name: "high+low returns none (XOR cohesive quadrant)",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityNone,
		},
		{
			// functional strength + same-module-same-owner is a co-located case:
			// strengthIsHigh=true, distanceIsHigh=false → XOR cohesive quadrant → none.
			// (same_module edges are excluded by the classifier in practice, but the
			// formula must be correct for cross_module_same_owner too.)
			name: "functional+same_owner low_vol returns none (XOR cohesive quadrant)",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			expected: SeverityNone,
		},

		// --- Symmetric unbalanced quadrant (high strength + high distance) ---
		{
			// high+high+low_vol → medium.
			name: "high+high low_vol returns medium",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			expected: SeverityMedium,
		},
		{
			// high+high+high_vol → critical.
			name: "high+high high_vol returns critical",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityCritical,
		},

		// --- Intrusive escalation paths ---
		{
			// intrusive + cross-module-diff-owner + low volatility → medium.
			name: "intrusive cross-module low_vol returns medium",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityLow,
			},
			expected: SeverityMedium,
		},
		{
			// intrusive + cross-module-diff-owner + high volatility → high.
			name: "intrusive cross-module high_vol returns high",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityHigh,
		},
		{
			// intrusive + cross-deploy-unit → critical (volatility irrelevant).
			name: "intrusive cross-deploy returns critical",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityCritical,
		},
		{
			// intrusive + same-owner falls through to the formula: high+low → XOR none.
			name: "intrusive same-owner falls through to XOR none",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BalanceResult(tt.c)
			if got != tt.expected {
				t.Errorf("BalanceResult(%+v) = %q, want %q", tt.c, got, tt.expected)
			}
		})
	}
}
