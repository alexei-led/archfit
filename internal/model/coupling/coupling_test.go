package coupling

import "testing"

// TestBalanceResult covers the severity table from spec §18.
func TestBalanceResult(t *testing.T) {
	tests := []struct {
		name     string
		c        Classification
		expected Severity
	}{
		{
			// low+low+low_vol → none (balanced).
			// Note: same_module is excluded by the classifier before BalanceResult is called;
			// cross_module_same_owner is the more realistic low-distance input here.
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
		{
			// low+high → low regardless of volatility.
			name: "low+high low_vol returns low",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			expected: SeverityLow,
		},
		{
			// low+high → low regardless of volatility.
			name: "low+high high_vol returns low",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			expected: SeverityLow,
		},
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
		{
			// high+low → low (high cohesion).
			name: "high+low returns low",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			expected: SeverityLow,
		},
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
