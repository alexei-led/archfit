package clones

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/factcache"
	reportmodel "github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// cacheScanRunner fakes jscpd with a --version probe; scans write reportJSON
// to the --output dir and are counted. When block is true a scan hangs until
// the context is cancelled (the watchdog-timeout shape).
func cacheScanRunner(reportJSON string, scans *int, block bool) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == toolName
		},
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("4.0.0\n")}, nil
			}
			if block {
				<-ctx.Done()
				return toolrun.Output{}, ctx.Err()
			}
			*scans++
			for i, arg := range cmd.Args {
				if arg == flagOutput && i+1 < len(cmd.Args) {
					_ = os.WriteFile(filepath.Join(cmd.Args[i+1], reportFile), []byte(reportJSON), 0o600)
					break
				}
			}
			return toolrun.Output{ExitCode: 0}, nil
		},
	}
}

// TestFactCache_HitServesParsedResult pins the clones cache contract: a warm
// run on an unchanged tree skips jscpd and serves byte-identical clusters and
// coverage from the parsed cached fact.
func TestFactCache_HitServesParsedResult(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := factcache.NewStore(t.TempDir())
	scans := 0
	runner := cacheScanRunner(jscpdSuccessJSON, &scans, false)
	ctx := context.Background()

	cold, coldCov, err := Run(ctx, runner, root, true, 0, nil, store)
	if err != nil {
		t.Fatalf("cold Run: %v", err)
	}
	warm, warmCov, err := Run(ctx, runner, root, true, 0, nil, store)
	if err != nil {
		t.Fatalf("warm Run: %v", err)
	}
	if scans != 1 {
		t.Fatalf("want 1 jscpd scan (warm run cached), got %d", scans)
	}
	if !reflect.DeepEqual(cold, warm) {
		t.Errorf("warm clusters differ from cold:\ncold: %+v\nwarm: %+v", cold, warm)
	}
	if !reflect.DeepEqual(coldCov, warmCov) {
		t.Errorf("warm coverage differs from cold:\ncold: %+v\nwarm: %+v", coldCov, warmCov)
	}
}

// TestFactCache_TimedOutRunNotCached pins the D3 rule at the analyzer level:
// a timed-out jscpd run writes nothing, so the next run re-executes and can
// heal with a real result.
func TestFactCache_TimedOutRunNotCached(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := factcache.NewStore(t.TempDir())
	scans := 0

	_, cov, err := Run(context.Background(), cacheScanRunner("", &scans, true), root, true, 10*time.Millisecond, nil, store)
	if err != nil {
		t.Fatalf("timed-out Run: %v", err)
	}
	if cov.Status != reportmodel.StatusTimedOut {
		t.Fatalf("want StatusTimedOut, got %s", cov.Status)
	}

	// Healthy re-run: must invoke jscpd (nothing was cached by the timeout).
	clusters, cov, err := Run(context.Background(), cacheScanRunner(jscpdSuccessJSON, &scans, false), root, true, 0, nil, store)
	if err != nil {
		t.Fatalf("healthy Run: %v", err)
	}
	if scans != 1 {
		t.Errorf("want the healthy run to scan (timeout not cached), got %d scans", scans)
	}
	if cov.Status != reportmodel.StatusOK || len(clusters) == 0 {
		t.Errorf("healthy run: want OK coverage with clusters, got %s / %d clusters", cov.Status, len(clusters))
	}
}
