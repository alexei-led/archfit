package score

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

// metric builds a MetricResult for a test Diagnostic.
func metric(name string, value float64, band, conf string) diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name: name, Value: value, Display: strconv.FormatFloat(value, 'f', 2, 64),
		Band: band, Confidence: conf,
	}
}

// bcAdv builds a Balanced-Coupling advisory finding (a rollup of count edges).
func bcAdv(from, to, strength, distance, vol string, scoreVal int, scoreBand, sev string, count int) finding.Finding {
	return finding.Finding{
		ID:       from + "->" + to,
		RuleID:   "bc/imbalanced_coupling",
		Kind:     "advisory",
		Status:   finding.StatusNew,
		Severity: finding.Severity(sev),
		Edge: finding.EdgeEvidence{
			From: finding.Endpoint{Module: from},
			To:   finding.Endpoint{Module: to},
		},
		MatchedBy: map[string]string{
			"strength":    strength,
			"distance":    distance,
			"volatility":  vol,
			"score_value": strconv.Itoa(scoreVal),
			"score_band":  scoreBand,
			"group_count": strconv.Itoa(count),
		},
	}
}

func gate(id string) finding.Finding {
	return finding.Finding{
		ID: id, RuleID: "no_internal_access", Kind: "gate",
		Status: finding.StatusNew, Severity: finding.SeverityHigh,
	}
}

func okCov(tool string) diagnostic.Coverage {
	return diagnostic.Coverage{Tool: tool, Status: diagnostic.StatusOK}
}

// richDiagnostic is a representative full Diagnostic with all dimensions feedable.
func richDiagnostic() diagnostic.Diagnostic {
	d := diagnostic.New()
	d.ConfigHash = "deadbeef"
	d.Metrics = []diagnostic.MetricResult{
		metric("encapsulation", 0.90, "strong", "high"),
		metric("coverage", 0.95, "strong", "high"),
		metric("cycle", 0, "strong", "high"),
		metric("blast_radius", 1, "info", "high"),
		metric("instability", 2, "info", "high"),
		metric("propagation_cost", 0.10, "info", "high"),
		metric("structural_weight", 1, "info", "high"),
		metric("hidden_coupling", 0, "info", "high"),
		metric("functional_candidates", 0, "info", "high"),
		metric("change_coupling", 1, "info", "high"),
		metric("change_amplification", 0, "info", "high"),
		metric("architecture_fitness", 0.67, "info", "high"),
	}
	d.Findings = []finding.Finding{
		bcAdv("a", "b", "functional", "cross_module_same_owner", "medium", 5, "medium", "medium", 3),
	}
	d.ToolCoverage = []diagnostic.Coverage{
		okCov("go/packages"), okCov("scip"), okCov("gitnexus"), okCov("lizard"), okCov("jscpd"),
	}
	return d
}

// dimByName returns the scorecard dimension with the given name.
func dimByName(t *testing.T, sc Scorecard, name string) Dimension {
	t.Helper()
	for _, d := range sc.Dimensions {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("dimension %q not found", name)
	return Dimension{}
}

// TestSynthesize_RuleInvariants asserts the scorecard contract holds on a
// representative Diagnostic: seven dimensions, band-matches-value, evidence per
// non-meta score, the high-quality-requires-confidence rule, and a meta last
// dimension.
func TestSynthesize_RuleInvariants(t *testing.T) {
	sc := Synthesize(richDiagnostic())

	if sc.RubricVersion != RubricVersion {
		t.Errorf("rubric_version = %d, want %d", sc.RubricVersion, RubricVersion)
	}
	if len(sc.Dimensions) != 7 {
		t.Fatalf("got %d dimensions, want 7", len(sc.Dimensions))
	}

	wantOrder := []string{
		DimBoundaryIntegrity, DimCouplingBalance, DimDependencyGraphHealth,
		DimCohesionModularity, DimChangeLocality, DimArchitectureFitness,
		DimAnalysisConfidence,
	}
	for i, name := range wantOrder {
		if sc.Dimensions[i].Name != name {
			t.Errorf("dimension[%d] = %q, want %q", i, sc.Dimensions[i].Name, name)
		}
	}

	meta := sc.Dimensions[6]
	if !meta.Meta || meta.Name != DimAnalysisConfidence {
		t.Errorf("last dimension should be the meta analysis_confidence, got %+v", meta)
	}

	for _, d := range sc.Dimensions {
		// band_matches_value
		if got := bandFor(d.Value); d.Band != got {
			t.Errorf("%s: band %q does not match value %d (want %q)", d.Name, d.Band, d.Value, got)
		}
		if d.Value < 0 || d.Value > 100 {
			t.Errorf("%s: value %d out of [0,100]", d.Name, d.Value)
		}
		// score_requires_evidence (non-meta)
		if !d.Meta && len(d.Evidence) == 0 {
			t.Errorf("%s: non-meta dimension has no evidence", d.Name)
		}
		// high_quality_requires_confidence
		if (d.Band == BandServiceable || d.Band == BandStrong) && d.Confidence == ConfidenceLow {
			t.Errorf("%s: band %q with low confidence violates high_quality_requires_confidence", d.Name, d.Band)
		}
	}

	// Overall is the mean of the six non-meta dimensions.
	sum := 0
	for _, d := range sc.Dimensions[:6] {
		sum += d.Value
	}
	wantOverall := (sum + 3) / 6 // round-half-up for positive ints
	if sc.Overall != wantOverall && sc.Overall != wantOverall-1 {
		t.Errorf("overall = %d, want ≈%d (mean of non-meta)", sc.Overall, wantOverall)
	}
	if sc.OverallBand != bandFor(sc.Overall) {
		t.Errorf("overall band %q does not match overall %d", sc.OverallBand, sc.Overall)
	}
}

// TestSynthesize_Deterministic asserts Synthesize is a pure function: two calls
// on the same input produce deeply-equal scorecards (no map-order leakage).
func TestSynthesize_Deterministic(t *testing.T) {
	d := richDiagnostic()
	a := Synthesize(d)
	b := Synthesize(d)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("Synthesize not deterministic:\n a=%+v\n b=%+v", a, b)
	}
}

// TestLowConfidenceCap asserts a dimension that would score serviceable/strong is
// capped to mixed (≤60) when its confidence is low.
func TestLowConfidenceCap(t *testing.T) {
	d := diagnostic.New()
	// Low file coverage → low baseline confidence for structural dimensions.
	d.Metrics = []diagnostic.MetricResult{
		metric("coverage", 0.30, "poor", "low"),
		// architecture_fitness would score 100 (all signals present)…
		metric("architecture_fitness", 1.0, "info", "high"),
	}
	sc := Synthesize(d)
	af := dimByName(t, sc, DimArchitectureFitness)
	if af.Confidence != ConfidenceLow {
		t.Fatalf("expected low confidence for architecture_fitness, got %q", af.Confidence)
	}
	if af.Value > 60 {
		t.Errorf("low-confidence dimension not capped: value %d (band %q)", af.Value, af.Band)
	}
	if af.Band == BandServiceable || af.Band == BandStrong {
		t.Errorf("low-confidence dimension presents high band %q", af.Band)
	}
}

// TestCouplingBalance covers the Balanced-Coupling derivation: no edges, balanced
// edges, a single worst-case edge, and pervasive worst-case edges.
func TestCouplingBalance(t *testing.T) {
	cb := func(edges ...finding.Finding) Dimension {
		d := diagnostic.New()
		d.Findings = edges
		// A non-degenerate graph (≥2 connected modules) so the 0-edge branch is the
		// "edges unclassified/all-balanced" case, not the single-module degenerate one.
		d.Metrics = []diagnostic.MetricResult{metric("blast_radius", 3, "info", "high")}
		return dimByName(t, Synthesize(d), DimCouplingBalance)
	}

	t.Run("no classified edges → unconfirmed, never strong", func(t *testing.T) {
		got := cb()
		if got.Value > 60 {
			t.Errorf("no-edges value = %d, want ≤60 (unconfirmed, not false-green)", got.Value)
		}
	})

	t.Run("balanced low-effort edges score high", func(t *testing.T) {
		got := cb(
			bcAdv("a", "b", "contract", "cross_module_same_owner", "low", 2, "none", "low", 10),
		)
		if got.Value < 61 {
			t.Errorf("low-effort value = %d, want serviceable+ (≥61)", got.Value)
		}
	})

	t.Run("a single worst-case edge caps at mixed", func(t *testing.T) {
		got := cb(
			bcAdv("a", "b", "contract", "cross_module_same_owner", "low", 1, "none", "low", 100),
			bcAdv("c", "d", "intrusive", "cross_deploy_unit", "high", 10, "critical", "critical", 1),
		)
		if got.Value > 60 {
			t.Errorf("worst-case present but value %d not capped to mixed (≤60)", got.Value)
		}
	})

	t.Run("pervasive worst-case caps at poor", func(t *testing.T) {
		got := cb(
			bcAdv("a", "b", "contract", "cross_module_same_owner", "low", 1, "none", "low", 50),
			bcAdv("c", "d", "intrusive", "cross_deploy_unit", "high", 10, "critical", "critical", 10),
		)
		if got.Value > 40 {
			t.Errorf("pervasive worst-case but value %d not capped to poor (≤40)", got.Value)
		}
	})

	t.Run("excepted and baseline edges do not count", func(t *testing.T) {
		withStatus := func(f finding.Finding, s finding.Status) finding.Finding {
			f.Status = s
			return f
		}
		worst := func(from, to string) finding.Finding {
			return bcAdv(from, to, "intrusive", "cross_deploy_unit", "high", 10, "critical", "critical", 10)
		}
		// Two worst-case edges, both operator-suppressed (excepted / baseline). They
		// must not penalise the dimension — same view the gate verdict takes. With no
		// counted edges left, the dimension is unconfirmed (mixed), never poor.
		got := cb(
			withStatus(worst("a", "b"), finding.StatusExcepted),
			withStatus(worst("c", "d"), finding.StatusBaseline),
		)
		if got.Value <= 40 {
			t.Errorf("suppressed edges value = %d, want > poor (>40); excepted/baseline must not penalise", got.Value)
		}
		if got.Value > 60 {
			t.Errorf("suppressed edges value = %d, want ≤60 (no counted edges = unconfirmed, not strong)", got.Value)
		}
	})
}

// TestCouplingBalance_EmptyEdges asserts zero classified coupling edges never
// produces a strong band. Zero edges means the classifier did not run (e.g. SCIP
// absent), the graph has no cross-boundary edges, or all edges are balanced — none
// distinguishable from the scorecard — so the result is capped at mixed and low
// confidence regardless of baseline coverage (coverage-gap cap).
func TestCouplingBalance_EmptyEdges(t *testing.T) {
	// A non-degenerate graph carries graph-shape metrics (≥2 connected modules).
	nonDegen := metricIndex{"blast_radius": metric("blast_radius", 3, "info", "high")}

	t.Run("non-degenerate + no edges → ≤60/low, never strong", func(t *testing.T) {
		got := couplingBalance(nil, nonDegen)
		if got.Value > 60 {
			t.Errorf("value = %d, want ≤60 (no false-green on 0 classified edges)", got.Value)
		}
		if got.Confidence != ConfidenceLow {
			t.Errorf("confidence = %q, want low", got.Confidence)
		}
	})

	t.Run("degenerate graph + no edges → 50/low", func(t *testing.T) {
		got := couplingBalance(nil, metricIndex{})
		if got.Value != 50 || got.Confidence != ConfidenceLow {
			t.Errorf("value/conf = %d/%q, want 50/low", got.Value, got.Confidence)
		}
	})

	t.Run("no edges never strong through Synthesize", func(t *testing.T) {
		d := diagnostic.New()
		d.Metrics = []diagnostic.MetricResult{metric("coverage", 0.2, "poor", "low")}
		cb := dimByName(t, Synthesize(d), DimCouplingBalance)
		if cb.Value > 60 {
			t.Errorf("no-edge coupling_balance = %d, want ≤60 (not false-green)", cb.Value)
		}
	})
}

// TestDegenerateGraph_NoFalseGreen is the regression guard for the single-module
// false-green (e.g. a single-crate Rust binary archfit sees as one node): with no
// graph-shape metrics, the structural dimensions must report unmeasured (≤60/low),
// not a vacuous strong — even when the encapsulation metric reports a vacuous 1.0.
func TestDegenerateGraph_NoFalseGreen(t *testing.T) {
	d := diagnostic.New()
	d.Metrics = []diagnostic.MetricResult{
		metric("encapsulation", 1.0, "strong", "high"), // vacuous on a 1-module graph
		metric("cycle", 0, "strong", "high"),
		metric("propagation_cost", 0.05, "info", "high"),
		// no blast_radius, no instability → degenerate graph
	}
	sc := Synthesize(d)
	for _, name := range []string{
		DimBoundaryIntegrity, DimDependencyGraphHealth, DimCohesionModularity, DimCouplingBalance,
	} {
		dim := dimByName(t, sc, name)
		if dim.Value > 60 {
			t.Errorf("%s = %d on a degenerate (<2-module) graph, want ≤60 (no false-green)", name, dim.Value)
		}
		if dim.Confidence != ConfidenceLow {
			t.Errorf("%s confidence = %q on a degenerate graph, want low", name, dim.Confidence)
		}
	}
}

// TestBoundaryIntegrity_GateViolations asserts active gate findings subtract from
// boundary integrity and are cited as evidence.
func TestBoundaryIntegrity_GateViolations(t *testing.T) {
	// blast_radius makes the graph non-degenerate so encapsulation 1.0 is a genuine
	// multi-module measurement, not the vacuous single-module case.
	clean := diagnostic.New()
	clean.Metrics = []diagnostic.MetricResult{metric("encapsulation", 1.0, "strong", "high"), metric("blast_radius", 2, "info", "high")}
	cleanDim := dimByName(t, Synthesize(clean), DimBoundaryIntegrity)

	dirty := diagnostic.New()
	dirty.Metrics = []diagnostic.MetricResult{metric("encapsulation", 1.0, "strong", "high"), metric("blast_radius", 2, "info", "high")}
	dirty.Findings = []finding.Finding{gate("g1"), gate("g2")}
	dirtyDim := dimByName(t, Synthesize(dirty), DimBoundaryIntegrity)

	if dirtyDim.Value >= cleanDim.Value {
		t.Errorf("gate violations did not lower boundary integrity: clean=%d dirty=%d", cleanDim.Value, dirtyDim.Value)
	}
	found := false
	for _, e := range dirtyDim.Evidence {
		if strings.Contains(e, "gate violation") {
			found = true
		}
	}
	if !found {
		t.Errorf("gate violations not cited in evidence: %v", dirtyDim.Evidence)
	}
}

// TestBoundaryIntegrity_EncapsulationNA asserts that when encapsulation cannot be
// measured the dimension does not fabricate a perfect baseline: it starts neutral
// with low confidence and cites the unmeasured baseline explicitly.
func TestBoundaryIntegrity_EncapsulationNA(t *testing.T) {
	// blast_radius present (non-degenerate graph) but no encapsulation metric → the
	// genuine encapsulation-n/a branch, not the single-module degenerate guard.
	mi := indexMetrics([]diagnostic.MetricResult{metric("blast_radius", 2, "info", "high")})
	dim := finalize(boundaryIntegrity(mi, nil, ConfidenceHigh))

	if dim.Value == 100 {
		t.Errorf("value = 100, encapsulation n/a must not fabricate a perfect score")
	}
	if dim.Value != 50 {
		t.Errorf("value = %d, want 50 (neutral unmeasured baseline)", dim.Value)
	}
	if dim.Confidence != ConfidenceLow {
		t.Errorf("confidence = %q, want low", dim.Confidence)
	}
	found := false
	for _, e := range dim.Evidence {
		if strings.Contains(e, "encapsulation: n/a") && strings.Contains(e, "unmeasured") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected explicit unmeasured-baseline note in evidence: %v", dim.Evidence)
	}
}

// TestArchitectureFitness_NA asserts that when the fitness scan never ran (metric
// n/a) the dimension reads as poor ("scan didn't run", value 40, low confidence),
// not a fabricated critical 10; while a real 0/3 scan stays critical.
func TestArchitectureFitness_NA(t *testing.T) {
	t.Run("metric n/a → poor 40 low", func(t *testing.T) {
		mi := indexMetrics([]diagnostic.MetricResult{metric("architecture_fitness", 0, "n/a", "low")})
		dim := finalize(architectureFitness(mi, ConfidenceHigh))
		if dim.Value != 40 {
			t.Errorf("value = %d, want 40", dim.Value)
		}
		if dim.Band != BandPoor {
			t.Errorf("band = %q, want poor", dim.Band)
		}
		if dim.Confidence != ConfidenceLow {
			t.Errorf("confidence = %q, want low", dim.Confidence)
		}
	})

	t.Run("ran, found 0/3 → critical", func(t *testing.T) {
		mi := indexMetrics([]diagnostic.MetricResult{metric("architecture_fitness", 0, "info", "high")})
		dim := finalize(architectureFitness(mi, ConfidenceHigh))
		if dim.Band != BandCritical {
			t.Errorf("band = %q, want critical (scan ran, 0/3 signals)", dim.Band)
		}
	})
}

// TestChangeLocality_Unmeasured asserts that with no delta base and no git history
// the dimension is neutral with low confidence rather than a false perfect score.
func TestChangeLocality_Unmeasured(t *testing.T) {
	d := diagnostic.New() // no change_locality / change_coupling / change_amplification metrics
	cl := dimByName(t, Synthesize(d), DimChangeLocality)
	if cl.Confidence != ConfidenceLow {
		t.Errorf("unmeasured change_locality confidence = %q, want low", cl.Confidence)
	}
	if cl.Value == 100 {
		t.Errorf("unmeasured change_locality should not be a perfect 100")
	}
}

// TestDependencyGraphHealth_PartialMetrics asserts that when only the secondary
// dependency metrics (instability, propagation_cost) ran — cycle and blast_radius
// disabled — the dimension still counts as measured: it keeps the base confidence
// and never claims "no dependency-graph metrics available".
func TestDependencyGraphHealth_PartialMetrics(t *testing.T) {
	mi := indexMetrics([]diagnostic.MetricResult{
		metric("instability", 2, "info", "high"),
		metric("propagation_cost", 0.10, "info", "high"),
	})
	dim := dependencyGraphHealth(mi, ConfidenceHigh)

	if dim.Confidence != ConfidenceHigh {
		t.Errorf("confidence = %q, want high (metrics were measured)", dim.Confidence)
	}
	for _, ev := range dim.Evidence {
		if strings.Contains(ev, "no dependency-graph metrics available") {
			t.Errorf("measured dimension wrongly reports absent metrics: %v", dim.Evidence)
		}
	}
	if dim.Value == 100 {
		t.Errorf("value = 100, want penalized by instability/propagation_cost")
	}
}

// TestCoverageConfidence_RespectsMetricConfidence asserts the scorecard baseline
// confidence is capped by the coverage metric's own confidence: a perfect
// extraction ratio (value 1.0) built on many unresolved imports (confidence low)
// must not yield a high baseline.
func TestCoverageConfidence_RespectsMetricConfidence(t *testing.T) {
	lowMI := indexMetrics([]diagnostic.MetricResult{metric("coverage", 1.0, "strong", "low")})
	if got := coverageConfidence(diagnostic.Diagnostic{}, lowMI); got != ConfidenceLow {
		t.Errorf("value 1.0 / metric-confidence low → baseline %q, want low", got)
	}

	highMI := indexMetrics([]diagnostic.MetricResult{metric("coverage", 1.0, "strong", "high")})
	if got := coverageConfidence(diagnostic.Diagnostic{}, highMI); got != ConfidenceHigh {
		t.Errorf("value 1.0 / metric-confidence high → baseline %q, want high", got)
	}
}

// TestAnalysisConfidence_SemanticToolsAbsent asserts missing semantic tools lower
// the meta confidence score versus a fully-covered run.
func TestAnalysisConfidence_SemanticToolsAbsent(t *testing.T) {
	full := richDiagnostic()
	fullMeta := dimByName(t, Synthesize(full), DimAnalysisConfidence)

	bare := diagnostic.New()
	bare.Metrics = []diagnostic.MetricResult{metric("coverage", 0.95, "strong", "high")}
	bare.ToolCoverage = []diagnostic.Coverage{okCov("go/packages")} // no scip/gitnexus/lizard/jscpd
	bareMeta := dimByName(t, Synthesize(bare), DimAnalysisConfidence)

	if bareMeta.Value >= fullMeta.Value {
		t.Errorf("absent semantic tools did not lower meta confidence: full=%d bare=%d", fullMeta.Value, bareMeta.Value)
	}
	if !bareMeta.Meta {
		t.Errorf("analysis_confidence should be marked meta")
	}
}

// TestAnalysisConfidence asserts the meta dimension is honest about coverage: an
// unanalysed repo (coverage n/a, no primary extractor) collapses to critical
// instead of reading pct(0)=0, and a fully-extracted run with the semantic tools
// absent only degrades gradually.
func TestAnalysisConfidence(t *testing.T) {
	cases := []struct {
		name     string
		metrics  []diagnostic.MetricResult
		coverage []diagnostic.Coverage
		wantBand Band
		maxValue int // value must be ≤ this (0 = no upper bound)
		minValue int // value must be ≥ this
	}{
		{
			name:     "coverage n/a + all primary absent → critical",
			metrics:  []diagnostic.MetricResult{metric("coverage", 0, "n/a", "low")},
			coverage: nil, // go/packages, dependency-cruiser, grimp all absent
			wantBand: BandCritical,
			maxValue: 20,
		},
		{
			name:    "coverage n/a but one primary + semantic present → graded, not critical",
			metrics: []diagnostic.MetricResult{metric("coverage", 0, "n/a", "low")},
			// 2/3 primary absent (−30), semantic tools present so they don't compound
			coverage: []diagnostic.Coverage{
				okCov("go/packages"),
				okCov("scip"), okCov("gitnexus"), okCov("lizard"), okCov("jscpd"),
			},
			minValue: 21,
			maxValue: 60,
		},
		{
			name: "coverage ok + all semantic absent → graded drop, not critical",
			// blast_radius+instability → a real (non-degenerate) graph, so the
			// measured-dimension ceiling does not bind and this exercises the
			// tool-coverage grading.
			metrics: []diagnostic.MetricResult{
				metric("coverage", 0.95, "strong", "high"),
				metric("blast_radius", 3, "info", "high"),
				metric("instability", 2, "info", "high"),
			},
			// primary present, semantic (scip/gitnexus/lizard/jscpd) all absent
			coverage: []diagnostic.Coverage{okCov("go/packages")},
			wantBand: BandServiceable,
			minValue: 61,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := diagnostic.New()
			d.Metrics = tc.metrics
			d.ToolCoverage = tc.coverage
			meta := dimByName(t, Synthesize(d), DimAnalysisConfidence)

			if !meta.Meta {
				t.Errorf("analysis_confidence should be marked meta")
			}
			if tc.wantBand != "" && meta.Band != tc.wantBand {
				t.Errorf("band = %q (value %d), want %q", meta.Band, meta.Value, tc.wantBand)
			}
			if tc.maxValue > 0 && meta.Value > tc.maxValue {
				t.Errorf("value = %d, want ≤ %d", meta.Value, tc.maxValue)
			}
			if tc.minValue > 0 && meta.Value < tc.minValue {
				t.Errorf("value = %d, want ≥ %d", meta.Value, tc.minValue)
			}
			if got := bandFor(meta.Value); meta.Band != got {
				t.Errorf("band %q does not match value %d (want %q)", meta.Band, meta.Value, got)
			}
		})
	}
}

// TestAnalysisConfidence_PrimaryExtractorTools asserts the meta dimension checks
// the Diagnostic's injected PrimaryExtractorTools list (not a hardcoded literal)
// and falls back to the built-in default when the list is empty.
func TestAnalysisConfidence_PrimaryExtractorTools(t *testing.T) {
	// coverage n/a forces the primary-extractor penalty branch. The only primary
	// tool with ok coverage is "cargo"; all semantic tools are present so they add
	// no penalty.
	base := func() diagnostic.Diagnostic {
		d := diagnostic.New()
		// blast_radius+instability → a non-degenerate graph so the measured-dimension
		// ceiling does not bind; this test isolates the primary-extractor penalty.
		d.Metrics = []diagnostic.MetricResult{
			metric("coverage", 0, "n/a", "low"),
			metric("blast_radius", 3, "info", "high"),
			metric("instability", 2, "info", "high"),
		}
		d.ToolCoverage = []diagnostic.Coverage{
			okCov("cargo"),
			okCov("scip"), okCov("gitnexus"), okCov("lizard"), okCov("jscpd"),
		}
		return d
	}

	// Injected list names the present tool → no primary extractor is absent → no penalty.
	injected := base()
	injected.PrimaryExtractorTools = []string{"cargo"}
	injectedMeta := dimByName(t, Synthesize(injected), DimAnalysisConfidence)
	if injectedMeta.Value != 60 {
		t.Errorf("injected list value = %d, want 60 (no primary absent)", injectedMeta.Value)
	}

	// Empty list → default set (go/packages, dependency-cruiser, grimp), all absent
	// → −45 penalty. Proves the empty-slice fallback uses the built-in default.
	fallback := base() // PrimaryExtractorTools left empty
	fallbackMeta := dimByName(t, Synthesize(fallback), DimAnalysisConfidence)
	if fallbackMeta.Value != 15 {
		t.Errorf("empty-list fallback value = %d, want 15 (default primaries absent)", fallbackMeta.Value)
	}

	if injectedMeta.Value <= fallbackMeta.Value {
		t.Errorf("injected list did not override default: injected=%d fallback=%d",
			injectedMeta.Value, fallbackMeta.Value)
	}
}

// TestAnalysisConfidence_NADimensionRatioCap is the Cal-6 regression: a fully-tooled
// run (coverage 1.0, every semantic tool present) over a real graph must NOT read 100
// when several scorecard dimensions came back n/a — the meta score reflects how many
// dimensions were actually measured, not just whether the tools ran.
func TestAnalysisConfidence_NADimensionRatioCap(t *testing.T) {
	// All tools present and a non-degenerate graph (blast_radius+instability), but no
	// encapsulation, no change history, no fitness scan → 5 of 6 dimensions n/a.
	d := diagnostic.New()
	d.Metrics = []diagnostic.MetricResult{
		metric("coverage", 1.0, "strong", "high"),
		metric("blast_radius", 0, "info", "high"),
		metric("instability", 0, "info", "high"),
	}
	d.ToolCoverage = []diagnostic.Coverage{
		okCov("go/packages"), okCov("scip"), okCov("gitnexus"), okCov("lizard"), okCov("jscpd"),
	}
	naMeta := dimByName(t, Synthesize(d), DimAnalysisConfidence)
	if naMeta.Value >= 100 {
		t.Errorf("meta must be capped below 100 when most dimensions are n/a, got %d", naMeta.Value)
	}
	if naMeta.Value < 50 {
		t.Errorf("a fully-tooled run should stay reasonably confident, got %d", naMeta.Value)
	}

	// A fully-measured run (all dimensions real) must out-score the n/a-heavy run.
	fullMeta := dimByName(t, Synthesize(richDiagnostic()), DimAnalysisConfidence)
	if fullMeta.Value <= naMeta.Value {
		t.Errorf("fully-measured run should out-score the n/a-heavy run: full=%d na=%d", fullMeta.Value, naMeta.Value)
	}
}
