package markdown_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/finding"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/output/jsonout"
	"github.com/alexei-led/archfit/internal/output/markdown"
	"github.com/alexei-led/archfit/internal/score"
)

const (
	secGate         = "Gate findings"
	secAdvisories   = "Advisories"
	secBCAdvisories = "Balanced Coupling advisories"
	secBeyondBC     = "Supporting structural metrics (beyond Balanced Coupling)"
	secDistanceConf = "Distance confidence"
	secConnascence  = "Connascence evidence"

	// Kept tool names.
	toolJscpd = "jscpd"

	// Kept info-metric names.
	metricCycle       = "cycle"
	metricBlastRadius = "blast_radius"
	metricUnbalanced  = "unbalanced_edge"
	metricEncap       = "encapsulation"

	// Band / confidence / status literals used in multiple tests.
	bandInfo       = "info"
	bandNA         = "n/a"
	bandGood       = "good"
	confidenceHigh = "high"
	confidenceLow  = "low"
	statusAbsent   = "absent"
	gateWarn       = "warn"

	// MatchedBy keys reused across BC advisory tests.
	mbStrength   = "strength"
	mbDistance   = "distance"
	mbVolatility = "volatility"

	// Kind, role, and file literals reused in syntax surface tests.
	kindFunction   = "function"
	roleHandler    = "handler"
	fileAPIHandler = "pkg/api/handler.go"
)

func TestRenderer_Format(t *testing.T) {
	r := markdown.New()
	if got := r.Format(); got != "markdown" {
		t.Errorf("Format() = %q, want %q", got, "markdown")
	}
}

// makeGateFinding creates a gate finding for testing.
func makeGateFinding(ruleID string, sev finding.Severity, status finding.Status) finding.Finding {
	f := finding.New(ruleID, graph.Edge{
		From: "pkg/a",
		To:   "pkg/b",
		Kind: graph.EdgeKind("uses"),
	}, nil)
	f.Severity = sev
	f.Status = status
	return f
}

// makeAdvisoryFinding creates an advisory finding for testing.
func makeAdvisoryFinding(ruleID string) finding.Finding {
	f := makeGateFinding(ruleID, finding.SeverityLow, finding.StatusNew)
	f.Kind = "advisory"
	return f
}

func TestRenderer_Render_EmptyDiagnostic(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	// Required sections always present.
	for _, want := range []string{"Summary", "Verdict", "pass"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	// Optional sections absent when no findings or metrics.
	for _, absent := range []string{secGate, secAdvisories, "Metrics", "Structural facts", "Syntax surface", "## Delta"} {
		if strings.Contains(out, absent) {
			t.Errorf("output should not contain %q in empty diagnostic\nfull output:\n%s", absent, out)
		}
	}
}

func TestRenderer_Render_Delta(t *testing.T) {
	newF := finding.Finding{
		ID: "n1", RuleID: "public_api_only", Kind: "gate",
		Status: finding.StatusNew, Severity: finding.SeverityHigh,
		Edge: finding.EdgeEvidence{From: finding.Endpoint{Path: "pkg/a"}, To: finding.Endpoint{Path: "pkg/b"}},
	}
	resolvedF := finding.Finding{
		ID: "r1", RuleID: "no_cycles", Kind: "gate", Status: finding.StatusFixed,
	}
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictWarn
	d.Findings = []finding.Finding{newF, resolvedF}
	d.Delta = &diagnostic.DeltaReport{
		New:      []string{newF.ID},
		Resolved: []string{resolvedF.ID},
	}

	var buf bytes.Buffer
	if err := markdown.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"## Delta",
		"### New (1)",
		"### Resolved (1)",
		"public_api_only",
		"pkg/a → pkg/b",
		"no_cycles",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("delta output missing %q\nfull output:\n%s", want, out)
		}
	}
	// Empty buckets render no subsection.
	for _, absent := range []string{"### Severity changed", "### Pre-existing", "### Touched by this change"} {
		if strings.Contains(out, absent) {
			t.Errorf("delta output should not contain empty bucket %q\nfull output:\n%s", absent, out)
		}
	}
}

// TestRenderer_Render_FileFacts verifies the structural-facts section: top
// modules per axis, deterministic order, neutral wording (no risk labels).
func TestRenderer_Render_FileFacts(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.FileFacts = []diagnostic.FileFact{
		{Module: "tui.polling_state", InboundModuleFanIn: 23, OutboundDestinations: 2, LOC: 310},
		{Module: "tui.directory_callbacks", InboundModuleFanIn: 1, OutboundDestinations: 46, LOC: 580},
		{Module: "config", InboundModuleFanIn: 19, OutboundDestinations: 0, LOC: 120},
		{Module: "leaf", InboundModuleFanIn: 0, OutboundDestinations: 1, LOC: 30},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "## Structural facts") {
		t.Fatalf("missing structural facts section\nfull output:\n%s", out)
	}
	// Axis lines list modules ranked by value, value in parens.
	for _, want := range []string{
		"inbound module fan-in: tui.polling_state (23), config (19), tui.directory_callbacks (1)",
		"outbound destinations: tui.directory_callbacks (46), tui.polling_state (2), leaf (1)",
		"LOC: tui.directory_callbacks (580), tui.polling_state (310), config (120), leaf (30)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
	// Neutrality: the section must not label anything as a hub or risk.
	for _, forbidden := range []string{"hub", "risk", "Hub", "Risk"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("structural facts must stay neutral; found %q\nfull output:\n%s", forbidden, out)
		}
	}
	// Zero-valued modules are not listed on that axis (leaf has 0 inbound).
	if strings.Contains(out, "leaf (0)") {
		t.Errorf("zero-valued module should not be listed\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_ConnascenceSummary(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Connascence = &diagnostic.ConnascenceReport{
		EdgesWithEvidence: 2,
		AbstainedEdges:    1,
		TotalEvidence:     3,
		ByKind:            map[string]int{"name": 2, "type": 1},
		BySource:          map[string]int{"go/types": 2, "scip": 1},
		Unmeasured:        []string{"position", "execution", "timing", "value", "identity"},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"## " + secConnascence,
		"Report-only. Static facts only",
		"edges with evidence: 2",
		"abstained edges: 1",
		"by kind: name=2, type=1",
		"by source: go/types=2, scip=1",
		"unmeasured: position, execution, timing, value, identity",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRenderer_Render_DynamicImports(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.DynamicImports = []diagnostic.DynamicImport{
		{Module: "app.plugins", Count: 12, Sites: []diagnostic.DynamicImportSite{
			{File: "app/plugins/loader.py", Line: 5, Kind: "lazy_import", Language: "python"},
			{File: "app/plugins/loader.py", Line: 9, Kind: "importlib", Language: "python"},
		}},
		{Module: "web", Count: 1, Sites: []diagnostic.DynamicImportSite{
			{File: "web/boot.ts", Line: 3, Kind: "require", Language: "typescript"},
		}},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "## Dynamic / lazy imports") {
		t.Fatalf("missing dynamic imports section\nfull output:\n%s", out)
	}
	// Header reports total sites across modules.
	if !strings.Contains(out, "13 sites across 2 modules") {
		t.Errorf("missing total/module count\nfull output:\n%s", out)
	}
	// Modules ranked by count, with a sample site shown.
	for _, want := range []string{
		"**app.plugins**: 12 (e.g. app/plugins/loader.py:5[lazy_import]",
		"**web**: 1 (e.g. web/boot.ts:3[require])",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
	// Report-only framing: must not present these as gate violations.
	if strings.Contains(out, "BC-UNBALANCED") {
		t.Errorf("dynamic imports must not render as BC advisories\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_DynamicImportsAbsentWhenEmpty(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "Dynamic / lazy imports") {
		t.Errorf("dynamic imports section should be omitted when empty\nfull output:\n%s", buf.String())
	}
}

func TestRenderer_Render_CoverageGaps(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.CoverageGaps = []diagnostic.CoverageGap{
		{Tool: "go/packages", InstallCmd: "https://go.dev/dl", AffectedMetrics: []string{"coverage", "coupling_balance"}, Gate: gateWarn},
		{Tool: toolJscpd, InstallCmd: "npm install -g jscpd", AffectedMetrics: []string{metricBlastRadius}, Gate: gateWarn},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "## Coverage gaps (2)") {
		t.Fatalf("missing coverage gaps section\nfull output:\n%s", out)
	}
	for _, want := range []string{
		"**go/packages** [gate: warn] — affects coverage, coupling_balance",
		"install: `https://go.dev/dl`",
		"**" + toolJscpd + "** [gate: warn] — affects blast_radius",
		"install: `npm install -g jscpd`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRenderer_Render_CoverageGapsAbsentWhenEmpty(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "Coverage gaps") {
		t.Errorf("coverage gaps section should be omitted when empty\nfull output:\n%s", buf.String())
	}
}

func TestRenderer_Render_ConfigWarnings(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.ConfigWarnings = []string{
		`module "internal/a" omits owner`,
		"jscpd: tool crashed mid-parse",
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "## Config warnings (2)") {
		t.Fatalf("missing config warnings section\nfull output:\n%s", out)
	}
	for _, want := range []string{
		`module "internal/a" omits owner`,
		"jscpd: tool crashed mid-parse",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRenderer_Render_ConfigWarningsAbsentWhenEmpty(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "Config warnings") {
		t.Errorf("config warnings section should be omitted when empty\nfull output:\n%s", buf.String())
	}
}

func TestRenderer_Render_GateFindings(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictFail
	d.Summary.GateFindings = 2
	d.Findings = []finding.Finding{
		makeGateFinding("forbidden_dep", finding.SeverityHigh, finding.StatusNew),
		makeGateFinding("cycle", finding.SeverityCritical, finding.StatusNew),
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	for _, want := range []string{secGate, "forbidden_dep", metricCycle, secGate} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	// No advisory section — no advisory findings.
	if strings.Contains(out, secAdvisories) {
		t.Errorf("output should not contain BC Advisories section\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_AdvisoryFindings(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Summary.Warnings = 2
	d.Findings = []finding.Finding{
		makeAdvisoryFinding("bc/imbalanced_coupling"),
		makeAdvisoryFinding("bc/imbalanced_coupling"),
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	// BC coupling advisories now render in the "Balanced Coupling advisories" section
	// with the BC lint-message format (ARCHFIT[BC-UNBALANCED ...]).
	if !strings.Contains(out, secBCAdvisories) {
		t.Errorf("output missing %q section\nfull output:\n%s", secBCAdvisories, out)
	}
	if !strings.Contains(out, "ARCHFIT[BC-UNBALANCED") {
		t.Errorf("output missing BC lint-message prefix ARCHFIT[BC-UNBALANCED]\nfull output:\n%s", out)
	}

	// No gate violations section — no gate findings.
	if strings.Contains(out, secGate) {
		t.Errorf("output should not contain Gate findings section\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_AdvisoryAbsentWhenNoAdvisory(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Findings = []finding.Finding{
		makeGateFinding("forbidden_dep", finding.SeverityLow, finding.StatusNew),
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	if strings.Contains(out, secAdvisories) {
		t.Errorf("BC Advisories section must be absent when no advisory findings\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_StalenessSection(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Findings = []finding.Finding{
		makeAdvisoryFinding("map/uncovered_path"),
		makeAdvisoryFinding("map/dead_rule"),
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	// map/ findings render under the consolidated Advisories section.
	for _, want := range []string{secAdvisories, "map/uncovered_path", "map/dead_rule"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRenderer_Render_ExceptionInventory(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Summary.WaiversUsed = 1
	waived := makeGateFinding("forbidden_dep", finding.SeverityLow, finding.StatusWaived)
	d.Findings = []finding.Finding{waived}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, secGate) {
		t.Errorf("output missing Exception Inventory section\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_Top10GateTruncation(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictFail

	// Create 15 gate findings.
	for i := range 15 {
		f := makeGateFinding("rule", finding.SeverityLow, finding.StatusNew)
		// Make each unique by varying path.
		f.Edge.From.Path = "pkg/" + string(rune('a'+i))
		d.Findings = append(d.Findings, f)
	}
	d.Summary.GateFindings = 15

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	// Full violation list has all 15; gate section is truncated to 10.
	// We can't count rows easily, but verify both sections exist.
	if !strings.Contains(out, secGate) {
		t.Errorf("output missing Critical Gate Violations\nfull output:\n%s", out)
	}
	if !strings.Contains(out, secGate) {
		t.Errorf("output missing Full Violation List\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_MetricsSection(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Metrics = []diagnostic.MetricResult{
		{Name: "coupling_ratio", Display: "0.42", Band: bandGood, Confidence: confidenceHigh},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()

	for _, want := range []string{"Metrics", "coupling_ratio", "0.42"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRenderer_Render_OutputIsValidText(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Must be non-empty valid UTF-8.
	out := buf.String()
	if len(out) == 0 {
		t.Error("Render() produced empty output")
	}
	if !strings.Contains(out, "archfit") {
		t.Errorf("output missing 'archfit' header\nfull output:\n%s", out)
	}
}

func TestRenderer_Render_NewInfoMetrics(t *testing.T) {
	// Confirm kept info-metrics render their display string and band in the Metrics section.
	tests := []struct {
		name    string
		metric  diagnostic.MetricResult
		wantSub []string
	}{
		{
			name: "cycle present",
			metric: diagnostic.MetricResult{
				Name:       metricCycle,
				Display:    "3 cycle(s)",
				Band:       bandInfo,
				Confidence: confidenceHigh,
			},
			wantSub: []string{metricCycle, "3 cycle(s)", bandInfo},
		},
		{
			name: "unbalanced_edge present",
			metric: diagnostic.MetricResult{
				Name:       metricUnbalanced,
				Display:    "5 unbalanced edge(s)",
				Band:       bandInfo,
				Confidence: confidenceHigh,
			},
			wantSub: []string{metricUnbalanced, "5 unbalanced", bandInfo},
		},
		{
			name: "cycle n/a",
			metric: diagnostic.MetricResult{
				Name:       metricCycle,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantSub: []string{metricCycle, bandNA},
		},
		{
			name: "unbalanced_edge n/a",
			metric: diagnostic.MetricResult{
				Name:       metricUnbalanced,
				Display:    bandNA,
				Band:       bandNA,
				Confidence: confidenceLow,
			},
			wantSub: []string{metricUnbalanced, bandNA},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := diagnostic.New()
			d.Verdict = diagnostic.VerdictPass
			d.Metrics = []diagnostic.MetricResult{tt.metric}

			r := markdown.New()
			var buf bytes.Buffer
			if err := r.Render(d, &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			out := buf.String()
			for _, want := range tt.wantSub {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\nfull output:\n%s", want, out)
				}
			}
		})
	}
}

func TestRenderer_Render_ToolCoverageNewTools(t *testing.T) {
	// Confirm scip-symbols and jscpd (clones) coverage rows render,
	// and that an absent tool's reason + enable step renders alongside its status.
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.ToolCoverage = []diagnostic.Coverage{
		{Tool: "scip-symbols", Status: statusAbsent, Reason: "install JS/TS dependencies (e.g. `npm install`) for semantic strength"},
		{Tool: "jscpd", Status: statusAbsent},
	}

	r := markdown.New()
	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{"scip-symbols", "jscpd"} {
		if !strings.Contains(out, want) {
			t.Errorf("coverage section missing %q\nfull output:\n%s", want, out)
		}
	}
	// The reason renders inline with the tool's coverage row.
	if !strings.Contains(out, "scip-symbols: absent — install JS/TS dependencies") {
		t.Errorf("coverage row missing inline reason\nfull output:\n%s", out)
	}
	// A reasonless row keeps the plain "<tool>: <status>" form (no dangling dash).
	if !strings.Contains(out, "jscpd: absent\n") {
		t.Errorf("reasonless coverage row should render plainly\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_AgentTasks verifies the repair-task section renders
// goal, files, constraints, and validation; and is absent without tasks.
func TestRenderer_Render_AgentTasks(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictFail
	d.AgentTasks = []diagnostic.AgentTask{{
		FindingID:   "abcdef1234567890",
		RuleID:      "no_internal_access",
		Goal:        "Replace the internal-API access from pkg/a/a.go to pkg/b/internal/impl.go with b's public API.",
		Constraints: []string{"Use only the public API of module b"},
		Files:       []string{"pkg/a/a.go", "pkg/b/internal/impl.go"},
		Validation:  []string{"archfit check -c .archfit.yaml --full"},
	}}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"## Agent tasks (1)",
		"**no_internal_access** [`abcdef12`]",
		"files: pkg/a/a.go, pkg/b/internal/impl.go",
		"constraint: Use only the public API of module b",
		"validate: `archfit check -c .archfit.yaml --full`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	empty := diagnostic.New()
	empty.Verdict = diagnostic.VerdictPass
	buf.Reset()
	if err := r.Render(empty, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(buf.String(), "Agent tasks") {
		t.Error("Agent tasks section must be absent when there are none")
	}
}

// TestRenderer_Render_BCLintMessage verifies that bc/imbalanced_coupling advisories
// render as ARCHFIT[BC-UNBALANCED <SEV>] lint messages with strength/distance/
// volatility, score breakdown, why, and cheapest-move fields from MatchedBy.
func TestRenderer_Render_BCLintMessage(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Summary.Warnings = 1

	f := makeAdvisoryFinding("bc/imbalanced_coupling")
	f.Severity = finding.SeverityHigh
	f.Edge.From.Path = "internal/payments/processor.go"
	f.Edge.To.Path = "internal/users/repo.go"
	f.Why = "implementation-level coupling across a deploy boundary to a volatile core module"
	f.MatchedBy = map[string]string{
		mbStrength:      "intrusive",
		mbDistance:      "cross_deploy_unit",
		mbVolatility:    "high",
		"score":         "intrusive(+8) cross_deploy(+5) vol_high(-0) = 13->10",
		"cheapest_move": "lower strength intrusive->contract (-5)",
	}
	d.Findings = []finding.Finding{f}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"ARCHFIT[BC-UNBALANCED HIGH]",
		"internal/payments/processor.go -> internal/users/repo.go",
		"integration strength: intrusive",
		"distance: cross_deploy_unit",
		"volatility: high",
		"score: intrusive(+8) cross_deploy(+5) vol_high(-0) = 13->10",
		"why: implementation-level coupling across a deploy boundary",
		"cheapest move: lower strength intrusive->contract (-5)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("BC lint message missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestRenderer_Render_BCLintMessage_NumericScore verifies that when an advisory
// carries the numeric score fields (score_value + score_band), the renderer prints
// "score: <value>/10 (<band>) [<scorer>]" rather than just the scorer name.
func TestRenderer_Render_BCLintMessage_NumericScore(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Summary.Warnings = 1

	f := makeAdvisoryFinding("bc/imbalanced_coupling")
	f.Severity = finding.SeverityHigh
	f.Edge.From.Path = "internal/payments/processor.go"
	f.Edge.To.Path = "internal/users/repo.go"
	f.MatchedBy = map[string]string{
		mbStrength:    "intrusive",
		mbDistance:    "cross_deploy_unit",
		mbVolatility:  "high",
		"score":       "multiplicative",
		"score_value": "10",
		"score_band":  "critical",
	}
	d.Findings = []finding.Finding{f}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "score: 10/10 (critical) [multiplicative]") {
		t.Errorf("BC lint message missing numeric score line\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_BCRollup verifies that a grouped BC advisory (group_count > 1)
// renders the rollup edge count and the section header reports rollups vs edges.
func TestRenderer_Render_BCRollup(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Summary.Warnings = 1

	f := makeAdvisoryFinding("bc/imbalanced_coupling")
	f.Severity = finding.SeverityMedium
	f.Edge.From.Module = "a"
	f.Edge.To.Module = "b"
	f.MatchedBy = map[string]string{
		mbStrength:      "functional",
		mbDistance:      "cross_module_diff_owner",
		mbVolatility:    "unknown",
		"group_count":   "42",
		"group_members": "id1,id2,id3",
	}
	d.Findings = []finding.Finding{f}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"## Balanced Coupling advisories (1 rollups, 42 edges)",
		"rollup: 42 same-shape edges (e.g. id1,id2,id3)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rollup output missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestRenderer_Render_BCNoRollupLineWhenSingle verifies a non-grouped advisory
// (group_count absent or 1) renders no rollup line.
func TestRenderer_Render_BCNoRollupLineWhenSingle(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	f := makeAdvisoryFinding("bc/imbalanced_coupling")
	f.Severity = finding.SeverityMedium
	f.Edge.From.Path = "pkg/a/a.go"
	f.Edge.To.Path = "pkg/b/internal/impl.go"
	f.MatchedBy = map[string]string{
		mbStrength:    "functional",
		mbDistance:    "cross_module_diff_owner",
		mbVolatility:  "unknown",
		"group_count": "1",
	}
	d.Findings = []finding.Finding{f}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "rollup:") {
		t.Errorf("single-edge advisory must not render a rollup line\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "## Balanced Coupling advisories (1 rollups, 1 edges)") {
		t.Errorf("header missing expected rollup/edge counts\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_ConfigHash verifies that config_hash appears in the report
// when set on the diagnostic, and is absent when empty.
func TestRenderer_Render_ConfigHash(t *testing.T) {
	r := markdown.New()

	t.Run("present when set", func(t *testing.T) {
		d := diagnostic.New()
		d.Verdict = diagnostic.VerdictPass
		d.ConfigHash = "abc123def456"

		var buf bytes.Buffer
		if err := r.Render(d, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Config hash") {
			t.Errorf("output missing Config hash when set\nfull output:\n%s", out)
		}
		if !strings.Contains(out, "abc123def456") {
			t.Errorf("output missing hash value\nfull output:\n%s", out)
		}
	})

	t.Run("absent when empty", func(t *testing.T) {
		d := diagnostic.New()
		d.Verdict = diagnostic.VerdictPass
		d.ConfigHash = ""

		var buf bytes.Buffer
		if err := r.Render(d, &buf); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
		if strings.Contains(buf.String(), "Config hash") {
			t.Error("Config hash must be absent when empty")
		}
	})
}

// TestRenderer_Render_BeyondBCMetrics verifies that beyond-BC metrics appear in
// the dedicated "Supporting structural metrics" section and NOT in primary Metrics.
func TestRenderer_Render_BeyondBCMetrics(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Metrics = []diagnostic.MetricResult{
		{Name: metricEncap, Display: "0.85", Band: bandGood, Confidence: confidenceHigh},
		{Name: metricCycle, Display: "0", Band: "none", Confidence: confidenceHigh},
		{Name: metricBlastRadius, Display: "0.12", Band: "low", Confidence: confidenceHigh},
		{Name: metricUnbalanced, Display: bandNA, Band: bandNA, Confidence: confidenceLow},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	// BC-primary metric in ## Metrics section.
	if !strings.Contains(out, "## Metrics") {
		t.Errorf("output missing primary Metrics section\nfull output:\n%s", out)
	}
	if !strings.Contains(out, metricEncap) {
		t.Errorf("encapsulation missing from output\nfull output:\n%s", out)
	}

	// Beyond-BC metrics in dedicated section.
	if !strings.Contains(out, secBeyondBC) {
		t.Errorf("output missing %q section\nfull output:\n%s", secBeyondBC, out)
	}
	for _, name := range []string{metricCycle, metricBlastRadius, metricUnbalanced} {
		if !strings.Contains(out, name) {
			t.Errorf("beyond-BC metric %q missing from output\nfull output:\n%s", name, out)
		}
	}
}

// TestRenderer_Render_BeyondBCLowConfidence verifies that beyond-BC metrics with
// low confidence render with a confidence qualifier in the dedicated section and
// that no footnote block is emitted (the proxy-footnote mechanism was removed).
func TestRenderer_Render_BeyondBCLowConfidence(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Metrics = []diagnostic.MetricResult{
		// Beyond-BC metric at low confidence: qualifier appended to band label.
		{Name: metricBlastRadius, Display: "0.12", Band: "low", Confidence: confidenceLow},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	// blast_radius must appear as a headline bullet with the confidence qualifier.
	if !strings.Contains(out, "- **blast_radius**: 0.12 — low (low confidence)") {
		t.Errorf("blast_radius headline with confidence qualifier missing\nfull output:\n%s", out)
	}
	// No footnote block should be emitted.
	if strings.Contains(out, "Low-confidence proxies (footnote") {
		t.Errorf("unexpected footnote block in output\nfull output:\n%s", out)
	}

	// JSON renderer retains every metric in full.
	var jbuf bytes.Buffer
	if err := jsonout.New().Render(d, score.Scorecard{}, nil, &jbuf); err != nil {
		t.Fatalf("json Render() error = %v", err)
	}
	jout := jbuf.String()
	for _, want := range []string{`"name":"blast_radius"`, `"confidence":"low"`} {
		if !strings.Contains(jout, want) {
			t.Errorf("JSON output missing %q\nfull output:\n%s", want, jout)
		}
	}
}

// TestRenderer_Render_ProxyHeadlineWhenHighConfidence verifies a beyond-BC metric is
// NOT footnoted when its confidence is high — only low-confidence proxies demote.
func TestRenderer_Render_ProxyHeadlineWhenHighConfidence(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.Metrics = []diagnostic.MetricResult{
		{Name: metricUnbalanced, Display: "0", Band: bandInfo, Confidence: confidenceHigh},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "- **unbalanced_edge**: 0 — info") {
		t.Errorf("high-confidence metric should stay a headline bullet\nfull output:\n%s", out)
	}
	if strings.Contains(out, "Low-confidence proxies (footnote") {
		t.Errorf("no footnote expected when all proxies are high confidence\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_DistanceConfidence verifies the "Distance confidence" section
// is always present and reflects tool coverage sources.
func TestRenderer_Render_DistanceConfidence(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	// The distance confidence section is always emitted.
	if !strings.Contains(out, secDistanceConf) {
		t.Errorf("output missing %q section\nfull output:\n%s", secDistanceConf, out)
	}
	// code_structure is always-on.
	if !strings.Contains(out, "code_structure") {
		t.Errorf("output missing code_structure entry\nfull output:\n%s", out)
	}
	// owner_source and deploy_unit_source noted as not reported when absent.
	if !strings.Contains(out, "owner_source") {
		t.Errorf("output missing owner_source entry\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "deploy_unit_source") {
		t.Errorf("output missing deploy_unit_source entry\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_EmptyDiagnosticHasDistanceConfidence verifies the distance
// confidence section is present even on a minimal empty diagnostic.
func TestRenderer_Render_EmptyDiagnosticHasDistanceConfidence(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	// Distance confidence always present — it documents what signals were used.
	if !strings.Contains(out, secDistanceConf) {
		t.Errorf("distance confidence section missing in empty diagnostic\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_SyntaxSurface_Present verifies that the "Syntax surface"
// section appears when SyntaxFacts is non-empty and contains declaration counts,
// the public API list grouped by file, and detected role/route summaries.
func TestRenderer_Render_SyntaxSurface_Present(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.SyntaxFacts = []diagnostic.SyntaxFact{
		{Language: "go", File: fileAPIHandler, Kind: kindFunction, Name: "HandleRequest", Exported: true, StartLine: 10},
		{Language: "go", File: fileAPIHandler, Kind: kindFunction, Name: "internalHelper", Exported: false, StartLine: 30},
		{Language: "go", File: fileAPIHandler, Kind: "route", Name: "GET /health", Exported: false, StartLine: 50, Framework: "gin"},
		{Language: "go", File: "pkg/repo/store.go", Kind: "struct", Name: "Store", Exported: true, StartLine: 5},
		{Language: "go", File: "pkg/repo/store.go", Kind: kindFunction, Name: "FindByID", Exported: true, StartLine: 20},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	// Section header present.
	if !strings.Contains(out, "## Syntax surface") {
		t.Fatalf("missing Syntax surface section\nfull output:\n%s", out)
	}

	// Total declaration count in header.
	if !strings.Contains(out, "5 declaration(s)") {
		t.Errorf("missing total declaration count\nfull output:\n%s", out)
	}

	// Kind counts present.
	for _, want := range []string{"- function: 3", "- route: 1", "- struct: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing kind count %q\nfull output:\n%s", want, out)
		}
	}

	// Exported count present.
	if !strings.Contains(out, "- exported (public API): 3") {
		t.Errorf("missing exported count\nfull output:\n%s", out)
	}

	// Public API section present with file grouping.
	if !strings.Contains(out, "### Public API") {
		t.Fatalf("missing Public API subsection\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "`"+fileAPIHandler+"`") {
		t.Errorf("missing file grouping for %s\nfull output:\n%s", fileAPIHandler, out)
	}
	if !strings.Contains(out, "`HandleRequest` (function)") {
		t.Errorf("missing exported decl HandleRequest\nfull output:\n%s", out)
	}

	// Detected routes section.
	if !strings.Contains(out, "### Detected routes") {
		t.Fatalf("missing Detected routes subsection\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "- route: 1 registration(s)") {
		t.Errorf("output missing route registration count\nfull output:\n%s", out)
	}

	// Non-exported declarations must NOT appear in the Public API list.
	if strings.Contains(out, "`internalHelper`") {
		t.Errorf("non-exported decl internalHelper must not appear in Public API\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_SyntaxSurface_Absent verifies that the "Syntax surface"
// section is omitted entirely when SyntaxFacts is nil/empty (syntax disabled or
// sg absent). No empty section, no false signal.
func TestRenderer_Render_SyntaxSurface_Absent(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	// SyntaxFacts intentionally empty.

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "Syntax surface") {
		t.Errorf("Syntax surface section must be absent when SyntaxFacts is empty\nfull output:\n%s", out)
	}
	if strings.Contains(out, "Public API") {
		t.Errorf("Public API subsection must be absent when SyntaxFacts is empty\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_SyntaxSurface_ExportedCap verifies that when exported
// declarations exceed syntaxSurfaceExportedTopN (20), the section caps the
// list and appends a "N more" overflow line.
func TestRenderer_Render_SyntaxSurface_ExportedCap(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass

	// 25 exported functions across one file.
	for i := range 25 {
		d.SyntaxFacts = append(d.SyntaxFacts, diagnostic.SyntaxFact{
			Language:  "go",
			File:      "pkg/big/api.go",
			Kind:      "function",
			Name:      "Func" + strings.Repeat("X", i),
			Exported:  true,
			StartLine: i + 1,
		})
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "## Syntax surface") {
		t.Fatalf("missing Syntax surface section\nfull output:\n%s", out)
	}
	// Overflow line must appear.
	if !strings.Contains(out, "+5 more exported declarations") {
		t.Errorf("missing overflow line for capped exported list\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_SyntaxSurface_RouteFramework verifies that route facts
// with a Framework field render the framework name in the Public API list.
func TestRenderer_Render_SyntaxSurface_RouteFramework(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.SyntaxFacts = []diagnostic.SyntaxFact{
		{Language: "go", File: "cmd/server/routes.go", Kind: "route", Name: "GET /ping",
			Exported: true, StartLine: 1, Framework: "gin"},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	// Framework must appear in brackets next to the role annotation.
	if !strings.Contains(out, "[gin]") {
		t.Errorf("missing framework annotation [gin]\nfull output:\n%s", out)
	}
}

// TestRenderer_Render_SyntaxSurface_PerModuleCounts verifies that per-module
// declaration counts are emitted when Module fields are populated, that files
// outside declared modules are bucketed as "(unscoped)", and that the Public API
// file header includes the module name when Module is set.
func TestRenderer_Render_SyntaxSurface_PerModuleCounts(t *testing.T) {
	r := markdown.New()
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.SyntaxFacts = []diagnostic.SyntaxFact{
		// module "api" — two facts, one exported
		{Language: "go", File: "pkg/api/handler.go", Module: "api", Kind: kindFunction, Name: "Handle", Exported: true, StartLine: 10},
		{Language: "go", File: "pkg/api/handler.go", Module: "api", Kind: kindFunction, Name: "internal", Exported: false, StartLine: 20},
		// module "svc" — one fact, exported
		{Language: "go", File: "pkg/svc/service.go", Module: "svc", Kind: "struct", Name: "Service", Exported: true, StartLine: 5},
		// outside declared modules — one fact
		{Language: "go", File: "scripts/gen.go", Module: "", Kind: kindFunction, Name: "Generate", Exported: false, StartLine: 1},
	}

	var buf bytes.Buffer
	if err := r.Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	// Per-module section header.
	if !strings.Contains(out, "Per module:") {
		t.Fatalf("missing per-module section\nfull output:\n%s", out)
	}

	// module "api" has 2 declarations.
	if !strings.Contains(out, "- api: 2") {
		t.Errorf("missing module api count\nfull output:\n%s", out)
	}

	// module "svc" has 1 declaration.
	if !strings.Contains(out, "- svc: 1") {
		t.Errorf("missing module svc count\nfull output:\n%s", out)
	}

	// Outside-module file buckets as "(unscoped)".
	if !strings.Contains(out, "- (unscoped): 1") {
		t.Errorf("missing (unscoped) count\nfull output:\n%s", out)
	}

	// Public API file header includes module name in brackets.
	if !strings.Contains(out, "[api]") {
		t.Errorf("Public API file header must include [api] module annotation\nfull output:\n%s", out)
	}
	if !strings.Contains(out, "[svc]") {
		t.Errorf("Public API file header must include [svc] module annotation\nfull output:\n%s", out)
	}
}

// TestRender_DeprecatedDep_PipeEscaping verifies that pipe characters and newlines
// in manifest deprecation note cells are escaped so the Markdown table is valid.
func TestRender_DeprecatedDep_PipeEscaping(t *testing.T) {
	d := diagnostic.New()
	d.Verdict = diagnostic.VerdictPass
	d.DeprecatedDeps = []diagnostic.DeprecatedDep{
		{
			File:    "go|mod",
			Kind:    "retract",
			Subject: "v1|0",
			Note:    "broken | see changelog\nuse v1.0.1",
		},
	}

	var buf bytes.Buffer
	if err := markdown.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	out := buf.String()

	// The raw pipe in the note must be escaped as \| so the table cell is valid.
	if strings.Contains(out, "broken | see") {
		t.Errorf("unescaped pipe in table cell corrupts markdown table; output:\n%s", out)
	}
	if !strings.Contains(out, `broken \| see`) {
		t.Errorf("pipe must be escaped as \\|; output:\n%s", out)
	}
	// Newline must be collapsed to a space.
	if strings.Contains(out, "changelog\nuse") {
		t.Errorf("newline in table cell corrupts markdown table; output:\n%s", out)
	}
	if !strings.Contains(out, "changelog use v1.0.1") {
		t.Errorf("newline must be collapsed to space; output:\n%s", out)
	}
	// Pipe in file field must be escaped.
	if strings.Contains(out, "go|mod") {
		t.Errorf("unescaped pipe in file cell corrupts markdown table; output:\n%s", out)
	}
	if !strings.Contains(out, `go\|mod`) {
		t.Errorf("pipe in file must be escaped as \\|; output:\n%s", out)
	}
	// Pipe in subject field must be escaped.
	if strings.Contains(out, "v1|0") {
		t.Errorf("unescaped pipe in subject cell corrupts markdown table; output:\n%s", out)
	}
	if !strings.Contains(out, `v1\|0`) {
		t.Errorf("pipe in subject must be escaped as \\|; output:\n%s", out)
	}
}
