package acquisition

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/decision"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestSuccessfulUnmatchedHistoryRemainsComparable(t *testing.T) {
	runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		if cmd.Args[0] == "log" {
			return toolrun.Output{Stdout: []byte("\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\nREADME.md\x00")}, nil
		}
		return toolrun.Output{Stdout: []byte("git version 2.49.0")}, nil
	}}
	modules := map[string]policy.ModuleDef{"core": {Paths: []string{"src/core/**"}}}
	p := policy.PolicySnapshot{Topology: policy.TopologyView{Modules: modules, ModuleMap: policy.BuildModuleMap(modules)}}
	history := buildVolatilityCorroboration(context.Background(), "/repo", "", p, runner)
	if history == nil || history.Status != "ok" || history.CommitsScanned != 1 || history.ModulesTouched != 0 {
		t.Fatalf("successful history became absent: %+v", history)
	}
	s := Service{Runner: runner}
	profile := s.measurementProfile(context.Background(), scope.Scope{GitRoot: "/repo"}, nil, history)
	if reasons := decision.CompareMeasurementProfiles(profile, profile); len(reasons) != 0 {
		t.Fatalf("successful no-match history is not comparable: %v", reasons)
	}
	if len(profile.Producers) != 1 || profile.Producers[0].Status != "ok" {
		t.Fatalf("history profile = %+v", profile.Producers)
	}
}
