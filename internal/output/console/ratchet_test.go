package console

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
)

const (
	metricCoverage   = "coverage"
	ratchetSection   = "METRIC RATCHET"
	baselineRef      = "baseline"
	dimensionsHeader = "\nDIMENSIONS"
)

// ratchetDoc is a run a metric ratchet blocked: verdict blocked, no active
// blocker, no required analyzer failing its gate, and one metric worse than the
// accepted baseline. The other two metrics did not worsen or were never
// compared, so neither may be named.
func ratchetDoc() report.Document {
	d := report.NewDocument()
	d.State.Verdict = report.StateBlocked
	d.State.Decision.HardGates = report.HardGateFail
	d.State.Dimensions.Operations.Gate = report.GateFail
	d.State.Dimensions.Operations.Metrics = []report.MetricValue{{Name: metricCoverage, Value: 0.75, Unit: "ratio"}}
	d.State.GateReference = &report.StateComparison{Status: report.ComparisonComparable, BaseRef: baselineRef, Reasons: []string{}}
	worse, flat := -0.25, 0.0
	d.Metrics = []report.MetricResult{
		{Name: metricCoverage, Value: 0.75, Delta: &worse, Direction: report.DirectionHigherIsBetter},
		{Name: "unbalanced_edge", Value: 4, Delta: &flat, Direction: report.DirectionHigherIsWorse},
		{Name: "cycle", Value: 2, Direction: report.DirectionHigherIsWorse},
	}
	return d
}

func renderDoc(t *testing.T, d report.Document) string {
	t.Helper()
	var b strings.Builder
	if err := New().Render(d, &b); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return b.String()
}

// ratchetBlock returns the METRIC RATCHET section, or "" when there is none.
func ratchetBlock(out string) string {
	start := strings.Index(out, ratchetSection)
	if start < 0 {
		return ""
	}
	block := out[start:]
	if end := strings.Index(block, dimensionsHeader); end >= 0 {
		block = block[:end]
	}
	return block
}

// TestRender_NamesTheRatchetThatBlocked pins the human half of a ratchet block:
// a tripped ratchet produces no finding and no task, so the text must name the
// metric, its accepted-baseline and current values, the gate it failed, the
// gate reference it was compared against, and the next step.
func TestRender_NamesTheRatchetThatBlocked(t *testing.T) {
	block := ratchetBlock(renderDoc(t, ratchetDoc()))
	for _, want := range []string{
		"worsened against the accepted baseline",
		"coverage: 1 → 0.75",
		"operations gate: fail",
		"gate reference: comparable  ·  reference: baseline",
		"archfit baseline",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("ratchet section is missing %q:\n%s", want, block)
		}
	}
	for _, unwanted := range []string{"unbalanced_edge", "cycle"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("ratchet section names %q, which did not worsen:\n%s", unwanted, block)
		}
	}
}

// gateRef is an active hard-gate finding reference routed to a dimension.
func gateRef(id string) report.FindingRef {
	return report.FindingRef{ID: id, RuleID: "no_" + id, Kind: report.FindingKindGate, Severity: "high", Status: "new"}
}

// TestRender_RatchetNeedsAContractProof pins the abstention: the section is
// printed only where the contract proves a ratchet blocked. A dimension gate
// fails from a ratchet, from a hard-gate finding routed to it, or (operations
// only) from a required analyzer that failed its gate; where the latter two
// can explain the failing gate, and when the run is not blocked, nothing is
// proven.
func TestRender_RatchetNeedsAContractProof(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*report.Document)
	}{
		{"a blocker routed to the metric's dimension explains its gate", func(d *report.Document) {
			d.State.Decision.ActiveBlockers = 1
			d.State.Dimensions.Operations.Findings = []report.FindingRef{gateRef("deploy")}
		}},
		{"a required analyzer failed its gate", func(d *report.Document) {
			d.CoverageGaps = []report.CoverageGap{{Tool: "go/packages", Gate: string(report.GateFail)}}
		}},
		{"the run is not blocked", func(d *report.Document) {
			d.State.Verdict = report.StateNeedsAttention
			d.State.Decision.HardGates = report.HardGatePass
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := ratchetDoc()
			tc.mutate(&d)
			if block := ratchetBlock(renderDoc(t, d)); block != "" {
				t.Errorf("ratchet section printed without proof:\n%s", block)
			}
		})
	}
}

// TestRender_RatchetShownBesideOtherBlockers pins the proof that survives other
// blockers: a dimension whose gate fails with no hard-gate finding routed to it
// (and, for operations, no required analyzer failing) failed from a ratchet.
// Only the worsened metrics that dimension owns are named; a worsened metric in
// a dimension a blocker explains is not.
func TestRender_RatchetShownBesideOtherBlockers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*report.Document)
	}{
		{"a blocker in another dimension", func(d *report.Document) {
			d.State.Decision.ActiveBlockers = 1
			d.State.Dimensions.Structure.Gate = report.GateFail
			d.State.Dimensions.Structure.Findings = []report.FindingRef{gateRef("cycle")}
		}},
		{"a required analyzer failed and the ratchet is outside operations", func(d *report.Document) {
			d.CoverageGaps = []report.CoverageGap{{Tool: "dependency-cruiser", Gate: string(report.GateFail)}}
			d.State.Dimensions.Complexity.Gate = report.GateFail
			d.State.Dimensions.Complexity.Metrics = d.State.Dimensions.Operations.Metrics
			d.State.Dimensions.Operations.Metrics = nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := ratchetDoc()
			worse := 3.0
			d.Metrics = append(d.Metrics, report.MetricResult{Name: "max_cyclomatic", Value: 9, Delta: &worse, Direction: report.DirectionHigherIsWorse})
			d.State.Dimensions.Structure.Metrics = []report.MetricValue{{Name: "max_cyclomatic", Value: 9}}
			tc.mutate(&d)
			block := ratchetBlock(renderDoc(t, d))
			if !strings.Contains(block, metricCoverage) {
				t.Errorf("ratchet section does not name %q:\n%s", metricCoverage, block)
			}
			if strings.Contains(block, "max_cyclomatic") {
				t.Errorf("ratchet section names a metric whose dimension a blocker or nothing explains:\n%s", block)
			}
		})
	}
}

// TestRender_RatchetDisclosesANonComparableReference pins the reference status:
// a ratchet compared against a reference written under other inputs must say
// so, with the reason, next to the numbers it blocked on.
func TestRender_RatchetDisclosesANonComparableReference(t *testing.T) {
	d := ratchetDoc()
	d.State.GateReference = &report.StateComparison{
		Status: report.ComparisonNonComparable, BaseRef: baselineRef, Reasons: []string{"config_hash differs from the stored reference"},
	}
	block := ratchetBlock(renderDoc(t, d))
	for _, want := range []string{"gate reference: non_comparable", "config_hash differs from the stored reference"} {
		if !strings.Contains(block, want) {
			t.Errorf("ratchet section is missing %q:\n%s", want, block)
		}
	}
}
