package ts_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/ts"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	depcruiseExecutable = "depcruise"
	ownedDepcruisePath  = "/owned/bin/depcruise"
	emptyCompilerConfig = `{"compilerOptions":{}}`
	forbiddenTSFile     = "forbidden.ts"
	typeScriptLanguage  = "typescript"
)

func ownedDepcruise(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("runs dependency-cruiser")
	}
	if binary := os.Getenv("ARCHFIT_TEST_DEPCRUISE"); binary != "" {
		return binary
	}
	info, ok := toolrun.New().Detect(t.Context(), depcruiseExecutable)
	if !ok {
		t.Skip("dependency-cruiser is not on PATH")
	}
	return info.Path
}

func ownedRunner(binary string, calls *int) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: name, Path: binary}, name == depcruiseExecutable
		},
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name != binary {
				return toolrun.Output{}, fmt.Errorf("unexpected producer: %s", cmd.Name)
			}
			if !slices.Contains(cmd.Args, "--version") && !slices.Contains(cmd.Args, "--info") {
				*calls++
			}
			return toolrun.New().Run(ctx, cmd)
		},
	}
}

func TestExtract_HermeticConfigCannotHideForbiddenImport(t *testing.T) {
	binary := ownedDepcruise(t)
	for _, configName := range []string{".dependency-cruiser.cjs", ".dependency-cruiser.js", ".dependency-cruiser.mjs", ".dependency-cruiser.json"} {
		t.Run(configName, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "executed")
			config := `require("node:fs").writeFileSync(` + "`" + marker + "`" + `,"executed"); module.exports={options:{exclude:"forbidden"}};`
			if strings.HasSuffix(configName, ".mjs") {
				config = `import fs from "node:fs"; fs.writeFileSync(` + "`" + marker + "`" + `,"executed"); export default {options:{exclude:"forbidden"}};`
			}
			if strings.HasSuffix(configName, ".json") {
				config = `{"options":{"exclude":"forbidden","includeOnly":"allowed","doNotFollow":".*"}}`
			}
			root := writeTSFixture(t, `import { secret } from "./forbidden"; export const value = secret;`, map[string]string{
				forbiddenTSFile: `export const secret = 42;`, configName: config,
				".npmrc":                      "registry=http://127.0.0.1:1\nscript-shell=./decoy\n",
				"bunfig.toml":                 "preload = [\"./decoy.cjs\"]\n",
				"decoy.cjs":                   `require("node:fs").writeFileSync(` + "`" + marker + "`" + `,"decoy");`,
				"node_modules/.bin/depcruise": "#!/bin/sh\nexit 99\n",
			})
			calls := 0
			ex := ts.New(ownedRunner(binary, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Src: ".", Internal: []string{forbiddenTSFile}})
			store := factcache.NewStore(t.TempDir())
			ex.Cache = store
			var first graph.Facts
			var firstCov evidence.Coverage
			for _, mode := range []string{"cold", "warm", "refresh"} {
				if mode == "refresh" {
					store.RefreshMode = true
				}
				facts, cov, err := ex.Extract(t.Context(), scope.Scope{Root: root})
				if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
					t.Fatalf("untrusted configuration executed: %v", statErr)
				}
				if err != nil || cov.Status != evidence.StatusOK {
					t.Fatalf("%s: error=%v coverage=%+v", mode, err, cov)
				}
				if !slices.ContainsFunc(facts.Edges, func(e graph.Edge) bool {
					return e.From == "file:a.ts" && e.To == "file:forbidden.ts" && e.Kind == graph.EdgeKindUsesInternal
				}) {
					t.Fatalf("%s: forbidden edge hidden: %+v", mode, facts)
				}
				if mode == "cold" {
					first, firstCov = facts, cov
				} else if !reflect.DeepEqual(first, facts) || firstCov != cov {
					t.Fatalf("%s differs from cold evidence", mode)
				}
			}
			if calls != 2 {
				t.Fatalf("cold/warm/refresh scans=%d, want 2", calls)
			}
		})
	}
}

func TestExtract_UnsupportedCompilerIsIncomplete(t *testing.T) {
	binary := os.Getenv("ARCHFIT_TEST_DEPCRUISE_UNSUPPORTED")
	if testing.Short() || binary == "" {
		t.Skip("requires exact dependency-cruiser with unsupported TypeScript")
	}
	root := writeTSFixture(t, `import { secret } from "./forbidden"; export const value = secret;`, map[string]string{
		forbiddenTSFile: `export const secret = 42;`, "tsconfig.json": emptyCompilerConfig,
	})
	calls := 0
	ex := ts.New(ownedRunner(binary, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."})
	ex.Cache = factcache.NewStore(t.TempDir())
	for range 2 {
		facts, cov, err := ex.Extract(t.Context(), scope.Scope{Root: root})
		if err != nil || cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 || cov.FilesApplicable != 2 || evidence.PartialFromUnresolvedSpecifiers(cov) || evidence.PartialFromDegradedPrecision(cov) {
			t.Fatalf("unsupported compiler must be incomplete: error=%v coverage=%+v facts=%+v", err, cov, facts)
		}
	}
}

func producerInfoFixture(version string) string {
	return "dependency-cruiser@" + version + "\nnode version found : v24.22.1\n✔ javascript * acorn@8.19.0\n✔ typescript >=2.0.0 <7.0.0 typescript@6.0.3\n"
}

func emptyTSFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExtract_MissingCompilerAndJSOnlyControl(t *testing.T) {
	binary, err := filepath.EvalSymlinks(ownedDepcruise(t))
	if err != nil {
		t.Fatal(err)
	}
	modules := filepath.Dir(filepath.Dir(filepath.Dir(binary)))
	isolated := filepath.Join(t.TempDir(), "node_modules")
	if err := os.CopyFS(isolated, os.DirFS(modules)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(isolated, typeScriptLanguage)); err != nil {
		t.Fatal(err)
	}
	binary = filepath.Join(isolated, "dependency-cruiser", "bin", filepath.Base(binary))
	for _, tc := range []struct{ name, source string }{
		{typeScriptLanguage, "a.ts"}, {"javascript", "a.js"}, {"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := emptyTSFixture(t)
			if tc.source != "" {
				if err := os.WriteFile(filepath.Join(root, tc.source), []byte(`export const value = 1;`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			ex := ts.New(ownedRunner(binary, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."})
			ex.Cache = factcache.NewStore(t.TempDir())
			for range 2 {
				facts, cov, err := ex.Extract(t.Context(), scope.Scope{Root: root})
				if err != nil {
					t.Fatal(err)
				}
				if tc.name == typeScriptLanguage {
					if cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing != 1 || cov.FilesApplicable != 1 || evidence.PartialFromUnresolvedSpecifiers(cov) {
						t.Fatalf("missing compiler was not incomplete: %+v", cov)
					}
				} else if cov.Status != evidence.StatusOK || cov.FilesSeen != len(facts.Nodes) || cov.UnresolvedInputsMissing != 0 {
					t.Fatalf("%s needs no TypeScript compiler: facts=%+v coverage=%+v", tc.name, facts, cov)
				}
			}
			if tc.name == typeScriptLanguage && calls != 0 {
				t.Fatalf("missing compiler must stop before scanning/cache replay: calls=%d", calls)
			}
		})
	}
}

func TestExtract_NodeRuntimeInjectionIgnored(t *testing.T) {
	binary := ownedDepcruise(t)
	marker := filepath.Join(t.TempDir(), "executed")
	preload := filepath.Join(t.TempDir(), "preload.cjs")
	if err := os.WriteFile(preload, []byte(`require("node:fs").writeFileSync(`+"`"+marker+"`"+`,"executed")`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NODE_OPTIONS", "--require="+preload)
	t.Setenv("NODE_PATH", filepath.Dir(preload))
	root := writeTSFixture(t, "export const value = 1", nil)
	calls := 0
	_, cov, err := ts.New(ownedRunner(binary, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Src: "."}).Extract(t.Context(), scope.Scope{Root: root})
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("NODE_OPTIONS preload ran: %v", statErr)
	}
	if err != nil || cov.Status != evidence.StatusOK {
		t.Fatalf("isolated runtime: coverage=%+v error=%v", cov, err)
	}
}

func TestExtract_TargetPATHProducerRejected(t *testing.T) {
	root := writeTSFixture(t, "export const value = 1", nil)
	bin := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(bin, depcruiseExecutable)
	marker := filepath.Join(root, "executed")
	if err := os.WriteFile(decoy, []byte("#!/bin/sh\nprintf executed > '"+marker+"'\n"), 0o700); err != nil { //nolint:gosec // The target-PATH decoy must be executable to prove it is rejected before launch.
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, cov, err := ts.New(toolrun.New(), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."}).Extract(t.Context(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 || !strings.Contains(cov.Reason, "refusing target-project") {
		t.Fatalf("target decoy selected: coverage=%+v error=%v", cov, err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("target decoy executed: %v", statErr)
	}
}

func TestExtract_TypeOnlyForbiddenImportsPreserved(t *testing.T) {
	binary := ownedDepcruise(t)
	root := writeTSFixture(t, `import type { Secret } from "./forbidden"; export type { Secret } from "./forbidden";`, map[string]string{
		forbiddenTSFile: "export interface Secret { value: number }",
	})
	calls := 0
	facts, cov, err := ts.New(ownedRunner(binary, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Src: ".", Internal: []string{forbiddenTSFile}}).Extract(t.Context(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusOK || !slices.ContainsFunc(facts.Edges, func(e graph.Edge) bool {
		return e.From == "file:a.ts" && e.To == "file:forbidden.ts" && e.Kind == graph.EdgeKindUsesInternal && e.StrengthHint == "contract"
	}) {
		t.Fatalf("type-only policy edge missing: facts=%+v coverage=%+v error=%v", facts, cov, err)
	}
}

func TestExtract_EmptyTSConfigIsNotMissingEvidence(t *testing.T) {
	binary := ownedDepcruise(t)
	root := emptyTSFixture(t)
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(emptyCompilerConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	facts, cov, err := ts.New(ownedRunner(binary, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Src: "."}).Extract(t.Context(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusOK || cov.FilesApplicable != 0 || len(facts.Nodes) != 0 {
		t.Fatalf("legitimate empty tree: facts=%+v coverage=%+v error=%v", facts, cov, err)
	}
}

func TestExtract_IncompleteGraphsNeverCache(t *testing.T) {
	for name, output := range map[string]string{
		"empty":                         `{"modules":[]}`,
		"unusable":                      `{}`,
		"missing-source-and-unresolved": `{"modules":[{"source":"other.ts","dependencies":[{"module":"unknown","couldNotResolve":true}]}]}`,
		"environment-issue":             `{"modules":[{"source":"a.ts"}],"summary":{"environment":{"issues":[{"name":"missing-typescript-transpiler","description":"unsupported"}]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTSFixture(t, "export const value = 1", nil)
			calls := 0
			ex := ts.New(cacheFixtureRunner(output, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."})
			ex.Cache = factcache.NewStore(t.TempDir())
			for range 2 {
				_, cov, err := ex.Extract(t.Context(), scope.Scope{Root: root})
				if err != nil || cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 || evidence.PartialFromUnresolvedSpecifiers(cov) || evidence.PartialFromDegradedPrecision(cov) {
					t.Fatalf("incomplete graph became comparable: coverage=%+v error=%v", cov, err)
				}
			}
			if calls != 2 {
				t.Fatalf("incomplete graph cached: calls=%d", calls)
			}
		})
	}
}

func TestExtract_CompilerAndRuntimeIdentityInvalidateCache(t *testing.T) {
	root := writeTSFixture(t, "export const value = 1", nil)
	calls := 0
	info := producerInfoFixture("18.7.0")
	runner := cacheFixtureRunner(`{"modules":[{"source":"a.ts"}]}`, &calls)
	run := runner.RunFunc
	runner.RunFunc = func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		if slices.Contains(cmd.Args, "--info") {
			return toolrun.Output{Stdout: []byte(info)}, nil
		}
		return run(ctx, cmd)
	}
	ex := ts.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Src: "."})
	ex.Cache = factcache.NewStore(t.TempDir())
	extract := func() evidence.Coverage {
		t.Helper()
		_, cov, err := ex.Extract(t.Context(), scope.Scope{Root: root})
		if err != nil || cov.Status != evidence.StatusOK || cov.MeasurementSettingsHash == "" {
			t.Fatalf("extract: coverage=%+v error=%v", cov, err)
		}
		return cov
	}
	first := extract()
	if first != extract() || calls != 1 {
		t.Fatalf("stable environment missed cache: calls=%d", calls)
	}
	info = strings.ReplaceAll(info, "typescript@6.0.3", "typescript@5.9.3")
	compiler := extract()
	if compiler.MeasurementSettingsHash == first.MeasurementSettingsHash || calls != 2 {
		t.Fatalf("compiler absent from identity: first=%+v compiler=%+v calls=%d", first, compiler, calls)
	}
	info = strings.ReplaceAll(info, "v24.22.1", "v24.15.0")
	runtime := extract()
	if runtime.MeasurementSettingsHash == compiler.MeasurementSettingsHash || calls != 3 {
		t.Fatalf("Node absent from identity: compiler=%+v runtime=%+v calls=%d", compiler, runtime, calls)
	}
}

func TestExtract_SourceInventoryFailsClosed(t *testing.T) {
	for _, kind := range []string{"cycle", "outside"} {
		t.Run(kind, func(t *testing.T) {
			root := emptyTSFixture(t)
			target := root
			if kind == "outside" {
				target = writeTSFixture(t, "export const hidden = 1", nil)
			}
			if err := os.Symlink(target, filepath.Join(root, "linked")); err != nil {
				t.Fatal(err)
			}
			runner := &toolrun.RunnerMock{} // Inventory failure must precede execution.
			_, cov, err := ts.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."}).Extract(t.Context(), scope.Scope{Root: root})
			if err != nil || cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 || evidence.PartialFromUnresolvedSpecifiers(cov) {
				t.Fatalf("%s inventory became valid empty evidence: coverage=%+v error=%v", kind, cov, err)
			}
		})
	}
}

func TestExtract_SymlinkOnlyTSSources(t *testing.T) {
	binary := ownedDepcruise(t)
	root := emptyTSFixture(t)
	if err := os.Mkdir(filepath.Join(root, "actual"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "actual", "a.ts"), []byte("export const value = 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "actual"), filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	calls := 0
	ex := ts.New(ownedRunner(binary, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."})
	facts, cov, err := ex.Extract(t.Context(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusOK || cov.FilesApplicable < 1 || len(facts.Nodes) == 0 {
		t.Fatalf("in-scope symlink: facts=%+v coverage=%+v error=%v", facts, cov, err)
	}
	// The scan root can itself be a symlink; compiler preflight still inventories
	// its TS inputs rather than assuming the symlink has no sources.
	symlinkRoot := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(root, symlinkRoot); err != nil {
		t.Fatal(err)
	}
	if missing := os.Getenv("ARCHFIT_TEST_DEPCRUISE_UNSUPPORTED"); missing != "" {
		ex = ts.New(ownedRunner(missing, &calls), evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."})
		_, cov, err = ex.Extract(t.Context(), scope.Scope{Root: symlinkRoot})
		if err != nil || cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 || cov.FilesApplicable == 0 {
			t.Fatalf("symlink TS inputs missed compiler preflight: coverage=%+v error=%v", cov, err)
		}
	}
}
