// Package reportschema generates a JSON Schema for the repository report,
// the `archfit.architecture-state.v1` document that `report --format json` emits at the
// document root.
//
// It exists because the output contract had no machine-readable form: consumers
// had only prose and nothing to validate or generate types from,
// while archfit.schema.json describes the CONFIG and cannot stand in for it.
// The schema is derived from the Go structs in internal/model/report, so the
// published contract and the emitting types cannot drift apart silently; the
// repo-committed schema at archfit.report.schema.json is pinned by the no-drift
// test and regenerated with `make schema`.
package reportschema

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"

	"github.com/alexei-led/archfit/v3/internal/model/report"
)

const (
	schemaID    = "https://raw.githubusercontent.com/alexei-led/archfit/main/archfit.report.schema.json"
	schemaDraft = "https://json-schema.org/draft/2020-12/schema"
)

// Generate produces the JSON Schema bytes for report.ArchitectureState.
//
// srcDir is the filesystem path to internal/model/report so AddGoComments can
// parse doc-comments into schema descriptions; tests pass "../model/report".
//
// Every field of the state document is always emitted — the contract's point is
// that a run which measured nothing still reports nine named dimensions rather
// than an absent key — so the reflector marks fields required unless they carry
// `omitempty`. That is the opposite default from the config schema, where
// almost every key is optional.
//
// The output is deterministically formatted with 2-space indentation.
func Generate(srcDir string) ([]byte, error) {
	r := &jsonschema.Reflector{
		// Inline the root's own fields rather than wrapping them in a $ref, so a
		// consumer sees schema_version/verdict/dimensions at the top level — the
		// shape the document actually has.
		ExpandedStruct: true,
	}

	if err := r.AddGoComments("github.com/alexei-led/archfit/v3/internal/model/report", srcDir); err != nil {
		return nil, fmt.Errorf("reportschema: AddGoComments: %w", err)
	}

	schema := r.Reflect(&report.ArchitectureState{})
	schema.ID = jsonschema.ID(schemaID)
	schema.Version = schemaDraft

	patchDefinitions(schema)

	buf, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("reportschema: marshal: %w", err)
	}
	return append(buf, '\n'), nil
}

// patchDefinitions pins the closed vocabularies the Go types express as named
// string types, which the reflector inlines as a bare {type: string}.
//
// A consumer that cannot tell `blocked` from a typo has no contract: these
// enums are the difference between validating a document and merely checking it
// is JSON. Each list mirrors the constants in internal/model/report.
func patchDefinitions(schema *jsonschema.Schema) {
	if schema.Properties != nil {
		if version, ok := schema.Properties.Get("schema_version"); ok {
			version.Enum = []any{report.StateSchemaVersion}
			version.Description = "The only architecture-state contract this binary emits."
		}
	}
	for name, def := range schema.Definitions {
		if def.Properties == nil {
			continue
		}
		switch name {
		case "ResultIdentity":
			pinIdentityVersion(def)
		case "DimensionState":
			if status, ok := def.Properties.Get("status"); ok {
				status.Enum = []any{
					string(report.MeasurementMeasured),
					string(report.MeasurementPartial),
					string(report.MeasurementUnmeasured),
				}
			}
		case "SeamScoreDistribution":
			// p10/p90 are *int and serialize as null below ten samples: a
			// percentile nobody can compute is reported as absent, never as 0.
			// The reflector renders them as plain integers, which would make the
			// published schema reject archfit's own output on any small seam.
			for _, field := range []string{"p10", "p90"} {
				percentile, ok := def.Properties.Get(field)
				if !ok {
					continue
				}
				percentile.Type = ""
				percentile.OneOf = []*jsonschema.Schema{{Type: "integer"}, {Type: "null"}}
				percentile.Description = "Nearest-rank percentile, or null below ten scored samples."
			}
		case "SeamDistance":
			// same_owner/same_deploy_unit are *bool and serialize as null unless
			// both sides are known: undeclared is not "different".
			for _, field := range []string{"same_owner", "same_deploy_unit"} {
				same, ok := def.Properties.Get(field)
				if !ok {
					continue
				}
				same.Type = ""
				same.OneOf = []*jsonschema.Schema{{Type: "boolean"}, {Type: "null"}}
				same.Description = "Whether both sides share the value, or null unless both sides are known."
			}
		}
	}
}

// pinIdentityVersion pins the identity block's own version string.
func pinIdentityVersion(def *jsonschema.Schema) {
	if version, ok := def.Properties.Get("schema_version"); ok {
		version.Enum = []any{report.IdentitySchemaVersion}
	}
}
