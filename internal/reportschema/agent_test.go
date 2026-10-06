package reportschema_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/output/agentout"
	"github.com/alexei-led/archfit/internal/reportschema"
)

const (
	agentSourceFile = "a/a.go"
	agentSchemaFile = "../../archfit.agent-result.schema.json"
	agentSrc        = "../output/agentout"
)

// TestAgentResultSchemaNoDrift verifies that the committed
// archfit.agent-result.schema.json matches what the agentout structs generate.
func TestAgentResultSchemaNoDrift(t *testing.T) {
	got, err := reportschema.GenerateAgentResult(agentSrc)
	if err != nil {
		t.Fatalf("GenerateAgentResult: %v", err)
	}
	if os.Getenv(envUpdateSchema) != "" {
		if err := os.WriteFile(agentSchemaFile, got, 0o600); err != nil {
			t.Fatalf("write schema: %v", err)
		}
		t.Logf("schema written to %s — review and commit", agentSchemaFile)
		return
	}
	want, err := os.ReadFile(agentSchemaFile)
	if err != nil {
		t.Fatalf("read %s (regenerate with `make schema`): %v", agentSchemaFile, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("schema drift — run `make schema` to regenerate %s", agentSchemaFile)
	}
}

// TestAgentResultSchemaAcceptsRenderedResults validates rendered results that
// fill every optional field — origin, truncation, gaps, unevaluated rules, and
// worsened metrics — and every committed agent-result document.
func TestAgentResultSchemaAcceptsRenderedResults(t *testing.T) {
	compiled := compileSchema(t, agentSchemaFile, "archfit.agent-result.schema.json")

	rich := report.NewDocument()
	rich.State.Verdict = report.StateBlocked
	rich.State.Findings = []report.Finding{{
		ID: "f1", Kind: report.FindingKindGate, RuleID: "no_x", Status: report.FindingStatusNew, Severity: report.FindingSeverityHigh,
		Edge:      report.FindingEdge{From: report.FindingEndpoint{Path: agentSourceFile}, To: report.FindingEndpoint{Path: "b"}, Kind: "imports"},
		Locations: []report.Location{{File: agentSourceFile, Line: 3}},
	}}
	rich.State.AgentTasks = []report.AgentTask{{
		FindingID: "f1", RuleID: "no_x", RepairKind: "code_change", Origin: "introduced",
		Goal: strings.Repeat("goal ", 200), Constraints: []string{"c"}, Files: []string{agentSourceFile},
		Validation: []string{"archfit check -c .archfit.yaml"},
	}}
	rich.State.Decision.UnevaluatedRequiredRules = []report.UnevaluatedRule{{RuleID: "r", Reason: "go/packages evidence is partial"}}
	rich.CoverageGaps = []report.CoverageGap{{Tool: "sg", Gate: "fail", InstallCmd: "brew install ast-grep"}}
	rich.Metrics = []report.MetricResult{{Name: "coverage", Value: 0.5, Delta: new(-0.25)}}

	empty := report.NewDocument()
	empty.State.Verdict = report.StateNeedsAttention
	docs := map[string][]byte{"rendered/empty": render(t, empty), "rendered/rich": render(t, rich)}
	for _, path := range findDocuments(t, agentout.SchemaVersion) {
		raw, err := os.ReadFile(path) //nolint:gosec // path comes from the repo walk
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		docs[filepath.ToSlash(mustRel(t, path))] = raw
	}
	if len(docs) < 3 {
		t.Fatal("no committed agent-result document found: the format-matrix baseline is missing")
	}
	for name, raw := range docs {
		t.Run(name, func(t *testing.T) {
			var instance any
			if err := json.Unmarshal(raw, &instance); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if err := compiled.Validate(instance); err != nil {
				t.Errorf("agent result does not validate against the published schema:\n%v", err)
			}
		})
	}
}

func render(t *testing.T, d report.Document) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := agentout.New().Render(d, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.Bytes()
}

func compileSchema(t *testing.T, file, resource string) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile(file) //nolint:gosec // fixed repo path
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(resource, doc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	compiled, err := c.Compile(resource)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return compiled
}

// findDocuments walks the repo for JSON files whose root declares version.
func findDocuments(t *testing.T, version string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == ".archfit-cache" || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		raw, readErr := os.ReadFile(path) //nolint:gosec // repo walk
		if readErr != nil {
			return nil
		}
		var root struct {
			SchemaVersion string `json:"schema_version"`
		}
		if json.Unmarshal(raw, &root) == nil && root.SchemaVersion == version {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
	return found
}
