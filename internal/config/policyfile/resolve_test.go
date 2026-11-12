package policyfile_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/policy/resolved"
)

const smallestPolicy = "version: 3\nowners: [alexei-led]\nmodules:\n  app:\n    paths: [cmd/**, pkg/**]\n"

// withModule is the smallest policy with the module app replaced by body.
func withModule(body string) string {
	return "version: 3\nowners: [alexei-led]\nmodules:\n  app:\n    paths: [pkg/**]\n" + body
}

func resolve(t *testing.T, src string) resolved.Policy {
	t.Helper()
	p, err := policyfile.Resolve([]byte(src))
	if err != nil {
		t.Fatalf("Resolve: %v\n%s", err, src)
	}
	return p
}

// TestResolveDefaults has one case per row of the defaults table of design
// 6.1, and the built-in defaults of 6.2.
func TestResolveDefaults(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		check func(t *testing.T, p resolved.Policy)
	}{
		{"depends_on absent: no other module", smallestPolicy, func(t *testing.T, p resolved.Policy) {
			if deps := p.Modules[modApp].DependsOn; len(deps) != 0 {
				t.Errorf("DependsOn = %v, want none", deps)
			}
		}},
		{"public absent: the whole module", smallestPolicy, func(t *testing.T, p resolved.Policy) {
			if got := p.Modules[modApp].Public; !reflect.DeepEqual(got, []string{cmdGlob, "pkg/**"}) {
				t.Errorf("Public = %v, want the paths", got)
			}
		}},
		{"public set: as written", withModule("    public: [pkg/api]\n"), func(t *testing.T, p resolved.Policy) {
			if got := p.Modules[modApp].Public; !reflect.DeepEqual(got, []string{"pkg/api"}) {
				t.Errorf("Public = %v", got)
			}
		}},
		{"volatility and subdomain absent: high", smallestPolicy, wantVolatility(resolved.VolatilityHigh)},
		{"subdomain core: high", withModule("    subdomain: core\n"), wantVolatility(resolved.VolatilityHigh)},
		{"subdomain supporting: low", withModule("    subdomain: supporting\n"), wantVolatility(resolved.VolatilityLow)},
		{"subdomain generic: low", withModule("    subdomain: generic\n"), wantVolatility(resolved.VolatilityLow)},
		{
			"volatility decides over subdomain", withModule("    subdomain: generic\n    volatility: medium\n"),
			wantVolatility(resolved.VolatilityMedium),
		},
		{"subdomain absent: none recorded", smallestPolicy, func(t *testing.T, p resolved.Policy) {
			if s := p.Modules[modApp].Subdomain; s != "" {
				t.Errorf("Subdomain = %q, want none", s)
			}
		}},
		{"layer and layers absent: no layer rule", smallestPolicy, func(t *testing.T, p resolved.Policy) {
			if p.Layers != nil || p.Modules[modApp].Layer != "" {
				t.Errorf("Layers = %v, Layer = %q, want none", p.Layers, p.Modules[modApp].Layer)
			}
		}},
		{"gate absent: advisory", smallestPolicy, func(t *testing.T, p resolved.Policy) {
			if p.Gate != resolved.GateAdvisory {
				t.Errorf("Gate = %q, want advisory", p.Gate)
			}
		}},
		{"rules and exceptions absent: none, built-in rules apply", smallestPolicy, func(t *testing.T, p resolved.Policy) {
			if p.Rules != nil || p.Exceptions != nil {
				t.Errorf("Rules = %v, Exceptions = %v, want none", p.Rules, p.Exceptions)
			}
			if p.Builtin.Graph == resolved.ModeOff || p.Builtin.Coupling == resolved.ModeOff {
				t.Errorf("Builtin = %+v, want the built-in rules on", p.Builtin)
			}
		}},
		{"generated and tests absent: only listed paths are out", smallestPolicy, func(t *testing.T, p resolved.Policy) {
			if p.Generated != nil || p.Tests != nil {
				t.Errorf("Generated = %v, Tests = %v, want none", p.Generated, p.Tests)
			}
		}},
		{"builtin absent: the defaults of 6.2", smallestPolicy, func(t *testing.T, p resolved.Policy) {
			want := resolved.Builtin{
				Graph: resolved.ModeAsk, Coupling: resolved.ModeDeny, Duplication: resolved.ModeAsk,
				Cycle: resolved.ModeDeny, Layers: resolved.ModeDeny, Unowned: resolved.ModeDeny, MinBalance: 7,
			}
			if p.Builtin != want {
				t.Errorf("Builtin = %+v, want %+v", p.Builtin, want)
			}
		}},
		{
			"builtin set: only the named keys change", smallestPolicy + "builtin: { graph: off, min_balance: 8 }\n",
			func(t *testing.T, p resolved.Policy) {
				if p.Builtin.Graph != resolved.ModeOff || p.Builtin.MinBalance != 8 || p.Builtin.Cycle != resolved.ModeDeny {
					t.Errorf("Builtin = %+v", p.Builtin)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { tt.check(t, resolve(t, tt.src)) })
	}
}

func wantVolatility(want resolved.Volatility) func(*testing.T, resolved.Policy) {
	return func(t *testing.T, p resolved.Policy) {
		t.Helper()
		if got := p.Modules[modApp].Volatility; got != want {
			t.Errorf("Volatility = %q, want %q", got, want)
		}
	}
}

func TestResolveEveryKey(t *testing.T) {
	data, err := os.ReadFile("testdata/valid/every-key.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p, err := policyfile.Resolve(data)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.Digest != resolved.Digest(data) {
		t.Errorf("Digest = %q, want the digest of the file bytes", p.Digest)
	}
	wantRules := []resolved.Rule{
		{
			ID: "no-cli-to-sql", Form: resolved.FormDeny, From: []string{modCLI, "cmd/tools/**"}, To: []string{"database/sql"},
			Why: "SQL belongs to the store.", Instead: "Call a store method.",
		},
		{
			ID: onlyRule, Form: resolved.FormOnly, Use: "process", In: []string{modCLI},
			Why: "Side effects stay at the edge.", Instead: "Return a command for the CLI to run.",
		},
	}
	if !reflect.DeepEqual(p.Rules, wantRules) {
		t.Errorf("Rules = %+v\nwant %+v", p.Rules, wantRules)
	}
	wantExceptions := []resolved.Exception{
		{Rule: "graph", From: modStore, To: modCLI, Why: "Migration in progress."},
		{
			Rule: resolved.RuleUnowned, From: "scripts/**", Expires: time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC),
			Why: "Scripts get a module next quarter.",
		},
		{Rule: onlyRule, From: modStore, Why: "The migration tool shells out until it moves to the CLI."},
	}
	if !reflect.DeepEqual(p.Exceptions, wantExceptions) {
		t.Errorf("Exceptions = %+v\nwant %+v", p.Exceptions, wantExceptions)
	}
	for key, want := range map[string]string{
		"languages.go.enabled":            "true",
		"languages.go.modules.include[0]": "services/**",
		"languages.typescript.enabled":    "false",
		"languages.python.enabled":        "auto",
		"analyzers.roslyn.timeout":        "1m30s",
	} {
		if got := p.Settings[key]; got != want {
			t.Errorf("Settings[%q] = %q, want %q", key, got, want)
		}
	}
	if p.Modules[modStore].Volatility != resolved.VolatilityMedium || p.Modules[core].Volatility != resolved.VolatilityHigh {
		t.Errorf("volatility: store %q, core %q", p.Modules[modStore].Volatility, p.Modules[core].Volatility)
	}
	if p.Builtin.MinBalance != 8 || p.Exclude[0] != "third_party/**" {
		t.Errorf("MinBalance = %d, Exclude = %v", p.Builtin.MinBalance, p.Exclude)
	}
}

func TestResolveErrors(t *testing.T) {
	tests := map[string]struct {
		src  string
		want []string
	}{
		"decode error": {src: "version: 2\n", want: []string{"archfit init"}},
		"unknown modules of the typical example": {
			src: mustRead(t, "testdata/valid/typical.yaml"),
			want: []string{
				`modules.pkg_chaos.depends_on: "pkg_container" is not a module`,
				`rules[0] (no-chaos-to-runtime).deny.to: "pkg_runtime" is not a module`,
				`rules[1] (docker-sdk-in-runtime).only.in: "pkg_runtime" is not a module`,
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := policyfile.Resolve([]byte(tt.src))
			if err == nil {
				t.Fatal("Resolve: want an error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q\nlacks %q", err, w)
				}
			}
		})
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
