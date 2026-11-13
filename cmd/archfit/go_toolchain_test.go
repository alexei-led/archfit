package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCheckNeverRunsTypedGoLoader(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for _, installed := range []bool{true, false} {
		name := "without go"
		if installed {
			name = "with go trap"
		}
		t.Run(name, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
				t.Fatal(err)
			}
			called := filepath.Join(t.TempDir(), "go-called")
			t.Setenv("ARCHFIT_TEST_GO_TRAP", called)
			if installed {
				trap := "#!/bin/sh\nprintf 'typed loader reached\\n' >> \"$ARCHFIT_TEST_GO_TRAP\"\nexit 91\n"
				if err := os.WriteFile(filepath.Join(bin, "go"), []byte(trap), 0o700); err != nil { //nolint:gosec // the test's executable trap
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("CGO_ENABLED", "0")
			t.Setenv("GOENV", "off")
			for _, tc := range []struct {
				name   string
				head   map[string]string
				golden string
				code   int
			}{
				{name: "clean", head: map[string]string{tcNetem: fixtureFiles()[tcNetem] + "\n// Unit is ms.\n"}, golden: "pass", code: 0},
				{name: "forbidden import", head: map[string]string{chaosFile: chaosImportsRuntime}, golden: "deny", code: 1},
			} {
				t.Run(tc.name, func(t *testing.T) {
					dir := fixtureRepo(t, fixtureFiles(), tc.head)
					got := runCLI(t, fixtureDate, cmdCheck, "-C", dir)
					if got.code != tc.code || got.stdout != readDesignExample(t, tc.golden) {
						t.Errorf("exit = %d, want %d; stdout:\n%s\nstderr:\n%s", got.code, tc.code, got.stdout, got.stderr)
					}
					if data, err := os.ReadFile(called); !os.IsNotExist(err) { //nolint:gosec // the trap marker in t.TempDir
						t.Errorf("check reached Go: %q (read error %v)", data, err)
					}
				})
			}
		})
	}
}
