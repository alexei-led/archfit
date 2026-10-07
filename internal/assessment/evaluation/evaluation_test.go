// Behavior tests for the assessment evaluation seam. They pin what Evaluate
// decides — findings, waivers, statuses, advisory filtering and rollup, metric
// gating, and verdict — so the capability migration can move
// ownership without moving semantics.
package evaluation_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/evaluation"
	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/assessment/status"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	ruleModuleCycle = "no_module_cycles"
	ruleForbidden   = "forbidden_dependency"
	ruleTypePattern = "forbidden_pattern"
	selCatalog      = "internal/catalog/**"
	ruleBC          = "bc/imbalanced_coupling"
	ruleStaleLabel  = "labels/stale"
	metricName      = "cycle_count"
	pathA           = "a/a.go"
	pathB           = "b/b.go"
	kindImports     = "imports"
	keyStrength     = "strength"
	keyDistance     = "distance"
	keyVolatility   = "volatility"
	strFunctional   = "functional"
	distSameOwner   = "cross_module"
	volLow          = "low"
)

var evaluatedAt = time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

// acceptedSet is an in-memory status.AcceptedSet. Evaluate consumes the seam,
// never baseline persistence.
type acceptedSet []status.AcceptedEntry

func (a acceptedSet) HasFingerprint(fp string) bool {
	return slices.ContainsFunc(a, func(e status.AcceptedEntry) bool { return e.Fingerprint == fp })
}
func (a acceptedSet) Entries() []status.AcceptedEntry { return a }

// stubRule emits a fixed finding slice so evaluation decisions, not rule
// matching, are under test.
type stubRule struct {
	id       string
	findings []finding.Finding
}

func (r stubRule) ID() string { return r.id }
func (r stubRule) Check(relationship.Set, rules.Evidence) []finding.Finding {
	return append([]finding.Finding(nil), r.findings...)
}

// stubMetric returns a canned MetricResult so verdict gating is exercised
// without a real calculator.
type stubMetric struct{ res result.MetricResult }

func (m stubMetric) Name() string    { return m.res.Name }
func (m stubMetric) Version() string { return "v1" }
func (m stubMetric) Calculate(signal.CollectedSignals) result.MetricResult {
	return m.res
}

var _ rules.Rule = stubRule{}
var _ metrics.Metric = stubMetric{}

func gateFinding(id string, sev finding.Severity) finding.Finding {
	return finding.Finding{
		ID: id, Kind: finding.KindGate, RuleID: ruleForbidden, Status: finding.StatusNew, Severity: sev,
		Edge:      finding.EdgeEvidence{From: finding.Endpoint{Path: pathA}, To: finding.Endpoint{Path: pathB}, Kind: kindImports},
		Locations: []relationship.Location{{File: pathA, Line: 3}},
	}
}

func metricValue(delta *float64, dir result.Direction) result.MetricResult {
	return result.MetricResult{Name: metricName, Value: 1, Delta: delta, Direction: dir}
}

// findingIDs returns the IDs of every finding carrying ruleID.
func findingIDs(fs []finding.Finding, ruleID string) []string {
	out := []string{}
	for _, f := range fs {
		if ruleID == "" || f.RuleID == ruleID {
			out = append(out, f.ID)
		}
	}
	return out
}

func findByID(t *testing.T, fs []finding.Finding, id string) finding.Finding {
	t.Helper()
	for _, f := range fs {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("finding %q not found in %v", id, findingIDs(fs, ""))
	return finding.Finding{}
}

func TestEvaluateAssignsGateStatusAndCounts(t *testing.T) {
	const (
		fpNew      = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		fpAccepted = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		fpWaived   = "cccccccccccccccccccccccccccccccc"
		fpExpired  = "dddddddddddddddddddddddddddddddd"
		fpGone     = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	)
	future := evaluatedAt.Add(24 * time.Hour).Format("2006-01-02")
	past := evaluatedAt.Add(-24 * time.Hour).Format("2006-01-02")

	tests := []struct {
		name            string
		findings        []finding.Finding
		accepted        acceptedSet
		waivers         policy.WaiverSet
		wantStatus      map[string]finding.Status
		wantGateNew     int
		wantWaiversUsed int
		wantVerdict     result.Verdict
	}{
		{
			name:        "unmatched gate finding is new and fails",
			findings:    []finding.Finding{gateFinding(fpNew, finding.SeverityHigh)},
			accepted:    acceptedSet{},
			wantStatus:  map[string]finding.Status{fpNew: finding.StatusNew},
			wantGateNew: 1,
			wantVerdict: result.VerdictFail,
		},
		{
			name:        "accepted fingerprint is baselined and does not gate",
			findings:    []finding.Finding{gateFinding(fpAccepted, finding.SeverityHigh)},
			accepted:    acceptedSet{{Fingerprint: fpAccepted, Kind: finding.KindGate}},
			wantStatus:  map[string]finding.Status{fpAccepted: finding.StatusBaseline},
			wantGateNew: 0,
			wantVerdict: result.VerdictPass,
		},
		{
			name:     "unexpired waiver suppresses the gate and is counted",
			findings: []finding.Finding{gateFinding(fpWaived, finding.SeverityHigh)},
			accepted: acceptedSet{},
			waivers: policy.WaiverSet{Waivers: []policy.WaiverDef{
				{Rule: ruleForbidden, From: pathA, To: pathB, Expires: future},
			}},
			wantStatus:      map[string]finding.Status{fpWaived: finding.StatusWaived},
			wantGateNew:     0,
			wantWaiversUsed: 1,
			wantVerdict:     result.VerdictPass,
		},
		{
			name:     "expired waiver re-gates the finding",
			findings: []finding.Finding{gateFinding(fpExpired, finding.SeverityHigh)},
			accepted: acceptedSet{},
			waivers: policy.WaiverSet{Waivers: []policy.WaiverDef{
				{Rule: ruleForbidden, From: pathA, To: pathB, Expires: past},
			}},
			wantStatus:  map[string]finding.Status{fpExpired: finding.StatusExpiredWaiver},
			wantGateNew: 1,
			wantVerdict: result.VerdictFail,
		},
		{
			name:        "accepted gate finding no longer present is reported fixed and does not gate",
			findings:    nil,
			accepted:    acceptedSet{{Fingerprint: fpGone, Kind: finding.KindGate, RuleID: ruleForbidden}},
			wantStatus:  map[string]finding.Status{fpGone: finding.StatusFixed},
			wantGateNew: 0,
			wantVerdict: result.VerdictPass,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluation.Evaluate(evaluation.Input{
				Rules:    evaluation.RulesetOf(stubRule{id: ruleForbidden, findings: test.findings}),
				Accepted: test.accepted,
				Policy:   policy.AssessmentPolicy{Waivers: test.waivers},
				Now:      evaluatedAt,
			})
			for id, want := range test.wantStatus {
				if s := findByID(t, got.Findings, id).Status; s != want {
					t.Errorf("finding %s status = %q, want %q", id, s, want)
				}
			}
			if got.GateFindings != test.wantGateNew {
				t.Errorf("GateFindings = %d, want %d", got.GateFindings, test.wantGateNew)
			}
			if got.WaiversUsed != test.wantWaiversUsed {
				t.Errorf("WaiversUsed = %d, want %d", got.WaiversUsed, test.wantWaiversUsed)
			}
			if got.Verdict != test.wantVerdict {
				t.Errorf("Verdict = %q, want %q", got.Verdict, test.wantVerdict)
			}
		})
	}
}

func TestEvaluateMetricGating(t *testing.T) {
	tests := []struct {
		name   string
		metric result.MetricResult
		gate   policy.MetricConfig
		want   result.Verdict
		// wantKind is the finding a tripped ratchet emits: a gate finding for a
		// blocking ratchet, an advisory for gate warn, none otherwise. The exit
		// code reads findings, so asserting only Verdict would miss the path.
		wantKind string
	}{
		{
			name:   "unmeasured delta never gates",
			metric: metricValue(nil, result.DirectionHigherIsBetter),
			want:   result.VerdictPass,
		},
		{
			name:   "improving metric never gates",
			metric: metricValue(new(float64(4)), result.DirectionHigherIsBetter),
			want:   result.VerdictPass,
		},
		{
			name:     "worsening higher-is-better metric fails when gate is unset",
			metric:   metricValue(new(float64(-1)), result.DirectionHigherIsBetter),
			want:     result.VerdictFail,
			wantKind: finding.KindGate,
		},
		{
			name:   "worsening metric inside min_delta tolerance passes",
			metric: metricValue(new(float64(-1)), result.DirectionHigherIsBetter),
			gate:   policy.MetricConfig{MinDelta: new(float64(2))},
			want:   result.VerdictPass,
		},
		{
			name:     "worsening metric beyond min_delta tolerance fails",
			metric:   metricValue(new(float64(-3)), result.DirectionHigherIsBetter),
			gate:     policy.MetricConfig{MinDelta: new(float64(2))},
			want:     result.VerdictFail,
			wantKind: finding.KindGate,
		},
		{
			name:     "gate warn downgrades a breach to warn",
			metric:   metricValue(new(float64(-1)), result.DirectionHigherIsBetter),
			gate:     policy.MetricConfig{Gate: string(policy.GateWarn)},
			want:     result.VerdictWarn,
			wantKind: finding.KindAdvisory,
		},
		{
			name:   "gate off skips the breach entirely",
			metric: metricValue(new(float64(-1)), result.DirectionHigherIsBetter),
			gate:   policy.MetricConfig{Gate: string(policy.GateOff)},
			want:   result.VerdictPass,
		},
		{
			name:     "rising higher-is-worse metric fails",
			metric:   metricValue(new(float64(1)), result.DirectionHigherIsWorse),
			want:     result.VerdictFail,
			wantKind: finding.KindGate,
		},
		{
			name:   "rising higher-is-worse metric inside max_new passes",
			metric: metricValue(new(float64(2)), result.DirectionHigherIsWorse),
			gate:   policy.MetricConfig{MaxNew: new(2)},
			want:   result.VerdictPass,
		},
		{
			name:   "falling higher-is-worse metric passes",
			metric: metricValue(new(float64(-2)), result.DirectionHigherIsWorse),
			want:   result.VerdictPass,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluation.Evaluate(evaluation.Input{
				Metrics:  evaluation.MetricsetOf(stubMetric{res: test.metric}),
				Gates:    map[string]policy.MetricConfig{metricName: test.gate},
				Accepted: acceptedSet{},
				Now:      evaluatedAt,

				IncludeAdvisories: true,
			})
			if got.Verdict != test.want {
				t.Errorf("Verdict = %q, want %q", got.Verdict, test.want)
			}
			if len(got.Metrics) != 1 || got.Metrics[0].Name != metricName {
				t.Errorf("Metrics = %+v, want one %s result", got.Metrics, metricName)
			}
			kind := ""
			for _, f := range got.Findings {
				if f.RuleID == "metric/"+metricName {
					kind = f.Kind
				}
			}
			if kind != test.wantKind {
				t.Errorf("ratchet finding kind = %q, want %q (findings %+v)", kind, test.wantKind, got.Findings)
			}
		})
	}
}

func TestEvaluateGateFindingOutranksMetricWarn(t *testing.T) {
	got := evaluation.Evaluate(evaluation.Input{
		Rules:    evaluation.RulesetOf(stubRule{id: ruleForbidden, findings: []finding.Finding{gateFinding("f1", finding.SeverityHigh)}}),
		Metrics:  evaluation.MetricsetOf(stubMetric{res: metricValue(new(float64(-1)), result.DirectionHigherIsBetter)}),
		Gates:    map[string]policy.MetricConfig{metricName: {Gate: string(policy.GateWarn)}},
		Accepted: acceptedSet{},
		Now:      evaluatedAt,
	})
	if got.Verdict != result.VerdictFail {
		t.Fatalf("Verdict = %q, want fail: a new gate finding outranks a metric warn", got.Verdict)
	}
}

// TestEvaluateKeepsTheModulesOfModuleKeyedFindings pins that evidence
// resolution only fills an empty module on an edge finding: a module-pair
// finding (module_cycle, module_dependencies) names its modules itself, so
// resolving its empty paths would erase both modules, and the empty module of
// an unowned importer must stay empty.
func TestEvaluateKeepsTheModulesOfModuleKeyedFindings(t *testing.T) {
	const fromModule, toModule = "orders", "payments"
	moduleKeyed := finding.Finding{
		ID: "mod", Kind: finding.KindGate, RuleID: ruleModuleCycle, Status: finding.StatusNew, Severity: finding.SeverityHigh,
		Edge: finding.EdgeEvidence{From: finding.Endpoint{Module: fromModule}, To: finding.Endpoint{Module: toModule}, Kind: "module_dependency"},
	}
	modules := map[string]policy.ModuleDef{assessModA: {Paths: []string{assessPathsA}}, assessModB: {Paths: []string{assessPathsB}}}
	// An unowned importer of a visible_to module: its path is the unowned
	// package, and a module map that would claim it must not name its module.
	unownedImporter := finding.Finding{
		ID: "unowned", Kind: finding.KindGate, RuleID: ruleModuleCycle, Status: finding.StatusNew, Severity: finding.SeverityHigh,
		Edge: finding.EdgeEvidence{From: finding.Endpoint{Path: pathA}, To: finding.Endpoint{Module: toModule}, Kind: "module_dependency"},
	}
	got := evaluation.Evaluate(evaluation.Input{
		Rules: evaluation.RulesetOf(stubRule{id: ruleModuleCycle, findings: []finding.Finding{
			moduleKeyed, unownedImporter, gateFinding("edge", finding.SeverityHigh),
		}}),
		Policy:   policy.AssessmentPolicy{Topology: policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}},
		Accepted: acceptedSet{},
		Now:      evaluatedAt,
	})
	if f := findByID(t, got.Findings, "mod"); f.Edge.From.Module != fromModule || f.Edge.To.Module != toModule {
		t.Errorf("module-keyed edge = %+v, want %s -> %s kept", f.Edge, fromModule, toModule)
	}
	if f := findByID(t, got.Findings, "unowned"); f.Edge.From.Module != "" || f.Edge.From.Path != pathA {
		t.Errorf("unowned importer edge = %+v, want from path %s and no module", f.Edge, pathA)
	}
	if f := findByID(t, got.Findings, "edge"); f.Edge.From.Module != assessModA || f.Edge.To.Module != assessModB {
		t.Errorf("path-keyed edge = %+v, want modules resolved from the paths", f.Edge)
	}
}

func TestEvaluateAdvisoryVisibilityAndWarnVerdict(t *testing.T) {
	candidate := relationship.AdvisoryCandidate{
		ID: "adv1", RuleID: ruleBC, Kind: finding.KindAdvisory, Severity: relationship.SeverityMedium,
		From: pathA, To: pathB, FromModule: "a", ToModule: "b", EdgeKind: kindImports,
		Why: "unbalanced", MatchedBy: map[string]string{keyStrength: strFunctional},
	}
	tests := []struct {
		name              string
		includeAdvisories bool
		wantVisible       int
		wantWarnings      int
		wantVerdict       result.Verdict
	}{
		{name: "advisories excluded", includeAdvisories: false, wantVisible: 0, wantWarnings: 0, wantVerdict: result.VerdictPass},
		{name: "advisories included, counted as a warning, verdict stays pass", includeAdvisories: true, wantVisible: 1, wantWarnings: 1, wantVerdict: result.VerdictPass},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluation.Evaluate(evaluation.Input{
				AdvisoryCandidates: []relationship.AdvisoryCandidate{candidate},
				Accepted:           acceptedSet{},
				IncludeAdvisories:  test.includeAdvisories,
				Now:                evaluatedAt,
			})
			if len(got.Findings) != test.wantVisible {
				t.Errorf("visible findings = %d, want %d", len(got.Findings), test.wantVisible)
			}
			if got.Warnings != test.wantWarnings {
				t.Errorf("Warnings = %d, want %d", got.Warnings, test.wantWarnings)
			}
			if got.Verdict != test.wantVerdict {
				t.Errorf("Verdict = %q, want %q", got.Verdict, test.wantVerdict)
			}
		})
	}
}

// An advisory carried in as a baselined gate-pass finding is what drives the
// verdict to warn; the candidate stream is counted separately as warnings.
func TestEvaluateBaselinedAdvisoryDrivesWarnVerdict(t *testing.T) {
	adv := finding.Finding{
		ID: "carried", Kind: finding.KindAdvisory, RuleID: ruleBC, Status: finding.StatusNew,
		Severity: finding.SeverityMedium,
		Edge:     finding.EdgeEvidence{From: finding.Endpoint{Path: pathA}, To: finding.Endpoint{Path: pathB}, Kind: kindImports},
	}
	got := evaluation.Evaluate(evaluation.Input{
		Rules:    evaluation.RulesetOf(stubRule{id: ruleBC, findings: []finding.Finding{adv}}),
		Accepted: acceptedSet{},
		Now:      evaluatedAt,
	})
	if got.Verdict != result.VerdictWarn {
		t.Fatalf("Verdict = %q, want warn: an active advisory warns", got.Verdict)
	}
	if got.GateFindings != 0 {
		t.Fatalf("GateFindings = %d, want 0: an advisory never gates", got.GateFindings)
	}
}

func TestEvaluateFiltersAdvisoriesByWaiverAndBaseline(t *testing.T) {
	candidate := relationship.AdvisoryCandidate{
		ID: "adv-waived", RuleID: ruleBC, Severity: relationship.SeverityHigh,
		From: pathA, To: pathB, FromModule: "a", ToModule: "b", EdgeKind: kindImports,
	}
	tests := []struct {
		name       string
		accepted   acceptedSet
		waivers    policy.WaiverSet
		wantStatus finding.Status
	}{
		{name: "new", accepted: acceptedSet{}, wantStatus: finding.StatusNew},
		{
			name:       "baselined advisory",
			accepted:   acceptedSet{{Fingerprint: "adv-waived", Kind: finding.KindAdvisory}},
			wantStatus: finding.StatusBaseline,
		},
		{
			name:     "waived advisory",
			accepted: acceptedSet{},
			waivers: policy.WaiverSet{Waivers: []policy.WaiverDef{
				{Rule: ruleBC, From: pathA, To: pathB, Expires: evaluatedAt.Add(24 * time.Hour).Format("2006-01-02")},
			}},
			wantStatus: finding.StatusWaived,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluation.Evaluate(evaluation.Input{
				AdvisoryCandidates: []relationship.AdvisoryCandidate{candidate},
				Accepted:           test.accepted,
				Policy:             policy.AssessmentPolicy{Waivers: test.waivers},
				IncludeAdvisories:  true,
				Now:                evaluatedAt,
			})
			if s := findByID(t, got.Findings, "adv-waived").Status; s != test.wantStatus {
				t.Errorf("advisory status = %q, want %q", s, test.wantStatus)
			}
		})
	}
}

// bc/imbalanced_coupling advisories on the same module pair with the same
// classification collapse to one representative carrying the member roll-up.
func TestEvaluateRollsUpBCAdvisoriesByModulePairAndClassification(t *testing.T) {
	same := func(id, from, to string) relationship.AdvisoryCandidate {
		return relationship.AdvisoryCandidate{
			ID: id, RuleID: ruleBC, Severity: relationship.SeverityHigh,
			From: from, To: to, FromModule: "a", ToModule: "b", EdgeKind: kindImports,
			Locations: []relationship.Location{{File: from, Line: 1}},
			MatchedBy: map[string]string{keyStrength: strFunctional, keyDistance: distSameOwner, keyVolatility: volLow},
		}
	}
	differing := same("adv-other", "a/c.go", "b/c.go")
	differing.MatchedBy = map[string]string{keyStrength: "intrusive", keyDistance: distSameOwner, keyVolatility: volLow}
	other := relationship.AdvisoryCandidate{
		ID: "adv-dup", RuleID: "bc/duplicated_knowledge", Severity: relationship.SeverityLow,
		From: pathA, To: pathB, FromModule: "a", ToModule: "b", EdgeKind: "clone",
	}

	got := evaluation.Evaluate(evaluation.Input{
		AdvisoryCandidates: []relationship.AdvisoryCandidate{
			same("adv-2", "a/x.go", "b/x.go"), same("adv-1", pathA, pathB), differing, other,
		},
		Accepted:          acceptedSet{},
		IncludeAdvisories: true,
		Now:               evaluatedAt,
	})

	bcIDs := findingIDs(got.Findings, ruleBC)
	if len(bcIDs) != 2 {
		t.Fatalf("bc advisories = %v, want 2 (one rolled-up group + one distinct classification)", bcIDs)
	}
	rep := findByID(t, got.Findings, "adv-1")
	if rep.MatchedBy["group_count"] != "2" {
		t.Errorf("group_count = %q, want 2", rep.MatchedBy["group_count"])
	}
	if members := rep.MatchedBy["group_members"]; !strings.Contains(members, "adv-1") || !strings.Contains(members, "adv-2") {
		t.Errorf("group_members = %q, want both member IDs", members)
	}
	if len(rep.Locations) != 2 {
		t.Errorf("rolled-up locations = %d, want 2 merged locations", len(rep.Locations))
	}
	if ids := findingIDs(got.Findings, "bc/duplicated_knowledge"); len(ids) != 1 {
		t.Errorf("non-bc advisories = %v, want the duplicated-knowledge finding passed through untouched", ids)
	}
}

// A rollup keeps every member ID for baseline capture, past the cap on the
// readable group_members list.
func TestEvaluateRollupKeepsEveryMemberPastTheDisplayCap(t *testing.T) {
	const n = 10
	candidates := make([]relationship.AdvisoryCandidate, 0, n)
	want := make([]string, 0, n)
	for i := range n {
		id := fmt.Sprintf("adv-%02d", i)
		want = append(want, id)
		candidates = append(candidates, relationship.AdvisoryCandidate{
			ID: id, RuleID: ruleBC, Severity: relationship.SeverityHigh,
			From: fmt.Sprintf("a/%02d.go", i), To: pathB, FromModule: "a", ToModule: "b", EdgeKind: kindImports,
			MatchedBy: map[string]string{keyStrength: strFunctional, keyDistance: distSameOwner, keyVolatility: volLow},
		})
	}

	got := evaluation.Evaluate(evaluation.Input{
		AdvisoryCandidates: candidates, Accepted: acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt,
	})

	rep := findByID(t, got.Findings, "adv-00")
	if !slices.Equal(rep.Members, want) {
		t.Errorf("Members = %v, want all %d member IDs", rep.Members, n)
	}
	if shown := strings.Split(rep.MatchedBy["group_members"], ","); len(shown) != 8 {
		t.Errorf("group_members lists %d IDs, want the display cap of 8", len(shown))
	}
}

func TestEvaluateEmitsStaleLabelAdvisories(t *testing.T) {
	got := evaluation.Evaluate(evaluation.Input{
		StaleLabelKeys:    []string{"a\x00b", "malformed-no-separator"},
		Accepted:          acceptedSet{},
		IncludeAdvisories: true,
		Now:               evaluatedAt,
	})
	ids := findingIDs(got.Findings, ruleStaleLabel)
	if len(ids) != 1 {
		t.Fatalf("stale-label advisories = %v, want 1 (the malformed key is dropped)", ids)
	}
	stale := findByID(t, got.Findings, ids[0])
	if stale.Severity != finding.SeverityLow {
		t.Errorf("stale-label severity = %q, want low", stale.Severity)
	}
	if stale.Edge.From.Module != "a" || stale.Edge.To.Module != "b" {
		t.Errorf("stale-label edge = %+v, want module pair a -> b", stale.Edge)
	}
	if !strings.Contains(stale.Why, "stale") {
		t.Errorf("stale-label why = %q, want it to name the staleness", stale.Why)
	}
}

// Gate findings are re-resolved against the relationship set: the classified
// edge supplies module labels and the severity the classification implies.
func TestEvaluateResolvesGateEvidenceFromRelationships(t *testing.T) {
	tests := []struct {
		name         string
		edges        []relationship.Edge
		wantSeverity finding.Severity
		wantModules  [2]string
	}{
		{
			name: "intrusive cross-module edge is high",
			edges: []relationship.Edge{{
				FromPath: pathA, ToPath: pathB, FromModule: "a", ToModule: "b", Kind: kindImports,
				Strength: relationship.StrengthIntrusive, Distance: relationship.DistanceCrossModuleDiffOwner,
			}},
			wantSeverity: finding.SeverityHigh,
			wantModules:  [2]string{"a", "b"},
		},
		{
			name: "intrusive same-module edge stays medium",
			edges: []relationship.Edge{{
				FromPath: pathA, ToPath: pathB, FromModule: "a", ToModule: "a", Kind: kindImports,
				Strength: relationship.StrengthIntrusive, Distance: relationship.DistanceSameModule,
			}},
			wantSeverity: finding.SeverityMedium,
			wantModules:  [2]string{"a", "a"},
		},
		{
			name: "contract edge stays medium",
			edges: []relationship.Edge{{
				FromPath: pathA, ToPath: pathB, FromModule: "a", ToModule: "b", Kind: kindImports,
				Strength: relationship.StrengthContract, Distance: relationship.DistanceCrossDeployUnit,
			}},
			wantSeverity: finding.SeverityMedium,
			wantModules:  [2]string{"a", "b"},
		},
		{
			name:         "no matching edge leaves the rule severity untouched",
			edges:        nil,
			wantSeverity: finding.SeverityCritical,
			wantModules:  [2]string{"", ""},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluation.Evaluate(evaluation.Input{
				Relationships: relationship.Set{Edges: test.edges},
				Rules:         evaluation.RulesetOf(stubRule{id: ruleForbidden, findings: []finding.Finding{gateFinding("f1", finding.SeverityCritical)}}),
				Accepted:      acceptedSet{},
				Now:           evaluatedAt,
			})
			f := findByID(t, got.Findings, "f1")
			if f.Severity != test.wantSeverity {
				t.Errorf("severity = %q, want %q", f.Severity, test.wantSeverity)
			}
			if got := [2]string{f.Edge.From.Module, f.Edge.To.Module}; got != test.wantModules {
				t.Errorf("edge modules = %v, want %v", got, test.wantModules)
			}
		})
	}
}

func TestEvaluateEmptyInputIsPassWithNoFindings(t *testing.T) {
	got := evaluation.Evaluate(evaluation.Input{Accepted: acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt})
	if len(got.Findings) != 0 || len(got.Metrics) != 0 {
		t.Fatalf("Result = %+v, want no findings and no metrics", got)
	}
	if got.Verdict != result.VerdictPass {
		t.Fatalf("Verdict = %q, want pass", got.Verdict)
	}
	if got.GateFindings != 0 || got.Warnings != 0 || got.WaiversUsed != 0 {
		t.Fatalf("counts = %+v, want all zero", got)
	}
}

func TestEvaluateEmitsStalenessAdvisoriesFromPolicy(t *testing.T) {
	set := relationship.Set{Nodes: []relationship.Node{
		{ID: "file:unmapped/x.go", Path: "unmapped/x.go", Kind: "file", Language: "go", FirstParty: true},
	}}
	modules := map[string]policy.ModuleDef{"ghost": {Paths: []string{"ghost/**"}}}
	pol := policy.AssessmentPolicy{
		Topology:  policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)},
		Staleness: policy.StalenessPolicy{Enabled: true, Threshold: 24 * time.Hour},
	}

	off := evaluation.Evaluate(evaluation.Input{
		Relationships: set, Accepted: acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt,
	})
	on := evaluation.Evaluate(evaluation.Input{
		Relationships: set, Policy: pol, Accepted: acceptedSet{}, IncludeAdvisories: true, Now: evaluatedAt,
	})
	if len(on.Findings) <= len(off.Findings) {
		t.Fatalf("staleness findings on = %d, off = %d; want enabling the policy to add advisories",
			len(on.Findings), len(off.Findings))
	}
	for _, f := range on.Findings {
		if f.Kind != finding.KindAdvisory {
			t.Errorf("staleness finding %s kind = %q, want advisory", f.ID, f.Kind)
		}
	}
	if on.GateFindings != 0 {
		t.Errorf("GateFindings = %d, want 0: staleness never gates", on.GateFindings)
	}
}
