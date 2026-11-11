package rulesquery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/application/rulesquery"
	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
	"github.com/alexei-led/archfit/v3/internal/testutil"
)

const (
	tcFile    = "pkg/tc/netem.go"
	chaosFile = "pkg/chaos/run.go"
	utilFile  = "pkg/util/x.go"
	graphRule = "graph"
)

// today is ten days before the first exception expires and after the second.
var today = time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC)

type policySource struct {
	p   resolved.Policy
	err error
}

func (s policySource) Policy(context.Context) (resolved.Policy, error) { return s.p, s.err }

type changedFiles struct {
	files []string
	err   error
}

func (c changedFiles) Changed(context.Context) ([]string, error) { return c.files, c.err }

func loadPolicy(t *testing.T) resolved.Policy {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "policy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := policyfile.Resolve(data)
	if err != nil {
		t.Fatalf("resolve the test policy: %v", err)
	}
	return p
}

func ask(t *testing.T, p resolved.Policy, changed []string, req rulesquery.Request) string {
	t.Helper()
	if req.Date.IsZero() {
		req.Date = today
	}
	svc := rulesquery.Service{Policies: policySource{p: p}, Changed: changedFiles{files: changed}}
	l, err := svc.Rules(context.Background(), req)
	if err != nil {
		t.Fatalf("Rules(%+v): %v", req, err)
	}
	var b strings.Builder
	if err := l.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestGoldens prints each form of `archfit rules` against the test policy.
// ARCHFIT_UPDATE_RULES=1 rewrites the goldens; read the diff before keeping it.
func TestGoldens(t *testing.T) {
	p := loadPolicy(t)
	changed := []string{
		tcFile, "pkg/tc/internal/shape.go", "pkg/chaos/netem/delay.go", "pkg/tc/netem_test.go",
		"pkg/gen/api.go", "scripts/release.go", tcFile,
	}
	for _, tc := range []struct {
		name string
		req  rulesquery.Request
	}{
		{name: "path", req: rulesquery.Request{Args: []string{chaosFile}}},
		{name: "changed", req: rulesquery.Request{}},
		{name: "rule_builtin", req: rulesquery.Request{Args: []string{graphRule}}},
		{name: "rule_coupling", req: rulesquery.Request{Args: []string{"coupling"}}},
		{name: "rule_deny", req: rulesquery.Request{Args: []string{"no-chaos-to-runtime"}}},
		{name: "rule_only", req: rulesquery.Request{Args: []string{"docker-sdk-in-runtime"}}},
		{name: "rule_note", req: rulesquery.Request{Args: []string{"fan-out-through-runner"}}},
		{name: "graph", req: rulesquery.Request{Graph: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ask(t, p, changed, tc.req)
			golden := filepath.Join("testdata", tc.name+".txt")
			if os.Getenv("ARCHFIT_UPDATE_RULES") != "" {
				if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden) //nolint:gosec // fixed golden names
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if got != string(want) {
				t.Errorf("rules %s differs from its golden:\n--- got\n%s--- golden\n%s", tc.name, got, want)
			}
		})
	}
}

// TestGoldensUseNoNotUsedWord holds every golden to the words of design 5.
func TestGoldensUseNoNotUsedWord(t *testing.T) {
	goldens, err := filepath.Glob(filepath.Join("testdata", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(goldens) == 0 {
		t.Fatal("no golden found")
	}
	for _, path := range goldens {
		raw, err := os.ReadFile(path) //nolint:gosec // golden files of this package
		if err != nil {
			t.Fatal(err)
		}
		if words := testutil.NotUsedIn(string(raw)); len(words) > 0 {
			t.Errorf("%s uses %q (design 5, the Not used column)", path, words)
		}
	}
}

func TestPaths(t *testing.T) {
	p := loadPolicy(t)
	for _, tc := range []struct {
		name    string
		args    []string
		date    time.Time
		has     []string
		hasNot  []string
		changed []string
	}{
		{
			name: "the most specific module owns a nested path",
			args: []string{"pkg/chaos/netem/delay.go"},
			has:  []string{"pkg/chaos/netem/delay.go: module pkg_chaos_netem\n", "  allowed dependencies: pkg_chaos\n"},
		},
		{
			name: "a path that does not exist yet has its module",
			args: []string{"./pkg/tc/new_file.go"},
			has:  []string{"pkg/tc/new_file.go: module pkg_tc\n"},
		},
		{
			name: "a directory gets the module whose glob covers it",
			args: []string{"pkg/util"},
			has:  []string{"pkg/util: module pkg_util\n"},
		},
		{
			name:   "an off rule is not listed",
			args:   []string{utilFile},
			hasNot: []string{"cycle:"},
		},
		{
			name: "min_balance shows in coupling",
			args: []string{utilFile},
			has:  []string{"coupling: new knowledge across a module boundary needs a balance of at least 8 (deny: fix the code)\n"},
		},
		{
			name: "a volatility from the subdomain names it",
			args: []string{"pkg/container/c.go"},
			has:  []string{"  volatility: low (subdomain supporting)\n"},
		},
		{
			name:   "a deny rule by module applies to its from side",
			args:   []string{chaosFile},
			has:    []string{"no-chaos-to-runtime: pkg_chaos must not depend on pkg_runtime\n"},
			hasNot: []string{"no-tc-internals-to-util"},
		},
		{
			name: "a deny rule by module applies to its to side",
			args: []string{"pkg/runtime/docker.go"},
			has:  []string{"no-chaos-to-runtime: pkg_chaos must not depend on pkg_runtime\n"},
		},
		{
			name: "a deny rule by path glob applies to a file it covers",
			args: []string{"pkg/tc/internal/shape.go"},
			has:  []string{"no-tc-internals-to-util: pkg/tc/internal/** must not depend on pkg_util\n"},
		},
		{
			name:   "a deny rule by path glob does not apply to another file of the module",
			args:   []string{tcFile},
			hasNot: []string{"no-tc-internals-to-util"},
		},
		{
			name: "an only rule applies to every module",
			args: []string{tcFile},
			has:  []string{"docker-sdk-in-runtime: github.com/docker/docker is used only in pkg_runtime\n"},
		},
		{
			name: "an exception matches by its to module",
			args: []string{"pkg/container/c.go"},
			has:  []string{"graph pkg_tc -> pkg_container: 2026-11-01 (expired)\n"},
		},
		{
			name: "an exception holds on its day",
			args: []string{utilFile},
			date: time.Date(2026, 11, 19, 23, 0, 0, 0, time.UTC),
			has:  []string{"duplication pkg_tc -> pkg_util: 2026-11-19 (0 days)\n"},
		},
		{
			name: "an exception is expired the day after",
			args: []string{utilFile},
			date: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
			has:  []string{"duplication pkg_tc -> pkg_util: 2026-11-19 (expired)\n"},
		},
		{
			name: "an exception without a date says so",
			args: []string{chaosFile},
			has:  []string{"no-chaos-to-runtime pkg_chaos -> pkg_runtime: no date\n"},
		},
		{
			name:   "an unowned exception matches by its glob",
			args:   []string{"scripts/release.go", "other/x.go"},
			has:    []string{"other/x.go, scripts/release.go: in no module\n", "unowned scripts/**: no date\n"},
			hasNot: []string{"layers:"},
		},
		{
			name:   "an unowned exception is absent when its glob covers no path",
			args:   []string{"other/x.go"},
			hasNot: []string{"exceptions:"},
		},
		{
			name: "declared tests and generated code are out of the rules",
			args: []string{"pkg/tc/netem_test.go", "pkg/gen/api.go"},
			has: []string{
				"pkg/tc/netem_test.go: declared tests\n  the rules skip these paths\n",
				"pkg/gen/api.go: declared generated\n  the rules skip these paths\n",
			},
			hasNot: []string{"module pkg_tc"},
		},
		{
			name: "files of one module share one block",
			args: []string{"pkg/tc/b.go", "pkg/tc/a.go"},
			has:  []string{"pkg/tc/a.go, pkg/tc/b.go: module pkg_tc\n"},
		},
		{
			name: "a word that is not a rule id is a path",
			args: []string{"graphs"},
			has:  []string{"graphs: in no module\n"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ask(t, p, tc.changed, rulesquery.Request{Args: tc.args, Date: tc.date})
			for _, want := range tc.has {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
			for _, unwanted := range tc.hasNot {
				if strings.Contains(got, unwanted) {
					t.Errorf("unexpected %q in:\n%s", unwanted, got)
				}
			}
			if n := strings.Count(got, "not checked:\n"); n != 1 {
				t.Errorf("the notes are listed %d times, want once:\n%s", n, got)
			}
		})
	}
}

func TestLayersRuleNeedsLayers(t *testing.T) {
	p := resolved.Policy{Modules: map[string]resolved.Module{"app": {Paths: []string{"app/**"}}}}.WithDefaults()
	got := ask(t, p, nil, rulesquery.Request{Args: []string{"app/main.go"}})
	if strings.Contains(got, "layers:") {
		t.Errorf("a policy without layers lists the layers rule:\n%s", got)
	}
	if strings.Contains(got, "not checked") {
		t.Errorf("a policy without notes lists notes:\n%s", got)
	}
	if !strings.Contains(got, "  layer: none\n  allowed dependencies: none\n") {
		t.Errorf("missing the defaults of the module:\n%s", got)
	}
}

func TestGraphWithoutLayers(t *testing.T) {
	p := resolved.Policy{Modules: map[string]resolved.Module{
		"b": {Paths: []string{"b/**"}},
		"a": {Paths: []string{"a/**"}, DependsOn: []string{"b"}},
	}}.WithDefaults()
	want := "layers: none\nno layer:\n  a -> b\n  b -> none\n"
	if got := ask(t, p, nil, rulesquery.Request{Graph: true}); got != want {
		t.Errorf("graph =\n%s\nwant\n%s", got, want)
	}
}

func TestChangedFiles(t *testing.T) {
	p := loadPolicy(t)
	if got := ask(t, p, nil, rulesquery.Request{}); got != "no changed files: name a path or a rule id\n" {
		t.Errorf("no changed files = %q", got)
	}
}

func TestErrors(t *testing.T) {
	p := loadPolicy(t)
	boom := errors.New("boom")
	for _, tc := range []struct {
		name string
		svc  rulesquery.Service
		req  rulesquery.Request
		want error
	}{
		{
			name: "graph with an argument",
			svc:  rulesquery.Service{Policies: policySource{p: p}},
			req:  rulesquery.Request{Graph: true, Args: []string{"pkg/tc"}},
			want: rulesquery.ErrUsage,
		},
		{
			name: "a rule id among other arguments",
			svc:  rulesquery.Service{Policies: policySource{p: p}},
			req:  rulesquery.Request{Args: []string{graphRule, tcFile}},
			want: rulesquery.ErrUsage,
		},
		{
			name: "the policy cannot be read",
			svc:  rulesquery.Service{Policies: policySource{err: boom}},
			req:  rulesquery.Request{Args: []string{"pkg/tc"}},
			want: boom,
		},
		{
			name: "the changed files cannot be read",
			svc:  rulesquery.Service{Policies: policySource{p: p}, Changed: changedFiles{err: boom}},
			req:  rulesquery.Request{},
			want: boom,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, err := tc.svc.Rules(context.Background(), tc.req)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if l.String() != "" {
				t.Errorf("an error left a listing: %q", l.String())
			}
		})
	}
}

func TestOffRuleInFull(t *testing.T) {
	got := ask(t, loadPolicy(t), nil, rulesquery.Request{Args: []string{"cycle"}})
	if want := "cycle: no new cycle between modules\n  mode: off (not checked)\n"; got != want {
		t.Errorf("cycle =\n%s\nwant\n%s", got, want)
	}
}
