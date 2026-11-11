package policyfile

import (
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
)

var (
	namesType  = reflect.TypeFor[Names]()
	switchType = reflect.TypeFor[Switch]()
)

// checkKinds walks the raw YAML tree beside the File type and reports a value of
// the wrong kind. The YAML decoder converts scalars silently (12 into a string
// field, "7" into an int field) and reads a null as an absent key; the published
// schema rejects all three, so the decoder must too.
func checkKinds(p *problems, v any, t reflect.Type, path string) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v == nil {
		p.add(path, "empty value: give a value or remove the key")
		return
	}
	switch t {
	case namesType:
		if list, ok := v.([]any); ok {
			checkKinds(p, list, reflect.TypeFor[[]string](), path)
			return
		}
		expectKind(p, path, v, "a name or a list of names", isString)
		return
	case switchType:
		expectKind(p, path, v, `true, false or "auto"`, func(v any) bool { return isString(v) || isBool(v) })
		return
	}
	switch t.Kind() {
	case reflect.String:
		expectKind(p, path, v, "a string", isString)
	case reflect.Int, reflect.Int64:
		expectKind(p, path, v, "a whole number", isInteger)
	case reflect.Bool:
		expectKind(p, path, v, "true or false", isBool)
	case reflect.Slice:
		list, ok := v.([]any)
		if !ok {
			p.add(path, "want a list")
			return
		}
		for i, item := range list {
			checkKinds(p, item, t.Elem(), fmt.Sprintf("%s[%d]", path, i))
		}
	case reflect.Map:
		m, ok := v.(map[string]any)
		if !ok {
			p.add(path, "want a mapping")
			return
		}
		for _, key := range slices.Sorted(maps.Keys(m)) {
			checkKinds(p, m[key], t.Elem(), joinPath(path, key))
		}
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			p.add(path, "want a mapping")
			return
		}
		fields := yamlFields(t)
		for _, key := range slices.Sorted(maps.Keys(m)) {
			// Unknown keys already failed the strict decode.
			if field, ok := fields[key]; ok {
				checkKinds(p, m[key], field.Type, joinPath(path, key))
			}
		}
	}
}

func expectKind(p *problems, path string, v any, want string, ok func(any) bool) {
	if !ok(v) {
		p.add(path, "%v is not %s", yamlValue(v), want)
	}
}

func isString(v any) bool { _, ok := v.(string); return ok }

func isBool(v any) bool { _, ok := v.(bool); return ok }

// isInteger follows JSON Schema: a number with no fraction is an integer.
func isInteger(v any) bool {
	switch n := v.(type) {
	case uint64, int64:
		return true
	case float64:
		return n == math.Trunc(n) && !math.IsInf(n, 0)
	}
	return false
}

// yamlFields maps each YAML key of a struct to its field.
func yamlFields(t reflect.Type) map[string]reflect.StructField {
	fields := make(map[string]reflect.StructField, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		fields[name] = f
	}
	return fields
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
