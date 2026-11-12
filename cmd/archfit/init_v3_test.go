package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/initcfg/policy3"
)

// initGoldens holds the text of `archfit init`.
const initGoldens = "testdata/init"

const (
	cmdInit      = "init"
	originRemote = "https://github.com/alexei-led/pumba.git"
	agentsNotes  = "# Agents\n\nRun make test before you push.\n"
)

// archfit3Policy is an Archfit 3 policy for the fixture repository.
const archfit3Policy = `version: 2
modules:
  pkg_chaos:
    paths: [pkg/chaos/**]
    depends_on: []
  pkg_runtime:
    paths: [pkg/runtime/**]
    depends_on: []
  pkg_tc:
    paths: [pkg/tc/**]
    depends_on: [pkg_util]
  pkg_util:
    paths: [pkg/util/**]
    depends_on: []
rules:
  - id: no-chaos-to-runtime
    type: forbidden_dependency
    from: pkg/chaos/**
    to: pkg/runtime/**
    rationale: A direct import ties a chaos action to one runtime.
    alternatives: [Add a method to container.Client.]
`

// initFixture is the fixture code without a policy: the copied literal of
// pkg/tc is an error that exists, which init writes as an exception.
func initFixture() map[string]string {
	files := fixtureFiles()
	delete(files, defaultConfigPath)
	files[tcInterface] = tcCopiesLiteral
	files[policy3.AgentsFile] = agentsNotes
	return files
}

// initRepo is fixtureRepo with an origin remote on GitHub, for the owners.
func initRepo(t *testing.T, base map[string]string) string {
	t.Helper()
	dir := fixtureRepo(t, base, nil)
	gitRun(t, dir, "remote", "add", "origin", originRemote)
	return dir
}

// commitAll commits the working tree on the branch of the change.
func commitAll(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "init", "--no-gpg-sign")
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // fixture files
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestInitAFreshRepository: init writes the policy and the block, lists the
// error that exists as an exception, writes no workflow, and the first check
// passes.
func TestInitAFreshRepository(t *testing.T) {
	t.Parallel()
	dir := initRepo(t, initFixture())
	got := runCLI(t, fixtureDate, cmdInit, "-C", dir)
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("init: exit %d, stderr %q", got.code, got.stderr)
	}
	assertCLIGolden(t, initGoldens, "fresh", got.stdout)

	if policy := readText(t, filepath.Join(dir, defaultConfigPath)); !isVersion3([]byte(policy)) {
		t.Errorf("the written policy is not version 3:\n%s", policy)
	}
	if agents := readText(t, filepath.Join(dir, policy3.AgentsFile)); agents != agentsNotes+"\n"+policy3.Block {
		t.Errorf("AGENTS.md =\n%s\nwant the notes, then the block", agents)
	}
	workflows, err := os.ReadDir(filepath.Join(dir, workflowsDir))
	if err != nil || len(workflows) != 1 {
		t.Errorf("workflows = %v, %v; want only the fixture's own (D11)", workflows, err)
	}

	commitAll(t, dir)
	check := runCLI(t, fixtureDate, cmdCheck, "-C", dir)
	if check.code != 0 || check.stdout != "archfit: pass\n" {
		t.Errorf("check after init: exit %d\n%s%s", check.code, check.stdout, check.stderr)
	}
}

// TestInitConvertsAnArchfit3Repository: the old files become one version 3
// policy, and check with that policy passes.
func TestInitConvertsAnArchfit3Repository(t *testing.T) {
	t.Parallel()
	files := initFixture()
	files[defaultConfigPath] = archfit3Policy
	files[policy3.BaselineFile] = `{"schema_version": "archfit.baseline.v3", "accepted": []}`
	files[policy3.AppFile] = "version: 1\nowners: [alexei-led]\ngate: required\n"
	dir := initRepo(t, files)

	got := runCLI(t, fixtureDate, cmdInit, "-C", dir)
	if got.code != 0 || got.stderr != "" {
		t.Fatalf("init: exit %d, stderr %q", got.code, got.stderr)
	}
	assertCLIGolden(t, initGoldens, "convert", got.stdout)
	for _, gone := range []string{policy3.BaselineFile, policy3.AppFile} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s is still there: %v", gone, err)
		}
	}

	commitAll(t, dir)
	check := runCLI(t, fixtureDate, cmdCheck, "-C", dir, "--policy", defaultConfigPath)
	if check.code != 0 || !strings.HasPrefix(check.stdout, "archfit: pass\n") {
		t.Errorf("check --policy after init: exit %d\n%s%s", check.code, check.stdout, check.stderr)
	}
}

// TestInitWithAVersion3Policy: init leaves the policy as it is and writes
// only a missing block; a second run writes nothing. Both exit 0.
func TestInitWithAVersion3Policy(t *testing.T) {
	t.Parallel()
	dir := initRepo(t, fixtureFiles())
	before := readText(t, filepath.Join(dir, defaultConfigPath))

	for _, name := range []string{"version3_block", "version3_nothing"} {
		got := runCLI(t, fixtureDate, cmdInit, "-C", dir)
		if got.code != 0 || got.stderr != "" {
			t.Fatalf("%s: exit %d, stderr %q", name, got.code, got.stderr)
		}
		assertCLIGolden(t, initGoldens, name, got.stdout)
	}
	if after := readText(t, filepath.Join(dir, defaultConfigPath)); after != before {
		t.Errorf("init changed the policy:\n%s", after)
	}
	if agents := readText(t, filepath.Join(dir, policy3.AgentsFile)); agents != policy3.Block {
		t.Errorf("AGENTS.md =\n%s\nwant the block", agents)
	}
}

// TestInitErrorsExit2: a real error exits 2, names its cause, and writes
// nothing.
func TestInitErrorsExit2(t *testing.T) {
	t.Parallel()
	unbalanced := initFixture()
	unbalanced[policy3.AgentsFile] = "<!-- archfit:start -->\nno end\n"
	oldVersion := initFixture()
	oldVersion[defaultConfigPath] = version1
	noRemote := initFixture()
	for _, tc := range []struct {
		name   string
		files  map[string]string
		remote bool
		want   string
	}{
		{name: "unbalanced markers", files: unbalanced, remote: true, want: "markers are unbalanced"},
		{name: "a version 1 policy", files: oldVersion, remote: true, want: "no Archfit 3 policy"},
		{name: "no origin remote", files: noRemote, want: "origin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := fixtureRepo(t, tc.files, nil)
			if tc.remote {
				gitRun(t, dir, "remote", "add", "origin", originRemote)
			}
			got := runCLI(t, fixtureDate, cmdInit, "-C", dir)
			if got.code != exitUnknown || got.stdout != "" || !strings.Contains(got.stderr, tc.want) {
				t.Errorf("exit %d, stdout %q, stderr %q; want 2 and %q", got.code, got.stdout, got.stderr, tc.want)
			}
			if _, had := tc.files[defaultConfigPath]; had {
				if policy := readText(t, filepath.Join(dir, defaultConfigPath)); policy != tc.files[defaultConfigPath] {
					t.Errorf("init changed the policy:\n%s", policy)
				}
			} else if _, err := os.Stat(filepath.Join(dir, defaultConfigPath)); !os.IsNotExist(err) {
				t.Errorf("init wrote a policy: %v", err)
			}
			if agents := readText(t, filepath.Join(dir, policy3.AgentsFile)); agents != tc.files[policy3.AgentsFile] {
				t.Errorf("init changed AGENTS.md:\n%s", agents)
			}
		})
	}

	got := runCLI(t, fixtureDate, cmdInit, "-C", t.TempDir())
	if got.code != exitUnknown || !strings.Contains(got.stderr, notARepo) {
		t.Errorf("outside a repository: exit %d, stderr %q", got.code, got.stderr)
	}
}
