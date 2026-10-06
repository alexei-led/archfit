package arch_test

import (
	"testing"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/internal/assessment/rules"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship"
)

const (
	pkgConfig        = "internal/config"
	pkgLLM           = "internal/llm"
	modConfigAdapter = "policy-config-adapter"
	modProviders     = "provider-adapters"
	typeModuleDeps   = "module_dependencies"
	globInternal     = "internal/**"
)

// retiredDenyRule is one of the 21 forbidden_dependency rules the module
// allowlists replaced in .archfit.yaml: its selectors, and an import it banned.
type retiredDenyRule struct {
	id, from, to  string
	importer, pkg string
}

// retiredDenyRules are the deny rules removed when visible_to on
// policy-config-adapter and provider-adapters took over their job.
var retiredDenyRules = []retiredDenyRule{
	{"model_no_config", "internal/model/**", pkgConfig, "internal/model/graph/graph.go", pkgConfig},
	{"policy_no_config", "internal/policy/**", pkgConfig, "internal/policy/module.go", pkgConfig},
	{"evidence_no_config", "internal/evidence/**", pkgConfig, "internal/evidence/acquisition/coverage.go", pkgConfig},
	{"classify_no_config", "internal/relationship/**", pkgConfig, "internal/relationship/classify/classify.go", pkgConfig},
	{"assessment_no_config", "internal/assessment/**", pkgConfig, "internal/assessment/rules/rules.go", pkgConfig},
	{"extract_no_config", "internal/extract/**", pkgConfig, "internal/extract/golang/golang.go", pkgConfig},
	{"scope_no_config", "internal/scope/**", pkgConfig, "internal/scope/scope.go", pkgConfig},
	{"facts_no_config", "internal/relationship/facts/**", pkgConfig, "internal/relationship/facts/facts.go", pkgConfig},
	{"syntax_no_config", "internal/syntax/**", pkgConfig, "internal/syntax/fileclass/fileclass.go", pkgConfig},
	{"report_ports_no_config", "internal/report/ports/**", pkgConfig, "internal/report/ports/ports.go", pkgConfig},
	{"baseline_no_config", "internal/baseline/**", pkgConfig, "internal/baseline/baseline.go", pkgConfig},
	{"output_no_config", "internal/output/**", pkgConfig, "internal/output/console/report.go", pkgConfig},
	{"labels_no_config", "internal/relationship/labels/**", pkgConfig, "internal/relationship/labels/labels.go", pkgConfig},
	{"labels_store_no_config", "internal/labels/labelsio/**", pkgConfig, "internal/labels/labelsio/labelsio.go", pkgConfig},
	{"toolrun_no_config", "internal/toolrun/**", pkgConfig, "internal/toolrun/toolrun.go", pkgConfig},
	{"factcache_no_config", "internal/factcache/**", pkgConfig, "internal/factcache/store.go", pkgConfig},
	{"history_no_config", "internal/history/**", pkgConfig, "internal/history/git/git.go", pkgConfig},
	{"ownership_no_config", "internal/ownership/**", pkgConfig, "internal/ownership/ownership.go", pkgConfig},
	{"application_no_config_adapter", "internal/application/**", "internal/config/**", "internal/application/analysis.go", pkgConfig},
	{"internal_no_llm", globInternal, "internal/llm/**", "internal/assessment/rules/rules.go", pkgLLM},
	{"application_no_provider_adapters", "internal/application/**", "internal/llm/**", "internal/application/analysis.go", pkgLLM + "/anthropic"},
}

// TestSelfModelAllowlistsCatchTheRetiredDenyRules is the paired proof for the
// self-config rewrite: every import one of the 21 retired deny rules banned is
// a module_dependencies finding under the current .archfit.yaml, the imports
// the composition roots and the config lifecycle make stay allowed, and an
// importer no module owns is now denied too, where no retired rule named it.
func TestSelfModelAllowlistsCatchTheRetiredDenyRules(t *testing.T) {
	allowlists := selfAllowlistRule(t)
	for _, retired := range retiredDenyRules {
		t.Run(retired.id, func(t *testing.T) {
			if !retired.banned(retired.importer, retired.pkg) {
				t.Fatalf("fixture import %s -> %s is not one %s banned", retired.importer, retired.pkg, retired.id)
			}
			got := allowlists.Check(importSet(retired.importer, retired.pkg), rules.Evidence{})
			if len(got) != 1 || !targetsAModuleWithVisibleTo(got[0].Edge.To.Module) {
				t.Errorf("module_dependencies findings = %+v, want one on policy-config-adapter or provider-adapters", got)
			}
		})
	}

	for _, allowed := range []struct{ importer, pkg string }{
		{"cmd/archfit/main.go", pkgConfig},
		{"cmd/archfit/main.go", pkgLLM},
		{"cmd/calibrate/main.go", pkgConfig},
		{"internal/configschema/schema.go", pkgConfig},
		{"internal/initcfg/discover_go.go", pkgConfig},
		{"internal/config/projection.go", pkgConfig},
	} {
		if got := allowlists.Check(importSet(allowed.importer, allowed.pkg), rules.Evidence{}); len(got) != 0 {
			t.Errorf("%s -> %s: findings = %+v, want none: the allowlist admits it", allowed.importer, allowed.pkg, got)
		}
	}

	const unowned = "internal/newtool/main.go"
	for _, retired := range retiredDenyRules {
		if retired.banned(unowned, pkgConfig) {
			t.Fatalf("retired rule %s already banned %s; pick an importer no rule named", retired.id, unowned)
		}
	}
	got := allowlists.Check(importSet(unowned, pkgConfig), rules.Evidence{})
	if len(got) != 1 || got[0].Edge.From.Path != "internal/newtool" || got[0].Edge.To.Module != modConfigAdapter {
		t.Errorf("unowned importer findings = %+v, want internal/newtool -> %s", got, modConfigAdapter)
	}
}

func (r retiredDenyRule) banned(importer, pkg string) bool {
	fromMatch, _ := doublestar.Match(r.from, importer)
	toMatch, _ := doublestar.Match(r.to, pkg)
	return fromMatch && toMatch
}

func targetsAModuleWithVisibleTo(module string) bool {
	return module == modConfigAdapter || module == modProviders
}

// selfAllowlistRule compiles the self-config's one module_dependencies rule.
func selfAllowlistRule(t *testing.T) rules.Rule {
	t.Helper()
	ruleConfig := loadSelfConfig(t).PolicySnapshot().Gates.Rules
	var defs []policy.RuleDef
	for _, def := range ruleConfig.Rules {
		if def.Type == typeModuleDeps {
			defs = append(defs, def)
		}
	}
	if len(defs) != 1 {
		t.Fatalf("self-config module_dependencies rules = %d, want 1", len(defs))
	}
	ruleConfig.Rules = defs
	compiled, err := rules.New(ruleConfig)
	if err != nil {
		t.Fatalf("compile self-config rules: %v", err)
	}
	return compiled[0]
}

// importSet is one Go import of pkg by the file importer, the edge shape the
// Go extractor emits.
func importSet(importer, pkg string) relationship.Set {
	return relationship.Set{Edges: []relationship.Edge{{
		FromID: "file:" + importer, ToID: "package:" + pkg,
		FromPath: importer, ToPath: pkg, Kind: "imports", Language: "go",
		Locations: []relationship.Location{{File: importer, Line: 3}},
	}}}
}
