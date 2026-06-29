package modularity_test

import (
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	panicDensityName    = "panic_density"
	panicDensityVersion = "panic_density.v1"
	panicOpKind         = "panic_op"
)

func si(facts ...diagnostic.SyntaxFact) signal.SizeInput {
	return signal.SizeInput{CommonInput: signal.CommonInput{SyntaxFacts: facts}}
}

func siWithIndex(idx map[string]fileclass.FileClass, facts ...diagnostic.SyntaxFact) signal.SizeInput {
	return signal.SizeInput{
		CommonInput: signal.CommonInput{SyntaxFacts: facts},
		Size:        signal.SizeSignals{FileClassIndex: idx},
	}
}

func TestPanicDensity_NoFacts_ReturnsNA(t *testing.T) {
	res := modularity.PanicDensityMetric{}.Calculate(signal.SizeInput{})
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no panic_op facts", res.Band)
	}
	if res.Name != panicDensityName {
		t.Errorf("Name = %q, want %s", res.Name, panicDensityName)
	}
}

func TestPanicDensity_OnlyOtherKinds_ReturnsNA(t *testing.T) {
	res := modularity.PanicDensityMetric{}.Calculate(si(
		diagnostic.SyntaxFact{Kind: kindFunctionSF, Module: modMyMod, File: fileARs, Language: langRust},
		diagnostic.SyntaxFact{Kind: kindStructStr, Module: modMyMod, File: fileARs, Language: langRust},
	))
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no panic_op facts among other facts", res.Band)
	}
}

func TestPanicDensity_SomeFacts_ReturnsTotalCount(t *testing.T) {
	res := modularity.PanicDensityMetric{}.Calculate(si(
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: unsafeActorMod, File: unsafeActorFile, Language: langRust},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: unsafeActorMod, File: unsafeActorFile, Language: langRust},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: "yazi::shim", File: "shim/cell.rs", Language: langRust},
		diagnostic.SyntaxFact{Kind: kindFunctionSF, Module: unsafeActorMod, File: unsafeActorFile, Language: langRust}, // ignored
	))
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
	}
	if res.Value != 3 {
		t.Errorf("Value = %v, want 3 (total panic_op count)", res.Value)
	}
	if res.Name != panicDensityName {
		t.Errorf("Name = %q, want %s", res.Name, panicDensityName)
	}
}

// TestPanicDensity_TestFileExcluded_CountsOnlyProd is the load-bearing proof that
// test files are excluded from the panic_density count.
func TestPanicDensity_TestFileExcluded_CountsOnlyProd(t *testing.T) {
	// 2 prod Go facts (main.go), 1 test Go file (foo_test.go) excluded,
	// 1 prod Rust fact (src/main.rs), 1 test Rust file (tests/integration.rs) excluded.
	res := modularity.PanicDensityMetric{}.Calculate(si(
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMyApp, File: fileMainGo, Language: langGo},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMyApp, File: fileMainGo, Language: langGo},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMyApp, File: fileFooTestGo, Language: langGo}, // excluded: *_test.go
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMyMod, File: "src/main.rs", Language: langRust},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMyMod, File: fileTestsIntegRs, Language: langRust}, // excluded: /tests/ segment
	))
	if res.Band == bandNAStr {
		t.Fatalf("Band = n/a, want info — production facts should produce a result")
	}
	if res.Value != 3 {
		t.Errorf("Value = %v, want 3 (2 Go prod + 1 Rust prod; 2 test files excluded)", res.Value)
	}
	if !strings.Contains(res.Display, "2") || !strings.Contains(res.Display, "excluded") {
		t.Errorf("Display = %q, want excluded count (2) mentioned", res.Display)
	}
}

// TestPanicDensity_AllTestFiles_ZeroProdExcludedReported verifies the segregate-
// not-hide policy: when all panic_op facts are in test/generated files, the result
// is informational (value 0) with the excluded count reported, not a bare n/a.
func TestPanicDensity_AllTestFiles_ZeroProdExcludedReported(t *testing.T) {
	res := modularity.PanicDensityMetric{}.Calculate(si(
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMyApp, File: fileFooTestGo, Language: langGo},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMyMod, File: fileTestsIntegRs, Language: langRust},
	))
	if res.Band == bandNAStr {
		t.Errorf("Band = n/a, want info — excluded count must be reported (segregate-not-hide policy)")
	}
	if res.Value != 0 {
		t.Errorf("Value = %v, want 0 production panics", res.Value)
	}
	if !strings.Contains(res.Display, "excluded") {
		t.Errorf("Display = %q, want excluded count mentioned", res.Display)
	}
}

func TestPanicDensity_NoModule_FallsBackToFile(t *testing.T) {
	res := modularity.PanicDensityMetric{}.Calculate(si(
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: "", File: fileMainRs, Language: langRust},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: "", File: fileMainRs, Language: langRust},
	))
	if res.Value != 2 {
		t.Errorf("Value = %v, want 2", res.Value)
	}
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
	}
}

// TestPanicDensity_MockFile_ExcludedViaIndex reproduces the pumba scenario:
// mock_*.go files that are NOT *_test.go must be excluded via FileClassIndex
// (or built-in filename patterns). Verifies that production panic_density
// approaches zero when all panics are in mock/generated code.
func TestPanicDensity_MockFile_ExcludedViaIndex(t *testing.T) {
	const mockFile = "mocks/mock_client.go"
	const prodFile = "internal/service/handler.go"
	const modMocks = "mocks"
	idx := map[string]fileclass.FileClass{
		mockFile: fileclass.Generated,
		prodFile: fileclass.Production,
	}
	res := modularity.PanicDensityMetric{}.Calculate(siWithIndex(idx,
		// 208 panics in mock file — all excluded
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMocks, File: mockFile, Language: langGo},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMocks, File: mockFile, Language: langGo},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: modMocks, File: mockFile, Language: langGo},
		// 1 real production panic
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: "service", File: prodFile, Language: langGo},
	))
	if res.Value != 1 {
		t.Errorf("Value = %v, want 1 (only the production panic counts; 3 mock panics excluded)", res.Value)
	}
	if !strings.Contains(res.Display, "3") || !strings.Contains(res.Display, "excluded") {
		t.Errorf("Display = %q, want '3 ... excluded' in evidence", res.Display)
	}
}

// TestPanicDensity_MockFileBuiltinPattern_ExcludedWithoutIndex verifies that
// mock_*.go files are classified as Generated even without a FileClassIndex,
// using the built-in filename heuristic in the LookupFileClass fallback.
func TestPanicDensity_MockFileBuiltinPattern_ExcludedWithoutIndex(t *testing.T) {
	// No index — relies on built-in isGeneratedFilename("mock_client.go")
	res := modularity.PanicDensityMetric{}.Calculate(si(
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: "mocks", File: "mocks/mock_client.go", Language: langGo},
		diagnostic.SyntaxFact{Kind: panicOpKind, Module: "service", File: "internal/service/handler.go", Language: langGo},
	))
	if res.Value != 1 {
		t.Errorf("Value = %v, want 1 (mock_client.go excluded by built-in filename pattern)", res.Value)
	}
}

func TestPanicDensity_Metadata(t *testing.T) {
	m := modularity.PanicDensityMetric{}
	if m.Name() != panicDensityName {
		t.Errorf("Name() = %q, want %s", m.Name(), panicDensityName)
	}
	if m.Version() != panicDensityVersion {
		t.Errorf("Version() = %q, want %s", m.Version(), panicDensityVersion)
	}
}
