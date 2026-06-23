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
		// Transactional cohesion: S=8(functional), D=2(same_module), V=10(high).
		// same_module is the cohesion guard → balance=10, band=none.
		{
			name: "transactional cohesion: functional/same_module/high",
			c: Classification{
				Strength:   StrengthFunctional,
				Distance:   DistanceSameModule,
				Volatility: VolatilityHigh,
			},
			wantBalance: 10,
			wantBand:    SeverityNone,
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
		// Ball of mud: S=3(model), D=2(same_module), V=10(high).
		// same_module guard → balance=10, band=none.
		// (The book's "ball of mud" example uses S near D at close distance;
		//  with same_module the cohesion guard fires. Test cross_module_same_owner instead.)
		{
			name: "ball of mud approximation: model/cross_module_same_owner/high",
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
		// Frozen legacy: S=10(intrusive), D=9(cross_deploy), V=3(low≈supporting).
		// VolatilityLow maps to V=3 (not V=1; frozen/legacy anchor added in Task 5).
		// |10-9|=1, 10-3=7, max(1,7)+1=8 → low.
		// (True frozen/legacy V=1 would yield balance=10; Task 5 adds that constant.)
		{
			name: "frozen legacy approx (VolatilityLow→V=3): intrusive/cross_deploy/low",
			c: Classification{
				Strength:   StrengthIntrusive,
				Distance:   DistanceCrossDeployUnit,
				Volatility: VolatilityLow,
			},
			wantBalance: 8,
			wantBand:    SeverityLow,
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
		// Same-module cohesion: any strength/vol → balance=10, none.
		{
			name:        "cohesion same_module",
			c:           Classification{Strength: StrengthContract, Distance: DistanceSameModule, Volatility: VolatilityHigh},
			wantBalance: 10, wantBand: SeverityNone, wantScored: true,
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
// concrete strength/distance/volatility combinations.
func TestBookScorer_BalanceRange(t *testing.T) {
	s := BookScorer{}
	strengths := []Strength{StrengthContract, StrengthModel, StrengthFunctional, StrengthSymmetric, StrengthIntrusive}
	distances := []Distance{DistanceSameModule, DistanceCrossModuleSameOwner, DistanceCrossModuleDiffOwner, DistanceCrossDeployUnit}
	vols := []Volatility{VolatilityLow, VolatilityMedium, VolatilityHigh, VolatilityUndeclared, VolatilityUnknown}

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
