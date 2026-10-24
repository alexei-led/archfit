package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/ts"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/scope"
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
		value, err := ts.MeasurementConfigHash(scope.Scope{Root: root}, evidenceports.ExtractConfig{Src: "."})
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
	if before != hash(otherRoot) {
		t.Fatal("ignored dependency-cruiser configuration changed identity")
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
			if hash, err := ts.MeasurementConfigHash(s, cfg); hash == "" || err != nil {
				t.Fatalf("ignored resolver must retain identity: hash=%q error=%v", hash, err)
			}
			calls := 0
			ex := ts.New(cacheFixtureRunner(`{"modules":[{"source":"a.ts","dependencies":[]}]}`, &calls), cfg)
			ex.Cache = factcache.NewStore(t.TempDir())
			for range 2 {
				if _, _, err := ex.Extract(context.Background(), s); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 1 {
				t.Fatalf("ignored resolver inputs must not disable cache: calls=%d", calls)
			}
		})
	}
	t.Run("ordinary enhanced resolver options", func(t *testing.T) {
		root := writeTSFixture(t, "export const a = 1", map[string]string{
			configName: `{"options":{"enhancedResolveOptions":{"extensions":[".js",".ts"],"mainFields":["module","main"]}}}`,
		})
		if hash, err := ts.MeasurementConfigHash(scope.Scope{Root: root}, evidenceports.ExtractConfig{Src: "."}); hash == "" || err != nil {
			t.Fatalf("ordinary resolver options must retain known identity: hash=%q error=%v", hash, err)
		}
	})
}

func TestMeasurementConfigHash_UnresolvedConfigurationIsUnknown(t *testing.T) {
	for name, extra := range map[string]map[string]string{
		"inherited": {tsconfigName: `{"extends":"@example/compiler-config"}`},
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTSFixture(t, "export const a = 1", extra)
			if hash, err := ts.MeasurementConfigHash(scope.Scope{Root: root}, evidenceports.ExtractConfig{Src: "."}); err == nil || hash != "" {
				t.Fatalf("unresolved configuration must return unknown: hash=%q error=%v", hash, err)
			}
		})
	}
}
