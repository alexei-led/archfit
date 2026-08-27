package configschema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
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
		Properties map[string]struct {
			Enum []any `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(got, &schema); err != nil {
		t.Fatalf("unmarshal generated schema: %v", err)
	}

	ruleDef, ok := schema.Defs["RuleDef"]
	if !ok {
		t.Fatal("RuleDef definition missing from generated schema")
	}
	if !slices.Equal(ruleDef.Required, []string{"id", "type"}) {
		t.Errorf("RuleDef.required = %v, want [id type] (rule ids feed stable finding fingerprints)", ruleDef.Required)
	}

	extDef, ok := schema.Defs["ExternalSystemDef"]
	if !ok {
		t.Fatal("ExternalSystemDef definition missing from generated schema")
	}
	if len(extDef.Required) != 1 || extDef.Required[0] != "targets" {
		t.Errorf("ExternalSystemDef.required = %v, want [targets]", extDef.Required)
	}
	wantVolatilityEnum := []any{"high", "medium", "low", "frozen"}
	if got := extDef.Properties["volatility"].Enum; !slices.Equal(got, wantVolatilityEnum) {
		t.Errorf("ExternalSystemDef.volatility enum = %v, want %v", got, wantVolatilityEnum)
	}

	couplingDef, ok := schema.Defs["CouplingConfig"]
	if !ok {
		t.Fatal("CouplingConfig definition missing from generated schema")
	}
	wantDKEnum := []any{"score", "advisory"}
	if got := couplingDef.Properties["duplicated_knowledge"].Enum; !slices.Equal(got, wantDKEnum) {
		t.Errorf("CouplingConfig.duplicated_knowledge enum = %v, want %v", got, wantDKEnum)
	}

	gateDef, ok := schema.Defs["CouplingGateDef"]
	if !ok {
		t.Fatal("CouplingGateDef definition missing from generated schema")
	}
	// The retired v1 knobs decode only so the migration can read them. An
	// editor that still offers them would suggest writing a config this binary
	// refuses to analyse.
	for _, retired := range []string{"min_band", "max_drop"} {
		if _, present := gateDef.Properties[retired]; present {
			t.Errorf("CouplingGateDef offers retired key %q — v2 rejects it at load", retired)
		}
	}
	if !slices.Equal(gateDef.Required, []string{"distributed_monolith"}) {
		t.Errorf("CouplingGateDef.required = %v, want [distributed_monolith] — an empty gate block gates nothing",
			gateDef.Required)
	}

	dmDef, ok := schema.Defs["DistributedMonolithDef"]
	if !ok {
		t.Fatal("DistributedMonolithDef definition missing from generated schema")
	}
	wantModeEnum := []any{"warn", "fail"}
	if got := dmDef.Properties["mode"].Enum; !slices.Equal(got, wantModeEnum) {
		t.Errorf("DistributedMonolithDef.mode enum = %v, want %v", got, wantModeEnum)
	}
	if dmDef.Properties["max_new_seams"].Minimum != "0" {
		t.Errorf("DistributedMonolithDef.max_new_seams.minimum = %q, want %q (a tolerated count is never negative)",
			dmDef.Properties["max_new_seams"].Minimum, "0")
	}

	if got := schema.Properties["version"].Enum; !slices.Equal(got, []any{float64(config.SchemaVersion)}) {
		t.Errorf("version enum = %v, want [%d] — v1 decodes for migration but never analyses",
			got, config.SchemaVersion)
	}
}
