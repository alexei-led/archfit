package git_test

import (
	"context"
	"errors"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestTouchCountsDistinguishesFailureAndNoMatches(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		exit   int
		status string
		calls  int
	}{
		{"runner failure", errors.New("git failed"), 0, "error", 1},
		{"nonzero exit", nil, 128, "error", 2},
		{"timeout", context.DeadlineExceeded, 0, "timeout", 1},
		{"successful no matches", nil, 0, "ok", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
				calls++
				return toolrun.Output{Stdout: []byte("\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\noutside.txt\x00"), ExitCode: tc.exit}, tc.err
			}}
			got := gitpkg.TouchCounts(context.Background(), "/repo", "", func(string) (string, bool) { return "", false }, runner)
			if string(got.Status) != tc.status || calls != tc.calls {
				t.Fatalf("result = %+v, calls = %d, want %s/%d", got, calls, tc.status, tc.calls)
			}
			if tc.status == "ok" && (got.CommitsScanned != 1 || !got.FullHistory) {
				t.Fatalf("lost successful history evidence: %+v", got)
			}
		})
	}
}
