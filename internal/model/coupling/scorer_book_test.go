package coupling

import "testing"

// TestBookScorer_ScoreBand verifies the book balance→severity mapping.
// Book: 1-2 critical · 3-4 high · 5-6 medium · 7-8 low · 9-10 none.
func TestBookScorer_ScoreBand(t *testing.T) {
	tests := []struct {
		balance int
		want    Severity
	}{
		{1, SeverityCritical},
		{2, SeverityCritical},
		{3, SeverityHigh},
		{4, SeverityHigh},
		{5, SeverityMedium},
		{6, SeverityMedium},
		{7, SeverityLow},
		{8, SeverityLow},
		{9, SeverityNone},
		{10, SeverityNone},
	}
	for _, tt := range tests {
		if got := ScoreBand(tt.balance); got != tt.want {
			t.Errorf("ScoreBand(%d) = %q, want %q", tt.balance, got, tt.want)
		}
	}
}

// TestBookScorer_BookExamples validates the book Ch10 worked examples.
// Formula: modularity=|S-D|, balance=max(modularity, 10-V)+1.
func TestBookScorer_BookExamples(t *testing.T) {
	s := BookScorer{}
	tests := []struct {
		name        string
		c           Classification
		wantBalance int
		wantBand    Severity
		wantScored  bool
	}{
		// Distributed monolith: S=9(symmetric), D=9(cross_deploy), V=10(high).
		// |9-9|=0, 10-10=0, max(0,0)+1=1 → critical.
		{
			name: "distributed monolith: symmetric/cross_deploy/high",
			c: Classification{
				Strength:   StrengthSymmetric,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			wantBalance: 1,
			wantBand:    SeverityCritical,
			wantScored:  true,
		},
		// Model over distance: S=3(model), D=9(cross_deploy), V=10(high).
		// |3-9|=6, 10-10=0, max(6,0)+1=7 → low.
		{
			name: "model over distance: model/cross_deploy/high",
			c: Classification{
				Strength:   StrengthModel,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			wantBalance: 7,
			wantBand:    SeverityLow,
			wantScored:  true,
		},
		// Book Ch10 Example 2 — transactional cohesion:
		// S=8(functional), D=2(same_module), V=10(high).
		// |8-2|=6, 10-10=0, max(6,0)+1=7 → low. Strong coupling close together
		// is cohesion: the worked numbers reproduce through the scorer.
		{
			name: "Ch10 Example 2 transactional cohesion: functional/same_module/high",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceSameModule,
				Volatility: VolatilityHigh,
			},
			wantBalance: 7,
			wantBand:    SeverityLow,
			wantScored:  true,
		},
		// Loose coupling: S=1(contract), D=9(cross_deploy), V=10(high).
		// |1-9|=8, 10-10=0, max(8,0)+1=9 → none.
		{
			name: "loose coupling: contract/cross_deploy/high",
			c: Classification{
				Strength:   StrengthContract,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityHigh,
			},
			wantBalance: 9,
			wantBand:    SeverityNone,
			wantScored:  true,
		},
		// Ball of mud (local complexity quadrant): S=3(model), D=2(same_module),
		// V=10(high). |3-2|=1, 10-10=0, max(1,0)+1=2 → critical. Low strength
		// close together is low cohesion — the book's big-ball-of-mud corner.
		{
			name: "ball of mud: model/same_module/high",
			c: Classification{
				Strength:   StrengthModel,
				Distance:   DistanceSameModule,
				Volatility: VolatilityHigh,
			},
			wantBalance: 2,
			wantBand:    SeverityCritical,
			wantScored:  true,
		},
		// Same shape one distance rung out: model/cross_module_same_owner/high.
		{
			name: "ball of mud at module seam: model/cross_module_same_owner/high",
			c: Classification{
				Strength:   StrengthModel,
				Distance:   DistanceCrossModuleSameOwner,
				Volatility: VolatilityHigh,
			},
			// |3-4|=1, 10-10=0, max(1,0)+1=2 → critical.
			wantBalance: 2,
			wantBand:    SeverityCritical,
			wantScored:  true,
		},
		// Frozen legacy: S=10(intrusive), D=9(cross_deploy), V=1(frozen).
		// |10-9|=1, 10-1=9, max(1,9)+1=10 → none.
		// Book example: even an intrusive cross-deploy edge into a frozen system is safe.
		{
			name: "frozen legacy: intrusive/cross_deploy/frozen",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityFrozen,
			},
			wantBalance: 10,
			wantBand:    SeverityNone,
			wantScored:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Score(tt.c)
			if got.Scored != tt.wantScored {
				t.Errorf("Scored = %v, want %v", got.Scored, tt.wantScored)
			}
			if got.Balance != tt.wantBalance {
				t.Errorf("Balance = %d, want %d", got.Balance, tt.wantBalance)
			}
			if got.Band != tt.wantBand {
				t.Errorf("Band = %q, want %q", got.Band, tt.wantBand)
			}
			if got.Reason != reasonBook {
				t.Errorf("Reason = %q, want %q", got.Reason, reasonBook)
			}
		})
	}
}

// TestBookScorer_FourCorners covers the key quadrants.
func TestBookScorer_FourCorners(t *testing.T) {
	s := BookScorer{}
	tests := []struct {
		name        string
		c           Classification
		wantBalance int
		wantBand    Severity
		wantScored  bool
	}{
		// Cohesion: high strength at same-module distance — balanced.
		// intrusive/same_module/high: |10-2|=8, max(8,0)+1=9 → none.
		{
			name:        "cohesion same_module",
			c:           Classification{Strength: StrengthIntrusive, Distance: DistanceSameModule, Volatility: VolatilityHigh},
			wantBalance: 9, wantBand: SeverityNone, wantScored: true,
		},
		// Local complexity: low strength at same-module distance — ball of mud.
		// contract/same_module/high: |1-2|=1, max(1,0)+1=2 → critical.
		{
			name:        "local complexity same_module",
			c:           Classification{Strength: StrengthContract, Distance: DistanceSameModule, Volatility: VolatilityHigh},
			wantBalance: 2, wantBand: SeverityCritical, wantScored: true,
		},
		// Distributed monolith: symmetric/cross_deploy/high → 1, critical.
		{
			name:        "distributed monolith",
			c:           Classification{Strength: StrengthSymmetric, Distance: DistanceCrossDeployUnit, Volatility: VolatilityHigh},
			wantBalance: 1, wantBand: SeverityCritical, wantScored: true,
		},
		// Loose: contract/cross_deploy/high → 9, none.
		{
			name:        "loose coupling",
			c:           Classification{Strength: StrengthContract, Distance: DistanceCrossDeployUnit, Volatility: VolatilityHigh},
			wantBalance: 9, wantBand: SeverityNone, wantScored: true,
		},
		// Ball of mud proxy: model/cross_module_same_owner/high → 2, critical.
		{
			name:        "ball of mud",
			c:           Classification{Strength: StrengthModel, Distance: DistanceCrossModuleSameOwner, Volatility: VolatilityHigh},
			wantBalance: 2, wantBand: SeverityCritical, wantScored: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Score(tt.c)
			if got.Scored != tt.wantScored {
				t.Errorf("Scored=%v want %v", got.Scored, tt.wantScored)
			}
			if got.Balance != tt.wantBalance {
				t.Errorf("Balance=%d want %d", got.Balance, tt.wantBalance)
			}
			if got.Band != tt.wantBand {
				t.Errorf("Band=%q want %q", got.Band, tt.wantBand)
			}
		})
	}
}

// TestLocalComplexity verifies the Ch10 local-complexity quadrant membership:
// low strength (contract/model) at same-module distance only.
func TestLocalComplexity(t *testing.T) {
	tests := []struct {
		strength Strength
		distance Distance
		want     bool
	}{
		{StrengthContract, DistanceSameModule, true},
		{StrengthModel, DistanceSameModule, true},
		{StrengthFunctional, DistanceSameModule, false},
		{StrengthSymmetric, DistanceSameModule, false},
		{StrengthIntrusive, DistanceSameModule, false},
		{StrengthContract, DistanceCrossModuleSameOwner, false},
		{StrengthModel, DistanceCrossDeployUnit, false},
	}
	for _, tt := range tests {
		cl := Classification{Strength: tt.strength, Distance: tt.distance}
		if got := LocalComplexity(cl); got != tt.want {
			t.Errorf("LocalComplexity(%s/%s) = %v, want %v", tt.strength, tt.distance, got, tt.want)
		}
	}
}

// TestBookScorer_Abstain verifies edges with unknown strength or distance are not scored.
func TestBookScorer_Abstain(t *testing.T) {
	s := BookScorer{}
	tests := []struct {
		name string
		c    Classification
	}{
		{
			name: "unknown strength",
			c:    Classification{Strength: StrengthUnknown, Distance: DistanceCrossDeployUnit, Volatility: VolatilityHigh},
		},
		{
			name: "unknown distance",
			c:    Classification{Strength: StrengthContract, Distance: DistanceUnknown, Volatility: VolatilityHigh},
		},
		{
			name: "both unknown",
			c:    Classification{Strength: StrengthUnknown, Distance: DistanceUnknown, Volatility: VolatilityUnknown},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Score(tt.c)
			if got.Scored {
				t.Errorf("expected abstain (Scored=false), got Scored=true balance=%d band=%q", got.Balance, got.Band)
			}
			if got.Balance != 0 {
				t.Errorf("Balance=%d, want 0 for abstained edge", got.Balance)
			}
		})
	}
}

// TestBookScorer_DeclaredExternal verifies the D=10 rung (book Ch10 Example 1,
// cross-vendor integration): a declared external system scores with the book
// formula at the distance ladder's far end instead of abstaining.
func TestBookScorer_DeclaredExternal(t *testing.T) {
	s := BookScorer{}
	tests := []struct {
		name        string
		c           Classification
		wantBalance int
		wantBand    Severity
	}{
		// Book Example 1 shape: contract to a stable vendor system.
		// S=1, D=10, V=3: |1-10|=9, 10-3=7, max(9,7)+1=10 → none.
		{
			name:        "contract to stable vendor: contract/declared_external/low",
			c:           Classification{Strength: StrengthContract, Distance: DistanceExternal, Volatility: VolatilityLow},
			wantBalance: 10,
			wantBand:    SeverityNone,
		},
		// Functional call into a stable vendor SDK.
		// S=8, D=10, V=3: |8-10|=2, 10-3=7, max(2,7)+1=8 → low.
		{
			name:        "functional to stable vendor: functional/declared_external/low",
			c:           Classification{Strength: StrengthFunctional, Distance: DistanceExternal, Volatility: VolatilityLow},
			wantBalance: 8,
			wantBand:    SeverityLow,
		},
		// Vendor lock: intrusive coupling to a volatile external system.
		// S=10, D=10, V=10: |10-10|=0, 10-10=0, max(0,0)+1=1 → critical.
		{
			name:        "vendor lock: intrusive/declared_external/high",
			c:           Classification{Strength: StrengthIntrusive, Distance: DistanceExternal, Volatility: VolatilityHigh},
			wantBalance: 1,
			wantBand:    SeverityCritical,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.Score(tt.c)
			if !got.Scored {
				t.Fatalf("expected Scored=true for declared external, got abstain")
			}
			if got.Breakdown.DistanceVal != 10 {
				t.Errorf("Breakdown.DistanceVal = %d, want 10", got.Breakdown.DistanceVal)
			}
			if got.Balance != tt.wantBalance {
				t.Errorf("Balance = %d, want %d", got.Balance, tt.wantBalance)
			}
			if got.Band != tt.wantBand {
				t.Errorf("Band = %q, want %q", got.Band, tt.wantBand)
			}
		})
	}

	// Unknown strength still abstains at declared-external distance —
	// abstain-not-fake is unchanged by the new rung.
	if got := s.Score(Classification{Strength: StrengthUnknown, Distance: DistanceExternal, Volatility: VolatilityLow}); got.Scored {
		t.Errorf("unknown strength at declared_external must abstain, got balance=%d", got.Balance)
	}
}

// TestBookScorer_VolatilityConservative checks that undeclared and unknown
// volatility both score conservatively (treated as V=10, worst case).
func TestBookScorer_VolatilityConservative(t *testing.T) {
	s := BookScorer{}
	// With V=10: vol_rescue=0, so balance depends only on modularity.
	// contract/cross_deploy: S=1, D=9, |1-9|=8, max(8,0)+1=9.
	base := Classification{Strength: StrengthContract, Distance: DistanceCrossDeployUnit}

	high := s.Score(Classification{Strength: base.Strength, Distance: base.Distance, Volatility: VolatilityHigh})
	undecl := s.Score(Classification{Strength: base.Strength, Distance: base.Distance, Volatility: VolatilityUndeclared})
	unk := s.Score(Classification{Strength: base.Strength, Distance: base.Distance, Volatility: VolatilityUnknown})

	if undecl.Balance != high.Balance || undecl.Band != high.Band {
		t.Errorf("undeclared should score like high: undecl balance=%d band=%q, high balance=%d band=%q",
			undecl.Balance, undecl.Band, high.Balance, high.Band)
	}
	if unk.Balance != high.Balance || unk.Band != high.Band {
		t.Errorf("unknown should score like high: unk balance=%d band=%q, high balance=%d band=%q",
			unk.Balance, unk.Band, high.Balance, high.Band)
	}
}

// TestBookScorer_BalanceRange verifies balance stays in [1,10] for all
// concrete strength/distance/volatility combinations, including same-module
// edges (scored at the same-module rung since Wave 5).
func TestBookScorer_BalanceRange(t *testing.T) {
	s := BookScorer{}
	strengths := []Strength{StrengthContract, StrengthModel, StrengthFunctional, StrengthSymmetric, StrengthIntrusive}
	distances := []Distance{DistanceSameModule, DistanceCrossModuleSameOwner, DistanceCrossModuleDiffOwner, DistanceCrossDeployUnit, DistanceExternal}
	vols := []Volatility{VolatilityFrozen, VolatilityLow, VolatilityMedium, VolatilityHigh, VolatilityUndeclared, VolatilityUnknown}

	for _, str := range strengths {
		for _, dist := range distances {
			for _, vol := range vols {
				c := Classification{Strength: str, Distance: dist, Volatility: vol}
				got := s.Score(c)
				if !got.Scored {
					t.Errorf("unexpected abstain: %s/%s/%s", str, dist, vol)
					continue
				}
				if got.Balance < 1 || got.Balance > 10 {
					t.Errorf("balance %d out of [1,10]: %s/%s/%s", got.Balance, str, dist, vol)
				}
				if got.Value != got.Balance {
					t.Errorf("Value=%d != Balance=%d: %s/%s/%s", got.Value, got.Balance, str, dist, vol)
				}
			}
		}
	}
}

// TestBookCheapestMove_NoVolatilityLever locks the Ch11 remediation levers:
// strength and distance are design properties an engineer can change; volatility
// comes from the domain, and the book never sanctions "make the domain more
// stable" as a coupling fix. Sweep every concrete classification — no
// cheapest_move may name volatility (neither lower_volatility nor
// declare_volatility).
func TestBookCheapestMove_NoVolatilityLever(t *testing.T) {
	s := BookScorer{}
	strengths := []Strength{StrengthContract, StrengthModel, StrengthFunctional, StrengthSymmetric, StrengthIntrusive}
	distances := []Distance{DistanceSameModule, DistanceCrossModuleSameOwner, DistanceCrossModuleDiffOwner, DistanceCrossDeployUnit, DistanceExternal}
	vols := []Volatility{VolatilityFrozen, VolatilityLow, VolatilityMedium, VolatilityHigh, VolatilityUndeclared, VolatilityUnknown}

	for _, str := range strengths {
		for _, dist := range distances {
			for _, vol := range vols {
				got := s.Score(Classification{Strength: str, Distance: dist, Volatility: vol})
				switch got.CheapestMove {
				case "", moveReduceStrength, moveReduceDistance:
				default:
					t.Errorf("cheapest move %q for %s/%s/%s — only strength/distance are Ch11 levers",
						got.CheapestMove, str, dist, vol)
				}
			}
		}
	}
}

// TestBookCheapestMove_Cases pins representative lever outcomes, including two
// combinations where the removed volatility lever used to win: they now fall to
// the best remaining strength/distance move, or to no move at all when neither
// single-rung change drops the band.
func TestBookCheapestMove_Cases(t *testing.T) {
	tests := []struct {
		name string
		c    Classification
		want string
	}{
		{
			// |8-9|=1, V=10 → balance 2 (critical). Strength→model: |3-9|=6 → 7 (low).
			name: "strength drop wins",
			c:    Classification{Strength: StrengthFunctional, Distance: DistanceCrossDeployUnit, Volatility: VolatilityHigh},
			want: moveReduceStrength,
		},
		{
			// |10-7|=3, V=10 → balance 4 (high). Distance→same_owner: |10-4|=6 → 7 (low).
			name: "distance drop wins",
			c:    Classification{Strength: StrengthIntrusive, Distance: DistanceCrossModuleDiffOwner, Volatility: VolatilityHigh},
			want: moveReduceDistance,
		},
		{
			// |8-7|=1, V=6 → balance 5 (medium). One-rung strength/distance moves both
			// land on balance 5 again; only the (removed) volatility lever dropped the
			// band. No move offered — honest silence beats an unsanctioned lever.
			name: "formerly lower_volatility → no move",
			c:    Classification{Strength: StrengthFunctional, Distance: DistanceCrossModuleDiffOwner, Volatility: VolatilityMedium},
			want: "",
		},
		{
			// |8-7|=1, V=10 → balance 2 (critical). Formerly declare_volatility (drop 3);
			// now the best sanctioned move: strength→model → balance 5 (medium, drop 2).
			name: "formerly declare_volatility → reduce_strength",
			c:    Classification{Strength: StrengthFunctional, Distance: DistanceCrossModuleDiffOwner, Volatility: VolatilityUndeclared},
			want: moveReduceStrength,
		},
	}

	s := BookScorer{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.Score(tt.c).CheapestMove; got != tt.want {
				t.Errorf("CheapestMove = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDefaultScorer_IsBook verifies DefaultScorer returns BookScorer.
func TestDefaultScorer_IsBook(t *testing.T) {
	s := DefaultScorer()
	if s == nil {
		t.Fatal("DefaultScorer() returned nil")
	}
	got := s.Score(Classification{
		Strength:   StrengthFunctional,
		Distance:   DistanceCrossDeployUnit,
		Volatility: VolatilityHigh,
	})
	if got.Reason != reasonBook {
		t.Errorf("DefaultScorer should be BookScorer; Reason = %q, want %q", got.Reason, reasonBook)
	}
	if !got.Scored {
		t.Errorf("DefaultScorer produced unscored edge for known strength/distance")
	}
}
