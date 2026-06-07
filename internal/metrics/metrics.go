package metrics

import (
	"fmt"
	"strings"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
)

// Band name constants (spec §10.1).
const (
	bandStrong      = "strong"
	bandServiceable = "serviceable"
	bandMixed       = "mixed"
	bandPoor        = "poor"
	bandCritical    = "critical"
)

// Confidence level constants.
const (
	confidenceHigh   = "high"
	confidenceMedium = "medium"
	confidenceLow    = "low"
)

// Metric is the interface every Phase 1 metric implements.
// Name returns the bare metric name (e.g. "encapsulation").
// Version returns the full version string (e.g. "encapsulation.v1").
// Calculate computes the metric from the provided input.
type Metric interface {
	Name() string
	Version() string
	Calculate(in MetricInput) diagnostic.MetricResult
}

// MetricInput is the complete input set for all metrics.
type MetricInput struct {
	Graph           *graph.Graph
	Classifications coupling.Index
	Findings        []finding.Finding // status-tagged findings from status stage
	Baseline        diagnostic.MetricSnapshot
	ToolCoverage    []diagnostic.Coverage // from extractors
}

// ---------------------------------------------------------------------------
// Band model (spec §10.1)
// ---------------------------------------------------------------------------

// bandScore maps a 0–10 score to a band name.
//
//	strong:      9.0–10.0
//	serviceable: 7.0–8.9
//	mixed:       5.0–6.9
//	poor:        3.0–4.9
//	critical:    0.0–2.9
func bandScore(score float64) string {
	switch {
	case score >= 9.0:
		return bandStrong
	case score >= 7.0:
		return bandServiceable
	case score >= 5.0:
		return bandMixed
	case score >= 3.0:
		return bandPoor
	default:
		return bandCritical
	}
}

// bandRank maps a band name to a comparable rank (higher = better).
func bandRank(band string) int {
	switch band {
	case bandStrong:
		return 4
	case bandServiceable:
		return 3
	case bandMixed:
		return 2
	case bandPoor:
		return 1
	default: // bandCritical
		return 0
	}
}

// bandByRank maps a rank back to a band name.
func bandByRank(rank int) string {
	switch rank {
	case 4:
		return bandStrong
	case 3:
		return bandServiceable
	case 2:
		return bandMixed
	case 1:
		return bandPoor
	default:
		return bandCritical
	}
}

// confidenceCapRank returns the maximum allowed band rank for a given confidence level.
//
//	high:   max "strong"      (rank 4)
//	medium: max "serviceable" (rank 3)
//	low:    max "mixed"       (rank 2)
func confidenceCapRank(confidence string) int {
	switch confidence {
	case confidenceHigh:
		return 4
	case confidenceMedium:
		return 3
	default: // confidenceLow or unknown
		return 2
	}
}

// applyConfidenceCap clamps band to the maximum allowed by confidence.
func applyConfidenceCap(band, confidence string) string {
	computed := bandRank(band)
	maxRank := confidenceCapRank(confidence)
	if computed > maxRank {
		return bandByRank(maxRank)
	}
	return band
}

// ---------------------------------------------------------------------------
// Delta helper
// ---------------------------------------------------------------------------

// computeDelta returns the delta between current and baseline for the given
// metric name and version. Returns nil if no matching baseline entry exists.
func computeDelta(current float64, baseline diagnostic.MetricSnapshot, name, version string) *float64 {
	if baseline == nil {
		return nil
	}
	snap, ok := baseline[name]
	if !ok || snap.Version != version {
		return nil
	}
	d := current - snap.Value
	return &d
}

// ---------------------------------------------------------------------------
// Distance rank for ordered comparison
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
// EncapsulationMetric (encapsulation.v1)
// ---------------------------------------------------------------------------

// EncapsulationMetric measures the ratio of contract cross-boundary edges
// to all cross-boundary edges (spec §10.4).
//
//	value = contract_cross_boundary / all_cross_boundary
//	If denominator == 0, value = 1.0 (perfect encapsulation — no cross-boundary edges).
type EncapsulationMetric struct{}

// Name returns "encapsulation".
func (m EncapsulationMetric) Name() string { return "encapsulation" }

// Version returns "encapsulation.v1".
func (m EncapsulationMetric) Version() string { return "encapsulation.v1" }

// Calculate computes the encapsulation ratio from cross-boundary edge classifications.
func (m EncapsulationMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	if in.Graph == nil {
		return m.result(1.0, confidenceHigh, in.Baseline)
	}

	var allCross, contractCross int
	for _, e := range in.Graph.Edges() {
		// Only dependency-type edges contribute to encapsulation measurement.
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
		// Cross-boundary: any distance that is not same_module.
		if cl.Distance == coupling.DistanceSameModule || cl.Distance == coupling.DistanceUnknown {
			continue
		}
		allCross++
		if cl.Strength == coupling.StrengthContract {
			contractCross++
		}
	}

	var value float64
	if allCross == 0 {
		value = 1.0
	} else {
		value = float64(contractCross) / float64(allCross)
	}

	return m.result(value, confidenceHigh, in.Baseline)
}

func (m EncapsulationMetric) result(value float64, confidence string, baseline diagnostic.MetricSnapshot) diagnostic.MetricResult {
	score := value * 10.0
	band := applyConfidenceCap(bandScore(score), confidence)
	delta := computeDelta(value, baseline, m.Name(), m.Version())
	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    fmt.Sprintf("%.1f/10", score),
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       "ratio",
		Definition: "contract_cross_boundary / all_cross_boundary",
		Delta:      delta,
	}
}

// ---------------------------------------------------------------------------
// UnbalancedEdgeMetric (unbalanced_edge.v1)
// ---------------------------------------------------------------------------

// UnbalancedEdgeMetric counts edges where strength=intrusive AND
// distance>=cross_module_different_owner AND volatility=high (spec §10.4).
// The primary value is the count of new_high edges.
type UnbalancedEdgeMetric struct{}

// Name returns "unbalanced_edge".
func (m UnbalancedEdgeMetric) Name() string { return "unbalanced_edge" }

// Version returns "unbalanced_edge.v1".
func (m UnbalancedEdgeMetric) Version() string { return "unbalanced_edge.v1" }

// Calculate counts high-risk unbalanced edges and cross-references findings for status.
func (m UnbalancedEdgeMetric) Calculate(in MetricInput) diagnostic.MetricResult {
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

	var newHigh, baselineHigh, exceptedHigh, expiredHigh int
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
		if cl.Volatility != coupling.VolatilityHigh {
			continue
		}

		// Determine status via finding index.
		fromPath := stripKindPrefix(e.From)
		toPath := stripKindPrefix(e.To)
		pp := pathPair{from: fromPath, to: toPath}
		st := findingStatus[pp] // zero value "" means no matching finding → treat as new

		switch st {
		case finding.StatusNew, "":
			newHigh++
		case finding.StatusBaseline:
			baselineHigh++
		case finding.StatusExcepted:
			exceptedHigh++
		case finding.StatusExpiredExcept:
			expiredHigh++
		}
	}

	value := float64(newHigh)
	confidence := confidenceHigh
	var band string
	if newHigh == 0 {
		band = bandStrong
	} else {
		band = bandCritical
	}
	band = applyConfidenceCap(band, confidence)

	display := fmt.Sprintf("%d new high-risk unbalanced edges", newHigh)
	delta := computeDelta(value, in.Baseline, m.Name(), m.Version())

	// baselineHigh, exceptedHigh, expiredHigh are recorded for future breakdown output (Phase 2).
	_ = baselineHigh
	_ = exceptedHigh
	_ = expiredHigh

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       "count",
		Definition: "intrusive edges with cross-module ownership and high volatility",
		Delta:      delta,
	}
}

// statusPriority returns a priority number for finding status; higher = more important to surface.
func statusPriority(s finding.Status) int {
	switch s {
	case finding.StatusNew:
		return 4
	case finding.StatusExpiredExcept:
		return 3
	case finding.StatusExcepted:
		return 2
	case finding.StatusBaseline:
		return 1
	default:
		return 0
	}
}

// stripKindPrefix removes the "kind:" prefix from a node ID.
func stripKindPrefix(id string) string {
	if _, after, ok := strings.Cut(id, ":"); ok {
		return after
	}
	return id
}

// ---------------------------------------------------------------------------
// CycleMetric (cycle.v1)
// ---------------------------------------------------------------------------

// CycleMetric detects dependency cycles using Tarjan's SCC algorithm on the
// graph's import edges. Each SCC of size > 1 is counted as one cycle (spec §10.4).
type CycleMetric struct{}

// Name returns "cycle".
func (m CycleMetric) Name() string { return "cycle" }

// Version returns "cycle.v1".
func (m CycleMetric) Version() string { return "cycle.v1" }

// Calculate counts strongly-connected components of size > 1 in the dependency graph.
func (m CycleMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	cycles := 0
	if in.Graph != nil {
		cycles = countCycles(in.Graph)
	}

	value := float64(cycles)
	confidence := confidenceHigh
	var band string
	if cycles == 0 {
		band = bandStrong
	} else {
		band = bandCritical
	}
	band = applyConfidenceCap(band, confidence)

	display := fmt.Sprintf("%d import cycles", cycles)
	delta := computeDelta(value, in.Baseline, m.Name(), m.Version())

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       "count",
		Definition: "number of import cycles",
		Delta:      delta,
	}
}

// countCycles counts strongly-connected components of size > 1 using
// Tarjan's algorithm applied to dependency edges only (imports, depends_on,
// uses_internal). belongs_to edges are excluded to avoid structural noise.
func countCycles(g *graph.Graph) int {
	// Build adjacency list from dependency edges only.
	adj := make(map[string][]string)
	nodeSet := make(map[string]struct{})
	for _, e := range g.Edges() {
		switch e.Kind {
		case graph.EdgeKindImports, graph.EdgeKindDependsOn, graph.EdgeKindUsesInternal:
		default:
			continue
		}
		adj[e.From] = append(adj[e.From], e.To)
		nodeSet[e.From] = struct{}{}
		nodeSet[e.To] = struct{}{}
	}

	// Tarjan's SCC.
	idx := 0
	indices := make(map[string]int)
	lowlink := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	sccCount := 0

	var strongConnect func(v string)
	strongConnect = func(v string) {
		indices[v] = idx
		lowlink[v] = idx
		idx++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, visited := indices[w]; !visited {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlink[v] {
					lowlink[v] = indices[w]
				}
			}
		}

		if lowlink[v] == indices[v] {
			// Pop SCC from stack.
			size := 0
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				size++
				if w == v {
					break
				}
			}
			if size > 1 {
				sccCount++
			}
		}
	}

	// Visit every node in sorted order for determinism.
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	sortStrings(nodes)

	for _, v := range nodes {
		if _, visited := indices[v]; !visited {
			strongConnect(v)
		}
	}

	return sccCount
}

// sortStrings sorts a string slice in place (insertion sort — fine for small slices).
func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		key := ss[i]
		j := i - 1
		for j >= 0 && ss[j] > key {
			ss[j+1] = ss[j]
			j--
		}
		ss[j+1] = key
	}
}

// ---------------------------------------------------------------------------
// CoverageMetric (coverage.v1)
// ---------------------------------------------------------------------------

// CoverageMetric reports extracted_files / applicable_files from ToolCoverage.
// Confidence degrades as the unresolved count grows relative to total files seen.
type CoverageMetric struct{}

// Name returns "coverage".
func (m CoverageMetric) Name() string { return "coverage" }

// Version returns "coverage.v1".
func (m CoverageMetric) Version() string { return "coverage.v1" }

// Calculate computes the coverage ratio and applies confidence-based band capping.
func (m CoverageMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	var totalApplicable, totalExtracted, totalUnresolved int
	for _, c := range in.ToolCoverage {
		totalApplicable += c.FilesApplicable
		totalExtracted += c.FilesSeen
		totalUnresolved += c.Unresolved
	}

	var value float64
	if totalApplicable == 0 {
		value = 1.0
	} else {
		value = float64(totalExtracted) / float64(totalApplicable)
	}

	// Confidence: based on unresolved ratio vs files seen.
	confidence := coverageConfidence(totalUnresolved, totalExtracted)

	score := value * 10.0
	band := applyConfidenceCap(bandScore(score), confidence)
	display := fmt.Sprintf("%.0f%% coverage", value*100)
	delta := computeDelta(value, in.Baseline, m.Name(), m.Version())

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       "ratio",
		Definition: "extracted_files / applicable_files",
		Delta:      delta,
	}
}

// coverageConfidence derives a confidence level from the unresolved/total ratio.
//
//	unresolved / total <= 0.05 → high
//	unresolved / total <= 0.20 → medium
//	otherwise                  → low
func coverageConfidence(unresolved, total int) string {
	if total == 0 {
		return confidenceHigh
	}
	ratio := float64(unresolved) / float64(total)
	switch {
	case ratio <= 0.05:
		return confidenceHigh
	case ratio <= 0.20:
		return confidenceMedium
	default:
		return confidenceLow
	}
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// New returns all Phase 1 metrics. Each metric reads its per-metric config
// via cfg.ForMetric(name) when needed (gate thresholds etc. are consumed by engine).
func New(_ config.Config) []Metric {
	return []Metric{
		EncapsulationMetric{},
		UnbalancedEdgeMetric{},
		CycleMetric{},
		CoverageMetric{},
	}
}
