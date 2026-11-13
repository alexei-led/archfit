package requestsend

import (
	"context"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestRoutingWithRealGit(t *testing.T) {
	dir := t.TempDir()
	runner := toolrun.New()
	git := func(args ...string) {
		t.Helper()
		out, err := runner.Run(context.Background(), toolrun.ToolCmd{Name: "git", Args: args, WorkDir: dir})
		if err != nil || out.ExitCode != 0 {
			t.Fatalf("git %v: exit %d, %v, %s", args, out.ExitCode, err, out.Stderr)
		}
	}
	git("init", "-q", "-b", "feature")
	git("config", "user.email", "test@example.invalid")
	git("config", "user.name", "test")
	git("commit", "--allow-empty", "-qm", "base", "--no-gpg-sign")
	git("remote", "add", "origin", "https://github.com/acme/shop.git")
	git("remote", "add", "fork", "git@github.com:worker/shop.git")
	git("update-ref", "refs/remotes/origin/feature", "HEAD")
	git("branch", "--set-upstream-to=origin/feature")
	git("config", "branch.feature.pushRemote", "fork")
	git("config", "push.default", "current")
	s := New(dir, runner)
	repo, head, err := s.repository(t.Context())
	if err != nil || repo != testRepo || head != forkHead {
		t.Fatalf("repo=%q head=%q err=%v", repo, head, err)
	}
	git("checkout", "--detach", "-q")
	repo, head, err = s.repository(t.Context())
	if err != nil || repo != testRepo || head != "" {
		t.Fatalf("detached: repo=%q head=%q err=%v", repo, head, err)
	}
}
