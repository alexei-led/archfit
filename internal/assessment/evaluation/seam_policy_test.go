package evaluation

import (
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/finding"
	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	modAPI      = "api"
	modOrders   = "orders"
	modBilling  = "billing"
	modStripe   = "stripe"
	layerEntry  = "entrypoint"
	layerApp    = "application"
	layerDom    = "domain"
	layerAdapt  = "adapter"
	edgeImports = "imports"
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

// TestSeamAllowedAgreesWithCanImport pins the seam status to the edge answer
// of `archfit policy can-import`: with no finding on the pair, a seam is
// allowed exactly when can-import answers allowed for an edge across it.
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
