package modularity_test

import (
	"testing"

	"github.com/alexei-led/archfit/internal/metrics/modularity"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
	"github.com/alexei-led/archfit/internal/model/signal"
)

const (
	structFieldDensityName    = "struct_field_density"
	structFieldDensityVersion = "struct_field_density.v1"
	structFieldKind           = "struct_field"
	structFieldMod            = "myapp/domain"
	structFieldFile           = "domain/model.go"
)

func TestStructFieldDensity_NoFacts_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: nil}
	res := modularity.StructFieldDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no syntax facts", res.Band)
	}
	if res.Name != structFieldDensityName {
		t.Errorf("Name = %q, want %s", res.Name, structFieldDensityName)
	}
}

func TestStructFieldDensity_OnlyOtherKinds_ReturnsNA(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: kindFunctionSF, Module: structFieldMod, File: structFieldFile},
		{Kind: "struct", Module: structFieldMod, File: structFieldFile},
	}}
	res := modularity.StructFieldDensityMetric{}.Calculate(in)
	if res.Band != bandNAStr {
		t.Errorf("Band = %q, want n/a when no struct_field facts among other facts", res.Band)
	}
}

func TestStructFieldDensity_SomeFacts_ReturnsTotalCount(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: structFieldKind, Module: structFieldMod, File: structFieldFile, Name: "Order", Count: 5},
		{Kind: structFieldKind, Module: structFieldMod, File: structFieldFile, Name: "User", Count: 3},
		{Kind: kindFunctionSF, Module: structFieldMod, File: structFieldFile, Name: "NewOrder"}, // ignored
	}}
	res := modularity.StructFieldDensityMetric{}.Calculate(in)
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
	}
	if res.Value != 8 {
		t.Errorf("Value = %v, want 8 (sum of field counts)", res.Value)
	}
	if res.Name != structFieldDensityName {
		t.Errorf("Name = %q, want %s", res.Name, structFieldDensityName)
	}
}

func TestStructFieldDensity_ZeroCount_Ignored(t *testing.T) {
	// Count=0 facts (tuple/unit structs) still contribute 0 to total —
	// they appear in maxCounts but add nothing to total.
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: structFieldKind, Module: structFieldMod, File: structFieldFile, Name: "UnitStruct", Count: 0},
		{Kind: structFieldKind, Module: structFieldMod, File: structFieldFile, Name: "BigStruct", Count: 4},
	}}
	res := modularity.StructFieldDensityMetric{}.Calculate(in)
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info (BigStruct provides a non-zero count)", res.Band)
	}
	// total = 0 + 4 = 4
	if res.Value != 4 {
		t.Errorf("Value = %v, want 4", res.Value)
	}
}

func TestStructFieldDensity_NoModule_FallsBackToFile(t *testing.T) {
	in := signal.CommonInput{SyntaxFacts: []diagnostic.SyntaxFact{
		{Kind: structFieldKind, Module: "", File: "cmd/main.go", Name: "Config", Count: 6},
	}}
	res := modularity.StructFieldDensityMetric{}.Calculate(in)
	if res.Value != 6 {
		t.Errorf("Value = %v, want 6", res.Value)
	}
	if res.Band != bandInfo {
		t.Errorf("Band = %q, want info", res.Band)
	}
}

func TestStructFieldDensity_Metadata(t *testing.T) {
	m := modularity.StructFieldDensityMetric{}
	if m.Name() != structFieldDensityName {
		t.Errorf("Name() = %q, want %s", m.Name(), structFieldDensityName)
	}
	if m.Version() != structFieldDensityVersion {
		t.Errorf("Version() = %q, want %s", m.Version(), structFieldDensityVersion)
	}
}
