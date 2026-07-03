package score

import (
	"strings"
	"testing"
)

// card builds a minimal scorecard with the given coupling_balance value/band.
func card(value int, band Band) Scorecard {
	return Scorecard{
		RubricVersion: RubricVersion,
		Overall:       value,
		OverallBand:   band,
		Dimensions:    []Dimension{{Name: DimCouplingBalance, Value: value, Band: band}},
	}
}

func TestEvaluateCouplingGate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		sc         Scorecard
		gate       CouplingGate
		baseScore  *int
		wantTrip   bool
		wantReason string // substring of one reason; "" = no reason expected
	}{
		{
			name:     "disabled gate never trips",
			sc:       card(5, BandCritical),
			gate:     CouplingGate{Enabled: false, MinBand: BandStrong, MaxDrop: new(0)},
			wantTrip: false,
		},
		{
			name:      "BandNA never gates even with both knobs set",
			sc:        card(0, BandNA),
			gate:      CouplingGate{Enabled: true, MinBand: BandPoor, MaxDrop: new(0)},
			baseScore: new(90),
			wantTrip:  false,
		},
		{
			name:       "band below floor trips",
			sc:         card(39, BandPoor),
			gate:       CouplingGate{Enabled: true, MinBand: BandMixed},
			wantTrip:   true,
			wantReason: `below the configured floor "mixed"`,
		},
		{
			name:     "band equal to floor does not trip",
			sc:       card(50, BandMixed),
			gate:     CouplingGate{Enabled: true, MinBand: BandMixed},
			wantTrip: false,
		},
		{
			name:     "band above floor does not trip",
			sc:       card(85, BandStrong),
			gate:     CouplingGate{Enabled: true, MinBand: BandMixed},
			wantTrip: false,
		},
		{
			name:       "drop over max_drop trips",
			sc:         card(70, BandServiceable),
			gate:       CouplingGate{Enabled: true, MaxDrop: new(5)},
			baseScore:  new(78),
			wantTrip:   true,
			wantReason: "dropped 8 points",
		},
		{
			name:      "drop equal to max_drop does not trip",
			sc:        card(73, BandServiceable),
			gate:      CouplingGate{Enabled: true, MaxDrop: new(5)},
			baseScore: new(78),
			wantTrip:  false,
		},
		{
			name:      "improvement never trips max_drop",
			sc:        card(90, BandStrong),
			gate:      CouplingGate{Enabled: true, MaxDrop: new(0)},
			baseScore: new(78),
			wantTrip:  false,
		},
		{
			name:      "max_drop zero trips on any drop",
			sc:        card(77, BandServiceable),
			gate:      CouplingGate{Enabled: true, MaxDrop: new(0)},
			baseScore: new(78),
			wantTrip:  true,
		},
		{
			name:     "max_drop without baseline score is skipped",
			sc:       card(10, BandCritical),
			gate:     CouplingGate{Enabled: true, MaxDrop: new(0)},
			wantTrip: false,
		},
		{
			// A stored 0 is a measured (terrible) score, distinct from nil
			// (unmeasured): the drop check runs — and a current score ≥0 can
			// never be a drop below 0, so it must not trip.
			name:      "baseline score zero is measured, not unmeasured",
			sc:        card(10, BandCritical),
			gate:      CouplingGate{Enabled: true, MaxDrop: new(0)},
			baseScore: new(0),
			wantTrip:  false,
		},
		{
			name:      "floor and drop both tripped yields two reasons",
			sc:        card(30, BandPoor),
			gate:      CouplingGate{Enabled: true, MinBand: BandMixed, MaxDrop: new(5)},
			baseScore: new(60),
			wantTrip:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := EvaluateCouplingGate(tc.sc, tc.gate, tc.baseScore)
			if got.Tripped != tc.wantTrip {
				t.Fatalf("Tripped = %v, want %v (reasons: %v)", got.Tripped, tc.wantTrip, got.Reasons)
			}
			if !tc.wantTrip && len(got.Reasons) != 0 {
				t.Fatalf("no trip but reasons = %v", got.Reasons)
			}
			if tc.wantTrip && len(got.Reasons) == 0 {
				t.Fatal("tripped but no reasons")
			}
			if tc.wantReason != "" && !strings.Contains(strings.Join(got.Reasons, "\n"), tc.wantReason) {
				t.Fatalf("reasons %v missing %q", got.Reasons, tc.wantReason)
			}
		})
	}
	t.Run("both knobs tripped emits one reason per knob", func(t *testing.T) {
		t.Parallel()
		got := EvaluateCouplingGate(card(30, BandPoor),
			CouplingGate{Enabled: true, MinBand: BandMixed, MaxDrop: new(5)}, new(60))
		if len(got.Reasons) != 2 {
			t.Fatalf("reasons = %v, want 2 entries", got.Reasons)
		}
	})
}
