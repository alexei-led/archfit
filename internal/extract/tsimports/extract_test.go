package tsimports

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// writeTree writes files under a new temporary root and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// fakeAstGrep is a runner whose ast-grep is present and answers every call
// with run; it records each call's paths.
func fakeAstGrep(run func(paths []string) (toolrun.Output, error)) (*toolrun.RunnerMock, *[][]string) {
	var calls [][]string
	return &toolrun.RunnerMock{
		DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, true },
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			paths := cmd.Args[slices.Index(cmd.Args, "--")+1:]
			calls = append(calls, paths)
			return run(paths)
		},
	}, &calls
}

func noMatches([]string) (toolrun.Output, error) { return toolrun.Output{}, nil }

func TestExtractFailures(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{srcX: ""})

	t.Run("ast-grep absent", func(t *testing.T) {
		t.Parallel()
		runner := &toolrun.RunnerMock{
			DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false },
		}
		if _, err := Extract(context.Background(), runner, root); !errors.Is(err, ErrNoAstGrep) {
			t.Errorf("err = %v, want ErrNoAstGrep", err)
		}
	})

	t.Run("no sources needs no ast-grep", func(t *testing.T) {
		t.Parallel()
		runner := &toolrun.RunnerMock{} // any call panics
		got, err := Extract(context.Background(), runner, writeTree(t, map[string]string{"main.go": "", "node_modules/x/i.js": ""}))
		if err != nil || len(got.Sources) != 0 || len(got.Imports) != 0 {
			t.Errorf("Extract = %+v, %v, want empty facts", got, err)
		}
	})

	t.Run("failed run names the exit and stderr", func(t *testing.T) {
		t.Parallel()
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 2, Stderr: []byte("bad rule\n")}, nil
		})
		_, err := Extract(context.Background(), runner, root)
		if err == nil || !strings.Contains(err.Error(), "exited 2: bad rule") {
			t.Errorf("err = %v, want the exit code and stderr", err)
		}
	})

	t.Run("a run a signal ended is an error, whatever it printed", func(t *testing.T) {
		t.Parallel()
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: -1, Stdout: []byte(`{"text":"'./a'","file":"src/x.ts","ruleId":"ts-source"}` + "\n")}, nil
		})
		if _, err := Extract(context.Background(), runner, root); err == nil || !strings.Contains(err.Error(), "exited -1") {
			t.Errorf("err = %v, want exited -1", err)
		}
	})

	t.Run("runner error is passed on", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) { return toolrun.Output{}, boom })
		if _, err := Extract(context.Background(), runner, root); !errors.Is(err, boom) {
			t.Errorf("err = %v, want boom", err)
		}
	})

	t.Run("bad output is an error", func(t *testing.T) {
		t.Parallel()
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte("{not json\n")}, nil
		})
		if _, err := Extract(context.Background(), runner, root); err == nil {
			t.Error("err = nil, want a read error")
		}
	})

	t.Run("missing root is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := Extract(context.Background(), &toolrun.RunnerMock{}, filepath.Join(root, "gone")); err == nil {
			t.Error("err = nil, want a walk error")
		}
	})
}

func TestExtractBatches(t *testing.T) {
	t.Parallel()
	files := map[string]string{}
	for i := range batchSize + 1 {
		files[fmt.Sprintf("src/f%03d.ts", i)] = ""
	}
	runner, calls := fakeAstGrep(noMatches)
	got, err := Extract(context.Background(), runner, writeTree(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || len((*calls)[0]) != batchSize || len((*calls)[1]) != 1 {
		t.Errorf("calls with %d paths, want 2 calls with %d and 1", len(*calls), batchSize)
	}
	if len(got.Sources) != batchSize+1 {
		t.Errorf("sources = %d, want %d", len(got.Sources), batchSize+1)
	}
}

func TestSpecifier(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rule, text, want string
		ok               bool
	}{
		{"ts-source", `"./a"`, specA, true},
		{"js-source", `'./a'`, specA, true},
		{"ts-reference", `/// <reference path="../types.d.ts" />`, "../types.d.ts", true},
		{"tsx-reference", `/// <reference path='types.d.ts' />`, "./types.d.ts", true},
		{"js-reference", `/// <reference path="/abs.d.ts" />`, "/abs.d.ts", true},
		{"ts-reference", `/// <reference types="node" />`, "", false},
		{"ts-source", `"`, "", false},
	}
	for _, tt := range tests {
		got, ok := specifier(sgMatch{RuleID: tt.rule, Text: tt.text})
		if got != tt.want || ok != tt.ok {
			t.Errorf("specifier(%s, %s) = %q, %v, want %q, %v", tt.rule, tt.text, got, ok, tt.want, tt.ok)
		}
	}
}

func TestExtractUnknownItems(t *testing.T) {
	t.Parallel()
	runner, _ := fakeAstGrep(noMatches) // the items do not depend on any import
	root := writeTree(t, map[string]string{
		tsconfigJSON:               extendsCompany,
		srcX:                       "",
		srcA:                       "",
		"lib/b.ts":                 "",
		"other/tsconfig.json":      `{"extends": ["./gone.json", "./gone.json"]}`,
		"other/c.ts":               "",
		"complete/tsconfig.json":   "{}",
		"complete/d.ts":            "",
		"nosource/tsconfig.json":   `{"extends": "@unused/tsconfig"}`,
		"nosource/notes/readme.md": "",
	})
	got, err := Extract(context.Background(), runner, root)
	if err != nil {
		t.Fatal(err)
	}
	want := []UnknownItem{
		{"other/tsconfig.json", "./gone.json", reasonMissing},
		{tsconfigJSON, companyConfig, reasonPackage},
	}
	if !slices.Equal(got.Unknown, want) {
		t.Errorf("Unknown = %+v, want %+v", got.Unknown, want)
	}
}

// TestExtractWithAstGrep runs the real ast-grep on the prototype's parse fixtures.
func TestExtractWithAstGrep(t *testing.T) {
	t.Parallel()
	runner := toolrun.New()
	if _, ok := runner.Detect(context.Background(), astGrep); !ok {
		t.Skip("ast-grep is not installed: install it to read TypeScript imports with the real parser")
	}

	t.Run("import forms", func(t *testing.T) {
		t.Parallel()
		source := "/// <reference path=\"../types.d.ts\" />\n" +
			"import a from './a';\n" +
			"import type { T } from './t';\n" +
			"export * from './re';\n" +
			"export type { Y } from './rey';\n" +
			"import eq = require('./eq');\n" +
			"const d = import('./dyn');\n" +
			"const r = require('./req');\n" +
			"// import './commented';\n" +
			"const s = \"import './in-string'\";\n"
		root := writeTree(t, map[string]string{
			srcX:        source,
			"src/y.tsx": source,
			"src/z.js":  "import a from './a';\nrequire('./req');\n",
		})
		got, err := Extract(context.Background(), runner, root)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"1 ../types.d.ts", "2 ./a", "3 ./t", "4 ./re", "5 ./rey", "6 ./eq", "7 ./dyn", "8 ./req"}
		for _, file := range []string{srcX, "src/y.tsx"} {
			if specs := specsOf(got, file); !slices.Equal(specs, want) {
				t.Errorf("%s: specifiers = %q, want %q", file, specs, want)
			}
		}
		if specs := specsOf(got, "src/z.js"); !slices.Equal(specs, []string{"1 ./a", "2 ./req"}) {
			t.Errorf("src/z.js: specifiers = %q, want [1 ./a 2 ./req]", specs)
		}
	})

	t.Run("skips node_modules and resolves", func(t *testing.T) {
		t.Parallel()
		root := writeTree(t, map[string]string{
			pkgJSON:                     "{}",
			srcX:                        "import a from './a';\nimport b from './gone';\nimport React from 'react';\n",
			srcA:                        "",
			"node_modules/dep/index.js": "import x from './x';\n",
		})
		got, err := Extract(context.Background(), runner, root)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{srcA, srcX}; !slices.Equal(got.Sources, want) {
			t.Errorf("sources = %q, want %q", got.Sources, want)
		}
		want := []Import{
			{File: srcX, Line: 1, Specifier: specA, Kind: KindFile, Target: srcA},
			{File: srcX, Line: 2, Specifier: "./gone", Kind: KindUnknown},
			{File: srcX, Line: 3, Specifier: pkgReact, Kind: KindExternal},
		}
		if !slices.Equal(got.Imports, want) {
			t.Errorf("imports = %+v, want %+v", got.Imports, want)
		}
	})
}

// specsOf returns "line specifier" for each import of file, in order.
func specsOf(f Facts, file string) []string {
	var out []string
	for _, imp := range f.Imports {
		if imp.File == file {
			out = append(out, fmt.Sprintf("%d %s", imp.Line, imp.Specifier))
		}
	}
	return out
}

func TestWorkspacePackagesDoNotFollowSymlinks(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		pnpmWorkspace:              "packages:\n  - 'libs/**'\n  - 'linked/*'\n",
		"libs/a/package.json":      `{"name": "@org/a"}`,
		"elsewhere/b/package.json": `{"name": "@org/b"}`,
	})
	// A loop like the links pnpm makes between workspace packages.
	if err := os.Symlink("..", filepath.Join(root, "libs", "a", "loop")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "linked"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "elsewhere", "b"), filepath.Join(root, "linked", "b")); err != nil {
		t.Fatal(err)
	}
	got := workspacePackages(os.DirFS(root))
	want := map[string]string{"@org/a": "libs/a", "@org/b": "linked/b"}
	if !maps.Equal(got, want) {
		t.Errorf("workspacePackages = %v, want %v", got, want)
	}
}
