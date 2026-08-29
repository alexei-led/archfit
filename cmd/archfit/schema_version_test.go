package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/initcfg"
)

func TestRenderedInitConfigLoadsAtTheCurrentSchema(t *testing.T) {
	t.Parallel()
	if initcfg.TargetSchemaVersion != config.SchemaVersion {
		t.Fatalf("init schema = %d, config schema = %d", initcfg.TargetSchemaVersion, config.SchemaVersion)
	}
	body := initcfg.Render(initcfg.DiscoveredConfig{
		Modules: []initcfg.ModuleDef{{Name: "a", Paths: []string{"pkg/a/**"}}},
	}, nil, false)
	path := filepath.Join(t.TempDir(), defaultConfigPath)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("config init output does not load: %v\n%s", err, body)
	}
	if cfg.Version != config.SchemaVersion {
		t.Errorf("rendered version = %d, want %d", cfg.Version, config.SchemaVersion)
	}
}
