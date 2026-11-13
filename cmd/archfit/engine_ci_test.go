package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestCheckWithReadOnlyGit(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, fixtureFiles(), nil)
	const untracked = "pkg/chaos/netem/new.go"
	writeFiles(t, dir, map[string]string{
		untracked:              chaosImportsRuntime,
		".archfit-cache/probe": "not a source file",
	})
	objects := filepath.Join(dir, ".git", "objects")
	before := objectFiles(t, objects)
	readOnlyGit(t, filepath.Join(dir, ".git"))
	for range 2 {
		got := runCLI(t, fixtureDate, cmdCheck, "-C", dir, flagFormat, formatJSON)
		var res result.Result
		if err := json.Unmarshal([]byte(got.stdout), &res); err != nil {
			t.Fatalf("check exit %d: %v; stdout %s; stderr %s", got.code, err, got.stdout, got.stderr)
		}
		found := false
		for _, e := range res.Errors {
			found = found || e.Path == untracked && e.Rule == "no-chaos-to-runtime" && e.FromChange
		}
		if got.code != 1 || res.Status != result.StatusFail || !found {
			t.Fatalf("check exit %d, result %+v; stderr %s", got.code, res, got.stderr)
		}
	}
	if after := objectFiles(t, objects); !reflect.DeepEqual(after, before) {
		t.Errorf("repository object files changed: before %v, after %v", before, after)
	}
}

func TestCheckCleanReadOnlyGitWithTouchedFiles(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, fixtureFiles(), nil)
	touched := time.Now().Add(time.Hour)
	for file := range fixtureFiles() {
		if err := os.Chtimes(filepath.Join(dir, file), touched, touched); err != nil {
			t.Fatal(err)
		}
	}
	gitDir := filepath.Join(dir, ".git")
	before := objectFiles(t, gitDir)
	repo, err := os.OpenRoot(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repo.Close() }()
	index, err := repo.ReadFile("index")
	if err != nil {
		t.Fatal(err)
	}
	readOnlyGit(t, gitDir)
	got := runCLI(t, fixtureDate, cmdCheck, "-C", dir, flagFormat, formatJSON)
	var res result.Result
	if err := json.Unmarshal([]byte(got.stdout), &res); err != nil {
		t.Fatalf("check exit %d: %v; stdout %s; stderr %s", got.code, err, got.stdout, got.stderr)
	}
	if got.code != 0 || res.Status != result.StatusPass {
		t.Fatalf("check exit %d, result %+v; stderr %s", got.code, res, got.stderr)
	}
	if after := objectFiles(t, gitDir); !reflect.DeepEqual(after, before) {
		t.Errorf("git files changed: before %v, after %v", before, after)
	}
	after, err := repo.ReadFile("index")
	if err != nil || !bytes.Equal(after, index) {
		t.Errorf("repository index changed: %v", err)
	}
}

func TestResultEngineIdentity(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })
	dir := fixtureRepo(t, fixtureFiles(), nil)
	for _, tc := range []struct{ build, want string }{
		{"v4.0.0-rc.1", "4.0.0-rc.1"},
		{"4.0.0", "4.0.0"},
		{"dev", "dev"},
		{"vv4.0.0", "v4.0.0"},
	} {
		t.Run(tc.build, func(t *testing.T) {
			version = tc.build
			for _, args := range [][]string{
				{cmdCheck, "-C", dir, flagFormat, formatJSON},
				{cmdCan, pkgTC, pkgUtil, "-C", dir, flagFormat, formatJSON},
			} {
				got := runCLI(t, fixtureDate, args...)
				var stamp struct{ Engine string }
				if err := json.Unmarshal([]byte(got.stdout), &stamp); err != nil {
					t.Fatalf("%v: exit %d: %v; %s", args, got.code, err, got.stderr)
				}
				if stamp.Engine != tc.want {
					t.Errorf("%s engine = %q, want %q", args[0], stamp.Engine, tc.want)
				}
			}
			in := identityInputs(context.Background(), toolrun.New(), filepath.Join(dir, defaultConfigPath), dir, dir, "")
			if in.EngineVersion != tc.want {
				t.Errorf("report engine = %q, want %q", in.EngineVersion, tc.want)
			}
			display := runCLI(t, fixtureDate, "--version")
			if display.code != 0 || !strings.Contains(display.stdout, "archfit version "+tc.build+" ") {
				t.Errorf("--version: %+v", display)
			}
		})
	}
}

// readOnlyGit restores permissions before TempDir cleanup, including on failure.
func readOnlyGit(t *testing.T, root string) {
	t.Helper()
	modes := map[string]fs.FileMode{}
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		modes[path] = info.Mode().Perm()
		return nil
	})
	t.Cleanup(func() {
		for path, mode := range modes {
			if err := os.Chmod(path, mode); err != nil {
				t.Error(err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for path, mode := range modes {
		if err := os.Chmod(path, mode&^0o222); err != nil {
			t.Fatal(err)
		}
	}
}

func objectFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && !strings.HasSuffix(entry.Name(), ".lock") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestHooksWithReadOnlyObjects(t *testing.T) {
	for _, event := range []string{hookEventPreCommit, hookEventPostToolUse} {
		t.Run(event, func(t *testing.T) {
			dir := hookFixture(t, nil)
			writeFiles(t, dir, map[string]string{chaosFile: chaosImportsRuntime})
			if event == hookEventPreCommit {
				gitFixture(t, dir, "add", chaosFile)
			}
			t.Chdir(dir)
			writeFiles(t, dir, map[string]string{
				".archfit-cache/probe": "cache is not staged",
				"untracked.txt":        "not committed",
			})
			objects := filepath.Join(dir, ".git", "objects")
			before := objectFiles(t, objects)
			readOnlyGit(t, objects)
			input, err := json.Marshal(hookEvent{Cwd: dir, SessionID: event})
			if err != nil {
				t.Fatal(err)
			}
			code, stdout, stderr := runHook(t, string(input), "run", event)
			if event == hookEventPreCommit {
				if code != 1 || !strings.Contains(stderr, "no-chaos-to-runtime") {
					t.Fatalf("pre-commit: exit %d, stdout %s, stderr %s", code, stdout, stderr)
				}
			} else if code != 2 || !strings.Contains(stderr, "no-chaos-to-runtime") {
				t.Fatalf("post-tool-use: exit %d, stdout %s, stderr %s", code, stdout, stderr)
			}
			if after := objectFiles(t, objects); !reflect.DeepEqual(after, before) {
				t.Errorf("repository object files changed: before %v, after %v", before, after)
			}
		})
	}
}
