package rust_test

import (
	"context"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/rust"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const measurementVersionFlag = "--version"

func TestModuleGraphVersionDoesNotDependOnCache(t *testing.T) {
	for _, cached := range []bool{false, true} {
		runner := moduleGraphRunner(libCargoMeta, libCrateDOT)
		run := runner.RunFunc
		runner.RunFunc = func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == toolModules && len(cmd.Args) == 1 && cmd.Args[0] == measurementVersionFlag {
				return toolrun.Output{Stdout: []byte("cargo-modules 0.25.0")}, nil
			}
			return run(ctx, cmd)
		}
		ex := rust.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, ModuleGraph: true})
		if cached {
			ex.Cache = factcache.NewStore(t.TempDir())
		}
		if _, _, err := ex.Extract(context.Background(), scope.Scope{Root: fixtureDir}); err != nil {
			t.Fatal(err)
		}
		if got := ex.LastModuleGraphCoverage().Version; got != "cargo-modules 0.25.0" {
			t.Fatalf("cache=%v version=%q", cached, got)
		}
	}
}
