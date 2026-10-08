package ts_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/ts"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestFactCache_ExtendedTSConfigMatchesFresh(t *testing.T) {
	if testing.Short() {
		t.Skip("runs dependency-cruiser")
	}
	ctx := context.Background()
	realRunner := toolrun.New()
	if _, ok := realRunner.Detect(ctx, "depcruise"); !ok {
		t.Skip("dependency-cruiser is not on PATH")
	}
	for _, outsideRoot := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "outside_root"}[outsideRoot], func(t *testing.T) {
			repo := t.TempDir()
			root := filepath.Join(repo, "project")
			base := filepath.Join(root, "config", "aliases.json")
			if outsideRoot {
				base = filepath.Join(repo, "shared", "aliases.json")
			}
			write := func(path, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			parent, err := filepath.Rel(filepath.Join(root, "config"), base)
			if err != nil {
				t.Fatal(err)
			}
			encodedParent, err := json.Marshal("./" + filepath.ToSlash(parent))
			if err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(root, "package.json"), `{"name":"config-cache-fixture"}`)
			write(filepath.Join(root, tsconfigName), `{"extends":"./config/layer.json"}`)
			if outsideRoot {
				write(filepath.Join(root, tsconfigName), `{"extends":["./config/layer.json"]}`)
			}
			write(filepath.Join(root, "config", "layer.json"), `{"extends":`+string(encodedParent)+`}`)
			write(filepath.Join(root, "main.js"), `import { value } from "@chosen"; export const result = value;`)
			write(filepath.Join(root, "allowed.js"), `export const value = 1;`)
			write(filepath.Join(root, "forbidden.js"), `export const value = 2;`)
			alias := func(target string) {
				t.Helper()
				body, marshalErr := json.Marshal(map[string]any{"compilerOptions": map[string]any{
					"baseUrl": root, "paths": map[string][]string{"@chosen": {"./" + target}},
				}})
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				write(base, string(body))
			}
			alias("allowed.js")
			calls := 0
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
					return toolrun.ToolInfo{Name: tool}, tool == launcherBunx
				},
				RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					idx := slices.Index(cmd.Args, "depcruise")
					if idx < 0 {
						t.Fatalf("unexpected command: %+v", cmd)
					}
					cmd.Name, cmd.Args = "depcruise", cmd.Args[idx+1:]
					if !slices.Contains(cmd.Args, "--version") {
						calls++
					}
					return realRunner.Run(ctx, cmd)
				},
			}
			ex := ts.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Src: "."})
			ex.Cache = factcache.NewStore(t.TempDir())
			extract := func() graph.Facts {
				t.Helper()
				facts, cov, extractErr := ex.Extract(ctx, scope.Scope{Root: root})
				if extractErr != nil || cov.Status != "ok" {
					t.Fatalf("Extract: coverage=%+v error=%v", cov, extractErr)
				}
				slices.SortFunc(facts.Nodes, func(a, b graph.Node) int { return strings.Compare(a.ID(), b.ID()) })
				return facts
			}
			before := extract()
			extract()
			if calls != 1 {
				t.Fatalf("unchanged config must hit cache: calls=%d", calls)
			}
			alias("forbidden.js")
			warm := extract()
			ex.Cache = nil
			fresh := extract()
			if reflect.DeepEqual(before.Edges, fresh.Edges) {
				t.Fatalf("alias change did not redirect the dependency: before=%+v fresh=%+v", before, fresh)
			}
			if !reflect.DeepEqual(warm, fresh) {
				t.Fatalf("warm facts differ from fresh facts: warm=%+v fresh=%+v", warm, fresh)
			}
			if calls != 3 {
				t.Fatalf("alias edit must rerun before fresh extraction: calls=%d", calls)
			}
		})
	}
}

func TestFactCache_UnsupportedConfigRunsFresh(t *testing.T) {
	for name, config := range map[string]string{
		"jsonc":         "{ // compiler configuration\n\"compilerOptions\": {}}",
		"package":       `{"extends":"@example/tsconfig"}`,
		"extensionless": `{"extends":"./config/aliases"}`,
		"missing":       `{"extends":"./missing.json"}`,
		"cycle":         `{"extends":"./tsconfig.json"}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTSFixture(t, "export const a = 1", map[string]string{tsconfigName: config})
			calls := 0
			runner := cacheFixtureRunner(`{"modules":[{"source":"a.ts","dependencies":[]}]}`, &calls)
			ex := ts.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: "."})
			ex.Cache = factcache.NewStore(t.TempDir())
			for range 2 {
				if _, _, err := ex.Extract(context.Background(), scope.Scope{Root: root}); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 2 {
				t.Fatalf("unresolved config inputs must bypass cache: calls=%d", calls)
			}
		})
	}
}

func TestFactCache_ExplicitConfigIsRelativeToWorkDir(t *testing.T) {
	root := writeTSFixture(t, "export const a = 1", nil)
	repo := filepath.Dir(root)
	configPath := filepath.Join(repo, filepath.Base(root)+"-config.json")
	if err := os.WriteFile(configPath, []byte(`{"compilerOptions":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(configPath) })
	calls := 0
	runner := cacheFixtureRunner(`{"modules":[{"source":"a.ts","dependencies":[]}]}`, &calls)
	ex := ts.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeAuto, Src: ".", TSConfig: filepath.Base(configPath)})
	ex.Cache = factcache.NewStore(t.TempDir())
	s := scope.Scope{Root: root, GitRoot: repo, SubtreePrefix: filepath.Base(root)}
	for range 2 {
		if _, _, err := ex.Extract(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("unchanged external config must hit cache: calls=%d", calls)
	}
	if err := os.WriteFile(configPath, []byte(`{"compilerOptions":{"baseUrl":"."}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ex.Extract(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("explicit config edit must invalidate cache: calls=%d", calls)
	}
}
