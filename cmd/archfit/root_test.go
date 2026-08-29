package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRun_Check_Root_ScopesLocCount is the key regression guard for the
// sc.Root = root wiring. Before the fix, resolveScanRoot falls back to
// gitRoot (the whole repo) even when --root names a subtree, so loc.Run
// walks the entire tree. After the fix, loc.Run is called with the subtree
// path, and files outside it do not appear in files_seen.
// TestRun_Check_Root_NonGitFullMode verifies that a plain directory (no git
// repository) analysed in full mode produces a scorecard and does NOT exit 3.
// This exercises the non-fatal RepoRoot path added in Task 2.
func TestRun_Check_Root_NonGitFullMode(t *testing.T) {
	t.Parallel()

	// Plain directory — deliberately NOT git-initialised.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".archfit.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := Run([]string{cmdAnalyze, "-c", cfgPath, flagRefresh, fmtJSON}, &buf)
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
// docs/archived/reports/eval-2026-07-02-v1.1.2/corpus-experiments.md): a subtree --root
// whose shared ancestor with the resolved git root is cased differently (e.g.
// git resolves to .../Repo while --root is typed .../repo/services/api) must
// still resolve SubtreePrefix correctly, so CODEOWNERS (which lives at gitRoot
// and is matched via gitRoot-relative paths) keeps working. Before the scope
// fix, owner_source silently collapsed from "codeowners" to "none" with zero
// warning, flipping the coupling_balance verdict.
// TestRun_Check_OwnerDegradation_CodeownersNoMatch_WarnsAndSurfacesSource is
// the disclosure-rule test: a CODEOWNERS file that exists but matches none of
// the configured modules is a suspicious degradation (the same symptom the
// case-variant --root bug produces), and must surface both as a distinct
// owner_source value and a config_warnings entry — not silently collapse to
// "none" with zero explanation.
// TestRun_Check_OwnerDegradation_None_NoWarning verifies that a genuinely
// unattributed repo (no CODEOWNERS, no git-author data) does NOT produce a
// degradation warning — SourceNone is a clean "nothing to attribute" result,
// not a defect, so it must stay quiet.
