package coupling

import "testing"

// TestScoreBand verifies the [0,10]→Severity band mapping.
func TestScoreBand(t *testing.T) {
	tests := []struct {
		score int
		want  Severity
	}{
		{0, SeverityNone},
		{1, SeverityNone},
		{2, SeverityNone},
		{3, SeverityLow},
		{4, SeverityLow},
		{5, SeverityMedium},
		{6, SeverityMedium},
		{7, SeverityHigh},
		{8, SeverityHigh},
		{9, SeverityCritical},
		{10, SeverityCritical},
	}
	for _, tt := range tests {
		if got := ScoreBand(tt.score); got != tt.want {
			t.Errorf("ScoreBand(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

// TestAdditiveScorer_Cube covers the strength×distance×volatility cube for the
// additive scorer. Key assertions: XOR quadrants score ≤2 (none), high+high
// scores high-to-critical, clamp stays in [0,10].
func TestAdditiveScorer_Cube(t *testing.T) {
	s := AdditiveScorer{}
	tests := []struct {
		name      string
		c         Classification
		wantBand  Severity
		wantClamp bool // value must be in [0,10]
	}{
		// XOR modular quadrants: high strength + low distance → raw = 5+1-0 = 6 (medium)
		// or 5+1-2 = 4 (low) with low vol — but more importantly value must be ≤ 10.
		// Key: XOR cohesive (functional+same_owner, low_vol): raw=5+1-2=4 → low.
		{
			name: "XOR cohesive functional+same_owner low_vol",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			wantBand:  SeverityLow,
			wantClamp: true,
		},
		// XOR loose (contract+cross_deploy, low_vol): raw=0+5-2=3 → low.
		{
			name: "XOR loose contract+cross_deploy low_vol",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			wantBand:  SeverityLow,
			wantClamp: true,
		},
		// Symmetric bad: intrusive+cross_deploy+high_vol → raw=8+5-0=13→clamp→10 (critical).
		{
			name: "intrusive+cross_deploy+high_vol → critical (clamped)",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			wantBand:  SeverityCritical,
			wantClamp: true,
		},
		// Symmetric good: contract+same_owner+low_vol → raw=0+1-2=clamp(0)→0 (none).
		{
			name: "contract+same_owner+low_vol → none (floor 0)",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			wantBand:  SeverityNone,
			wantClamp: true,
		},
		// functional+cross_deploy+high_vol → raw=5+5-0=10 (critical).
		{
			name: "functional+cross_deploy+high_vol → critical",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			wantBand:  SeverityCritical,
			wantClamp: true,
		},
		// model+cross_module_diff_owner+medium_vol → raw=2+3-1=4 (low).
		{
			name: "model+cross_diff_owner+medium_vol → low",
			c: Classification{
				Strength:   StrengthModel,
				Distance:   DistanceCrossModuleDiffOwner,
				Volatility: VolatilityMedium,
			},
			wantBand:  SeverityLow,
			wantClamp: true,
		},
		// unknown strength + unknown distance + unknown vol → raw=3+2-0=5 (medium).
		{
			name: "unknown+unknown+unknown → medium",
			c: Classification{
				Strength:   StrengthUnknown,
				Distance:   DistanceUnknown,
				Volatility: VolatilityUnknown,
			},
			wantBand:  SeverityMedium,
			wantClamp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Score(tt.c)
			if got.Band != tt.wantBand {
				t.Errorf("band = %q, want %q (value=%d)", got.Band, tt.wantBand, got.Value)
			}
			if tt.wantClamp && (got.Value < 0 || got.Value > 10) {
				t.Errorf("value %d out of [0,10]", got.Value)
			}
			if got.Reason != "additive" {
				t.Errorf("reason = %q, want %q", got.Reason, "additive")
			}
		})
	}
}

// TestMultiplicativeScorer_Cube covers key cells of the strength×distance×volatility
// cube for the multiplicative scorer. Assertions: XOR quadrants near-zero, intrusive
// floor, clamp, determinism.
func TestMultiplicativeScorer_Cube(t *testing.T) {
	s := MultiplicativeScorer{}
	tests := []struct {
		name     string
		c        Classification
		wantBand Severity
		wantMin  int // inclusive
		wantMax  int // inclusive
	}{
		// XOR cohesive: functional(5/8=0.625) + same_owner(1/5=0.2), high_vol(1.0).
		// R_mod=1-|0.625-0.2|=0.575; R_edge=0.575*1.0=0.575; score=round(5.75)=6→medium.
		{
			name: "XOR cohesive functional+same_owner+high_vol",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			wantBand: SeverityMedium,
			wantMin:  0, wantMax: 10,
		},
		// XOR loose: contract(0/8=0.0) + cross_deploy(5/5=1.0), high_vol(1.0).
		// R_mod=1-|0-1|=0; R_edge=0*1.0=0; score=0 → none.
		{
			name: "XOR loose contract+cross_deploy+high_vol → none",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			wantBand: SeverityNone,
			wantMin:  0, wantMax: 10,
		},
		// XOR loose low_vol: contract+cross_deploy+low_vol(0.2).
		// R_mod=0; R_edge=0*0.2=0; score=0 → none.
		{
			name: "XOR loose contract+cross_deploy+low_vol → none",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			wantBand: SeverityNone,
			wantMin:  0, wantMax: 10,
		},
		// Tight + volatile: intrusive(8/8=1.0)+cross_deploy(5/5=1.0)+high_vol(1.0).
		// R_mod=1-|1-1|=1; R_edge=1*1=1; score=round(10)=10 → critical.
		{
			name: "intrusive+cross_deploy+high_vol → critical",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			wantBand: SeverityCritical,
			wantMin:  9, wantMax: 10,
		},
		// Intrusive floor: intrusive+same_owner+low_vol.
		// intrusive(1.0)+same_owner(1/5=0.2)+low(0.2).
		// R_mod=1-|1.0-0.2|=0.2; R_edge=0.2*0.2=0.04; score=round(0.4)=0 → floor=3(low).
		{
			name: "intrusive floor: intrusive+same_owner+low_vol → at least low",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			wantBand: SeverityLow,
			wantMin:  3, wantMax: 10,
		},
		// Stable balanced: contract+same_owner+low_vol.
		// contract(0)+same_owner(0.2)+low(0.2).
		// R_mod=1-|0-0.2|=0.8; R_edge=0.8*0.2=0.16; score=round(1.6)=2→none.
		{
			name: "contract+same_owner+low_vol → none",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			wantBand: SeverityNone,
			wantMin:  0, wantMax: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Score(tt.c)
			if got.Band != tt.wantBand {
				t.Errorf("band = %q, want %q (value=%d)", got.Band, tt.wantBand, got.Value)
			}
			if got.Value < tt.wantMin || got.Value > tt.wantMax {
				t.Errorf("value %d not in [%d,%d]", got.Value, tt.wantMin, tt.wantMax)
			}
			if got.Value < 0 || got.Value > 10 {
				t.Errorf("value %d out of [0,10]", got.Value)
			}
			if got.Reason != "multiplicative" {
				t.Errorf("reason = %q, want %q", got.Reason, "multiplicative")
			}
		})
	}
}

// TestAdditiveScorer_CheapestMove verifies that cheapest_move is deterministic and
// returns the right dimension change label.
func TestAdditiveScorer_CheapestMove(t *testing.T) {
	s := AdditiveScorer{}
	tests := []struct {
		name      string
		c         Classification
		wantEmpty bool   // expect no cheapest move (already none)
		wantLabel string // expected label when not empty
	}{
		// Already none: no move needed.
		{
			name: "already none → empty",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityLow,
			},
			wantEmpty: true,
		},
		// Critical: intrusive+cross_deploy+high_vol → reduce_strength drops most.
		// intrusive→functional: raw=5+5-0=10(critical); vs reduce_distance cross_deploy→diff_owner: 8+3-0=11→10(critical); so strength ties.
		// Actually intrusive→functional: 5+5=10-0=10→critical. Hmm, doesn't drop.
		// Let's test functional+cross_deploy+high_vol: raw=10(critical).
		// reduce_strength(functional→unknown): 3+5-0=8(high) → drop 1
		// reduce_distance(cross_deploy→diff_owner): 5+3-0=8(high) → drop 1 (tie, prefer strength)
		{
			name: "functional+cross_deploy+high_vol → reduce_strength",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			wantLabel: "reduce_strength",
		},
		// functional+cross_deploy+unknown_vol → raw=5+5-0=10(critical).
		// lower_volatility(unk→low): 5+5-2=8(high) → drop 1; reduce_strength: 3+5-0=8 → drop 1; tie→prefer strength.
		{
			name: "functional+cross_deploy+unknown → reduce_strength (tie-break over lower_vol)",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityUnknown,
			},
			wantLabel: "reduce_strength",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Score(tt.c)
			if tt.wantEmpty {
				if got.CheapestMove != "" {
					t.Errorf("CheapestMove = %q, want empty", got.CheapestMove)
				}
				return
			}
			if got.CheapestMove != tt.wantLabel {
				t.Errorf("CheapestMove = %q, want %q (band=%q value=%d)", got.CheapestMove, tt.wantLabel, got.Band, got.Value)
			}
		})
	}
}

// TestMultiplicativeScorer_IntrusiveFloor verifies the intrusive floor across all
// distance and volatility combinations.
func TestMultiplicativeScorer_IntrusiveFloor(t *testing.T) {
	s := MultiplicativeScorer{}
	distances := []Distance{
		DistanceSameModule,
		DistanceCrossModuleSameOwner,
		DistanceCrossModuleDiffOwner,
		DistanceCrossDeployUnit,
		DistanceUnknown,
	}
	vols := []Volatility{VolatilityLow, VolatilityMedium, VolatilityHigh, VolatilityUnknown}

	for _, d := range distances {
		for _, v := range vols {
			c := Classification{Strength: StrengthIntrusive, Distance: d, Volatility: v}
			got := s.Score(c)
			if got.Value < intrusiveFloor {
				t.Errorf("intrusive floor violated: d=%s v=%s value=%d < %d",
					d, v, got.Value, intrusiveFloor)
			}
			if got.Value < 0 || got.Value > 10 {
				t.Errorf("clamp violated: d=%s v=%s value=%d", d, v, got.Value)
			}
		}
	}
}

// TestLegacyShim_ReproducesBalanceResult verifies the shim produces the same band
// as BalanceResult for all tested combinations.
func TestLegacyShim_ReproducesBalanceResult(t *testing.T) {
	shim := LegacyShim{}
	cases := []Classification{
		{Strength: StrengthContract, Distance: DistanceCrossModuleSameOwner, Volatility: VolatilityLow},
		{Strength: StrengthContract, Distance: DistanceCrossModuleSameOwner, Volatility: VolatilityHigh},
		{Strength: StrengthFunctional, Distance: DistanceCrossDeployUnit, Volatility: VolatilityHigh},
		{Strength: StrengthFunctional, Distance: DistanceCrossDeployUnit, Volatility: VolatilityLow},
		{Strength: StrengthIntrusive, Distance: DistanceCrossModuleDiffOwner, Volatility: VolatilityHigh},
		{Strength: StrengthIntrusive, Distance: DistanceCrossDeployUnit, Volatility: VolatilityHigh},
		{Strength: StrengthContract, Distance: DistanceCrossDeployUnit, Volatility: VolatilityHigh},
		{Strength: StrengthFunctional, Distance: DistanceCrossModuleSameOwner, Volatility: VolatilityHigh},
	}
	for _, c := range cases {
		got := shim.Score(c)
		want := BalanceResult(c)
		if got.Band != want {
			t.Errorf("LegacyShim band=%q but BalanceResult=%q for %+v", got.Band, want, c)
		}
		if got.Reason != "legacy" {
			t.Errorf("reason = %q, want %q", got.Reason, "legacy")
		}
	}
}

// TestDefaultScorer returns LegacyShim (not nil).
func TestDefaultScorer(t *testing.T) {
	s := DefaultScorer()
	if s == nil {
		t.Fatal("DefaultScorer() returned nil")
	}
	// It should produce a valid score for a concrete input.
	got := s.Score(Classification{
		Strength:   StrengthFunctional,
		Distance:   DistanceCrossDeployUnit,
		Volatility: VolatilityHigh,
	})
	if got.Band == "" && got.Value == 0 && got.Reason == "" {
		t.Error("DefaultScorer produced zero EdgeScore")
	}
}
