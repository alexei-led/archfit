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
		return dimByName(t, Synthesize(d), DimCouplingBalance)
	}

	t.Run("no edges → high, not flagged", func(t *testing.T) {
		got := cb()
		if got.Value < 81 {
			t.Errorf("no-edges value = %d, want strong (≥81)", got.Value)
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
		// must not penalise the dimension — same view the gate verdict takes.
		got := cb(
			withStatus(worst("a", "b"), finding.StatusExcepted),
			withStatus(worst("c", "d"), finding.StatusBaseline),
		)
		if got.Value < 81 {
			t.Errorf("suppressed edges value = %d, want strong (≥81); excepted/baseline must not count", got.Value)
		}
	})
}

// TestCouplingBalance_EmptyEdgesBase asserts the empty-edges branch reads the
// baseline coverage confidence: low coverage → neutral 50/low (no false-green),
// medium-or-better coverage → the established 90/medium "no unbalanced coupling".
func TestCouplingBalance_EmptyEdgesBase(t *testing.T) {
	t.Run("empty edges + low base → 50/low", func(t *testing.T) {
		got := couplingBalance(nil, ConfidenceLow)
		if got.Value != 50 {
			t.Errorf("value = %d, want 50", got.Value)
		}
		if got.Confidence != ConfidenceLow {
			t.Errorf("confidence = %q, want low", got.Confidence)
		}
	})

	t.Run("empty edges + medium base → 90/medium", func(t *testing.T) {
		got := couplingBalance(nil, ConfidenceMedium)
		if got.Value != 90 {
			t.Errorf("value = %d, want 90", got.Value)
		}
		if got.Confidence != ConfidenceMedium {
			t.Errorf("confidence = %q, want medium", got.Confidence)
		}
	})

	t.Run("low base propagates through Synthesize", func(t *testing.T) {
		// A coverage metric with value 0.2 drives the baseline confidence to low,
		// so an un-analysed repo with no BC edges must not present a strong 90.
		d := diagnostic.New()
		d.Metrics = []diagnostic.MetricResult{metric("coverage", 0.2, "poor", "low")}
		cb := dimByName(t, Synthesize(d), DimCouplingBalance)
		if cb.Value > 60 {
			t.Errorf("low-coverage no-edge coupling_balance = %d, want ≤60 (not false-green)", cb.Value)
		}
	})
}

// TestBoundaryIntegrity_GateViolations asserts active gate findings subtract from
// boundary integrity and are cited as evidence.
func TestBoundaryIntegrity_GateViolations(t *testing.T) {
	clean := diagnostic.New()
	clean.Metrics = []diagnostic.MetricResult{metric("encapsulation", 1.0, "strong", "high")}
	cleanDim := dimByName(t, Synthesize(clean), DimBoundaryIntegrity)

	dirty := diagnostic.New()
	dirty.Metrics = []diagnostic.MetricResult{metric("encapsulation", 1.0, "strong", "high")}
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
			name:    "coverage ok + all semantic absent → graded drop, not critical",
			metrics: []diagnostic.MetricResult{metric("coverage", 0.95, "strong", "high")},
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
