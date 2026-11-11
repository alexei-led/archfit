package record_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/output/record"
)

const resultSchemaFile = "../../../archfit.result.schema.json"

// checkExamples are the examples of design 8.1 that are check results.
var checkExamples = []string{"pass", "deny", ruleGraph, "duplication", "policy_differs", "expired_exception"}

// resultExamples are the result goldens of the contract: the check examples
// of design 8.1 and the App's waiting document.
var resultExamples = append(slices.Clone(checkExamples), "waiting")

// TestJSONIsTheContractExamples renders every golden of the result contract
// from its own decoded JSON and expects the same bytes back, valid against
// the published schema.
func TestJSONIsTheContractExamples(t *testing.T) {
	schema := compileResultSchema(t)
	for _, name := range resultExamples {
		t.Run(name, func(t *testing.T) {
			var r result.Result
			decodeExample(t, name, &r)
			var buf bytes.Buffer
			if err := record.ResultJSON(&buf, r); err != nil {
				t.Fatalf("ResultJSON: %v", err)
			}
			assertExampleBytes(t, name, buf.Bytes())
			assertValid(t, schema, buf.Bytes())
		})
	}
	t.Run("can", func(t *testing.T) {
		var a result.CanAnswer
		decodeExample(t, "can", &a)
		var buf bytes.Buffer
		if err := record.AnswerJSON(&buf, a); err != nil {
			t.Fatalf("AnswerJSON: %v", err)
		}
		assertExampleBytes(t, "can", buf.Bytes())
		assertValid(t, schema, buf.Bytes())
	})
}

func TestResultJSONWritesEmptyListsNotNull(t *testing.T) {
	var r result.Result
	decodeExample(t, "pass", &r)
	r.Errors, r.Unknown = nil, nil
	var buf bytes.Buffer
	if err := record.ResultJSON(&buf, r); err != nil {
		t.Fatalf("ResultJSON: %v", err)
	}
	for _, want := range []string{`"errors": []`, `"unknown": []`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("JSON lacks %s:\n%s", want, buf.String())
		}
	}
	assertValid(t, compileResultSchema(t), buf.Bytes())
}

func TestJSONAndSARIFReturnWriteErrors(t *testing.T) {
	for name, write := range map[string]func() error{
		"ResultJSON":  func() error { return record.ResultJSON(failingWriter{}, result.Result{}) },
		"AnswerJSON":  func() error { return record.AnswerJSON(failingWriter{}, result.CanAnswer{}) },
		"ResultSARIF": func() error { return record.ResultSARIF(failingWriter{}, result.Result{}) },
	} {
		if err := write(); !errors.Is(err, errWrite) {
			t.Errorf("%s error = %v, want %v", name, err, errWrite)
		}
	}
}

func assertExampleBytes(t *testing.T, name string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join(contractExamples, name+".json")) //nolint:gosec // fixed example names
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("JSON differs from the contract example %s:\n--- got\n%s--- example\n%s", name, got, want)
	}
}

func compileResultSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile(resultSchemaFile)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("archfit.result.schema.json", doc); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	schema, err := c.Compile("archfit.result.schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return schema
}

func assertValid(t *testing.T, schema *jsonschema.Schema, doc []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if err := schema.Validate(v); err != nil {
		t.Errorf("output does not validate against %s:\n%v", resultSchemaFile, err)
	}
}
