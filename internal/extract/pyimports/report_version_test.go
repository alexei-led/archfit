package pyimports

import (
	"context"
	"errors"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestReportVersionFailureRemainsIncomplete(t *testing.T) {
	t.Parallel()
	for _, mode := range []evidenceports.ToolMode{evidenceports.ModeAuto, evidenceports.ModeOn} {
		for _, processErr := range []error{nil, errors.New("version probe failed")} {
			t.Run(string(mode)+":"+stringError(processErr), func(t *testing.T) {
				t.Parallel()
				root := writeTree(t, map[string]string{srcInit: ""})
				runner := &toolrun.RunnerMock{
					DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, true },
					RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
						if len(cmd.Args) == 1 && cmd.Args[0] == "--version" {
							return toolrun.Output{ExitCode: 2, Stderr: []byte("no version")}, processErr
						}
						return toolrun.Output{ExitCode: 1}, nil
					},
				}
				_, cov, err := New(runner, evidenceports.ExtractConfig{Mode: mode}).Extract(context.Background(), scope.Scope{Root: root})
				if cov.Status != evidence.StatusPartial || cov.Reason == "" || (err != nil) != (mode == evidenceports.ModeOn) {
					t.Fatalf("coverage=%+v err=%v, want incomplete producer identity", cov, err)
				}
			})
		}
	}
}

func stringError(err error) string {
	if err == nil {
		return "exit"
	}
	return "error"
}
