package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	globalStateDensityName    = "global_state_density"
	globalStateDensityVersion = "global_state_density.v1"
	globalStateKind           = "global_state"
	globalStateActorMod       = "herdr::sync"
	globalStateSyncFile       = "sync/state.rs"
)

func TestGlobalStateDensity_NoFacts_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: nil}
	res := modularity.GlobalStateDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no global_state facts", res.Band)
	}
	if res.Name != globalStateDensityName {
		t.Errorf("Name = %q, want %s", res.Name, globalStateDensityName)
	}
}

func TestGlobalStateDensity_OnlyOtherKinds_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: kindFunctionSF, Module: modMyMod, File: fileARs},
		{Kind: kindStructStr, Module: modMyMod, File: fileARs},
	}}
	res := modularity.GlobalStateDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no global_state facts among other facts", res.Band)
	}
}

func TestGlobalStateDensity_SomeFacts_ReturnsTotalCount(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: globalStateKind, Module: globalStateActorMod, File: globalStateSyncFile, Language: langRust},
		{Kind: globalStateKind, Module: globalStateActorMod, File: globalStateSyncFile, Language: langRust},
		{Kind: globalStateKind, Module: "herdr::config", File: "config/global.rs", Language: langRust},
		{Kind: "function", Module: globalStateActorMod, File: globalStateSyncFile}, // ignored
	}}
	res := modularity.GlobalStateDensityMetric{}.Calculate(in)
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
	}
	if res.Value != 3 {
		t.Errorf("Value = %v, want 3 (total global_state count)", res.Value)
	}
	if res.Name != globalStateDensityName {
		t.Errorf("Name = %q, want %s", res.Name, globalStateDensityName)
	}
}

func TestGlobalStateDensity_NoModule_FallsBackToFile(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: globalStateKind, Module: "", File: fileMainRs, Language: langRust},
		{Kind: globalStateKind, Module: "", File: fileMainRs, Language: langRust},
	}}
	res := modularity.GlobalStateDensityMetric{}.Calculate(in)
	if res.Value != 2 {
		t.Errorf("Value = %v, want 2", res.Value)
	}
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
	}
}

func TestGlobalStateDensity_TestFileFact_IsCounted(t *testing.T) {
	// global_state_density intentionally counts global_state in test files (like unsafe_density).
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: globalStateKind, Module: globalStateActorMod, File: "sync/state_test.rs", Language: langRust},
	}}
	res := modularity.GlobalStateDensityMetric{}.Calculate(in)
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info (test-file global_state fact must be counted)", res.Band)
	}
	if res.Value != 1 {
		t.Errorf("Value = %v, want 1 (test-file fact counted, not skipped)", res.Value)
	}
}

func TestGlobalStateDensity_Metadata(t *testing.T) {
	m := modularity.GlobalStateDensityMetric{}
	if m.Name() != globalStateDensityName {
		t.Errorf("Name() = %q, want %s", m.Name(), globalStateDensityName)
	}
	if m.Version() != globalStateDensityVersion {
		t.Errorf("Version() = %q, want %s", m.Version(), globalStateDensityVersion)
	}
}
