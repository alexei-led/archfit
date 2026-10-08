package git_test

import (
	"context"
	"strings"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestTouchCounts_CountsOneCommitPerModule(t *testing.T) {
	t.Parallel()

	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if len(cmd.Args) < 3 || cmd.Args[0] != "log" || cmd.Args[1] != "--format=%x00%H" || cmd.Args[2] != "--name-only" {
				t.Fatalf("unexpected args: %v", cmd.Args)
			}
			return toolrun.Output{Stdout: []byte(
				"\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\n" +
					"internal/a/file1.go\x00" +
					"internal/a/file2.go\x00" +
					"internal/b/file.go\x00" +
					"\x00" +
					"\x00bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\x00\n" +
					"internal/a/file3.go\x00" +
					"\x00"), ExitCode: 0}, nil
		},
	}
	moduleFor := func(path string) (string, bool) {
		switch {
		case strings.HasPrefix(path, "internal/a/"):
			return "a", true
		case strings.HasPrefix(path, "internal/b/"):
			return "b", true
		default:
			return "", false
		}
	}

	got := gitpkg.TouchCounts(context.Background(), "/repo", "", moduleFor, mock)
	if got.Status != gitpkg.ModuleTouchStatusOK {
		t.Fatalf("status = %q, want ok", got.Status)
	}
	if got.CommitWindow != 500 || got.FullHistory {
		t.Fatalf("window/full = %d/%t, want 500/false", got.CommitWindow, got.FullHistory)
	}
	if got.CommitsScanned != 2 {
		t.Fatalf("commits_scanned = %d, want 2", got.CommitsScanned)
	}
	if got.TouchedByModule["a"] != 2 {
		t.Fatalf("touches[a] = %d, want 2", got.TouchedByModule["a"])
	}
	if got.TouchedByModule["b"] != 1 {
		t.Fatalf("touches[b] = %d, want 1", got.TouchedByModule["b"])
	}
	ranked := got.RankedModules()
	if len(ranked) != 2 || ranked[0] != "a" || ranked[1] != "b" {
		t.Fatalf("ranked = %v, want [a b]", ranked)
	}
}

func TestTouchCounts_SubtreeRebase(t *testing.T) {
	t.Parallel()

	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(
				"\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\n" +
					"services/api/handler.go\x00" +
					"other/main.go\x00"), ExitCode: 0}, nil
		},
	}
	moduleFor := func(path string) (string, bool) {
		if path == "handler.go" {
			return "api", true
		}
		return "", false
	}

	got := gitpkg.TouchCounts(context.Background(), "/repo", "services/api", moduleFor, mock)
	if got.TouchedByModule["api"] != 1 {
		t.Fatalf("touches = %+v, want api=1", got.TouchedByModule)
	}
}

func TestTouchCounts_FullHistoryFallback(t *testing.T) {
	t.Parallel()

	calls := 0
	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			calls++
			switch calls {
			case 1:
				if len(cmd.Args) < 6 || cmd.Args[4] != "-n" || cmd.Args[5] != "500" {
					t.Fatalf("bounded args = %v, want -n 500", cmd.Args)
				}
				return toolrun.Output{Stdout: []byte("\x00aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00\noutside.txt\x00"), ExitCode: 0}, nil
			case 2:
				return toolrun.Output{Stdout: []byte("\x00bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\x00\ninternal/a/file.go\x00"), ExitCode: 0}, nil
			default:
				t.Fatalf("unexpected extra call %d", calls)
				return toolrun.Output{}, nil
			}
		},
	}
	moduleFor := func(path string) (string, bool) {
		if path == "internal/a/file.go" {
			return "a", true
		}
		return "", false
	}

	got := gitpkg.TouchCounts(context.Background(), "/repo", "", moduleFor, mock)
	if got.Status != gitpkg.ModuleTouchStatusOK || !got.FullHistory {
		t.Fatalf("status/full = %q/%t, want ok/true", got.Status, got.FullHistory)
	}
	if got.TouchedByModule["a"] != 1 {
		t.Fatalf("touches = %+v, want a=1", got.TouchedByModule)
	}
}

func TestTouchCounts_Timeout(t *testing.T) {
	t.Parallel()

	mock := &toolrun.RunnerMock{
		RunFunc: func(_ context.Context, _ toolrun.ToolCmd) (toolrun.Output, error) {
			return toolrun.Output{}, context.DeadlineExceeded
		},
	}
	moduleFor := func(_ string) (string, bool) { return "", false }

	got := gitpkg.TouchCounts(context.Background(), "/repo", "", moduleFor, mock)
	if got.Status != gitpkg.ModuleTouchStatusTimeout {
		t.Fatalf("status = %q, want timeout", got.Status)
	}
}
