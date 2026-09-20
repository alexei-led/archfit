package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/scope"
)

func TestMeasurementConfigHash_CheckoutIndependentAndTracksInheritedConfig(t *testing.T) {
	config := map[string]string{
		tsconfigName:          `{"extends":"./config/aliases.json"}`,
		"config/aliases.json": `{"compilerOptions":{"baseUrl":"..","paths":{"@chosen":["allowed.js"]}}}`,
	}
	root := writeTSFixture(t, "export const a = 1", config)
	otherRoot := writeTSFixture(t, "export const a = 2", config)
	hash := func(root string) string {
		t.Helper()
		value, err := ts.MeasurementConfigHash(scope.Scope{Root: root}, evidenceports.ExtractConfig{})
		if err != nil || value == "" {
			t.Fatalf("configuration identity=%q error=%v", value, err)
		}
		return value
	}
	before := hash(root)
	if before != hash(otherRoot) {
		t.Fatal("checkout root or source content changed configuration identity")
	}
	if err := os.WriteFile(filepath.Join(root, "config", "aliases.json"), []byte(`{"compilerOptions":{"baseUrl":"..","paths":{"@chosen":["forbidden.js"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if before == hash(root) {
		t.Fatal("inherited compiler configuration change did not change identity")
	}
	if err := os.WriteFile(filepath.Join(otherRoot, ".dependency-cruiser.json"), []byte(`{"options":{"doNotFollow":{"path":"node_modules"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if before == hash(otherRoot) {
		t.Fatal("dependency-cruiser configuration change did not change identity")
	}
}

func TestMeasurementConfigHash_JSONResolverOptions(t *testing.T) {
	const configName = ".dependency-cruiser.json"
	for name, options := range map[string]string{
		"webpack": `{"webpackConfig":{"fileName":"../webpack.config.js","env":{"production":true}}}`,
		"babel":   `{"babelConfig":{"fileName":"../babel.config.json"}}`,
		"plugins": `{"enhancedResolveOptions":{"plugins":["resolver-plugin"]}}`,
		"pnp":     `{"externalModuleResolutionStrategy":"yarn-pnp"}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTSFixture(t, "export const a = 1", map[string]string{
				configName: `{"options":` + options + `}`,
			})
			s := scope.Scope{Root: root}
			cfg := evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."}
			if hash, err := ts.MeasurementConfigHash(s, cfg); hash != "" || err == nil || !strings.Contains(err.Error(), "configuration inputs unresolved") {
				t.Fatalf("opaque resolver must return unknown identity: hash=%q error=%v", hash, err)
			}
			calls := 0
			ex := ts.New(cacheFixtureRunner(`{"modules":[{"source":"a.ts","dependencies":[]}]}`, &calls), cfg)
			ex.Cache = factcache.NewStore(t.TempDir())
			for range 2 {
				if _, _, err := ex.Extract(context.Background(), s); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 2 {
				t.Fatalf("opaque resolver inputs must bypass cache: calls=%d", calls)
			}
		})
	}
	t.Run("ordinary enhanced resolver options", func(t *testing.T) {
		root := writeTSFixture(t, "export const a = 1", map[string]string{
			configName: `{"options":{"enhancedResolveOptions":{"extensions":[".js",".ts"],"mainFields":["module","main"]}}}`,
		})
		if hash, err := ts.MeasurementConfigHash(scope.Scope{Root: root}, evidenceports.ExtractConfig{}); hash == "" || err != nil {
			t.Fatalf("ordinary resolver options must retain known identity: hash=%q error=%v", hash, err)
		}
	})
}

func TestMeasurementConfigHash_UnresolvedConfigurationIsUnknown(t *testing.T) {
	for name, extra := range map[string]map[string]string{
		"dynamic":    {".dependency-cruiser.cjs": `module.exports = require("./shared.cjs")`},
		"inherited":  {tsconfigName: `{"extends":"@example/compiler-config"}`},
		"indirected": {".dependency-cruiser.json": `{"extends":"./shared.json"}`},
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTSFixture(t, "export const a = 1", extra)
			if hash, err := ts.MeasurementConfigHash(scope.Scope{Root: root}, evidenceports.ExtractConfig{}); err == nil || hash != "" {
				t.Fatalf("unresolved configuration must return unknown: hash=%q error=%v", hash, err)
			}
		})
	}
}
