package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	cmdDoctor        = "doctor"
	cmdCheck         = "check"
	cmdAnalyze       = "analyze"
	flagVersion      = "--version"
	flagRoot         = "--root"
	flagJSON         = "--json"
	filePkgAA        = "pkg/a/a.go"
	goModStub        = "module example.com/test\n\ngo 1.21\n"
	minimalValidYAML = "version: 3\nowners: [owner]\nmodules: {all: {paths: ['**']}}\n"
)

func gitInitFixtureRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	cmd.Env = scrubGitFixtureEnv(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init failed (git unavailable?): %v\n%s", err, out)
	}
}

func scrubGitFixtureEnv(env []string) []string {
	blocked := map[string]bool{
		"GIT_DIR":        true,
		"GIT_WORK_TREE":  true,
		"GIT_COMMON_DIR": true,
		"GIT_PREFIX":     true,
	}

	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found && blocked[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func TestRun_Version(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := Run([]string{flagVersion}, &buf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	out := buf.String()
	if !strings.Contains(out, "archfit version") {
		t.Errorf("expected version output, got: %q", out)
	}
}

func TestRun_Doctor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	code := Run([]string{cmdDoctor}, &buf)
	// Exit code varies with which tools are installed, but doctor always renders
	// a report and never crashes (a panic would surface as a non-0/1 code via the
	// recover in Run, or a t-level failure).
	if code != 0 && code != 1 {
		t.Errorf("doctor exit = %d, want 0 (all present) or 1 (some missing)", code)
	}
	if strings.TrimSpace(buf.String()) == "" {
		t.Errorf("doctor produced no output; want a tool report")
	}
}

func runArchfit(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := RunWithStderr(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func scopeRepo(t *testing.T, cfg, a string) string {
	t.Helper()
	dir := hookRepo(t, cfg, a)
	gitFixture(t, dir, "branch", "-M", branchMain)
	gitFixture(t, dir, "checkout", "-q", "-b", "feature")
	return dir
}

func commitFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		writeFixtureFile(t, dir, name, content)
	}
	gitCommitFixture(t, dir)
}

func implSource() string { return "package impl\n\nfunc " + "Secret() string { return \"s\" }\n" }

const hookRuleSection = `rules:
  - id: no_b_impl
    deny: {from: pkg/a/**, to: pkg/b/impl/**}
    why: Keep implementation private.
    instead: Use the public API.
`

const lintCleanModules = `version: 3
owners: [owner]
modules:
  a:
    paths: ["pkg/a/**"]
    subdomain: core
    volatility: low
  b:
    paths: ["pkg/b/**"]
    subdomain: core
    volatility: low
`

const branchMain = "main"
