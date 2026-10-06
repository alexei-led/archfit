package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	agentsMDGoldenDir = "testdata/agents-md"
	cmdAgentsMD       = "agents-md"
	repoConfig        = "../../.archfit.yaml"
)

// TestAgentsMDGoldens pins the rendered block for every example config.
// The engine's own block is pinned by AGENTS.md itself
// (TestAgentsMDRepositoryBlockIsCurrent). Regenerate deliberately with
// ARCHFIT_UPDATE_AGENTS_MD=1.
func TestAgentsMDGoldens(t *testing.T) {
	t.Parallel()
	configs, err := filepath.Glob("../../examples/*.archfit.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) == 0 {
		t.Fatal("no example config found: the golden test is vacuous")
	}
	for _, cfg := range configs {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(cfg), "."), ".yaml") + ".md"
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runArchfit(t, cmdAgentsMD, "-c", cfg)
			if code != 0 {
				t.Fatalf("exit = %d\n%s", code, stderr)
			}
			golden := filepath.Join(agentsMDGoldenDir, name)
			if os.Getenv("ARCHFIT_UPDATE_AGENTS_MD") != "" {
				if err := os.WriteFile(golden, []byte(stdout), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden) //nolint:gosec // fixed testdata path
			if err != nil {
				t.Fatalf("read %s (regenerate with ARCHFIT_UPDATE_AGENTS_MD=1): %v", golden, err)
			}
			if stdout != string(want) {
				t.Errorf("block differs from %s:\n%s", golden, firstDiffLine(string(want), stdout))
			}
		})
	}
}

// TestAgentsMDWriteAndCheck pins the file contract: text outside the markers
// stays byte-identical, a second write changes nothing, and --check exits 1 on
// a missing or stale block and 0 on a current one.
func TestAgentsMDWriteAndCheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "AGENTS.md")
	before := "# Agents\n\nHand-written rules stay.\n"
	if err := os.WriteFile(file, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := "../../examples/ddd.archfit.yaml"
	run := func(args ...string) int {
		code, _, stderr := runArchfit(t, append([]string{cmdAgentsMD, "-c", cfg, "--file", file}, args...)...)
		if code == 3 {
			t.Fatalf("agents-md %v: %s", args, stderr)
		}
		return code
	}

	if code := run("--check"); code != 1 {
		t.Errorf("--check on a file without the block: exit %d, want 1", code)
	}
	if code := run("--write"); code != 0 {
		t.Fatalf("--write: exit %d", code)
	}
	first, err := os.ReadFile(file) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(first, []byte(before)) || !bytes.Contains(first, []byte(agentsMDStart)) {
		t.Fatalf("write did not append the block after the hand-written text:\n%s", first)
	}
	if code := run("--write"); code != 0 {
		t.Fatalf("second --write: exit %d", code)
	}
	second, err := os.ReadFile(file) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("a second write changed the file")
	}
	if code := run("--check"); code != 0 {
		t.Errorf("--check on a current block: exit %d, want 0", code)
	}

	after := "\n## Footer\n\nAlso hand-written.\n"
	stale := strings.Replace(string(second), "| `catalog` |", "| `catalog_old` |", 1) + after
	if err := os.WriteFile(file, []byte(stale), 0o600); err != nil { //nolint:gosec // test temp path
		t.Fatal(err)
	}
	if code := run("--check"); code != 1 {
		t.Errorf("--check on a stale block: exit %d, want 1", code)
	}
	if code := run("--write"); code != 0 {
		t.Fatalf("--write over a stale block: exit %d", code)
	}
	fixed, err := os.ReadFile(file) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	if string(fixed) != string(second)+after {
		t.Errorf("rewrite did not keep the text around the block:\n%s", fixed)
	}
}

func TestAgentsMDUsageErrors(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(file, []byte(agentsMDEnd+"\n"+agentsMDStart+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--write", "--check"},
		{"--check", "--file", file},
		{"-c", filepath.Join(t.TempDir(), "nope.yaml")},
	} {
		if code, _, _ := runArchfit(t, append([]string{cmdAgentsMD, "-c", "../../examples/ddd.archfit.yaml"}, args...)...); code != 3 {
			t.Errorf("agents-md %v: exit %d, want 3", args, code)
		}
	}
}

// TestAgentsMDRepositoryBlockIsCurrent is the engine's own drift check: the
// AGENTS.md block equals the block .archfit.yaml renders.
func TestAgentsMDRepositoryBlockIsCurrent(t *testing.T) {
	t.Parallel()
	if code, stdout, stderr := runArchfit(t, cmdAgentsMD, "-c", repoConfig, "--file", "../../AGENTS.md", "--check"); code != 0 {
		t.Errorf("AGENTS.md is out of date; run: archfit agents-md --write\n%s%s", stdout, stderr)
	}
}
