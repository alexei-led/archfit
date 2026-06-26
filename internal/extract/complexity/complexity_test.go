package complexity

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/toolrun"
)

// lizardPath is a stand-in resolved path returned by the test runners' Detect.
const lizardPath = "/usr/bin/lizard"

// testRoot is the synthetic repo root used across test fixtures.
const testRoot = "/repo"

// ---------------------------------------------------------------------------
// parseLizardCSV — ported from cmd/archfit/complexity_parse_test.go
// ---------------------------------------------------------------------------

func TestParseLizardCSV_ValidRows(t *testing.T) {
	// nloc,ccn,token,param,length,location,file,function,long_name,start,end
	csvData := `283,72,1352,4,337,parse_entries@401-737@/r/src/transcript_parser.py,/r/src/transcript_parser.py,parse_entries,parse_entries( self),401,737
5,3,20,1,6,small@10-15@/r/src/x.py,/r/src/x.py,small,small( a),10,15
`
	funcs := parseLizardCSV([]byte(csvData), "/r")
	if len(funcs) != 2 {
		t.Fatalf("expected 2 funcs, got %d", len(funcs))
	}
	if funcs[0].Name != "parse_entries" || funcs[0].CCN != 72 || funcs[0].Line != 401 {
		t.Errorf("bad first record: %+v", funcs[0])
	}
	if funcs[0].File != "src/transcript_parser.py" {
		t.Errorf("file should be repo-relative, got %q", funcs[0].File)
	}
	if funcs[1].Name != "small" || funcs[1].CCN != 3 {
		t.Errorf("bad second record: %+v", funcs[1])
	}
}

// TestParseLizardCSV_TableDriven covers good rows, malformed CCN, and short rows.
func TestParseLizardCSV_TableDriven(t *testing.T) {
	root := testRoot
	tests := []struct {
		name      string
		csv       string
		wantCount int
		wantName  string
		wantCCN   int
		wantFile  string
	}{
		{
			name:      "valid row slash-normalised",
			csv:       "10,5,100,2,20,fn@1-20@/repo/pkg/a.go,/repo/pkg/a.go,fn,fn(),1,20\n",
			wantCount: 1,
			wantName:  "fn",
			wantCCN:   5,
			wantFile:  "pkg/a.go",
		},
		{
			name:      "non-numeric CCN skipped",
			csv:       "10,abc,100,2,20,fn@1-20@/repo/pkg/b.go,/repo/pkg/b.go,fn,fn(),1,20\n",
			wantCount: 0,
		},
		{
			name:      "short row (9 fields) skipped",
			csv:       "10,5,100,2,20,fn@1-20@/repo/pkg/c.go,/repo/pkg/c.go,fn,fn()\n",
			wantCount: 0,
		},
		{
			name:      "whitespace-padded CCN parsed",
			csv:       "10, 7 ,100,2,20,fn@1-20@/repo/pkg/d.go,/repo/pkg/d.go,fn,fn(),1,20\n",
			wantCount: 1,
			wantCCN:   7,
		},
		{
			name:      "path already relative kept as-is",
			csv:       "10,3,100,2,20,fn@1-20@pkg/e.go,pkg/e.go,fn,fn(),1,20\n",
			wantCount: 1,
			wantFile:  "pkg/e.go",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			funcs := parseLizardCSV([]byte(tc.csv), root)
			if len(funcs) != tc.wantCount {
				t.Fatalf("got %d funcs, want %d", len(funcs), tc.wantCount)
			}
			if tc.wantCount == 0 {
				return
			}
			if tc.wantName != "" && funcs[0].Name != tc.wantName {
				t.Errorf("Name = %q, want %q", funcs[0].Name, tc.wantName)
			}
			if tc.wantCCN != 0 && funcs[0].CCN != tc.wantCCN {
				t.Errorf("CCN = %d, want %d", funcs[0].CCN, tc.wantCCN)
			}
			if tc.wantFile != "" && funcs[0].File != tc.wantFile {
				t.Errorf("File = %q, want %q", funcs[0].File, tc.wantFile)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Run (lizard backend) — disabled / absent / ok paths
// ---------------------------------------------------------------------------

// lizardRunner builds a RunnerMock that detects lizard and returns canned CSV.
func lizardRunner(csvOutput string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: lizardPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 0, Stdout: []byte(csvOutput)}, nil
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

// failRunner detects lizard but the run exits non-zero.
func failRunner() *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: lizardPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 1}, nil
		},
	}
}

func TestRun_Disabled(t *testing.T) {
	funcs, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), false, BackendLizard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(funcs) != 0 {
		t.Errorf("expected empty funcs for disabled, got %d", len(funcs))
	}
	if cov.Status != statusAbsent {
		t.Errorf("Status = %q, want %q", cov.Status, statusAbsent)
	}
	// Zero file counts — coverage metric value must not shift.
	if cov.FilesSeen != 0 || cov.FilesApplicable != 0 {
		t.Errorf("disabled: FilesSeen=%d FilesApplicable=%d, want both 0", cov.FilesSeen, cov.FilesApplicable)
	}
	if cov.Tool != toolName {
		t.Errorf("Tool = %q, want %q", cov.Tool, toolName)
	}
}

func TestRun_AbsentTool(t *testing.T) {
	funcs, cov, err := Run(context.Background(), absentRunner(), t.TempDir(), true, BackendLizard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(funcs) != 0 {
		t.Errorf("expected empty funcs for absent tool, got %d", len(funcs))
	}
	if cov.Status != statusAbsent {
		t.Errorf("Status = %q, want %q", cov.Status, statusAbsent)
	}
	if cov.FilesSeen != 0 || cov.FilesApplicable != 0 {
		t.Errorf("absent: FilesSeen=%d FilesApplicable=%d, want both 0", cov.FilesSeen, cov.FilesApplicable)
	}
}

func TestRun_ToolFailure(t *testing.T) {
	funcs, cov, err := Run(context.Background(), failRunner(), t.TempDir(), true, BackendLizard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(funcs) != 0 {
		t.Errorf("expected empty funcs on tool failure, got %d", len(funcs))
	}
	if cov.Status != statusAbsent {
		t.Errorf("Status = %q, want %q", cov.Status, statusAbsent)
	}
}

func TestRun_Success(t *testing.T) {
	root := t.TempDir()
	// CSV uses /root prefix; path will be made relative inside parseLizardCSV.
	csv := "10,5,100,2,20,fn@1-20@/root/pkg/a.go,/root/pkg/a.go,fn,fn(),1,20\n"
	funcs, cov, err := Run(context.Background(), lizardRunner(csv), root, true, BackendLizard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Errorf("Status = %q, want %q", cov.Status, statusOK)
	}
	// FilesSeen/FilesApplicable must be 0 on success too (output stability).
	if cov.FilesSeen != 0 || cov.FilesApplicable != 0 {
		t.Errorf("ok: FilesSeen=%d FilesApplicable=%d, want both 0", cov.FilesSeen, cov.FilesApplicable)
	}
	// We get funcs back (path will differ from /root when root != /root, that's fine).
	_ = funcs
}

func TestRun_CoverageToolName(t *testing.T) {
	_, cov, _ := Run(context.Background(), absentRunner(), t.TempDir(), false, BackendLizard)
	if cov.Tool != toolName {
		t.Errorf("Tool = %q, want %q", cov.Tool, toolName)
	}
}

// TestRun_DisabledVsAbsent verifies disabled and absent-tool produce
// identical coverage shape (status absent, zero file counts) but distinct,
// actionable reasons so the report explains why complexity is n/a.
func TestRun_DisabledVsAbsent(t *testing.T) {
	root := t.TempDir()
	_, covDisabled, _ := Run(context.Background(), absentRunner(), root, false, BackendLizard)
	_, covAbsent, _ := Run(context.Background(), absentRunner(), root, true, BackendLizard)

	if covDisabled.Status != covAbsent.Status {
		t.Errorf("disabled Status %q != absent Status %q", covDisabled.Status, covAbsent.Status)
	}
	for _, cov := range []diagnostic.Coverage{covDisabled, covAbsent} {
		if cov.FilesSeen != 0 || cov.FilesApplicable != 0 || cov.Unresolved != 0 {
			t.Errorf("non-zero file counts: %+v", cov)
		}
	}
	if covDisabled.Reason == covAbsent.Reason {
		t.Errorf("disabled and absent should give different reasons, both = %q", covDisabled.Reason)
	}
}

// capturingRunner detects lizard, records the args of the last Run call, and
// returns the canned output. Used to assert how lizard is invoked.
func capturingRunner(out toolrun.Output, gotArgs *[]string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: lizardPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			*gotArgs = cmd.Args
			return out, nil
		},
	}
}

// TestRun_LanguageFlags asserts lizard is invoked with an explicit -l flag for
// every language archfit supports, so Python and TS are analysed regardless of
// lizard's default (version-dependent) language set.
func TestRun_LanguageFlags(t *testing.T) {
	var args []string
	_, _, err := Run(context.Background(), capturingRunner(toolrun.Output{ExitCode: 0}, &args), t.TempDir(), true, BackendLizard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]bool{"go": false, "python": false, "javascript": false, "typescript": false, "tsx": false}
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-l" {
			if _, ok := want[args[i+1]]; ok {
				want[args[i+1]] = true
			}
		}
	}
	for lang, seen := range want {
		if !seen {
			t.Errorf("lizard not scoped to language %q; args = %v", lang, args)
		}
	}
}

// TestRun_PythonAndTypeScriptExtraction asserts the extractor yields per-function
// records for Python (.py), TypeScript (.ts), and TSX (.tsx) sources — the
// languages that previously reported complexity n/a.
func TestRun_PythonAndTypeScriptExtraction(t *testing.T) {
	root := t.TempDir()
	row := func(ccn int, rel string) string {
		abs := filepath.Join(root, rel)
		return fmt.Sprintf("20,%d,200,3,40,fn@1-40@%s,%s,fn,fn(),1,40\n", ccn, abs, abs)
	}
	csv := row(8, "svc/api.py") + row(6, "web/app.ts") + row(4, "web/view.tsx")
	funcs, cov, err := Run(context.Background(), lizardRunner(csv), root, true, BackendLizard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Fatalf("Status = %q, want %q", cov.Status, statusOK)
	}
	byFile := map[string]int{}
	for _, f := range funcs {
		byFile[f.File] = f.CCN
	}
	for file, wantCCN := range map[string]int{"svc/api.py": 8, "web/app.ts": 6, "web/view.tsx": 4} {
		if got, ok := byFile[file]; !ok {
			t.Errorf("missing complexity record for %q; got %v", file, byFile)
		} else if got != wantCCN {
			t.Errorf("%s CCN = %d, want %d", file, got, wantCCN)
		}
	}
}

// TestRun_NonZeroExitWithOutput is the regression for the n/a-when-installed bug:
// lizard returns its warning count as the exit code, so a non-zero exit that
// still produced parseable CSV is a successful analysis (it found hot functions),
// not a failure. The records must be kept and coverage reported ok.
func TestRun_NonZeroExitWithOutput(t *testing.T) {
	root := t.TempDir()
	csv := "30,22,300,4,60,hot@1-60@/root/svc/big.py,/root/svc/big.py,hot,hot(a),1,60\n"
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			if tool == toolName {
				return toolrun.ToolInfo{Name: tool, Path: lizardPath}, true
			}
			return toolrun.ToolInfo{}, false
		},
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			// Exit 1 = one threshold warning, but CSV is still emitted.
			return toolrun.Output{ExitCode: 1, Stdout: []byte(csv)}, nil
		},
	}
	funcs, cov, err := Run(context.Background(), runner, root, true, BackendLizard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusOK {
		t.Errorf("Status = %q, want %q (non-zero exit with output is success)", cov.Status, statusOK)
	}
	if len(funcs) != 1 || funcs[0].CCN != 22 {
		t.Errorf("expected the hot function to survive non-zero exit, got %+v", funcs)
	}
}

// TestRun_NonZeroExitNoOutput asserts a non-zero exit with no parseable records
// is still a genuine failure (distinguished from the warning-count case above).
func TestRun_NonZeroExitNoOutput(t *testing.T) {
	funcs, cov, err := Run(context.Background(), failRunner(), t.TempDir(), true, BackendLizard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cov.Status != statusAbsent || cov.Reason != reasonRunFailed {
		t.Errorf("Status=%q Reason=%q, want absent/%q", cov.Status, cov.Reason, reasonRunFailed)
	}
	if len(funcs) != 0 {
		t.Errorf("expected no funcs on genuine failure, got %d", len(funcs))
	}
}

// TestRun_AbsentReasons asserts each absent/partial path carries its distinct,
// actionable reason (opt-in off, not installed, run failed).
func TestRun_AbsentReasons(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name       string
		runner     toolrun.Runner
		enabled    bool
		backend    string
		wantReason string
	}{
		{"lizard opt-in off", absentRunner(), false, BackendLizard, reasonDisabled},
		{"lizard not installed", absentRunner(), true, BackendLizard, reasonLizardMissing},
		{"lizard run failed", failRunner(), true, BackendLizard, reasonRunFailed},
		{"auto opt-in off", absentRunner(), false, BackendAuto, reasonDisabled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, cov, err := Run(context.Background(), tc.runner, root, tc.enabled, tc.backend)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cov.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", cov.Reason, tc.wantReason)
			}
		})
	}
}
