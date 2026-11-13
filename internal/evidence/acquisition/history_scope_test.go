package acquisition

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestHistoryCrateUncertaintyRespectsDeclaredScope(t *testing.T) {
	const rustModule = "rust"
	for _, tc := range []struct {
		name       string
		modules    map[string]policy.ModuleDef
		wantStatus string
	}{
		{"unowned Rust outside Go TS Python modules", map[string]policy.ModuleDef{
			"go": {Paths: []string{"go/**"}}, "ts": {Paths: []string{"web/**/*.ts"}}, "py": {Paths: []string{"app.**"}},
		}, "ok"},
		{"bare crate identity", map[string]policy.ModuleDef{rustModule: {Paths: []string{"my-crate"}}}, "partial"},
		{"nested crate identity", map[string]policy.ModuleDef{rustModule: {Paths: []string{"my_crate::core::**"}}}, "partial"},
		{"physical Rust module", map[string]policy.ModuleDef{rustModule: {Paths: []string{"fixtures/**"}}}, "ok"},
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
		})
	}
}
