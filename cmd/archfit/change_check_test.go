package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitpkg "github.com/alexei-led/archfit/v3/internal/history/git"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// The record goldens of internal/output/record are written from design 8.1.
// The CLI goldens are those files: `archfit check` and `archfit can` on the
// fixture repository below must print each example byte for byte.
const designExamples = "../../internal/output/record/testdata"

// fixturePolicy is a version 3 policy for the fixture repository, the modules
// of design 8.1.
const fixturePolicy = `version: 3
owners: [alexei-led]
modules:
  pkg_chaos:
    paths: [pkg/chaos/**]
    volatility: high
    depends_on: [pkg_runtime]
  pkg_runtime:
    paths: [pkg/runtime/**]
    volatility: low
  pkg_tc:
    paths: [pkg/tc/**]
    volatility: high
  pkg_util:
    paths: [pkg/util/**]
    volatility: low
rules:
  - id: no-chaos-to-runtime
    deny: { from: pkg_chaos, to: pkg_runtime }
    why: A direct import ties a chaos action to one runtime.
    instead: Add a method to container.Client.
`

// expiredException adds to fixturePolicy an exception for the copy of
// util.go's literal, with a date.
const expiredException = `exceptions:
  - rule: duplication
    from: pkg_tc
    to: pkg_util
    expires: 2026-11-19
    why: "Request #431 is open."
`

const protectingWorkflow = `on: pull_request
jobs:
  archfit:
    runs-on: ubuntu-latest
    steps:
      - uses: alexei-led/archfit-action@v4
`

// Names the tests share.
const (
	cmdCan      = "can"
	cmdRules    = "rules"
	flagFormat  = "--format"
	fixtureDate = "2026-11-10"
	tcNetem     = "pkg/tc/netem.go"
	runtimeGo   = "pkg/runtime/runtime.go"
	notARepo    = "is not in a git repository"
	pkgTC       = "pkg/tc"
	pkgUtil     = "pkg/util"
	apiPolicy   = "services/api/.archfit.yaml"
	version1    = "version: 1\n"
	statusPass  = string(result.StatusPass)
	statusFail  = string(result.StatusFail)
)

// copiedLiteral is the literal the copy example repeats.
const copiedLiteral = `"container %s is not running"`

// fixtureFiles is the base tree: committed on main, which origin/main and
// origin/HEAD name.
func fixtureFiles() map[string]string {
	return map[string]string{
		"go.mod":                        "module example.com/pumba\n\ngo 1.24\n",
		defaultConfigPath:               fixturePolicy,
		".github/workflows/archfit.yml": protectingWorkflow,
		"pkg/chaos/netem/netem.go":      "package netem\n\n// Run adds delay.\nfunc Run() {}\n",
		runtimeGo:                       "package runtime\n\n// Do runs.\nfunc Do() {}\n",
		tcNetem:                         "package tc\n\n// Netem shapes traffic.\nfunc Netem() string { return \"netem\" }\n",
		"pkg/util/util.go": "package util\n\n" +
			"// Helpers shared by every package.\n" + strings.Repeat("//\n", 10) +
			"// NotRunning is the message of a stopped container.\n" +
			"const NotRunning = " + copiedLiteral + "\n",
	}
}

// Head versions of files for the examples. The import or the literal sits on
// the line design 8.1 names.
const (
	chaosImportsRuntime = "package netem\n\n// Run adds delay.\n//\n// It needs the runtime.\n\nimport (\n" +
		"\t\"example.com/pumba/pkg/runtime\"\n)\n\n// Run adds delay.\nfunc Run() { runtime.Do() }\n"
	tcImportsUtil = "package tc\n\n// Netem shapes traffic.\n//\n// One interface at a time.\n//\n// For the chaos actions.\n\nimport (\n" +
		"\t\"fmt\"\n\n\t\"example.com/pumba/pkg/util\"\n)\n\n// Netem shapes traffic.\nfunc Netem() string { return fmt.Sprint(util.NotRunning) }\n"
)

// tcCopiesLiteral copies util.go's literal on line 12.
var tcCopiesLiteral = "package tc\n\n// Interface names the tc errors.\n" + strings.Repeat("//\n", 7) +
	"// errNotRunning is the message of a stopped container.\nvar errNotRunning = " + copiedLiteral + "\n"

func requireGitBinary(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // the test's own git commands
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// fixtureRepo commits base on main, points origin/main and origin/HEAD at it,
// then commits head on the branch change: the change of a pull request.
func fixtureRepo(t *testing.T, base, head map[string]string) string {
	t.Helper()
	requireGitBinary(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	writeFiles(t, dir, base)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "base", "--no-gpg-sign")
	gitRun(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	gitRun(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	gitRun(t, dir, "checkout", "-q", "-b", "change")
	writeFiles(t, dir, head)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "change", "--allow-empty", "--no-gpg-sign")
	return dir
}

// day is a fixed clock: exceptions and expiry dates are judged on it.
func day(date string) func() time.Time {
	return func() time.Time {
		d, err := time.Parse(time.DateOnly, date)
		if err != nil {
			panic(err)
		}
		return d
	}
}

type cliRun struct {
	code           int
	stdout, stderr string
}

// runCLI runs archfit with args through the command line parser, on the date.
func runCLI(t *testing.T, date string, args ...string) cliRun {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runDeps(args, &appDeps{Runner: toolrun.New(), Stdout: &stdout, Stderr: &stderr, now: day(date)})
	return cliRun{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func readDesignExample(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(designExamples, name+".txt")) //nolint:gosec // test goldens
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func with(files map[string]string, more map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range files {
		out[k] = v
	}
	for k, v := range more {
		out[k] = v
	}
	return out
}

// TestDesignExamplesFromTheCLI runs every example of design 8.1 on a fixture
// repository and compares the output with the example, byte for byte.
func TestDesignExamplesFromTheCLI(t *testing.T) {
	t.Parallel()
	base := fixtureFiles()
	for _, tc := range []struct {
		name       string
		base, head map[string]string
		date       string
		args       []string
		wantCode   int
	}{
		{name: statusPass, base: base, head: map[string]string{tcNetem: base[tcNetem] + "\n// Unit is ms.\n"},
			date: fixtureDate, args: []string{cmdCheck}, wantCode: 0},
		{name: "deny", base: base, head: map[string]string{"pkg/chaos/netem/netem.go": chaosImportsRuntime},
			date: fixtureDate, args: []string{cmdCheck}, wantCode: 1},
		{name: "graph", base: base, head: map[string]string{tcNetem: tcImportsUtil},
			date: fixtureDate, args: []string{cmdCheck}, wantCode: 1},
		{name: "duplication", base: base, head: map[string]string{"pkg/tc/interface.go": tcCopiesLiteral},
			date: fixtureDate, args: []string{cmdCheck}, wantCode: 1},
		{name: "policy_differs", base: base, head: map[string]string{defaultConfigPath: fixturePolicy + "  - id: one-more\n    note: A note.\n    why: It is a note.\n"},
			date: fixtureDate, args: []string{cmdCheck}, wantCode: 2},
		{
			name: "expired_exception",
			base: with(base, map[string]string{defaultConfigPath: fixturePolicy + expiredException, "pkg/tc/interface.go": tcCopiesLiteral}),
			head: map[string]string{runtimeGo: base[runtimeGo] + "\n// Done.\n"},
			date: "2026-11-20", args: []string{cmdCheck}, wantCode: 1,
		},
		{name: "can", base: base, date: fixtureDate, args: []string{cmdCan, pkgTC, pkgUtil}, wantCode: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := fixtureRepo(t, tc.base, tc.head)
			got := runCLI(t, tc.date, append(tc.args, "-C", dir)...)
			if want := readDesignExample(t, tc.name); got.stdout != want {
				t.Errorf("stdout:\n--- got\n%s--- want\n%s--- stderr\n%s", got.stdout, want, got.stderr)
			}
			if got.code != tc.wantCode {
				t.Errorf("exit = %d, want %d; stderr: %s", got.code, tc.wantCode, got.stderr)
			}
		})
	}
}

// graphChange is the fixture with the change of the graph example.
func graphChange(t *testing.T, base map[string]string) string {
	t.Helper()
	return fixtureRepo(t, base, map[string]string{tcNetem: tcImportsUtil})
}

func TestCheckFormats(t *testing.T) {
	t.Parallel()
	dir := graphChange(t, fixtureFiles())
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{name: formatJSON, args: []string{flagFormat, formatJSON}, want: []string{`"schema": "archfit.result.v1"`, `"status": "` + statusFail + `"`, `"rule": "graph"`}},
		{name: formatSarif, args: []string{flagFormat, formatSarif}, want: []string{`"version": "2.1.0"`, `"ruleId": "graph"`}},
		{name: "verbose", args: []string{"--verbose"}, want: []string{"archfit: fail, 1 error\npolicy: .archfit.yaml sha256:", "\nengine: dev\n"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := runCLI(t, fixtureDate, append([]string{cmdCheck, "-C", dir}, tc.args...)...)
			if got.code != exitNo {
				t.Errorf("exit = %d, want 1; stderr: %s", got.code, got.stderr)
			}
			for _, want := range tc.want {
				if !strings.Contains(got.stdout, want) {
					t.Errorf("stdout does not hold %q:\n%s", want, got.stdout)
				}
			}
		})
	}
}

// TestCheckWithoutAProtectingWorkflow prints the line of design 9.4.
func TestCheckWithoutAProtectingWorkflow(t *testing.T) {
	t.Parallel()
	base := fixtureFiles()
	delete(base, ".github/workflows/archfit.yml")
	dir := fixtureRepo(t, base, nil)
	got := runCLI(t, fixtureDate, cmdCheck, "-C", dir)
	if want := "archfit: pass\npolicy: not protected\n"; got.stdout != want || got.code != 0 {
		t.Errorf("check = %d %q, want 0 %q; stderr: %s", got.code, got.stdout, want, got.stderr)
	}
	can := runCLI(t, fixtureDate, cmdCan, "pkg/chaos", "pkg/runtime", "-C", dir)
	if !strings.HasSuffix(can.stdout, "policy: not protected\n") {
		t.Errorf("can does not say the policy is not protected:\n%s", can.stdout)
	}
}

// TestCheckNotesGoToStderr: a target branch with no policy file is judged
// with the local policy, and the note says so on stderr, not in the record.
func TestCheckNotesGoToStderr(t *testing.T) {
	t.Parallel()
	base := fixtureFiles()
	delete(base, defaultConfigPath)
	dir := fixtureRepo(t, base, map[string]string{defaultConfigPath: fixturePolicy})
	got := runCLI(t, fixtureDate, cmdCheck, "-C", dir)
	if got.stdout != "archfit: pass\n" || got.code != 0 {
		t.Errorf("check = %d %q, want 0 %q", got.code, got.stdout, "archfit: pass\n")
	}
	if want := "archfit: origin/main has no .archfit.yaml: judged with the local policy\n"; got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
}

type lastOutput struct {
	Tree   string `json:"tree"`
	Output struct {
		Schema string `json:"schema"`
		Status string `json:"status"`
		Answer string `json:"answer"`
	} `json:"output"`
}

func readLast(t *testing.T, dir string) (lastOutput, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".archfit-cache", "last.json")) //nolint:gosec // fixture path
	if errors.Is(err, fs.ErrNotExist) {
		return lastOutput{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	var last lastOutput
	if err := json.Unmarshal(data, &last); err != nil {
		t.Fatalf("last.json: %v\n%s", err, data)
	}
	return last, true
}

// TestLastOutputForRequest: check and can keep their output with the tree id
// for archfit request; a --policy preview keeps nothing.
func TestLastOutputForRequest(t *testing.T) {
	t.Parallel()
	dir := graphChange(t, fixtureFiles())
	tree, err := gitpkg.WorkingTreeID(context.Background(), dir, toolrun.New())
	if err != nil {
		t.Fatal(err)
	}

	writeFiles(t, dir, map[string]string{"candidate.yaml": fixturePolicy})
	runCLI(t, fixtureDate, cmdCheck, "-C", dir, "--policy", "candidate.yaml")
	if _, ok := readLast(t, dir); ok {
		t.Fatal("a --policy preview stored its result")
	}
	_ = os.Remove(filepath.Join(dir, "candidate.yaml"))

	runCLI(t, fixtureDate, cmdCheck, "-C", dir)
	last, ok := readLast(t, dir)
	if !ok || last.Tree != tree || last.Output.Schema != "archfit.result.v1" || last.Output.Status != statusFail {
		t.Errorf("after check: last = %+v (found %v), want tree %s and the fail result", last, ok, tree)
	}

	runCLI(t, fixtureDate, cmdCan, pkgTC, pkgUtil, "-C", dir)
	last, _ = readLast(t, dir)
	if last.Tree != tree || last.Output.Answer != "no" {
		t.Errorf("after can: last = %+v, want tree %s and the no answer", last, tree)
	}
}

// TestCheckSameDateSameOutput: the date is an input; the same tree, policy
// and date give the same bytes.
func TestCheckSameDateSameOutput(t *testing.T) {
	t.Parallel()
	dir := graphChange(t, fixtureFiles())
	first := runCLI(t, fixtureDate, cmdCheck, "-C", dir, flagFormat, formatJSON)
	second := runCLI(t, fixtureDate, cmdCheck, "-C", dir, flagFormat, formatJSON)
	if first.stdout != second.stdout || first.code != second.code {
		t.Errorf("two runs differ:\n%s\n%s", first.stdout, second.stdout)
	}
}

// TestVersion3UsageAndToolErrorsExit2: wrong usage and a tool error give no
// verdict: exit 2, the message on stderr, nothing on stdout.
func TestVersion3UsageAndToolErrorsExit2(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, fixtureFiles(), nil)
	plain := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "a version 2 format", args: []string{cmdCheck, "-C", dir, flagFormat, "markdown"}, want: "--format markdown needs a version 2 policy"},
		{name: "two formats", args: []string{cmdCheck, "-C", dir, flagFormat, formatJSON, flagFormat, formatSarif}, want: "--format takes one value"},
		{name: "a version 2 flag", args: []string{cmdCheck, "-C", dir, "--json"}, want: "[--json]: these flags need a version 2 policy"},
		{name: "-c names a version 3 file", args: []string{cmdCheck, "-c", filepath.Join(dir, defaultConfigPath)}, want: "a version 3 check reads .archfit.yaml"},
		{name: "not a git repository", args: []string{cmdCheck, "-C", plain}, want: notARepo},
		{name: "can with one argument", args: []string{cmdCan, pkgTC, "-C", dir}, want: "usage: name a file"},
		{name: "rules mixing forms", args: []string{cmdRules, "--graph", pkgTC, "-C", dir}, want: "usage: --graph takes no path"},
		{name: "can outside a repository", args: []string{cmdCan, "a", "b", "-C", plain}, want: notARepo},
		{name: "rules outside a repository", args: []string{cmdRules, "-C", plain}, want: notARepo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := runCLI(t, fixtureDate, tc.args...)
			if got.code != exitUnknown || got.stdout != "" || !strings.Contains(got.stderr, tc.want) {
				t.Errorf("= %d, stdout %q, stderr %q; want 2, no stdout, stderr holding %q", got.code, got.stdout, got.stderr, tc.want)
			}
		})
	}
}

// TestCheckDispatchesOnThePolicyVersion (D1): a version 2 policy keeps the
// Archfit 3 check and its exit codes; a version 3 flag there is its usage
// error (3).
func TestCheckDispatchesOnThePolicyVersion(t *testing.T) {
	t.Parallel()
	v2 := writeViolatingRepo(t)
	old := runCLI(t, fixtureDate, cmdCheck, "-c", v2, flagFormat, formatJSON)
	if old.code != 1 || !strings.Contains(old.stdout, `"schema_version"`) {
		t.Errorf("version 2: exit %d, want 1 with the Archfit 3 JSON:\n%s\n%s", old.code, old.stdout, old.stderr)
	}

	// A file that is not version 3 keeps the Archfit 3 path and its exit 3,
	// even when it does not parse or names no version.
	broken := filepath.Join(t.TempDir(), defaultConfigPath)
	for _, data := range []string{"version: 2\nmodules: [\n", version1, "modules: {}\n"} {
		if err := os.WriteFile(broken, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := runCLI(t, fixtureDate, cmdCheck, "-c", broken); got.code != 3 {
			t.Errorf("-c with %q: exit %d, want 3; stderr %q", data, got.code, got.stderr)
		}
	}
	if got := runCLI(t, fixtureDate, cmdCheck, "-c", t.TempDir()); got.code != 3 {
		t.Errorf("-c naming a directory: exit %d, want 3; stderr %q", got.code, got.stderr)
	}

	requireGitBinary(t)
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	writeFiles(t, repo, map[string]string{defaultConfigPath: minimalValidYAML})
	got := runCLI(t, fixtureDate, cmdCheck, "-C", repo)
	if got.code != 3 || !strings.Contains(got.stderr, "-C and --verbose need a version 3 policy") {
		t.Errorf("version 2 with -C: exit %d, stderr %q; want 3", got.code, got.stderr)
	}
}

// TestCanExitCodes: yes is 0, no is 1, unknown is 2.
func TestCanExitCodes(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, fixtureFiles(), nil)
	for _, tc := range []struct {
		from, to string
		want     int
	}{
		{tcNetem, pkgTC, 0},
		{pkgTC, pkgUtil, exitNo},
		{"tools/gen.go", pkgTC, exitUnknown},
	} {
		got := runCLI(t, fixtureDate, cmdCan, tc.from, tc.to, "-C", dir)
		if got.code != tc.want {
			t.Errorf("can %s %s = %d, want %d\n%s%s", tc.from, tc.to, got.code, tc.want, got.stdout, got.stderr)
		}
	}
}

// TestPolicyFlagReadsFromTheStartDirectory: --policy .archfit.yaml with -C in
// a subdirectory names that directory's file, as the record shows.
func TestPolicyFlagReadsFromTheStartDirectory(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{apiPolicy: "version: 3\nowners: [alexei-led]\n"}), nil)
	got := runCLI(t, fixtureDate, cmdCheck, "-C", filepath.Join(dir, "services", "api"), "--policy", defaultConfigPath)
	if got.code != exitUnknown || !strings.HasPrefix(got.stdout, apiPolicy+": policy: ") {
		t.Errorf("= %d\n%s%s; want unknown about services/api/.archfit.yaml", got.code, got.stdout, got.stderr)
	}
}

// TestRulesWithoutATargetBranch: with no origin/HEAD, rules with no argument
// says to name a path or a rule id, not to pass a flag it does not take.
func TestRulesWithoutATargetBranch(t *testing.T) {
	t.Parallel()
	requireGitBinary(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	writeFiles(t, dir, fixtureFiles())
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "base", "--no-gpg-sign")
	got := runCLI(t, fixtureDate, cmdRules, "-C", dir)
	if got.code != exitUnknown || !strings.Contains(got.stderr, "origin/HEAD is not set: name a path or a rule id") || strings.Contains(got.stderr, "--base") {
		t.Errorf("= %d, stderr %q", got.code, got.stderr)
	}
}
