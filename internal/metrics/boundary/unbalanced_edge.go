package boundary

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/metrics/internal/result"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/signal"
)

// ---------------------------------------------------------------------------
// Distance rank for ordered comparison (shared by unbalanced_edge + change_locality)
// ---------------------------------------------------------------------------

// distanceRank maps a Distance to a numeric rank for >= comparisons.
// Higher rank = greater distance. Unknown returns -1.
func distanceRank(d coupling.Distance) int {
	switch d {
	case coupling.DistanceSameModule:
		return 0
	case coupling.DistanceCrossModuleSameOwner:
		return 1
	case coupling.DistanceCrossModuleDiffOwner:
		return 2
	case coupling.DistanceCrossDeployUnit:
		return 3
	default: // DistanceUnknown
		return -1
	}
}

// ---------------------------------------------------------------------------
// UnbalancedEdgeMetric (unbalanced_edge.v2)
// ---------------------------------------------------------------------------

// UnbalancedEdgeMetric counts edges where strength=intrusive AND
// distance>=cross_module_different_owner AND volatility=high (spec §10.4).
// The primary value is the count of new_high edges.
type UnbalancedEdgeMetric struct{}

// Name returns "unbalanced_edge".
func (m UnbalancedEdgeMetric) Name() string { return "unbalanced_edge" }

// Version returns "unbalanced_edge.v2".
func (m UnbalancedEdgeMetric) Version() string { return "unbalanced_edge.v2" }

// Calculate counts high-risk unbalanced edges and cross-references findings for status.
func (m UnbalancedEdgeMetric) Calculate(in signal.CommonInput) diagnostic.MetricResult {
	// Build an edge-key → finding status index keyed by (from-path, to-path).
	// finding.EdgeEvidence carries stripped paths (kind prefix removed);
	// coupling.Index keys use full node IDs. We strip the kind prefix when
	// looking up the finding status.
	type pathPair struct{ from, to string }
	findingStatus := make(map[pathPair]finding.Status)
	for _, f := range in.Findings {
		pp := pathPair{from: f.Edge.From.Path, to: f.Edge.To.Path}
		// Keep the highest-priority status when multiple findings share the same pair.
		existing, exists := findingStatus[pp]
		if !exists || statusPriority(f.Status) > statusPriority(existing) {
			findingStatus[pp] = f.Status
		}
	}

	var newHigh, candidates, candidatesKnownVol int
	crossModuleDiffOwnerRank := distanceRank(coupling.DistanceCrossModuleDiffOwner)

	for _, e := range in.Graph.Edges() {
		if e.Kind != graph.EdgeKindImports &&
			e.Kind != graph.EdgeKindDependsOn &&
			e.Kind != graph.EdgeKindUsesInternal {
			continue
		}
		key := e.From + "\x00" + e.To + "\x00" + string(e.Kind)
		cl, ok := in.Classifications[key]
		if !ok {
			continue
		}
		// High-risk: intrusive AND distance>=cross_module_different_owner AND high volatility.
		if cl.Strength != coupling.StrengthIntrusive {
			continue
		}
		if distanceRank(cl.Distance) < crossModuleDiffOwnerRank {
			continue
		}
		// This edge is a candidate (intrusive + far). Whether it is *unbalanced*
		// turns on volatility — track how many candidates we can actually assess.
		// Both unknown (unresolvable) and undeclared (config gap) count as
		// unassessable here.
		candidates++
		if coupling.VolatilityResolved(cl.Volatility) {
			candidatesKnownVol++
		}
		if cl.Volatility != coupling.VolatilityHigh {
			continue
		}

		// Determine status via finding index.
		pp := pathPair{from: graph.NodePath(e.From), to: graph.NodePath(e.To)}
		st := findingStatus[pp] // zero value "" means no matching finding → treat as new
		if st == finding.StatusNew || st == "" {
			newHigh++
		}
	}

	// Candidates exist but none has a known volatility → the high-volatility test
	// cannot be evaluated, so the count is indeterminate, not a clean zero. Report
	// n/a rather than a false "strong" (same discipline as encapsulation: absence of
	// evidence is not evidence of balance). No candidates at all → genuine 0/strong.
	if candidates > 0 && candidatesKnownVol == 0 {
		return m.naResult()
	}

	value := float64(newHigh)
	confidence := result.ConfidenceHigh
	var band string
	if newHigh == 0 {
		band = result.BandStrong
	} else {
		band = result.BandCritical
	}
	band = result.ApplyConfidenceCap(band, confidence)

	display := fmt.Sprintf("%d new high-risk unbalanced edges", newHigh)
	delta := result.ComputeDelta(value, in.Baseline, m.Name(), m.Version())

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: "intrusive edges with cross-module ownership and high volatility",
		Delta:      delta,
	}
}

// naResult reports unbalanced_edge as indeterminate: intrusive cross-module
// candidate edges exist, but none has a known volatility, so whether any is
// unbalanced cannot be decided. Band is result.BandNA (not strong), Delta nil.
func (m UnbalancedEdgeMetric) naResult() diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      0,
		Display:    result.BandNA,
		Band:       result.BandNA,
		Confidence: result.ConfidenceLow,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: "intrusive edges with cross-module ownership and high volatility",
		Delta:      nil,
	}
}

// statusPriority returns a priority number for finding status; higher = more important to surface.
func statusPriority(s finding.Status) int {
	switch s {
	case finding.StatusNew:
		return 4
	case finding.StatusExpiredWaiver:
		return 3
	case finding.StatusWaived:
		return 2
	case finding.StatusBaseline:
		return 1
	default:
		return 0
	}
}
