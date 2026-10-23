package evaluation_test

import (
	"testing"

	modevidence "github.com/alexei-led/archfit/v3/internal/model/evidence"
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
		absent  bool
	}{
		{"omitted only", policy.RuleDef{Type: ruleForbidden, From: omittedPython, To: externalLibrary}, true, false},
		{"selected package", policy.RuleDef{Type: ruleForbidden, From: selectedPython, To: externalLibrary}, false, false},
		{"mixed source", policy.RuleDef{Type: ruleForbidden, From: "**", To: externalLibrary}, true, false},
		{"all source", policy.RuleDef{Type: ruleForbidden, To: externalLibrary}, true, false},
		{"mixed target", policy.RuleDef{Type: ruleForbidden, From: selectedPython, To: "**"}, true, false},
		{"mixed module", policy.RuleDef{Type: ruleForbidden, FromModule: "all", To: externalLibrary}, true, false},
		{"whole graph cycle", policy.RuleDef{Type: metricCycle}, true, false},
		{"guard over omitted source", policy.RuleDef{Type: ruleForbidden, From: omittedPython, To: externalLibrary, Guard: true}, true, false},
		{"pattern reads source", policy.RuleDef{Type: ruleTypePattern, From: omittedPython}, false, false},
		{"absent Python cycle", policy.RuleDef{Type: metricCycle}, false, true},
		{"absent Python broad rule", policy.RuleDef{Type: ruleForbidden, To: externalLibrary}, false, true},
		{"absent Python module rule", policy.RuleDef{Type: typeLayerDir}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diag, in := vacuityFixture()
			in.Facts.FileLOC["scripts/run.py"] = 10
			in.Facts.SourceSelectors["scripts/run.py"] = "scripts.run"
			in.Facts.UnanalysedFiles = map[string]struct{}{"scripts/run.py": {}}
			if tc.absent {
				in.Facts.UnanalysedFiles[pyCoreFile] = struct{}{}
				for i := range diag.ToolCoverage {
					if diag.ToolCoverage[i].Tool == assessGrimp {
						diag.ToolCoverage[i].Status = modevidence.StatusAbsent
					}
				}
			}
			withModules(&in, map[string]policy.ModuleDef{"all": {Paths: []string{selectedPython, omittedPython, "internal/**"}}})
			withRule(&in, tc.rule)
			got := unevaluatedReasons(diag, in)
			if (len(got) != 0) != tc.unknown {
				t.Fatalf("unevaluated=%v want unknown=%v", got, tc.unknown)
			}
		})
	}
}
