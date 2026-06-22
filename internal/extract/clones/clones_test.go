package clones

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/model/clone"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	// fileAB is a test path reused across fixture JSON and assertions.
	fileAB = "internal/b/b.go"
)

// jscpdSuccessJSON is a canned jscpd JSON report with two duplicate entries over
// a 12-file scan (statistics.total.sources).
const jscpdSuccessJSON = `{
	"duplicates": [
		{
			"firstFile":  {"name": "internal/a/a.go"},
			"secondFile": {"name": "internal/b/b.go"},
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
				if arg == "--output" && i+1 < len(cmd.Args) {
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
				if arg == "--output" && i+1 < len(cmd.Args) {
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
	clusters, cov, err := Run(context.Background(), makeReportRunner(jscpdSuccessJSON), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusOK {
		t.Errorf("coverage status = %q, want %q", cov.Status, diagnostic.StatusOK)
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
}

func TestRun_AbsentTool(t *testing.T) {
	clusters, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusAbsent {
		t.Errorf("coverage status = %q, want %q", cov.Status, diagnostic.StatusAbsent)
	}
	if len(clusters) != 0 {
		t.Errorf("expected empty clusters, got %d", len(clusters))
	}
}

func TestRun_Disabled(t *testing.T) {
	// enabled=false → disabled status (not absent), no Detect call needed.
	// The tool may or may not be installed; the user turned it off in config.
	clusters, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusDisabled {
		t.Errorf("coverage status = %q, want %q", cov.Status, diagnostic.StatusDisabled)
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
	_, covDisabled, _ := Run(context.Background(), absentRunner(), t.TempDir(), false)
	if covDisabled.Status != diagnostic.StatusDisabled {
		t.Errorf("disabled status = %q, want %q", covDisabled.Status, diagnostic.StatusDisabled)
	}
	if covDisabled.Reason != reasonDisabled {
		t.Errorf("disabled reason = %q, want %q", covDisabled.Reason, reasonDisabled)
	}

	_, covAbsent, _ := Run(context.Background(), absentRunner(), t.TempDir(), true)
	if covAbsent.Status != diagnostic.StatusAbsent {
		t.Errorf("absent status = %q, want %q", covAbsent.Status, diagnostic.StatusAbsent)
	}
	if covAbsent.Reason != reasonNotInstalled {
		t.Errorf("absent reason = %q, want %q", covAbsent.Reason, reasonNotInstalled)
	}
}

func TestRun_MalformedOutput(t *testing.T) {
	clusters, cov, err := Run(context.Background(), malformedRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, diagnostic.StatusPartial)
	}
	if len(clusters) != 0 {
		t.Errorf("expected empty clusters for malformed output, got %d", len(clusters))
	}
}

func TestRun_ToolFailure(t *testing.T) {
	clusters, cov, err := Run(context.Background(), failRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != diagnostic.StatusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, diagnostic.StatusPartial)
	}
	if len(clusters) != 0 {
		t.Errorf("expected empty clusters for tool failure, got %d", len(clusters))
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
