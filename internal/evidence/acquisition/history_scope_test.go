package acquisition

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestHistoryCrateUncertaintyRespectsDeclaredScope(t *testing.T) {
	const rustModule = "rust"
	for _, tc := range []struct {
		name           string
		modules        map[string]policy.ModuleDef
		wantStatus     string
		wantComparable bool
	}{
		{"unowned Rust outside Go TS Python modules", map[string]policy.ModuleDef{
			"go": {Paths: []string{"go/**"}}, "ts": {Paths: []string{"web/**/*.ts"}}, "py": {Paths: []string{"app.**"}},
		}, "ok", true},
		{"bare crate identity", map[string]policy.ModuleDef{rustModule: {Paths: []string{"my-crate"}}}, "partial", false},
		{"nested crate identity", map[string]policy.ModuleDef{rustModule: {Paths: []string{"my_crate::core::**"}}}, "partial", false},
		{"physical Rust module", map[string]policy.ModuleDef{rustModule: {Paths: []string{"fixtures/**"}}}, "ok", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
				if cmd.Args[0] == "log" {
					return toolrun.Output{Stdout: []byte("\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\ngo/a.go\x00web/a.ts\x00app/logic.py\x00fixtures/unowned.rs\x00")}, nil
				}
				return toolrun.Output{Stdout: []byte("git version 2.49.0")}, nil
			}}
			p := policy.PolicySnapshot{Topology: policy.TopologyView{Modules: tc.modules, ModuleMap: policy.BuildModuleMap(tc.modules)}}
			history := buildVolatilityCorroboration(context.Background(), "/repo", "", p, runner)
			if history == nil || history.Status != tc.wantStatus {
				t.Fatalf("history = %+v, want %s", history, tc.wantStatus)
			}
			service := Service{Runner: runner}
			profile := service.measurementProfile(context.Background(), scope.Scope{GitRoot: "/repo"}, nil, history)
			reasons := decision.CompareMeasurementProfiles(profile, profile)
			if (len(reasons) == 0) != tc.wantComparable {
				t.Fatalf("comparable = %t, want %t, reasons=%v", len(reasons) == 0, tc.wantComparable, reasons)
			}
		})
	}
}
