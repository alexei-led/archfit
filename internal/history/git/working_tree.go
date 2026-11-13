package git

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// cacheDir is archfit's cache directory at the repository root.
const cacheDir = ".archfit-cache"

// WorkingTreeID returns the git tree id of the working tree as a commit of
// every change would hold it: modified, deleted, renamed and untracked files,
// staged or not, as `git add --all` stages them (ignored files stay out). An
// edit, a new file or a removed file gives another id; the same content gives
// the same id, wherever dir is in the repository. The .archfit-cache directory
// at the root is left out, ignored or not: archfit writes there, and its own
// writes must not change the id.
//
// The staging happens in a private copy of the index, as in AddedLines, so the
// repository's index, HEAD and working tree stay as they are.
// Use an ObjectStore runner for writes outside the repository; keep it alive
// for readers of the returned tree id. The cache is excluded before staging.
func WorkingTreeID(ctx context.Context, dir string, runner toolrun.Runner) (string, error) {
	root, err := RepoRoot(ctx, dir, runner)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "archfit-tree-index-*")
	if err != nil {
		return "", fmt.Errorf("working tree index: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	index, err := stageWorkingTree(ctx, runner, root, tmp)
	if err != nil {
		return "", err
	}
	env := []string{"GIT_INDEX_FILE=" + index}
	out, err := runTree(ctx, root, runner, env, "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
