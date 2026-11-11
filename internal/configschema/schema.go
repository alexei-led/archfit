// Package configschema generates archfit.schema.json, the JSON Schema of the
// Archfit policy file (.archfit.yaml, schema 3). The schema is reflected from the
// types in internal/config/policyfile, with their doc comments as descriptions.
//
// The committed archfit.schema.json must match Generate: the no-drift test and
// the `make schema` target enforce it. The schema states exactly what
// policyfile.Decode accepts; a test runs every policyfile fixture through both.
package configschema

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
)

const (
	schemaID          = "https://raw.githubusercontent.com/alexei-led/archfit/main/archfit.schema.json"
	schemaDraft       = "https://json-schema.org/draft/2020-12/schema"
	schemaTitle       = "Archfit policy (schema 3)"
	schemaDescription = "The Archfit policy, .archfit.yaml: modules, allowed dependencies, rules and exceptions. Archfit 4 reads it. Archfit 3 reads version 2 and uses the schema of tag v3.4.0."

	// commentBase is this package's import path. AddGoComments keys comments by
	// path.Join(base, walked directory), so joining "../config/policyfile" onto it
	// gives the policyfile import path the reflected types carry.
	commentBase = "github.com/alexei-led/archfit/v3/internal/configschema"

	typeString = "string"
)

// Generate produces the JSON Schema bytes for policyfile.File.
//
// srcDir is the path of internal/config/policyfile relative to this package
// (tests pass "../config/policyfile"), so its doc comments become descriptions.
// The output uses 2-space indentation and ends with a newline.
func Generate(srcDir string) ([]byte, error) {
	r := &jsonschema.Reflector{
		// Property names are the YAML keys.
		FieldNameTag: "yaml",
		// The root properties sit at the top level, not behind a $ref.
		ExpandedStruct: true,
		// Only fields tagged `jsonschema:"required"` are required.
		RequiredFromJSONSchemaTags: true,
		Mapper:                     mapType,
	}
	if err := r.AddGoComments(commentBase, srcDir); err != nil {
		return nil, fmt.Errorf("configschema: AddGoComments: %w", err)
	}

	schema := r.Reflect(&policyfile.File{})
	schema.ID = jsonschema.ID(schemaID)
	schema.Version = schemaDraft
	schema.Title = schemaTitle
	schema.Description = schemaDescription
	patch(schema)

	buf, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("configschema: marshal: %w", err)
	}
	return append(buf, '\n'), nil
}

// mapType publishes the closed value lists and the two YAML unions of
// policyfile. Each call returns a new schema, because the reflector writes the
// field's description and default into it.
func mapType(t reflect.Type) *jsonschema.Schema {
	switch t {
	case reflect.TypeFor[policyfile.Gate]():
		return enum(policyfile.Gates)
	case reflect.TypeFor[policyfile.Mode]():
		return enum(policyfile.Modes)
	case reflect.TypeFor[policyfile.Volatility]():
		return enum(policyfile.Volatilities)
	case reflect.TypeFor[policyfile.Subdomain]():
		return enum(policyfile.Subdomains)
	case reflect.TypeFor[policyfile.CoverageFormat]():
		return enum(policyfile.CoverageFormats)
	case reflect.TypeFor[policyfile.Switch]():
		return &jsonschema.Schema{OneOf: []*jsonschema.Schema{
			{Type: "boolean"},
			{Const: string(policyfile.SwitchAuto)},
		}}
	case reflect.TypeFor[policyfile.Names]():
		name := func() *jsonschema.Schema { return &jsonschema.Schema{Type: typeString, MinLength: one()} }
		return &jsonschema.Schema{OneOf: []*jsonschema.Schema{
			name(),
			{Type: "array", Items: name(), MinItems: one()},
		}}
	}
	return nil
}

func enum[T ~string](values []T) *jsonschema.Schema {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return &jsonschema.Schema{Type: typeString, Enum: out}
}

func one() *uint64 {
	n := uint64(1)
	return &n
}

// patch adds the constraints that struct tags cannot express. Each mirrors a
// check in policyfile.Decode.
func patch(schema *jsonschema.Schema) {
	// An empty string is never a value: Decode rejects an explicit "" rather
	// than read it as an absent key.
	nonEmptyStrings(schema)
	for _, def := range schema.Definitions {
		nonEmptyStrings(def)
	}

	if version, ok := schema.Properties.Get("version"); ok {
		version.Const = policyfile.Version
	}
	if modules, ok := schema.Properties.Get("modules"); ok {
		modules.MinProperties = one()
	}

	// A deny rule also needs instead. This sits beside the oneOf, not inside its
	// deny branch: there, a rule with deny and note but no instead would fail the
	// deny branch, match only the note branch, and pass.
	if rule := schema.Definitions["Rule"]; rule != nil {
		rule.DependentRequired = map[string][]string{"deny": {"instead"}}
	}

	// An unowned exception names a path and no target; every other exception
	// names the target module.
	if exception := schema.Definitions["Exception"]; exception != nil {
		exception.If = requiredProperty("rule", &jsonschema.Schema{Const: policyfile.RuleUnowned})
		exception.Then = &jsonschema.Schema{Not: &jsonschema.Schema{Required: []string{"to"}}}
		exception.Else = &jsonschema.Schema{Required: []string{"to"}}
	}

	// A timeout is a Go duration without a sign.
	if analyzer := schema.Definitions["TimedAnalyzer"]; analyzer != nil {
		if timeout, ok := analyzer.Properties.Get("timeout"); ok {
			timeout.Pattern = policyfile.DurationPattern
		}
	}

	// Enabled coverage needs at least one source.
	if coverage := schema.Definitions["Coverage"]; coverage != nil {
		coverage.If = requiredProperty("enabled", &jsonschema.Schema{Const: true})
		coverage.Then = requiredProperty("sources", &jsonschema.Schema{MinItems: one()})
	}
}

// nonEmptyStrings gives every string schema reachable from s, without following
// $ref, a minimum length of 1.
func nonEmptyStrings(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if s.Type == typeString && s.MinLength == nil {
		s.MinLength = one()
	}
	nonEmptyStrings(s.Items)
	nonEmptyStrings(s.AdditionalProperties)
	for _, branch := range s.OneOf {
		nonEmptyStrings(branch)
	}
	if s.Properties != nil {
		for pair := s.Properties.Oldest(); pair != nil; pair = pair.Next() {
			nonEmptyStrings(pair.Value)
		}
	}
}

// requiredProperty is a schema that requires key and constrains its value.
func requiredProperty(key string, value *jsonschema.Schema) *jsonschema.Schema {
	s := &jsonschema.Schema{Properties: jsonschema.NewProperties(), Required: []string{key}}
	s.Properties.Set(key, value)
	return s
}
