package reportschema_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/result"
	"github.com/alexei-led/archfit/v3/internal/reportschema"
)

const (
	resultSchemaFile = "../../archfit.result.schema.json"
	resultSrc        = "../model/result"
	resultGoldens    = "../model/result/testdata"
	goldenCan        = "can"
	goldenGraph      = "graph"
	goldenPass       = "pass"
	goldenClean      = "repository_clean"
	goldenDrift      = "repository_drift"
)

// TestResultSchemaNoDrift verifies that the committed archfit.result.schema.json
// matches what the internal/model/result types generate.
func TestResultSchemaNoDrift(t *testing.T) {
	got, err := reportschema.GenerateResult(resultSrc)
	if err != nil {
		t.Fatalf("GenerateResult: %v", err)
	}
	if os.Getenv(envUpdateSchema) != "" {
		if err := os.WriteFile(resultSchemaFile, got, 0o600); err != nil {
			t.Fatalf("write schema: %v", err)
		}
		t.Logf("schema written to %s — review and commit", resultSchemaFile)
		return
	}
	want, err := os.ReadFile(resultSchemaFile)
	if err != nil {
		t.Fatalf("read %s (regenerate with `make schema`): %v", resultSchemaFile, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("schema drift — run `make schema` to regenerate %s", resultSchemaFile)
	}
}

// TestResultSchemaAcceptsEveryGolden validates the golden of every example of
// design 8.1, and the App's waiting document, against the published schema.
func TestResultSchemaAcceptsEveryGolden(t *testing.T) {
	compiled := compileSchema(t, resultSchemaFile, "archfit.result.schema.json")
	paths, err := filepath.Glob(filepath.Join(resultGoldens, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		goldenCan, "deny", "duplication", "expired_exception", goldenGraph, goldenPass, "policy_differs",
		goldenClean, goldenDrift, "repository_expired", "waiting",
	}
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, strings.TrimSuffix(filepath.Base(path), ".json"))
	}
	if !slices.Equal(names, want) {
		t.Fatalf("goldens = %v, want %v", names, want)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if err := compiled.Validate(readJSON(t, path)); err != nil {
				t.Errorf("golden does not validate against the published schema:\n%v", err)
			}
		})
	}
}

// TestResultSchemaRejectsBrokenDocuments changes one thing in a valid golden
// and expects the schema to refuse it: a contract that only parses JSON is no
// contract.
func TestResultSchemaRejectsBrokenDocuments(t *testing.T) {
	compiled := compileSchema(t, resultSchemaFile, "archfit.result.schema.json")
	tests := []struct {
		name   string
		golden string
		change func(doc map[string]any)
	}{
		{"unknown status", goldenGraph, func(d map[string]any) { d["status"] = "blocked" }},
		{"wrong schema id", goldenGraph, func(d map[string]any) { d["schema"] = "archfit.result.v2" }},
		{"can schema on a result", goldenGraph, func(d map[string]any) { d["schema"] = result.CanSchemaVersion }},
		{"result schema on a can answer", goldenCan, func(d map[string]any) { d["schema"] = result.SchemaVersion }},
		{"extra key", goldenGraph, func(d map[string]any) { d["verdict"] = true }},
		{"extra key in an error", goldenGraph, func(d map[string]any) { firstError(d)["severity"] = "high" }},
		{"missing rule", goldenGraph, func(d map[string]any) { delete(firstError(d), "rule") }},
		{"missing from_change", goldenGraph, func(d map[string]any) { delete(firstError(d), "from_change") }},
		{"balance 11", goldenGraph, func(d map[string]any) { balance(firstError(d))["value"] = 11 }},
		{"balance 0", goldenGraph, func(d map[string]any) { balance(firstError(d))["value"] = 0 }},
		{"min_balance 0", goldenGraph, func(d map[string]any) { balance(firstError(d))["min"] = 0 }},
		{"line 0", goldenGraph, func(d map[string]any) { firstError(d)["line"] = 0 }},
		{"next kind ask", goldenGraph, func(d map[string]any) { firstError(d)["next"].(map[string]any)["kind"] = "ask" }},
		{"bad digest", goldenGraph, func(d map[string]any) { d["policy"].(map[string]any)["digest"] = "md5:abc" }},
		{"bad date", goldenGraph, func(d map[string]any) { d["date"] = "10 Nov 2026" }},
		{"bad commit", goldenGraph, func(d map[string]any) { d["head"] = "HEAD" }},
		{"null errors", goldenPass, func(d map[string]any) { d["errors"] = nil }},
		{"unknown cause", "policy_differs", func(d map[string]any) {
			d["unknown"].([]any)[0].(map[string]any)["cause"] = "config"
		}},
		{"bad exception date", "expired_exception", func(d map[string]any) {
			firstError(d)["exception"].(map[string]any)["expires"] = "soon"
		}},
		{"answer maybe", goldenCan, func(d map[string]any) { d["answer"] = "maybe" }},
		{"unknown repository state", goldenClean, func(d map[string]any) { repository(d)["state"] = "policy_changed" }},
		{"null drift", goldenClean, func(d map[string]any) { repository(d)["drift"] = nil }},
		{"missing expiring", goldenClean, func(d map[string]any) { delete(repository(d), "expiring") }},
		{"extra key in the repository", goldenClean, func(d map[string]any) { repository(d)["unread"] = []any{} }},
		{"bad expiring date", goldenClean, func(d map[string]any) {
			repository(d)["expiring"].([]any)[0].(map[string]any)["expires"] = "in 9 days"
		}},
		{"drift without from_change", goldenDrift, func(d map[string]any) {
			delete(repository(d)["drift"].([]any)[0].(map[string]any), "from_change")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := readJSON(t, filepath.Join(resultGoldens, tc.golden+".json")).(map[string]any)
			tc.change(doc)
			if err := compiled.Validate(doc); err == nil {
				t.Error("schema accepted a broken document")
			}
		})
	}
}

// TestResultSchemaDeclaresTheStatuses pins the four statuses and that waiting
// is documented as the App's: the engine never emits it.
func TestResultSchemaDeclaresTheStatuses(t *testing.T) {
	var doc struct {
		Defs struct {
			Result struct {
				Properties struct {
					Status struct {
						Enum        []string `json:"enum"`
						Description string   `json:"description"`
					} `json:"status"`
				} `json:"properties"`
			} `json:"Result"`
		} `json:"$defs"`
	}
	raw, err := os.ReadFile(resultSchemaFile)
	if err != nil {
		t.Fatalf("read %s: %v", resultSchemaFile, err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	status := doc.Defs.Result.Properties.Status
	if want := []string{
		string(result.StatusPass), string(result.StatusFail), string(result.StatusWaiting), string(result.StatusUnknown),
	}; !slices.Equal(status.Enum, want) {
		t.Errorf("status enum = %v, want %v", status.Enum, want)
	}
	if !strings.Contains(status.Description, "The App sets waiting") {
		t.Errorf("status description = %q, want it to say the App sets waiting", status.Description)
	}
}

func repository(d map[string]any) map[string]any {
	return d["repository"].(map[string]any)
}

func readJSON(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // fixed test paths
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return v
}

func firstError(doc map[string]any) map[string]any {
	return doc["errors"].([]any)[0].(map[string]any)
}

func balance(e map[string]any) map[string]any {
	return e["balance"].(map[string]any)
}
