package clones

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/model/clone"
	reportmodel "github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	// fileAB is a test path reused across fixture JSON and assertions.
	fileAB = "internal/b/b.go"
)

// jscpdSuccessJSON is a canned jscpd JSON report with two duplicate entries over
// a 12-file scan (statistics.total.sources). The first entry's start/end mirror
// jscpd's real reporter output (verified against a live jscpd run); the second
// omits them to cover reports from older jscpd versions with no line data.
const jscpdSuccessJSON = `{
	"duplicates": [
		{
			"firstFile":  {"name": "internal/a/a.go", "start": 3, "end": 12},
			"secondFile": {"name": "internal/b/b.go", "start": 8, "end": 17},
			"lines": 25
		},
		{
			"firstFile":  {"name": "internal/c/c.go"},
			"secondFile": {"name": "internal/d/d.go"},
			"lines": 10
		}
	],
	"statistics": {"total": {"sources": 12}}
}`

// jscpdEmptyJSON is a valid report with no duplicates.
const jscpdEmptyJSON = `{"duplicates": []}`

// makeReportRunner returns a RunnerMock that:
//   - detects jscpd as present
//   - on Run: writes the given JSON to <outDir>/jscpd-report.json (where outDir
//     is taken from the --output argument) and exits 0
func makeReportRunner(reportJSON string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			// Find --output arg and write the report file there.
			for i, arg := range cmd.Args {
				if arg == flagOutput && i+1 < len(cmd.Args) {
					outDir := cmd.Args[i+1]
					dest := filepath.Join(outDir, reportFile)
					_ = os.WriteFile(dest, []byte(reportJSON), 0o600)
					break
				}
			}
			return toolrun.Output{ExitCode: 0}, nil
		},
	}
}

// absentRunner returns a RunnerMock where no tool is found.
func absentRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
}

// failRunner detects jscpd but Run returns exit code 1 (tool failure).
func failRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 1}, nil
		},
	}
}

// malformedRunner detects jscpd and writes malformed JSON to the report file.
func malformedRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			for i, arg := range cmd.Args {
				if arg == flagOutput && i+1 < len(cmd.Args) {
					outDir := cmd.Args[i+1]
					dest := filepath.Join(outDir, reportFile)
					_ = os.WriteFile(dest, []byte("not valid json at all"), 0o600)
					break
				}
			}
			return toolrun.Output{ExitCode: 0}, nil
		},
	}
}

func TestRun_Success(t *testing.T) {
	clusters, cov, err := Run(context.Background(), makeReportRunner(jscpdSuccessJSON), t.TempDir(), true, 0, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusOK {
		t.Errorf("coverage status = %q, want %q", cov.Status, reportmodel.StatusOK)
	}
	if cov.FilesSeen != 12 || cov.FilesApplicable != 12 {
		t.Errorf("coverage files = %d/%d, want 12/12 (scanned files, not clone pairs)", cov.FilesSeen, cov.FilesApplicable)
	}
	if len(clusters) != 2 {
		t.Fatalf("clusters len = %d, want 2", len(clusters))
	}
	if clusters[0].Files[0] != "internal/a/a.go" || clusters[0].Files[1] != fileAB {
		t.Errorf("cluster[0].Files = %v, want [internal/a/a.go %s]", clusters[0].Files, fileAB)
	}
	if clusters[0].Lines != 25 {
		t.Errorf("cluster[0].Lines = %d, want 25", clusters[0].Lines)
	}
	// B6: jscpd's real per-side start/end lines must survive JSON parsing into
	// clone.Cluster.Locations instead of being silently dropped.
	wantLocs := []clone.LineRange{{StartLine: 3, EndLine: 12}, {StartLine: 8, EndLine: 17}}
	if len(clusters[0].Locations) != 2 || clusters[0].Locations[0] != wantLocs[0] || clusters[0].Locations[1] != wantLocs[1] {
		t.Errorf("cluster[0].Locations = %v, want %v", clusters[0].Locations, wantLocs)
	}
	// The second duplicate carries no start/end in the fixture (older jscpd
	// report) — Locations must degrade to the zero value, not panic or omit
	// the slice entirely.
	if len(clusters[1].Locations) != 2 || clusters[1].Locations[0] != (clone.LineRange{}) {
		t.Errorf("cluster[1].Locations = %v, want two zero-value entries", clusters[1].Locations)
	}
}

func TestRun_AbsentTool(t *testing.T) {
	clusters, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), true, 0, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusAbsent {
		t.Errorf("coverage status = %q, want %q", cov.Status, reportmodel.StatusAbsent)
	}
	if len(clusters) != 0 {
		t.Errorf("expected empty clusters, got %d", len(clusters))
	}
}

func TestRun_Disabled(t *testing.T) {
	// enabled=false → disabled status (not absent), no Detect call needed.
	// The tool may or may not be installed; the user turned it off in config.
	clusters, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), false, 0, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusDisabled {
		t.Errorf("coverage status = %q, want %q", cov.Status, reportmodel.StatusDisabled)
	}
	if len(clusters) != 0 {
		t.Errorf("expected empty clusters for disabled tool, got %d", len(clusters))
	}
}

// TestRun_StatusDistinction asserts that disabled-by-config and tool-absent
// produce different statuses so the pipeline can distinguish them.
// disabled → StatusDisabled (do not show "install" prompt).
// not installed but enabled → StatusAbsent (show "install" prompt).
func TestRun_StatusDistinction(t *testing.T) {
	_, covDisabled, _ := Run(context.Background(), absentRunner(), t.TempDir(), false, 0, nil, nil)
	if covDisabled.Status != reportmodel.StatusDisabled {
		t.Errorf("disabled status = %q, want %q", covDisabled.Status, reportmodel.StatusDisabled)
	}
	if covDisabled.Reason != reasonDisabled {
		t.Errorf("disabled reason = %q, want %q", covDisabled.Reason, reasonDisabled)
	}

	_, covAbsent, _ := Run(context.Background(), absentRunner(), t.TempDir(), true, 0, nil, nil)
	if covAbsent.Status != reportmodel.StatusAbsent {
		t.Errorf("absent status = %q, want %q", covAbsent.Status, reportmodel.StatusAbsent)
	}
	if covAbsent.Reason != reasonNotInstalled {
		t.Errorf("absent reason = %q, want %q", covAbsent.Reason, reasonNotInstalled)
	}
}

func TestRun_MalformedOutput(t *testing.T) {
	clusters, cov, err := Run(context.Background(), malformedRunner(), t.TempDir(), true, 0, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, reportmodel.StatusPartial)
	}
	if len(clusters) != 0 {
		t.Errorf("expected empty clusters for malformed output, got %d", len(clusters))
	}
}

func TestRun_ToolFailure(t *testing.T) {
	clusters, cov, err := Run(context.Background(), failRunner(), t.TempDir(), true, 0, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, reportmodel.StatusPartial)
	}
	if len(clusters) != 0 {
		t.Errorf("expected empty clusters for tool failure, got %d", len(clusters))
	}
}

// blockingRunner detects jscpd but its Run method blocks until the context is done.
// Used to test that the per-analyzer timeout fires and returns StatusTimedOut.
func blockingRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(ctx context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			<-ctx.Done()
			return toolrun.Output{}, ctx.Err()
		},
	}
}

// TestRun_Timeout asserts that when the per-analyzer watchdog fires the run
// returns StatusTimedOut coverage, a nil error, and no deadlock.
// The test must complete quickly (short timeout + blocking runner).
func TestRun_Timeout(t *testing.T) {
	clusters, cov, err := Run(context.Background(), blockingRunner(), t.TempDir(), true, 10*time.Millisecond, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusTimedOut {
		t.Errorf("status = %q, want %q", cov.Status, reportmodel.StatusTimedOut)
	}
	if cov.Reason != reasonTimedOut {
		t.Errorf("reason = %q, want %q", cov.Reason, reasonTimedOut)
	}
	if len(clusters) != 0 {
		t.Errorf("expected no clusters on timeout, got %d", len(clusters))
	}
}

func TestModulePairs_CrossModule(t *testing.T) {
	clusters := []clone.Cluster{
		{Files: []string{"internal/a/a.go", "internal/b/b.go"}, Lines: 25},
		{Files: []string{"internal/c/c.go", "internal/d/d.go"}, Lines: 10},
	}
	// key: use directory as module (mimics Go fileToModuleKey behaviour)
	key := func(f string) string {
		for i := len(f) - 1; i >= 0; i-- {
			if f[i] == '/' {
				return f[:i]
			}
		}
		return ""
	}
	pairs := clone.ModulePairs(clusters, key)
	if len(pairs) != 2 {
		t.Fatalf("pairs len = %d, want 2", len(pairs))
	}
	if pairs[0] != [2]string{"internal/a", "internal/b"} {
		t.Errorf("pairs[0] = %v, want [internal/a internal/b]", pairs[0])
	}
	if pairs[1] != [2]string{"internal/c", "internal/d"} {
		t.Errorf("pairs[1] = %v, want [internal/c internal/d]", pairs[1])
	}
}

func TestModulePairs_SameModule_Skipped(t *testing.T) {
	clusters := []clone.Cluster{
		{Files: []string{"internal/a/x.go", "internal/a/y.go"}, Lines: 15},
	}
	key := func(f string) string {
		for i := len(f) - 1; i >= 0; i-- {
			if f[i] == '/' {
				return f[:i]
			}
		}
		return ""
	}
	pairs := clone.ModulePairs(clusters, key)
	if len(pairs) != 0 {
		t.Errorf("expected 0 pairs for same-module cluster, got %d: %v", len(pairs), pairs)
	}
}

func TestModulePairs_Dedup(t *testing.T) {
	// Two clusters both map to the same module pair — should produce one pair.
	clusters := []clone.Cluster{
		{Files: []string{"internal/a/x.go", "internal/b/y.go"}, Lines: 10},
		{Files: []string{"internal/a/z.go", "internal/b/w.go"}, Lines: 5},
	}
	key := func(f string) string {
		for i := len(f) - 1; i >= 0; i-- {
			if f[i] == '/' {
				return f[:i]
			}
		}
		return ""
	}
	pairs := clone.ModulePairs(clusters, key)
	if len(pairs) != 1 {
		t.Errorf("expected 1 deduplicated pair, got %d: %v", len(pairs), pairs)
	}
}

func TestModulePairs_EmptyKey_Skipped(t *testing.T) {
	// Files that map to empty key should be ignored.
	clusters := []clone.Cluster{
		{Files: []string{"rootfile.go", "internal/b/b.go"}, Lines: 8},
	}
	// key returns empty for root files (no slash), non-empty for others
	key := func(f string) string {
		for i := len(f) - 1; i >= 0; i-- {
			if f[i] == '/' {
				return f[:i]
			}
		}
		return ""
	}
	pairs := clone.ModulePairs(clusters, key)
	// rootfile.go → empty key (skipped); only b remains; single-mod cluster → no pairs
	if len(pairs) != 0 {
		t.Errorf("expected 0 pairs when one file has empty key, got %d", len(pairs))
	}
}

// capturingRunner detects jscpd as present and captures the ToolCmd passed to Run.
// The captured command is stored via the provided pointer so the caller can inspect
// which args were passed to jscpd (e.g. whether --ignore is present).
func capturingRunner(captured *toolrun.ToolCmd, reportJSON string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			*captured = cmd
			// Write the report so Run can parse it successfully.
			for i, arg := range cmd.Args {
				if arg == flagOutput && i+1 < len(cmd.Args) {
					outDir := cmd.Args[i+1]
					dest := filepath.Join(outDir, reportFile)
					_ = os.WriteFile(dest, []byte(reportJSON), 0o600)
					break
				}
			}
			return toolrun.Output{ExitCode: 0}, nil
		},
	}
}

// hasArg returns true when args contains the target string.
func hasArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

// argAfter returns the value immediately following flag in args, or "" if absent.
func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// TestRun_ExclusionsPassedAsIgnore verifies that when exclusions are provided,
// jscpd is invoked with --ignore "<comma-separated-globs>".
func TestRun_ExclusionsPassedAsIgnore(t *testing.T) {
	excl := []string{"**/vendor/**", "**/testdata/**"}
	var captured toolrun.ToolCmd
	_, _, err := Run(context.Background(), capturingRunner(&captured, jscpdEmptyJSON), t.TempDir(), true, 0, excl, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasArg(captured.Args, flagIgnore) {
		t.Errorf("jscpd args %v missing %q flag", captured.Args, flagIgnore)
	}
	got := argAfter(captured.Args, flagIgnore)
	want := "**/vendor/**,**/testdata/**"
	if got != want {
		t.Errorf("--ignore value = %q, want %q", got, want)
	}
}

// TestRun_NoExclusions_NoIgnoreFlag verifies that when no exclusions are configured
// the jscpd invocation is byte-identical to before this change: no --ignore flag.
func TestRun_NoExclusions_NoIgnoreFlag(t *testing.T) {
	var captured toolrun.ToolCmd
	_, _, err := Run(context.Background(), capturingRunner(&captured, jscpdEmptyJSON), t.TempDir(), true, 0, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasArg(captured.Args, flagIgnore) {
		t.Errorf("jscpd args %v unexpectedly contains %q (no exclusions configured)", captured.Args, flagIgnore)
	}
}

func TestParseJscpdReport_Success(t *testing.T) {
	clusters, filesScanned, err := parseJscpdReport([]byte(jscpdSuccessJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("want 2 clusters, got %d", len(clusters))
	}
	if clusters[0].Lines != 25 {
		t.Errorf("clusters[0].Lines = %d, want 25", clusters[0].Lines)
	}
	// FilesSeen tracks files scanned, not clone pairs: 12 sources, 2 pairs.
	if filesScanned != 12 {
		t.Errorf("filesScanned = %d, want 12 (statistics.total.sources, not clone count)", filesScanned)
	}
}

func TestParseJscpdReport_Empty(t *testing.T) {
	clusters, filesScanned, err := parseJscpdReport([]byte(jscpdEmptyJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("want 0 clusters, got %d", len(clusters))
	}
	if filesScanned != 0 {
		t.Errorf("filesScanned = %d, want 0 (no statistics block)", filesScanned)
	}
}

func TestParseJscpdReport_Malformed(t *testing.T) {
	_, _, err := parseJscpdReport([]byte("not json"))
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

// exitOneWithReportRunner simulates old npm jscpd (≤3.x) behavior: exits with
// code 1 when duplicates are found, but writes a valid JSON report to disk
// before exiting. archfit must NOT discard the report on non-zero exit.
func exitOneWithReportRunner(reportJSON string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			// Write the report (like real jscpd does) then return exit code 1.
			for i, arg := range cmd.Args {
				if arg == flagOutput && i+1 < len(cmd.Args) {
					outDir := cmd.Args[i+1]
					dest := filepath.Join(outDir, reportFile)
					_ = os.WriteFile(dest, []byte(reportJSON), 0o600)
					break
				}
			}
			return toolrun.Output{ExitCode: 1}, nil
		},
	}
}

// TestRun_NonZeroExitWithValidReport is the regression test for the old npm
// jscpd behavior: exit code 1 + valid report on disk → clusters must be
// non-empty. Before the fix, archfit discarded the report on ExitCode != 0.
func TestRun_NonZeroExitWithValidReport(t *testing.T) {
	const rustReport = `{
		"duplicates": [
			{
				"firstFile":  {"name": "src/module_a/lib.rs"},
				"secondFile": {"name": "src/module_b/lib.rs"},
				"lines": 12
			}
		],
		"statistics": {"total": {"sources": 2}}
	}`
	clusters, cov, err := Run(context.Background(), exitOneWithReportRunner(rustReport), t.TempDir(), true, 0, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != reportmodel.StatusOK {
		t.Errorf("coverage status = %q, want %q (report on disk must not be discarded on exit 1)", cov.Status, reportmodel.StatusOK)
	}
	if len(clusters) != 1 {
		t.Fatalf("clusters len = %d, want 1 (exit 1 + valid report should yield clusters)", len(clusters))
	}
	if clusters[0].Files[0] != "src/module_a/lib.rs" || clusters[0].Files[1] != "src/module_b/lib.rs" {
		t.Errorf("cluster files = %v, want [src/module_a/lib.rs src/module_b/lib.rs]", clusters[0].Files)
	}
	if clusters[0].Lines != 12 {
		t.Errorf("cluster lines = %d, want 12", clusters[0].Lines)
	}
	if cov.FilesSeen != 2 {
		t.Errorf("files seen = %d, want 2", cov.FilesSeen)
	}
}

// TestRun_RealTool_RustClones runs jscpd against a tiny Rust fixture with an
// identical block in two files and asserts non-empty clusters. Skipped under
// -short so CI can opt out when jscpd is not installed.
func TestRun_RealTool_RustClones(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-tool test under -short")
	}

	// Build a minimal Rust fixture: two files sharing an identical 8-line function.
	root := t.TempDir()
	modA := filepath.Join(root, "src", "module_a")
	modB := filepath.Join(root, "src", "module_b")
	if err := os.MkdirAll(modA, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(modB, 0o750); err != nil {
		t.Fatal(err)
	}
	sharedBlock := `pub fn process_items(items: &[String]) -> Vec<String> {
    let mut result = Vec::new();
    for item in items {
        if item.len() > 3 {
            result.push(item.clone());
        }
    }
    result
}
`
	if err := os.WriteFile(filepath.Join(modA, "lib.rs"), []byte(sharedBlock+"\npub fn unique_a() -> i32 { 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modB, "lib.rs"), []byte(sharedBlock+"\npub fn unique_b() -> i32 { 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := toolrun.New()
	clusters, cov, err := Run(context.Background(), runner, root, true, 0, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status == reportmodel.StatusAbsent {
		t.Skip("jscpd not installed — skipping real-tool test")
	}
	if cov.Status != reportmodel.StatusOK {
		t.Fatalf("coverage status = %q (reason: %s), want %q", cov.Status, cov.Reason, reportmodel.StatusOK)
	}
	if len(clusters) == 0 {
		t.Error("expected at least one clone cluster for identical Rust blocks, got 0")
	}
}
