package boundary

import (
	"fmt"

	assessmentresult "github.com/alexei-led/archfit/v3/internal/assessment/result"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/metrics/internal/result"
	signal "github.com/alexei-led/archfit/v3/internal/assessment/signals"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

// isModuleBoundary reports whether a distance token names a module boundary.
// Under bc_score.v7 every boundary sits at the same rung (D=9), so owner and
// deploy unit no longer separate "far" edges from "near" ones. Declared external
// systems stay out of this metric, as before.
func isModuleBoundary(d relationship.Distance) bool {
	switch d {
	case relationship.DistanceCrossModule, relationship.DistanceCrossModuleDiffOwner, relationship.DistanceCrossDeployUnit:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// UnbalancedEdgeMetric (unbalanced_edge.v3)
// ---------------------------------------------------------------------------

// UnbalancedEdgeMetric counts edges where strength=intrusive AND
// distance=any module boundary AND volatility=high (spec §10.4).
// The primary value is the count of new_high edges.
type UnbalancedEdgeMetric struct{}

// Name returns "unbalanced_edge".
func (m UnbalancedEdgeMetric) Name() string { return "unbalanced_edge" }

// Version returns "unbalanced_edge.v3".
func (m UnbalancedEdgeMetric) Version() string { return "unbalanced_edge.v3" }

// Calculate counts high-risk unbalanced edges and cross-references findings for status.
func (m UnbalancedEdgeMetric) Calculate(in signal.CommonInput) assessmentresult.MetricResult {
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

	for _, e := range in.Relationships.DependencyEdges() {
		// High-risk: intrusive AND a module boundary AND high volatility.
		if e.Strength != relationship.StrengthIntrusive {
			continue
		}
		if !isModuleBoundary(e.Distance) {
			continue
		}
		// This edge is a candidate (intrusive + far). Whether it is *unbalanced*
		// turns on volatility — track how many candidates we can actually assess.
		// Both unknown (unresolvable) and undeclared (config gap) count as
		// unassessable here.
		candidates++
		if relationship.VolatilityResolved(e.Volatility) {
			candidatesKnownVol++
		}
		if e.Volatility != relationship.VolatilityHigh {
			continue
		}

		// Determine status via finding index.
		pp := pathPair{from: e.FromPath, to: e.ToPath}
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

	return assessmentresult.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: "intrusive edges across a module boundary into a high-volatility target",
		Delta:      delta,
		Direction:  assessmentresult.DirectionHigherIsWorse,
	}
}

// naResult reports unbalanced_edge as indeterminate: intrusive cross-module
// candidate edges exist, but none has a known volatility, so whether any is
// unbalanced cannot be decided. Band is result.BandNA (not strong), Delta nil.
func (m UnbalancedEdgeMetric) naResult() assessmentresult.MetricResult {
	return assessmentresult.MetricResult{
		Name:       m.Name(),
		Value:      0,
		Display:    result.BandNA,
		Band:       result.BandNA,
		Confidence: result.ConfidenceLow,
		Version:    m.Version(),
		Mode:       result.ModeCount,
		Definition: "intrusive edges across a module boundary into a high-volatility target",
		Delta:      nil,
		Direction:  assessmentresult.DirectionHigherIsWorse,
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
