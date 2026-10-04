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

// TestRun_Analyze_Base_InvocationPathSpelling runs `analyze --base HEAD` on an
// unchanged tree from the canonical directory, a symlink to it, and (on a
// case-insensitive filesystem) a case variant of it. The shell's spelling of
// the working directory must not reach model_hash: before the fix, `go list`
// echoed the inherited PWD, the Go main package fell outside the canonical scan
// root, the head side lost its deploy unit, and the comparison read
// non_comparable on model_hash.
func TestRun_Analyze_Base_InvocationPathSpelling(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(parent, "repo")
	for name, content := range map[string]string{
		markerGoMod:          goModStub,
		"cmd/server/main.go": goMainSrc,
		"pkg/app/app.go":     "package app\n\nfunc Run() {}\n",
		defaultConfigPath: "version: 2\nmodules:\n" +
			"  server:\n    paths: [\"cmd/server/**\"]\n    owner: team-a\n" +
			"  app:\n    paths: [\"pkg/app/**\"]\n    owner: team-a\n",
	} {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitInitFixtureRepo(t, repo)
	gitCommitAll(t, repo, "initial commit")
	symlink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, symlink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	type run struct {
		Comparison struct {
			Status    string   `json:"status"`
			Reasons   []string `json:"reasons"`
			ModelHash string   `json:"model_hash"`
		} `json:"comparison"`
	}
	analyzeFrom := func(t *testing.T, dir string) run {
		t.Helper()
		t.Chdir(dir)
		code, stdout, stderr := runArchfit(t, cmdAnalyze, flagBase, "HEAD", fmtJSON, flagRefresh, "-c", defaultConfigPath)
		if code != 0 {
			t.Fatalf("analyze --base HEAD from %s: exit = %d\nstderr:\n%s", dir, code, stderr)
		}
		var got run
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("invalid JSON from %s: %v\n%s", dir, err, stdout)
		}
		if got.Comparison.Status != "comparable" {
			t.Errorf("comparison from %s = %q %v, want comparable on an unchanged tree", dir, got.Comparison.Status, got.Comparison.Reasons)
		}
		return got
	}

	canonical := analyzeFrom(t, repo)
	for name, alias := range map[string]string{
		"symlink":      symlink,
		"case variant": filepath.Join(parent, "REPO"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(alias); err != nil {
				t.Skipf("%s does not resolve here (case-sensitive filesystem): %v", alias, err)
			}
			if got := analyzeFrom(t, alias); got.Comparison.ModelHash != canonical.Comparison.ModelHash {
				t.Errorf("model_hash from %s = %s, want %s (the canonical run's)", alias, got.Comparison.ModelHash, canonical.Comparison.ModelHash)
			}
		})
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
