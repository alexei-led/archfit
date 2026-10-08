package acquisition_test

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const historyCoreModule = "core"

func TestHistoryAttributesPythonDeletedFiles(t *testing.T) {
	cfg := config.Config{Modules: map[string]policy.ModuleDef{historyCoreModule: {Paths: []string{"app.core**"}}}}
	calls := 0
	runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
		calls++
		return toolrun.Output{Stdout: []byte("\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\nservice/src/app/core/deleted.py\x00service/src/app/core/__init__.py\x00")}, nil
	}}
	got := acquisition.BuildVolatilityCorroboration(context.Background(), "/repo", "service", cfg.PolicySnapshot(), runner)
	if got == nil || got.ModulesTouched != 1 || len(got.TopTouched) != 1 || got.TopTouched[0].TouchCommits != 1 || calls != 1 {
		t.Fatalf("history = %+v, calls = %d", got, calls)
	}
}

func TestHistoryAttributesProducerCrates(t *testing.T) {
	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		historyCoreModule: {Paths: []string{"actual-core"}},
		"nested":          {Paths: []string{"actual-nested"}},
	}}
	calls := 0
	runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
		calls++
		return toolrun.Output{Stdout: []byte("\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\nworkspace/crates/directory/src/deleted.rs\x00workspace/crates/directory/src/lib.rs\x00workspace/crates/directory/nested/src/lib.rs\x00elsewhere/src/lib.rs\x00")}, nil
	}}
	got := acquisition.BuildVolatilityCorroboration(context.Background(), "/repo", "workspace", cfg.PolicySnapshot(), runner,
		graph.CrateRoot{Dir: "crates/directory", Name: "actual-core"}, graph.CrateRoot{Dir: "crates/directory/nested", Name: "actual-nested"})
	if got == nil || got.ModulesTouched != 2 || len(got.TopTouched) != 2 || got.FullHistory || calls != 1 {
		t.Fatalf("history = %+v, calls = %d", got, calls)
	}
	for _, touched := range got.TopTouched {
		if touched.TouchCommits != 1 {
			t.Fatalf("double-counted same-commit files: %+v", touched)
		}
	}
}

func TestHistoryDoesNotGuessUnavailableCrateIdentities(t *testing.T) {
	cfg := config.Config{Modules: map[string]policy.ModuleDef{historyCoreModule: {Paths: []string{historyCoreModule}}}}
	runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
		return toolrun.Output{Stdout: []byte("\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\ncore/src/lib.rs\x00")}, nil
	}}
	got := acquisition.BuildVolatilityCorroboration(context.Background(), "/repo", "", cfg.PolicySnapshot(), runner)
	if got == nil || got.ModulesTouched != 0 || got.Status != "partial" {
		t.Fatalf("unavailable crate identity was inferred: %+v", got)
	}
}
