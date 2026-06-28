package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_Check_Root_ScopesLocCount is the key regression guard for the
// sc.Root = root wiring. Before the fix, resolveScanRoot falls back to
// gitRoot (the whole repo) even when --root names a subtree, so loc.Run
// walks the entire tree. After the fix, loc.Run is called with the subtree
// path, and files outside it do not appear in files_seen.
func TestRun_Check_Root_ScopesLocCount(t *testing.T) {
	t.Parallel()

	// Repo has source files in two distinct subtrees.
	repoDir := t.TempDir()
	files := map[string]string{
		markerGoMod:      "module example.com/mono\n\ngo 1.21\n",
		"other/other.go": "package other\n",
		"sub/go.mod":     "module example.com/sub\n\ngo 1.21\n",
		"sub/service.go": "package sub\n",
	}
	for name, content := range files {
		path := filepath.Join(repoDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, repoDir)

	// Config lives in a separate temp dir (external-CI shape) so configDir
	// does not influence which subtree is scanned.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, ".archfit.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(repoDir, "sub")

	// ── sub-only scan ────────────────────────────────────────────────────────
	// --root sub: loc must see exactly 1 file (sub/service.go); other/other.go
	// is outside the subtree and must not appear.
	var buf bytes.Buffer
	code := Run([]string{cmdCheck, flagRoot, subDir, "-c", cfgPath, flagFull, fmtJSON}, &buf)
	if code != 0 && code != 1 {
		t.Fatalf("check --root sub: exit = %d, want 0 or 1\n%s", code, buf.String())
	}

	subSeen, subOK := locFilesSeen(t, buf.Bytes())
	if !subOK {
		t.Fatalf("--root sub: no 'loc' entry in tool_coverage\n%s", buf.String())
	}
	if subSeen != 1 {
		t.Errorf("--root sub: loc files_seen = %d, want 1 (subtree only; other/ must be excluded)", subSeen)
	}

	// ── whole-repo scan ──────────────────────────────────────────────────────
	// --root repo: loc must see both source files (≥2). This is the control
	// arm that proves the sub-only scan is a real scope reduction and not just
	// an instrument error (both files missing).
	buf.Reset()
	code = Run([]string{cmdCheck, flagRoot, repoDir, "-c", cfgPath, flagFull, fmtJSON}, &buf)
	if code != 0 && code != 1 {
		t.Fatalf("check --root repo: exit = %d, want 0 or 1\n%s", code, buf.String())
	}
	repoSeen, repoOK := locFilesSeen(t, buf.Bytes())
	if !repoOK {
		t.Fatalf("--root repo: no 'loc' entry in tool_coverage\n%s", buf.String())
	}
	if repoSeen < 2 {
		t.Errorf("--root repo: loc files_seen = %d, want ≥2 (both subtrees included)", repoSeen)
	}
}

// locFilesSeen parses a diagnostic JSON blob and returns (files_seen, found)
// for the "loc" tool. The found=false guard prevents a missing entry from
// silently reading as 0 and masking an instrument failure.
func locFilesSeen(t *testing.T, data []byte) (int, bool) {
	t.Helper()
	var d struct {
		ToolCoverage []struct {
			Tool      string `json:"tool"`
			FilesSeen int    `json:"files_seen"`
		} `json:"tool_coverage"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatalf("locFilesSeen: invalid JSON: %v", err)
	}
	for _, c := range d.ToolCoverage {
		if c.Tool == toolLoc {
			return c.FilesSeen, true
		}
	}
	return 0, false
}

// TestRun_Check_Root_NonGitFullMode verifies that a plain directory (no git
// repository) analysed with --full produces a scorecard and does NOT exit 3.
// This exercises the non-fatal RepoRoot path added in Task 2.
func TestRun_Check_Root_NonGitFullMode(t *testing.T) {
	t.Parallel()

	// Plain directory — deliberately NOT git-initialised.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".archfit.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := Run([]string{cmdCheck, "-c", cfgPath, flagFull, fmtJSON}, &buf)
	if code == 3 {
		t.Fatalf("non-git full mode: exit = 3 (want 0 or 1 — should produce a scorecard)\n%s", buf.String())
	}

	var d struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
		t.Fatalf("non-git full mode: invalid JSON output: %v\n%s", err, buf.String())
	}
	if d.Verdict == "" {
		t.Error("non-git full mode: empty verdict in JSON output")
	}
}

// TestRun_Check_Root_OutputWarningUsesRoot is a regression guard for
// outputInsideRootWarning using s.Root (the resolved ScanRoot) rather than the
// git toplevel. When --root names the same directory that holds the config,
// rel(ScanRoot, configDir) = "." → no warning. Before the sc.Root=root fix,
// s.Root was the git toplevel, so rel(gitRoot, sub) = "sub" → spurious warning.
//
// Note: on macOS, t.TempDir returns /var/…, but git resolves the symlink to
// /private/var/…. The two paths differ by symlink, so filepath.Rel between
// them returns "../.." rather than "sub", and the warning does not fire even
// before the fix. The test still validates the correct post-fix behaviour on all
// platforms; use TestRun_Check_Root_ScopesLocCount as the discriminating
// before/after guard on macOS.
func TestRun_Check_Root_OutputWarningUsesRoot(t *testing.T) {
	t.Parallel()

	// Repo with a source file at the root; config lives in a subdir that is
	// also the intended scan root.
	repoDir := t.TempDir()
	subDir := filepath.Join(repoDir, "sub")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, markerGoMod), []byte("module example.com/m\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(subDir, ".archfit.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitInitFixtureRepo(t, repoDir)

	var buf bytes.Buffer
	code := Run([]string{cmdCheck, flagRoot, subDir, "-c", cfgPath, flagFull, fmtJSON}, &buf)
	if code == 3 {
		t.Fatalf("unexpected exit 3: %s", buf.String())
	}

	var d struct {
		ConfigWarnings []string `json:"config_warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	for _, w := range d.ConfigWarnings {
		if strings.Contains(w, "output written inside analyzed root") {
			t.Errorf("spurious 'output written inside analyzed root' warning when config is at scan-root dir: %q", w)
		}
	}
}
