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
	"path/filepath"
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/policy"
)

const (
	schemaID    = "https://raw.githubusercontent.com/alexei-led/archfit/main/archfit.schema.json"
	schemaDraft = "https://json-schema.org/draft/2020-12/schema"
	typeString  = "string" // JSON Schema scalar type, reused across patches

	patternPkgPath = "github.com/alexei-led/archfit/internal/model/pattern"
)

// toolModeSchema is the union that replaces the inlined {type: string} for any
// struct field named "enabled" that carries ToolMode. It mirrors what the YAML
// config actually accepts: a native boolean (true/false) or the string "auto".
// The legacy "on"/"off" spellings are rejected by evidenceports.ToolMode.UnmarshalYAML.
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

// schemaDefinitionName maps a Go type to its published $defs key. pattern.Def is
// the neutral kernel spelling of what .archfit.yaml calls a pattern definition;
// the schema has always published it as "PatternDef" and must keep doing so.
func schemaDefinitionName(t reflect.Type) string {
	if t.PkgPath() == patternPkgPath && t.Name() == "Def" {
		return "PatternDef"
	}
	return t.Name()
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
		// The published $defs keys are part of the schema contract: editors and
		// validators reference them, and patchDefinitions keys its enum/required
		// tightening off the same names. Go type names may move between packages
		// during refactors, so pin the one name whose Go identifier no longer
		// matches its schema identity instead of silently renaming a $def.
		Namer: schemaDefinitionName,
	}

	// Pull doc-comments from the source so each property gets a description.
	if err := r.AddGoComments("github.com/alexei-led/archfit/internal/config", srcDir); err != nil {
		return nil, fmt.Errorf("configschema: AddGoComments: %w", err)
	}
	// Policy declarations (ModuleDef, RuleDef, WaiverDef, MetricEntry,
	// ExternalSystemDef, …) live in internal/policy, so their doc-comments become
	// property descriptions. AddGoComments keys the map by
	// Join(base, relativeWalkDir); with policyDir == srcDir/../policy the walk dir
	// is "../policy", so joining it onto the config import path resolves to
	// internal/policy. Same join trick for the acquisition port types (ToolMode)
	// and the neutral pattern definition.
	policyDir := filepath.Join(srcDir, "..", "policy")
	if err := r.AddGoComments("github.com/alexei-led/archfit/internal/config", policyDir); err != nil {
		return nil, fmt.Errorf("configschema: AddGoComments(policy): %w", err)
	}
	portsDir := filepath.Join(srcDir, "..", "evidence", "ports")
	if err := r.AddGoComments("github.com/alexei-led/archfit/internal/config", portsDir); err != nil {
		return nil, fmt.Errorf("configschema: AddGoComments(ports): %w", err)
	}
	patternDir := filepath.Join(srcDir, "..", "model", "pattern")
	if err := r.AddGoComments("github.com/alexei-led/archfit/internal/model", patternDir); err != nil {
		return nil, fmt.Errorf("configschema: AddGoComments(pattern): %w", err)
	}

	schema := r.Reflect(&config.Config{})
	// MetricsConfig's ordinary entries are decoded through its custom mixed-map
	// contract, so no Go field directly references MetricEntry for reflection.
	// Publish that definition explicitly before the mixed-map patch points at it.
	metricEntry := r.Reflect(&policy.MetricEntry{})
	metricEntry.Version, metricEntry.ID = "", ""
	schema.Definitions["MetricEntry"] = metricEntry

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
		if name == "MetricsConfig" {
			// Metrics is a mixed public map: one reserved diagnostic integer and
			// every other key an ordinary object-shaped MetricEntry. A negative
			// lookahead keeps the generic object schema from also applying to the
			// reserved scalar.
			def.PatternProperties = map[string]*jsonschema.Schema{
				`^(?!function_loc_threshold$).+$`: {Ref: "#/$defs/MetricEntry"},
			}
			def.AdditionalProperties = jsonschema.FalseSchema
		}
		if name == "CouplingConfig" {
			// Mirror validate(): clone-only duplicated knowledge has exactly two
			// policies. Empty/unset defaults to score in Config.ForClassify.
			if dk, ok := def.Properties.Get("duplicated_knowledge"); ok && dk.Type == typeString {
				dk.Enum = []any{"score", "advisory"}
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
			// Mirror validateRules/rules.New: every rule needs a stable id for
			// finding fingerprints/baseline matching, and an empty or
			// unrecognized `type` is a hard load error.
			def.Required = []string{"id", "type"}
		}
		if name == "PatternDef" {
			// Mirror validateRules (internal/config): ast-grep runs
			// `sg --lang <lang> --pattern <rule>` and keys findings by id, so
			// a partial pattern entry is a hard load error.
			def.Required = []string{"id", "lang", "rule"}
		}
		if name == "CouplingGateDef" {
			// Mirror validateCouplingGate (internal/config): the retired v1
			// knobs are not offered at all (they decode only so the migration
			// can read them), and an empty gate block gates nothing, so the
			// distributed-monolith stanza is required when the block is
			// present. Without this, editors show green on exactly the
			// validated-but-inert configs the gate was built to prevent.
			def.Properties.Delete("min_band")
			def.Properties.Delete("max_drop")
			def.Required = []string{"distributed_monolith"}
		}
		if name == "DistributedMonolithDef" {
			// Mirror validateCouplingGate: two modes, and a tolerated new-seam
			// count is never negative.
			if mode, ok := def.Properties.Get("mode"); ok && mode.Type == typeString {
				mode.Enum = []any{"warn", "fail"}
			}
			if maxNew, ok := def.Properties.Get("max_new_seams"); ok {
				maxNew.Minimum = "0"
			}
		}
	}
	// The schema version is the only accepted analysis schema. A v1 file
	// decodes for migration but never analyses, so an editor must not show it
	// green. The root Config is inlined, not a named definition, so it is
	// patched here rather than in the loop above.
	if schema.Properties != nil {
		if version, ok := schema.Properties.Get("version"); ok {
			version.Enum = []any{config.SchemaVersion}
		}
	}
}
