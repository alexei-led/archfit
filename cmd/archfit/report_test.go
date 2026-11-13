package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

// reportGoldens holds the state part of `archfit report`: the text before
// "measurements:". The measurements depend on the tools of the machine.
const reportGoldens = "testdata/report"

const (
	cmdReport    = "report"
	measurements = "measurements:\n"
	// caseNotProtected names the cases and goldens of a repository no
	// workflow protects.
	caseNotProtected = "not_protected"
)

// reportPolicy adds to fixturePolicy a note and an exception close to its
// date for the copied literal.
const reportPolicy = fixturePolicy + `  - id: keep-docs
    note: Each module has a README.
    why: New readers start there.
exceptions:
  - rule: duplication
    from: pkg_tc
    to: pkg_util
    expires: 2026-11-19
    why: "Request #431 is open."
`

// statePart is the text of the report before its measurements.
func statePart(t *testing.T, text string) string {
	t.Helper()
	i := strings.Index(text, measurements)
	if i < 0 {
		t.Fatalf("the report has no measurements:\n%s", text)
	}
	return text[:i]
}

// TestReportFromTheCLI: the state of the working tree, the measurements, and
// exit 0 whatever the state is.
func TestReportFromTheCLI(t *testing.T) {
	t.Parallel()
	base := with(fixtureFiles(), map[string]string{defaultConfigPath: reportPolicy, tcInterface: tcCopiesLiteral})
	unprotected := with(base, nil)
	delete(unprotected, ".github/workflows/archfit.yml")
	for _, tc := range []struct {
		name       string
		base, head map[string]string
	}{
		{name: "clean", base: base},
		{name: "drift", base: base, head: map[string]string{tcNetem: tcImportsUtil}},
		{name: caseNotProtected, base: unprotected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := fixtureRepo(t, tc.base, tc.head)
			got := runCLI(t, fixtureDate, cmdReport, "-C", dir)
			if got.code != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", got.code, got.stderr)
			}
			assertCLIGolden(t, reportGoldens, tc.name, statePart(t, got.stdout))
			if protected := !strings.HasSuffix(got.stdout, "policy: not protected\n"); protected != (tc.name != caseNotProtected) {
				t.Errorf("policy: not protected shown = %v:\n%s", !protected, got.stdout)
			}
		})
	}
}

// TestReportJSON: the report document carries the repository state.
func TestReportJSON(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{defaultConfigPath: reportPolicy}), map[string]string{tcNetem: tcImportsUtil})
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir, flagFormat, formatJSON)
	if got.code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", got.code, got.stderr)
	}
	var doc report.ArchitectureState
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, got.stdout)
	}
	r := doc.Repository
	if doc.SchemaVersion != report.StateSchemaVersion || r == nil || r.State != report.RepositoryDrift ||
		len(r.Drift) != 1 || r.Drift[0].Rule != ruleGraph || len(r.NotChecked) != 1 || !r.Protected {
		t.Errorf("document %s with repository %+v; want the drift of the graph error and one note", doc.SchemaVersion, r)
	}
}

// TestReportOnAVersion2Policy: the measurements only, and the way to a state.
func TestReportOnAVersion2Policy(t *testing.T) {
	t.Parallel()
	files := fixtureFiles()
	files[defaultConfigPath] = "version: 2\nmodules:\n  pkg_tc:\n    paths: [pkg/tc/**]\n    depends_on: []\n"
	dir := fixtureRepo(t, files, nil)
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir)
	if got.code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", got.code, got.stderr)
	}
	if want := "repository: no state: the policy is version 2: run archfit init\n"; statePart(t, got.stdout) != want {
		t.Errorf("report:\n%s\nwant it to start with %q", got.stdout, want)
	}
}

// TestReportOnAPolicyThatDeclaresTests: the Go extractor reads no test file,
// yet the policy's tests glob matches the test files of the tree, so the
// report gives a state, as check does.
func TestReportOnAPolicyThatDeclaresTests(t *testing.T) {
	t.Parallel()
	files := with(fixtureFiles(), map[string]string{
		defaultConfigPath:       fixturePolicy + "tests:\n  - \"**/*_test.go\"\n",
		"pkg/util/util_test.go": "package util\n\nimport \"testing\"\n\nfunc TestUtil(t *testing.T) {}\n",
	})
	dir := fixtureRepo(t, files, nil)
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir)
	if got.code != 0 || !strings.HasPrefix(got.stdout, "repository: clean\n") {
		t.Errorf("exit %d, stdout %q, stderr %q; want 0 and a clean state", got.code, got.stdout, got.stderr)
	}
}

// TestReportErrorsExit2: no policy, or one that does not fit the tree, stops
// the report with exit 2 and prints nothing.
func TestReportErrorsExit2(t *testing.T) {
	t.Parallel()
	noPolicy := fixtureFiles()
	delete(noPolicy, defaultConfigPath)
	deadGlob := with(fixtureFiles(), map[string]string{defaultConfigPath: strings.Replace(fixturePolicy, "pkg/util/**", "pkg/utils/**", 1)})
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{name: "no policy", files: noPolicy, want: "run archfit init"},
		{name: "a glob that matches nothing", files: deadGlob, want: "pkg/utils/**"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := fixtureRepo(t, tc.files, nil)
			got := runCLI(t, fixtureDate, cmdReport, "-C", dir)
			if got.code != exitUnknown || got.stdout != "" || !strings.Contains(got.stderr, tc.want) {
				t.Errorf("exit %d, stdout %q, stderr %q; want 2 and %q", got.code, got.stdout, got.stderr, tc.want)
			}
		})
	}
}
