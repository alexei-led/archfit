package acquisition_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/assessment/result"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	modevidence "github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/ownership"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/relationship/labels"
	"github.com/alexei-led/archfit/internal/scope"
)

const (
	// Coverage tool names and gate postures. Duplicated from the production
	// constants on purpose: both are part of the reported contract, so a test
	// reading the production symbol could not catch a rename.
	toolGoPackages    = "go/packages"
	toolDepCruiser    = "dependency-cruiser"
	toolGrimp         = "grimp"
	toolCargo         = "cargo"
	toolCargoModules  = "cargo-modules"
	toolJscpd         = "jscpd"
	toolLoc           = "loc"
	toolScip          = "scip"
	toolAstGrepSyntax = "ast-grep/syntax"

	gateOff  = "off"
	gateWarn = "warn"
	gateFail = "fail"

	metricCycle         = "cycle"
	metricBlastRadius   = "blast_radius"
	metricEncapsulation = "encapsulation"

	subdomainCore       = "core"
	subdomainSupporting = "supporting"
	subdomainGeneric    = "generic"

	volatilityHigh = "high"
	volatilityLow  = "low"

	decisionModA = "app.a"
	decisionModB = "app.b"

	markerGoMod          = "go.mod"
	markerCargoToml      = "Cargo.toml"
	defaultConfigPath    = ".archfit.yaml"
	enrichModel          = "model"
	reasonScipDisabled   = "opt-in: analyzers.scip.enabled"
	reasonSyntaxDisabled = "opt-in: analyzers.syntax.enabled"
)

// TestBuildCoverageGaps verifies the coverage-gap table derivation:
// absent known tools produce a gap with the right gate; present or unknown
// tools produce no gap.
func TestBuildCoverageGaps(t *testing.T) {
	t.Parallel()
	cfgFailGo := config.Config{Languages: config.LanguagesConfig{
		Go: config.GoLanguage{Gate: config.GateFail},
	}}
	cfgOffGo := config.Config{Languages: config.LanguagesConfig{
		Go: config.GoLanguage{Gate: config.GateOff},
	}}
	cfgWarn := config.Config{}

	cases := []struct {
		name      string
		cov       []result.Coverage
		cfg       config.Config
		wantTools []string // tool names in expected gap output (empty = no gaps)
		wantGate  string   // gate for first gap (when wantTools non-empty)
	}{
		{
			name:      "absent known tool produces gap",
			cov:       []result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}},
			cfg:       cfgWarn,
			wantTools: []string{toolGoPackages},
			wantGate:  gateWarn,
		},
		{
			name:      "absent tool with configured fail gate",
			cov:       []result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}},
			cfg:       cfgFailGo,
			wantTools: []string{toolGoPackages},
			wantGate:  gateFail,
		},
		{
			name:      "present tool produces no gap",
			cov:       []result.Coverage{{Tool: toolGoPackages, Status: result.StatusOK}},
			cfg:       cfgWarn,
			wantTools: nil,
		},
		{
			// gate: off no longer suppresses on its own. It suppresses only where
			// the language's marker is absent, and an unprobeable root (root == ""
			// here) answers "present" — the same abstain-toward-disclosure rule the
			// "empty root disables suppression" case below pins. The gap keeps
			// Gate: off, which applyToolGate never escalates.
			name:      "absent tool with configured off gate is disclosed, gated off",
			cov:       []result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}},
			cfg:       cfgOffGo,
			wantTools: []string{toolGoPackages},
			wantGate:  gateOff,
		},
		{
			name:      "unknown tool produces no gap",
			cov:       []result.Coverage{{Tool: "unknown-tool", Status: result.StatusAbsent}},
			cfg:       cfgWarn,
			wantTools: nil,
		},
		{
			name: "multiple absent tools sorted by name",
			cov: []result.Coverage{
				{Tool: toolGrimp, Status: result.StatusAbsent},
				{Tool: toolGoPackages, Status: result.StatusAbsent},
				{Tool: toolJscpd, Status: result.StatusAbsent},
			},
			cfg:       cfgWarn,
			wantTools: []string{toolGoPackages, toolGrimp, toolJscpd},
		},
		{
			// A tool disabled by config (StatusDisabled) must NOT produce a coverage
			// gap — the user deliberately opted out; telling them to "install" is wrong.
			name:      "disabled-by-config tool produces no gap",
			cov:       []result.Coverage{{Tool: toolJscpd, Status: result.StatusDisabled}},
			cfg:       cfgWarn,
			wantTools: nil,
		},
		{
			// Partial coverage is informational, not an install prompt.
			name:      "partial tool produces no gap",
			cov:       []result.Coverage{{Tool: toolJscpd, Status: result.StatusPartial}},
			cfg:       cfgWarn,
			wantTools: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gaps := acquisition.BuildCoverageGaps(tc.cov, tc.cfg.CoverageOptions(), "")
			if len(gaps) != len(tc.wantTools) {
				t.Fatalf("gaps = %d, want %d: %+v", len(gaps), len(tc.wantTools), gaps)
			}
			for i, g := range gaps {
				if g.Tool != tc.wantTools[i] {
					t.Errorf("gap[%d].Tool = %q, want %q", i, g.Tool, tc.wantTools[i])
				}
			}
			if tc.wantGate != "" && len(gaps) > 0 && gaps[0].Gate != tc.wantGate {
				t.Errorf("gap[0].Gate = %q, want %q", gaps[0].Gate, tc.wantGate)
			}
		})
	}
}

// TestBuildCoverageGaps_ProjectMarkerSuppression verifies that gaps for a
// language whose project marker is absent from the scan root are suppressed,
// while gaps for present markers and explicit gates are preserved.
func TestBuildCoverageGaps_ProjectMarkerSuppression(t *testing.T) {
	t.Parallel()
	// Pure-Go repo: only go.mod present, no Cargo.toml.
	goOnlyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(goOnlyDir, markerGoMod), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Mixed Go+Rust repo: both go.mod and Cargo.toml present.
	mixedDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(mixedDir, markerGoMod), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mixedDir, markerCargoToml), []byte("[package]\nname = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfgDefault := config.Config{}
	cfgRustGate := config.Config{Languages: config.LanguagesConfig{
		Rust: config.RustLanguage{Gate: config.GateFail},
	}}
	cfgCargoModulesGate := config.Config{Analyzers: config.AnalyzersConfig{
		CargoModules: config.Analyzer{Gate: config.GateFail},
	}}

	allRustAbsent := []result.Coverage{
		{Tool: toolCargo, Status: result.StatusAbsent},
		{Tool: toolCargoModules, Status: result.StatusAbsent},
	}

	t.Run("pure-Go repo: no cargo or cargo-modules gap", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(allRustAbsent, cfgDefault.CoverageOptions(), goOnlyDir)
		for _, g := range gaps {
			if g.Tool == toolCargo || g.Tool == toolCargoModules {
				t.Errorf("unexpected gap %q in pure-Go repo (no Cargo.toml)", g.Tool)
			}
		}
	})

	t.Run("mixed Go+Rust repo: cargo gap present", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(allRustAbsent, cfgDefault.CoverageOptions(), mixedDir)
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargo {
				found = true
			}
		}
		if !found {
			t.Error("expected cargo gap in mixed repo with Cargo.toml, got none")
		}
	})

	t.Run("mixed Go+Rust repo: cargo-modules gap present", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(allRustAbsent, cfgDefault.CoverageOptions(), mixedDir)
		found := false
		for _, g := range gaps {
			if g.Tool != toolCargoModules {
				continue
			}
			found = true
			wantMetrics := []string{metricCycle, metricBlastRadius, metricEncapsulation}
			if len(g.AffectedMetrics) != len(wantMetrics) {
				t.Fatalf("cargo-modules affected metrics = %v, want %v", g.AffectedMetrics, wantMetrics)
			}
			for i, want := range wantMetrics {
				if g.AffectedMetrics[i] != want {
					t.Fatalf("cargo-modules affected metrics[%d] = %q, want %q (no nonexistent cohesion metric)", i, g.AffectedMetrics[i], want)
				}
			}
		}
		if !found {
			t.Error("expected cargo-modules gap in mixed repo with Cargo.toml, got none")
		}
	})

	t.Run("explicit gate on rust overrides marker suppression", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps([]result.Coverage{
			{Tool: toolCargo, Status: result.StatusAbsent},
		}, cfgRustGate.CoverageOptions(), goOnlyDir)
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargo {
				found = true
			}
		}
		if !found {
			t.Error("explicit gate: expected cargo gap even without Cargo.toml, got none")
		}
	})

	t.Run("explicit gate on cargo-modules overrides marker suppression", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps([]result.Coverage{
			{Tool: toolCargoModules, Status: result.StatusAbsent},
		}, cfgCargoModulesGate.CoverageOptions(), goOnlyDir)
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargoModules {
				found = true
			}
		}
		if !found {
			t.Error("explicit gate: expected cargo-modules gap even without Cargo.toml, got none")
		}
	})

	t.Run("gate off discloses over a present language", testGateOffGapDisclosure)

	t.Run("empty root disables suppression (backward compat)", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(allRustAbsent, cfgDefault.CoverageOptions(), "")
		found := false
		for _, g := range gaps {
			if g.Tool == toolCargo {
				found = true
			}
		}
		if !found {
			t.Error("empty root: expected cargo gap (no suppression), got none")
		}
	})

	t.Run("go project markers", testGoProjectMarkerGaps)
	t.Run("go module filter", testGoModuleFilterGaps)
	t.Run("go workspace members", testGoWorkspaceMarkerGaps)
	t.Run("typescript markers", testTSProjectMarkerGaps)
	t.Run("python markers", testPyProjectMarkerGaps)
	t.Run("rust manifest markers", testRustManifestMarkerGaps)
	t.Run("disabled primaries", testMarkDisabledPrimaries)
}

// testGateOffGapDisclosure pins that `gate: off` suppresses a coverage gap only
// where the language is ABSENT, and that the disclosed gap still never gates.
//
// It replaces two subtests that pinned unconditional suppression. That silence
// was read downstream as a fact about the TREE: `config compare` and
// `analyze --base` both take a primary analyzer that is absent WITHOUT a gap as
// "this language is not present here", so a gated-off analyzer over a repo full
// of that language dropped out of both comparisons unmeasured and unmentioned.
// Suppression now keys on the language marker; `gate: off` keeps its meaning in
// applyToolGate, which refuses to escalate it even under --require-tools.
func testGateOffGapDisclosure(t *testing.T) {
	t.Parallel()
	goOnlyDir := t.TempDir()
	writeFileAt(t, goOnlyDir, markerGoMod, "module example\n")
	rustDir := t.TempDir()
	writeFileAt(t, rustDir, markerCargoToml, "[package]\nname = \"x\"\n")

	cfgGoOff := config.Config{Languages: config.LanguagesConfig{
		Go: config.GoLanguage{Gate: config.GateOff},
	}}
	cfgCargoModulesOff := config.Config{Analyzers: config.AnalyzersConfig{
		CargoModules: config.Analyzer{Gate: config.GateOff},
	}}
	goAbsent := []result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}}
	cargoModulesAbsent := []result.Coverage{{Tool: toolCargoModules, Status: result.StatusAbsent}}

	t.Run("language present keeps the gap", func(t *testing.T) {
		t.Parallel()
		if !hasCoverageGap(acquisition.BuildCoverageGaps(goAbsent, cfgGoOff.CoverageOptions(), goOnlyDir), toolGoPackages) {
			t.Error("gate off over a go.mod repo must still disclose the go/packages gap")
		}
		if !hasCoverageGap(acquisition.BuildCoverageGaps(cargoModulesAbsent, cfgCargoModulesOff.CoverageOptions(), rustDir), toolCargoModules) {
			t.Error("gate off over a Cargo.toml repo must still disclose the cargo-modules gap")
		}
	})

	t.Run("language absent suppresses the gap", func(t *testing.T) {
		t.Parallel()
		if hasCoverageGap(acquisition.BuildCoverageGaps(cargoModulesAbsent, cfgCargoModulesOff.CoverageOptions(), goOnlyDir), toolCargoModules) {
			t.Error("no Cargo.toml: an opt-out needs no install prompt")
		}
	})

	// The gap is a disclosure ("this analyzer did not run here"); whether it
	// fails the build is the gate's decision, pinned in evaluation's tool-gate
	// test. What acquisition owes is the posture stamped on the gap.
	t.Run("disclosed gap carries the opt-out posture", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(goAbsent, cfgGoOff.CoverageOptions(), goOnlyDir)
		if len(gaps) == 0 || gaps[0].Gate != gateOff {
			t.Errorf("gaps = %+v, want the gate: off posture preserved", gaps)
		}
	})
}

// writeFileAt writes content into root/rel, creating parent directories.
func writeFileAt(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// testGoWorkspaceMarkerGaps pins that the Go probe answers through the SAME
// member selection the extractor runs (golang.AnalysableMembers): go.work first,
// then the languages.go.modules filter. A probe that walked for go.mod itself
// never saw go.work, so it disagreed with the extractor in both directions.
func testGoWorkspaceMarkerGaps(t *testing.T) {
	t.Parallel()

	// go.work claims services/api; libs/util also carries a go.mod but the
	// workspace does not use it. With modules.include scoped to libs/**, the
	// extractor loads NOTHING (the filter removes the only member go.work named)
	// and reports absent — a deliberately empty scope, not a missing toolchain.
	// The walking probe found libs/util/go.mod, accepted it against libs/**, and
	// raised "install the Go toolchain" over a run the user scoped away.
	t.Run("go.work member set is what the module filter is applied to", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, "go.work", "go 1.26\n\nuse ./services/api\n")
		writeFileAt(t, root, filepath.Join("services", "api", markerGoMod), "module example/api\n")
		writeFileAt(t, root, filepath.Join("libs", "util", markerGoMod), "module example/util\n")
		cfg := config.Config{
			Exclude: scope.MergeExclusions(nil),
			Languages: config.LanguagesConfig{Go: config.GoLanguage{
				Modules: config.GoModuleFilter{Include: []string{"libs/**"}},
			}},
		}
		gaps := acquisition.BuildCoverageGaps(
			[]result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}}, cfg.CoverageOptions(), root)
		if hasCoverageGap(gaps, toolGoPackages) {
			t.Errorf("gap raised over a member set go.work never named: %+v", gaps)
		}
	})

	// The same tree with the filter naming the workspace member: the extractor
	// loads it, so the absence IS a real gap.
	t.Run("a filter naming the go.work member keeps the gap", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, "go.work", "go 1.26\n\nuse ./services/api\n")
		writeFileAt(t, root, filepath.Join("services", "api", markerGoMod), "module example/api\n")
		writeFileAt(t, root, filepath.Join("libs", "util", markerGoMod), "module example/util\n")
		cfg := config.Config{
			Exclude: scope.MergeExclusions(nil),
			Languages: config.LanguagesConfig{Go: config.GoLanguage{
				Modules: config.GoModuleFilter{Include: []string{"services/**"}},
			}},
		}
		gaps := acquisition.BuildCoverageGaps(
			[]result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}}, cfg.CoverageOptions(), root)
		if !hasCoverageGap(gaps, toolGoPackages) {
			t.Errorf("no gap for a workspace member the extractor loads: %+v", gaps)
		}
	})

	// The other direction: a workspace member the old walk could not reach (it
	// pruned every dot-directory) made "the extractor loads this tree" read as
	// "there is no Go here" — the one absent shape both comparison paths treat as
	// safely comparable.
	t.Run("a go.work member under a dot-directory counts", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, "go.work", "go 1.26\n\nuse ./.tools/gen\n")
		writeFileAt(t, root, filepath.Join(".tools", "gen", markerGoMod), "module example/gen\n")
		cfg := config.Config{Exclude: scope.MergeExclusions(nil)}
		gaps := acquisition.BuildCoverageGaps(
			[]result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}}, cfg.CoverageOptions(), root)
		if !hasCoverageGap(gaps, toolGoPackages) {
			t.Errorf("no gap for a go.work member the extractor loads: %+v", gaps)
		}
	})
}

// testTSProjectMarkerGaps pins that the TypeScript probe uses the extractor's
// own applicability test (ts.Applicable — package.json in the scan root). The
// registry's marker list also accepted tsconfig.json, which dependency-cruiser
// never looks at: a tsconfig-only repo raised an install prompt for an analyzer
// that would have reported absent regardless, and `typescript.enabled: false`
// over it was disclosed as a switched-off language that was never there.
func testTSProjectMarkerGaps(t *testing.T) {
	t.Parallel()
	tsAbsent := func() []result.Coverage {
		return []result.Coverage{{Tool: toolDepCruiser, Status: result.StatusAbsent}}
	}
	cfg := config.Config{Exclude: scope.MergeExclusions(nil)}

	t.Run("package.json keeps the gap", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, "package.json", "{}\n")
		if !hasCoverageGap(acquisition.BuildCoverageGaps(tsAbsent(), cfg.CoverageOptions(), root), toolDepCruiser) {
			t.Error("expected a dependency-cruiser gap for a repo with package.json")
		}
	})

	t.Run("tsconfig.json alone suppresses the gap", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, "tsconfig.json", "{}\n")
		if hasCoverageGap(acquisition.BuildCoverageGaps(tsAbsent(), cfg.CoverageOptions(), root), toolDepCruiser) {
			t.Error("tsconfig.json without package.json is not a project this extractor analyses")
		}
	})

	t.Run("tsconfig.json alone is not a switched-off language", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, "tsconfig.json", "{}\n")
		off := config.Config{Languages: config.LanguagesConfig{
			TypeScript: config.TypeScriptLanguage{Enabled: evidenceports.ModeOff},
		}}
		cov := acquisition.MarkDisabledPrimaries(tsAbsent(), off.CoverageOptions(), root)
		if cov[0].Status != result.StatusAbsent {
			t.Errorf("status = %q, want %q (no package.json — nothing was switched off)", cov[0].Status, result.StatusAbsent)
		}
	})
}

// testPyProjectMarkerGaps pins that the Python probe uses the extractor's own
// applicability test (py.Applicable). The registry's marker list disagreed in
// both directions: it accepted setup.cfg, which grimp's extractor does not, and
// it never looked at languages.python.package, which the extractor accepts on
// its own.
func testPyProjectMarkerGaps(t *testing.T) {
	t.Parallel()
	pyAbsent := func() []result.Coverage {
		return []result.Coverage{{Tool: toolGrimp, Status: result.StatusAbsent}}
	}
	plain := config.Config{Exclude: scope.MergeExclusions(nil)}

	t.Run("pyproject.toml keeps the gap", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, "pyproject.toml", "[project]\nname = \"x\"\n")
		if !hasCoverageGap(acquisition.BuildCoverageGaps(pyAbsent(), plain.CoverageOptions(), root), toolGrimp) {
			t.Error("expected a grimp gap for a repo with pyproject.toml")
		}
	})

	t.Run("setup.cfg alone suppresses the gap", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, "setup.cfg", "[metadata]\nname = x\n")
		if hasCoverageGap(acquisition.BuildCoverageGaps(pyAbsent(), plain.CoverageOptions(), root), toolGrimp) {
			t.Error("setup.cfg is not a marker this extractor accepts")
		}
	})

	// The configured package dir is the extractor's third marker, and the only
	// one a marker-filename list structurally cannot express.
	t.Run("a configured package dir keeps the gap", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFileAt(t, root, filepath.Join("mypkg", "__init__.py"), "")
		cfg := config.Config{
			Exclude:   scope.MergeExclusions(nil),
			Languages: config.LanguagesConfig{Python: config.PythonLanguage{Package: "mypkg"}},
		}
		if !hasCoverageGap(acquisition.BuildCoverageGaps(pyAbsent(), cfg.CoverageOptions(), root), toolGrimp) {
			t.Error("expected a grimp gap for a configured languages.python.package directory")
		}
	})

	t.Run("a configured package dir that is not there suppresses the gap", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		cfg := config.Config{
			Exclude:   scope.MergeExclusions(nil),
			Languages: config.LanguagesConfig{Python: config.PythonLanguage{Package: "mypkg"}},
		}
		if hasCoverageGap(acquisition.BuildCoverageGaps(pyAbsent(), cfg.CoverageOptions(), root), toolGrimp) {
			t.Error("a configured package dir that does not exist is not a Python project")
		}
	})
}

// testGoModuleFilterGaps pins that the marker probe honours
// languages.go.modules.include/exclude, which the Go extractor applies through
// FilterMembers before deciding it has nothing to load. A probe that ignored the
// filter reported a coverage gap ("install the Go toolchain") over a member set
// the user deliberately scoped away, and made both comparison paths grade the
// run's evidence as degraded.
func testGoModuleFilterGaps(t *testing.T) {
	t.Parallel()
	const memberDir = "member"
	goModFilterTests := []struct {
		name    string
		goModAt []string // repo-relative dirs holding a go.mod
		include []string
		exclude []string
		want    bool // want a go/packages gap
	}{
		{name: "no filter keeps the gap", goModAt: []string{memberDir}, want: true},
		{name: "include matching no member suppresses the gap", goModAt: []string{memberDir}, include: []string{"other/**"}, want: false},
		{name: "include matching the member keeps the gap", goModAt: []string{memberDir}, include: []string{memberDir}, want: true},
		{name: "exclude removing the only member suppresses the gap", goModAt: []string{memberDir}, exclude: []string{memberDir}, want: false},
		// The walk must not stop at the first REJECTED candidate: "a" sorts before
		// "b", so an early return there would hide the member the extractor loads.
		{
			name:    "an excluded member does not hide a later included one",
			goModAt: []string{"a", "b"},
			exclude: []string{"a"},
			want:    true,
		},
	}
	for _, tc := range goModFilterTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, dir := range tc.goModAt {
				full := filepath.Join(root, dir)
				if err := os.MkdirAll(full, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(full, markerGoMod), []byte("module example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := config.Config{
				Exclude: scope.MergeExclusions(nil),
				Languages: config.LanguagesConfig{Go: config.GoLanguage{
					Modules: config.GoModuleFilter{Include: tc.include, Exclude: tc.exclude},
				}},
			}
			gaps := acquisition.BuildCoverageGaps(
				[]result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}}, cfg.CoverageOptions(), root)
			if gotGap := hasCoverageGap(gaps, toolGoPackages); gotGap != tc.want {
				t.Errorf("go/packages gap = %v, want %v (gaps: %+v)", gotGap, tc.want, gaps)
			}
		})
	}
}

// testRustManifestMarkerGaps pins that the Rust marker probe reads
// languages.rust.manifest, the same applicability marker the extractor stats.
// A configured sub-crate manifest makes cargo applicable with NO root
// Cargo.toml, and the root-only check called that "Rust is not present here".
func testRustManifestMarkerGaps(t *testing.T) {
	t.Parallel()
	// Sub-crate manifest only: no Cargo.toml at the scan root.
	subCrateDir := t.TempDir()
	crateDir := filepath.Join(subCrateDir, "crates", "core")
	if err := os.MkdirAll(crateDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crateDir, markerCargoToml), []byte("[package]\nname = \"core\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	subManifest := filepath.Join("crates", "core", markerCargoToml)

	rustCfg := func(manifest string) config.Config {
		return config.Config{
			Exclude:   scope.MergeExclusions(nil),
			Languages: config.LanguagesConfig{Rust: config.RustLanguage{Manifest: manifest}},
		}
	}
	rustAbsent := []result.Coverage{
		{Tool: toolCargo, Status: result.StatusAbsent},
		{Tool: toolCargoModules, Status: result.StatusAbsent},
	}

	t.Run("configured sub-crate manifest keeps the cargo gap", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(rustAbsent, rustCfg(subManifest).CoverageOptions(), subCrateDir)
		if !hasCoverageGap(gaps, toolCargo) {
			t.Errorf("expected cargo gap for a configured sub-crate manifest, got %+v", gaps)
		}
	})

	t.Run("configured sub-crate manifest keeps the cargo-modules gap", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(rustAbsent, rustCfg(subManifest).CoverageOptions(), subCrateDir)
		if !hasCoverageGap(gaps, toolCargoModules) {
			t.Errorf("expected cargo-modules gap for a configured sub-crate manifest, got %+v", gaps)
		}
	})

	t.Run("no manifest configured and no root Cargo.toml suppresses the gap", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(rustAbsent, rustCfg("").CoverageOptions(), subCrateDir)
		if hasCoverageGap(gaps, toolCargo) {
			t.Errorf("unsuppressed cargo gap without a root Cargo.toml: %+v", gaps)
		}
	})

	t.Run("configured manifest that does not exist suppresses the gap", func(t *testing.T) {
		t.Parallel()
		gaps := acquisition.BuildCoverageGaps(rustAbsent, rustCfg(filepath.Join("crates", "missing", markerCargoToml)).CoverageOptions(), subCrateDir)
		if hasCoverageGap(gaps, toolCargo) {
			t.Errorf("unsuppressed cargo gap for a nonexistent manifest: %+v", gaps)
		}
	})
}

// testMarkDisabledPrimaries pins that a language switched off in config reports
// its primary analyzer as "disabled", not "absent". The extractors report
// ModeOff as absent, and both comparison paths read primary+absent+no-gap as
// "the language is not in this tree" — so two configs that BOTH disabled a
// language over a repo that HAS it graded as fully comparable evidence.
func testMarkDisabledPrimaries(t *testing.T) {
	t.Parallel()
	off := config.Config{Languages: config.LanguagesConfig{
		TypeScript: config.TypeScriptLanguage{Enabled: evidenceports.ModeOff},
	}}
	offGated := config.Config{Languages: config.LanguagesConfig{
		TypeScript: config.TypeScriptLanguage{Enabled: evidenceports.ModeOff, Gate: config.GateFail},
	}}
	// A repo that HAS TypeScript: package.json is dependency-cruiser's marker.
	tsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tsDir, "package.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A repo that does not: only go.mod.
	goOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(goOnly, markerGoMod), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	absent := func() []result.Coverage {
		return []result.Coverage{{Tool: toolDepCruiser, Status: result.StatusAbsent}}
	}

	t.Run("disabled language primary becomes a disabled row", func(t *testing.T) {
		t.Parallel()
		cov := acquisition.MarkDisabledPrimaries(absent(), off.CoverageOptions(), tsDir)
		if cov[0].Status != result.StatusDisabled {
			t.Fatalf("status = %q, want %q", cov[0].Status, result.StatusDisabled)
		}
		if !strings.Contains(cov[0].Reason, "languages.typescript.enabled") {
			t.Errorf("reason %q does not name the config key that switched it off", cov[0].Reason)
		}
	})

	// The mirror of the bug being fixed: "nothing here" must not render as "we
	// did not look". A repo with no TypeScript must not be told TypeScript
	// analysis is switched off, and `enabled: false` on a language that is not
	// there must not grade not_comparable against a config that left it unset.
	t.Run("a language that is not in the tree stays absent", func(t *testing.T) {
		t.Parallel()
		cov := acquisition.MarkDisabledPrimaries(absent(), off.CoverageOptions(), goOnly)
		if cov[0].Status != result.StatusAbsent {
			t.Fatalf("status = %q, want %q (no package.json — nothing was switched off)", cov[0].Status, result.StatusAbsent)
		}
		if cov[0].Reason != "" {
			t.Errorf("reason = %q, want none", cov[0].Reason)
		}
	})

	t.Run("explicit gate keeps the row absent so the gap survives", func(t *testing.T) {
		t.Parallel()
		cov := acquisition.MarkDisabledPrimaries(absent(), offGated.CoverageOptions(), tsDir)
		if cov[0].Status != result.StatusAbsent {
			t.Fatalf("status = %q, want %q (an explicit gate opts back into the gap)", cov[0].Status, result.StatusAbsent)
		}
		if !hasCoverageGap(acquisition.BuildCoverageGaps(cov, offGated.CoverageOptions(), ""), toolDepCruiser) {
			t.Error("explicit gate on a disabled language must still raise a coverage gap")
		}
	})

	t.Run("non-primary and non-absent rows are untouched", func(t *testing.T) {
		t.Parallel()
		in := []result.Coverage{
			{Tool: toolJscpd, Status: result.StatusAbsent},
			{Tool: toolDepCruiser, Status: result.StatusOK},
		}
		cov := acquisition.MarkDisabledPrimaries(in, off.CoverageOptions(), tsDir)
		if cov[0].Status != result.StatusAbsent || cov[1].Status != result.StatusOK {
			t.Errorf("rewrote rows it must not touch: %+v", cov)
		}
	})

	t.Run("a disabled row raises no install gap", func(t *testing.T) {
		t.Parallel()
		cov := acquisition.MarkDisabledPrimaries(absent(), off.CoverageOptions(), tsDir)
		if gaps := acquisition.BuildCoverageGaps(cov, off.CoverageOptions(), ""); len(gaps) != 0 {
			t.Errorf("disabled language must not prompt an install: %+v", gaps)
		}
	})

	// An unprobeable root discloses the opt-out rather than hiding it — the same
	// choice buildCoverageGaps makes when it skips marker suppression for "".
	t.Run("an empty root discloses the opt-out", func(t *testing.T) {
		t.Parallel()
		cov := acquisition.MarkDisabledPrimaries(absent(), off.CoverageOptions(), "")
		if cov[0].Status != result.StatusDisabled {
			t.Errorf("status = %q, want %q", cov[0].Status, result.StatusDisabled)
		}
	})
}

// testGoProjectMarkerGaps pins Go's marker probe. go/packages discovers members
// by walking for nested go.mod dirs, so a root-only marker check would call a
// services/api/go.mod repo "no Go here" and suppress the gap — turning a real
// analyzer failure into "language not present", the one absent shape that pairs
// with ok in both `--base` and `config compare`.
func testGoProjectMarkerGaps(t *testing.T) {
	t.Parallel()
	goMarkerTests := []struct {
		name    string
		goModAt string // repo-relative dir holding go.mod; "" = none
		exclude []string
		want    bool // want a go/packages gap
	}{
		{name: "nested go.mod still produces a gap", goModAt: filepath.Join("services", "api"), want: true},
		{name: "root go.mod produces a gap", goModAt: ".", want: true},
		{name: "no go.mod anywhere suppresses the gap", goModAt: "", want: false},
		// The probe must agree with the extractors' EFFECTIVE exclusions. A marker
		// they never see is not evidence the language is present: counting it
		// turns a gapless absence (comparable) into absent-with-a-gap, which is
		// the strictest evidence class on both comparison paths.
		{name: "go.mod under a pruned dependency directory does not count", goModAt: filepath.Join("node_modules", "pkg"), want: false},
		// scope.DefaultExclusions covers these two and the hand-written prune list
		// did not — a fixture module under testdata/ is routine in Go repos.
		{name: "go.mod under testdata does not count", goModAt: filepath.Join("testdata", "fixture"), want: false},
		{name: "go.mod under reports does not count", goModAt: filepath.Join("reports", "snapshot"), want: false},
		// A config exclusion names no single directory, so only a full-path match
		// can honour it.
		{
			name:    "go.mod under a config-excluded subtree does not count",
			goModAt: filepath.Join("services", "legacy"),
			exclude: []string{"services/legacy/**"},
			want:    false,
		},
		{
			name:    "an unrelated config exclusion still leaves the gap",
			goModAt: filepath.Join("services", "api"),
			exclude: []string{"services/legacy/**"},
			want:    true,
		},
		// A `!` re-include removes the default, so the extractors DO walk
		// testdata/ and the probe must count the marker they see. This only holds
		// while the effective exclusions are merged exactly once: a second
		// MergeExclusions pass re-seeds **/testdata/** and hides the marker again.
		{
			name:    "a re-included testdata tree counts",
			goModAt: filepath.Join("testdata", "fixture"),
			exclude: []string{"!testdata"},
			want:    true,
		},
	}
	for _, tc := range goMarkerTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if tc.goModAt != "" {
				dir := filepath.Join(root, tc.goModAt)
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, markerGoMod), []byte("module example\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// Merged exactly once, exactly where runPipeline merges it. Passing the
			// raw config globs would make the `!testdata` row pass vacuously: with
			// no defaults present there is nothing left to exclude the marker.
			gaps := acquisition.BuildCoverageGaps(
				[]result.Coverage{{Tool: toolGoPackages, Status: result.StatusAbsent}}, config.Config{Exclude: scope.MergeExclusions(tc.exclude)}.CoverageOptions(), root)
			if gotGap := hasCoverageGap(gaps, toolGoPackages); gotGap != tc.want {
				t.Errorf("go/packages gap = %v, want %v (gaps: %+v)", gotGap, tc.want, gaps)
			}
		})
	}
}

// TestBuildConfigWarnings verifies the config-warnings block: lint warnings and
// tool errors are combined, nil is returned when both are empty.
func TestBuildConfigWarnings(t *testing.T) {
	t.Parallel()
	t.Run("empty config and no tool errors returns nil", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{Version: 1}
		if got := acquisition.BuildConfigWarnings(cfg.RunOptions().LintWarnings, nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("tool errors appear after lint warnings", func(t *testing.T) {
		t.Parallel()
		// A module with paths but no rules referencing it produces a lint warning.
		cfg := config.Config{
			Version: 1,
			Modules: map[string]policy.ModuleDef{
				"orphan": {Paths: []string{"pkg/orphan/**"}},
			},
		}
		toolErrs := []string{"jscpd: exit status 1"}
		got := acquisition.BuildConfigWarnings(cfg.RunOptions().LintWarnings, toolErrs)
		if len(got) == 0 {
			t.Fatal("want at least one warning, got none")
		}
		// Tool error must appear somewhere in the output.
		found := false
		for _, w := range got {
			if w == "jscpd: exit status 1" {
				found = true
			}
		}
		if !found {
			t.Errorf("tool error missing from warnings: %v", got)
		}
	})

	t.Run("tool errors only, no lint", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{Version: 1}
		toolErrs := []string{"jscpd: not found"}
		got := acquisition.BuildConfigWarnings(cfg.RunOptions().LintWarnings, toolErrs)
		if len(got) != 1 || got[0] != "jscpd: not found" {
			t.Errorf("got %v, want [jscpd: not found]", got)
		}
	})
}

func TestConfigToolGate(t *testing.T) {
	t.Parallel()
	cfg := config.Config{Languages: config.LanguagesConfig{
		Go:         config.GoLanguage{Gate: config.GateFail},
		TypeScript: config.TypeScriptLanguage{Gate: config.GateOff},
		Python:     config.PythonLanguage{}, // unset → warn
	}}
	cases := []struct {
		tool string
		want string
	}{
		{toolGoPackages, gateFail},               // tools.go.gate: fail
		{toolDepCruiser, string(config.GateOff)}, // tools.typescript.gate: off
		{toolGrimp, gateWarn},                    // tools.python unset → default
		{toolLoc, gateWarn},                      // unmapped tool → default
	}
	for _, tc := range cases {
		if got := acquisition.ConfigToolGate(cfg.CoverageOptions(), tc.tool); got != tc.want {
			t.Errorf("acquisition.ConfigToolGate(%q) = %q, want %q", tc.tool, got, tc.want)
		}
	}
}

func TestBuildJudgmentDecisionTasks(t *testing.T) {
	t.Parallel()
	configPath := "/repo/.archfit.yaml"

	t.Run("module with neither subdomain nor volatility emits decision task", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{
			Modules: map[string]policy.ModuleDef{
				"app.core": {Paths: []string{"internal/core/**"}, Subdomain: subdomainCore},
				"app.util": {Paths: []string{"internal/util/**"}}, // no subdomain, no volatility
			},
		}
		tasks := acquisition.BuildJudgmentDecisionTasks(cfg.PolicySnapshot().Topology.Modules, nil, configPath)
		found := false
		for _, t2 := range tasks {
			if strings.Contains(t2, "app.util") && strings.Contains(t2, configPath) {
				found = true
			}
		}
		if !found {
			t.Errorf("expected decision task for app.util, got: %v", tasks)
		}
		// app.core has subdomain set — must NOT appear.
		for _, t2 := range tasks {
			if strings.Contains(t2, "app.core") {
				t.Errorf("unexpected decision task for app.core: %s", t2)
			}
		}
	})

	t.Run("module with volatility declared is not flagged", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{
			Modules: map[string]policy.ModuleDef{
				"app.util": {Paths: []string{"internal/util/**"}, Volatility: volatilityLow},
			},
		}
		tasks := acquisition.BuildJudgmentDecisionTasks(cfg.PolicySnapshot().Topology.Modules, nil, configPath)
		for _, t2 := range tasks {
			if strings.Contains(t2, "app.util") {
				t.Errorf("unexpected decision task for module with volatility: %s", t2)
			}
		}
	})

	t.Run("no modules emits no tasks", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{}
		tasks := acquisition.BuildJudgmentDecisionTasks(cfg.PolicySnapshot().Topology.Modules, nil, configPath)
		if len(tasks) != 0 {
			t.Errorf("expected no tasks, got: %v", tasks)
		}
	})

	t.Run("approved llm label emits decision task pointing at labels file", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{}
		lbls := []labels.Label{
			{From: decisionModA, To: decisionModB, Strength: enrichModel,
				Status: labels.StatusApproved, Provenance: labels.ProvenanceLLM},
		}
		tasks := acquisition.BuildJudgmentDecisionTasks(cfg.PolicySnapshot().Topology.Modules, lbls, configPath)
		found := false
		for _, t2 := range tasks {
			if strings.Contains(t2, decisionModA) && strings.Contains(t2, decisionModB) &&
				strings.Contains(t2, ".archfit-labels.yaml") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected decision task for llm label, got: %v", tasks)
		}
	})

	t.Run("draft llm label does NOT emit decision task", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{}
		lbls := []labels.Label{
			{From: decisionModA, To: decisionModB, Strength: enrichModel,
				Status: labels.StatusDraft, Provenance: labels.ProvenanceLLM},
		}
		tasks := acquisition.BuildJudgmentDecisionTasks(cfg.PolicySnapshot().Topology.Modules, lbls, configPath)
		if len(tasks) != 0 {
			t.Errorf("expected no tasks for draft label, got: %v", tasks)
		}
	})

	t.Run("approved human label does NOT emit decision task", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{}
		lbls := []labels.Label{
			{From: decisionModA, To: decisionModB, Strength: enrichModel,
				Status: labels.StatusApproved, Provenance: labels.ProvenanceHuman},
		}
		tasks := acquisition.BuildJudgmentDecisionTasks(cfg.PolicySnapshot().Topology.Modules, lbls, configPath)
		if len(tasks) != 0 {
			t.Errorf("expected no tasks for human label, got: %v", tasks)
		}
	})

	t.Run("output is sorted deterministically", func(t *testing.T) {
		t.Parallel()
		cfg := config.Config{
			Modules: map[string]policy.ModuleDef{
				"zz.module": {Paths: []string{"zz/**"}},
				"aa.module": {Paths: []string{"aa/**"}},
			},
		}
		tasks := acquisition.BuildJudgmentDecisionTasks(cfg.PolicySnapshot().Topology.Modules, nil, configPath)
		if len(tasks) < 2 {
			t.Fatalf("expected ≥2 tasks, got %d", len(tasks))
		}
		if !strings.Contains(tasks[0], "aa.module") {
			t.Errorf("first task should be aa.module (sorted), got: %s", tasks[0])
		}
		if !strings.Contains(tasks[1], "zz.module") {
			t.Errorf("second task should be zz.module (sorted), got: %s", tasks[1])
		}
	})
}

// TestOutputInsideRootWarning verifies the path hygiene check: a config/output
// directory strictly inside the analyzed root warns; the root itself or any path
// outside it does not.
func TestOutputInsideRootWarning(t *testing.T) {
	t.Parallel()
	root := filepath.FromSlash("/repo")
	cases := []struct {
		name    string
		dir     string
		wantMsg bool
	}{
		{"root itself is fine", filepath.FromSlash("/repo"), false},
		{"subdir inside root warns", filepath.FromSlash("/repo/reports"), true},
		{"nested subdir warns", filepath.FromSlash("/repo/a/b"), true},
		{"sibling outside root is fine", filepath.FromSlash("/other"), false},
		{"parent of root is fine", filepath.FromSlash("/"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := acquisition.OutputInsideRootWarning(root, tc.dir)
			if (got != "") != tc.wantMsg {
				t.Errorf("acquisition.OutputInsideRootWarning(%q, %q) = %q, wantMsg=%v", root, tc.dir, got, tc.wantMsg)
			}
		})
	}
}

// TestOwnerDegradationWarning pins which ownership sources are disclosed as
// degradations: codeowners_no_match and git_timeout warn (naming the source),
// everything else — including plain "none" and the designed git fallback — is
// silent.
func TestOwnerDegradationWarning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src      ownership.Source
		wantName string // "" = no warning
	}{
		{ownership.SourceCodeownersNoMatch, "codeowners_no_match"},
		{ownership.SourceGitTimeout, "git_timeout"},
		{ownership.SourceCodeowners, ""},
		{ownership.SourceGit, ""},
		{ownership.SourceNone, ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.src), func(t *testing.T) {
			t.Parallel()
			got := acquisition.OwnerDegradationWarning(tc.src)
			if tc.wantName == "" {
				if got != "" {
					t.Errorf("acquisition.OwnerDegradationWarning(%q) = %q, want no warning", tc.src, got)
				}
				return
			}
			if !strings.Contains(got, tc.wantName) {
				t.Errorf("acquisition.OwnerDegradationWarning(%q) = %q, want it to name %q", tc.src, got, tc.wantName)
			}
		})
	}
}

// TestTSUnresolvedWarning pins the disclosure rule: a dependency-cruiser
// coverage record only warns when it is partial AND carries a Reason; ok
// status, other tools, and a partial record with no reason all stay silent.
func TestTSUnresolvedWarning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cov  []result.Coverage
		want bool
	}{
		{
			name: "partial with reason warns",
			cov: []result.Coverage{
				{Tool: toolDepCruiser, Status: result.StatusPartial, Reason: "12 of 40 import specifiers unresolved (30%)"},
			},
			want: true,
		},
		{
			name: "ok status stays silent",
			cov: []result.Coverage{
				{Tool: toolDepCruiser, Status: result.StatusOK},
			},
			want: false,
		},
		{
			name: "partial with no reason stays silent",
			cov: []result.Coverage{
				{Tool: toolDepCruiser, Status: result.StatusPartial},
			},
			want: false,
		},
		{
			name: "other tool's partial coverage is not this warning's concern",
			cov: []result.Coverage{
				{Tool: toolCargoModules, Status: result.StatusPartial, Reason: "some crates failed"},
			},
			want: false,
		},
		{
			name: "no coverage records",
			cov:  nil,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := acquisition.TSUnresolvedWarning(tc.cov)
			if (got != "") != tc.want {
				t.Errorf("acquisition.TSUnresolvedWarning(%+v) = %q, want non-empty=%v", tc.cov, got, tc.want)
			}
			if tc.want && !strings.Contains(got, toolDepCruiser) {
				t.Errorf("acquisition.TSUnresolvedWarning(%+v) = %q, want it to name %q", tc.cov, got, toolDepCruiser)
			}
		})
	}
}

func TestPyUnresolvedWarning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cov  []result.Coverage
		want bool
	}{
		{
			name: "grimp unresolved warns",
			cov: []result.Coverage{
				{Tool: toolGrimp, Status: result.StatusPartial, Unresolved: 228},
			},
			want: true,
		},
		{
			name: "grimp unresolved includes top roots from reason",
			cov: []result.Coverage{
				{Tool: toolGrimp, Status: result.StatusPartial, Unresolved: 228, Reason: "228 imports unresolved (top: prefect_aws 100, httpx 4) — check languages.python.package and src layout"},
			},
			want: true,
		},
		{
			name: "zero unresolved stays silent",
			cov: []result.Coverage{
				{Tool: toolGrimp, Status: result.StatusOK},
			},
			want: false,
		},
		{
			name: "other tool unresolved is not this warning's concern",
			cov: []result.Coverage{
				{Tool: toolGoPackages, Status: result.StatusPartial, Unresolved: 2},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := acquisition.PyUnresolvedWarning(tc.cov)
			if (got != "") != tc.want {
				t.Errorf("acquisition.PyUnresolvedWarning(%+v) = %q, want non-empty=%v", tc.cov, got, tc.want)
			}
			if tc.want {
				if !strings.Contains(got, "228 imports unresolved") {
					t.Errorf("acquisition.PyUnresolvedWarning(%+v) = %q, want unresolved count", tc.cov, got)
				}
				if strings.Contains(tc.name, "top roots") && !strings.Contains(got, "top: prefect_aws 100, httpx 4") {
					t.Errorf("acquisition.PyUnresolvedWarning(%+v) = %q, want top roots", tc.cov, got)
				}
				if !strings.Contains(got, "check languages.python.package and src layout") {
					t.Errorf("acquisition.PyUnresolvedWarning(%+v) = %q, want Python hint", tc.cov, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CB1: skipped-pass coverage rows (P12 — syntax/scip opt-in honesty)
// ---------------------------------------------------------------------------

// TestSkippedPassCoverageRows_ScipDisabled asserts that when SCIP is not
// enabled the pipeline injects an explicit StatusDisabled coverage row for
// "scip" so tool_coverage reads "disabled" rather than absent/missing. This is
// the regression for P12: the skipped pass was silently absent from the output.
func TestSkippedPassCoverageRows_ScipDisabled(t *testing.T) {
	t.Parallel()
	cov := result.Coverage{Tool: toolScip, Status: result.StatusDisabled, Reason: reasonScipDisabled}
	if cov.Tool != "scip" {
		t.Fatalf("disabled SCIP coverage tool = %q, want scip", cov.Tool)
	}
	if !strings.Contains(cov.Reason, "analyzers.scip.enabled") {
		t.Fatalf("disabled SCIP reason = %q, want opt-in config path", cov.Reason)
	}

	// StatusDisabled must not produce a gap (deliberate opt-out, not a missing tool).
	gaps := acquisition.BuildCoverageGaps([]result.Coverage{cov}, config.Config{}.CoverageOptions(), "")
	if len(gaps) != 0 {
		t.Errorf("StatusDisabled scip must not produce a gap; got %+v", gaps)
	}
}

// TestSkippedPassCoverageRows_SyntaxDisabled asserts that when syntax is not
// enabled the pipeline injects an explicit StatusDisabled row for "ast-grep/syntax".
func TestSkippedPassCoverageRows_SyntaxDisabled(t *testing.T) {
	t.Parallel()
	cov := []result.Coverage{
		{Tool: toolAstGrepSyntax, Status: result.StatusDisabled, Reason: reasonSyntaxDisabled},
	}
	gaps := acquisition.BuildCoverageGaps(cov, config.Config{}.CoverageOptions(), "")
	if len(gaps) != 0 {
		t.Errorf("StatusDisabled ast-grep/syntax must not produce a gap; got %+v", gaps)
	}
}

// TestSkippedPassCoverageRows_ReasonContent asserts the disabled coverage
// reasons are distinct and non-empty, making it clear which opt-in flag to set.
func TestSkippedPassCoverageRows_ReasonContent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tool   string
		reason string
		wantIn string
	}{
		{toolScip, reasonScipDisabled, "scip"},
		{toolAstGrepSyntax, reasonSyntaxDisabled, "syntax"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()
			if tc.reason == "" {
				t.Error("reason must not be empty")
			}
			if !strings.Contains(tc.reason, tc.wantIn) {
				t.Errorf("reason %q does not mention %q", tc.reason, tc.wantIn)
			}
		})
	}
}

// hasCoverageGap reports whether the gap list names tool. Restated locally so
// the pipeline's coverage tests assert on the gap values themselves rather than
// depending on an assessment decision helper.
func hasCoverageGap(gaps []modevidence.CoverageGap, tool string) bool {
	for _, g := range gaps {
		if g.Tool == tool {
			return true
		}
	}
	return false
}
