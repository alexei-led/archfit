package initcfg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
)

// Test-local constants to satisfy goconst across the update-report test file.
// Reuses testAlpha ("alpha") from disambiguate_test.go and
// testAnnVolatility ("low") + testSvc ("svc") from existing files.
const (
	testBeta       = "beta"
	testGone       = "gone"
	testSupporting = "supporting"
	testMedium     = "medium"
	testSvcPath    = "internal/svc/**"
	testOldPath    = "old/**"
	testNewPath    = "new/**"
)

// minimalConfigWrap wraps a modules: stanza in a valid .archfit.yaml for round-trip testing.
func minimalConfigWrap(modulesYAML string) string {
	return "version: 1\nlayers:\n  - core\n  - adapter\nmodules:\n" + modulesYAML +
		"rules:\n  - id: no-forbidden-deps\n    type: forbidden_dependency\n    gate: warn\n"
}

// roundTripReport writes yaml to a temp file and loads it via config.Load.
func roundTripReport(t *testing.T, yaml string) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("config.Load rejected YAML: %v\n---\n%s", err, yaml)
	}
	return cfg
}

// extractAddedStanza pulls the indented stanza lines from an ADDED section.
func extractAddedStanza(report string) string {
	var lines []string
	inStanza := false
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, "ADDED") {
			inStanza = true
			continue
		}
		if inStanza {
			if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				break
			}
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Empty
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Empty(t *testing.T) {
	r := UpdateReport{StructuralInSync: true}
	got := RenderUpdateReport(r, nil, nil)
	if !strings.Contains(got, "structurally in sync") {
		t.Errorf("expected in-sync line, got:\n%s", got)
	}
	for _, sec := range []string{"ADDED", "REMOVED", "PATH DRIFT", "UNCLASSIFIED"} {
		if strings.Contains(got, sec) {
			t.Errorf("unexpected section %q in empty report:\n%s", sec, got)
		}
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Added_NoAnn
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Added_NoAnn(t *testing.T) {
	r := UpdateReport{
		Added: []ModuleDef{
			{Name: testSvc, Paths: []string{testSvcPath}, Layer: layerCore},
		},
	}
	layers := []string{layerCore, layerAdapter}
	got := RenderUpdateReport(r, nil, layers)

	if !strings.Contains(got, "ADDED") {
		t.Errorf("missing ADDED section:\n%s", got)
	}
	if !strings.Contains(got, testSvc+":") {
		t.Errorf("missing module key 'svc:':\n%s", got)
	}
	if !strings.Contains(got, testSvcPath) {
		t.Errorf("missing paths glob:\n%s", got)
	}
	if !strings.Contains(got, "layer: "+layerCore) {
		t.Errorf("missing live layer:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Added_WithAnn
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Added_WithAnn(t *testing.T) {
	r := UpdateReport{
		Added: []ModuleDef{
			{Name: testSvc, Paths: []string{testSvcPath}, Layer: layerCore},
		},
	}
	layers := []string{layerCore, layerAdapter}
	ann := map[string]ModuleAnnotation{
		testSvc: {Subdomain: testSupporting, Volatility: testMedium, Layer: layerCore},
	}
	got := RenderUpdateReport(r, ann, layers)

	if !strings.Contains(got, "subdomain: "+testSupporting) {
		t.Errorf("missing live subdomain:\n%s", got)
	}
	if !strings.Contains(got, "volatility: "+testMedium) {
		t.Errorf("missing live volatility:\n%s", got)
	}
	if !strings.Contains(got, "layer: "+layerCore) {
		t.Errorf("missing live layer:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Added_OutOfSetLayer_RenderedAsComment
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Added_OutOfSetLayer_RenderedAsComment(t *testing.T) {
	const outOfSet = "domain" // NOT in allowedLayers
	layers := []string{layerCore, layerAdapter}

	// With ann whose layer is out of set — should be a comment, not live.
	r := UpdateReport{
		Added: []ModuleDef{
			// m.Layer also out of set so resolved layer has no live value
			{Name: testSvc, Paths: []string{testSvcPath}, Layer: outOfSet},
		},
	}
	ann := map[string]ModuleAnnotation{
		testSvc: {Subdomain: layerCore, Volatility: testAnnVolatility, Layer: outOfSet},
	}
	got := RenderUpdateReport(r, ann, layers)

	// ann.Layer is out of set; m.Layer (outOfSet) also not in layers → no live layer.
	if strings.Contains(got, "\n    layer: "+outOfSet+"\n") || strings.Contains(got, "\n  layer: "+outOfSet+"\n") {
		t.Errorf("out-of-set layer written live:\n%s", got)
	}
	// Must appear in a comment.
	if !strings.Contains(got, "# llm layer: "+outOfSet) {
		t.Errorf("out-of-set layer missing from comment:\n%s", got)
	}
	if !strings.Contains(got, "not in layers:") {
		t.Errorf("missing 'not in layers:' note:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Added_StanzaRoundTrips
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Added_StanzaRoundTrips(t *testing.T) {
	const newsvc = "newsvc"
	r := UpdateReport{
		Added: []ModuleDef{
			{Name: newsvc, Paths: []string{"internal/newsvc/**"}, Layer: layerCore},
		},
	}
	layers := []string{layerCore, layerAdapter}
	ann := map[string]ModuleAnnotation{
		newsvc: {Subdomain: testSupporting, Volatility: testMedium, Layer: layerCore},
	}
	got := RenderUpdateReport(r, ann, layers)

	stanzaLines := extractAddedStanza(got)
	fullYAML := minimalConfigWrap(stanzaLines + "\n")
	loaded := roundTripReport(t, fullYAML)

	mod, ok := loaded.Modules[newsvc]
	if !ok {
		t.Fatalf("round-trip: module %q not found in loaded config\n---\n%s", newsvc, fullYAML)
	}
	if mod.Subdomain != testSupporting {
		t.Errorf("round-trip subdomain = %q, want %q", mod.Subdomain, testSupporting)
	}
	if mod.Volatility != testMedium {
		t.Errorf("round-trip volatility = %q, want %q", mod.Volatility, testMedium)
	}
	if mod.Layer != layerCore {
		t.Errorf("round-trip layer = %q, want %q", mod.Layer, layerCore)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Removed
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Removed(t *testing.T) {
	r := UpdateReport{
		Removed: []ExistingModule{
			{Name: "old", Paths: []string{testOldPath}},
		},
	}
	got := RenderUpdateReport(r, nil, nil)

	if !strings.Contains(got, "REMOVED") {
		t.Errorf("missing REMOVED section:\n%s", got)
	}
	if !strings.Contains(got, "old") {
		t.Errorf("missing module name 'old':\n%s", got)
	}
	if !strings.Contains(got, "verify or remove") {
		t.Errorf("missing 'verify or remove' note:\n%s", got)
	}
	if strings.Contains(got, "structurally in sync") {
		t.Errorf("unexpected in-sync line when Removed is non-empty:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_PathDrift
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_PathDrift(t *testing.T) {
	r := UpdateReport{
		PathDrift: []PathDelta{
			{
				Name:            testSvc,
				ConfigPaths:     []string{testOldPath},
				DiscoveredPaths: []string{testNewPath},
			},
		},
	}
	got := RenderUpdateReport(r, nil, nil)

	if !strings.Contains(got, "PATH DRIFT") {
		t.Errorf("missing PATH DRIFT section:\n%s", got)
	}
	if !strings.Contains(got, testSvc) {
		t.Errorf("missing module name:\n%s", got)
	}
	if !strings.Contains(got, testOldPath) {
		t.Errorf("missing config path:\n%s", got)
	}
	if !strings.Contains(got, testNewPath) {
		t.Errorf("missing discovered path:\n%s", got)
	}
	// Must state explicitly that --apply replaces paths and a backup is written.
	if !strings.Contains(got, "--apply") {
		t.Errorf("PATH DRIFT section missing '--apply' mention:\n%s", got)
	}
	if !strings.Contains(got, "REPLACES") {
		t.Errorf("PATH DRIFT section missing 'REPLACES' note:\n%s", got)
	}
	if !strings.Contains(got, "backup") {
		t.Errorf("PATH DRIFT section missing backup mention:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Unclassified_NoAnn
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Unclassified_NoAnn(t *testing.T) {
	r := UpdateReport{
		Unclassified:     []string{testSvc},
		StructuralInSync: true,
	}
	got := RenderUpdateReport(r, nil, nil)

	if !strings.Contains(got, "UNCLASSIFIED") {
		t.Errorf("missing UNCLASSIFIED section:\n%s", got)
	}
	if !strings.Contains(got, testSvc) {
		t.Errorf("missing module name:\n%s", got)
	}
	if !strings.Contains(got, "--llm") {
		t.Errorf("missing '--llm' suggestion:\n%s", got)
	}
	if !strings.Contains(got, "structurally in sync") {
		t.Errorf("missing in-sync line:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Unclassified_WithAnn
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Unclassified_WithAnn(t *testing.T) {
	r := UpdateReport{
		Unclassified:     []string{testSvc},
		StructuralInSync: true,
	}
	ann := map[string]ModuleAnnotation{
		testSvc: {Subdomain: testSupporting, Volatility: testMedium, Layer: layerCore},
	}
	got := RenderUpdateReport(r, ann, []string{layerCore})

	if !strings.Contains(got, testSupporting) {
		t.Errorf("missing llm subdomain in UNCLASSIFIED:\n%s", got)
	}
	if !strings.Contains(got, testMedium) {
		t.Errorf("missing llm volatility in UNCLASSIFIED:\n%s", got)
	}
	if !strings.Contains(got, layerCore) {
		t.Errorf("missing llm layer in UNCLASSIFIED:\n%s", got)
	}
	// Should NOT suggest --llm when ann has the module.
	if strings.Contains(got, "--llm") {
		t.Errorf("unexpected '--llm' suggestion when ann present:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_StructurallyInSync
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_StructurallyInSync(t *testing.T) {
	r := UpdateReport{StructuralInSync: true}
	got := RenderUpdateReport(r, nil, nil)
	if !strings.Contains(got, "structurally in sync") {
		t.Errorf("expected 'structurally in sync', got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_DeployUnitHints
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_DeployUnitHints(t *testing.T) {
	r := UpdateReport{
		StructuralInSync: true,
		DeployUnitSuggestions: []DeployUnitSuggestion{
			{Module: "web", Unit: "web-service", Source: "cmd/web"},
		},
	}

	got := RenderUpdateReport(r, nil, nil)
	for _, want := range []string{
		"DEPLOY UNIT HINTS (1 deterministic config proposal(s) — apply manually after review)",
		"web:",
		"deploy_unit: web-service",
		"source: cmd/web",
		"structurally in sync",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_Deterministic
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_Deterministic(t *testing.T) {
	r := UpdateReport{
		Added: []ModuleDef{
			{Name: testAlpha, Paths: []string{testAlpha + "/**"}, Layer: layerCore},
			{Name: testBeta, Paths: []string{testBeta + "/**"}, Layer: layerAdapter},
		},
		Removed: []ExistingModule{
			{Name: testGone, Paths: []string{testGone + "/**"}},
		},
		PathDrift: []PathDelta{
			{Name: testSvc, ConfigPaths: []string{testOldPath}, DiscoveredPaths: []string{testNewPath}},
		},
		Unclassified: []string{"partial"},
	}
	layers := []string{layerCore, layerAdapter}

	out1 := RenderUpdateReport(r, nil, layers)
	out2 := RenderUpdateReport(r, nil, layers)
	if out1 != out2 {
		t.Errorf("RenderUpdateReport is not deterministic:\n--- run1 ---\n%s\n--- run2 ---\n%s", out1, out2)
	}
}

// ---------------------------------------------------------------------------
// TestRenderUpdateReport_EmptySectionsOmitted
// ---------------------------------------------------------------------------

func TestRenderUpdateReport_EmptySectionsOmitted(t *testing.T) {
	r := UpdateReport{
		Removed: []ExistingModule{{Name: testGone}},
	}
	got := RenderUpdateReport(r, nil, nil)

	for _, sec := range []string{"ADDED", "PATH DRIFT", "UNCLASSIFIED", "structurally in sync"} {
		if strings.Contains(got, sec) {
			t.Errorf("empty section %q should be omitted:\n%s", sec, got)
		}
	}
	if !strings.Contains(got, "REMOVED") {
		t.Errorf("REMOVED section missing:\n%s", got)
	}
}
