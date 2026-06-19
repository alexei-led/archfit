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
		// functional+cross_deploy+high_vol → raw=5+5-0=10 (critical).
		// reduce_strength (functional→unknown): 3+5-0=8 (high) → drops one band.
		// reduce_distance (cross_deploy→diff_owner): 5+3-0=8 (high) → also one band; tie → prefer strength.
		// (intrusive+cross_deploy doesn't change band via reduce_strength in the additive
		//  model, so functional is the clearer reduce_strength demonstration.)
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
			// Same-module edges are not cross-boundary coupling → guarded to 0;
			// the intrusive floor applies only to cross-boundary edges.
			if d == DistanceSameModule {
				if got.Value != 0 {
					t.Errorf("same-module should score 0: d=%s v=%s value=%d", d, v, got.Value)
				}
				continue
			}
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

// TestAsyncBridgeDistanceBump verifies that setting AsyncBridge=true raises the
// score relative to the same Classification with AsyncBridge=false, for both
// AdditiveScorer and MultiplicativeScorer. The bump models lifecycle decoupling
// (+1 distance level) and is report-only — never changes the gate verdict.
func TestAsyncBridgeDistanceBump(t *testing.T) {
	base := Classification{
		Strength:   StrengthFunctional,
		Distance:   DistanceCrossModuleSameOwner,
		Volatility: VolatilityHigh,
	}
	bridged := base
	bridged.AsyncBridge = true

	t.Run("AdditiveScorer", func(t *testing.T) {
		s := AdditiveScorer{}
		without := s.Score(base)
		with := s.Score(bridged)
		if with.Value <= without.Value {
			t.Errorf("AsyncBridge should raise score: without=%d with=%d", without.Value, with.Value)
		}
	})

	t.Run("MultiplicativeScorer", func(t *testing.T) {
		s := MultiplicativeScorer{}
		without := s.Score(base)
		with := s.Score(bridged)
		// Multiplicative uses continuous XOR; bump moves dNorm closer to sNorm
		// (functional=5/8=0.625, same_owner=1/5=0.2 → bumped=2/5=0.4).
		// R_mod without=1-|0.625-0.2|=0.575; with=1-|0.625-0.4|=0.775.
		// Both * high_vol(1.0): scores 6 → 8. Value must be higher.
		if with.Value <= without.Value {
			t.Errorf("AsyncBridge should raise score: without=%d with=%d", without.Value, with.Value)
		}
	})

	t.Run("AsyncBridge_at_max_distance_no_overflow", func(t *testing.T) {
		// When already at cross_deploy (dv=5), bump is clamped to 5 — no overflow.
		atMax := Classification{
			Strength:    StrengthFunctional,
			Distance:    DistanceCrossDeployUnit,
			Volatility:  VolatilityHigh,
			AsyncBridge: true,
		}
		for _, s := range []Scorer{AdditiveScorer{}, MultiplicativeScorer{}} {
			got := s.Score(atMax)
			if got.Value < 0 || got.Value > 10 {
				t.Errorf("clamp violated at max distance with AsyncBridge: value=%d", got.Value)
			}
		}
	})
}

// TestVolatilityMoveLabel verifies the cheapest-move label distinguishes a
// config gap (undeclared → "declare_volatility") from a declarable level
// (→ "lower_volatility"). Both scorers route their volatility move through this
// helper, so the label replacement (Task 10) holds for either.
func TestVolatilityMoveLabel(t *testing.T) {
	if got := volatilityMoveLabel(VolatilityUndeclared); got != moveDeclareVolatility {
		t.Errorf("undeclared → %q, want %q", got, moveDeclareVolatility)
	}
	for _, v := range []Volatility{VolatilityHigh, VolatilityMedium, VolatilityLow, VolatilityUnknown} {
		if got := volatilityMoveLabel(v); got != moveLowerVolatility {
			t.Errorf("%s → %q, want %q", v, got, moveLowerVolatility)
		}
	}
}

// TestVolatilityUndeclaredScoresLikeUnknown asserts that an undeclared volatility
// is scored identically to unknown by both scorers: undeclared changes the
// guidance, never the number. This keeps already-classified output byte-stable.
func TestVolatilityUndeclaredScoresLikeUnknown(t *testing.T) {
	combos := []struct {
		s Strength
		d Distance
	}{
		{StrengthFunctional, DistanceCrossModuleDiffOwner},
		{StrengthIntrusive, DistanceCrossDeployUnit},
		{StrengthContract, DistanceCrossDeployUnit},
		{StrengthModel, DistanceCrossModuleSameOwner},
		{StrengthUnknown, DistanceUnknown},
	}
	for _, scorer := range []Scorer{AdditiveScorer{}, MultiplicativeScorer{}} {
		for _, cc := range combos {
			undeclared := scorer.Score(Classification{Strength: cc.s, Distance: cc.d, Volatility: VolatilityUndeclared})
			unknown := scorer.Score(Classification{Strength: cc.s, Distance: cc.d, Volatility: VolatilityUnknown})
			if undeclared.Value != unknown.Value || undeclared.Band != unknown.Band {
				t.Errorf("%T %s/%s: undeclared=(%d,%s) unknown=(%d,%s) — must match",
					scorer, cc.s, cc.d, undeclared.Value, undeclared.Band, unknown.Value, unknown.Band)
			}
		}
	}
}

// TestMultiplicativeScorer_CheapestMove_DeclareVsLower verifies the cheapest-move
// label flips with declaredness on the same structural edge: when volatility is
// the band-dropping move, an undeclared target reads "declare_volatility" and a
// declared (medium) target reads "lower_volatility".
func TestMultiplicativeScorer_CheapestMove_DeclareVsLower(t *testing.T) {
	s := MultiplicativeScorer{}
	// functional(0.625) + cross_module_diff_owner(0.6): R_mod≈0.975, so declaring/
	// lowering volatility to low (norm 0.2) drops two bands — the strict winner over
	// the single-band strength/distance reductions.
	base := Classification{Strength: StrengthFunctional, Distance: DistanceCrossModuleDiffOwner}

	undeclared := base
	undeclared.Volatility = VolatilityUndeclared
	if got := s.Score(undeclared).CheapestMove; got != moveDeclareVolatility {
		t.Errorf("undeclared edge cheapest move = %q, want %q", got, moveDeclareVolatility)
	}

	declared := base
	declared.Volatility = VolatilityMedium
	if got := s.Score(declared).CheapestMove; got != moveLowerVolatility {
		t.Errorf("declared (medium) edge cheapest move = %q, want %q", got, moveLowerVolatility)
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
	// DefaultScorer is locked to MultiplicativeScorer (Task 16) — assert the type,
	// so a revert to a different default is caught.
	if got.Reason != reasonMultiplicative {
		t.Errorf("DefaultScorer should be the locked MultiplicativeScorer; Reason = %q, want %q", got.Reason, reasonMultiplicative)
	}
}
