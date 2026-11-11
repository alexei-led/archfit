package policyfile_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
)

const (
	owner    = "alexei-led"
	modApp   = "app"
	modUtil  = "pkg_util"
	modStore = "store"
	modCLI   = "cli"
	layerCmd = "cmd"
	core     = "core"
	cmdGlob  = "cmd/**"
)

func ptr[T any](v T) *T { return &v }

func decodeFixture(t *testing.T, path string) (policyfile.File, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return policyfile.Decode(data)
}

func TestDecodeValidFixtures(t *testing.T) {
	tests := map[string]policyfile.File{
		"smallest": {
			Version: 3,
			Owners:  []string{owner},
			Modules: map[string]policyfile.Module{modApp: {Paths: []string{cmdGlob, "pkg/**"}}},
		},
		"typical": {
			Version: 3,
			Owners:  []string{owner},
			Gate:    policyfile.GateRequired,
			Layers:  []string{core, layerCmd},
			Modules: map[string]policyfile.Module{
				"pkg_chaos": {
					Paths:      []string{"pkg/chaos/**"},
					Public:     []string{"pkg/chaos", "pkg/chaos/cliflags"},
					Volatility: policyfile.VolatilityHigh,
					Layer:      core,
					DependsOn:  []string{"pkg_container", modUtil},
				},
				"pkg_tc": {
					Paths: []string{"pkg/tc/**"}, Public: []string{"pkg/tc"},
					Volatility: policyfile.VolatilityHigh, Layer: core,
				},
				modUtil: {Paths: []string{"pkg/util/**"}, Volatility: policyfile.VolatilityLow, Layer: core},
			},
			Rules: []policyfile.Rule{
				{
					ID:      "no-chaos-to-runtime",
					Deny:    &policyfile.Deny{From: policyfile.Names{"pkg_chaos"}, To: policyfile.Names{"pkg_runtime"}},
					Why:     "A direct import ties a chaos action to one runtime.",
					Instead: "Add a method to container.Client.",
				},
				{
					ID:   "docker-sdk-in-runtime",
					Only: &policyfile.Only{Use: "github.com/docker/docker", In: policyfile.Names{"pkg_runtime"}},
					Why:  "One adapter owns the Docker API.",
				},
				{
					ID:   "fan-out-through-runner",
					Note: "Chaos actions fan out through RunOnContainers.",
					Why:  "One place checks the target limit before injection.",
				},
			},
			Exceptions: []policyfile.Exception{{
				Rule: "duplication", From: "pkg_tc", To: modUtil, Expires: "2026-11-19", Why: "Request #431 is open.",
			}},
		},
		"every-key": {
			Version: 3,
			Owners:  []string{owner, "octocat"},
			Gate:    policyfile.GateAdvisory,
			Layers:  []string{"domain", "adapter", layerCmd},
			Modules: map[string]policyfile.Module{
				core: {
					Paths: []string{"internal/core/**"}, Public: []string{"internal/core"},
					Subdomain: policyfile.SubdomainCore, Layer: "domain", DependsOn: []string{},
				},
				modStore: {
					Paths: []string{"internal/store/**"}, Volatility: policyfile.VolatilityMedium,
					Subdomain: policyfile.SubdomainSupporting, Layer: "adapter", DependsOn: []string{core},
				},
				modCLI: {Paths: []string{cmdGlob}, Layer: layerCmd, DependsOn: []string{core, modStore}},
			},
			Builtin: policyfile.Builtin{
				Graph: policyfile.ModeDeny, Coupling: policyfile.ModeAsk, Duplication: policyfile.ModeOff,
				Cycle: policyfile.ModeDeny, Layers: policyfile.ModeAsk, Unowned: policyfile.ModeOff,
				MinBalance: ptr(8),
			},
			Rules: []policyfile.Rule{
				{
					ID:      "no-cli-to-sql",
					Deny:    &policyfile.Deny{From: policyfile.Names{modCLI, "cmd/tools/**"}, To: policyfile.Names{"database/sql"}},
					Why:     "SQL belongs to the store.",
					Instead: "Call a store method.",
				},
				{
					ID:      "process-in-cli",
					Only:    &policyfile.Only{Use: "process", In: policyfile.Names{modCLI}},
					Why:     "Side effects stay at the edge.",
					Instead: "Return a command for the CLI to run.",
				},
			},
			Exceptions: []policyfile.Exception{
				{Rule: "graph", From: modStore, To: modCLI, Why: "Migration in progress."},
				{Rule: policyfile.RuleUnowned, From: "scripts/**", Expires: "2027-01-31", Why: "Scripts get a module next quarter."},
			},
			Generated: []string{"**/*.pb.go"},
			Tests:     []string{"**/*_test.go", "internal/testutil/**"},
			Exclude:   []string{"third_party/**"},
			Languages: policyfile.Languages{
				Go: policyfile.GoLanguage{
					Enabled: policyfile.SwitchOn,
					Modules: policyfile.GoModules{Include: []string{"services/**"}, Exclude: []string{"services/legacy"}},
				},
				TypeScript: policyfile.Language{Enabled: policyfile.SwitchOff},
				Python:     policyfile.PythonLanguage{Enabled: policyfile.SwitchAuto, Package: "myapp"},
				Rust: policyfile.RustLanguage{
					Enabled: policyfile.SwitchAuto, Manifest: "Cargo.toml", Features: []string{modCLI}, IncludeDevDeps: true,
				},
				DotNet: policyfile.DotNetLanguage{Enabled: policyfile.SwitchOff, Projects: []string{"App.slnx"}},
			},
			Analyzers: policyfile.Analyzers{
				Syntax:       policyfile.Analyzer{Enabled: policyfile.SwitchOn},
				CargoModules: policyfile.Analyzer{Enabled: policyfile.SwitchOff},
				Roslyn:       policyfile.TimedAnalyzer{Enabled: policyfile.SwitchOn, Timeout: "1m30s"},
			},
			Coverage: policyfile.Coverage{
				Enabled: true,
				Sources: []policyfile.CoverageSource{{
					Path: "coverage.out", Format: "go-coverprofile", SidecarPath: "coverage.out.sidecar.json",
					MaxBytes: ptr(int64(1048576)), MaxFacts: ptr(5000),
				}},
			},
		},
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := decodeFixture(t, filepath.Join("testdata", "valid", name+".yaml"))
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Decode =\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

// TestDecodeKeepsAbsentKeysZero lets the policy model tell an absent key from a
// set one: the smallest policy leaves every defaulted key at its zero value.
func TestDecodeKeepsAbsentKeysZero(t *testing.T) {
	got, err := decodeFixture(t, filepath.Join("testdata", "valid", "smallest.yaml"))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	app := got.Modules[modApp]
	if got.Gate != "" || got.Layers != nil || got.Builtin != (policyfile.Builtin{}) ||
		app.Public != nil || app.Volatility != "" || app.Subdomain != "" || app.DependsOn != nil {
		t.Errorf("absent keys decoded to values: %+v", got)
	}
}

// invalidFixtures maps each file in testdata/invalid to the messages its error must contain.
var invalidFixtures = map[string][]string{
	"unknown-key":          {`unknown field "owner"`, "6:5"},
	"version-missing":      {`version: missing: add "version: 3"`},
	"version-2":            {`version 2 is the Archfit 3 policy format: run "archfit init" to convert it to version 3`},
	"version-4":            {"version 4: this engine reads version 3"},
	"version-string":       {`version "3": this engine reads version 3`},
	"expires-shape":        {`exceptions[0].expires: "19-11-2026" is not a date: use YYYY-MM-DD`},
	"expires-impossible":   {`exceptions[0].expires: "2026-02-30" is not a date: use YYYY-MM-DD`},
	"rule-two-forms":       {"rules[0] (two-forms): has deny and note: a rule has exactly one of deny, only or note"},
	"rule-no-form":         {"rules[0] (no-form): needs one of deny, only or note"},
	"builtin-mode-unknown": {`builtin.graph: "warn" is not one of deny, ask, off`},
	"min-balance-low":      {"builtin.min_balance: 0 is outside 1 to 10"},
	"min-balance-high":     {"builtin.min_balance: 11 is outside 1 to 10"},
	"volatility-unknown":   {`modules.app.volatility: "frozen" is not one of low, medium, high`},
	"subdomain-unknown":    {`modules.app.subdomain: "platform" is not one of core, supporting, generic`},
	"gate-unknown":         {`gate: "fail" is not one of advisory, required`},
	"enabled-unknown":      {`languages.go.enabled: "on" is not true, false or "auto"`},
	"owners-missing":       {"owners: required: list at least one GitHub login"},
	"owners-empty":         {"owners: required: list at least one GitHub login"},
	"modules-missing":      {"modules: required: declare at least one module"},
	"paths-missing":        {"modules.app.paths: required: list at least one glob"},
	"public-empty":         {"modules.app.public: empty: list at least one glob, or remove the key"},
	"module-duplicate":     {`"app"`, "already defined"},
	"layers-duplicate":     {`layers: "core" is listed twice`},
	"why-missing":          {"rules[0] (no-why).why: required: give the reason for the rule"},
	"instead-missing":      {"rules[0] (no-instead).instead: required with deny: say what to do instead"},
	"names-empty":          {"rules[0] (empty-from).deny.from: empty: name a module or a path glob"},
	"exception-to-missing": {"exceptions[0].to: required: name the module the errors reach"},
	"exception-unowned-to": {"exceptions[0].to: unowned has no target module: remove to"},
	"timeout-bad":          {`analyzers.roslyn.timeout: "five minutes" is not a Go duration, such as 5m`},
	"coverage-no-sources":  {"coverage.sources: required when coverage is enabled: list at least one file"},
}

func TestDecodeInvalidFixtures(t *testing.T) {
	for name, wants := range invalidFixtures {
		t.Run(name, func(t *testing.T) {
			_, err := decodeFixture(t, filepath.Join("testdata", "invalid", name+".yaml"))
			if err == nil {
				t.Fatal("Decode accepted an invalid policy")
			}
			for _, want := range wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestEveryInvalidFixtureHasExpectedMessages(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "invalid", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(invalidFixtures) {
		t.Errorf("testdata/invalid has %d files, the table has %d rows", len(files), len(invalidFixtures))
	}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".yaml")
		if _, ok := invalidFixtures[name]; !ok {
			t.Errorf("%s has no row in invalidFixtures", file)
		}
	}
}

// TestDecodeReportsEveryProblem lists all problems at once, so an owner fixes
// the file in one pass.
func TestDecodeReportsEveryProblem(t *testing.T) {
	_, err := policyfile.Decode([]byte(`version: 3
owners: []
modules:
  app:
    paths: [cmd/**]
    volatility: frozen
builtin:
  min_balance: 0
`))
	if err == nil {
		t.Fatal("Decode accepted an invalid policy")
	}
	for _, want := range []string{"owners:", "modules.app.volatility:", "builtin.min_balance:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestDecodeSwitchForms(t *testing.T) {
	tests := []struct {
		enabled, wantErr string
		want             policyfile.Switch
	}{
		{enabled: "true", want: policyfile.SwitchOn},
		{enabled: "false", want: policyfile.SwitchOff},
		{enabled: "auto", want: policyfile.SwitchAuto},
		{enabled: `"true"`, wantErr: `languages.go.enabled: "true" is not true, false or "auto"`},
		{enabled: "1", wantErr: `languages.go.enabled: 1 is not true, false or "auto"`},
		{enabled: "[true]", wantErr: `enabled takes true, false or "auto"`},
	}
	for _, tt := range tests {
		t.Run(tt.enabled, func(t *testing.T) {
			got, err := policyfile.Decode([]byte("version: 3\nowners: [o]\nmodules:\n  app:\n    paths: [cmd/**]\nlanguages:\n  go:\n    enabled: " + tt.enabled + "\n"))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if got.Languages.Go.Enabled != tt.want {
				t.Errorf("enabled = %q, want %q", got.Languages.Go.Enabled, tt.want)
			}
		})
	}
}

func TestDecodeNamesForms(t *testing.T) {
	tests := []struct {
		name, entry string
		want        policyfile.Names
		wantErr     string
	}{
		{name: "absent", entry: "", wantErr: "rules[0] (r).deny.from: required: name a module or a path glob"},
		{name: "one name", entry: "from: app", want: policyfile.Names{"app"}},
		{name: "a list", entry: "from: [app, cmd/**]", want: policyfile.Names{"app", "cmd/**"}},
		{name: "empty string", entry: `from: ""`, wantErr: "rules[0] (r).deny.from: has an empty name"},
		{name: "empty name in a list", entry: `from: [app, ""]`, wantErr: "rules[0] (r).deny.from: has an empty name"},
		{name: "null", entry: "from: ", wantErr: "rules[0].deny.from: empty value: give a value or remove the key"},
		{name: "a map", entry: "from: {a: b}", wantErr: "want one name or a list of names"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := policyfile.Decode([]byte(`version: 3
owners: [alexei-led]
modules:
  app:
    paths: [cmd/**]
rules:
  - id: r
    deny: { ` + tt.entry + `, to: lib }
    why: w
    instead: i
`))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if from := got.Rules[0].Deny.From; !reflect.DeepEqual(from, tt.want) {
				t.Errorf("deny.from = %v, want %v", from, tt.want)
			}
		})
	}
}
