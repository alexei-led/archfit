package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const bandInformational = "informational"

// writeFixture writes a synthetic full.json to dir/<name>.json and returns the path.
func writeFixture(t *testing.T, dir string, name string, doc fullJSON) string {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", p, err)
	}
	return p
}

func TestClassifyFinding_AgreeHardcoded(t *testing.T) {
	f := Finding{
		ID:         "3.1",
		BaseStatus: statusAgree,
	}
	// rd is nil — agree is hardcoded, no probe needed.
	got := ClassifyFinding(f, nil)
	if got != statusAgree {
		t.Errorf("got %q, want %q", got, statusAgree)
	}
}

func TestClassifyFinding_LLMRoutedHardcoded(t *testing.T) {
	f := Finding{
		ID:         "1.1",
		BaseStatus: statusLLMRoutedDesign,
	}
	got := ClassifyFinding(f, nil)
	if got != statusLLMRoutedDesign {
		t.Errorf("got %q, want %q", got, statusLLMRoutedDesign)
	}
}

func TestClassifyFinding_NotSurfacedWhenNoData(t *testing.T) {
	f := Finding{
		ID:          findingID61,
		ProbeMetric: probeMetricUnsafeDensity,
		ProbeKind:   probeKindUnsafeOp,
	}
	// nil rd = repo not scanned.
	got := ClassifyFinding(f, nil)
	if got != statusNotSurfaced {
		t.Errorf("got %q, want %q", got, statusNotSurfaced)
	}
}

func TestClassifyFinding_SurfacedByMetric(t *testing.T) {
	rd := &repoData{
		metricPosSignal: map[string]bool{probeMetricUnsafeDensity: true},
		factKinds:       map[string]bool{},
	}
	f := Finding{
		ID:          findingID61,
		ProbeMetric: probeMetricUnsafeDensity,
		ProbeKind:   probeKindUnsafeOp,
	}
	got := ClassifyFinding(f, rd)
	if got != statusSurfaced {
		t.Errorf("got %q, want %q", got, statusSurfaced)
	}
}

func TestClassifyFinding_SurfacedByKind(t *testing.T) {
	rd := &repoData{
		metricPosSignal: map[string]bool{},
		factKinds:       map[string]bool{probeKindUnsafeOp: true},
	}
	f := Finding{
		ID:          findingID61,
		ProbeMetric: probeMetricUnsafeDensity,
		ProbeKind:   probeKindUnsafeOp,
	}
	got := ClassifyFinding(f, rd)
	if got != statusSurfaced {
		t.Errorf("got %q, want %q", got, statusSurfaced)
	}
}

// TestClassifyFinding_NotSurfacedWhenMetricNA verifies that a metric present in
// full.json with band "n/a" does NOT count as surfaced. The detector ran but
// produced no meaningful result for that repo.
func TestClassifyFinding_NotSurfacedWhenMetricNA(t *testing.T) {
	// metric band=n/a → not in either signal map
	rd := &repoData{
		metricPosSignal:  map[string]bool{},
		metricZeroSignal: map[string]bool{},
		factKinds:        map[string]bool{},
	}
	f := Finding{
		ID:          findingID61,
		ProbeMetric: probeMetricUnsafeDensity,
	}
	got := ClassifyFinding(f, rd)
	if got != statusNotSurfaced {
		t.Errorf("got %q, want %q (metric n/a must not count as surfaced)", got, statusNotSurfaced)
	}
}

func TestClassifyFinding_NotSurfacedWhenSignalAbsent(t *testing.T) {
	rd := &repoData{
		// existing metrics — none are the future detector names
		metricPosSignal: map[string]bool{
			"encapsulation": true, "structural_weight": true, "coverage": true,
		},
		factKinds: map[string]bool{"function": true, "struct": true},
	}
	f := Finding{
		ID:          "8.1",
		ProbeMetric: "panic_density",
		ProbeKind:   "panic_op",
	}
	got := ClassifyFinding(f, rd)
	if got != statusNotSurfaced {
		t.Errorf("got %q, want %q", got, statusNotSurfaced)
	}
}

func TestParseFullJSON_BasicMetricsAndFacts(t *testing.T) {
	dir := t.TempDir()
	doc := fullJSON{
		Metrics: []metricEntry{
			{Name: "encapsulation", Band: bandNA, Value: 0},
			{Name: "structural_weight", Band: "strong", Value: 42},
		},
		SyntaxFacts: []syntaxFact{
			{Kind: "struct"},
			{Kind: "function"},
		},
	}
	path := writeFixture(t, dir, "full.json", doc)

	rd, err := parseFullJSON(path)
	if err != nil {
		t.Fatalf("parseFullJSON: %v", err)
	}
	// encapsulation has band n/a → in neither map.
	if rd.metricPosSignal["encapsulation"] {
		t.Error("encapsulation has band n/a — must not be in metricPosSignal")
	}
	if rd.metricZeroSignal["encapsulation"] {
		t.Error("encapsulation has band n/a — must not be in metricZeroSignal")
	}
	// structural_weight has value=42 → positive signal.
	if !rd.metricPosSignal["structural_weight"] {
		t.Error("structural_weight has band strong, value=42 — must be in metricPosSignal")
	}
	if !rd.factKinds["struct"] {
		t.Error("expected struct in factKinds")
	}
}

func TestParseFullJSON_MissingFile(t *testing.T) {
	_, err := parseFullJSON("/nonexistent/path/full.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseFullJSON_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(p, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := parseFullJSON(p)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadRepoData_FlatLayout(t *testing.T) {
	dir := t.TempDir()
	// flat layout: <dir>/<repo>-archfit.json
	doc := fullJSON{
		Metrics:     []metricEntry{{Name: probeMetricTestDensity, Band: bandInformational, Value: 5}},
		SyntaxFacts: []syntaxFact{{Kind: probeKindTestFn}},
	}
	writeFixture(t, dir, "yazi-archfit.json", doc)
	writeFixture(t, dir, "herdr-archfit.json", fullJSON{})

	result, err := loadRepoData(dir)
	if err != nil {
		t.Fatalf("loadRepoData: %v", err)
	}
	if result["yazi"] == nil {
		t.Error("expected yazi entry")
	}
	if !result["yazi"].metricPosSignal[probeMetricTestDensity] {
		t.Error("expected test_density metric for yazi in metricPosSignal")
	}
	if result["herdr"] == nil {
		t.Error("expected herdr entry")
	}
}

func TestLoadRepoData_SubdirLayout(t *testing.T) {
	dir := t.TempDir()
	// subdir layout: <dir>/<repo>/full.json
	repoDir := filepath.Join(dir, "pumba")
	if err := os.Mkdir(repoDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	doc := fullJSON{
		SyntaxFacts: []syntaxFact{{Kind: "test_import"}},
	}
	writeFixture(t, repoDir, "full.json", doc)

	result, err := loadRepoData(dir)
	if err != nil {
		t.Fatalf("loadRepoData: %v", err)
	}
	if result["pumba"] == nil {
		t.Error("expected pumba entry")
	}
	if !result["pumba"].factKinds["test_import"] {
		t.Error("expected test_import fact kind for pumba")
	}
}

// TestAllSurfaced_NoFindingUnclassified verifies the coverage table is internally
// consistent: when all detectors have shipped (all probe metrics/kinds present),
// every finding classifies to exactly one of {surfaced | llm-routed-by-design | agree}
// and none is left not-surfaced (i.e. the table is complete).
//
// This is the acceptance gate for Task 12: "no finding left unclassified."
// Absence-signal probes (SurfaceWhenZero=true) surface on metricZeroSignal; the
// synthetic fullRepo populates both signal maps so polarity does not hide findings.
func TestAllSurfaced_NoFindingUnclassified(t *testing.T) {
	// Build a repoData per repo that contains every probe signal declared in the inventory.
	// Populate both maps so absence-signal and positive-signal probes both satisfy.
	allPosMetrics := map[string]bool{}
	allZeroMetrics := map[string]bool{}
	allKinds := map[string]bool{}
	for _, f := range inventory {
		if f.ProbeMetric != "" {
			if f.SurfaceWhenZero {
				allZeroMetrics[f.ProbeMetric] = true
			} else {
				allPosMetrics[f.ProbeMetric] = true
			}
		}
		if f.ProbeKind != "" {
			allKinds[f.ProbeKind] = true
		}
	}
	fullRepo := &repoData{
		metricPosSignal:  allPosMetrics,
		metricZeroSignal: allZeroMetrics,
		factKinds:        allKinds,
	}
	repos := map[string]*repoData{
		repoArchfit:   fullRepo,
		repoCcgram:    fullRepo,
		repoCodegraph: fullRepo,
		repoHerdr:     fullRepo,
		repoPumba:     fullRepo,
		repoYazi:      fullRepo,
	}

	// Every finding must be classified — none must be not-surfaced.
	for _, f := range inventory {
		status := ClassifyFinding(f, repos[f.Repo])
		if status == statusNotSurfaced {
			t.Errorf("finding %s (%s) is not-surfaced with all detectors shipped: probe metric=%q kind=%q surfaceWhenZero=%v",
				f.ID, f.Title, f.ProbeMetric, f.ProbeKind, f.SurfaceWhenZero)
		}
		if status != statusSurfaced && status != statusAgree && status != statusLLMRoutedDesign {
			t.Errorf("finding %s (%s) has unexpected status %q", f.ID, f.Title, status)
		}
	}
}

// TestAllSurfaced_NoUnknownStatus verifies the inventory has no rows that would
// produce an unexpected or empty status (guards against typos in BaseStatus).
func TestAllSurfaced_NoUnknownStatus(t *testing.T) {
	validStatuses := map[string]bool{
		statusSurfaced:        true,
		statusLLMRoutedDesign: true,
		statusAgree:           true,
		statusNotSurfaced:     true,
	}
	emptyRepo := &repoData{
		metricPosSignal:  map[string]bool{},
		metricZeroSignal: map[string]bool{},
		factKinds:        map[string]bool{},
	}
	repos := map[string]*repoData{
		repoArchfit:   emptyRepo,
		repoCcgram:    emptyRepo,
		repoCodegraph: emptyRepo,
		repoHerdr:     emptyRepo,
		repoPumba:     emptyRepo,
		repoYazi:      emptyRepo,
	}

	for _, f := range inventory {
		status := ClassifyFinding(f, repos[f.Repo])
		if !validStatuses[status] {
			t.Errorf("finding %s (%s) produced unknown status %q", f.ID, f.Title, status)
		}
		if status == "" {
			t.Errorf("finding %s (%s) produced empty status", f.ID, f.Title)
		}
	}
}

// TestBaseline_ExpectedCounts verifies the 20-row inventory produces the expected
// 1 agree / 9 llm-routed / 10 not-surfaced split when no detector data is present.
func TestBaseline_ExpectedCounts(t *testing.T) {
	// Empty repoData for all repos simulates baseline (no detectors shipped).
	emptyRepo := &repoData{
		metricPosSignal:  map[string]bool{},
		metricZeroSignal: map[string]bool{},
		factKinds:        map[string]bool{},
	}
	repos := map[string]*repoData{
		repoArchfit:   emptyRepo,
		repoCcgram:    emptyRepo,
		repoCodegraph: emptyRepo,
		repoHerdr:     emptyRepo,
		repoPumba:     emptyRepo,
		repoYazi:      emptyRepo,
	}

	counts := map[string]int{}
	for _, f := range inventory {
		status := ClassifyFinding(f, repos[f.Repo])
		counts[status]++
	}

	wantAgree := 1
	wantLLM := 9
	wantNotSurfaced := 10

	if counts[statusAgree] != wantAgree {
		t.Errorf("agree: got %d, want %d", counts[statusAgree], wantAgree)
	}
	if counts[statusLLMRoutedDesign] != wantLLM {
		t.Errorf("llm-routed: got %d, want %d", counts[statusLLMRoutedDesign], wantLLM)
	}
	if counts[statusNotSurfaced] != wantNotSurfaced {
		t.Errorf("not-surfaced: got %d, want %d", counts[statusNotSurfaced], wantNotSurfaced)
	}
	if counts[statusSurfaced] != 0 {
		t.Errorf("surfaced: got %d, want 0", counts[statusSurfaced])
	}
	total := counts[statusAgree] + counts[statusLLMRoutedDesign] + counts[statusNotSurfaced] + counts[statusSurfaced]
	if total != 20 {
		t.Errorf("total rows: got %d, want 20", total)
	}
}

func TestClassifyFinding_NotSurfacedWhenMetricValueZero(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.json", fullJSON{
		Metrics: []metricEntry{
			{Name: probeMetricUnsafeDensity, Band: bandInformational, Value: 0},
		},
	})
	rd, err := parseFullJSON(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatalf("parseFullJSON: %v", err)
	}
	f := Finding{ID: findingID61, ProbeMetric: probeMetricUnsafeDensity}
	got := ClassifyFinding(f, rd)
	if got != statusNotSurfaced {
		t.Errorf("got %q, want %q (value=0 default-polarity must not be surfaced)", got, statusNotSurfaced)
	}
}

func TestClassifyFinding_SurfacedWhenMetricValuePositive(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.json", fullJSON{
		Metrics: []metricEntry{
			{Name: probeMetricUnsafeDensity, Band: bandInformational, Value: 1.5},
		},
	})
	rd, err := parseFullJSON(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatalf("parseFullJSON: %v", err)
	}
	f := Finding{ID: findingID61, ProbeMetric: probeMetricUnsafeDensity}
	got := ClassifyFinding(f, rd)
	if got != statusSurfaced {
		t.Errorf("got %q, want %q (value>0 must be surfaced)", got, statusSurfaced)
	}
}

// TestClassifyFinding_AbsenceSignal_SurfacedWhenZero is the key acceptance test for
// finding 13.2 "No unit tests (herdr)": test_density == 0 (detector ran, found zero
// tests) IS the signal. The probe must surface on value==0, not on value>0.
func TestClassifyFinding_AbsenceSignal_SurfacedWhenZero(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.json", fullJSON{
		Metrics: []metricEntry{
			// test_density ran, band is real, but value = 0 — no tests found.
			{Name: probeMetricTestDensity, Band: bandInformational, Value: 0},
		},
	})
	rd, err := parseFullJSON(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatalf("parseFullJSON: %v", err)
	}
	// SurfaceWhenZero=true: this is an absence-signal probe — zero IS the finding.
	f := Finding{
		ID:              findingID132,
		ProbeMetric:     probeMetricTestDensity,
		SurfaceWhenZero: true,
	}
	got := ClassifyFinding(f, rd)
	if got != statusSurfaced {
		t.Errorf("got %q, want %q (value=0 absence-signal must be surfaced)", got, statusSurfaced)
	}
}

// TestClassifyFinding_AbsenceSignal_NotSurfacedWhenPositive verifies that an
// absence-signal probe (SurfaceWhenZero=true) does NOT surface when value > 0
// (tests were found — the "no tests" finding does not apply).
func TestClassifyFinding_AbsenceSignal_NotSurfacedWhenPositive(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.json", fullJSON{
		Metrics: []metricEntry{
			{Name: probeMetricTestDensity, Band: bandInformational, Value: 3.7},
		},
	})
	rd, err := parseFullJSON(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatalf("parseFullJSON: %v", err)
	}
	f := Finding{
		ID:              findingID132,
		ProbeMetric:     probeMetricTestDensity,
		SurfaceWhenZero: true,
	}
	got := ClassifyFinding(f, rd)
	if got != statusNotSurfaced {
		t.Errorf("got %q, want %q (absence-signal must not surface when value>0)", got, statusNotSurfaced)
	}
}

// TestClassifyFinding_AbsenceSignal_NotSurfacedWhenToolAbsent verifies that
// tool-absent (metric not in JSON) is NEVER surfaced, even for absence-signal probes.
func TestClassifyFinding_AbsenceSignal_NotSurfacedWhenToolAbsent(t *testing.T) {
	rd := &repoData{
		metricPosSignal:  map[string]bool{},
		metricZeroSignal: map[string]bool{},
		factKinds:        map[string]bool{},
	}
	f := Finding{
		ID:              findingID132,
		ProbeMetric:     probeMetricTestDensity,
		SurfaceWhenZero: true,
	}
	got := ClassifyFinding(f, rd)
	if got != statusNotSurfaced {
		t.Errorf("got %q, want %q (tool absent must never be surfaced)", got, statusNotSurfaced)
	}
}

// TestClassifyFinding_AbsenceSignal_NotSurfacedWhenNA verifies that band "n/a"
// (tool ran but metric is not applicable) is NEVER surfaced for absence-signal probes.
func TestClassifyFinding_AbsenceSignal_NotSurfacedWhenNA(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.json", fullJSON{
		Metrics: []metricEntry{
			{Name: probeMetricTestDensity, Band: bandNA, Value: 0},
		},
	})
	rd, err := parseFullJSON(filepath.Join(dir, "test.json"))
	if err != nil {
		t.Fatalf("parseFullJSON: %v", err)
	}
	f := Finding{
		ID:              findingID132,
		ProbeMetric:     probeMetricTestDensity,
		SurfaceWhenZero: true,
	}
	got := ClassifyFinding(f, rd)
	if got != statusNotSurfaced {
		t.Errorf("got %q, want %q (band n/a must never be surfaced)", got, statusNotSurfaced)
	}
}
