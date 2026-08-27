package state

// DimensionSignal is everything the verdict may know about one dimension: was
// its evidence gathered, and did its gate fire.
//
// It is deliberately not the envelope. The aggregator physically cannot reach a
// metric, a coverage count, or a finding's text, so "the decision is
// metric-blind" is a property of the type signature rather than a rule someone
// has to keep remembering.
type DimensionSignal struct {
	Status MeasurementStatus
	Gate   GateState
}

// Signals projects the nine envelopes down to what the aggregator may read, in
// contract order.
func (d Dimensions) Signals() []DimensionSignal {
	all := d.All()
	out := make([]DimensionSignal, 0, len(all))
	for _, dim := range all {
		out = append(out, DimensionSignal{Status: dim.Status, Gate: dim.Gate})
	}
	return out
}

// DecisionInput is the complete input to the architecture verdict: the explicit
// hard-gate result, the two active finding populations as counts, and the
// per-dimension signals. Nothing else may influence the decision.
type DecisionInput struct {
	// HardGates is the repository hard-gate result the classifier already
	// decided. Unmeasured means no gate was evaluated, which can never be
	// healthy.
	HardGates HardGateState
	// ActiveBlockers counts active hard-gate findings. A required-tool policy
	// failure blocks without one, which is why HardGates is carried separately.
	ActiveBlockers int
	// ActiveDiagnostics counts active advisories.
	ActiveDiagnostics int
	Dimensions        []DimensionSignal
}

// Decide is the architecture verdict and its explicit counters.
//
// The three rules are exhaustive and read in order:
//
//   - blocked: the hard gates failed — an active hard-gate finding, or a
//     reportable required-tool policy failure.
//   - needs_attention: no blocker, but at least one active diagnostic exists or
//     at least one dimension is partial/unmeasured. Missing evidence must never
//     become a green result by omission.
//   - healthy: the hard gates passed, nothing is active, and all nine
//     dimensions were measured.
//
// No averaged score, band, or threshold participates. A dimension's metrics are
// not reachable from here.
func Decide(in DecisionInput) (Verdict, Decision) {
	attention, unknown := 0, 0
	for _, sig := range in.Dimensions {
		if sig.Gate == GateWarn || sig.Gate == GateFail {
			attention++
		}
		if sig.Status != Measured {
			unknown++
		}
	}
	decision := Decision{
		HardGates:           in.HardGates,
		ActiveBlockers:      in.ActiveBlockers,
		AttentionDimensions: attention,
		UnknownDimensions:   unknown,
	}
	switch {
	case in.HardGates == HardGateFail:
		return Blocked, decision
	case in.ActiveDiagnostics > 0 || unknown > 0 || in.HardGates != HardGatePass:
		return NeedsAttention, decision
	default:
		return Healthy, decision
	}
}
