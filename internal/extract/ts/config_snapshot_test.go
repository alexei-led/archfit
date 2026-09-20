package ts_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/extract/ts"
	"github.com/alexei-led/archfit/internal/factcache"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const snapshotConfigName = ".dependency-cruiser.cjs"

func nativeSnapshotExtractor(t *testing.T) *ts.Extractor {
	t.Helper()
	if testing.Short() {
		t.Skip("runs dependency-cruiser")
	}
	runner := toolrun.New()
	binary := os.Getenv("ARCHFIT_TEST_DEPCRUISE")
	if binary == "" {
		info, ok := runner.Detect(context.Background(), "depcruise")
		if !ok {
			t.Skip("dependency-cruiser is not on PATH")
		}
		binary = info.Path
	}
	adapter := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == launcherBunx
		},
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			index := slices.Index(cmd.Args, "depcruise")
			if index < 0 {
				t.Fatalf("unexpected command: %+v", cmd)
			}
			cmd.Name, cmd.Args = binary, cmd.Args[index+1:]
			return runner.Run(ctx, cmd)
		},
	}
	ex := ts.New(adapter, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Src: "."})
	ex.Cache = factcache.NewStore(t.TempDir())
	return ex
}

func snapshotExtract(t *testing.T, ex *ts.Extractor, root string) (graph.Facts, evidence.Coverage) {
	t.Helper()
	facts, cov, err := ex.Extract(context.Background(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusOK {
		t.Fatalf("extract error=%v coverage=%+v", err, cov)
	}
	return facts, cov
}

func TestNativeConfigSnapshotUsesActualConsumedOptions(t *testing.T) {
	ex := nativeSnapshotExtractor(t)
	config := `const fs = require("node:fs");
const settings = require("./settings.json");
if (process.env.ARCHFIT_CONFIG_EVALUATIONS) fs.appendFileSync(process.env.ARCHFIT_CONFIG_EVALUATIONS, "1");
module.exports = {options: {doNotFollow: {path: "node_modules"}, extraExtensionsToScan: process.env.ARCHFIT_EXTRA === "on" ? [settings.extension] : []}};`
	extra := map[string]string{snapshotConfigName: config, "settings.json": `{"extension":".foo"}`, "asset.foo": "plain text", "asset.bar": "plain text"}
	root := writeTSFixture(t, "export const value = 1", extra)
	counterDir := t.TempDir()
	counter := filepath.Join(counterDir, "evaluations")
	t.Setenv("ARCHFIT_CONFIG_EVALUATIONS", counter)
	t.Setenv("ARCHFIT_EXTRA", "off")
	beforeFacts, before := snapshotExtract(t, ex, root)
	if before.MeasurementSettingsHash == "" {
		t.Fatalf("native default cjs config was unknown: %+v", before)
	}
	data, err := fs.ReadFile(os.DirFS(counterDir), "evaluations")
	if err != nil || string(data) != "1" {
		t.Fatalf("config was not evaluated exactly once: %q %v", data, err)
	}
	t.Setenv("ARCHFIT_EXTRA", "on")
	afterFacts, after := snapshotExtract(t, ex, root)
	if before.MeasurementSettingsHash == after.MeasurementSettingsHash {
		t.Fatal("environment-dependent extraction option did not change identity")
	}
	if len(beforeFacts.Nodes) >= len(afterFacts.Nodes) {
		t.Fatalf("extraExtensionsToScan did not change actual graph: %d -> %d", len(beforeFacts.Nodes), len(afterFacts.Nodes))
	}
	if err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"extension":".bar"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, imported := snapshotExtract(t, ex, root)
	if imported.MeasurementSettingsHash == after.MeasurementSettingsHash {
		t.Fatal("imported JSON change did not change consumed settings identity")
	}
	data, err = fs.ReadFile(os.DirFS(counterDir), "evaluations")
	if err != nil || string(data) != "111" {
		t.Fatalf("dynamic configuration was cached or evaluated twice: %q %v", data, err)
	}
}

func TestNativeConfigSnapshotIsCheckoutAndSourceIndependent(t *testing.T) {
	ex := nativeSnapshotExtractor(t)
	config := map[string]string{snapshotConfigName: `module.exports = {options:{doNotFollow:{path:"node_modules"}, enhancedResolveOptions:{conditionNames:["import","require","node","default"]}}};`}
	config["main.js"] = "export const value = 1"
	root := writeTSFixture(t, "export const value = 1", config)
	config["main.js"] = "export const value = 2"
	other := writeTSFixture(t, "export const value = 2", config)
	_, first := snapshotExtract(t, ex, root)
	_, second := snapshotExtract(t, ex, other)
	if first.MeasurementSettingsHash == "" || first.MeasurementSettingsHash != second.MeasurementSettingsHash {
		t.Fatalf("source/checkout affected identity: %+v %+v", first, second)
	}
}

func TestNativeConfigSnapshotPreservesRootRelativeResolution(t *testing.T) {
	ex := nativeSnapshotExtractor(t)
	config := map[string]string{
		snapshotConfigName: `module.exports={extends:"./shared.cjs",options:{baseDir:__dirname}};`,
		"shared.cjs":       `module.exports={options:{doNotFollow:{path:"node_modules"}}};`,
		"main.js":          `import {value} from "./chosen.js"; export const a=value;`,
		"chosen.js":        `export const value=1;`,
	}
	firstRoot := writeTSFixture(t, `import {value} from "@chosen"; export const a=value;`, config)
	secondRoot := writeTSFixture(t, `import {value} from "@chosen"; export const a=value+1;`, config)
	firstFacts, first := snapshotExtract(t, ex, firstRoot)
	_, second := snapshotExtract(t, ex, secondRoot)
	if first.MeasurementSettingsHash == "" || first.MeasurementSettingsHash != second.MeasurementSettingsHash {
		t.Fatalf("resolved config paths depend on checkout: %+v %+v", first, second)
	}
	if !slices.ContainsFunc(firstFacts.Edges, func(edge graph.Edge) bool { return edge.To == "file:chosen.js" }) {
		t.Fatalf("private wrapper moved relative resolution: %+v", firstFacts.Edges)
	}
}

func TestNativeConfigSnapshotUnsupportedResolverKeepsFactsUnknown(t *testing.T) {
	ex := nativeSnapshotExtractor(t)
	root := writeTSFixture(t, "export const value = 1", map[string]string{
		snapshotConfigName:   `module.exports = {options:{webpackConfig:{fileName:"./webpack.config.cjs"}}};`,
		"webpack.config.cjs": `module.exports = {resolve:{extensions:[".js"]}};`,
	})
	facts, cov := snapshotExtract(t, ex, root)
	if len(facts.Nodes) == 0 || cov.MeasurementSettingsHash != "" {
		t.Fatalf("unsupported resolver must preserve facts without identity: facts=%+v coverage=%+v", facts, cov)
	}
	if !strings.Contains(cov.Version, ".") {
		t.Fatalf("actual runtime version missing: %+v", cov)
	}
}
