package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
)

func TestValidateConfigRulesRejectsUnknownRuleType(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nrules:\n  - id: bad\n    type: bogus_type\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateConfigRules(cfg); err == nil || !strings.Contains(err.Error(), "unknown rule type") {
		t.Fatalf("want unknown-rule-type error, got %v", err)
	}
}
