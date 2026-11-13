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
)

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
