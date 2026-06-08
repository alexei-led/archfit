package coupling

import "testing"

// TestBalanceResult covers all 6 rows of the severity specification table.
func TestBalanceResult(t *testing.T) {
	tests := []struct {
		name     string
		c        Classification
		expected Severity
	}{
		{
			// Row 1: balanced → none.
			// contract (low-strength) + same_module (low-distance) → balanced.
			name: "balanced returns none",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceSameModule,
				Volatility: VolatilityHigh,
			},
			expected: SeverityNone,
		},
		{
			// Row 2: imbalanced + low volatility → low.
			// contract (low-strength) + cross_deploy_unit (high-distance) → imbalanced.
			name: "imbalanced low volatility returns low",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			expected: SeverityLow,
		},
		{
			// Row 3: imbalanced + high volatility → medium.
			// contract (low-strength) + cross_deploy_unit (high-distance) → imbalanced.
			name: "imbalanced high volatility returns medium",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityMedium,
		},
		{
			// Row 4: intrusive + cross-module-diff-owner + low volatility → medium.
			name: "intrusive cross-module low volatility returns medium",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityLow,
			},
			expected: SeverityMedium,
		},
		{
			// Row 5: intrusive + cross-module-diff-owner + high volatility → high.
			name: "intrusive cross-module high volatility returns high",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityHigh,
		},
		{
			// Row 6: intrusive + cross-deploy-unit → critical (volatility irrelevant).
			name: "intrusive cross-deploy returns critical",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityCritical,
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
