package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// AddedLines returns the lines the working tree adds against base, per
// repository-relative file: the added lines of modified, staged and renamed
// files, and every line of an untracked file git does not ignore. Renames are
// detected, so a moved file adds only the lines it changed. A deleted file has
// no entry. Paths are relative to the repository root, whatever dir is.
// It runs: git diff -M --unified=0 <base> -- and git ls-files -z --others
// --exclude-standard
func AddedLines(ctx context.Context, dir, base string, runner toolrun.Runner) (evidence.AddedLines, error) {
	if strings.HasPrefix(base, "-") {
		return nil, fmt.Errorf("invalid base %q: a ref may not start with '-'", base)
	}
	root, err := RepoRoot(ctx, dir, runner)
	if err != nil {
		return nil, err
	}
	// quotePath=true quotes every name with a byte git escapes, so a name
	// arrives either plain or as a C string with octal bytes, never as raw
	// bytes inside quotes.
	diff, err := runChecked(ctx, root, runner, "-c", "core.quotePath=true", "diff", "--no-color", "--no-ext-diff",
		"--no-textconv", "-M", "--unified=0", "--src-prefix=a/", "--dst-prefix=b/", base, "--")
	if err != nil {
		return nil, err
	}
	added, err := parseAddedLines(diff)
	if err != nil {
		return nil, err
	}
	others, err := runChecked(ctx, root, runner, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for name := range strings.SplitSeq(string(others), "\x00") {
		if name == "" {
			continue
		}
		n, err := lineCount(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		for line := 1; line <= n; line++ {
			added[name] = append(added[name], line)
		}
	}
	return added, nil
}

// runChecked runs git in dir and returns its stdout untrimmed; a non-zero exit
// is an error.
func runChecked(ctx context.Context, dir string, runner toolrun.Runner, args ...string) ([]byte, error) {
	out, err := run(ctx, dir, runner, args...)
	if err != nil {
		return nil, err
	}
	if out.ExitCode != 0 {
		return nil, fmt.Errorf("git %s exited %d: %s", strings.Join(args, " "), out.ExitCode, strings.TrimSpace(string(out.Stderr)))
	}
	return out.Stdout, nil
}

// parseAddedLines reads the new-side line ranges of a unified diff with no
// context lines. It counts the lines of each hunk, so an added line that reads
// like a header ("+++ x") is never taken for one.
func parseAddedLines(diff []byte) (evidence.AddedLines, error) {
	added := evidence.AddedLines{}
	var file string
	var oldLeft, newLeft int
	for line := range strings.SplitSeq(string(diff), "\n") {
		if oldLeft > 0 || newLeft > 0 {
			switch {
			case strings.HasPrefix(line, "-"):
				oldLeft--
			case strings.HasPrefix(line, "+"):
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
			oldCount, start, count, err := hunkRanges(line)
			if err != nil {
				return nil, err
			}
			oldLeft, newLeft = oldCount, count
			for n := start; n < start+count && file != ""; n++ {
				added[file] = append(added[file], n)
			}
		}
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

// lineCount returns the number of lines git diff would show for an untracked
// file: the lines of a regular file, one for a symlink (its target), none for a
// directory (an untracked nested repository). A symlink is never followed.
func lineCount(path string) (int, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, fmt.Errorf("read untracked file: %w", err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return 1, nil
	case !info.Mode().IsRegular():
		return 0, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // an untracked file git listed under the repository root
	if err != nil {
		return 0, fmt.Errorf("read untracked file: %w", err)
	}
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n, nil
}
