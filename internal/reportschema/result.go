package reportschema

import (
	"encoding/json"
	"fmt"
	"maps"
	"strconv"

	"github.com/invopop/jsonschema"

	"github.com/alexei-led/archfit/v3/internal/model/result"
)

const resultSchemaID = "https://raw.githubusercontent.com/alexei-led/archfit/main/archfit.result.schema.json"

// Patterns the result contract pins on its strings.
const (
	datePattern   = `^[0-9]{4}-[0-9]{2}-[0-9]{2}$`
	digestPattern = `^sha256:[0-9a-f]{64}$`
	// commitPattern accepts SHA-1 and SHA-256 object names.
	commitPattern = `^[0-9a-f]{40}([0-9a-f]{24})?$`
)

// Definition names the reflector gives the internal/model/result types.
const (
	defResult    = "Result"
	defCanAnswer = "CanAnswer"
	defError     = "Error"
	defUnknown   = "Unknown"
	defNext      = "Next"
	defBalance   = "Balance"
	defException = "Exception"
	defPolicy    = "Policy"
)

// GenerateResult produces the JSON Schema bytes for the Archfit 4 result
// contract: the result.Result document that `archfit check --format json`
// prints and the result.CanAnswer document that `archfit can --format json`
// prints. The root is oneOf the two; each document's schema field tells them
// apart.
//
// srcDir is the filesystem path to internal/model/result so AddGoComments can
// parse doc-comments into schema descriptions; tests pass "../model/result".
// A field is required unless it carries `omitempty`, as in the state schema.
func GenerateResult(srcDir string) ([]byte, error) {
	r := &jsonschema.Reflector{}
	// AddGoComments keys each comment by path.Join(base, the walked dir), so
	// base is this package's import path: srcDir is relative to it.
	if err := r.AddGoComments("github.com/alexei-led/archfit/v3/internal/reportschema", srcDir); err != nil {
		return nil, fmt.Errorf("reportschema: AddGoComments: %w", err)
	}

	check := r.Reflect(&result.Result{})
	can := r.Reflect(&result.CanAnswer{})
	defs := jsonschema.Definitions{}
	maps.Copy(defs, check.Definitions)
	maps.Copy(defs, can.Definitions)

	schema := &jsonschema.Schema{
		Version:     schemaDraft,
		ID:          jsonschema.ID(resultSchemaID),
		Description: "The result of archfit check (schema archfit.result.v1) or the answer of archfit can (schema archfit.can.v1).",
		OneOf:       []*jsonschema.Schema{{Ref: check.Ref}, {Ref: can.Ref}},
		Definitions: defs,
	}
	if err := patchResultDefinitions(defs); err != nil {
		return nil, err
	}

	buf, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("reportschema: marshal: %w", err)
	}
	return append(buf, '\n'), nil
}

// patchResultDefinitions pins the closed vocabularies, bounds and string
// shapes that the Go types express as named strings and plain ints. Each list
// mirrors the constants in internal/model/result. A renamed type or field
// returns an error instead of a schema that silently lost a constraint.
func patchResultDefinitions(defs jsonschema.Definitions) error {
	var missing []string
	property := func(def, name string) *jsonschema.Schema {
		if d, ok := defs[def]; ok && d.Properties != nil {
			if p, ok := d.Properties.Get(name); ok {
				return p
			}
		}
		missing = append(missing, def+"."+name)
		return &jsonschema.Schema{}
	}
	enum := func(def, name string, values ...any) { property(def, name).Enum = values }
	pattern := func(def, name, re string) { property(def, name).Pattern = re }
	between := func(def, name string, lo, hi int) {
		p := property(def, name)
		p.Minimum = json.Number(strconv.Itoa(lo))
		p.Maximum = json.Number(strconv.Itoa(hi))
	}

	enum(defResult, "schema", result.SchemaVersion)
	status := property(defResult, "status")
	status.Enum = []any{
		string(result.StatusPass), string(result.StatusFail),
		string(result.StatusWaiting), string(result.StatusUnknown),
	}
	status.Description = "The status of the change. The App sets waiting; the engine never emits it."
	pattern(defResult, "date", datePattern)
	pattern(defResult, "base", commitPattern)
	pattern(defResult, "head", commitPattern)

	enum(defCanAnswer, "schema", result.CanSchemaVersion)
	enum(defCanAnswer, "answer", string(result.ReplyYes), string(result.ReplyNo), string(result.ReplyUnknown))
	pattern(defCanAnswer, "date", datePattern)
	pattern(defCanAnswer, "head", commitPattern)

	property(defError, "line").Minimum = "1"
	property(defUnknown, "line").Minimum = "1"
	enum(defUnknown, "cause", string(result.CausePolicy), string(result.CauseBase), string(result.CauseTool), string(result.CauseImport))
	enum(defNext, "kind", string(result.NextFix), string(result.NextCommand), string(result.NextWait), string(result.NextRestore))
	between(defBalance, "value", 1, 10)
	between(defBalance, "min", 1, 10)
	pattern(defException, "expires", datePattern)
	pattern(defPolicy, "digest", digestPattern)

	one := uint64(1)
	for _, f := range [][2]string{
		{defResult, "summary"}, {defResult, "engine"}, {defCanAnswer, "from"}, {defCanAnswer, "to"}, {defCanAnswer, "engine"},
		{defError, "path"}, {defError, "rule"}, {defError, "what"}, {defError, "key"},
		{defUnknown, "what"}, {defNext, "text"}, {defPolicy, "path"}, {defException, "rule"},
	} {
		property(f[0], f[1]).MinLength = &one
	}
	if len(missing) > 0 {
		return fmt.Errorf("reportschema: result contract has no %v", missing)
	}
	return nil
}
