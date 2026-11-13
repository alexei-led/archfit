package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// AddedLines returns the lines the working tree adds against base, per
// repository-relative file. It compares base with the working tree as a commit
// of every change would hold it: modified, deleted, renamed and untracked
// files, staged or not, as `git add --all` stages them (ignored files stay
// out). Renames are detected across all of them, so a moved file adds only
// the lines it changed, whether it moved with git mv or with mv, and a file
// that is only no longer tracked adds nothing. A deleted file has no entry.
// Every file is read as text, including one git would show as binary. Paths
// are relative to the repository root, whatever dir is.
//
// The staging happens in a private copy of the index, so the repository's
// index, HEAD and working tree stay as they are. Use an ObjectStore runner to
// keep changed blobs outside the repository until the command finishes.
// It runs: git add --all into the private index, then git diff --cached
// --unified=0 -M <base>
func AddedLines(ctx context.Context, dir, base string, runner toolrun.Runner) (evidence.AddedLines, error) {
	if err := checkRef("base", base); err != nil {
		return nil, err
	}
	root, err := RepoRoot(ctx, dir, runner)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "archfit-added-index-*")
	if err != nil {
		return nil, fmt.Errorf("added lines index: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	index, err := stageWorkingTree(ctx, runner, root, tmp)
	if err != nil {
		return nil, err
	}
	// Every option that git config can change in this output is pinned, so
	// the same trees give the same lines on every machine. -l0 lifts
	// diff.renameLimit: past it git stops pairing edited renames and shows the
	// whole file as added. quotePath=true
	// quotes every name with a byte git escapes, so a name arrives either
	// plain or as a C string with octal bytes, never as raw bytes in quotes.
	diff, err := runTree(ctx, root, runner, []string{"GIT_INDEX_FILE=" + index},
		"-c", "core.quotePath=true", "diff", "--cached", "--no-color", "--no-ext-diff", "--no-textconv", "--text",
		"-M", "-l0", "--unified=0", "--inter-hunk-context=0", "--diff-algorithm=myers", "--indent-heuristic",
		"--src-prefix=a/", "--dst-prefix=b/", base, "--")
	if err != nil {
		return nil, err
	}
	return parseAddedLines(diff)
}

// stageWorkingTree copies the repository's index into dir and stages every
// change except the archfit cache in the copy. It returns
// the copy's path. The repository's own index is only read.
func stageWorkingTree(ctx context.Context, runner toolrun.Runner, root, dir string) (string, error) {
	repoIndex, err := runGit(ctx, runner, root, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(repoIndex) {
		repoIndex = filepath.Join(root, repoIndex)
	}
	index := filepath.Join(dir, "index")
	if err := copyIndex(repoIndex, index); err != nil {
		return "", err
	}
	env := []string{"GIT_INDEX_FILE=" + index}
	if _, err := runTree(ctx, root, runner, env, "rm", "-r", "-q", "--cached", "--ignore-unmatch", "--", cacheDir); err != nil {
		return "", err
	}
	_, err = runTree(ctx, root, runner, env, "add", "--all", "--", ".", ":(top,exclude)"+cacheDir)
	return index, err
}

// copyIndex copies the index at from to to with its modification time. Git
// re-reads a file whose index entry is not older than the index itself (a
// "racily clean" entry, written in the same second); a copy with a fresh time
// would make git trust the stat data of such an entry and keep the old blob.
// A missing index is no error: git add starts an empty one.
func copyIndex(from, to string) error {
	info, err := os.Stat(from)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("copy the index: %w", err)
	}
	data, err := os.ReadFile(from) //nolint:gosec // the repository's own index file
	if err != nil {
		return fmt.Errorf("copy the index: %w", err)
	}
	if err := os.WriteFile(to, data, 0o600); err != nil { //nolint:gosec // a file in our own temporary directory
		return fmt.Errorf("copy the index: %w", err)
	}
	if err := os.Chtimes(to, info.ModTime(), info.ModTime()); err != nil {
		return fmt.Errorf("copy the index: %w", err)
	}
	return nil
}

// parseAddedLines reads the added lines of a unified diff. It counts the lines
// of each hunk, so an added line that reads like a header ("+++ x") is never
// taken for one, and it numbers new lines one by one, so a context line is
// never taken for an added one.
func parseAddedLines(diff []byte) (evidence.AddedLines, error) {
	added := evidence.AddedLines{}
	var file string
	var oldLeft, newLeft, next int
	for line := range strings.SplitSeq(string(diff), "\n") {
		if oldLeft > 0 || newLeft > 0 {
			switch {
			case strings.HasPrefix(line, "+"):
				if file != "" {
					added[file] = append(added[file], next)
				}
				next++
				newLeft--
			case strings.HasPrefix(line, "-"):
				oldLeft--
			case strings.HasPrefix(line, " "):
				next++
				oldLeft--
				newLeft--
			case strings.HasPrefix(line, `\`): // "\ No newline at end of file"
			default:
				return nil, fmt.Errorf("git diff: unexpected line in a hunk of %s: %q", file, line)
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "diff --git "):
			file = ""
		case strings.HasPrefix(line, "+++ "):
			name, err := diffPath(strings.TrimPrefix(line, "+++ "))
			if err != nil {
				return nil, err
			}
			file = name
		case strings.HasPrefix(line, "@@ "):
			var err error
			if oldLeft, next, newLeft, err = hunkRanges(line); err != nil {
				return nil, err
			}
		}
	}
	if oldLeft > 0 || newLeft > 0 {
		return nil, fmt.Errorf("git diff: the last hunk of %s is cut short", file)
	}
	return added, nil
}

// diffPath reads the name on a "+++ " line: "/dev/null" for a deleted file,
// else "b/<path>", quoted as a C string when it has special bytes. Git ends a
// name that holds a space with a tab.
func diffPath(field string) (string, error) {
	field = strings.TrimSuffix(field, "\t")
	if field == "/dev/null" {
		return "", nil
	}
	if strings.HasPrefix(field, `"`) {
		unquoted, err := strconv.Unquote(field)
		if err != nil {
			return "", fmt.Errorf("git diff: read the name %s: %w", field, err)
		}
		field = unquoted
	}
	name, ok := strings.CutPrefix(field, "b/")
	if !ok {
		return "", fmt.Errorf("git diff: name %q has no b/ prefix", field)
	}
	return name, nil
}

// hunkRanges reads "@@ -a[,b] +c[,d] @@": the old line count, and the start
// and count of the new lines. A count left out is one.
func hunkRanges(header string) (oldCount, start, count int, err error) {
	fields := strings.Fields(header)
	if len(fields) < 3 || !strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return 0, 0, 0, fmt.Errorf("git diff: bad hunk header %q", header)
	}
	_, oldCount, err = lineRange(fields[1][1:])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("git diff: bad hunk header %q: %w", header, err)
	}
	start, count, err = lineRange(fields[2][1:])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("git diff: bad hunk header %q: %w", header, err)
	}
	return oldCount, start, count, nil
}

func lineRange(s string) (start, count int, err error) {
	first, rest, hasCount := strings.Cut(s, ",")
	if start, err = strconv.Atoi(first); err != nil {
		return 0, 0, err
	}
	if !hasCount {
		return start, 1, nil
	}
	count, err = strconv.Atoi(rest)
	return start, count, err
}
