package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	gitpkg "github.com/alexei-led/archfit/internal/history/git"
	"github.com/alexei-led/archfit/internal/toolrun"
)

func TestTouchCountsRealEmptyRepository(t *testing.T) {
	for _, tc := range []struct {
		name       string
		initialize bool
		corrupt    bool
		want       gitpkg.ModuleTouchStatus
	}{
		{"unborn branch", true, false, gitpkg.ModuleTouchStatusOK},
		{"not a repository", false, false, gitpkg.ModuleTouchStatusError},
		{"broken branch object", true, true, gitpkg.ModuleTouchStatusError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			runner := toolrun.New()
			ctx := context.Background()
			if tc.initialize {
				out, err := runner.Run(ctx, toolrun.ToolCmd{Name: "git", Args: []string{"init", "--initial-branch=main"}, WorkDir: root})
				if err != nil || out.ExitCode != 0 {
					t.Fatalf("git init: %v, %+v", err, out)
				}
			}
			if tc.corrupt {
				if err := os.WriteFile(filepath.Join(root, ".git", "refs", "heads", "main"), []byte("0000000000000000000000000000000000000001\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got := gitpkg.TouchCounts(ctx, root, "", func(string) (string, bool) { return "", false }, runner)
			if got.Status != tc.want || got.CommitsScanned != 0 || len(got.TouchedByModule) != 0 {
				t.Fatalf("history = %+v, want %s with zero commits", got, tc.want)
			}
		})
	}
}
