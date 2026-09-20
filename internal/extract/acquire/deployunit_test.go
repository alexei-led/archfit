package acquire_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/acquire"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestCollectDeployUnitDiscoveryCompleteness(t *testing.T) {
	const goList = "go list"
	for _, tc := range []struct {
		name       string
		missing    bool
		runError   error
		exitCode   int
		main       bool
		docker     bool
		wantStatus string
		wantReason string
		wantUnits  int
	}{
		{name: "completed empty", wantStatus: evidence.StatusOK},
		{name: "completed main", main: true, wantStatus: evidence.StatusOK, wantUnits: 1},
		{name: "timeout", runError: context.DeadlineExceeded, wantStatus: evidence.StatusTimedOut, wantReason: goList},
		{name: "failed process", exitCode: 1, wantStatus: evidence.StatusPartial, wantReason: goList},
		{name: "execution error", runError: errors.New("cannot execute"), wantStatus: evidence.StatusPartial, wantReason: "cannot execute"},
		{name: "missing tool", missing: true, wantStatus: evidence.StatusAbsent, wantReason: "Go toolchain"},
		{name: "mixed sources timeout", runError: context.DeadlineExceeded, docker: true, wantStatus: evidence.StatusTimedOut, wantReason: goList, wantUnits: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.docker {
				if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
					return toolrun.ToolInfo{Name: name}, name == "go" && !tc.missing
				},
				RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
					var stdout []byte
					if tc.main {
						stdout = []byte(filepath.Join(root, "cmd", "app") + "\n")
					}
					return toolrun.Output{Stdout: stdout, ExitCode: tc.exitCode}, tc.runError
				},
			}
			got := acquire.Collect(context.Background(), root, acquire.Options{}, runner, nil)
			for _, coverage := range got.ExtraCoverage {
				if coverage.Tool != "deploy-unit" {
					continue
				}
				if coverage.Status != tc.wantStatus || !strings.Contains(coverage.Reason, tc.wantReason) || coverage.FilesSeen != tc.wantUnits {
					t.Fatalf("deploy coverage = %+v; want status %q, reason containing %q, %d units", coverage, tc.wantStatus, tc.wantReason, tc.wantUnits)
				}
				return
			}
			t.Fatal("missing deploy-unit coverage")
		})
	}
}
