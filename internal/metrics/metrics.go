package metrics

import (
	"fmt"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/clones"
	"github.com/alexei-led/archfit/internal/fitness"
	"github.com/alexei-led/archfit/internal/model/coupling"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/model/symbol"
)

// Band name constants (spec §10.1).
const (
	bandStrong      = "strong"
	bandServiceable = "serviceable"
	bandMixed       = "mixed"
	bandPoor        = "poor"
	bandCritical    = "critical"
	// bandNA marks a metric that cannot be scored because the underlying signal
	// is absent (e.g. encapsulation when no cross-boundary edge has a classified
	// strength). It is neither good nor bad — it means "no evidence", and must
	// not be conflated with bandCritical (a measured, bad value).
	bandNA = "n/a"
	// bandInformational marks a report-only metric that surfaces facts (hubs,
	// change-impact) without asserting a 0–10 quality verdict. Never gates.
	bandInformational = "info"
)

// Confidence level constants.
const (
	confidenceHigh   = "high"
	confidenceMedium = "medium"
	confidenceLow    = "low"
)

// Metric mode constants (the "mode" JSON field, spec §10).
const (
	modeRatio = "ratio"
	modeCount = "count"
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

// ChangeHistory carries git-derived volatility signals into the engine for the
// modularity metrics. Both maps are empty when no git history is available.
type ChangeHistory struct {
	FileChurn      map[string]int    // file -> recent commit count
	CoChange       map[[2]string]int // sorted file pair -> commits touching both
	FileLOC        map[string]int    // source file -> lines of code (tests excluded)
	Complexity     []ComplexityFunc  // per-function cyclomatic complexity (external tool)
	FitnessSignals fitness.Signals   // architecture-intent enforcement signals (filesystem scan)
	CloneClusters  []clones.Cluster  // duplicated code blocks across files (clone detector)
	// GitnexusImpact maps module path → historical change-impact count from the
	// gitnexus CLI. Nil/empty when gitnexus is disabled or absent; risk_hub uses it
	// as an optional multiplicative factor (never alters surface-breadth computation).
	GitnexusImpact map[string]int
	// ExtraCoverage carries tool-coverage records for opt-in tools that run in cmd
	// (clones, gitnexus) rather than through the engine extractor loop. The engine
	// appends these to the diagnostic ToolCoverage slice after building its own records.
	ExtraCoverage []diagnostic.Coverage
}

// MetricInput is the complete input set for all metrics.
type MetricInput struct {
	Graph           *graph.Graph
	Classifications coupling.Index
	Findings        []finding.Finding // status-tagged findings from status stage
	Baseline        diagnostic.MetricSnapshot
	ToolCoverage    []diagnostic.Coverage // from extractors
	// FileChurn maps a repo-relative source file to its recent commit count (git
	// history). Empty when no git history is available; modularity metrics that
	// depend on volatility then report n/a.
	FileChurn map[string]int
	// CoChange maps a sorted file pair to the number of commits that touched both —
	// the logical-coupling signal for hidden_coupling. Empty when unavailable.
	CoChange map[[2]string]int
	// FileLOC maps a source file to its line count (tests excluded), for the
	// structural_weight (size-skew / god-module) metric. Empty when unavailable.
	FileLOC map[string]int
	// Complexity is per-function cyclomatic complexity from an external tool
	// (lizard), for the complexity metric. Empty when the opt-in tool is off/absent.
	Complexity []ComplexityFunc
	// SymbolGraph is per-symbol module ownership, fan-in, and cross-module reference
	// edges from a SCIP index. Empty when SCIP is off or the indexer is absent;
	// metrics that need it must report n/a when SymbolGraph.Empty() is true.
	SymbolGraph symbol.Graph
	// FitnessSignals carries the results of the architecture-intent enforcement scan
	// (arch tests, import-linter config, arch-linter in CI). Zero-value Signals
	// (EvidencePaths == nil) means the scan was never run; metrics must report n/a.
	FitnessSignals fitness.Signals
	// CloneClusters holds duplicated-code blocks detected by an external clone
	// detector (e.g. jscpd). Empty when the tool is disabled or absent; metrics
	// that need it must report n/a when CloneClusters is nil/empty.
	CloneClusters []clones.Cluster
	// GitnexusImpact maps module path → historical change-impact count from the
	// gitnexus CLI (tools.gitnexus.enabled: on). Nil/empty (the default) leaves
	// risk_hub behaviour exactly as today (surface-breadth × volatility only).
	// When non-empty, risk_hub incorporates it as a bounded additional factor.
	GitnexusImpact map[string]int
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

// EncapsulationMetric measures, among cross-boundary edges that take a stance on
// boundary respect, the fraction that go through a contract (spec §10.4).
//
//	value = contract / (contract + intrusive) cross-boundary edges
//	Functional and model coupling are normal public use, not boundary verdicts,
//	so they are excluded from the denominator (alongside unknown).
//	No cross-boundary edges at all  → value 1.0 (vacuously encapsulated).
//	Cross-boundary edges but none contract/intrusive → bandNA (no boundary signal).
//	Confidence scales with the contract+intrusive fraction of cross-boundary edges.
type EncapsulationMetric struct{}

// Name returns "encapsulation".
func (m EncapsulationMetric) Name() string { return "encapsulation" }

// Version returns "encapsulation.v1".
func (m EncapsulationMetric) Version() string { return "encapsulation.v1" }

// Calculate computes the encapsulation ratio from cross-boundary edge classifications.
//
// The ratio is contract edges over the edges that take a boundary stance
// (contract + intrusive). Functional, model, and unknown strengths are excluded:
// functional/model are normal public coupling and unknown is absence of evidence —
// none is a boundary violation, so none should drag the ratio toward 0.
//
// The metric is indeterminate (bandNA) in two cases: (1) no cross-boundary edge is
// contract or intrusive (no signal); (2) there are no intrusive edges at all — then
// the ratio is trivially ~1.0 and cannot distinguish earned encapsulation from the
// compiler-boundary case (Go/TS, where every cross-package import is forced through
// an exported API). Reporting 10/10 there is the over-score this avoids; the
// discriminating modularity signal lives in change-amplification, hidden-coupling,
// and cycles instead.
func (m EncapsulationMetric) Calculate(in MetricInput) diagnostic.MetricResult {
	if in.Graph == nil {
		return m.result(1.0, confidenceHigh, in.Baseline)
	}

	var allCross, classifiedCross, contractCross, intrusiveCross int
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
		if isClassifiedStrength(cl.Strength) {
			classifiedCross++
			switch cl.Strength {
			case coupling.StrengthContract:
				contractCross++
			case coupling.StrengthIntrusive:
				intrusiveCross++
			}
		}
	}

	// No cross-boundary coupling at all → vacuously encapsulated.
	if allCross == 0 {
		return m.result(1.0, confidenceHigh, in.Baseline)
	}
	// Coupling exists but no edge strength could be classified → no signal.
	if classifiedCross == 0 {
		return m.naResult(in.Baseline)
	}
	// No intrusive edge to contrast against → the ratio is trivially ~1.0 and
	// non-discriminating. This is the compiler-boundary case (Go/TS: every cross-
	// package import is through an exported API, so "contract" is forced, not
	// earned). Report n/a rather than a false 10/10; the realistic modularity signal
	// lives in change-amplification, hidden-coupling, and cycles.
	if intrusiveCross == 0 {
		return m.naResult(in.Baseline)
	}

	value := float64(contractCross) / float64(classifiedCross)
	return m.result(value, classificationConfidence(classifiedCross, allCross), in.Baseline)
}

// isClassifiedStrength reports whether a strength takes a stance on boundary
// respect, i.e. counts in the encapsulation ratio. Only contract (goes through a
// published interface) and intrusive (reaches into internals) do: encapsulation
// measures contract-vs-internal-leak. Functional (calling a public function) and
// model (using a public data type) are normal public coupling — neither a contract
// nor a leak — so, like unknown, they are excluded from the denominator rather than
// counted against the score. Including them would crush the ratio for any codebase
// that mostly calls public functions (the common case) and produce a false critical.
func isClassifiedStrength(s coupling.Strength) bool {
	switch s {
	case coupling.StrengthContract, coupling.StrengthIntrusive:
		return true
	default:
		return false
	}
}

// classificationConfidence derives confidence from how much of the cross-boundary
// coupling could actually be classified. A score computed from a small classified
// fraction is downgraded by the band cap so the tool never over-claims a good band
// on thin evidence (it never inflates a bad band — see applyConfidenceCap).
func classificationConfidence(classified, all int) string {
	switch {
	case classified*5 >= all*4: // ≥ 80% classified
		return confidenceHigh
	case classified*2 >= all: // ≥ 50% classified
		return confidenceMedium
	default:
		return confidenceLow
	}
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
		Mode:       modeRatio,
		Definition: "contract / (contract + intrusive) cross-boundary edges (functional, model, unknown excluded)",
		Delta:      delta,
	}
}

// naResult reports the encapsulation metric as indeterminate: cross-boundary
// coupling exists but no edge strength could be classified, so there is no honest
// score. Band is bandNA (not critical), and Delta is nil so the verdict logic does
// not treat an absent score as a regression.
func (m EncapsulationMetric) naResult(_ diagnostic.MetricSnapshot) diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      0,
		Display:    bandNA,
		Band:       bandNA,
		Confidence: confidenceLow,
		Version:    m.Version(),
		Mode:       modeRatio,
		Definition: "contract / (contract + intrusive) cross-boundary edges (functional, model, unknown excluded)",
		Delta:      nil,
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
		candidates++
		if cl.Volatility != coupling.VolatilityUnknown {
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

	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      value,
		Display:    display,
		Band:       band,
		Confidence: confidence,
		Version:    m.Version(),
		Mode:       modeCount,
		Definition: "intrusive edges with cross-module ownership and high volatility",
		Delta:      delta,
	}
}

// naResult reports unbalanced_edge as indeterminate: intrusive cross-module
// candidate edges exist, but none has a known volatility, so whether any is
// unbalanced cannot be decided. Band is bandNA (not strong), Delta nil.
func (m UnbalancedEdgeMetric) naResult() diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name:       m.Name(),
		Value:      0,
		Display:    bandNA,
		Band:       bandNA,
		Confidence: confidenceLow,
		Version:    m.Version(),
		Mode:       modeCount,
		Definition: "intrusive edges with cross-module ownership and high volatility",
		Delta:      nil,
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
		Mode:       modeCount,
		Definition: "number of import cycles",
		Delta:      delta,
	}
}

// countCycles counts strongly-connected components of size > 1 using
// the shared Tarjan SCC detection in graph.Graph.Cycles.
func countCycles(g *graph.Graph) int {
	return len(g.Cycles())
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
		Mode:       modeRatio,
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
//
// Volatility for RiskHubMetric is captured here, before any call to
// config.ApplyVolatility, so churn-derived values never contaminate the
// risk_hub signal (that would double-count with change_amplification).
func New(cfg config.Config) []Metric {
	return []Metric{
		EncapsulationMetric{},
		UnbalancedEdgeMetric{},
		CycleMetric{},
		CoverageMetric{},
		BlastRadiusMetric{},
		ChangeAmplificationMetric{},
		HiddenCouplingMetric{},
		StructuralWeightMetric{},
		ComplexityMetric{},
		newRiskHubMetric(cfg),
		ArchitectureFitnessMetric{},
		FunctionalCandidatesMetric{},
	}
}

// newRiskHubMetric builds a RiskHubMetric with volatility multipliers derived
// exclusively from the explicit config (Subdomain/Volatility fields). This must
// be called before config.ApplyVolatility so that only hand-authored values
// influence the metric — churn-derived volatility must not reach risk_hub.
func newRiskHubMetric(cfg config.Config) RiskHubMetric {
	mv := make(map[string]float64, len(cfg.Modules))
	for name, def := range cfg.Modules {
		mv[name] = volatilityBandMultiplier(def.Volatility)
	}
	return RiskHubMetric{moduleVolatility: mv}
}
