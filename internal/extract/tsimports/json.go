package tsimports

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"regexp"
	"strings"
)

// object is a JSON object that keeps its key order. The order decides: the
// first active `exports` condition that names a file wins, and so does the
// first of two `paths` patterns with the same prefix.
type object struct {
	keys []string
	vals map[string]any
}

// get returns the value of key, or nil when the object is nil or lacks it.
func (o *object) get(key string) any {
	if o == nil {
		return nil
	}
	return o.vals[key]
}

// has reports whether the object has key, even with a null value.
func (o *object) has(key string) bool {
	if o == nil {
		return false
	}
	_, ok := o.vals[key]
	return ok
}

// asObject returns v as an object, or nil when it is not one.
func asObject(v any) *object {
	o, _ := v.(*object)
	return o
}

// asString returns v as a string and whether it is one.
func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// decodeJSON parses one JSON value. Objects become *object, arrays []any,
// numbers float64. A duplicate key keeps its first position and its last
// value, as Python's json module does.
func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("tsimports: data after the JSON value")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		o := &object{vals: map[string]any{}}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := keyTok.(string)
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			if _, dup := o.vals[key]; !dup {
				o.keys = append(o.keys, key)
			}
			o.vals[key] = v
		}
		_, err := dec.Token()
		return o, err
	case '[':
		arr := []any{}
		for dec.More() {
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token()
		return arr, err
	default:
		return nil, errors.New("tsimports: unexpected JSON delimiter")
	}
}

// readJSON returns the object a JSON file holds; a missing, unreadable or
// invalid file, or one that holds no object, gives nil.
func readJSON(fsys fs.FS, name string) *object {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil
	}
	v, err := decodeJSON(bytes.TrimPrefix(data, []byte(byteOrderMark)))
	if err != nil {
		return nil
	}
	return asObject(v)
}

// byteOrderMark may start a file; Node and TypeScript drop it before parsing.
const byteOrderMark = "\uFEFF"

var (
	// A string is matched first, so comment markers inside it stay.
	jsoncComment       = regexp.MustCompile(`"(?:\\.|[^"\\])*"|//[^\n]*|(?s:/\*.*?\*/)`)
	jsoncTrailingComma = regexp.MustCompile(`"(?:\\.|[^"\\])*"|,\s*[}\]]`)
)

// readJSONC reads a JSON file with comments and trailing commas, as tsconfig
// allows. Comments go first, so a comma before a comment still trails. A file
// with nothing but comments and space is an empty object, as in TypeScript.
func readJSONC(fsys fs.FS, name string) *object {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil
	}
	text := jsoncComment.ReplaceAllStringFunc(strings.TrimPrefix(string(data), byteOrderMark), func(m string) string {
		if strings.HasPrefix(m, `"`) {
			return m
		}
		return ""
	})
	text = jsoncTrailingComma.ReplaceAllStringFunc(text, func(m string) string {
		if strings.HasPrefix(m, `"`) {
			return m
		}
		return m[1:]
	})
	if strings.TrimSpace(text) == "" {
		return &object{vals: map[string]any{}}
	}
	v, err := decodeJSON([]byte(text))
	if err != nil {
		return nil
	}
	return asObject(v)
}
