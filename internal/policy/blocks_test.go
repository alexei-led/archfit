package policy_test

import (
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
)

const forbiddenDependency = "forbidden_dependency"

func TestRuleDefBlocks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ruleType, gate string
		want           bool
	}{
		{ruleType: forbiddenDependency, gate: "", want: true},
		{ruleType: forbiddenDependency, gate: "fail", want: true},
		{ruleType: forbiddenDependency, gate: "warn", want: false},
		{ruleType: forbiddenDependency, gate: "off", want: false},
		{ruleType: "module_cycle", gate: "", want: true},
		{ruleType: "public_api_change", gate: "", want: false},
		{ruleType: "public_api_type_leak", gate: "", want: false},
		{ruleType: "public_api_type_leak", gate: "fail", want: true},
	} {
		if got := (policy.RuleDef{Type: tc.ruleType, Gate: tc.gate}).Blocks(); got != tc.want {
			t.Errorf("%s gate %q: Blocks = %v, want %v", tc.ruleType, tc.gate, got, tc.want)
		}
	}
}
