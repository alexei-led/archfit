package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

func TestReportJSONOmitsLegacyDecisionAndTasks(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{defaultConfigPath: reportPolicy}), map[string]string{tcNetem: tcImportsUtil})
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir, flagFormat, formatJSON)
	if got.code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", got.code, got.stderr)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{"verdict", "decision", "agent_tasks", "advisory_tasks"} {
		if _, ok := doc[field]; ok {
			t.Errorf("report contains retired field %q", field)
		}
	}
	if _, ok := doc["repository"]; !ok {
		t.Error("report has no repository state")
	}
	if _, ok := doc["measurement"]; !ok {
		t.Error("report has no measurements")
	}
}

func TestReportRepositoryErrorsUseExceptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, date, policy, want string
		head                     map[string]string
		drift, expired           int
		unread                   bool
	}{
		{name: "without errors", want: report.RepositoryClean},
		{name: "drift", head: map[string]string{tcNetem: tcImportsUtil}, want: report.RepositoryDrift, drift: 1},
		{name: "exception covers copy", policy: fixturePolicy + expiredException, head: map[string]string{tcInterface: tcCopiesLiteral}, want: report.RepositoryClean},
		{name: "exception expired", date: "2026-11-20", policy: fixturePolicy + expiredException, head: map[string]string{tcInterface: tcCopiesLiteral}, want: report.RepositoryExpired, expired: 1},
		{name: "unread source", head: map[string]string{"pkg/util/broken.go": "package util\nimport (\n"}, want: report.RepositoryClean, unread: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			policy, date := tc.policy, tc.date
			if policy == "" {
				policy = fixturePolicy
			}
			if date == "" {
				date = fixtureDate
			}
			dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{defaultConfigPath: policy}), tc.head)
			got := runCLI(t, date, cmdReport, "-C", dir, flagFormat, formatJSON)
			if got.code != 0 {
				t.Fatalf("exit = %d, want 0; stderr: %s", got.code, got.stderr)
			}
			var doc report.ArchitectureState
			if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
				t.Fatal(err)
			}
			r := doc.Repository
			if r == nil || r.State != tc.want || len(r.Drift) != tc.drift || len(r.Expired) != tc.expired {
				t.Fatalf("repository = %+v, want %s with %d drift and %d expired", r, tc.want, tc.drift, tc.expired)
			}
			if (len(r.Unread) > 0) != tc.unread {
				t.Fatalf("unread = %+v, want unread=%v", r.Unread, tc.unread)
			}
			text := runCLI(t, date, cmdReport, "-C", dir)
			if text.code != 0 || !strings.HasPrefix(text.stdout, "repository: "+tc.want) {
				t.Fatalf("text report exit %d:\n%s\n%s", text.code, text.stdout, text.stderr)
			}
			if tc.unread && !strings.Contains(text.stdout, "not read") {
				t.Fatalf("text report hid unread source:\n%s", text.stdout)
			}
		})
	}
}
