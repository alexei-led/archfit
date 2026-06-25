package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	unsafeDensityName    = "unsafe_density"
	unsafeDensityVersion = "unsafe_density.v1"
	unsafeActorFile      = "actor/lives.rs"
	unsafeOpKind         = "unsafe_op"
	unsafeActorMod       = "yazi::actor"
)

func TestUnsafeDensity_NoFacts_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: nil}
	res := modularity.UnsafeDensityMetric{}.Calculate(in)
	if res.Band != "n/a" {
		t.Errorf("Band = %q, want n/a when no unsafe_op facts", res.Band)
	}
	if res.Name != unsafeDensityName {
		t.Errorf("Name = %q, want %s", res.Name, unsafeDensityName)
	}
}

func TestUnsafeDensity_OnlyOtherKinds_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: "function", Module: "mymod", File: "a.rs"},
		{Kind: "struct", Module: "mymod", File: "a.rs"},
	}}
	res := modularity.UnsafeDensityMetric{}.Calculate(in)
	if res.Band != "n/a" {
		t.Errorf("Band = %q, want n/a when no unsafe_op facts among other facts", res.Band)
	}
}

func TestUnsafeDensity_SomeFacts_ReturnsTotalCount(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: unsafeOpKind, Module: unsafeActorMod, File: unsafeActorFile},
		{Kind: unsafeOpKind, Module: unsafeActorMod, File: unsafeActorFile},
		{Kind: unsafeOpKind, Module: "yazi::shim", File: "shim/cell.rs"},
		{Kind: "function", Module: unsafeActorMod, File: unsafeActorFile}, // ignored
	}}
	res := modularity.UnsafeDensityMetric{}.Calculate(in)
	if res.Band != "info" {
		t.Errorf("Band = %q, want info", res.Band)
	}
	if res.Value != 3 {
		t.Errorf("Value = %v, want 3 (total unsafe_op count)", res.Value)
	}
	if res.Name != unsafeDensityName {
		t.Errorf("Name = %q, want %s", res.Name, unsafeDensityName)
	}
}

func TestUnsafeDensity_NoModule_FallsBackToFile(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: unsafeOpKind, Module: "", File: "main.rs"},
		{Kind: unsafeOpKind, Module: "", File: "main.rs"},
	}}
	res := modularity.UnsafeDensityMetric{}.Calculate(in)
	if res.Value != 2 {
		t.Errorf("Value = %v, want 2", res.Value)
	}
	if res.Band != "info" {
		t.Errorf("Band = %q, want info", res.Band)
	}
}

func TestUnsafeDensity_Metadata(t *testing.T) {
	m := modularity.UnsafeDensityMetric{}
	if m.Name() != unsafeDensityName {
		t.Errorf("Name() = %q, want %s", m.Name(), unsafeDensityName)
	}
	if m.Version() != unsafeDensityVersion {
		t.Errorf("Version() = %q, want %s", m.Version(), unsafeDensityVersion)
	}
}
