package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestReportV2RejectedByStrictV1Schema(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, fixtureFiles(), nil)
	got := runCLI(t, fixtureDate, cmdReport, "-C", dir, flagFormat, formatJSON)
	if got.code != 0 {
		t.Fatalf("exit %d: %s", got.code, got.stderr)
	}
	f, err := os.Open("../../internal/reportschema/testdata/report-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("old.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile("old.json")
	if err != nil {
		t.Fatal(err)
	}
	var actual any
	if err := json.Unmarshal([]byte(got.stdout), &actual); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(actual); err == nil {
		t.Fatal("strict v1 accepted the v2 report")
	}
}
