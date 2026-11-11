package tsimports

import (
	"maps"
	"testing"
	"testing/fstest"
)

// Literals the tests share.
const (
	srcX     = "src/x.ts"
	srcA     = "src/a.ts"
	specA    = "./a"
	pkgJSON  = "package.json"
	pkgReact = "react"

	pnpmWorkspace = "pnpm-workspace.yaml"
	depsA         = "deps/a.ts"
)

// tree builds an in-memory file tree from file names and contents.
func tree(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

// resolveTree is the fixture tree of scripts/eval/extractors/ts_imports_test.py.
var resolveTree = map[string]string{
	pkgJSON: `{"workspaces": {"packages": ["packages/*"]}}`,
	"tsconfig.json": "{\n  // JSONC: comments and trailing commas\n" +
		`  "compilerOptions": {"baseUrl": ".", "paths": {"@app/*": ["src/*"], "@lib": ["src/lib/index.ts"],},},` + "\n}\n",
	srcA:               "",
	"src/b.tsx":        "",
	"src/c/index.ts":   "",
	"src/esm.ts":       "",
	"src/legacy.js":    "",
	"src/types.d.ts":   "",
	"src/style.css":    "",
	"src/lib/index.ts": "",
	"packages/core/package.json": `{
		"name": "@acme/core",
		"exports": {
			".": {"types": "./dist/index.d.ts", "code": "./src/index.ts", "default": "./dist/index.js"},
			"./plain": "./src/plain.ts",
			"./internal/*": {"code": "./src/internal/*.ts"},
			"./private": null,
			"./blocked": {"code": null, "default": "./src/plain.ts"},
			"./*": {"code": "./src/*.ts"}
		},
		"imports": {"#store": {"storybook": "./src/store.mock.ts", "default": "./src/store.ts"}}
	}`,
	"packages/core/tsconfig.json":        `{"extends": "../../tsconfig.json", "compilerOptions": {"customConditions": ["code"]}}`,
	"packages/core/src/index.ts":         "",
	"packages/core/src/plain.ts":         "",
	"packages/core/src/store.ts":         "",
	"packages/core/src/internal/x.ts":    "",
	"packages/old/package.json":          `{"name": "old", "main": "./lib/main.js"}`,
	"packages/old/lib/main.js":           "",
	"packages/old/lib/extra.js":          "",
	"packages/node10/tsconfig.json":      `{"compilerOptions": {"moduleResolution": "node"}}`,
	"packages/node10/package.json":       `{"name": "node10"}`,
	"packages/node10/use.js":             "",
	"packages/core/preview.js":           "",
	"packages/core/src/private.ts":       "",
	"packages/loop/package.json":         `{"name": "loop", "main": "."}`,
	"packages/loop/index.js":             "",
	"apps/web/tsconfig.json":             `{"compilerOptions": {"baseUrl": "src"}}`,
	"apps/web/src/components/Button.tsx": "",
	"nx/tsconfig.base.json":              `{"compilerOptions": {"baseUrl": "."}}`,
	"nx/apps/web/tsconfig.json":          `{"extends": ["../../tsconfig.base.json", "./tsconfig.paths.json", "./tsconfig.strict.json"]}`,
	"nx/apps/web/tsconfig.paths.json":    `{"compilerOptions": {"paths": {"@web/*": ["apps/web/src/*"]}}}`,
	"nx/apps/web/tsconfig.strict.json":   `{"compilerOptions": {"strict": true}}`,
	"nx/apps/web/src/x.ts":               "",
	"star/tsconfig.json":                 `{"compilerOptions": {"paths": {"*": ["types/*"]}}}`,
	"@app/zzz.ts":                        "",
	// Beyond the prototype's fixtures: both conditions name a file, so only the
	// document order of the exports object decides.
	"packages/order/package.json": `{"name": "order", "exports": {"import": "./b.ts", "default": "./a.ts"}}`,
	"packages/order/a.ts":         "",
	"packages/order/b.ts":         "",
	// An installed dependency: no answer may depend on it being there.
	"deps/tsconfig.json":                             `{"extends": "./node_modules/@tsconfig/base/tsconfig.json", "compilerOptions": {"paths": {"react": ["node_modules/@types/react/index.d.ts"]}}}`,
	"deps/node_modules/@types/react/index.d.ts":      "",
	"deps/node_modules/@tsconfig/base/tsconfig.json": `{"compilerOptions": {"baseUrl": "../../.."}}`,
	"deps/node_modules/x/index.js":                   "",
	"deps/lib.ts":                                    "",
}

func TestResolve(t *testing.T) {
	t.Parallel()
	const core = "packages/core/src/index.ts"
	tests := []struct {
		name     string
		importer string
		spec     string
		kind     Kind
		target   string
	}{
		{"relative, extension probed", srcX, specA, KindFile, srcA},
		{"relative tsx", srcX, "./b", KindFile, "src/b.tsx"},
		{"directory index", srcX, "./c", KindFile, "src/c/index.ts"},
		{".js specifier names a .ts file", srcX, "./esm.js", KindFile, "src/esm.ts"},
		{".js file that exists", srcX, "./legacy.js", KindFile, "src/legacy.js"},
		{".ts specifier names a declaration", srcX, "./types.ts", KindFile, "src/types.d.ts"},
		{"asset with a query", srcX, "./style.css?raw", KindFile, "src/style.css"},
		{"missing relative is unknown", srcX, "./gone", KindUnknown, ""},
		{"relative above the root is unknown", srcX, "../../x", KindUnknown, ""},
		{"absolute path is unknown", srcX, "/src/a.ts", KindUnknown, ""},
		{"paths wildcard", srcX, "@app/a", KindFile, srcA},
		{"paths match without a file is unknown", srcX, "@app/missing", KindUnknown, ""},
		{"paths match without a file skips baseUrl", srcX, "@app/zzz", KindUnknown, ""},
		{"catch-all paths does not make a package first-party", "star/a.ts", pkgReact, KindExternal, ""},
		{"baseUrl alone resolves a bare specifier", "apps/web/src/app.tsx", "components/Button", KindFile, "apps/web/src/components/Button.tsx"},
		{"array extends merges; paths use the inherited baseUrl", "nx/apps/web/src/main.ts", "@web/x", KindFile, "nx/apps/web/src/x.ts"},
		{"exports null blocks a subpath a pattern would match", core, "@acme/core/private", KindUnknown, ""},
		{"exports null under an active condition blocks", core, "@acme/core/blocked", KindUnknown, ""},
		{"main naming its own dir falls back to index", srcX, "loop", KindFile, "packages/loop/index.js"},
		{"paths exact", srcX, "@lib", KindFile, "src/lib/index.ts"},
		{"exports with the tsconfig condition", core, "@acme/core", KindFile, core},
		{"exports without the condition reaches only dist", srcX, "@acme/core", KindUnknown, ""},
		{"exports string subpath", srcX, "@acme/core/plain", KindFile, "packages/core/src/plain.ts"},
		{"exports pattern subpath", core, "@acme/core/internal/x", KindFile, "packages/core/src/internal/x.ts"},
		{"subpath that exports does not list", srcX, "@acme/core/preview", KindUnknown, ""},
		{"package imports field", core, "#store", KindFile, "packages/core/src/store.ts"},
		{"package imports without a match is unknown", core, "#gone", KindUnknown, ""},
		{"main field", srcX, "old", KindFile, "packages/old/lib/main.js"},
		{"subpath without exports", srcX, "old/lib/extra", KindFile, "packages/old/lib/extra.js"},
		{"node10 ignores exports", "packages/node10/use.js", "@acme/core/preview", KindFile, "packages/core/preview.js"},
		{"exports conditions in document order", srcX, "order", KindFile, "packages/order/b.ts"},
		{"paths alias into node_modules is external", depsA, pkgReact, KindExternal, ""},
		{"relative path into node_modules is external", depsA, "./node_modules/x", KindExternal, ""},
		{"extends into node_modules is not read", depsA, "lib", KindExternal, ""},
		{"third-party package", srcX, pkgReact, KindExternal, ""},
		{"node builtin", srcX, "node:fs", KindExternal, ""},
	}
	r := newResolver(tree(resolveTree))
	for _, tt := range tests {
		kind, target := r.resolve(tt.importer, tt.spec)
		if kind != tt.kind || target != tt.target {
			t.Errorf("%s: resolve(%q, %q) = (%s, %q), want (%s, %q)",
				tt.name, tt.importer, tt.spec, kind, target, tt.kind, tt.target)
		}
	}
}

func TestWorkspacePackages(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		pkgJSON:                      "{}",
		pnpmWorkspace:                "# workspace\npackages:\n  - 'libs/*'\n  - \"!libs/skip\"\ncatalog:\n  - x\n",
		"libs/a/package.json":        `{"name": "@org/a"}`,
		"libs/skip/package.json":     `{"name": "@org/skip"}`,
		"libs/nameless/package.json": `{}`,
	})
	got := workspacePackages(fsys)
	if want := map[string]string{"@org/a": "libs/a"}; !maps.Equal(got, want) {
		t.Errorf("workspacePackages = %v, want %v", got, want)
	}
}

func TestPnpmPatterns(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{
		pnpmWorkspace: "packages:\n- libs/*\nonlyBuiltDependencies:\n- esbuild\n",
	})
	got := pnpmPatterns(fsys)
	if len(got) != 1 || got[0] != "libs/*" {
		t.Errorf("pnpmPatterns = %q, want [libs/*]", got)
	}
}

func TestReadJSONC(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want string // the value of key "a", or "" when the file reads as nothing
	}{
		{"line comment", "{\n// c\n\"a\": \"x\"}", "x"},
		{"block comment", `{/* c */ "a": "x"}`, "x"},
		{"comment markers inside a string stay", `{"a": "src/* // not a comment */"}`, "src/* // not a comment */"},
		{"trailing comma before a comment", "{\"a\": \"x\", // c\n}", "x"},
		{"trailing comma inside a string stays", `{"a": ",}"}`, ",}"},
		{"invalid JSON reads as nothing", `{"a": }`, ""},
		{"a list reads as nothing", `["a"]`, ""},
	}
	for _, tt := range tests {
		fsys := tree(map[string]string{"tsconfig.json": tt.text})
		got, _ := asString(readJSONC(fsys, "tsconfig.json").get("a"))
		if got != tt.want {
			t.Errorf("%s: a = %q, want %q", tt.name, got, tt.want)
		}
	}
}
