// Behavior tests for the acquisition stage seam: what one Acquire resolves, how
// often it resolves it, and what it discloses.
package acquisition_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// gitOnlyRunner answers the git probes scope resolution makes and records every
// command, so a repeated repository walk is directly observable.
type gitOnlyRunner struct {
	root  string
	calls []string
}

func (r *gitOnlyRunner) Stream(ctx context.Context, cmd toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
	out, err := r.Run(ctx, cmd)
	if err != nil {
		return out, err
	}
	return out, consume(bytes.NewReader(out.Stdout))
}

func (r *gitOnlyRunner) Detect(context.Context, string) (toolrun.ToolInfo, bool) {
	return toolrun.ToolInfo{}, false
}

func (r *gitOnlyRunner) Run(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	r.calls = append(r.calls, cmd.Name+" "+strings.Join(cmd.Args, " "))
	if cmd.Name != "git" {
		return toolrun.Output{ExitCode: 1}, nil
	}
	if len(cmd.Args) >= 2 && cmd.Args[0] == "rev-parse" && cmd.Args[1] == "--show-toplevel" {
		return toolrun.Output{Stdout: []byte(r.root + "\n")}, nil
	}
	return toolrun.Output{}, nil
}

func acquisitionService(t *testing.T, root string, runner toolrun.Runner, stderr *bytes.Buffer) *acquisition.Service {
	t.Helper()
	cfg := config.Config{Version: 1, Modules: map[string]policy.ModuleDef{
		"a": {Paths: []string{"a/**"}},
	}}
	return &acquisition.Service{
		ConfigPath: filepath.Join(root, ".archfit.yaml"), Root: root,
		Options: cfg.RunOptions(), Policy: cfg.PolicySnapshot(),
		Runner: runner, Stderr: stderr,
	}
}

// TestAcquireResolvesOwnershipOncePerRunAndKeepsNoState pins two things. One
// run resolves ownership in a single pass — the ownership walk is the only
// `--format=%ae` history read, and it costs at most the documented bounded
// window plus its full-history fallback, never one pass per consumer. And the
// service carries nothing between runs: a second Acquire issues the same
// commands as the first, so a base-tree sub-run cannot inherit head-tree state.
func TestAcquireResolvesOwnershipOncePerRunAndKeepsNoState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "a.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &gitOnlyRunner{root: root}
	var stderr bytes.Buffer

	svc := acquisitionService(t, root, runner, &stderr)
	acquired, err := svc.Acquire(context.Background(), application.AnalysisRequest{EvaluatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if acquired.Context.OwnerSource == "" {
		t.Error("owner_source was not recorded: a run must disclose how ownership resolved")
	}
	first := append([]string(nil), runner.calls...)
	// The ownership pass is the only author-history read. Two is the ceiling:
	// the bounded window, then its full-history fallback when the window was
	// empty. A third would mean a second consumer resolved ownership itself.
	if got := countCalls(first, "--format=%ae"); got > 2 {
		t.Errorf("ownership resolved over %d history walks, want one pass: %v", got, first)
	}

	runner.calls = nil
	if _, err := svc.Acquire(context.Background(), application.AnalysisRequest{EvaluatedAt: time.Unix(1, 0)}); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != len(first) {
		t.Errorf("second run issued %d commands, first issued %d: acquisition must keep no state between runs", len(runner.calls), len(first))
	}
}

func countCalls(calls []string, substr string) int {
	n := 0
	for _, call := range calls {
		if strings.Contains(call, substr) {
			n++
		}
	}
	return n
}

// TestAcquireCarriesTheRunContextForLaterStages pins the context contract: every
// identity a later stage needs is resolved here, so nothing downstream has to
// re-read the config file or re-probe the tree.
func TestAcquireCarriesTheRunContextForLaterStages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".archfit.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	acquired, err := acquisitionService(t, root, &gitOnlyRunner{root: root}, &stderr).
		Acquire(context.Background(), application.AnalysisRequest{EvaluatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	ctx := acquired.Context
	if !ctx.Now.Equal(at) {
		t.Errorf("Now = %v, want the caller's instant: waiver expiry and staleness measure against one clock", ctx.Now)
	}
	if ctx.ConfigSource != cfgPath || ctx.ScanRoot != root {
		t.Errorf("config source %q / scan root %q, want the request's own paths", ctx.ConfigSource, ctx.ScanRoot)
	}
	if ctx.ConfigHash == "" {
		t.Error("config hash is empty: the origin delta cannot tell a code change from a policy change without it")
	}
	if len(ctx.PrimaryExtractorTools) == 0 {
		t.Error("primary extractor tools were not recorded")
	}
	if ctx.Scope.Root == "" {
		t.Error("scope was not resolved")
	}
}

// TestAcquireRequiresARunner pins the fail-fast contract: acquisition without a
// process boundary cannot observe anything, and must say so rather than return
// an empty measurement that reads like a clean tree.
func TestAcquireRequiresARunner(t *testing.T) {
	t.Parallel()
	svc := &acquisition.Service{ConfigPath: ".archfit.yaml"}
	if _, err := svc.Acquire(context.Background(), application.AnalysisRequest{}); err == nil {
		t.Fatal("a nil runner was accepted")
	}
}

// TestAcquireReadsPinnedLabelsFromTheRunBundle pins the compare invariant: a
// candidate config outside the current bundle must still classify with the
// current bundle's approved labels. Reading them next to the candidate file
// instead would attribute a label difference to the candidate configuration.
func TestAcquireReadsPinnedLabelsFromTheRunBundle(t *testing.T) {
	t.Parallel()
	bundle, elsewhere := t.TempDir(), t.TempDir()
	loader := &recordingLabelLoader{}
	svc := acquisitionService(t, bundle, &gitOnlyRunner{root: bundle}, &bytes.Buffer{})
	// The service was built for a config living in `elsewhere` — the shape
	// `config compare` produces for its candidate side.
	svc.ConfigPath = filepath.Join(elsewhere, "candidate.yaml")
	svc.Labels = loader

	if _, err := svc.Acquire(context.Background(), application.AnalysisRequest{
		ConfigSource: svc.ConfigPath, BundleDir: bundle, Root: bundle,
	}); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(bundle, ".archfit-labels.yaml"); loader.path != want {
		t.Fatalf("labels loaded from %q, want the run bundle %q", loader.path, want)
	}
}

type recordingLabelLoader struct{ path string }

func (l *recordingLabelLoader) Load(path string) ([]labels.Label, error) {
	l.path = path
	return nil, nil
}
