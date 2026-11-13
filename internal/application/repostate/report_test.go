package repostate_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/application/repostate"
	"github.com/alexei-led/archfit/v3/internal/assessment/change"
	"github.com/alexei-led/archfit/v3/internal/assessment/count"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const policyPath = ".archfit.yaml"

// TestReportProjectsEveryPart maps each part of the state into the report
// contract, as the record shows it.
func TestReportProjectsEveryPart(t *testing.T) {
	expired := graphX(day(-1))
	r := repostate.Repository{
		State: repostate.StateExpired,
		Drift: []count.Error{{Error: change.Error{
			Key: runtimeKey, From: tc, To: runtime,
			Sites: []change.Site{{File: tcFile, Line: 6}, {File: tcFile, Line: 9}},
			What:  "pkg_tc -> pkg_runtime is not an allowed dependency", Why: "a new dependency between modules needs an owner",
			Instead: "Use pkg_util.", Balance: &result.Balance{Value: 6, Min: 7},
		}}},
		Expired: []count.Error{{
			Error:     change.Error{Key: expiredKey, From: tc, To: util, What: "graph pkg_tc -> pkg_util expired on 2026-11-09", Why: "an exception with a date fails every change after that date"},
			Exception: &expired,
		}},
		Expiring:   []repostate.Expiring{{Exception: graphX(day(3)), Expires: "2026-11-13 (3 days)"}},
		NotChecked: []resolved.Rule{{ID: "keep-docs", Form: resolved.FormNote, Note: "Each module has a README.", Why: "New readers start there."}},
		Balances:   []repostate.Balance{{From: chaos, To: util, Balance: result.Balance{Value: 9, Min: 7}}},
		Unknown:    []evidence.UnknownItem{{File: "web/app.ts", Line: 3, Reason: "import ./x does not resolve", Fix: "add web/x.ts", Cause: evidence.CauseImport}},
	}
	want := report.RepositoryState{
		State: report.RepositoryExpired,
		Drift: []report.RepositoryError{{
			Path: tcFile, Line: 6, Rule: resolved.RuleGraph, From: tc, To: runtime,
			What: "pkg_tc -> pkg_runtime is not an allowed dependency", Why: "a new dependency between modules needs an owner",
			Instead: "Use pkg_util.", Balance: &report.RepositoryBalance{Value: 6, Min: 7},
		}},
		Expired: []report.RepositoryError{{
			Path: policyPath, Rule: count.RuleException, From: tc, To: util, Expires: "2026-11-09",
			What: "graph pkg_tc -> pkg_util expired on 2026-11-09", Why: "an exception with a date fails every change after that date",
		}},
		Expiring:   []report.ExpiringException{{Rule: resolved.RuleGraph, From: tc, To: util, Expires: "2026-11-13 (3 days)", Why: "Request #431 is open."}},
		NotChecked: []report.NoteRule{{ID: "keep-docs", Note: "Each module has a README.", Why: "New readers start there."}},
		Balances:   []report.AllowedDependency{{From: chaos, To: util, Balance: report.RepositoryBalance{Value: 9, Min: 7}}},
		Unread:     []report.UnreadPlace{{Path: "web/app.ts", Line: 3, Reason: "import ./x does not resolve", Fix: "add web/x.ts"}},
		Protected:  true,
	}
	if got := r.Report(policyPath, true); !reflect.DeepEqual(got, want) {
		t.Errorf("Report =\n%+v\nwant\n%+v", got, want)
	}
}

// TestReportOfACleanRepositoryHasEveryKey: empty parts are empty lists, so a
// reader of the JSON finds each key.
func TestReportOfACleanRepositoryHasEveryKey(t *testing.T) {
	r := repostate.Repository{State: repostate.StateClean}
	data, err := json.Marshal(r.Report(policyPath, false))
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"state":"clean","drift":[],"expired":[],"expiring":[],"not_checked":[],"balances":[],"unread":[],"protected":false}`
	if string(data) != want {
		t.Errorf("JSON = %s\nwant %s", data, want)
	}
}
