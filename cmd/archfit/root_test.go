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
	code := Run([]string{cmdAnalyze, flagRoot, subDir, "-c", cfgPath, flagFull, fmtJSON}, &buf)
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
	code = Run([]string{cmdAnalyze, flagRoot, repoDir, "-c", cfgPath, flagFull, fmtJSON}, &buf)
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
	code := Run([]string{cmdAnalyze, "-c", cfgPath, flagFull, fmtJSON}, &buf)
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
	code := Run([]string{cmdAnalyze, flagRoot, subDir, "-c", cfgPath, flagFull, fmtJSON}, &buf)
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

// isCaseInsensitiveFS probes whether dir's filesystem treats "a" and "A" as
// the same file — the precondition for the case-variant --root bug. Probe,
// don't assume by GOOS: Linux can mount case-insensitive volumes and macOS
// can be case-sensitive.
func isCaseInsensitiveFS(t *testing.T, dir string) bool {
	t.Helper()
	lower := filepath.Join(dir, "archfit-case-probe")
	if err := os.WriteFile(lower, []byte("x"), 0o600); err != nil {
		t.Fatalf("case probe write: %v", err)
	}
	upper := filepath.Join(dir, "ARCHFIT-CASE-PROBE")
	_, err := os.Stat(upper)
	return err == nil
}

// TestRun_Check_Root_CaseVariantSubtree_OwnerSourceCodeowners is the Wave 2
// Task 4 case-bug repro (the omni corpus finding — see
// reports/eval-2026-07-02-v1.1.2/corpus-experiments.md): a subtree --root
// whose shared ancestor with the resolved git root is cased differently (e.g.
// git resolves to .../Repo while --root is typed .../repo/services/api) must
// still resolve SubtreePrefix correctly, so CODEOWNERS (which lives at gitRoot
// and is matched via gitRoot-relative paths) keeps working. Before the scope
// fix, owner_source silently collapsed from "codeowners" to "none" with zero
// warning, flipping the coupling_balance verdict.
func TestRun_Check_Root_CaseVariantSubtree_OwnerSourceCodeowners(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	if !isCaseInsensitiveFS(t, parent) {
		t.Skip("case-sensitive filesystem — case-variant --root cannot alias the git root here")
	}

	repoDir := filepath.Join(parent, "Repo")
	subDir := filepath.Join(repoDir, "services", "api")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "handler.go"), []byte("package api\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".github"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".github", "CODEOWNERS"), []byte("/services/api @api-team\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitInitFixtureRepo(t, repoDir)

	// Module paths are scanRoot-relative (--root IS the services/api subtree,
	// so the scanned files' module-relative path is just "handler.go").
	cfgPath := filepath.Join(repoDir, ".archfit.yaml")
	cfg := "version: 1\nmodules:\n  api:\n    paths: [\"**\"]\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	// Case-variant --root: lowercase "repo" aliases the real "Repo" dir on a
	// case-insensitive filesystem; "services/api" keeps its real case, matching
	// the corpus repro shape (only the gitRoot-shared ancestor is wrong).
	caseVariantRoot := filepath.Join(parent, "repo", "services", "api")

	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, flagRoot, caseVariantRoot, "-c", cfgPath, flagFull, fmtJSON}, &buf)
	if code == 3 {
		t.Fatalf("unexpected exit 3: %s", buf.String())
	}

	var d struct {
		OwnerSource string `json:"owner_source"`
	}
	if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if d.OwnerSource != "codeowners" {
		t.Errorf("owner_source = %q, want %q (case-variant --root must not corrupt CODEOWNERS subtree resolution)", d.OwnerSource, "codeowners")
	}
}

// TestRun_Check_OwnerDegradation_CodeownersNoMatch_WarnsAndSurfacesSource is
// the disclosure-rule test: a CODEOWNERS file that exists but matches none of
// the configured modules is a suspicious degradation (the same symptom the
// case-variant --root bug produces), and must surface both as a distinct
// owner_source value and a config_warnings entry — not silently collapse to
// "none" with zero explanation.
func TestRun_Check_OwnerDegradation_CodeownersNoMatch_WarnsAndSurfacesSource(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "src", "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".github"), 0o750); err != nil {
		t.Fatal(err)
	}
	// CODEOWNERS exists but its only rule matches nothing under the configured module.
	if err := os.WriteFile(filepath.Join(repoDir, ".github", "CODEOWNERS"), []byte("docs/ @docs-team\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitInitFixtureRepo(t, repoDir)

	cfgPath := filepath.Join(repoDir, ".archfit.yaml")
	cfg := "version: 1\nmodules:\n  app:\n    paths: [\"src/**\"]\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, "-c", cfgPath, flagFull, fmtJSON}, &buf)
	if code == 3 {
		t.Fatalf("unexpected exit 3: %s", buf.String())
	}

	var d struct {
		OwnerSource    string   `json:"owner_source"`
		ConfigWarnings []string `json:"config_warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if d.OwnerSource != "codeowners_no_match" {
		t.Errorf("owner_source = %q, want %q", d.OwnerSource, "codeowners_no_match")
	}
	found := false
	for _, w := range d.ConfigWarnings {
		if strings.Contains(w, "CODEOWNERS") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a config_warnings entry naming CODEOWNERS as the degradation cause, got: %v", d.ConfigWarnings)
	}
}

// TestRun_Check_OwnerDegradation_None_NoWarning verifies that a genuinely
// unattributed repo (no CODEOWNERS, no git-author data) does NOT produce a
// degradation warning — SourceNone is a clean "nothing to attribute" result,
// not a defect, so it must stay quiet.
func TestRun_Check_OwnerDegradation_None_NoWarning(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "src", "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitInitFixtureRepo(t, repoDir)

	cfgPath := filepath.Join(repoDir, ".archfit.yaml")
	cfg := "version: 1\nmodules:\n  app:\n    paths: [\"src/**\"]\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, "-c", cfgPath, flagFull, fmtJSON}, &buf)
	if code == 3 {
		t.Fatalf("unexpected exit 3: %s", buf.String())
	}

	var d struct {
		OwnerSource    string   `json:"owner_source"`
		ConfigWarnings []string `json:"config_warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &d); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if d.OwnerSource != "none" {
		t.Errorf("owner_source = %q, want %q", d.OwnerSource, "none")
	}
	for _, w := range d.ConfigWarnings {
		if strings.Contains(w, "owner resolution") {
			t.Errorf("unexpected owner-degradation warning for a clean SourceNone result: %q", w)
		}
	}
}
