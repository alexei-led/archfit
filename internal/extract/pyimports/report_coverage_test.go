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

func TestReportFailedProducerPreservesApplicableSources(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mode    evidenceports.ToolMode
		missing bool
		scan    toolrun.Output
		scanErr error
		version toolrun.Output
		wantErr bool
	}{
		{name: "missing parser auto", missing: true},
		{name: "missing parser required", mode: evidenceports.ModeOn, missing: true, wantErr: true},
		{name: "scan failure auto", scan: toolrun.Output{ExitCode: 2}},
		{name: "scan failure required", mode: evidenceports.ModeOn, scan: toolrun.Output{ExitCode: 2}, wantErr: true},
		{name: "scan process error", scanErr: errors.New("scan failed")},
		{name: "version failure auto", scan: toolrun.Output{ExitCode: 1}, version: toolrun.Output{ExitCode: 2}},
		{name: "version failure required", mode: evidenceports.ModeOn, scan: toolrun.Output{ExitCode: 1}, version: toolrun.Output{ExitCode: 2}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, map[string]string{srcInit: "", srcA: "", "src/app/skip.py": "", "src/app/overlap.py": "", "src/app/overlap/__init__.py": ""})
			runner := &toolrun.RunnerMock{
				DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, !tc.missing },
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					if len(cmd.Args) == 1 && cmd.Args[0] == "--version" {
						return tc.version, nil
					}
					return tc.scan, tc.scanErr
				},
			}
			cfg := evidenceports.ExtractConfig{Mode: tc.mode, Exclusions: []string{"src/app/skip.py"}}
			facts, cov, err := New(runner, cfg).Extract(context.Background(), scope.Scope{Root: root})
			if (err != nil) != tc.wantErr || cov.Status != evidence.StatusPartial || cov.Reason == "" || cov.FilesApplicable != 3 || cov.FilesSeen != 0 || len(facts.Nodes) != 0 || len(facts.Edges) != 0 {
				t.Fatalf("facts=%+v coverage=%+v err=%v", facts, cov, err)
			}
		})
	}
}
