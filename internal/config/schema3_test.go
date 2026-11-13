package config_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/alexei-led/archfit/v3/internal/application"
	"github.com/alexei-led/archfit/v3/internal/config"
	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/evidence/acquisition"
	suppliedcoverage "github.com/alexei-led/archfit/v3/internal/extract/coverage"
	"github.com/alexei-led/archfit/v3/internal/model/report"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func reportConfig(t *testing.T, path string) config.Config {
	t.Helper()
	cfg, err := config.ReportConfig(readFixture(t, path))
	if err != nil {
		t.Fatalf("ReportConfig(%s): %v", path, err)
	}
	return cfg
}

// The golden is the whole projected Config: every view the report reads is a
// function of it. Read a changed golden before you accept it.
func TestReportConfigTypicalPolicyGolden(t *testing.T) {
	t.Parallel()
	cfg := reportConfig(t, "testdata/schema3/typical.yaml")
	// JSON, not YAML: it keeps a nil list (null) apart from an empty one ([]),
	// and depends_on: [] is what "may depend on no other module" means.
	got, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	want := readFixture(t, "testdata/schema3/typical.golden.json")
	if !bytes.Equal(got, want) {
		t.Errorf("projection differs from testdata/schema3/typical.golden.json:\n--- got\n%s--- golden\n%s", got, want)
	}
}

func TestReportConfigDefaultsOfSection61(t *testing.T) {
	t.Parallel()
	cfg := reportConfig(t, "testdata/schema3/typical.yaml")
	tests := map[string]struct {
		module     string
		public     []string
		volatility policyfile.Volatility
		dependsOn  []string
	}{
		"declared public, volatility and depends_on": {
			module: "pkg_chaos", public: []string{"pkg/chaos", "pkg/chaos/cliflags"}, volatility: policyfile.VolatilityHigh,
			dependsOn: []string{"pkg_container", "pkg_util"},
		},
		"no public: the whole module": {module: "pkg_util", public: []string{"pkg/util/**"}, volatility: policyfile.VolatilityLow, dependsOn: []string{}},
		"no volatility, no subdomain: high": {
			module: "pkg_container", public: []string{"pkg/container/**"}, volatility: policyfile.VolatilityHigh, dependsOn: []string{},
		},
		"generic subdomain: low": {module: "pkg_runtime", public: []string{"pkg/runtime/**"}, volatility: policyfile.VolatilityLow, dependsOn: []string{}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			def, ok := cfg.Modules[tt.module]
			if !ok {
				t.Fatalf("module %q missing from %v", tt.module, cfg.Modules)
			}
			if !slices.Equal(def.Public, tt.public) || def.Volatility != string(tt.volatility) {
				t.Errorf("public %v, volatility %q; want %v, %q", def.Public, def.Volatility, tt.public, tt.volatility)
			}
			// A nil list would let the module depend on any module in Archfit 3.
			if def.DependsOn == nil || !slices.Equal(def.DependsOn, tt.dependsOn) {
				t.Errorf("depends_on %#v, want %#v", def.DependsOn, tt.dependsOn)
			}
			if def.Internal != nil || def.Owner != "" || def.DeployUnit != "" {
				t.Errorf("internal %v, owner %q, deploy unit %q: want none", def.Internal, def.Owner, def.DeployUnit)
			}
		})
	}
}

func TestReportConfigRemovesGating(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"testdata/schema3/typical.yaml", "policyfile/testdata/valid/every-key.yaml"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			cfg := reportConfig(t, path)
			snapshot := cfg.PolicySnapshot()
			gates := snapshot.Gates
			if len(gates.Rules.Rules) != 0 || len(snapshot.Assessment.Waivers.Waivers) != 0 {
				t.Errorf("rules %v, waivers %v: want none", gates.Rules.Rules, snapshot.Assessment.Waivers.Waivers)
			}
			if gates.Coupling.Mode != policy.DistributedMonolithWarn {
				t.Errorf("coupling gate mode %q, want warn: only fail blocks", gates.Coupling.Mode)
			}
			if len(gates.Metrics) != 0 || gates.ModuleReview != "" || gates.UncoveredChange != "" {
				t.Errorf("metric gates %v, module review %q, uncovered change %q: want defaults", gates.Metrics, gates.ModuleReview, gates.UncoveredChange)
			}
			for _, id := range []string{
				config.LangGo, config.LangTypeScript, config.LangPython, config.LangRust, config.LangDotNet,
				config.ToolSyntax, config.ToolScip, config.ToolClones, config.ToolCargoModules, config.ToolRoslyn,
			} {
				if g := cfg.ToolGate(id); g != "" {
					t.Errorf("tool gate %s = %q, want the warn default", id, g)
				}
			}
			if cfg.ScipEnabled() || cfg.ClonesEnabled() {
				t.Errorf("scip %v, clones %v: want both off", cfg.ScipEnabled(), cfg.ClonesEnabled())
			}
			if _, ok := cfg.LLM(); ok || len(cfg.ExternalSystems) != 0 || cfg.TargetBranch != "" {
				t.Errorf("ai, external systems or target branch set: %+v", cfg)
			}
			if err := config.ValidateRules(cfg); err != nil {
				t.Errorf("ValidateRules: %v", err)
			}
		})
	}
}

func TestReportConfigToolKeysReachTheirViews(t *testing.T) {
	t.Parallel()
	cfg := reportConfig(t, "policyfile/testdata/valid/every-key.yaml")
	every, err := policyfile.Decode(readFixture(t, "policyfile/testdata/valid/every-key.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	goExtract := cfg.ForExtract(config.LangGo)
	pyExtract := cfg.ForExtract(config.LangPython)
	rustExtract := cfg.ForExtract(config.LangRust)
	dotnetExtract := cfg.ForExtract(config.LangDotNet)
	coverage := cfg.SuppliedCoverageOptions()
	tests := map[string]struct{ got, want any }{
		"languages.go.modules.include":    {goExtract.GoModuleInclude, []string{"services/**"}},
		"languages.go.modules.exclude":    {goExtract.GoModuleExclude, []string{"services/legacy"}},
		"languages.python.package":        {pyExtract.PyPackage, "myapp"},
		"languages.rust.manifest":         {rustExtract.CargoManifest, "Cargo.toml"},
		"languages.rust.features":         {rustExtract.CargoFeatures, []string{"cli"}},
		"languages.rust.include_dev_deps": {rustExtract.IncludeDevDeps, true},
		"languages.dotnet.projects":       {dotnetExtract.DotNetProjects, []string{"App.slnx"}},
		"analyzers.roslyn.timeout":        {dotnetExtract.TypeGraphTimeout, 90 * time.Second},
		"exclude":                         {cfg.ForScope().Exclusions, []string{"third_party/**"}},
		"generated":                       {cfg.ForFileClass().GeneratedGlobs, []string{"**/*.pb.go"}},
		"tests":                           {cfg.ForFileClass().TestGlobs, []string{"**/*_test.go", "internal/testutil/**"}},
		"layers":                          {cfg.Layers, every.Layers},
		"coverage.enabled":                {coverage.Enabled, true},
		"coverage.sources": {coverage.Sources, []suppliedcoverage.Source{{
			Path: "coverage.out", Format: "go-coverprofile", SidecarPath: "coverage.out.sidecar.json",
			MaxBytes: 1048576, MaxFacts: 5000,
		}}},
	}
	for key, tt := range tests {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			got, _ := yaml.Marshal(tt.got)
			want, _ := yaml.Marshal(tt.want)
			if !bytes.Equal(got, want) {
				t.Errorf("got %s want %s", got, want)
			}
		})
	}
}

// flippedSwitches sets every switch to the value every-key.yaml does not use,
// so each switch is seen on and off across the two policies.
const flippedSwitches = `version: 3
owners: [octocat]
modules:
  app:
    paths: [cmd/**]
languages:
  go: {enabled: false}
  typescript: {enabled: true}
  python: {enabled: true}
  rust: {enabled: false}
  dotnet: {enabled: true}
analyzers:
  syntax: {enabled: false}
  cargo_modules: {enabled: true}
  roslyn: {enabled: false}
`

func TestReportConfigSwitchesReachToolModes(t *testing.T) {
	t.Parallel()
	languages := []string{config.LangGo, config.LangTypeScript, config.LangPython, config.LangRust, config.LangDotNet}
	ids := append(slices.Clone(languages), config.ToolSyntax, config.ToolCargoModules, config.ToolRoslyn)
	flipped, err := config.ReportConfig([]byte(flippedSwitches))
	if err != nil {
		t.Fatal(err)
	}
	on, off, auto := config.ModeOn, config.ModeOff, config.ModeAuto
	tests := map[string]struct {
		cfg  config.Config
		want []config.ToolMode // in ids order
	}{
		"every-key.yaml":      {reportConfig(t, "policyfile/testdata/valid/every-key.yaml"), []config.ToolMode{on, off, auto, auto, off, on, off, on}},
		"each switch flipped": {flipped, []config.ToolMode{off, on, on, off, on, off, on, off}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := tt.cfg
			for i, id := range ids {
				if got := cfg.ToolMode(id); got != tt.want[i] {
					t.Errorf("ToolMode(%s) = %q, want %q", id, got, tt.want[i])
				}
			}
			for _, lang := range languages {
				if got := cfg.ForExtract(lang).Mode; got != cfg.ToolMode(lang) {
					t.Errorf("ForExtract(%s).Mode = %q, want %q", lang, got, cfg.ToolMode(lang))
				}
			}
			if cfg.SyntaxEnabled() != (cfg.ToolMode(config.ToolSyntax) == on) {
				t.Errorf("syntax enabled %v with mode %q", cfg.SyntaxEnabled(), cfg.ToolMode(config.ToolSyntax))
			}
			if got := cfg.ForExtract(config.LangRust).ModuleGraph; got != (cfg.ToolMode(config.ToolCargoModules) == on) {
				t.Errorf("rust module graph %v with cargo_modules %q", got, cfg.ToolMode(config.ToolCargoModules))
			}
			if got := cfg.ForExtract(config.LangDotNet).TypeGraphDisabled; got != (cfg.ToolMode(config.ToolRoslyn) == off) {
				t.Errorf(".NET type graph disabled %v with roslyn %q", got, cfg.ToolMode(config.ToolRoslyn))
			}
		})
	}
}

func TestReportConfigAbsentSwitchesAreAuto(t *testing.T) {
	t.Parallel()
	cfg := reportConfig(t, "policyfile/testdata/valid/smallest.yaml")
	for _, id := range []string{
		config.LangGo, config.LangTypeScript, config.LangPython, config.LangRust, config.LangDotNet,
		config.ToolSyntax, config.ToolCargoModules, config.ToolRoslyn,
	} {
		if mode := cfg.ToolMode(id); mode != config.ModeAuto {
			t.Errorf("ToolMode(%s) = %q, want auto", id, mode)
		}
	}
	// Auto keeps each analyzer's own default: syntax and cargo_modules off,
	// roslyn on where its helper is installed.
	if cfg.SyntaxEnabled() || cfg.CargoModulesEnabled() || !cfg.RoslynEnabled() {
		t.Errorf("syntax %v, cargo_modules %v, roslyn %v", cfg.SyntaxEnabled(), cfg.CargoModulesEnabled(), cfg.RoslynEnabled())
	}
	if cfg.Coverage.Enabled || len(cfg.Coverage.Sources) != 0 {
		t.Errorf("coverage %+v, want off", cfg.Coverage)
	}
}

func TestReportConfigRefusesWhatResolveRefuses(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		src  string
		want string
	}{
		"Archfit 3 policy":  {src: string(readFixture(t, "policyfile/testdata/invalid/version-2.yaml")), want: "archfit init"},
		"unknown module":    {src: string(readFixture(t, "policyfile/testdata/valid/typical.yaml")), want: `"pkg_container" is not a module`},
		"not a policy file": {src: "version: 3\nowners: [a]\n", want: "modules"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := config.ReportConfig([]byte(tt.src))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ReportConfig error %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// reportPolicy declares two modules. The check would report the imports of
// a.go: a has no depends_on, a deny rule names a -> b, and they point from the
// inner layer out. The report only measures them.
const reportPolicy = `version: 3
owners: [octocat]
gate: required
layers: [core, app]
modules:
  a:
    paths: [a/**]
    layer: core
  b:
    paths: [b/**]
    public: [b/api]
    volatility: high
    layer: app
rules:
  - id: no-a-to-b
    deny: { from: a, to: b }
    why: a stays independent of b.
    instead: Move the shared part into a.
`

var reportRepo = map[string]string{
	".archfit.yaml": reportPolicy,
	"go.mod":        "module example.com/report\n\ngo 1.22\n",
	"a/a.go":        "package a\n\nimport (\n\t\"example.com/report/b\"\n\t\"example.com/report/b/api\"\n)\n\nfunc A() string { return b.B() + api.API() }\n",
	"b/b.go":        "package b\n\nimport \"example.com/report/b/api\"\n\nfunc B() string { return api.API() }\n",
	"b/api/api.go":  "package api\n\nfunc API() string { return \"api\" }\n",
}

func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The analyze pipeline runs on a schema 3 policy: it measures, and what the
// check would report gives no gate result.
func TestReportPipelineRunsOnSchema3AndDoesNotGate(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go/packages on a temporary module")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	t.Parallel()
	root := writeRepo(t, reportRepo)
	configPath := filepath.Join(root, ".archfit.yaml")
	cfg, err := config.ReportConfig(readFixture(t, configPath))
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	stages := application.StageExecutor{
		Preparer: config.Preparer{Config: cfg, Stderr: &stderr},
		Evidence: &acquisition.Service{
			ConfigPath: configPath, Root: root,
			Options: cfg.RunOptions(), Policy: cfg.PolicySnapshot(),
			Runner: toolrun.New(), Stderr: &stderr,
		},
		Stderr: &stderr,
	}
	resp, err := application.Service{Stages: stages}.Execute(context.Background(), application.Request{
		ConfigSource: configPath, BundleDir: root, JSON: true,
	})
	if err != nil {
		t.Fatalf("Execute: %v\nstderr:\n%s", err, stderr.String())
	}
	doc := resp.Document

	for _, name := range []string{"cycle", "blast_radius"} {
		i := slices.IndexFunc(doc.Metrics, func(m report.MetricResult) bool { return m.Name == name })
		if i < 0 || doc.Metrics[i].Band == string(report.ScoreBandNA) {
			t.Errorf("metric %s missing or not measured: %+v", name, doc.Metrics)
		}
	}
	balance := slices.IndexFunc(doc.Score.Dimensions, func(d report.Dimension) bool { return d.Name == report.DimCouplingBalance })
	if balance < 0 || doc.Score.Dimensions[balance].Band == report.ScoreBandNA {
		t.Errorf("coupling_balance not measured: %+v", doc.Score.Dimensions)
	}
	// a uses b/api, b's public part, and b itself, past it. With no internal
	// glob the Go type evidence decides past public: a call is functional, as
	// in the check, not intrusive.
	if e := doc.ClassifiedEdges; e == nil || e.ByStrength["contract"] != 1 || e.ByStrength["functional"] != 1 || e.ByStrength["intrusive"] != 0 {
		t.Errorf("classified edges %+v: want one contract use of public and one functional use past it", e)
	}

	for _, f := range doc.Findings {
		if f.Kind == report.FindingKindGate {
			t.Errorf("gate result %s/%s: %s", f.Kind, f.RuleID, f.Why)
		}
	}
	// The distributed-monolith rule still evaluates in warn mode and may say
	// that it abstains (task 34 removes it). It never blocks: only fail mode
	// with a comparable reference sets the verdict to fail.
	if doc.Summary.GateFindings != 0 {
		t.Errorf("gate findings %d: want none", doc.Summary.GateFindings)
	}
	if resp.Outcome == application.OutcomeFail || doc.Verdict == report.VerdictFail {
		t.Errorf("outcome %q, verdict %q: the report never fails", resp.Outcome, doc.Verdict)
	}
}
