package score

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
)

const (
	metricBlastRadius = "blast_radius"
	sevCritical       = "critical"
	sevLow            = "low"
)

func metric(name string, value float64, band, conf string) diagnostic.MetricResult {
	return diagnostic.MetricResult{
		Name: name, Value: value, Display: strconv.FormatFloat(value, 'f', 2, 64),
		Band: band, Confidence: conf,
	}
}

func nonDegenMetricIndex() metricIndex {
	return metricIndex{metricBlastRadius: metric(metricBlastRadius, 3, "info", "high")}
}

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

func okCov(tool string) diagnostic.Coverage {
	return diagnostic.Coverage{Tool: tool, Status: diagnostic.StatusOK}
}

func richDiagnostic() diagnostic.Diagnostic {
	d := diagnostic.New()
	d.ConfigHash = "deadbeef"
	d.Metrics = []diagnostic.MetricResult{
		metric("coverage", 0.95, "strong", "high"),
		metric("blast_radius", 1, "info", "high"),
	}
	d.Findings = []finding.Finding{
		bcAdv("a", "b", "functional", "cross_module_same_owner", "medium", 5, "medium", "medium", 3),
	}
	d.ToolCoverage = []diagnostic.Coverage{
		okCov("go/packages"), okCov("scip"), okCov("ast-grep"), okCov("jscpd"),
	}
	return d
}

func couplingBalanceDim(t *testing.T, sc Scorecard) Dimension {
	t.Helper()
	for _, d := range sc.Dimensions {
		if d.Name == DimCouplingBalance {
			return d
		}
	}
	t.Fatalf("dimension %q not found", DimCouplingBalance)
	return Dimension{}
}

func TestSynthesize_RuleInvariants(t *testing.T) {
	sc := Synthesize(richDiagnostic())

	if sc.RubricVersion != RubricVersion {
		t.Errorf("rubric_version = %d, want %d", sc.RubricVersion, RubricVersion)
	}
	if len(sc.Dimensions) != 1 {
		t.Fatalf("got %d dimensions, want 1", len(sc.Dimensions))
	}

	cb := sc.Dimensions[0]
	if cb.Name != DimCouplingBalance {
		t.Errorf("dimension[0] = %q, want %q", cb.Name, DimCouplingBalance)
	}

	if got := bandFor(cb.Value); cb.Band != got {
		t.Errorf("%s: band %q does not match value %d (want %q)", cb.Name, cb.Band, cb.Value, got)
	}
	if cb.Value < 0 || cb.Value > 100 {
		t.Errorf("%s: value %d out of [0,100]", cb.Name, cb.Value)
	}
	if len(cb.Evidence) == 0 {
		t.Errorf("%s: dimension has no evidence", cb.Name)
	}
	if (cb.Band == BandServiceable || cb.Band == BandStrong) && cb.Confidence == ConfidenceLow {
		t.Errorf("%s: band %q with low confidence violates high_quality_requires_confidence", cb.Name, cb.Band)
	}

	if sc.Overall != cb.Value {
		t.Errorf("overall = %d, want %d (coupling_balance value)", sc.Overall, cb.Value)
	}
	if sc.OverallBand != bandFor(sc.Overall) {
		t.Errorf("overall band %q does not match overall %d", sc.OverallBand, sc.Overall)
	}
}

func TestSynthesize_Deterministic(t *testing.T) {
	d := richDiagnostic()
	a := Synthesize(d)
	b := Synthesize(d)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("Synthesize not deterministic:\n a=%+v\n b=%+v", a, b)
	}
}

func TestLowConfidenceCap(t *testing.T) {
	d := diagnostic.New()
	d.Metrics = []diagnostic.MetricResult{
		metric("coverage", 0.30, "poor", "low"),
		metric("blast_radius", 3, "info", "high"),
	}
	sc := Synthesize(d)
	cb := couplingBalanceDim(t, sc)
	if cb.Value > 60 {
		t.Errorf("low-confidence / no-edge value not capped: value %d (band %q)", cb.Value, cb.Band)
	}
}

func TestCouplingBalance(t *testing.T) {
	cb := func(edges ...finding.Finding) Dimension {
		d := diagnostic.New()
		d.Findings = edges
		d.Metrics = []diagnostic.MetricResult{metric("blast_radius", 3, "info", "high")}
		return couplingBalanceDim(t, Synthesize(d))
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

	t.Run("waived and baseline edges do not count", func(t *testing.T) {
		withStatus := func(f finding.Finding, s finding.Status) finding.Finding {
			f.Status = s
			return f
		}
		worst := func(from, to string) finding.Finding {
			return bcAdv(from, to, "intrusive", "cross_deploy_unit", "high", 10, "critical", "critical", 10)
		}
		got := cb(
			withStatus(worst("a", "b"), finding.StatusWaived),
			withStatus(worst("c", "d"), finding.StatusBaseline),
		)
		if got.Value <= 40 {
			t.Errorf("suppressed edges value = %d, want > poor (>40); waived/baseline must not penalise", got.Value)
		}
		if got.Value > 60 {
			t.Errorf("suppressed edges value = %d, want ≤60 (no counted edges = unconfirmed, not strong)", got.Value)
		}
	})
}

func TestCouplingBalance_EmptyEdges(t *testing.T) {
	nonDegen := nonDegenMetricIndex()

	t.Run("non-degenerate + no edges → ≤60/low, never strong", func(t *testing.T) {
		got := couplingBalance(nil, nonDegen, nil)
		if got.Value > 60 {
			t.Errorf("value = %d, want ≤60 (no false-green on 0 classified edges)", got.Value)
		}
		if got.Confidence != ConfidenceLow {
			t.Errorf("confidence = %q, want low", got.Confidence)
		}
	})

	t.Run("degenerate graph + no edges → n/a (single-module: no cross-module coupling to measure)", func(t *testing.T) {
		got := couplingBalance(nil, metricIndex{}, nil)
		if got.Band != BandNA {
			t.Errorf("band = %q, want n/a (degenerate graph is unmeasured, not a fabricated 50)", got.Band)
		}
	})

	t.Run("no edges never strong through Synthesize", func(t *testing.T) {
		d := diagnostic.New()
		d.Metrics = []diagnostic.MetricResult{metric("coverage", 0.2, "poor", "low")}
		cb := couplingBalanceDim(t, Synthesize(d))
		if cb.Value > 60 {
			t.Errorf("no-edge coupling_balance = %d, want ≤60 (not false-green)", cb.Value)
		}
	})
}

func TestDegenerateGraph_NoFalseGreen(t *testing.T) {
	d := diagnostic.New()
	d.Metrics = []diagnostic.MetricResult{
		metric("cycle", 0, "strong", "high"),
	}
	sc := Synthesize(d)
	dim := couplingBalanceDim(t, sc)
	if dim.Value > 60 {
		t.Errorf("%s = %d on a degenerate (<2-module) graph, want ≤60 (no false-green)", dim.Name, dim.Value)
	}
	if dim.Confidence != ConfidenceLow {
		t.Errorf("%s confidence = %q on a degenerate graph, want low", dim.Name, dim.Confidence)
	}
}

func TestCouplingBalance_Distribution(t *testing.T) {
	nonDegen := nonDegenMetricIndex()

	summary := func(scored, abstained int, meanBalance float64, bySev map[string]int) *diagnostic.ClassifiedEdgeSummary {
		s := &diagnostic.ClassifiedEdgeSummary{
			Total:       scored + abstained,
			Scored:      scored,
			Abstained:   abstained,
			MeanBalance: meanBalance,
			BySeverity:  bySev,
		}
		return s
	}

	cases := []struct {
		name       string
		sum        *diagnostic.ClassifiedEdgeSummary
		wantMinVal int
		wantMaxVal int
		wantConf   Confidence
		wantBand   Band
	}{
		{
			name:       "all balanced high scored fraction → strong/high",
			sum:        summary(50, 0, 9.0, map[string]int{"none": 50}),
			wantMinVal: 88, wantMaxVal: 90,
			wantConf: ConfidenceHigh,
			wantBand: BandStrong,
		},
		{
			name:       "unknown-heavy 30% scored → low confidence",
			sum:        summary(3, 7, 8.0, map[string]int{sevLow: 3}),
			wantConf:   ConfidenceLow,
			wantMaxVal: 60,
		},
		{
			name:       "zero scored with abstained → n/a (unmeasured, not fabricated mixed)",
			sum:        summary(0, 5, 0.0, nil),
			wantMinVal: 0, wantMaxVal: 0,
			wantConf: ConfidenceLow,
			wantBand: BandNA,
		},
		{
			name:       "zero cross-boundary → n/a (unmeasured, not fabricated mixed)",
			sum:        summary(0, 0, 0.0, nil),
			wantMinVal: 0, wantMaxVal: 0,
			wantConf: ConfidenceLow,
			wantBand: BandNA,
		},
		{
			name:       "critical edge caps at 60",
			sum:        summary(10, 0, 7.0, map[string]int{sevLow: 9, sevCritical: 1}),
			wantMaxVal: 60,
			wantConf:   ConfidenceHigh,
		},
		{
			name:       "low-distance critical → cap at 60 (not distributed-monolith)",
			sum:        summary(20, 0, 7.0, map[string]int{sevLow: 19, sevCritical: 1}),
			wantMinVal: 41,
			wantMaxVal: 60,
			wantConf:   ConfidenceHigh,
		},
		{
			name: "pervasive distributed-monolith → cap at 40",
			sum: func() *diagnostic.ClassifiedEdgeSummary {
				s := summary(20, 0, 7.0, map[string]int{sevLow: 18, sevCritical: 2})
				s.DistributedMonolith = 2
				return s
			}(),
			wantMaxVal: 40,
			wantConf:   ConfidenceHigh,
		},
		{
			name:       "55% scored → medium confidence",
			sum:        summary(11, 9, 8.0, map[string]int{sevLow: 11}),
			wantConf:   ConfidenceMedium,
			wantMinVal: 61,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := finalize(couplingBalance(nil, nonDegen, tc.sum))
			if tc.wantConf != "" && got.Confidence != tc.wantConf {
				t.Errorf("confidence = %q, want %q", got.Confidence, tc.wantConf)
			}
			if tc.wantBand != "" && got.Band != tc.wantBand {
				t.Errorf("band = %q (value %d), want %q", got.Band, got.Value, tc.wantBand)
			}
			if tc.wantMinVal > 0 && got.Value < tc.wantMinVal {
				t.Errorf("value = %d, want ≥%d", got.Value, tc.wantMinVal)
			}
			if tc.wantMaxVal > 0 && got.Value > tc.wantMaxVal {
				t.Errorf("value = %d, want ≤%d", got.Value, tc.wantMaxVal)
			}
			if !got.Band.Unmeasured() && got.Band != bandFor(got.Value) {
				t.Errorf("band %q does not match value %d (want %q)", got.Band, got.Value, bandFor(got.Value))
			}
			if (got.Band == BandServiceable || got.Band == BandStrong) && got.Confidence == ConfidenceLow {
				t.Errorf("band %q with low confidence violates high_quality_requires_confidence", got.Band)
			}
		})
	}
}

func TestCouplingBalance_Distribution_AdvisoryTailIndependent(t *testing.T) {
	nonDegen := nonDegenMetricIndex()

	sum := &diagnostic.ClassifiedEdgeSummary{
		Total: 10, Scored: 10, Abstained: 0,
		MeanBalance: 9.0,
		BySeverity:  map[string]int{"none": 9, sevCritical: 1},
	}
	worstAdvisory := bcAdv("a", "b", "intrusive", "cross_deploy_unit", "high", 1, "critical", "critical", 1)

	got := finalize(couplingBalance([]bcEdge{
		{
			strength: worstAdvisory.MatchedBy["strength"],
			distance: worstAdvisory.MatchedBy["distance"],
			band:     worstAdvisory.MatchedBy["score_band"],
			severity: string(worstAdvisory.Severity),
			effort:   atoiDefault(worstAdvisory.MatchedBy["score_value"], -1),
			count:    1,
		},
	}, nonDegen, sum))

	if got.Value > 60 {
		t.Errorf("critical edge should cap at 60, got %d", got.Value)
	}
	foundWorst := false
	for _, ev := range got.Evidence {
		if strings.Contains(ev, "critical-band edges: 1") {
			foundWorst = true
		}
	}
	if !foundWorst {
		t.Errorf("evidence missing critical-band count: %v", got.Evidence)
	}
}

func TestCouplingBalance_LLMProvenance_LowersConfidence(t *testing.T) {
	nonDegen := nonDegenMetricIndex()

	cases := []struct {
		name        string
		scored      int
		llmApproved int
		wantConf    Confidence
	}{
		{
			name:        "no llm labels → confidence unaffected (high)",
			scored:      10,
			llmApproved: 0,
			wantConf:    ConfidenceHigh,
		},
		{
			name:        "llm labels <20% → confidence unaffected",
			scored:      10,
			llmApproved: 1,
			wantConf:    ConfidenceHigh,
		},
		{
			name:        "llm labels ≥20% → confidence lowered by one band (high→medium)",
			scored:      10,
			llmApproved: 2,
			wantConf:    ConfidenceMedium,
		},
		{
			name:        "llm labels majority → confidence lowered by one band (high→medium)",
			scored:      10,
			llmApproved: 8,
			wantConf:    ConfidenceMedium,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sum := &diagnostic.ClassifiedEdgeSummary{
				Total:       tc.scored,
				Scored:      tc.scored,
				Abstained:   0,
				MeanBalance: 9.0,
				BySeverity:  map[string]int{sevLow: tc.scored},
				LLMApproved: tc.llmApproved,
			}
			got := couplingBalance(nil, nonDegen, sum)
			if got.Confidence != tc.wantConf {
				t.Errorf("confidence = %q, want %q (llmApproved=%d, scored=%d)",
					got.Confidence, tc.wantConf, tc.llmApproved, tc.scored)
			}
		})
	}
}

func TestCouplingBalance_LLMProvenance_EvidenceString(t *testing.T) {
	nonDegen := nonDegenMetricIndex()
	sum := &diagnostic.ClassifiedEdgeSummary{
		Total: 10, Scored: 10, MeanBalance: 9.0,
		BySeverity:  map[string]int{sevLow: 10},
		LLMApproved: 3,
	}
	got := couplingBalance(nil, nonDegen, sum)
	found := false
	for _, ev := range got.Evidence {
		if strings.Contains(ev, "llm-provenance") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected llm-provenance mention in evidence, got: %v", got.Evidence)
	}
}

func TestCouplingBalance_DeclaredExternalEvidence(t *testing.T) {
	nonDegen := nonDegenMetricIndex()

	t.Run("declared-external count surfaced in evidence when present", func(t *testing.T) {
		sum := &diagnostic.ClassifiedEdgeSummary{
			Total:            10,
			Scored:           10,
			MeanBalance:      9.0,
			BySeverity:       map[string]int{sevLow: 10},
			DeclaredExternal: 4,
		}
		got := couplingBalance(nil, nonDegen, sum)

		found := false
		for _, ev := range got.Evidence {
			if strings.Contains(ev, "4 declared external-system edges scored at D=10 (external_systems)") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected declared-external evidence line, got: %v", got.Evidence)
		}
	})

	t.Run("no declared-external mention when count is zero", func(t *testing.T) {
		sum := &diagnostic.ClassifiedEdgeSummary{
			Total:       10,
			Scored:      10,
			MeanBalance: 9.0,
			BySeverity:  map[string]int{sevLow: 10},
		}
		got := couplingBalance(nil, nonDegen, sum)

		for _, ev := range got.Evidence {
			if strings.Contains(ev, "declared external-system edges") {
				t.Errorf("did not expect declared-external evidence line, got: %v", got.Evidence)
			}
		}
	})
}

func TestCouplingBalance_ExternalEdgesExcluded(t *testing.T) {
	nonDegen := nonDegenMetricIndex()

	t.Run("external count surfaced in evidence when present", func(t *testing.T) {
		sum := &diagnostic.ClassifiedEdgeSummary{
			Total:       457,
			Scored:      10,
			Abstained:   0,
			External:    447,
			MeanBalance: 9.0,
			BySeverity:  map[string]int{sevLow: 10},
		}
		got := couplingBalance(nil, nonDegen, sum)

		if got.Confidence != ConfidenceHigh {
			t.Errorf("confidence = %q, want high (internal-only fraction is 100%%)", got.Confidence)
		}

		foundExternal := false
		for _, ev := range got.Evidence {
			if strings.Contains(ev, "447 external/library edges excluded") {
				foundExternal = true
			}
		}
		if !foundExternal {
			t.Errorf("expected external-exclusion line in evidence, got: %v", got.Evidence)
		}

		foundInternal := false
		for _, ev := range got.Evidence {
			if strings.Contains(ev, "internal") {
				foundInternal = true
			}
		}
		if !foundInternal {
			t.Errorf("evidence must mention 'internal' cross-boundary edges, got: %v", got.Evidence)
		}
	})

	t.Run("zero internal scored edges + many external → n/a with external in evidence", func(t *testing.T) {
		sum := &diagnostic.ClassifiedEdgeSummary{
			Total:     300,
			Scored:    0,
			Abstained: 0,
			External:  300,
		}
		got := finalize(couplingBalance(nil, nonDegen, sum))

		if got.Value != 0 {
			t.Errorf("value = %d, want 0 (unmeasured: all edges external)", got.Value)
		}
		if got.Band != BandNA {
			t.Errorf("band = %q, want n/a (coupling unmeasured, not fabricated mixed)", got.Band)
		}
		if got.Confidence != ConfidenceLow {
			t.Errorf("confidence = %q, want low", got.Confidence)
		}
		foundExternal := false
		for _, ev := range got.Evidence {
			if strings.Contains(ev, "300 external/library edges excluded") {
				foundExternal = true
			}
		}
		if !foundExternal {
			t.Errorf("expected external-exclusion line in evidence (zero-internal path), got: %v", got.Evidence)
		}
	})

	t.Run("zero external → evidence has no external line", func(t *testing.T) {
		sum := &diagnostic.ClassifiedEdgeSummary{
			Total:       5,
			Scored:      5,
			Abstained:   0,
			External:    0,
			MeanBalance: 8.0,
			BySeverity:  map[string]int{sevLow: 5},
		}
		got := couplingBalance(nil, nonDegen, sum)

		for _, ev := range got.Evidence {
			if strings.Contains(ev, "external") {
				t.Errorf("unexpected external line in evidence when External==0: %q", ev)
			}
		}
	})

	t.Run("internal confidence unaffected by external count", func(t *testing.T) {
		sum := &diagnostic.ClassifiedEdgeSummary{
			Total:       1080,
			Scored:      80,
			Abstained:   0,
			External:    1000,
			MeanBalance: 8.0,
			BySeverity:  map[string]int{sevLow: 80},
		}
		got := couplingBalance(nil, nonDegen, sum)

		if got.Confidence != ConfidenceHigh {
			t.Errorf("confidence = %q, want high (external edges must not count in scored fraction)", got.Confidence)
		}
	})
}

func TestSynthesize_TSUnresolvedPartial_LowersConfidence(t *testing.T) {
	diagWithTSCoverage := func(unresolved, specifiersSeen int, status string) diagnostic.Diagnostic {
		d := diagnostic.New()
		d.Metrics = []diagnostic.MetricResult{metric("blast_radius", 3, "info", "high")}
		d.ClassifiedEdges = &diagnostic.ClassifiedEdgeSummary{
			Total: 50, Scored: 50, Abstained: 0,
			MeanBalance: 9.0,
			BySeverity:  map[string]int{sevLow: 50},
		}
		d.ToolCoverage = []diagnostic.Coverage{
			{Tool: toolDepCruiser, Status: status, FilesSeen: 10, SpecifiersSeen: specifiersSeen, Unresolved: unresolved},
		}
		return d
	}

	t.Run("unresolved ratio above ceiling caps confidence to medium", func(t *testing.T) {
		cb := couplingBalanceDim(t, Synthesize(diagWithTSCoverage(20, 100, diagnostic.StatusPartial))) // 20%
		if cb.Confidence != ConfidenceMedium {
			t.Errorf("confidence = %q, want medium", cb.Confidence)
		}
		found := false
		for _, ev := range cb.Evidence {
			if strings.Contains(ev, "unresolved-specifier ratio") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected unresolved-ratio mention in evidence, got: %v", cb.Evidence)
		}
	})

	// Also the denominator regression guard: 5/100 specifiers is 5% (under the
	// ceiling) while 5 over the 10 FilesSeen would be 50% — a FilesSeen
	// denominator would cap here and contradict the disclosed specifier ratio.
	t.Run("unresolved ratio within ceiling leaves confidence high", func(t *testing.T) {
		cb := couplingBalanceDim(t, Synthesize(diagWithTSCoverage(5, 100, diagnostic.StatusPartial))) // 5%
		if cb.Confidence != ConfidenceHigh {
			t.Errorf("confidence = %q, want high", cb.Confidence)
		}
	})

	t.Run("ok status never triggers the cap regardless of ratio", func(t *testing.T) {
		cb := couplingBalanceDim(t, Synthesize(diagWithTSCoverage(20, 100, diagnostic.StatusOK)))
		if cb.Confidence != ConfidenceHigh {
			t.Errorf("confidence = %q, want high (status ok, not partial)", cb.Confidence)
		}
	})

	t.Run("specifiers untracked (0) abstains rather than cap on a proxy ratio", func(t *testing.T) {
		cb := couplingBalanceDim(t, Synthesize(diagWithTSCoverage(20, 0, diagnostic.StatusPartial)))
		if cb.Confidence != ConfidenceHigh {
			t.Errorf("confidence = %q, want high (SpecifiersSeen 0 = untracked, no cap)", cb.Confidence)
		}
	})
}

// TestSynthesize_ConfidenceCapsNeverStack pins the minimum-band rule: two
// simultaneous cap triggers (cargo-modules partial + TS unresolved ratio over
// ceiling) land at medium — one step down — never stacked to low.
func TestSynthesize_ConfidenceCapsNeverStack(t *testing.T) {
	d := diagnostic.New()
	d.Metrics = []diagnostic.MetricResult{metric("blast_radius", 3, "info", "high")}
	d.ClassifiedEdges = &diagnostic.ClassifiedEdgeSummary{
		Total: 50, Scored: 50, Abstained: 0,
		MeanBalance: 9.0,
		BySeverity:  map[string]int{sevLow: 50},
	}
	d.ToolCoverage = []diagnostic.Coverage{
		{Tool: "cargo-modules", Status: diagnostic.StatusPartial},
		{Tool: toolDepCruiser, Status: diagnostic.StatusPartial, FilesSeen: 10, SpecifiersSeen: 100, Unresolved: 20},
	}

	cb := couplingBalanceDim(t, Synthesize(d))
	if cb.Confidence != ConfidenceMedium {
		t.Errorf("confidence = %q, want medium (caps are a floor, not cumulative downgrades)", cb.Confidence)
	}
}
