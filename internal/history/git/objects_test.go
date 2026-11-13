package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestPrivateObjectsLifetime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runner := toolrun.New()
	root := filepath.Join(t.TempDir(), "repo: with spaces")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	command := func(r toolrun.Runner, args ...string) {
		t.Helper()
		out, err := r.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: args, WorkDir: root})
		if err != nil || out.ExitCode != 0 {
			t.Fatalf("git %v: %v, exit %d: %s", args, err, out.ExitCode, out.Stderr)
		}
	}
	command(runner, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, "tracked"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command(runner, "add", ".")
	command(runner, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-qm", "base")
	base, err := HeadRef(ctx, root, runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, cacheDir), 0o700); err != nil {
		t.Fatal(err)
	}
	cacheFile := filepath.Join(root, cacheDir, "private")
	if err := os.WriteFile(cacheFile, []byte("not an object\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewObjectStore(runner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tree, err := WorkingTreeID(ctx, root, store)
	if err != nil {
		t.Fatal(err)
	}
	files, err := TreeFiles(ctx, root, tree, store)
	if err != nil || !slices.Equal(files, []string{"tracked", "untracked"}) {
		t.Fatalf("files = %v, %v", files, err)
	}
	added, err := AddedLines(ctx, root, base, store)
	if err != nil || !slices.Equal(added["untracked"], []int{1}) || len(added) != 1 {
		t.Fatalf("added = %v, %v", added, err)
	}
	var content bytes.Buffer
	out, err := store.Stream(ctx, toolrun.ToolCmd{Name: gitBinary, Args: []string{"show", tree + ":untracked"}, WorkDir: root}, func(r io.Reader) error {
		_, err := io.Copy(&content, r)
		return err
	})
	if err != nil || out.ExitCode != 0 || content.String() != "new\n" {
		t.Fatalf("stream private blob = %q, exit %d, %v", content.String(), out.ExitCode, err)
	}
	out, err = runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: []string{"cat-file", "-e", tree}, WorkDir: root})
	if err != nil || out.ExitCode == 0 {
		t.Fatalf("private tree leaked: exit %d, %v", out.ExitCode, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("object directory remains: %v", err)
	}
}

func TestPrivateObjectsOutsideRepository(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runner, err := NewObjectStore(toolrun.New())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	root := t.TempDir()
	for _, args := range [][]string{{"--version"}, {"init", "-q"}, {"ls-files"}} {
		out, err := runner.Run(ctx, toolrun.ToolCmd{Name: gitBinary, Args: args, WorkDir: root})
		if err != nil || out.ExitCode != 0 {
			t.Fatalf("git %v: exit %d, %v, %s", args, out.ExitCode, err, out.Stderr)
		}
	}
}
