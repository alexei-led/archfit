package git

import (
	"bytes"
	"context"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// TreeFiles returns every file of the git tree id tree, repository-relative
// and slash-separated, in git's order. A tree id from WorkingTreeID lists the
// working tree as a commit of every change would hold it.
// It runs: git ls-tree -r -z --name-only <tree>
func TreeFiles(ctx context.Context, dir, tree string, runner toolrun.Runner) ([]string, error) {
	if err := checkRef("tree", tree); err != nil {
		return nil, err
	}
	out, err := runTree(ctx, dir, runner, nil, "ls-tree", "-r", "-z", "--name-only", tree)
	if err != nil {
		return nil, err
	}
	var files []string
	for name := range bytes.SplitSeq(out, []byte{0}) {
		if len(name) > 0 {
			files = append(files, string(name))
		}
	}
	return files, nil
}
