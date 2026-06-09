package gitnexus

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/internal/toolrun"
)

// gitnexusSuccessJSON is a canned gitnexus JSON response with two modules.
const gitnexusSuccessJSON = `[
	{"module":"internal/store","impact":42},
	{"module":"internal/util","impact":3}
]`

// gitnexusEmptyJSON is a valid but empty response.
const gitnexusEmptyJSON = `[]`

// makeImpactRunner returns a RunnerMock that detects gitnexus and returns the
// given JSON on stdout with exit code 0.
func makeImpactRunner(jsonOutput string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 0, Stdout: []byte(jsonOutput)}, nil
		},
	}
}

// absentRunner returns a RunnerMock where no tool is detected.
func absentRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, _ string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{}, false
		},
	}
}

// failRunner detects gitnexus but Run returns exit code 1.
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

// malformedRunner detects gitnexus and returns non-JSON stdout.
func malformedRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: "/usr/bin/" + tool}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 0, Stdout: []byte("not valid json at all")}, nil
		},
	}
}

func TestRun_EnabledPresent(t *testing.T) {
	impact, cov, err := Run(context.Background(), makeImpactRunner(gitnexusSuccessJSON), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusOK)
	}
	if len(impact) != 2 {
		t.Fatalf("impact len = %d, want 2", len(impact))
	}
	if impact["internal/store"] != 42 {
		t.Errorf("internal/store impact = %d, want 42", impact["internal/store"])
	}
	if impact["internal/util"] != 3 {
		t.Errorf("internal/util impact = %d, want 3", impact["internal/util"])
	}
}

func TestRun_Disabled(t *testing.T) {
	impact, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusAbsent {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusAbsent)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map for disabled tool, got %d entries", len(impact))
	}
}

func TestRun_AbsentTool(t *testing.T) {
	impact, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusAbsent {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusAbsent)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map when tool absent, got %d entries", len(impact))
	}
}

func TestRun_ToolFailure(t *testing.T) {
	impact, cov, err := Run(context.Background(), failRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusPartial)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map on tool failure, got %d entries", len(impact))
	}
}

func TestRun_MalformedOutput(t *testing.T) {
	impact, cov, err := Run(context.Background(), malformedRunner(), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusPartial {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusPartial)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map for malformed output, got %d entries", len(impact))
	}
}

func TestRun_EmptyResult(t *testing.T) {
	impact, cov, err := Run(context.Background(), makeImpactRunner(gitnexusEmptyJSON), t.TempDir(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Errorf("coverage status = %q, want %q", cov.Status, statusOK)
	}
	if len(impact) != 0 {
		t.Errorf("expected empty impact map for empty result, got %d entries", len(impact))
	}
}

func TestParseImpact_Success(t *testing.T) {
	m, err := parseImpact([]byte(gitnexusSuccessJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("want 2 entries, got %d", len(m))
	}
	if m["internal/store"] != 42 {
		t.Errorf("internal/store = %d, want 42", m["internal/store"])
	}
}

func TestParseImpact_Empty(t *testing.T) {
	m, err := parseImpact([]byte(gitnexusEmptyJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("want 0 entries, got %d", len(m))
	}
}

func TestParseImpact_Malformed(t *testing.T) {
	_, err := parseImpact([]byte("not json"))
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

func TestParseImpact_EmptyModuleSkipped(t *testing.T) {
	// Records with an empty module key must be silently skipped.
	data := []byte(`[{"module":"","impact":99},{"module":"internal/real","impact":5}]`)
	m, err := parseImpact(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 1 {
		t.Fatalf("want 1 entry (empty module skipped), got %d", len(m))
	}
	if m["internal/real"] != 5 {
		t.Errorf("internal/real = %d, want 5", m["internal/real"])
	}
}
