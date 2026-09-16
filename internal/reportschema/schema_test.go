package reportschema_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/alexei-led/archfit/internal/model/report"
	"github.com/alexei-led/archfit/internal/reportschema"
)

const (
	schemaFile = "../../archfit.state.schema.json"
	reportSrc  = "../model/report"
	// envUpdateSchema regenerates the committed schema, matching the config
	// schema's workflow so both are refreshed by one `make schema`.
	envUpdateSchema = "ARCHFIT_UPDATE_SCHEMA"
	// repoRoot is where the walk for real state documents starts.
	repoRoot = "../.."
)

// TestStateSchemaNoDrift verifies that the committed archfit.state.schema.json
// matches what the current report structs generate. A published contract that
// silently stops describing the emitted document is worse than no contract:
// consumers generate types from it.
func TestStateSchemaNoDrift(t *testing.T) {
	got, err := reportschema.Generate(reportSrc)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if os.Getenv(envUpdateSchema) != "" {
		if err := os.WriteFile(schemaFile, got, 0o600); err != nil {
			t.Fatalf("write schema: %v", err)
		}
		t.Logf("schema written to %s — review and commit", schemaFile)
		return
	}

	want, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatalf("read %s (regenerate with `make schema`): %v", schemaFile, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("schema drift — run `make schema` to regenerate %s", schemaFile)
	}
}

// TestStateSchemaDeclaresTheContractVocabulary pins the parts a consumer
// validates against rather than merely parses: the document's own version and
// the closed verdict set. Generating a schema is not the same as publishing
// one that can reject a typo.
func TestStateSchemaDeclaresTheContractVocabulary(t *testing.T) {
	raw, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatalf("read %s: %v", schemaFile, err)
	}
	var doc struct {
		Properties struct {
			SchemaVersion struct {
				Enum []string `json:"enum"`
			} `json:"schema_version"`
			Verdict struct {
				Enum []string `json:"enum"`
			} `json:"verdict"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	if want := []string{report.StateSchemaVersion}; !slices.Equal(doc.Properties.SchemaVersion.Enum, want) {
		t.Errorf("schema_version enum = %v, want %v", doc.Properties.SchemaVersion.Enum, want)
	}
	want := []string{
		string(report.StateHealthy),
		string(report.StateNeedsAttention),
		string(report.StateBlocked),
	}
	if !slices.Equal(doc.Properties.Verdict.Enum, want) {
		t.Errorf("verdict enum = %v, want %v", doc.Properties.Verdict.Enum, want)
	}
}

// TestStateSchemaAcceptsEveryCommittedStateDocument validates the real
// architecture-state documents committed in this repo against the published
// schema.
//
// This is the half TestStateSchemaNoDrift cannot cover. No-drift proves the
// schema matches the Go structs; it does not prove the schema ACCEPTS what
// archfit actually writes — a wrongly required field, or a nullable the
// reflector rendered as non-nullable, would ship a contract that rejects our
// own output. The documents are discovered rather than listed so a baseline
// added later is covered without editing this test.
func TestStateSchemaAcceptsEveryCommittedStateDocument(t *testing.T) {
	compiled := compileStateSchema(t)
	docs := findStateDocuments(t)
	if len(docs) == 0 {
		t.Fatal("no committed architecture-state documents found — this test would pass vacuously")
	}

	for _, path := range docs {
		t.Run(filepath.ToSlash(mustRel(t, path)), func(t *testing.T) {
			raw, err := os.ReadFile(path) //nolint:gosec // path comes from the repo walk below
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var instance any
			if err := json.Unmarshal(raw, &instance); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if err := compiled.Validate(instance); err != nil {
				t.Errorf("committed state document does not validate against the published schema:\n%v", err)
			}
		})
	}
}

func compileStateSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	raw, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatalf("read %s: %v", schemaFile, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	const resource = "archfit.state.schema.json"
	if err := c.AddResource(resource, doc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	compiled, err := c.Compile(resource)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return compiled
}

// findStateDocuments walks the repo for JSON files whose ROOT declares the
// state contract's schema_version. Matching on the declared version rather than
// on a filename convention means a document only counts when it claims to BE
// one, and nesting the state inside another document (SARIF) does not qualify.
func findStateDocuments(t *testing.T) []string {
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
		// The ROOT must claim the contract. Matching the version string anywhere
		// in the file also catches SARIF, which carries the state nested under
		// run.properties and is a different document with its own schema.
		var root struct {
			SchemaVersion string `json:"schema_version"`
		}
		if json.Unmarshal(raw, &root) == nil && root.SchemaVersion == report.StateSchemaVersion {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
	return found
}

func mustRel(t *testing.T, path string) string {
	t.Helper()

	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return path
	}
	return rel
}
