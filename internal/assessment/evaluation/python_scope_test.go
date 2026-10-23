package evaluation_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

func TestPythonOmittedSourceCannotHideInMixedRuleScope(t *testing.T) {
	const externalLibrary = "external_lib"
	const selectedPython = "app.**"
	const omittedPython = "scripts.**"
	for _, tc := range []struct {
		name    string
		rule    policy.RuleDef
		unknown bool
	}{
		{"omitted only", policy.RuleDef{Type: ruleForbidden, From: omittedPython, To: externalLibrary}, true},
		{"selected package", policy.RuleDef{Type: ruleForbidden, From: selectedPython, To: externalLibrary}, false},
		{"mixed source", policy.RuleDef{Type: ruleForbidden, From: "**", To: externalLibrary}, true},
		{"all source", policy.RuleDef{Type: ruleForbidden, To: externalLibrary}, true},
		{"mixed target", policy.RuleDef{Type: ruleForbidden, From: selectedPython, To: "**"}, true},
		{"mixed module", policy.RuleDef{Type: ruleForbidden, FromModule: "all", To: externalLibrary}, true},
		{"whole graph cycle", policy.RuleDef{Type: metricCycle}, true},
		{"guard over omitted source", policy.RuleDef{Type: ruleForbidden, From: omittedPython, To: externalLibrary, Guard: true}, true},
		{"pattern reads source", policy.RuleDef{Type: ruleTypePattern, From: omittedPython}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := vacuityFixture()
			in.Facts.FileLOC["scripts/run.py"] = 10
			in.Facts.SourceSelectors["scripts/run.py"] = "scripts.run"
			in.Facts.UnanalysedFiles = map[string]struct{}{"scripts/run.py": {}}
			withModules(&in, map[string]policy.ModuleDef{"all": {Paths: []string{selectedPython, omittedPython}}})
			withRule(&in, tc.rule)
			got := unevaluatedReasons(diag, in)
			if (len(got) != 0) != tc.unknown {
				t.Fatalf("unevaluated=%v want unknown=%v", got, tc.unknown)
			}
		})
	}
}
