package pyimports

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

const missingReportPackage = "missing_report_package"

func TestReportQueryUsesTheResolverInventory(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		srcInit: "", srcA: "", "src/app/plugins/b.py": "",
		"loose.py": "", "src/app/shadow.py": "", "src/app/shadow/__init__.py": "",
	})
	for _, tc := range []struct {
		name, from, target string
		exclude            []string
		want               error
	}{
		{"dotted namespace", srcA, "app.plugins.b", nil, nil},
		{"file namespace", srcA, "src/app/plugins/b.py", nil, nil},
		{"missing target", srcA, "app.gone", nil, evidenceports.ErrNotDecidable},
		{"external", srcA, "os", nil, evidenceports.ErrNotDecidable},
		{"loose importer", "loose.py", modA, nil, evidenceports.ErrNotDecidable},
		{"excluded importer", srcA, modA, []string{"src/app/a.py"}, evidenceports.ErrNotExtracted},
		{"shadowed body", "src/app/shadow.py", modA, nil, evidenceports.ErrNotDecidable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			facts, err := QueryEdge(root, evidenceports.ExtractConfig{Exclusions: tc.exclude, Internal: []string{"app.plugins.*"}}, tc.from, tc.target)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v, want %v", err, tc.want)
			}
			if err == nil && (len(facts.Edges) != 1 || facts.Edges[0].From != "module:app.a" || facts.Edges[0].To != "module:app.plugins.b" || facts.Edges[0].Kind != graph.EdgeKindUsesInternal || facts.Edges[0].Locations[0].File != srcA) {
				t.Errorf("query edge = %+v", facts.Edges)
			}
		})
	}
}

func TestReportFailuresDoNotClaimCompleteEvidence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mode    evidenceports.ToolMode
		output  toolrun.Output
		runErr  error
		wantErr bool
	}{
		{"failed scan auto", evidenceports.ModeAuto, toolrun.Output{ExitCode: 2, Stderr: []byte("broken parser")}, nil, false},
		{"failed scan required", evidenceports.ModeOn, toolrun.Output{ExitCode: 2}, nil, true},
		{"invalid output", evidenceports.ModeAuto, toolrun.Output{Stdout: []byte("not JSON")}, nil, false},
		{"process error", evidenceports.ModeAuto, toolrun.Output{}, errors.New("cannot start parser"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, map[string]string{srcInit: ""})
			runner := &toolrun.RunnerMock{
				DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, true },
				RunFunc:    func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) { return tc.output, tc.runErr },
			}
			_, cov, err := New(runner, evidenceports.ExtractConfig{Mode: tc.mode}).Extract(context.Background(), scope.Scope{Root: root})
			if (err != nil) != tc.wantErr || cov.Status == evidence.StatusOK || cov.Reason == "" {
				t.Fatalf("coverage=%+v err=%v, want incomplete evidence (error=%v)", cov, err, tc.wantErr)
			}
		})
	}
	t.Run("cancelled", func(t *testing.T) {
		t.Parallel()
		root := writeTree(t, map[string]string{srcInit: ""})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		runner := &toolrun.RunnerMock{
			DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, true },
			RunFunc:    func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) { return toolrun.Output{}, ctx.Err() },
		}
		_, _, err := New(runner, evidenceports.ExtractConfig{}).Extract(ctx, scope.Scope{Root: root})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want cancellation", err)
		}
	})
	t.Run("missing configured package", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		_, cov, err := New(&toolrun.RunnerMock{}, evidenceports.ExtractConfig{PyPackage: missingReportPackage}).Extract(context.Background(), scope.Scope{Root: root})
		if err != nil || cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, missingReportPackage) {
			t.Fatalf("coverage=%+v err=%v, want missing configured source", cov, err)
		}
	})
}
