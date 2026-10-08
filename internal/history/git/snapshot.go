package git

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// snapshotDate is the fixed author and committer date of an index snapshot. A
// fixed date makes the snapshot commit a pure function of the staged tree, its
// parent, and the author, so the same staged content always gets the same SHA
// and checks out at the same path.
const snapshotDate = "2000-01-01T00:00:00Z"

// fallbackName and fallbackEmail sign a snapshot when git knows no identity.
const (
	fallbackName  = "archfit"
	fallbackEmail = "archfit@localhost"
)

// snapshotIdentity is the author git will record on the real commit, at the
// fixed date. The run's git-author owner fallback reads the snapshot as the
// newest commit, so a placeholder author would be credited with every staged
// file and could take a module's ownership in the hook but not in CI.
func snapshotIdentity(ctx context.Context, runner toolrun.Runner, gitRoot string) []string {
	name, email := fallbackName, fallbackEmail
	if ident, err := runGit(ctx, runner, gitRoot, nil, "var", "GIT_AUTHOR_IDENT"); err == nil {
		// "Name <email> 1700000000 +0000"
		if n, rest, ok := strings.Cut(ident, " <"); ok {
			if e, _, ok := strings.Cut(rest, ">"); ok {
				name, email = n, e
			}
		}
	}
	return []string{
		"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email, "GIT_AUTHOR_DATE=" + snapshotDate,
		"GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email, "GIT_COMMITTER_DATE=" + snapshotDate,
	}
}

// SnapshotIndex records the staged content of the repository at gitRoot as a
// commit object and returns its SHA, so a pre-commit hook can judge what the
// commit will contain rather than the files on disk. indexFile is the index
// git commit hands its hooks in GIT_INDEX_FILE (git commit -a and git commit
// <path> use a temporary one); empty means the repository's own index.
//
// Nothing the user sees changes: git write-tree runs over a private copy of
// the index (it writes its cache-tree back into the index it reads, and git
// commit holds that file locked), the parent is HEAD when HEAD exists, and no
// ref moves. The tree and commit objects stay unreachable until git gc prunes
// them. Unmerged index entries make write-tree fail, and so the snapshot.
func SnapshotIndex(ctx context.Context, runner toolrun.Runner, gitRoot, indexFile string) (string, error) {
	if indexFile == "" {
		path, err := runGit(ctx, runner, gitRoot, nil, "rev-parse", "--git-path", "index")
		if err != nil {
			return "", err
		}
		indexFile = path
	}
	if !filepath.IsAbs(indexFile) {
		indexFile = filepath.Join(gitRoot, indexFile)
	}
	private, err := copyToTemp(indexFile)
	if err != nil {
		return "", fmt.Errorf("copy the index: %w", err)
	}
	defer func() { _ = os.Remove(private) }()

	tree, err := runGit(ctx, runner, gitRoot, []string{"GIT_INDEX_FILE=" + private}, "write-tree")
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", "--no-gpg-sign", tree, "-m", "archfit staged snapshot"}
	if parent, perr := ResolveCommit(ctx, gitRoot, "HEAD", runner); perr == nil {
		args = append(args, "-p", parent)
	}
	return runGit(ctx, runner, gitRoot, snapshotIdentity(ctx, runner, gitRoot), args...)
}

// runGit runs one git command in dir with the git-redirect variables scrubbed
// and extra appended, and returns its trimmed stdout.
func runGit(ctx context.Context, runner toolrun.Runner, dir string, extra []string, args ...string) (string, error) {
	out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: args, Env: append(CleanEnv(), extra...), Timeout: gitTimeout, WorkDir: dir})
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	if out.ExitCode != 0 {
		return "", fmt.Errorf("git %s exited %d: %s", args[0], out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	return strings.TrimSpace(string(out.Stdout)), nil
}

func copyToTemp(src string) (string, error) {
	in, err := os.Open(src) // #nosec G304 -- the repository's own index file
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	out, err := os.CreateTemp("", "archfit-index-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(out.Name())
		return "", err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(out.Name())
		return "", err
	}
	return out.Name(), nil
}
