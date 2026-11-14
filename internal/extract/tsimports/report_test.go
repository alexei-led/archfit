package tsimports

import (
	"context"
	"errors"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const reportEmptyModule = "export {};\n"

func TestReportParserFailuresStayIncomplete(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mode    evidenceports.ToolMode
		output  toolrun.Output
		runErr  error
		wantErr bool
	}{
		{"scan failure auto", evidenceports.ModeAuto, toolrun.Output{ExitCode: 2, Stderr: []byte("parser failed")}, nil, false},
		{"scan failure required", evidenceports.ModeOn, toolrun.Output{ExitCode: 2}, nil, true},
		{"malformed scan output", evidenceports.ModeAuto, toolrun.Output{Stdout: []byte("not json")}, nil, false},
		{"process error", evidenceports.ModeAuto, toolrun.Output{}, errors.New("cannot start parser"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, map[string]string{"a.ts": reportEmptyModule})
			runner := &toolrun.RunnerMock{
				DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, true },
				RunFunc:    func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) { return tc.output, tc.runErr },
			}
			facts, cov, err := New(runner, evidenceports.ExtractConfig{Mode: tc.mode}).Extract(context.Background(), scope.Scope{Root: root})
			if (err != nil) != tc.wantErr || cov.Status == evidence.StatusOK || cov.Reason == "" || cov.FilesApplicable != 1 || cov.FilesSeen != 0 || len(facts.Nodes) != 0 {
				t.Fatalf("coverage=%+v err=%v", cov, err)
			}
		})
	}
}

func TestReportVersionFailureStaysIncomplete(t *testing.T) {
	t.Parallel()
	for _, mode := range []evidenceports.ToolMode{evidenceports.ModeAuto, evidenceports.ModeOn} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, map[string]string{"a.ts": reportEmptyModule})
			runner := &toolrun.RunnerMock{
				DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, true },
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					if len(cmd.Args) == 1 && cmd.Args[0] == "--version" {
						return toolrun.Output{ExitCode: 2, Stderr: []byte("no version")}, nil
					}
					return toolrun.Output{ExitCode: 1}, nil
				},
			}
			facts, cov, err := New(runner, evidenceports.ExtractConfig{Mode: mode}).Extract(context.Background(), scope.Scope{Root: root})
			if cov.Status != evidence.StatusPartial || cov.Reason == "" || (err != nil) != (mode == evidenceports.ModeOn) || cov.FilesApplicable != 1 || cov.FilesSeen != 0 || len(facts.Nodes) != 0 {
				t.Fatalf("coverage=%+v err=%v", cov, err)
			}
		})
	}
}

func TestReportQuerySpellsInternalFileEdge(t *testing.T) {
	t.Parallel()
	cfg := evidenceports.ExtractConfig{Internal: []string{"private/**"}}
	facts, err := QueryEdge("", cfg, "app/a.ts", "private/b.ts")
	if err != nil || len(facts.Edges) != 1 || facts.Edges[0].From != "file:app/a.ts" || facts.Edges[0].To != "file:private/b.ts" || facts.Edges[0].Kind != graph.EdgeKindUsesInternal {
		t.Fatalf("facts=%+v err=%v", facts, err)
	}
	if _, err := QueryEdge("", cfg, "app/a.ts", "private/b"); err == nil || !strings.Contains(err.Error(), "source files") {
		t.Fatalf("invalid target err=%v", err)
	}
}

func TestReportApplicabilityRespectsExclusions(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{"ignored/a.ts": reportEmptyModule})
	cfg := evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Exclusions: []string{"ignored", "ignored/**"}}
	runner := &toolrun.RunnerMock{DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) {
		t.Fatal("excluded source triggered parser")
		return toolrun.ToolInfo{}, false
	}}
	if Applicable(root, cfg.Exclusions) {
		t.Fatal("excluded source is applicable")
	}
	_, cov, err := New(runner, cfg).Extract(context.Background(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusAbsent {
		t.Fatalf("coverage=%+v err=%v", cov, err)
	}
}
