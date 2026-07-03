package configschema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/configschema"
)

// schemaFile is the committed schema path, relative to the repo root.
// The test is run with cwd = internal/configschema, so ../../ reaches the root.
const schemaFile = "../../archfit.schema.json"

// TestSchemaNoDrift verifies that the committed archfit.schema.json is in sync
// with what Generate produces from the current internal/config structs.
//
// To update the committed file: ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
func TestSchemaNoDrift(t *testing.T) {
	got, err := configschema.Generate("../config")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if os.Getenv("ARCHFIT_UPDATE_SCHEMA") == "1" {
		abs, _ := filepath.Abs(schemaFile)
		if err := os.WriteFile(abs, got, 0o600); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
		t.Logf("wrote %s (%d bytes)", abs, len(got))
		return
	}

	want, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatalf("read %s: %v — run `make schema` to generate it", schemaFile, err)
	}

	if string(got) != string(want) {
		t.Errorf("schema drift — run `make schema` to regenerate archfit.schema.json\n"+
			"got %d bytes, want %d bytes", len(got), len(want))
	}
}

// TestSchemaPatchedDefinitions asserts the patchDefinitions semantics directly,
// so a silently no-op'ed patch (typo'd definition name, library rename) fails
// here instead of being "fixed" by regenerating the drift snapshot.
func TestSchemaPatchedDefinitions(t *testing.T) {
	got, err := configschema.Generate("../config")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var schema struct {
		Defs map[string]struct {
			Required   []string         `json:"required"`
			AnyOf      []map[string]any `json:"anyOf"`
			Properties map[string]struct {
				Enum    []any       `json:"enum"`
				Minimum json.Number `json:"minimum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(got, &schema); err != nil {
		t.Fatalf("unmarshal generated schema: %v", err)
	}

	ruleDef, ok := schema.Defs["RuleDef"]
	if !ok {
		t.Fatal("RuleDef definition missing from generated schema")
	}
	if len(ruleDef.Required) != 1 || ruleDef.Required[0] != "type" {
		t.Errorf("RuleDef.required = %v, want [type] (rules.New hard-errors on a missing type)", ruleDef.Required)
	}

	extDef, ok := schema.Defs["ExternalSystemDef"]
	if !ok {
		t.Fatal("ExternalSystemDef definition missing from generated schema")
	}
	if len(extDef.Required) != 1 || extDef.Required[0] != "targets" {
		t.Errorf("ExternalSystemDef.required = %v, want [targets]", extDef.Required)
	}
	if got := len(extDef.Properties["volatility"].Enum); got != 4 {
		t.Errorf("ExternalSystemDef.volatility enum has %d values, want 4 (high/medium/low/frozen)", got)
	}

	gateDef, ok := schema.Defs["CouplingGateDef"]
	if !ok {
		t.Fatal("CouplingGateDef definition missing from generated schema")
	}
	if len(gateDef.AnyOf) != 2 {
		t.Errorf("CouplingGateDef.anyOf has %d branches, want 2 (min_band or max_drop required)", len(gateDef.AnyOf))
	}
	if gateDef.Properties["max_drop"].Minimum != "0" {
		t.Errorf("CouplingGateDef.max_drop.minimum = %q, want %q (negative drop rejected)", gateDef.Properties["max_drop"].Minimum, "0")
	}
	if got := len(gateDef.Properties["min_band"].Enum); got != 4 {
		t.Errorf("CouplingGateDef.min_band enum has %d values, want 4 (critical excluded — could never trip)", got)
	}
}
