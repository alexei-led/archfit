package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/assessment/metrics"
	"github.com/alexei-led/archfit/internal/assessment/rules"
	signal "github.com/alexei-led/archfit/internal/assessment/signals"
	"github.com/alexei-led/archfit/internal/baseline"
	"github.com/alexei-led/archfit/internal/config"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	goextract "github.com/alexei-led/archfit/internal/extract/golang"
	"github.com/alexei-led/archfit/internal/model/module"
	"github.com/alexei-led/archfit/internal/policy"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/view"
)

// goldenFixtureRoot returns the absolute path to testdata/golang, which contains
// real .go files that go/packages can load (go/packages shells out to go list).
func goldenFixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file is .../internal/analysispipeline/golden_test.go → go up one dir to internal testdata
	root := filepath.Join(filepath.Dir(file), "..", "testdata", "golang")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	return abs
}

// goldenConfig builds a ClassifyConfig, rules slice, and metrics slice that
// match the testdata/golang fixture (two modules a and b, forbidden dependency
// from a into b's internal package).
func goldenConfig() (view.ClassifyConfig, []rules.Rule, []metrics.Metric) {
	modules := map[string]module.ModuleDef{
		"a": {
			Paths:    []string{globModuleA},
			Public:   []string{globModuleA},
			Internal: []string{},
		},
		"b": {
			Paths:    []string{globModuleB},
			Public:   []string{globModuleB},
			Internal: []string{globModuleBInternal},
		},
	}

	cfg := config.Config{
		Version: 1,
		Modules: modules,
		Rules: []view.RuleDef{
			{
				ID:   "no_internal_access",
				Type: "forbidden_dependency",
				Gate: gateFail,
				From: globModuleA,
				To:   globModuleBInternal,
			},
		},
	}

	classifyCfg := cfg.ForClassify()
	rs, err := rules.New(cfg.ForRules())
	if err != nil {
		panic("goldenConfig: " + err.Error())
	}
	ms := metrics.New(cfg.Metrics)
	return classifyCfg, rs, ms
}

// TestGolden_DoubleRun runs the owner stages twice with identical inputs against the
// testdata/golang fixture and asserts byte-identical JSON-encoded output.
// This is CI gate 2: determinism of the full pipeline.
func TestGolden_DoubleRun(t *testing.T) {
	fixtureRoot := goldenFixtureRoot(t)

	classifyCfg, rs, ms := goldenConfig()

	extractor := goextract.New(view.ExtractConfig{})
	base := baseline.Baseline{SchemaVersion: baseline.SchemaVersion}
	// Fixed timestamp — any wall-clock source would break determinism.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := scope.Scope{Root: fixtureRoot, Mode: scope.ModeFull}

	runOnce := func() []byte {
		t.Helper()
		diag, err := RunStages(
			context.Background(),
			StageInput{
				Mode:        Mode{Full: true},
				Scope:       s,
				Policy:      policyForClassify(classifyCfg),
				Extractors:  []evidenceports.Extractor{extractor},
				Patterns:    evidenceports.NopPatternProvider{},
				Resolver:    evidenceports.NopSymbolResolver{},
				PatternCfg:  view.PatternConfig{},
				Rules:       rs,
				Metrics:     ms,
				Accepted:    base,
				BaseMetrics: base.Metrics,
				Signals:     signal.RunSignals{},
				Now:         now,
			},
		)
		if err != nil {
			t.Fatalf("RunStages: %v", err)
		}
		var buf bytes.Buffer
		if encErr := json.NewEncoder(&buf).Encode(diag); encErr != nil {
			t.Fatalf("encode: %v", encErr)
		}
		return buf.Bytes()
	}

	first := runOnce()
	second := runOnce()

	if !bytes.Equal(first, second) {
		t.Errorf("double-run outputs differ:\nfirst:  %s\nsecond: %s", first, second)
	}

	// Sanity check: the output is non-empty valid JSON.
	if len(first) == 0 {
		t.Error("output is empty")
	}
	var check map[string]any
	if err := json.Unmarshal(first, &check); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

const (
	globModuleA         = "pkg/a/**"
	globModuleB         = "pkg/b/**"
	globModuleBInternal = "pkg/b/internal/**"
)

func policyForClassify(cfg view.ClassifyConfig) policy.PolicySnapshot {
	owners := make(map[string]string)
	for name, def := range cfg.Modules {
		if def.Owner != "" {
			owners[name] = def.Owner
		}
	}
	topology := policy.TopologyView{Modules: cfg.Modules, Layers: cfg.Layers, ModuleMap: cfg.ModuleMap, ExternalSystems: cfg.ExternalSystems, ExplicitOwners: cfg.ExplicitOwners}
	return policy.New(topology, policy.RelationshipPolicy{MinimumSeverity: cfg.BCAdvisoryMinSeverity, VolatilityCascadeEnabled: cfg.VolatilityCascadeEnabled, DuplicatedKnowledge: cfg.DuplicatedKnowledgePolicy}, policy.AssessmentPolicy{Topology: topology}, policy.GatePolicy{}, owners, nil)
}
