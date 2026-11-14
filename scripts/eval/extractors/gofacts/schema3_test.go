package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsUseSchema3Settings(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		".archfit.yaml": "version: 3\nowners: [owner]\nexclude: [skip/**]\nmodules: {app: {paths: ['src/**']}}\nlanguages: {python: {package: shop}}\n",
	})
	path := filepath.Join(root, ".archfit.yaml")
	var out bytes.Buffer
	if err := run(t.Context(), []string{"modules", "-config", path, "-lang", "typescript"}, strings.NewReader(`["src/app.ts","other.ts"]`), &out); err != nil {
		t.Fatal(err)
	}
	var modules map[string]string
	if err := json.Unmarshal(out.Bytes(), &modules); err != nil {
		t.Fatal(err)
	}
	if modules["src/app.ts"] != "app" || modules["other.ts"] != "" {
		t.Fatalf("modules = %v", modules)
	}
	cfg, note, err := loadConfig(t.Context(), path)
	if err != nil || note != "" || cfg.ForExtract("python").PyPackage != "shop" || len(cfg.Exclude) != 1 || cfg.Exclude[0] != "skip/**" {
		t.Fatalf("config=%+v note=%q error=%v", cfg, note, err)
	}
}

func TestReferenceRejectsRetiredProducers(t *testing.T) {
	t.Parallel()
	for _, lang := range []string{"typescript", "python"} {
		t.Run(lang, func(t *testing.T) {
			var out bytes.Buffer
			err := run(t.Context(), []string{"reference", "-lang", lang, "-root", t.TempDir()}, strings.NewReader(""), &out)
			if err == nil || !strings.Contains(err.Error(), "removed") || out.Len() != 0 {
				t.Fatalf("error %v, output %q", err, out.String())
			}
		})
	}
}
