package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestReportEvidenceValidatesPublishedSchema(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{defaultConfigPath: reportPolicy}), map[string]string{tcNetem: tcImportsUtil})
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir, flagFormat, formatJSON)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	raw, err := os.Open("../../archfit.report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	doc, err := jsonschema.UnmarshalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("report.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile("report.json")
	if err != nil {
		t.Fatal(err)
	}
	var actual any
	if err := json.Unmarshal([]byte(got.stdout), &actual); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(actual); err != nil {
		t.Fatalf("actual CLI output invalid: %v", err)
	}
}
