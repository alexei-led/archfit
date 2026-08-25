package pipeline

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	testCorModuleCore = "mod.core"
	testCorModuleUtil = "mod.util"
	testCorPathCore   = "internal/core/**"
	testCorPathUtil   = "internal/util/**"
)

func TestBuildVolatilityCorroboration_RankedTouches(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		testCorModuleCore: {Paths: []string{testCorPathCore}, Subdomain: subdomainCore},
		testCorModuleUtil: {Paths: []string{testCorPathUtil}, Volatility: volatilityLow},
	}}
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(
				"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
					"internal/core/a.go\n" +
					"internal/util/u.go\n" +
					"\n" +
					"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
					"internal/core/b.go\n"), ExitCode: 0}, nil
		},
	}

	got := BuildVolatilityCorroboration(context.Background(), "/repo", "", PolicySnapshot(cfg), runner)
	if got == nil {
		t.Fatal("buildVolatilityCorroboration = nil, want block")
	}
	if got.Source != volatilityCorroborationSrc || got.Status != "ok" {
		t.Fatalf("source/status = %q/%q", got.Source, got.Status)
	}
	if got.CommitWindow != 500 || got.FullHistory {
		t.Fatalf("window/full = %d/%t, want 500/false", got.CommitWindow, got.FullHistory)
	}
	if got.ModulesTouched != 2 || got.CommitsScanned != 2 {
		t.Fatalf("modules/commits = %d/%d, want 2/2", got.ModulesTouched, got.CommitsScanned)
	}
	if len(got.TopTouched) != 2 {
		t.Fatalf("top_touched len = %d, want 2: %+v", len(got.TopTouched), got.TopTouched)
	}
	if got.TopTouched[0].Module != testCorModuleCore || got.TopTouched[0].TouchCommits != 2 || got.TopTouched[0].DeclaredVolatility != volatilityHigh {
		t.Fatalf("top_touched[0] = %+v", got.TopTouched[0])
	}
	if got.TopTouched[1].Module != testCorModuleUtil || got.TopTouched[1].DeclaredVolatility != volatilityLow {
		t.Fatalf("top_touched[1] = %+v", got.TopTouched[1])
	}
}

func TestBuildVolatilityCorroboration_TimeoutStillReportsStatus(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		testCorModuleCore: {Paths: []string{testCorPathCore}, Subdomain: subdomainCore},
	}}
	runner := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{}, context.DeadlineExceeded
		},
	}

	got := BuildVolatilityCorroboration(context.Background(), "/repo", "", PolicySnapshot(cfg), runner)
	if got == nil {
		t.Fatal("buildVolatilityCorroboration = nil, want timeout block")
	}
	if got.Status != "timeout" || got.ModulesTouched != 0 || len(got.TopTouched) != 0 {
		t.Fatalf("timeout block = %+v", got)
	}
}

func TestBuildVolatilityCorroboration_NonGitOmitted(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Modules: map[string]policy.ModuleDef{
		testCorModuleCore: {Paths: []string{testCorPathCore}, Subdomain: subdomainCore},
	}}
	if got := BuildVolatilityCorroboration(context.Background(), "", "", PolicySnapshot(cfg), nil); got != nil {
		t.Fatalf("buildVolatilityCorroboration = %+v, want nil when git root is absent", got)
	}
}
