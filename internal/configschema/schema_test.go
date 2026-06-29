package configschema_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/configschema"
)

// schemaFile is the committed schema path, relative to the repo root.
// The test is run with cwd = internal/configschema, so ../../ reaches the root.
const schemaFile = "../../archfit.schema.json"

// TestSchemaNoDrift verifies that the committed archfit.schema.json is in sync
// with what Generate produces from the current internal/config structs.
//
// To update the committed file: ARCHFIT_UPDATE_SCHEMA=1 go test ./internal/configschema/ -run TestSchemaNoDrift -count=1
func TestSchemaNoDrift(t *testing.T) {
	got, err := configschema.Generate("../config")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if os.Getenv("ARCHFIT_UPDATE_SCHEMA") == "1" {
		abs, _ := filepath.Abs(schemaFile)
		if err := os.WriteFile(abs, got, 0o600); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
		t.Logf("wrote %s (%d bytes)", abs, len(got))
		return
	}

	want, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatalf("read %s: %v — run `make schema` to generate it", schemaFile, err)
	}

	if string(got) != string(want) {
		t.Errorf("schema drift — run `make schema` to regenerate archfit.schema.json\n"+
			"got %d bytes, want %d bytes", len(got), len(want))
	}
}
