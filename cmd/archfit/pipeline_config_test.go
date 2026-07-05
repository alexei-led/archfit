package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadConfig must reject a config whose rules block cannot construct —
// rule types are validated by rules.New, not config.Load, so without
// validateConfigRules doctor/init/update would report a config as healthy
// that analyze later rejects.
func TestLoadConfig_RejectsUnknownRuleType(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".archfit.yaml")
	yaml := "version: 1\nrules:\n  - id: bad\n    type: bogus_type\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadConfig(context.Background(), path, false)
	if err == nil || !strings.Contains(err.Error(), "unknown rule type") {
		t.Fatalf("want unknown-rule-type error, got %v", err)
	}
}
