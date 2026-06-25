package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	panicDensityName    = "panic_density"
	panicDensityVersion = "panic_density.v1"
	panicOpKind         = "panic_op"
)

func TestPanicDensity_NoFacts_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: nil}
	res := modularity.PanicDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no panic_op facts", res.Band)
	}
	if res.Name != panicDensityName {
		t.Errorf("Name = %q, want %s", res.Name, panicDensityName)
	}
}

func TestPanicDensity_OnlyOtherKinds_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: kindFunctionSF, Module: modMyMod, File: fileARs, Language: langRust},
		{Kind: kindStructStr, Module: modMyMod, File: fileARs, Language: langRust},
	}}
	res := modularity.PanicDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no panic_op facts among other facts", res.Band)
	}
}

func TestPanicDensity_SomeFacts_ReturnsTotalCount(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: panicOpKind, Module: unsafeActorMod, File: unsafeActorFile, Language: langRust},
		{Kind: panicOpKind, Module: unsafeActorMod, File: unsafeActorFile, Language: langRust},
		{Kind: panicOpKind, Module: "yazi::shim", File: "shim/cell.rs", Language: langRust},
		{Kind: kindFunctionSF, Module: unsafeActorMod, File: unsafeActorFile, Language: langRust}, // ignored
	}}
	res := modularity.PanicDensityMetric{}.Calculate(in)
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
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: panicOpKind, Module: modMyApp, File: "main.go", Language: langGo},
		{Kind: panicOpKind, Module: modMyApp, File: "main.go", Language: langGo},
		{Kind: panicOpKind, Module: modMyApp, File: "foo_test.go", Language: langGo}, // excluded: *_test.go
		{Kind: panicOpKind, Module: modMyMod, File: "src/main.rs", Language: langRust},
		{Kind: panicOpKind, Module: modMyMod, File: "tests/integration.rs", Language: langRust}, // excluded: /tests/ segment
	}}
	res := modularity.PanicDensityMetric{}.Calculate(in)
	if res.Band == bandNAStr {
		t.Fatalf("Band = n/a, want info — production facts should produce a result")
	}
	if res.Value != 3 {
		t.Errorf("Value = %v, want 3 (2 Go prod + 1 Rust prod; 2 test files excluded)", res.Value)
	}
}

func TestPanicDensity_AllTestFiles_ReturnsNA(t *testing.T) {
	// All facts are in test files — result must be n/a (no production panics).
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: panicOpKind, Module: modMyApp, File: "foo_test.go", Language: langGo},
		{Kind: panicOpKind, Module: modMyMod, File: "tests/integration.rs", Language: langRust},
	}}
	res := modularity.PanicDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when all panic_op facts are in test files", res.Band)
	}
}

func TestPanicDensity_NoModule_FallsBackToFile(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: panicOpKind, Module: "", File: fileMainRs, Language: langRust},
		{Kind: panicOpKind, Module: "", File: fileMainRs, Language: langRust},
	}}
	res := modularity.PanicDensityMetric{}.Calculate(in)
	if res.Value != 2 {
		t.Errorf("Value = %v, want 2", res.Value)
	}
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
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
