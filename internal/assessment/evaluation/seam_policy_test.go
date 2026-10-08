package evaluation

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/assessment/finding"
	"github.com/alexei-led/archfit/v3/internal/assessment/result"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/relationship"
)

const (
	modAPI       = "api"
	modOrders    = "orders"
	modBilling   = "billing"
	modStripe    = "stripe"
	layerEntry   = "entrypoint"
	layerApp     = "application"
	layerDom     = "domain"
	layerAdapt   = "adapter"
	edgeImports  = "imports"
	modMember    = "tools/member"
	pkgBilling   = "internal/billing"
	modPlaceRust = "core::place"
	modTypesRust = "core::types"
)

// hexagonalPolicy declares four modules on three ranked layers (domain inner,
// entrypoint outer) and an unlayered adapter, with orders allowed to depend on
// billing by an allowlist, under the given rules.
func hexagonalPolicy(rules ...policy.RuleDef) policy.PolicySnapshot {
	modules := map[string]policy.ModuleDef{
		modAPI:     {Paths: []string{"internal/api/**"}, Layer: layerEntry},
		modOrders:  {Paths: []string{"internal/orders/**"}, Layer: layerApp, DependsOn: []string{modBilling}},
		modBilling: {Paths: []string{"internal/billing/**"}, Layer: layerDom},
		modStripe:  {Paths: []string{"internal/stripe/**"}, Layer: layerAdapt},
	}
	topology := policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules), Layers: []string{layerDom, layerApp, layerEntry}}
	return policy.New(topology, policy.RelationshipPolicy{Topology: topology}, policy.AssessmentPolicy{Topology: topology},
		policy.GatePolicy{Rules: policy.RuleConfig{Rules: rules, ModuleMap: topology.ModuleMap, Layers: topology.Layers}}, nil, nil)
}

var (
	layerRule     = policy.RuleDef{ID: "layers", Type: ruleTypeLayerOrder, Gate: gateFail}
	allowlistRule = policy.RuleDef{ID: "allowlists", Type: ruleTypeModuleDependencies, Gate: gateFail}
)

func pairFinding(kind string, status finding.Status, from, to string) finding.Finding {
	return finding.Finding{Kind: kind, Status: status, Edge: finding.EdgeEvidence{
		From: finding.Endpoint{Module: from}, To: finding.Endpoint{Module: to},
	}}
}

func TestSeamPolicyStatus(t *testing.T) {
	t.Parallel()
	gate, advisory := finding.KindGate, finding.KindAdvisory
	for _, tc := range []struct {
		name     string
		from, to string
		findings []finding.Finding
		rules    []policy.RuleDef
		want     string
	}{
		{name: "active gate finding", from: modBilling, to: modStripe, want: seamViolation,
			findings: []finding.Finding{pairFinding(gate, finding.StatusNew, modBilling, modStripe), pairFinding(advisory, finding.StatusNew, modBilling, modStripe)}},
		{name: "expired waiver is active", from: modBilling, to: modStripe, want: seamViolation,
			findings: []finding.Finding{pairFinding(gate, finding.StatusExpiredWaiver, modBilling, modStripe)}},
		{name: "baselined new cross-module dependency is accepted, not allowed", from: modOrders, to: modBilling,
			rules: []policy.RuleDef{allowlistRule}, want: seamAccepted,
			findings: []finding.Finding{pairFinding(gate, finding.StatusBaseline, modOrders, modBilling)}},
		{name: "waived gate finding", from: modBilling, to: modStripe, want: seamAccepted,
			findings: []finding.Finding{pairFinding(gate, finding.StatusWaived, modBilling, modStripe), pairFinding(advisory, finding.StatusNew, modBilling, modStripe)}},
		{name: "advisory only", from: modAPI, to: modOrders, rules: []policy.RuleDef{layerRule}, want: seamAdvisory,
			findings: []finding.Finding{pairFinding(advisory, finding.StatusNew, modAPI, modOrders)}},
		{name: "a baselined advisory names nothing", from: modOrders, to: modBilling, rules: []policy.RuleDef{allowlistRule}, want: seamAllowed,
			findings: []finding.Finding{pairFinding(advisory, finding.StatusBaseline, modOrders, modBilling), pairFinding(advisory, finding.StatusWaived, modOrders, modBilling)}},
		{name: "a fixed finding names nothing", from: modBilling, to: modStripe, want: seamObserved,
			findings: []finding.Finding{pairFinding(gate, finding.StatusFixed, modBilling, modStripe)}},
		{name: "a finding on the reverse pair does not count", from: modStripe, to: modBilling, want: seamObserved,
			findings: []finding.Finding{pairFinding(gate, finding.StatusNew, modBilling, modStripe)}},
		{name: "allowlist under a fail-gated rule", from: modOrders, to: modBilling, rules: []policy.RuleDef{allowlistRule}, want: seamAllowed},
		{name: "allowlist under a warn-gated rule decides nothing", from: modOrders, to: modBilling,
			rules: []policy.RuleDef{{ID: "allowlists", Type: ruleTypeModuleDependencies, Gate: "warn"}}, want: seamObserved},
		{name: "layer order outer to inner", from: modAPI, to: modBilling, rules: []policy.RuleDef{layerRule}, want: seamAllowed},
		{name: "layer order inner to outer is not permission", from: modBilling, to: modAPI, rules: []policy.RuleDef{layerRule}, want: seamObserved},
		{name: "unranked layer is not permission", from: modAPI, to: modStripe, rules: []policy.RuleDef{layerRule}, want: seamObserved},
		{name: "undeclared module", from: "go.work/member", to: modBilling, rules: []policy.RuleDef{layerRule, allowlistRule}, want: seamObserved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diag := &result.Result{Findings: tc.findings, Seams: []result.Seam{{FromModule: tc.from, ToModule: tc.to}}}
			attachSeamPolicy(diag, hexagonalPolicy(tc.rules...))
			if got := diag.Seams[0].Policy; got != tc.want {
				t.Errorf("policy = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSeamPolicyPlacesFindingsByTheGraphModule pins how a finding reaches its
// seam: through the module the seam ledger keys its endpoint path under. A
// module-pair finding whose importer no declared module owns, a mixed-side
// finding, and a Rust crate::mod edge all name a declared module, or none,
// while the seam names the graph's module.
func TestSeamPolicyPlacesFindingsByTheGraphModule(t *testing.T) {
	t.Parallel()
	set := relationship.Set{Edges: []relationship.Edge{
		{FromPath: modMember + "/run.go", ToPath: pkgBilling, FromModule: modMember, ToModule: modBilling, Language: goLanguage},
		{FromPath: "main.go", ToPath: pkgBilling, FromModule: modAPI, ToModule: modBilling, Language: goLanguage},
		{FromPath: "myapp.cli", ToPath: "myapp.core", FromModule: "py-cli", ToModule: "py-core", Language: languagePython},
		{FromPath: modPlaceRust, ToPath: modTypesRust, FromModule: modPlaceRust, ToModule: modTypesRust},
	}}
	if got := seamEndpointModules(set)["."]; got != modAPI {
		t.Errorf("root package module = %q, want %q (a dotted Python path has no directory)", got, modAPI)
	}
	root := finding.Finding{Kind: finding.KindGate, Status: finding.StatusNew, Edge: finding.EdgeEvidence{
		From: finding.Endpoint{Path: "."}, To: finding.Endpoint{Module: modBilling},
	}}
	unowned := finding.Finding{Kind: finding.KindGate, Status: finding.StatusNew, Edge: finding.EdgeEvidence{
		From: finding.Endpoint{Path: modMember}, To: finding.Endpoint{Module: modBilling},
	}}
	crate := finding.Finding{Kind: finding.KindGate, Status: finding.StatusBaseline, Edge: finding.EdgeEvidence{
		From: finding.Endpoint{Path: modPlaceRust, Module: "core"}, To: finding.Endpoint{Path: modTypesRust, Module: "core"},
	}}
	diag := &result.Result{
		Findings:            []finding.Finding{unowned, crate, root},
		SeamEndpointModules: seamEndpointModules(set),
		Seams: []result.Seam{{FromModule: modMember, ToModule: modBilling}, {FromModule: modPlaceRust, ToModule: modTypesRust},
			{FromModule: modAPI, ToModule: modBilling}},
	}
	attachSeamPolicy(diag, hexagonalPolicy(layerRule))
	if got := []string{diag.Seams[0].Policy, diag.Seams[1].Policy, diag.Seams[2].Policy}; got[0] != seamViolation || got[1] != seamAccepted || got[2] != seamViolation {
		t.Errorf("policies = %v, want [violation accepted violation]", got)
	}
}

// TestSeamAllowedAgreesWithCanImport pins the seam status to the edge answer
// of `archfit policy can-import`: with no finding on the pair and no
// whole-graph rule (which can-import leaves not_decided for one edge and the
// run decides), a seam is allowed exactly when can-import answers allowed for
// an edge across it.
func TestSeamAllowedAgreesWithCanImport(t *testing.T) {
	t.Parallel()
	pkg := map[string]string{modAPI: "internal/api", modOrders: "internal/orders", modBilling: "internal/billing", modStripe: "internal/stripe"}
	for _, rules := range [][]policy.RuleDef{{layerRule}, {allowlistRule}, {layerRule, allowlistRule}} {
		p := hexagonalPolicy(rules...)
		for _, pair := range [][2]string{{modAPI, modBilling}, {modOrders, modBilling}, {modAPI, modStripe}, {modOrders, modAPI}} {
			edge := relationship.Edge{
				FromID: "package:" + pkg[pair[0]], ToID: "package:" + pkg[pair[1]], FromPath: pkg[pair[0]], ToPath: pkg[pair[1]],
				FromModule: pair[0], ToModule: pair[1], Kind: edgeImports, Language: goLanguage,
			}
			judged, err := JudgeEdge(JudgeInput{Relationships: relationship.Set{Edges: []relationship.Edge{edge}}, Policy: p})
			if err != nil {
				t.Fatal(err)
			}
			if judged.Answer == AnswerDenied {
				continue
			}
			diag := &result.Result{Seams: []result.Seam{{FromModule: pair[0], ToModule: pair[1]}}}
			attachSeamPolicy(diag, p)
			if (judged.Answer == AnswerAllowed) != (diag.Seams[0].Policy == seamAllowed) {
				t.Errorf("rules %v, %s -> %s: can-import %q, seam policy %q", rules, pair[0], pair[1], judged.Answer, diag.Seams[0].Policy)
			}
		}
	}
}

func TestSeamPolicyFollowRuleHypothesis(t *testing.T) {
	t.Parallel()
	gate, advisory := finding.KindGate, finding.KindAdvisory
	const original = "introduce_contract"
	for _, tc := range []struct {
		name     string
		findings []finding.Finding
		rules    []policy.RuleDef
		want     string
	}{
		{name: "violation", want: "follow_rule", findings: []finding.Finding{pairFinding(gate, finding.StatusNew, modBilling, modStripe)}},
		{name: "accepted", want: original, findings: []finding.Finding{pairFinding(gate, finding.StatusBaseline, modBilling, modStripe)}},
		{name: "advisory", want: original, findings: []finding.Finding{pairFinding(advisory, finding.StatusNew, modBilling, modStripe)}},
		{name: "allowed", want: original, rules: []policy.RuleDef{layerRule}},
		{name: "observed", want: original},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			from, to := modBilling, modStripe
			if tc.name == "allowed" {
				from, to = modAPI, modBilling
			}
			diag := &result.Result{Findings: tc.findings, Seams: []result.Seam{{FromModule: from, ToModule: to, Hypothesis: original}}}
			attachSeamPolicy(diag, hexagonalPolicy(tc.rules...))
			if got := diag.Seams[0].Hypothesis; got != tc.want {
				t.Errorf("hypothesis = %q, want %q", got, tc.want)
			}
		})
	}
}
