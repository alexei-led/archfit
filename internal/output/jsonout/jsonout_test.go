package jsonout_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/output/jsonout"
)

func TestRendererFormat(t *testing.T) {
	if got := jsonout.New().Format(); got != "json" {
		t.Fatalf("Format() = %q, want json", got)
	}
}

func TestRendererWritesArchitectureStateAtRoot(t *testing.T) {
	d := report.NewDocument()
	d.State.Verdict = report.StateNeedsAttention

	var buf bytes.Buffer
	if err := jsonout.New().Render(d, &buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &root); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"schema_version", "verdict", "decision", "comparison", "measurement", "dimensions", "coverage", "findings", "agent_tasks", "seams"} {
		if _, ok := root[key]; !ok {
			t.Errorf("missing root key %q", key)
		}
	}
	if _, ok := root["score"]; ok {
		t.Error("legacy score field must not be present")
	}
	if string(root["verdict"]) != `"needs_attention"` {
		t.Errorf("verdict = %s", root["verdict"])
	}
}

func TestRendererIsByteStable(t *testing.T) {
	d := report.NewDocument()
	var first, second bytes.Buffer
	if err := jsonout.New().Render(d, &first); err != nil {
		t.Fatal(err)
	}
	if err := jsonout.New().Render(d, &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("repeated renders differ")
	}
}
