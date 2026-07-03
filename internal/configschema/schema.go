// Package configschema generates a JSON Schema for archfit's .archfit.yaml configuration file.
// The schema is derived from the Go structs in internal/config, using yaml struct tags as keys.
//
// Use Generate to produce the schema bytes; the repo-committed schema at
// archfit.schema.json must stay in sync — enforced by the no-drift test and
// the `make schema` target.
package configschema

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"

	"github.com/alexei-led/archfit/internal/config"
)

const (
	schemaID    = "https://raw.githubusercontent.com/alexei-led/archfit/main/archfit.schema.json"
	schemaDraft = "https://json-schema.org/draft/2020-12/schema"
	typeString  = "string" // JSON Schema scalar type, reused across patches
)

// toolModeSchema is the union that replaces the inlined {type: string} for any
// struct field named "enabled" that carries ToolMode. It mirrors what the YAML
// config actually accepts: a native boolean (true/false) or the string "auto".
// The legacy "on"/"off" spellings are rejected by config.ToolMode.UnmarshalYAML.
var toolModeSchema = &jsonschema.Schema{
	Description: "Enable state: true | false | \"auto\" (on/off are not accepted)",
	OneOf: []*jsonschema.Schema{
		{Type: "boolean"},
		{Const: "auto"},
	},
}

// gateModeSchema tightens the inlined {type: string} for the "gate" field to
// only the three accepted values.
var gateModeSchema = &jsonschema.Schema{
	Type:        typeString,
	Enum:        []any{"off", "warn", "fail"},
	Description: "Gate posture: off (advisory, never fails) | warn (default, exit 0) | fail (hard gate)",
}

// Generate produces the JSON Schema bytes for config.Config.
//
// srcDir is the filesystem path to internal/config (the package directory)
// so that AddGoComments can parse doc-comments into schema descriptions.
// Callers in tests pass "../config"; cmd-level callers pass the absolute path.
//
// The output is deterministically formatted with 2-space indentation.
func Generate(srcDir string) ([]byte, error) {
	r := &jsonschema.Reflector{
		// Use yaml struct tags as property names instead of json tags.
		FieldNameTag: "yaml",
		// Inline Config's own fields into the root schema object rather than
		// wrapping them in a $ref, so editors see properties at the top level.
		ExpandedStruct: true,
		// Almost every key in .archfit.yaml is optional — validate() enforces
		// enums and ranges only when a key is present. Without this flag the
		// reflector marks every field lacking a yaml `omitempty` as required,
		// so valid configs (knob-only metric entries, rules without both
		// module and layer selectors) fail schema validation in editors. Only
		// fields tagged `jsonschema:"required"` (Config.Version) are required.
		RequiredFromJSONSchemaTags: true,
	}

	// Pull doc-comments from the source so each property gets a description.
	if err := r.AddGoComments("github.com/alexei-led/archfit/internal/config", srcDir); err != nil {
		return nil, fmt.Errorf("configschema: AddGoComments: %w", err)
	}

	schema := r.Reflect(&config.Config{})

	// Stable identity headers.
	schema.ID = jsonschema.ID(schemaID)
	schema.Version = schemaDraft

	// Post-process: ToolMode and GateMode are inlined by invopop as {type: string}
	// because they are named string types. Patch every struct definition to replace
	// the inlined schemas with the correct union/enum shapes.
	patchDefinitions(schema)

	// Marshal with stable 2-space indentation.
	buf, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("configschema: marshal: %w", err)
	}

	// Append a trailing newline for POSIX-correct text files.
	buf = append(buf, '\n')
	return buf, nil
}

// patchDefinitions walks all named definitions in the schema and replaces
// inlined ToolMode ("enabled" property, {type:string}) and GateMode
// ("gate" property, {type:string}) with the correct union/enum schemas, and
// tightens CouplingGateDef to mirror validateCouplingGate.
func patchDefinitions(schema *jsonschema.Schema) {
	for name, def := range schema.Definitions {
		if def.Properties == nil {
			continue
		}
		if enabled, ok := def.Properties.Get("enabled"); ok {
			if enabled.Type == typeString {
				// Replace in place: copy the union schema's fields.
				*enabled = *toolModeSchema
			}
		}
		if gate, ok := def.Properties.Get("gate"); ok {
			if gate.Type == typeString && gate.Enum == nil {
				*gate = *gateModeSchema
			}
		}
		if name == "ExternalSystemDef" {
			// Mirror validateExternalSystem (internal/config): at least one
			// targets glob (an empty entry declares nothing) and a real
			// volatility level when one is set.
			if targets, ok := def.Properties.Get("targets"); ok {
				one := uint64(1)
				targets.MinItems = &one
			}
			if vol, ok := def.Properties.Get("volatility"); ok && vol.Type == typeString {
				vol.Enum = []any{"high", "medium", "low", "frozen"}
			}
			def.Required = []string{"targets"}
		}
		if name == "RuleDef" {
			// Mirror rules.New (internal/rules): an empty or unrecognized rule
			// `type` is a hard load error, so the schema must not show a
			// typeless rule entry as valid. `id` stays optional — validation
			// substitutes a positional placeholder.
			def.Required = []string{"type"}
		}
		if name == "CouplingGateDef" {
			// Mirror validateCouplingGate (internal/config): min_band is one of
			// the four band floors (critical rejected — could never trip), and
			// an empty gate block gates nothing, so at least one knob is
			// required. Without this, editors show green on exactly the
			// validated-but-inert configs the gate was built to prevent.
			if minBand, ok := def.Properties.Get("min_band"); ok && minBand.Type == typeString {
				minBand.Enum = []any{"poor", "mixed", "serviceable", "strong"}
			}
			if maxDrop, ok := def.Properties.Get("max_drop"); ok {
				maxDrop.Minimum = "0" // validateCouplingGate rejects a negative drop
			}
			def.AnyOf = []*jsonschema.Schema{
				{Required: []string{"min_band"}},
				{Required: []string{"max_drop"}},
			}
		}
	}
}
