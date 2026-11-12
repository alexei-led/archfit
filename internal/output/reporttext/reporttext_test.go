package reporttext_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/output/reporttext"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

const (
	tc   = "pkg_tc"
	util = "pkg_util"
)

// Regenerate the goldens with ARCHFIT_UPDATE_REPORTTEXT=1 and read the diff.
const goldens = "testdata"

var measured = []report.MetricResult{
	{Name: "coupling_balance", Value: 78, Display: "78", Band: "mixed"},
	{Name: "cycle", Value: 0},
	{Name: "encapsulation", Display: "n/a", Band: "n/a"},
}

func graphError() report.RepositoryError {
	return report.RepositoryError{
		Path: "pkg/tc/netem.go", Line: 12, Rule: "graph", From: tc, To: util,
		What: "pkg_tc -> pkg_util is not an allowed dependency", Why: "a new dependency between modules needs an owner",
		Balance: &report.RepositoryBalance{Value: 9, Min: 7},
	}
}

func state(r report.RepositoryState) report.Document {
	d := report.NewDocument()
	d.Metrics = measured
	d.State.Repository = &r
	return d
}

func empty(s string) report.RepositoryState {
	return report.RepositoryState{
		State: s, Drift: []report.RepositoryError{}, Expired: []report.RepositoryError{},
		Expiring: []report.ExpiringException{}, NotChecked: []report.NoteRule{},
		Balances: []report.AllowedDependency{}, Unread: []report.UnreadPlace{}, Protected: true,
	}
}

func TestRenderGoldens(t *testing.T) {
	drift := empty(report.RepositoryDrift)
	drift.Drift = []report.RepositoryError{graphError()}
	drift.Expiring = []report.ExpiringException{{Rule: "duplication", From: tc, To: util, Expires: "2026-11-19 (9 days)"}}
	drift.NotChecked = []report.NoteRule{{ID: "keep-docs", Note: "Each module has a README.", Why: "New readers start there."}}
	drift.Balances = []report.AllowedDependency{{From: "pkg_chaos", To: "pkg_runtime", Balance: report.RepositoryBalance{Value: 9, Min: 7}}}
	drift.Unread = []report.UnreadPlace{
		{Path: "web/app.ts", Line: 3, Reason: `import "./x" does not resolve`, Fix: "add web/x.ts"},
		{Reason: "ast-grep is not installed", Fix: "archfit doctor --fix"},
	}
	drift.Protected = false

	expired := empty(report.RepositoryExpired)
	expired.Expired = []report.RepositoryError{{
		Path: ".archfit.yaml", Rule: "exception", From: tc, To: util, Expires: "2026-11-19",
		What: "duplication pkg_tc -> pkg_util expired on 2026-11-19", Why: "an exception with a date fails every change after that date",
	}}
	expired.Drift = []report.RepositoryError{graphError()}

	version2 := report.NewDocument()
	version2.Metrics = measured

	for _, tc := range []struct {
		name string
		doc  report.Document
	}{
		{name: "clean", doc: state(empty(report.RepositoryClean))},
		{name: "drift", doc: state(drift)},
		{name: "expired", doc: state(expired)},
		{name: "version2", doc: version2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := reporttext.New().Render(tc.doc, &buf); err != nil {
				t.Fatal(err)
			}
			got := buf.String()
			if words := testutil.NotUsedIn(got); len(words) > 0 {
				t.Errorf("the report uses %q (design 5, the Not used column):\n%s", words, got)
			}
			path := filepath.Join(goldens, tc.name+".txt")
			if os.Getenv("ARCHFIT_UPDATE_REPORTTEXT") != "" {
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path) //nolint:gosec // test goldens
			if err != nil {
				t.Fatalf("%v (regenerate with ARCHFIT_UPDATE_REPORTTEXT=1)", err)
			}
			if got != string(want) {
				t.Errorf("%s:\n--- got\n%s--- want\n%s", path, got, want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	if got := reporttext.New().Format(); got != "text" {
		t.Errorf("Format = %q, want text", got)
	}
}
