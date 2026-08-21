package initcfg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/model/module"
)

// Test-local constants to satisfy goconst across initcfg tests.
const (
	testAnnVolatility  = "low"
	testEvidenceREADME = "doc:README.md"
	testExtractPath    = "internal/extract/**"
	testSanitizeFooBar = "foo bar"
)

// baseCfg is a reusable DiscoveredConfig for annotation tests.
// cfg.Layers contains "core" and "adapter" (derived from the two modules),
// so "core" IS in allowedLayers and "domain" is NOT.
func annotationBaseCfg() DiscoveredConfig {
	return DiscoveredConfig{
		ModulePath: testExampleMod,
		HasGo:      true,
		Layers:     []string{layerCore, layerAdapter},
		Modules: []ModuleDef{
			{
				Name:   testClassify,
				Paths:  []string{testClassifyPath},
				Public: []string{"internal/classify"},
				Layer:  layerCore,
			},
			{
				Name:  adapterExtract,
				Paths: []string{testExtractPath},
				Layer: layerAdapter,
			},
		},
	}
}

// roundTrip writes yaml to a temp file and loads it via config.Load.
func roundTrip(t *testing.T, yaml string) config.Config {
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

// ---------------------------------------------------------------------------
// TestRender_NilAnnotation_ByteIdentical
// ---------------------------------------------------------------------------

// TestRender_NilAnnotation_ByteIdentical verifies that Render(cfg, nil, false)
// produces byte-identical output regardless of the apply flag value — i.e. nil
// annotation always falls through to the original code path.
func TestRender_NilAnnotation_ByteIdentical(t *testing.T) {
	cfg := annotationBaseCfg()
	out1 := Render(cfg, nil, false)
	out2 := Render(cfg, nil, true)
	if out1 != out2 {
		t.Errorf("nil annotation: apply=false vs apply=true outputs differ\n--- false ---\n%s\n--- true ---\n%s", out1, out2)
	}
	// Must not contain any annotation comment markers injected by writeModuleStanza.
	// Use precise patterns to avoid matching the static tools-section LLM config template.
	for _, marker := range []string{"# subdomain:", "# volatility:", "# llm layer:", "# llm: consider renaming"} {
		if strings.Contains(out1, marker) {
			t.Errorf("nil annotation output contains annotation marker %q\n%s", marker, out1)
		}
	}
	// Must contain the live layer.
	if !strings.Contains(out1, "layer: "+layerCore) {
		t.Errorf("nil annotation output missing live layer; got:\n%s", out1)
	}
}

// ---------------------------------------------------------------------------
// TestRender_PlanMode_CommentedAnnotation
// ---------------------------------------------------------------------------

// TestRender_PlanMode_CommentedAnnotation verifies that apply=false writes
// subdomain/volatility as comments and does NOT write them as live fields.
func TestRender_PlanMode_CommentedAnnotation(t *testing.T) {
	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			Subdomain:  layerCore,
			Volatility: testAnnVolatility,
			Layer:      layerCore, // in allowedLayers
		},
	}
	out := Render(cfg, ann, false)

	// Must have commented subdomain/volatility.
	if !strings.Contains(out, "# subdomain: "+layerCore) {
		t.Errorf("plan mode missing commented subdomain:\n%s", out)
	}
	if !strings.Contains(out, "# volatility: "+testAnnVolatility) {
		t.Errorf("plan mode missing commented volatility:\n%s", out)
	}
	// Must NOT have live subdomain: or volatility:.
	for _, live := range []string{"\n  subdomain: " + layerCore, "\n  volatility: " + testAnnVolatility} {
		if strings.Contains(out, live) {
			t.Errorf("plan mode must not contain live field %q:\n%s", live, out)
		}
	}
	// Layer is in set — written live.
	if !strings.Contains(out, "layer: "+layerCore) {
		t.Errorf("plan mode missing live layer:\n%s", out)
	}
	// Round-trip: comments are inert — config.Load should succeed.
	roundTrip(t, out)
}

func TestRender_PlanMode_RendersAnnotationMetadata(t *testing.T) {
	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			Subdomain:    layerCore,
			Volatility:   testAnnVolatility,
			Rationale:    "docs describe the classify boundary",
			EvidenceRefs: []string{testEvidenceREADME},
			Basis:        DraftBasisSemanticJudgment,
		},
	}
	out := Render(cfg, ann, false)
	for _, want := range []string{
		"# basis: semantic_judgment",
		"# evidence_refs: " + testEvidenceREADME,
		"# rationale: docs describe the classify boundary",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan mode missing metadata %q:\n%s", want, out)
		}
	}
	roundTrip(t, out)
}

func TestRender_PlanMode_RendersRuleSuggestionsAsComments(t *testing.T) {
	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			RuleSuggestions: []RuleSuggestion{{
				ID:           "no-classify-to-extract",
				Type:         "forbidden_dependency",
				Gate:         "warn",
				From:         "internal/classify/**",
				To:           testExtractPath,
				Rationale:    "classify should stay core",
				EvidenceRefs: []string{testEvidenceREADME},
				Basis:        DraftBasisSemanticJudgment,
			}},
		},
	}
	out := Render(cfg, ann, false)
	for _, want := range []string{
		"# LLM rule suggestions",
		"# - type: forbidden_dependency",
		"#   id: no-classify-to-extract",
		"#   source_module: classify",
		"#   evidence_refs: " + testEvidenceREADME,
		"#   basis: semantic_judgment",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rule suggestion output missing %q:\n%s", want, out)
		}
	}
	loaded := roundTrip(t, out)
	if len(loaded.Rules) != 1 {
		t.Fatalf("commented rule suggestions must stay inert, loaded rules = %+v", loaded.Rules)
	}
}

func TestRenderAppliedReview_PrefixesAnnotationMetadataWithPlus(t *testing.T) {
	r := UpdateReport{Unclassified: []string{testClassify}}
	ann := map[string]ModuleAnnotation{
		testClassify: {
			Subdomain:    layerCore,
			Volatility:   testAnnVolatility,
			Basis:        DraftBasisSemanticJudgment,
			EvidenceRefs: []string{testEvidenceREADME},
			Rationale:    "docs describe the classify boundary",
		},
	}
	out := RenderAppliedReview(r, ann)
	for _, want := range []string{
		"+ subdomain: " + layerCore,
		"+ volatility: " + testAnnVolatility,
		"+ basis: semantic_judgment",
		"+ evidence_refs: " + testEvidenceREADME,
		"+ rationale: docs describe the classify boundary",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("review output missing %q:\n%s", want, out)
		}
	}

	// Everything --apply refuses to write is review-only, not just the semantic
	// proposals. When apply DID write an edit, this appendix was the only output
	// and it showed none of these, so a run with an edit disclosed less than the
	// same run without --apply.
	t.Run("renders every review-only module section", func(t *testing.T) {
		full := UpdateReport{
			Issues:       []ModuleIssue{newModuleIssue(IssueMissingOwner, "billing")},
			Unclassified: []string{"shipping"},
			Pathless:     []string{testPathlessModule},
			Removed:      []ExistingModule{{Name: testGhostModule}},
			NameDrift:    []NameDrift{{ConfigName: "internal/foo", DiscoveredName: "foo", Paths: []string{"internal/foo/**"}}},
		}
		got := RenderAppliedReview(full, nil)
		for _, want := range []string{
			"ISSUES", "billing", IssueMissingOwner,
			testUnclassifiedSection, "shipping",
			"UNCHECKED", testPathlessModule,
			"UNMATCHED", testGhostModule,
			"NAME DRIFT", "internal/foo",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("applied review missing %q:\n%s", want, got)
			}
		}
	})

	// The same sections stay in the preview: one helper, two renderers.
	t.Run("preview renders the unchecked section too", func(t *testing.T) {
		got := RenderUpdateReport(UpdateReport{Pathless: []string{testPathlessModule}}, nil, nil)
		if !strings.Contains(got, "UNCHECKED") || !strings.Contains(got, testPathlessModule) {
			t.Errorf("preview missing the unchecked section:\n%s", got)
		}
	})
}

func TestRender_PlanMode_RendersExternalSystemSuggestionsAsComments(t *testing.T) {
	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			ExternalSystemSuggestions: []ExternalSystemSuggestion{{
				Name:         "payments-vendor",
				Targets:      []string{"github.com/vendor/sdk/**"},
				Volatility:   testAnnVolatility,
				Rationale:    "README names the vendor seam",
				EvidenceRefs: []string{testEvidenceREADME},
				Basis:        DraftBasisSemanticJudgment,
			}},
		},
	}
	out := Render(cfg, ann, false)
	for _, want := range []string{
		"# LLM external_systems suggestions",
		"# external_systems:",
		"#   payments-vendor:",
		"#     source_module: classify",
		"#       - \"github.com/vendor/sdk/**\"",
		"#     volatility: " + testAnnVolatility,
		"#     evidence_refs: " + testEvidenceREADME,
		"#     basis: semantic_judgment",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("external system suggestion output missing %q:\n%s", want, out)
		}
	}
	loaded := roundTrip(t, out)
	if len(loaded.ExternalSystems) != 0 {
		t.Fatalf("commented external_systems suggestions must stay inert, loaded external systems = %+v", loaded.ExternalSystems)
	}
}

// ---------------------------------------------------------------------------
// TestRender_ApplyMode_LiveAnnotation
// ---------------------------------------------------------------------------

// TestRender_ApplyMode_LiveAnnotation verifies that apply=true writes
// subdomain/volatility as live fields and does NOT write them as comments.
func TestRender_ApplyMode_LiveAnnotation(t *testing.T) {
	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			Subdomain:  layerCore,
			Volatility: testAnnVolatility,
			Layer:      layerCore,
		},
	}
	out := Render(cfg, ann, true)

	// Must have live subdomain and volatility.
	if !strings.Contains(out, "subdomain: "+layerCore) {
		t.Errorf("apply mode missing live subdomain:\n%s", out)
	}
	if !strings.Contains(out, "volatility: "+testAnnVolatility) {
		t.Errorf("apply mode missing live volatility:\n%s", out)
	}
	// Must NOT have commented-out annotation lines for these fields.
	if strings.Contains(out, "# subdomain: "+layerCore) {
		t.Errorf("apply mode must not have commented subdomain:\n%s", out)
	}
	if strings.Contains(out, "# volatility: "+testAnnVolatility) {
		t.Errorf("apply mode must not have commented volatility:\n%s", out)
	}
	// Layer is in set — written live.
	if !strings.Contains(out, "layer: "+layerCore) {
		t.Errorf("apply mode missing live layer:\n%s", out)
	}
	// Round-trip: live fields must survive config.Load.
	loaded := roundTrip(t, out)
	mod := loaded.Modules[testClassify]
	if mod.Subdomain != layerCore {
		t.Errorf("round-trip subdomain = %q, want %q", mod.Subdomain, layerCore)
	}
	if mod.Volatility != testAnnVolatility {
		t.Errorf("round-trip volatility = %q, want %q", mod.Volatility, testAnnVolatility)
	}
}

// ---------------------------------------------------------------------------
// TestRender_Role
// ---------------------------------------------------------------------------

// TestRender_Role verifies a suggested module role is rendered live in apply
// mode (and round-trips into config.Modules[].Role) and as a review comment in
// plan mode — the draft plumbing the Task-7 enrich/autopilot drafters populate.
func TestRender_Role(t *testing.T) {
	const role = "composition_root"
	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {Role: role},
	}

	// Plan mode: commented (inert), so the round-trip leaves Role unset.
	plan := Render(cfg, ann, false)
	if !strings.Contains(plan, "# role: "+role) {
		t.Errorf("plan mode missing commented role:\n%s", plan)
	}
	if got := roundTrip(t, plan).Modules[testClassify].Role; got != "" {
		t.Errorf("plan mode role round-tripped live = %q, want empty (comment is inert)", got)
	}

	// Apply mode: live field, not commented, and survives config.Load.
	apply := Render(cfg, ann, true)
	if strings.Contains(apply, "# role: "+role) {
		t.Errorf("apply mode must not have commented role:\n%s", apply)
	}
	if got := roundTrip(t, apply).Modules[testClassify].Role; got != module.RoleCompositionRoot {
		t.Errorf("apply mode round-trip role = %q, want %q", got, module.RoleCompositionRoot)
	}
}

// ---------------------------------------------------------------------------
// TestRender_OutOfSetLayer_NeverLive
// ---------------------------------------------------------------------------

// TestRender_OutOfSetLayer_NeverLive verifies that a layer value not in
// allowedLayers is NEVER written as a live field in either mode.
func TestRender_OutOfSetLayer_NeverLive(t *testing.T) {
	const outOfSetLayer = "domain" // NOT in allowedLayers (core, adapter)

	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			Subdomain:  layerCore,
			Volatility: testAnnVolatility,
			Layer:      outOfSetLayer,
		},
	}

	for _, apply := range []bool{false, true} {
		out := Render(cfg, ann, apply)

		// "domain" must NEVER appear as a live layer field.
		// Check for the live YAML pattern (indented key-value line) to avoid matching
		// the comment "# llm layer: domain" which contains "layer: domain" as a substring.
		if strings.Contains(out, "\n  layer: "+outOfSetLayer+"\n") {
			t.Errorf("apply=%v: out-of-set layer written live:\n%s", apply, out)
		}
		// Must appear in a comment.
		if !strings.Contains(out, "# llm layer: "+outOfSetLayer) {
			t.Errorf("apply=%v: out-of-set layer missing from comment:\n%s", apply, out)
		}
		// The not-in-layers note must be present.
		if !strings.Contains(out, "not in layers:") {
			t.Errorf("apply=%v: missing 'not in layers:' annotation:\n%s", apply, out)
		}
		// The live layer falls back to m.Layer (core) which IS in allowedLayers.
		if !strings.Contains(out, "layer: "+layerCore) {
			t.Errorf("apply=%v: fallback live layer missing:\n%s", apply, out)
		}
		// Round-trip.
		roundTrip(t, out)
	}
}

// ---------------------------------------------------------------------------
// TestRender_HeuristicLayerAbsentFromAllowedLayers
// ---------------------------------------------------------------------------

// TestRender_HeuristicLayerAbsentFromAllowedLayers verifies that when m.Layer
// is not in allowedLayers (e.g. update AddModule where config layers differ),
// the layer is NOT written live — even with ann==nil.
// When m.Layer IS in allowedLayers, the nil path writes it verbatim (byte-identical
// guarantee for init, where cfg.Layers always contains every m.Layer).
//
// This guards the update AddModule path where the target config's layers: list
// may not include a freshly-discovered layer.
func TestRender_HeuristicLayerAbsentFromAllowedLayers(t *testing.T) {
	// Build a cfg where one module's m.Layer ("engine") is NOT in cfg.Layers.
	cfg := DiscoveredConfig{
		ModulePath: testExampleMod,
		HasGo:      true,
		Layers:     []string{layerCore, layerAdapter}, // "engine" is absent
		Modules: []ModuleDef{
			{
				Name:  layerEngine,
				Paths: []string{"internal/engine/**"},
				Layer: layerEngine, // NOT in cfg.Layers above
			},
		},
	}

	// --- nil annotation, m.Layer NOT in allowedLayers ---
	// writeModuleStanza must NOT write a live layer: line.
	var b strings.Builder
	writeModuleStanza(&b, layerEngine, cfg.Modules[0], cfg.Layers, nil, false)
	out := b.String()

	if strings.Contains(out, "\n  layer: "+layerEngine+"\n") {
		t.Errorf("nil annotation with out-of-set m.Layer must not write live layer, got:\n%s", out)
	}

	// --- nil annotation, m.Layer IN allowedLayers ---
	// This is the real init guarantee: cfg.Layers ⊇ every m.Layer, so the layer
	// is always written verbatim (byte-identical to the pre-annotation code path).
	modInSet := ModuleDef{
		Name:  layerCore,
		Paths: []string{"internal/classify/**"},
		Layer: layerCore, // IS in cfg.Layers
	}
	var b3 strings.Builder
	writeModuleStanza(&b3, layerCore, modInSet, cfg.Layers, nil, false)
	out3 := b3.String()
	if !strings.Contains(out3, "layer: "+layerCore) {
		t.Errorf("nil annotation with in-set m.Layer must write layer verbatim; got:\n%s", out3)
	}

	// --- non-nil annotation, ann.Layer also out-of-set ---
	ann := &ModuleAnnotation{
		Subdomain:  testSupporting,
		Volatility: "medium",
		Layer:      layerEngine, // still not in allowedLayers
	}
	var b2 strings.Builder
	writeModuleStanza(&b2, layerEngine, cfg.Modules[0], cfg.Layers, ann, false)
	out2 := b2.String()

	// The resolved layer falls back to m.Layer (engine) which is also not in
	// allowedLayers, so NO live layer: field.
	// Use the live-line pattern to avoid matching the comment "# llm layer: engine".
	if strings.Contains(out2, "\n    layer: "+layerEngine+"\n") || strings.Contains(out2, "\n  layer: "+layerEngine+"\n") {
		t.Errorf("non-nil annotation with out-of-set m.Layer must not write live layer:\n%s", out2)
	}
	if !strings.Contains(out2, "# llm layer: "+layerEngine) {
		t.Errorf("non-nil annotation: out-of-set layer must appear in comment:\n%s", out2)
	}
}

// ---------------------------------------------------------------------------
// TestRender_InjectionGuard
// ---------------------------------------------------------------------------

// TestRender_InjectionGuard verifies that a malicious LLM SuggestedName
// containing a newline cannot escape a YAML comment into live YAML.
// The output must still round-trip through config.Load without injecting
// any unexpected fields.
func TestRender_InjectionGuard(t *testing.T) {
	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			Subdomain:     layerCore,
			Volatility:    testAnnVolatility,
			Layer:         layerCore,
			SuggestedName: "x\"\n  volatility: high",
		},
	}

	out := Render(cfg, ann, false)

	// The injected newline must not appear literally in the output.
	if strings.Contains(out, "\n  volatility: high") {
		t.Errorf("injection: newline escaped comment, live YAML injected:\n%s", out)
	}
	// The sanitized form (spaces) should appear in the rename comment.
	if !strings.Contains(out, "# llm: consider renaming") {
		t.Errorf("injection test: rename comment missing:\n%s", out)
	}
	// Round-trip: injected value must not create unexpected fields.
	loaded := roundTrip(t, out)
	// classify must NOT have volatility: high from injection.
	mod := loaded.Modules[testClassify]
	if mod.Volatility == "high" {
		t.Errorf("injection succeeded: volatility was injected as live 'high'")
	}

	// Also test injection in apply mode.
	out2 := Render(cfg, ann, true)
	if strings.Contains(out2, "\n  volatility: high") {
		t.Errorf("injection (apply): newline escaped comment:\n%s", out2)
	}
	roundTrip(t, out2)
}

// ---------------------------------------------------------------------------
// TestRender_SuggestedName_SameAsKey_NoComment
// ---------------------------------------------------------------------------

// TestRender_SuggestedName_SameAsKey_NoComment verifies that no rename comment
// is emitted when SuggestedName equals the module key.
func TestRender_SuggestedName_SameAsKey_NoComment(t *testing.T) {
	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			Subdomain:     layerCore,
			Volatility:    testAnnVolatility,
			Layer:         layerCore,
			SuggestedName: testClassify, // same as key
		},
	}
	out := Render(cfg, ann, false)
	if strings.Contains(out, "# llm: consider renaming") {
		t.Errorf("rename comment emitted when SuggestedName == key:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// TestRender_ApplyMode_OutOfSetFallbackToMLayer
// ---------------------------------------------------------------------------

// TestRender_ApplyMode_OutOfSetFallbackToMLayer verifies that in apply mode,
// when ann.Layer is out of set, the fallback m.Layer (if in allowedLayers)
// is written live.
func TestRender_ApplyMode_OutOfSetFallbackToMLayer(t *testing.T) {
	const outOfSetLayer = "domain" // NOT in allowedLayers; fallback to m.Layer = "core"

	cfg := annotationBaseCfg()
	ann := map[string]ModuleAnnotation{
		testClassify: {
			Subdomain:  layerCore,
			Volatility: testAnnVolatility,
			Layer:      outOfSetLayer,
		},
	}
	out := Render(cfg, ann, true)

	// ann.Layer is out of set → never live.
	// Check for the live YAML pattern to avoid matching "# llm layer: domain".
	if strings.Contains(out, "\n  layer: "+outOfSetLayer+"\n") {
		t.Errorf("apply mode: out-of-set ann.Layer written live:\n%s", out)
	}
	// Fallback m.Layer = "core" IS in allowedLayers → written live.
	if !strings.Contains(out, "layer: "+layerCore) {
		t.Errorf("apply mode: fallback m.Layer not written live:\n%s", out)
	}
	// Round-trip.
	loaded := roundTrip(t, out)
	mod := loaded.Modules[testClassify]
	if mod.Layer != layerCore {
		t.Errorf("round-trip layer = %q, want %q", mod.Layer, layerCore)
	}
	if mod.Subdomain != layerCore {
		t.Errorf("round-trip subdomain = %q, want %q", mod.Subdomain, layerCore)
	}
}

// ---------------------------------------------------------------------------
// TestSanitizeComment
// ---------------------------------------------------------------------------

func TestSanitizeComment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean", "hello world", "hello world"},
		{"newline", "foo\nbar", testSanitizeFooBar},
		{"cr", "foo\rbar", testSanitizeFooBar},
		{"crlf", "foo\r\nbar", "foo  bar"},
		{"tab", "foo\tbar", testSanitizeFooBar},
		{"del", "foo\x7Fbar", testSanitizeFooBar},
		{"trim", "  hello  ", "hello"},
		{"cap200", strings.Repeat("x", 250), strings.Repeat("x", 200)},
		{"injection", "x\"\n  volatility: high", "x\"   volatility: high"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeComment(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeComment(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestYamlScalar_SpecialCharLayerRoundTrip
// ---------------------------------------------------------------------------

// TestYamlScalar_SpecialCharLayerRoundTrip verifies that a layer value containing
// special YAML characters (e.g. "foo # bar") is quoted on write and round-trips
// through config.Load with the original value intact (not truncated to "foo").
func TestYamlScalar_SpecialCharLayerRoundTrip(t *testing.T) {
	const specialLayer = "foo # bar"

	// Build a cfg that has specialLayer in its Layers list so it passes the gate.
	cfg := DiscoveredConfig{
		ModulePath: testExampleMod,
		HasGo:      true,
		Layers:     []string{specialLayer, layerCore},
		Modules: []ModuleDef{
			{
				Name:  "mymod",
				Paths: []string{"internal/mymod/**"},
				Layer: specialLayer,
			},
		},
	}
	out := Render(cfg, nil, false)

	// The full value must appear somewhere in the output.
	if !strings.Contains(out, specialLayer) {
		t.Errorf("layer %q not found in output:\n%s", specialLayer, out)
	}
	// Must be quoted — bare form would be "layer: foo # bar" which YAML
	// treats as a comment, truncating the value to "foo".
	if strings.Contains(out, "layer: "+specialLayer) {
		t.Errorf("layer with '#' must be quoted, not bare:\n%s", out)
	}

	// Round-trip: config.Load must recover the full value (not truncated to "foo").
	loaded := roundTrip(t, out)
	mod := loaded.Modules["mymod"]
	if mod.Layer != specialLayer {
		t.Errorf("round-trip layer = %q, want %q", mod.Layer, specialLayer)
	}
}
