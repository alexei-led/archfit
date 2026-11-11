package configschema_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/alexei-led/archfit/v3/internal/config/policyfile"
	"github.com/alexei-led/archfit/v3/internal/configschema"
	suppliedcoverage "github.com/alexei-led/archfit/v3/internal/extract/coverage"
)

const (
	// schemaFile is the committed schema; tests run in internal/configschema.
	schemaFile = "../../archfit.schema.json"
	policyDir  = "../config/policyfile"
)

func generate(t *testing.T) []byte {
	t.Helper()
	got, err := configschema.Generate(policyDir)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return got
}

// TestSchemaNoDrift keeps the committed archfit.schema.json equal to Generate.
//
// To update the committed file: ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
func TestSchemaNoDrift(t *testing.T) {
	got := generate(t)
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
	if !bytes.Equal(got, want) {
		t.Errorf("schema drift — run `make schema` to regenerate archfit.schema.json\ngot %d bytes, want %d bytes", len(got), len(want))
	}
}

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(generate(t)))
	if err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const resource = "archfit.schema.json"
	if err := compiler.AddResource(resource, doc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return compiled
}

// schemaAccepts converts YAML to JSON as an editor does and validates it. A
// document that does not convert, such as one with a duplicate key, is rejected.
func schemaAccepts(compiled *jsonschema.Schema, data []byte) error {
	converted, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(converted))
	if err != nil {
		return err
	}
	return compiled.Validate(doc)
}

// TestSchemaAgreesWithDecoder holds the published schema and policyfile.Decode to
// one contract: every policyfile fixture, and each edge case below, passes the
// schema exactly when Decode accepts it.
func TestSchemaAgreesWithDecoder(t *testing.T) {
	compiled := compileSchema(t)
	files, err := filepath.Glob(filepath.Join(policyDir, "testdata", "*", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 30 {
		t.Fatalf("found %d policyfile fixtures, want the whole set", len(files))
	}
	cases := map[string][]byte{}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Clean(file))
		if err != nil {
			t.Fatal(err)
		}
		cases[filepath.Base(filepath.Dir(file))+"/"+filepath.Base(file)] = data
	}
	const head = "version: 3\nowners: [o]\nmodules:\n  app:\n    paths: [cmd/**]\n"
	for name, tail := range map[string]string{
		"quoted min_balance": "builtin:\n  min_balance: \"7\"\n",
		"number as why":      "rules:\n  - id: r\n    note: n\n    why: 12\n",
		"null deny":          "rules:\n  - id: r\n    deny:\n    why: w\n    instead: i\n",
		"only with one name": "rules:\n  - id: r\n    only: { use: process, in: app }\n    why: w\n",
		"note with instead":  "rules:\n  - id: r\n    note: n\n    why: w\n    instead: i\n",
		"empty depends_on":   "layers: []\ngenerated: []\n",
		"enabled auto":       "analyzers:\n  roslyn:\n    enabled: auto\n",
		"quoted true":        "analyzers:\n  roslyn:\n    enabled: \"true\"\n",
		"coverage off":       "coverage:\n  enabled: false\n",
		"max_facts zero":     "coverage:\n  sources:\n    - path: c.out\n      max_facts: 0\n",
		"zero timeout":       "analyzers:\n  roslyn:\n    timeout: 0s\n",
		"negative timeout":   "analyzers:\n  roslyn:\n    timeout: -5m\n",
		"float min_balance":  "builtin:\n  min_balance: 7.0\n",
		"null exclude":       "exclude:\n",
		"date-like id":       "rules:\n  - id: 2026-11-19\n    note: n\n    why: w\n",
		"empty note on deny": "rules:\n  - id: r\n    deny: { from: app, to: lib }\n    note: \"\"\n    why: w\n    instead: i\n",
		"empty instead":      "rules:\n  - id: r\n    only: { use: process, in: app }\n    why: w\n    instead: \"\"\n",
		"deny and only":      "rules:\n  - id: r\n    deny: { from: app, to: lib }\n    only: { use: process, in: app }\n    why: w\n",
		"empty builtin mode": "builtin:\n  graph: \"\"\n",
		"empty volatility":   "version: 3\nowners: [o]\nmodules:\n  app:\n    paths: [cmd/**]\n    volatility: \"\"\n",
		"empty layer":        "version: 3\nowners: [o]\nmodules:\n  app:\n    paths: [cmd/**]\n    layer: \"\"\n",
		"empty path":         "version: 3\nowners: [o]\nmodules:\n  app:\n    paths: [\"\"]\n",
		"empty owner":        "version: 3\nowners: [\"\"]\nmodules:\n  app:\n    paths: [cmd/**]\n",
		"empty glob":         "generated: [\"\"]\n",
		"empty format":       "coverage:\n  sources:\n    - path: c.out\n      format: \"\"\n",
		"empty timeout":      "analyzers:\n  roslyn:\n    timeout: \"\"\n",
		"empty enabled":      "languages:\n  go:\n    enabled: \"\"\n",
		"version float":      "version: 3.0\nowners: [o]\nmodules:\n  app:\n    paths: [cmd/**]\n",
		"version 2.0":        "version: 2.0\nowners: [o]\nmodules:\n  app:\n    paths: [cmd/**]\n",
	} {
		doc := tail
		if !strings.HasPrefix(tail, "version:") {
			doc = head + tail
		}
		cases["edge/"+name] = []byte(doc)
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, decodeErr := policyfile.Decode(data)
			schemaErr := schemaAccepts(compiled, data)
			if (decodeErr == nil) != (schemaErr == nil) {
				t.Errorf("decoder and schema disagree:\nDecode: %v\nschema: %v", decodeErr, schemaErr)
			}
			if strings.HasPrefix(name, "valid/") && decodeErr != nil {
				t.Errorf("valid fixture rejected: %v", decodeErr)
			}
			if strings.HasPrefix(name, "invalid/") && decodeErr == nil {
				t.Error("invalid fixture accepted")
			}
		})
	}
}

type schemaDoc struct {
	Properties map[string]property `json:"properties"`
	Defs       map[string]struct {
		Properties map[string]property `json:"properties"`
	} `json:"$defs"`
}

type property struct {
	Description string `json:"description"`
	Default     any    `json:"default"`
	Enum        []any  `json:"enum"`
	Minimum     any    `json:"minimum"`
}

func decodeSchema(t *testing.T) schemaDoc {
	t.Helper()
	var doc schemaDoc
	if err := json.Unmarshal(generate(t), &doc); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return doc
}

// TestSchemaDescriptionsCarryTheDefaults keeps the defaults table of design 6.1
// in the published descriptions and default keywords.
func TestSchemaDescriptionsCarryTheDefaults(t *testing.T) {
	doc := decodeSchema(t)
	ask, deny := string(policyfile.ModeAsk), string(policyfile.ModeDeny)
	module := doc.Defs["Module"].Properties
	builtin := doc.Defs["Builtin"].Properties
	tests := []struct {
		key  string
		prop property
		want string
	}{
		{"depends_on", module["depends_on"], "Absent: the module may depend on no other module."},
		{"public", module["public"], "Absent: the whole module is public."},
		{"volatility", module["volatility"], "Absent: taken from subdomain (core is high; supporting and generic are low), otherwise high."},
		{"subdomain", module["subdomain"], "Absent: no business role is recorded."},
		{"layer", module["layer"], "Absent: the module has no layer."},
		{"layers", doc.Properties["layers"], "Absent: no layer rule."},
		{"gate", doc.Properties["gate"], "Absent: advisory, so the check reports and does not block."},
		{"rules", doc.Properties["rules"], "Absent: none; the built-in rules still apply."},
		{"exceptions", doc.Properties["exceptions"], "Absent: none."},
		{"generated", doc.Properties["generated"], "only listed paths are out; a file name that looks generated is not enough."},
		{"tests", doc.Properties["tests"], "only listed paths are out."},
	}
	for _, tt := range tests {
		if !strings.Contains(tt.prop.Description, tt.want) {
			t.Errorf("%s description = %q, want it to contain %q", tt.key, tt.prop.Description, tt.want)
		}
	}

	for key, want := range map[string]any{
		"graph": ask, "coupling": deny, "duplication": ask,
		"cycle": deny, "layers": deny, "unowned": deny, "min_balance": float64(7),
	} {
		if got := builtin[key].Default; got != want {
			t.Errorf("builtin.%s default = %#v, want %#v", key, got, want)
		}
	}
	if got := doc.Properties["gate"].Default; got != string(policyfile.GateAdvisory) {
		t.Errorf("gate default = %#v, want advisory", got)
	}
}

// TestSchemaEveryPropertyHasADescription fails when the doc comments stop
// reaching the schema, for example after the policyfile package moves.
func TestSchemaEveryPropertyHasADescription(t *testing.T) {
	doc := decodeSchema(t)
	for key, prop := range doc.Properties {
		if prop.Description == "" {
			t.Errorf("property %s has no description", key)
		}
	}
	for name, def := range doc.Defs {
		for key, prop := range def.Properties {
			if prop.Description == "" {
				t.Errorf("$defs.%s.%s has no description", name, key)
			}
		}
	}
}

// TestSchemaCoverageMatchesTheParser keeps the coverage keys equal to what the
// supplied-coverage parser accepts and defaults to.
func TestSchemaCoverageMatchesTheParser(t *testing.T) {
	source := decodeSchema(t).Defs["CoverageSource"].Properties
	formats := suppliedcoverage.SupportedFormats()
	want := make([]any, 0, len(formats))
	for _, format := range formats {
		want = append(want, format)
	}
	if got := source["format"].Enum; !slices.Equal(got, want) {
		t.Errorf("CoverageSource.format enum = %v, want %v", got, want)
	}
	if got := source["max_bytes"]; got.Minimum != float64(1) || got.Default != float64(suppliedcoverage.DefaultMaxBytes) {
		t.Errorf("CoverageSource.max_bytes = %+v, want minimum 1, default %d", got, suppliedcoverage.DefaultMaxBytes)
	}
	if got := source["max_facts"]; got.Minimum != float64(1) || got.Default != float64(suppliedcoverage.DefaultMaxFacts) {
		t.Errorf("CoverageSource.max_facts = %+v, want minimum 1, default %d", got, suppliedcoverage.DefaultMaxFacts)
	}
}
