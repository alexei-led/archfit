// Behavior tests for the architecture verdict. They pin the three fixtures the
// migration is defined by — healthy, needs attention, blocked — and the rule
// that missing evidence can never become a green result.
package state_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/state"
)

// measuredSignals is nine measured, passing dimensions: the only shape from
// which healthy is reachable at all.
func measuredSignals() []state.DimensionSignal {
	out := make([]state.DimensionSignal, state.DimensionCount)
	for i := range out {
		out[i] = state.DimensionSignal{Status: state.Measured, Gate: state.GatePass}
	}
	return out
}

func signalsWith(index int, sig state.DimensionSignal) []state.DimensionSignal {
	out := measuredSignals()
	out[index] = sig
	return out
}

// TestDecide is the verdict table. Blocked wins over everything; healthy needs
// nine measured dimensions, a passing hard gate, and nothing active.
func TestDecide(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input state.DecisionInput
		want  state.Verdict
	}{
		{
			name:  "everything measured, nothing active",
			input: state.DecisionInput{HardGates: state.HardGatePass, Dimensions: measuredSignals()},
			want:  state.Healthy,
		},
		{
			name: "an active hard-gate finding blocks",
			input: state.DecisionInput{HardGates: state.HardGateFail, ActiveBlockers: 1,
				Dimensions: signalsWith(1, state.DimensionSignal{Status: state.Measured, Gate: state.GateFail})},
			want: state.Blocked,
		},
		{
			name: "a required-tool failure blocks without a finding",
			input: state.DecisionInput{HardGates: state.HardGateFail,
				Dimensions: signalsWith(7, state.DimensionSignal{Status: state.Partial, Gate: state.GateFail})},
			want: state.Blocked,
		},
		{
			name: "an active diagnostic needs attention",
			input: state.DecisionInput{HardGates: state.HardGatePass, ActiveDiagnostics: 1,
				Dimensions: signalsWith(3, state.DimensionSignal{Status: state.Measured, Gate: state.GateWarn})},
			want: state.NeedsAttention,
		},
		{
			name: "one partial dimension needs attention",
			input: state.DecisionInput{HardGates: state.HardGatePass,
				Dimensions: signalsWith(4, state.DimensionSignal{Status: state.Partial, Gate: state.GatePass})},
			want: state.NeedsAttention,
		},
		{
			name: "one unmeasured dimension needs attention",
			input: state.DecisionInput{HardGates: state.HardGatePass,
				Dimensions: signalsWith(8, state.DimensionSignal{Status: state.Unmeasured, Gate: state.GateNotApplicable})},
			want: state.NeedsAttention,
		},
		{
			name:  "unevaluated hard gates can never be healthy",
			input: state.DecisionInput{HardGates: state.HardGateUnmeasured, Dimensions: measuredSignals()},
			want:  state.NeedsAttention,
		},
		{
			name: "blocked wins over an unmeasured dimension",
			input: state.DecisionInput{HardGates: state.HardGateFail, ActiveBlockers: 3, ActiveDiagnostics: 4,
				Dimensions: signalsWith(2, state.DimensionSignal{Status: state.Unmeasured})},
			want: state.Blocked,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verdict, decision := state.Decide(tc.input)
			if verdict != tc.want {
				t.Errorf("verdict = %q, want %q", verdict, tc.want)
			}
			if decision.HardGates != tc.input.HardGates {
				t.Errorf("hard gates = %q, want the classified result %q", decision.HardGates, tc.input.HardGates)
			}
			if decision.ActiveBlockers != tc.input.ActiveBlockers {
				t.Errorf("active blockers = %d, want %d", decision.ActiveBlockers, tc.input.ActiveBlockers)
			}
		})
	}
}

// TestDecideCounters pins the two counters apart: attention counts dimensions
// whose gate fired, unknown counts dimensions whose evidence is incomplete. A
// warning dimension that measured everything is not unknown, and an unmeasured
// dimension with no gate is not an attention item.
func TestDecideCounters(t *testing.T) {
	t.Parallel()
	signals := measuredSignals()
	signals[0] = state.DimensionSignal{Status: state.Measured, Gate: state.GateWarn}
	signals[1] = state.DimensionSignal{Status: state.Measured, Gate: state.GateFail}
	signals[2] = state.DimensionSignal{Status: state.Partial, Gate: state.GatePass}
	signals[3] = state.DimensionSignal{Status: state.Unmeasured, Gate: state.GateNotApplicable}

	_, decision := state.Decide(state.DecisionInput{HardGates: state.HardGateFail, ActiveBlockers: 1, Dimensions: signals})
	if decision.AttentionDimensions != 2 {
		t.Errorf("attention dimensions = %d, want 2 (one warn, one fail)", decision.AttentionDimensions)
	}
	if decision.UnknownDimensions != 2 {
		t.Errorf("unknown dimensions = %d, want 2 (one partial, one unmeasured)", decision.UnknownDimensions)
	}
}

// TestSignalsProjectsInContractOrder pins that what the aggregator reads is the
// nine envelopes' status and gate, in contract order, and nothing else.
func TestSignalsProjectsInContractOrder(t *testing.T) {
	t.Parallel()
	st := state.New()
	st.Dimensions.Coupling.Status = state.Partial
	st.Dimensions.Coupling.Gate = state.GateWarn

	signals := st.Dimensions.Signals()
	if len(signals) != state.DimensionCount {
		t.Fatalf("signals = %d, want %d", len(signals), state.DimensionCount)
	}
	for i, dim := range st.Dimensions.All() {
		if signals[i].Status != dim.Status || signals[i].Gate != dim.Gate {
			t.Errorf("signal %d (%s) = %+v, want (%q, %q)", i, dim.Name, signals[i], dim.Status, dim.Gate)
		}
	}
}
