package arch_test

import (
	"testing"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const (
	pkgConfig    = "internal/config"
	pkgLLM       = "internal/llm"
	globInternal = "internal/**"
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

// The schema 3 graph must still reject the imports banned by the retired rules.
func TestSelfModelAllowlistsCatchTheRetiredDenyRules(t *testing.T) {
	p := selfPolicy3(t)
	for _, retired := range retiredDenyRules {
		t.Run(retired.id, func(t *testing.T) {
			if !retired.banned(retired.importer, retired.pkg) {
				t.Fatal("invalid retired-rule fixture")
			}
			errs := change.Judge(p, change.Use{From: change.End{Path: retired.importer, IsFile: true}, To: change.End{Path: retired.pkg}})
			for _, err := range errs {
				if err.Key.Rule == resolved.RuleGraph {
					return
				}
			}
			t.Errorf("%s -> %s: graph rule did not reject: %+v", retired.importer, retired.pkg, errs)
		})
	}
	for _, allowed := range []struct{ importer, pkg string }{
		{"cmd/archfit/main.go", pkgConfig}, {"cmd/archfit/main.go", pkgLLM},
		{"cmd/calibrate/main.go", pkgConfig}, {"internal/configschema/schema.go", pkgConfig},
		{"internal/initcfg/discover_go.go", pkgConfig}, {"internal/config/projection.go", pkgConfig},
	} {
		errs := change.Judge(p, change.Use{From: change.End{Path: allowed.importer, IsFile: true}, To: change.End{Path: allowed.pkg}})
		for _, err := range errs {
			if err.Key.Rule == resolved.RuleGraph {
				t.Errorf("allowed %s -> %s: %+v", allowed.importer, allowed.pkg, err)
			}
		}
	}
	if _, ok := p.ModuleOf("internal/newtool/main.go"); ok {
		t.Error("unowned fixture unexpectedly has an owner")
	}
}

func (r retiredDenyRule) banned(importer, pkg string) bool {
	fromMatch, _ := doublestar.Match(r.from, importer)
	toMatch, _ := doublestar.Match(r.to, pkg)
	return fromMatch && toMatch
}
