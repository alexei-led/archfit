package git_test

import (
	"context"
	"errors"
	"testing"

	gitpkg "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestTouchCountsPublishesOnlyObservedWindows(t *testing.T) {
	for _, tc := range []struct {
		name    string
		bounded bool
		err     error
		want    gitpkg.ModuleTouchStatus
	}{
		{"initial timeout", false, context.DeadlineExceeded, gitpkg.ModuleTouchStatusTimeout},
		{"initial failure", false, errors.New("git failed"), gitpkg.ModuleTouchStatusError},
		{"empty history", false, nil, gitpkg.ModuleTouchStatusOK},
		{"fallback timeout", true, context.DeadlineExceeded, gitpkg.ModuleTouchStatusTimeout},
		{"fallback failure", true, errors.New("git failed"), gitpkg.ModuleTouchStatusError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			runner := &toolrun.RunnerMock{RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
				calls++
				if tc.bounded && calls == 1 {
					return toolrun.Output{Stdout: []byte("\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\noutside.txt\x00")}, nil
				}
				return toolrun.Output{}, tc.err
			}}
			got := gitpkg.TouchCounts(context.Background(), "/repo", "", func(string) (string, bool) { return "", false }, runner)
			wantCommits, wantWindow := 0, 0
			if tc.bounded {
				wantCommits, wantWindow = 1, 500
			}
			if got.Status != tc.want || got.CommitsScanned != wantCommits || got.CommitWindow != wantWindow || got.FullHistory {
				t.Fatalf("history = %+v, want status %s, commits %d, window %d, full false", got, tc.want, wantCommits, wantWindow)
			}
		})
	}
}
