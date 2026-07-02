package score

import "fmt"

// CouplingGate is the projected `coupling.gate:` config view (built by
// couplingGateView in cmd — config sits below score in the layer order, so
// this projection cannot be a Config method). Enabled is true when the block
// is present; MinBand and MaxDrop are independent knobs — either may be unset.
type CouplingGate struct {
	Enabled bool
	// MinBand is the band floor: the gate trips when the current
	// coupling_balance band ranks below it. Empty = no floor check.
	MinBand Band
	// MaxDrop is the tolerated point drop of the coupling_balance value against
	// the baseline-stored score. nil = no drop check; 0 = any drop trips.
	MaxDrop *int
}

// CouplingGateTrip is the outcome of EvaluateCouplingGate. Reasons holds one
// human-readable line per tripped knob (empty when not tripped).
type CouplingGateTrip struct {
	Tripped bool
	Reasons []string
}

// EvaluateCouplingGate decides whether the coupling_balance dimension trips the
// verdict gate. Pure decision over the synthesised scorecard:
//
//   - gate disabled (no coupling.gate block) → never trips.
//   - BandNA never gates: an unmeasured dimension is an abstention, not a
//     failure — partial-coverage runs must not flip CI red.
//   - MinBand: trips when the overall band ranks below the configured floor.
//   - MaxDrop: trips when baselineScore − current overall exceeds it.
//     baselineScore is the coupling_balance value stored at `archfit baseline`
//     time; nil (no baseline, or baseline recorded while unmeasured) means the
//     drop cannot be anchored, so the check is skipped.
func EvaluateCouplingGate(sc Scorecard, gate CouplingGate, baselineScore *int) CouplingGateTrip {
	if !gate.Enabled || sc.OverallBand.Unmeasured() {
		return CouplingGateTrip{}
	}
	var trip CouplingGateTrip
	if gate.MinBand != "" && BandRank(sc.OverallBand) < BandRank(gate.MinBand) {
		trip.Tripped = true
		trip.Reasons = append(trip.Reasons, fmt.Sprintf(
			"coupling_balance band %q (score %d) is below the configured floor %q (coupling.gate.min_band)",
			sc.OverallBand, sc.Overall, gate.MinBand))
	}
	if gate.MaxDrop != nil && baselineScore != nil {
		if drop := *baselineScore - sc.Overall; drop > *gate.MaxDrop {
			trip.Tripped = true
			trip.Reasons = append(trip.Reasons, fmt.Sprintf(
				"coupling_balance dropped %d points (baseline %d → current %d), exceeding coupling.gate.max_drop %d",
				drop, *baselineScore, sc.Overall, *gate.MaxDrop))
		}
	}
	return trip
}
