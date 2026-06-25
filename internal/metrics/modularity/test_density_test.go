package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	testDensityName    = "test_density"
	testDensityVersion = "test_density.v1"
	testFnKind         = "test_fn"
)

func TestTestDensity_NoFacts_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: nil}
	res := modularity.TestDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no test_fn facts", res.Band)
	}
	if res.Name != testDensityName {
		t.Errorf("Name = %q, want %s", res.Name, testDensityName)
	}
}

func TestTestDensity_OnlyOtherKinds_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: kindFunctionSF, Module: modMyMod, File: fileARs, Language: langRust},
		{Kind: kindStructStr, Module: modMyMod, File: fileARs, Language: langRust},
	}}
	res := modularity.TestDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no test_fn facts among other facts", res.Band)
	}
}

func TestTestDensity_SomeFacts_ReturnsTotalCount(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: testFnKind, Module: modMyApp, File: "main_test.go", Language: langGo},
		{Kind: testFnKind, Module: modMyApp, File: "main_test.go", Language: langGo},
		{Kind: testFnKind, Module: modMyMod, File: "src/lib_test.rs", Language: langRust},
		{Kind: kindFunctionSF, Module: modMyApp, File: fileMainGo, Language: langGo}, // ignored
	}}
	res := modularity.TestDensityMetric{}.Calculate(in)
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
	}
	if res.Value != 3 {
		t.Errorf("Value = %v, want 3 (total test_fn count)", res.Value)
	}
	if res.Name != testDensityName {
		t.Errorf("Name = %q, want %s", res.Name, testDensityName)
	}
}

// TestTestDensity_TestFileFacts_AreCounted is the load-bearing proof that test-file
// facts are COUNTED (not excluded) — the inverse of panic_density's exclusion logic.
func TestTestDensity_TestFileFacts_AreCounted(t *testing.T) {
	// All facts are in test files — all must be counted.
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: testFnKind, Module: modMyApp, File: fileFooTestGo, Language: langGo},
		{Kind: testFnKind, Module: modMyApp, File: "bar_test.go", Language: langGo},
		{Kind: testFnKind, Module: modMyMod, File: fileTestsIntegRs, Language: langRust},
	}}
	res := modularity.TestDensityMetric{}.Calculate(in)
	if res.Band == bandNAStr {
		t.Fatalf("Band = n/a, want info — test-file facts must be counted, not excluded")
	}
	if res.Value != 3 {
		t.Errorf("Value = %v, want 3 (all test-file facts must count)", res.Value)
	}
}

func TestTestDensity_NoModule_FallsBackToFile(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: testFnKind, Module: "", File: fileMainRs, Language: langRust},
		{Kind: testFnKind, Module: "", File: fileMainRs, Language: langRust},
	}}
	res := modularity.TestDensityMetric{}.Calculate(in)
	if res.Value != 2 {
		t.Errorf("Value = %v, want 2", res.Value)
	}
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
	}
}

func TestTestDensity_Metadata(t *testing.T) {
	m := modularity.TestDensityMetric{}
	if m.Name() != testDensityName {
		t.Errorf("Name() = %q, want %s", m.Name(), testDensityName)
	}
	if m.Version() != testDensityVersion {
		t.Errorf("Version() = %q, want %s", m.Version(), testDensityVersion)
	}
}
