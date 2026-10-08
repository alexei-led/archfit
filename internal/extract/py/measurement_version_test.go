package py_test

import (
	"context"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/py"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestMeasurementVersionNamesActualPythonProducer(t *testing.T) {
	for _, producer := range []string{"grimp 3.12; python 3.13.7", ""} {
		runner := &toolrun.RunnerMock{
			DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{Name: "uv"}, true },
			RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
				if len(cmd.Args) == 1 && cmd.Args[0] == "--version" {
					return toolrun.Output{Stdout: []byte("uv 0.9.0")}, nil
				}
				return toolrun.Output{Stdout: []byte(`{"edges":[],"producer_version":"` + producer + `"}`)}, nil
			},
		}
		ex := py.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto})
		_, cov, err := ex.Extract(context.Background(), scope.Scope{Root: testRoot})
		if err != nil {
			t.Fatal(err)
		}
		if cov.Version != producer {
			t.Fatalf("version=%q, want actual producer %q", cov.Version, producer)
		}
	}
}
