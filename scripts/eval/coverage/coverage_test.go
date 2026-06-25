package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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
		metricNames: map[string]bool{probeMetricUnsafeDensity: true},
		factKinds:   map[string]bool{},
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
		metricNames: map[string]bool{},
		factKinds:   map[string]bool{probeKindUnsafeOp: true},
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

func TestClassifyFinding_NotSurfacedWhenSignalAbsent(t *testing.T) {
	rd := &repoData{
		// existing metrics — none are the future detector names
		metricNames: map[string]bool{
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
			{Name: "encapsulation"},
			{Name: "structural_weight"},
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
	if !rd.metricNames["encapsulation"] {
		t.Error("expected encapsulation in metricNames")
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
		Metrics:     []metricEntry{{Name: probeMetricTestDensity}},
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
	if !result["yazi"].metricNames[probeMetricTestDensity] {
		t.Error("expected test_density metric for yazi")
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

// TestBaseline_ExpectedCounts verifies the 20-row inventory produces the expected
// 1 agree / 8 llm-routed / 11 not-surfaced split when no detector data is present.
func TestBaseline_ExpectedCounts(t *testing.T) {
	// Empty repoData for all repos simulates baseline (no detectors shipped).
	emptyRepo := &repoData{
		metricNames: map[string]bool{},
		factKinds:   map[string]bool{},
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
	wantLLM := 8
	wantNotSurfaced := 11

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
