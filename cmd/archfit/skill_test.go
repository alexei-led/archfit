package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkillInstall pins that install writes exactly the repository's
// skills/archfit tree, rewrites nothing on a second run, and refuses to
// overwrite a local change without --force, and that the skill stays small.
func TestSkillInstall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	install := func(args ...string) (int, string) {
		code, stdout, stderr := runArchfit(t, append([]string{"skill", "install", "--dir", dir}, args...)...)
		return code, stdout + stderr
	}
	if code, out := install(); code != 0 || !strings.Contains(out, "files written") {
		t.Fatalf("install: exit %d\n%s", code, out)
	}
	source := filepath.Join("..", "..", "skills", skillName)
	// The skill is the worker loop an agent loads every session: its body
	// stays under 1 KB, owner material lives in docs/agents.md.
	if raw, err := os.ReadFile(filepath.Join(source, "SKILL.md")); err != nil { //nolint:gosec // repo path
		t.Fatal(err)
	} else if parts := strings.SplitN(string(raw), "---\n", 3); len(parts) != 3 || len(parts[2]) > 1024 {
		t.Errorf("SKILL.md body is %d bytes, budget 1024", len(parts[len(parts)-1]))
	}
	count := 0
	err := filepath.WalkDir(source, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(source, p)
		want, _ := os.ReadFile(p) //nolint:gosec // repo walk
		got, err := os.ReadFile(filepath.Join(dir, skillName, rel))
		if err != nil || string(got) != string(want) {
			t.Errorf("%s: installed copy differs from the repository (%v)", rel, err)
		}
		count++
		return nil
	})
	if err != nil || count == 0 {
		t.Fatalf("walk %s: %v (%d files)", source, err, count)
	}
	if code, out := install(); code != 0 || !strings.Contains(out, ": 0 of ") {
		t.Errorf("second install rewrote files: exit %d\n%s", code, out)
	}
	skillFile := filepath.Join(dir, skillName, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("local edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out := install(); code != 3 || !strings.Contains(out, "SKILL.md differ") || !strings.Contains(out, "--force") {
		t.Errorf("install over a local change: exit %d, want 3\n%s", code, out)
	}
	if code, _ := install("--force"); code != 0 {
		t.Errorf("install --force: exit %d", code)
	}
	if got, _ := os.ReadFile(skillFile); string(got) == "local edit\n" { //nolint:gosec // test temp path
		t.Error("--force did not overwrite the local change")
	}
}
