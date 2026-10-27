package agentout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
	reporttest "github.com/alexei-led/archfit/v3/internal/testutil/report"
)

const (
	validation = "archfit check -c .archfit.yaml --require-tools"
	sourceFile = "internal/a/a.go"
	pkgSource  = "pkg/a/a.go"
)

type taskSpec struct {
	id, rule, kind, origin, severity string
	from, to, edgeKind               string
	files                            []string
	line                             int
	// loc is the location file; empty means the from node, and "-" none.
	loc string
}

func gateTask(id, rule string) taskSpec {
	return taskSpec{
		id: id, rule: rule, kind: repairCodeChange, severity: report.FindingSeverityHigh,
		from: sourceFile, to: "internal/b", edgeKind: "imports",
		files: []string{sourceFile, "internal/b/b.go"}, line: 5,
	}
}

func document(verdict report.StateVerdict, specs ...taskSpec) report.Document {
	d := report.NewDocument()
	d.State.Verdict = verdict
	for _, s := range specs {
		d.State.Findings = append(d.State.Findings, report.Finding{
			ID: s.id, Kind: report.FindingKindGate, RuleID: s.rule, Status: report.FindingStatusNew, Severity: s.severity,
			Edge: report.FindingEdge{
				From: report.FindingEndpoint{Path: s.from}, To: report.FindingEndpoint{Path: s.to}, Kind: s.edgeKind,
			},
			Locations: locations(s),
		})
		d.State.AgentTasks = append(d.State.AgentTasks, report.AgentTask{
			FindingID: s.id, RuleID: s.rule, RepairKind: s.kind, Origin: s.origin,
			Goal:        "Remove the forbidden dependency from " + s.from + " on " + s.to + ".",
			Constraints: []string{"constraint of " + s.rule},
			Files:       s.files, Validation: []string{validation},
		})
	}
	return d
}

func locations(s taskSpec) []report.Location {
	switch s.loc {
	case "":
		return []report.Location{{File: s.from, Line: s.line}}
	case "-":
		return nil
	default:
		return []report.Location{{File: s.loc, Line: s.line}}
	}
}

func withUnevaluated(d report.Document, reasons ...string) report.Document {
	for i, reason := range reasons {
		d.State.Decision.UnevaluatedRequiredRules = append(d.State.Decision.UnevaluatedRequiredRules,
			report.UnevaluatedRule{RuleID: fmt.Sprintf("rule_%d", i), Reason: reason})
	}
	return d
}

func render(t *testing.T, d report.Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := New().Render(d, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.Bytes()
}

func decode(t *testing.T, raw []byte) Result {
	t.Helper()
	var r Result
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return r
}

// e1Case is one row of the change-status table: the five cases the result
// contract must agree on, plus the edges around them.
type e1Case struct {
	name          string
	doc           report.Document
	wantStatus    ChangeStatus
	wantAction    NextAction
	wantAttention []string
}

func changeCases() []e1Case {
	ownerTask := gateTask("b2", "new_dep")
	ownerTask.kind = repairNeedsOwnerDecision
	ownerTask.from, ownerTask.to = "internal/c/c.go", "internal/d"
	preExisting := gateTask("c3", "no_x")
	preExisting.origin, preExisting.from = report.OriginPreExisting, "internal/old/old.go"
	introduced := gateTask("d4", "no_x")
	introduced.origin = "introduced"
	unknown := gateTask("e5", "no_x")
	unknown.origin = "unknown"
	ratchetTripped := document(report.StateBlocked, gateTask("m1", "metric/coverage"))
	requiredTool := document(report.StateBlocked)
	requiredTool.CoverageGaps = []report.CoverageGap{{Tool: "sg", Gate: string(report.GateFail)}}
	warnTool := document(report.StateNeedsAttention)
	warnTool.CoverageGaps = []report.CoverageGap{{Tool: "sg", Gate: string(report.GateWarn)}}

	return []e1Case{
		{name: "new violation", doc: document(report.StateBlocked, introduced), wantStatus: ChangeFail, wantAction: ActionRepair},
		{name: "existing violation", doc: document(report.StateBlocked, preExisting), wantStatus: ChangePass, wantAction: ActionNone, wantAttention: []string{AttentionExistingViolation}},
		{name: "unmeasured ratchet from profile drift", doc: withUnevaluated(document(report.StateNeedsAttention), "reference not comparable (drift: measurement_profile): 3 metric ratchets cannot be evaluated"), wantStatus: ChangePass, wantAction: ActionNone, wantAttention: []string{AttentionUnmeasuredRatchet}},
		{name: "dead selector", doc: withUnevaluated(document(report.StateNeedsAttention), "selector matches nothing: from internal/x/**"), wantStatus: ChangePass, wantAction: ActionNone, wantAttention: []string{AttentionDeadSelector}},
		{name: "missing evidence", doc: withUnevaluated(document(report.StateNeedsAttention), "go/packages evidence is partial"), wantStatus: ChangeUnknown, wantAction: ActionRestoreEvidence},

		{name: "no base: every blocker is in scope", doc: document(report.StateBlocked, gateTask("a1", "no_x")), wantStatus: ChangeFail, wantAction: ActionRepair},
		{name: "code change wins over owner decision", doc: document(report.StateBlocked, ownerTask, gateTask("a1", "no_x")), wantStatus: ChangeFail, wantAction: ActionRepair},
		{name: "unknown origin is in scope", doc: document(report.StateBlocked, unknown), wantStatus: ChangeFail, wantAction: ActionRepair},
		{name: "tripped ratchet is a repair", doc: ratchetTripped, wantStatus: ChangeFail, wantAction: ActionRepair},
		{name: "only owner decisions", doc: document(report.StateBlocked, ownerTask), wantStatus: ChangeFail, wantAction: ActionAskOwner},
		{name: "in-scope repair beats existing one", doc: document(report.StateBlocked, preExisting, introduced), wantStatus: ChangeFail, wantAction: ActionRepair, wantAttention: []string{AttentionExistingViolation}},
		{name: "unanalysed selector is missing evidence", doc: withUnevaluated(document(report.StateNeedsAttention), "selector matches only source no dependency producer analyses: from web/**"), wantStatus: ChangeUnknown, wantAction: ActionRestoreEvidence},
		{name: "missing evidence beside a dead selector", doc: withUnevaluated(document(report.StateNeedsAttention), "go/packages evidence is partial", "selector matches nothing: to x"), wantStatus: ChangeUnknown, wantAction: ActionRestoreEvidence, wantAttention: []string{AttentionDeadSelector}},
		{name: "required analyzer failed", doc: requiredTool, wantStatus: ChangeUnknown, wantAction: ActionRestoreEvidence},
		{name: "warn-gated analyzer gap is not a blocker", doc: warnTool, wantStatus: ChangePass, wantAction: ActionNone},
		{name: "blocked with nothing to name", doc: document(report.StateBlocked), wantStatus: ChangePass, wantAction: ActionNone, wantAttention: []string{AttentionBlockedWithoutFinding}},
		{name: "needs attention", doc: document(report.StateNeedsAttention), wantStatus: ChangePass, wantAction: ActionNone},
		{name: "healthy", doc: document(report.StateHealthy), wantStatus: ChangePass, wantAction: ActionNone},
	}
}

func attentionKinds(r Result) []string {
	kinds := make([]string, 0, len(r.Repository.Attention))
	for _, item := range r.Repository.Attention {
		kinds = append(kinds, item.Kind)
	}
	return kinds
}

func TestChangeStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range changeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decode(t, render(t, tc.doc))
			if got.Change.Status != tc.wantStatus || got.NextAction != tc.wantAction {
				t.Errorf("change %q next_action %q, want %q %q (summary %q)", got.Change.Status, got.NextAction, tc.wantStatus, tc.wantAction, got.Summary)
			}
			if !slices.Equal(attentionKinds(got), append([]string{}, tc.wantAttention...)) {
				t.Errorf("attention = %v, want %v", attentionKinds(got), tc.wantAttention)
			}
			if got.Change.Blocks() != (tc.wantStatus == ChangeFail) {
				t.Errorf("Blocks() = %v for status %q", got.Change.Blocks(), got.Change.Status)
			}
			if got.Change.Status != ChangeFail && len(got.Change.Blockers) > 0 {
				t.Errorf("status %q lists blockers %v", got.Change.Status, got.Change.Blockers)
			}
		})
	}
}

// TestCleanMainIsPassWithOneAttention is the regression case for the result
// contract: a clean tree whose ratchet reference does not compare used to ask
// the owner with zero repairs. The change passes; the repository has one item.
func TestCleanMainIsPassWithOneAttention(t *testing.T) {
	t.Parallel()
	d := withUnevaluated(document(report.StateNeedsAttention), "reference not comparable (drift: measurement_profile): 3 metric ratchets cannot be evaluated")
	got := decode(t, render(t, d))
	if got.Change.Status != ChangePass || got.NextAction != ActionNone || len(got.Change.Blockers) != 0 {
		t.Fatalf("change %q next_action %q blockers %d, want pass none 0", got.Change.Status, got.NextAction, len(got.Change.Blockers))
	}
	if len(got.Repository.Attention) != 1 || got.Repository.Attention[0].Kind != AttentionUnmeasuredRatchet {
		t.Errorf("attention = %+v, want one unmeasured_ratchet", got.Repository.Attention)
	}
}

// caseIdentity is the fixed identity the fixtures carry.
func caseIdentity() report.ResultIdentity {
	return report.ResultIdentity{
		SchemaVersion: report.IdentitySchemaVersion, PolicyDigest: strings.Repeat("a", 64), BaselineDigest: strings.Repeat("b", 64),
		LabelsDigest: strings.Repeat("c", 64), BaseSHA: strings.Repeat("d", 40), HeadTreeDigest: strings.Repeat("e", 40),
		EngineVersion: "v0.0.0-test", MeasurementProfileHash: strings.Repeat("f", 64),
	}
}

// pinFixture compares got with the file at path. ARCHFIT_UPDATE_AGENT_CASES=1
// rewrites it; review the diff before committing.
func pinFixture(t *testing.T, path string, got []byte) {
	t.Helper()
	if os.Getenv("ARCHFIT_UPDATE_AGENT_CASES") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // path is built from a constant case name
	if err != nil {
		t.Fatalf("read %s: %v (ARCHFIT_UPDATE_AGENT_CASES=1 writes it)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s drifted; review and rerun with ARCHFIT_UPDATE_AGENT_CASES=1\n got: %s\nwant: %s", path, got, want)
	}
}

// TestCasesFixtures pins the agent JSON and the check text of the five
// change-status cases. The App ingest test reads the JSON files.
func TestCasesFixtures(t *testing.T) {
	for _, tc := range changeCases()[:5] {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.doc
			d.State.Identity = caseIdentity()
			base := filepath.Join("testdata", "cases", strings.ReplaceAll(tc.name, " ", "_"))
			pinFixture(t, base+".json", render(t, d))
			var text bytes.Buffer
			if err := NewText().Render(d, &text); err != nil {
				t.Fatal(err)
			}
			pinFixture(t, base+".txt", text.Bytes())
		})
	}
}

func TestTwoRulesOnOneEdgeAreOneRepair(t *testing.T) {
	t.Parallel()
	second := gateTask("a2", "layer_inversion")
	second.severity = report.FindingSeverityCritical
	got := decode(t, render(t, document(report.StateBlocked, gateTask("a1", "no_x"), second)))

	if len(got.Change.Blockers) != 1 {
		t.Fatalf("repairs = %d, want 1: %+v", len(got.Change.Blockers), got.Change.Blockers)
	}
	rep := got.Change.Blockers[0]
	for _, check := range []struct {
		name      string
		got, want any
	}{
		{"finding_ids", rep.FindingIDs, []string{"a1", "a2"}},
		{"rule_ids", rep.RuleIDs, []string{"layer_inversion", "no_x"}},
		{"severity", rep.Severity, report.FindingSeverityCritical},
		{"edge", *rep.Edge, Edge{From: sourceFile, To: "internal/b", Kind: "imports"}},
		{"at", rep.At, []report.Location{{File: sourceFile, Line: 5}}},
		{"edit", rep.Edit, []string{sourceFile}},
		{"constraints", rep.Constraints, []string{"constraint of no_x", "constraint of layer_inversion"}},
	} {
		if fmt.Sprint(check.got) != fmt.Sprint(check.want) {
			t.Errorf("%s = %v, want %v", check.name, check.got, check.want)
		}
	}
	if got.Validate != validation+" --format agent" {
		t.Errorf("validate = %q", got.Validate)
	}
}

func TestEditListsOnlyTheSourceSide(t *testing.T) {
	t.Parallel()
	spec := func(from, to, loc string, files ...string) taskSpec {
		s := gateTask("a1", "no_x")
		s.from, s.to, s.loc, s.files = from, to, loc, files
		return s
	}
	for _, tc := range []struct {
		name string
		spec taskSpec
		want []string
	}{
		{name: "go internal target file", spec: spec(pkgSource, "pkg/b/internal/impl.go", "", pkgSource, "pkg/b/internal/impl.go"), want: []string{pkgSource}},
		{name: "go subpackage imports its parent", spec: spec("internal/foo/bar/x.go", "internal/foo", "", "internal/foo", "internal/foo/bar/x.go"), want: []string{"internal/foo/bar/x.go"}},
		{name: "python dotted target", spec: spec("myapp.handlers", "myapp.domain", "src/myapp/handlers.py", "src/myapp/domain.py", "src/myapp/handlers.py"), want: []string{"src/myapp/handlers.py"}},
		{name: "rust crate target", spec: spec("my-app", "my-core", "crates/my-app/Cargo.toml", "crates/my-app/Cargo.toml", "crates/my-core"), want: []string{"crates/my-app/Cargo.toml"}},
		{name: "typescript edge without locations", spec: spec("web/a.ts", "web/b.ts", "-", "web/a.ts", "web/b.ts"), want: []string{"web/a.ts"}},
		{name: "rust crate::mod edge without locations abstains", spec: spec("mycrate::a", "mycrate::b", "-", "crates/mycrate/src/a.rs", "crates/mycrate/src/b.rs"), want: []string{}},
		{name: "python edge without locations abstains", spec: spec("pkg.a", "pkg.b", "-", "pkg/a.py", "pkg/b.py"), want: []string{}},
		{name: "module pair without nodes lists every task file", spec: spec("", "", "-", "internal/x/x.go", "internal/y/y.go"), want: []string{"internal/x/x.go", "internal/y/y.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decode(t, render(t, document(report.StateBlocked, tc.spec)))
			if !slices.Equal(got.Change.Blockers[0].Edit, tc.want) {
				t.Errorf("edit = %v, want %v", got.Change.Blockers[0].Edit, tc.want)
			}
		})
	}
}

// TestEmptyListsAreArrays pins that a repair with no task file still
// serializes every list as an array: the schema rejects null.
func TestEmptyListsAreArrays(t *testing.T) {
	t.Parallel()
	pair := gateTask("a1", "module_cycle")
	pair.from, pair.to, pair.loc, pair.files = "", "", "-", nil
	raw := string(render(t, document(report.StateBlocked, pair)))
	if strings.Contains(raw, "null") {
		t.Errorf("result serializes a null list: %s", raw)
	}
}

func TestOnlyDependenciesGroup(t *testing.T) {
	t.Parallel()
	apiChange := func(id string) taskSpec {
		s := gateTask(id, "api_change")
		s.from, s.to, s.edgeKind = "internal/api", "internal/api", ""
		return s
	}
	got := decode(t, render(t, document(report.StateBlocked, apiChange("a1"), apiChange("a2"))))
	if len(got.Change.Blockers) != 2 {
		t.Errorf("repairs = %d, want one per public_api finding", len(got.Change.Blockers))
	}
}

func TestGroupOrigin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		origins []string
		want    string
		inScope bool
	}{
		{name: "no base", origins: []string{"", ""}, want: "", inScope: true},
		{name: "all pre-existing", origins: []string{report.OriginPreExisting, report.OriginPreExisting}, want: report.OriginPreExisting},
		{name: "introduced wins", origins: []string{report.OriginPreExisting, report.OriginIntroduced}, want: report.OriginIntroduced, inScope: true},
		{name: "mixed is unknown", origins: []string{report.OriginPreExisting, report.OriginUnknown}, want: report.OriginUnknown, inScope: true},
		{name: "missing origin is never pre-existing", origins: []string{"", report.OriginPreExisting}, want: report.OriginUnknown, inScope: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			specs := make([]taskSpec, 0, len(tc.origins))
			for i, origin := range tc.origins {
				s := gateTask(fmt.Sprintf("a%d", i), fmt.Sprintf("rule_%d", i))
				s.origin = origin
				specs = append(specs, s)
			}
			rep := buildRepairs(document(report.StateBlocked, specs...).State)[0]
			if rep.Origin != tc.want || rep.InScope != tc.inScope {
				t.Errorf("origin %q in_scope %v, want %q %v", rep.Origin, rep.InScope, tc.want, tc.inScope)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	advisoryOnly := document(report.StateBlocked)
	advisoryOnly.AdvisoryTasks = []report.AdvisoryTask{{FindingID: "x", Validation: []string{validation}}}
	for _, tc := range []struct {
		name string
		doc  report.Document
		want string
	}{
		{name: "from a repair task", doc: document(report.StateBlocked, gateTask("a1", "no_x")), want: validation + " --format agent"},
		{name: "from an advisory task", doc: advisoryOnly, want: validation + " --format agent"},
		{name: "absent with no task", doc: document(report.StateNeedsAttention), want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := decode(t, render(t, tc.doc)).Validate; got != tc.want {
				t.Errorf("validate = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRepairOrder(t *testing.T) {
	t.Parallel()
	out := gateTask("a0", "no_x")
	out.origin, out.from = report.OriginPreExisting, "internal/out.go"
	owner := gateTask("a1", "new_dep")
	owner.kind, owner.from = repairNeedsOwnerDecision, "internal/owner.go"
	low := gateTask("a2", "no_x")
	low.severity, low.from = report.FindingSeverityLow, "internal/low.go"
	critical := gateTask("a9", "no_x")
	critical.severity, critical.from = report.FindingSeverityCritical, "internal/critical.go"
	high := gateTask("a3", "no_x")
	high.from = "internal/high.go"

	got := decode(t, render(t, document(report.StateBlocked, out, owner, low, critical, high)))
	order := make([]string, 0, len(got.Change.Blockers))
	for _, r := range got.Change.Blockers {
		order = append(order, r.FindingIDs[0])
	}
	if want := []string{"a9", "a3", "a2", "a1"}; !slices.Equal(order, want) {
		t.Errorf("blocker order = %v, want %v", order, want)
	}
	if kinds := attentionKinds(got); !slices.Equal(kinds, []string{AttentionExistingViolation}) {
		t.Errorf("a pre_existing repair must be repository attention, got %v", kinds)
	}
}

// realisticRepair is the size of a real forbidden_dependency repair: a 32-hex
// ID, a Go import edge, the engine's goal template, the rule's constraint
// text, and a declared rationale.
func realisticRepair(i int) taskSpec {
	s := gateTask(fmt.Sprintf("%032x", i), fmt.Sprintf("core_no_adapter_%02d", i))
	s.from = fmt.Sprintf("internal/relationship/scoring/scorer_book_%02d.go", i)
	s.to = "internal/toolrun"
	s.files = []string{s.from}
	return s
}

func realisticDocument(n int) report.Document {
	specs := make([]taskSpec, 0, n)
	for i := range n {
		specs = append(specs, realisticRepair(i))
	}
	d := document(report.StateBlocked, specs...)
	for i := range d.State.AgentTasks {
		task := &d.State.AgentTasks[i]
		task.Goal = "Remove the forbidden dependency from " + d.State.Findings[i].Edge.From.Path +
			" on internal/toolrun; move shared behavior to a location permitted by the existing dependency rules."
		task.Constraints = []string{
			"Remove the dependency or move the code",
			"rationale: the core ring decides over gathered facts and never runs a subprocess",
		}
	}
	return d
}

func TestBudget(t *testing.T) {
	t.Parallel()
	t.Run("ten repairs fit", func(t *testing.T) {
		t.Parallel()
		raw := render(t, realisticDocument(10))
		if len(raw) > budgetBytes {
			t.Fatalf("ten repairs take %d bytes, budget %d", len(raw), budgetBytes)
		}
		got := decode(t, raw)
		if len(got.Change.Blockers) != 10 || got.Truncated || got.Omitted.Blockers != 0 {
			t.Errorf("ten repairs were cut: repairs=%d truncated=%v omitted=%d", len(got.Change.Blockers), got.Truncated, got.Omitted.Blockers)
		}
	})
	t.Run("two runs are byte-identical", func(t *testing.T) {
		t.Parallel()
		d := realisticDocument(40)
		if first, second := render(t, d), render(t, d); !bytes.Equal(first, second) {
			t.Error("two renders of one document differ")
		}
	})
	t.Run("over budget moves tail repairs into omitted", func(t *testing.T) {
		t.Parallel()
		raw := render(t, realisticDocument(60))
		got := decode(t, raw)
		if len(raw) > budgetBytes {
			t.Errorf("result takes %d bytes, budget %d", len(raw), budgetBytes)
		}
		if !got.Truncated || got.Omitted.Blockers == 0 {
			t.Errorf("truncated=%v omitted.repairs=%d, want a cut", got.Truncated, got.Omitted.Blockers)
		}
		if len(got.Change.Blockers)+got.Omitted.Blockers != 60 {
			t.Errorf("repairs %d + omitted %d != 60", len(got.Change.Blockers), got.Omitted.Blockers)
		}
		if got.NextAction != ActionRepair || !strings.Contains(got.Summary, "60 blockers") {
			t.Errorf("the budget changed the decision: %q, %q", got.NextAction, got.Summary)
		}
	})
	t.Run("header and one repair always stay", func(t *testing.T) {
		t.Parallel()
		d := realisticDocument(1)
		task := &d.State.AgentTasks[0]
		for i := range 200 {
			file := fmt.Sprintf("internal/relationship/file_%03d.go", i)
			task.Files = append(task.Files, file)
			d.State.Findings[0].Locations = append(d.State.Findings[0].Locations, report.Location{File: file, Line: 1})
			task.Constraints = append(task.Constraints, strings.Repeat("long constraint ", 30)+strconv.Itoa(i))
		}
		d = withUnevaluated(d, slices.Repeat([]string{strings.Repeat("reason ", 100)}, 50)...)
		raw := render(t, d)
		if len(raw) > budgetBytes {
			t.Errorf("result takes %d bytes, budget %d", len(raw), budgetBytes)
		}
		got := decode(t, raw)
		if len(got.Change.Blockers) != 1 || !got.Truncated {
			t.Fatalf("repairs=%d truncated=%v, want the first repair kept and the result truncated", len(got.Change.Blockers), got.Truncated)
		}
		rep := got.Change.Blockers[0]
		for _, list := range []struct {
			name          string
			kept, omitted int
			total         int
		}{
			{"at", len(rep.At), rep.AtOmitted, 201},
			{"edit", len(rep.Edit), rep.EditOmitted, 201},
			{"constraints", len(rep.Constraints), rep.ConstraintsOmitted, 202},
		} {
			if list.kept > tightListLen || list.kept+list.omitted != list.total {
				t.Errorf("%s: kept %d + omitted %d, want at most %d kept of %d", list.name, list.kept, list.omitted, tightListLen, list.total)
			}
		}
		if got.Omitted.UnevaluatedRules+len(got.Change.UnevaluatedRules) != 50 {
			t.Errorf("unevaluated rules lost: kept %d, omitted %d", len(got.Change.UnevaluatedRules), got.Omitted.UnevaluatedRules)
		}
	})
}

func TestTextIsBounded(t *testing.T) {
	t.Parallel()
	d := document(report.StateBlocked, gateTask("a1", "no_x"))
	d.State.AgentTasks[0].Goal = strings.Repeat("g", 3600)
	raw := render(t, d)
	got := decode(t, raw)
	if n := len([]rune(got.Change.Blockers[0].Goal)); n != textRunes {
		t.Errorf("goal has %d runes, want %d", n, textRunes)
	}
	if !got.Truncated {
		t.Error("a cut goal must set truncated")
	}
	violations, err := reporttest.AppTextViolations(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("App text violations: %v", violations)
	}
}

// TestCompleteness pins that every active gate task reaches a repair: the
// digest may group and order tasks, never lose one.
func TestCompleteness(t *testing.T) {
	t.Parallel()
	d := realisticDocument(10)
	d.State.AgentTasks = append(d.State.AgentTasks, report.AgentTask{FindingID: "orphan", RuleID: "seam", RepairKind: repairCodeChange})
	got := decode(t, render(t, d))
	var ids []string
	for _, r := range got.Change.Blockers {
		ids = append(ids, r.FindingIDs...)
	}
	slices.Sort(ids)
	want := make([]string, 0, len(d.State.AgentTasks))
	for _, task := range d.State.AgentTasks {
		want = append(want, task.FindingID)
	}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Errorf("repair finding IDs = %v, want every task %v", ids, want)
	}
}

func TestAdvisoriesAreCountedNotListed(t *testing.T) {
	t.Parallel()
	d := document(report.StateNeedsAttention)
	for i, status := range []string{report.FindingStatusNew, report.FindingStatusBaseline, report.FindingStatusExpiredWaiver} {
		d.State.Findings = append(d.State.Findings, report.Finding{ID: strconv.Itoa(i), Kind: report.FindingKindAdvisory, Status: status})
	}
	got := decode(t, render(t, d))
	if got.Omitted.Advisories != 2 {
		t.Errorf("omitted.advisories = %d, want 2 active", got.Omitted.Advisories)
	}
}
