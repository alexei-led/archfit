package git_test

import (
	"context"
	"testing"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestOriginURL(t *testing.T) {
	requireGit(t)
	t.Parallel()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")

	if url, ok, err := gitpkg.OriginURL(context.Background(), dir, toolrun.New()); err != nil || ok || url != "" {
		t.Errorf("without origin: OriginURL = %q, %v, %v; want no URL and no error", url, ok, err)
	}

	const remote = "git@github.com:acme/shop.git"
	gitIn(t, dir, "remote", "add", "origin", remote)
	gitIn(t, dir, "remote", "add", "upstream", "https://github.com/other/shop.git")
	if url, ok, err := gitpkg.OriginURL(context.Background(), dir, toolrun.New()); err != nil || !ok || url != remote {
		t.Errorf("OriginURL = %q, %v, %v; want %q", url, ok, err, remote)
	}
}
