package tsimports

import (
	"maps"
	"slices"
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

	tsconfigJSON   = "tsconfig.json"
	companyConfig  = "@company/tsconfig"
	pkgExtX        = "pkgext/src/x.ts"
	extMain        = "ext/main.ts"
	internalFoo    = "@internal/foo"
	extendsCompany = `{"extends": "@company/tsconfig"}`
	chainX         = "chain/src/x.ts"
	corePlain      = "packages/core/src/plain.ts"
	specGone       = "./gone"
	pkgLodash      = "lodash"
	extMdecl       = "ext/mdecl.d.mts"
	extBoth        = "ext/both.ts"
	jsAliasX       = "jsalias/src/x.ts"
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
	pkgJSON: `{"workspaces": {"packages": ["packages/*"]}, "devDependencies": {"vitest": "3"}}`,
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
	corePlain:                            "",
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
	// A tsconfig that extends a package: the paths it may set are unknown.
	"pkgext/tsconfig.json": extendsCompany,
	"pkgext/package.json": `{"dependencies": {"react": "19"}, "devDependencies": {"@types/estree": "1"},
		"peerDependencies": {"@scope/peer": "1"}, "optionalDependencies": {"fsevents": "2"}}`,
	"pkgext/src/y.ts":              "",
	"chain/tsconfig.json":          `{"extends": "./tsconfig.base.json"}`,
	"chain/tsconfig.base.json":     `{"extends": "@company/tsconfig", "compilerOptions": {"paths": {"@chain/*": ["src/*"]}}}`,
	"chain/src/y.ts":               "",
	chainX:                         "",
	"chain/complete/tsconfig.json": `{"compilerOptions": {"paths": {"@chain/*": ["../src/*"]}}}`,
	// A specifier extension and the files TypeScript tries in its place.
	"ext/model.js":    "",
	"ext/model.ts":    "",
	"ext/only.js":     "",
	"ext/mod.mjs":     "",
	"ext/mod.mts":     "",
	"ext/mdecl.mjs":   "",
	extMdecl:          "",
	"ext/com.cjs":     "",
	"ext/com.cts":     "",
	"ext/view.jsx":    "",
	"ext/view.tsx":    "",
	"ext/decl.js":     "",
	"ext/decl.d.ts":   "",
	extBoth:           "",
	"ext/both.d.ts":   "",
	"ext/jsxonly.jsx": "",
	"ext/tsxonly.tsx": "",
	"ext/tsonly.ts":   "",
	"ext/cdecl.d.cts": "",
	"ext/tx.tsx":      "",
	"ext/tx.d.ts":     "",
	// paths targets that name a file with a script extension.
	"jsalias/tsconfig.json":      `{"compilerOptions": {"paths": {"foo": ["./src/foo.js"], "bar": ["./src/bar.js"]}}}`,
	"jsalias/src/foo.js":         "",
	"jsalias/src/foo.ts":         "",
	"jsalias/src/bar.ts":         "",
	"packages/decl/package.json": `{"name": "decl", "types": "./index.d.ts"}`,
	"packages/decl/index.d.ts":   "",
	"packages/decl/index.ts":     "",
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
		{"missing relative is unknown", srcX, specGone, KindUnknown, ""},
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
		{"exports string subpath", srcX, "@acme/core/plain", KindFile, corePlain},
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
		// The unread base config sets baseUrl: TypeScript places "lib" on deps/lib.ts.
		{"under an extends into node_modules a bare name is unknown", depsA, "lib", KindUnknown, ""},
		{"third-party package", srcX, pkgReact, KindExternal, ""},
		{"node builtin", srcX, "node:fs", KindExternal, ""},
		{"complete tsconfig: an undeclared bare name is external", srcX, pkgLodash, KindExternal, ""},

		{"package extends: an alias is unknown", pkgExtX, internalFoo, KindUnknown, ""},
		{"package extends: an undeclared package is unknown", pkgExtX, pkgLodash, KindUnknown, ""},
		{"package extends: a dependency is external", pkgExtX, pkgReact, KindExternal, ""},
		{"package extends: a dependency's subpath is external", pkgExtX, "react/jsx-runtime", KindExternal, ""},
		{"package extends: a scoped peer dependency's subpath is external", pkgExtX, "@scope/peer/sub", KindExternal, ""},
		{"package extends: an optional dependency is external", pkgExtX, "fsevents", KindExternal, ""},
		{"package extends: a package declared through @types is external", pkgExtX, "estree", KindExternal, ""},
		{"package extends: a root manifest declares too", pkgExtX, "vitest", KindExternal, ""},
		{"package extends: a bare built-in is external", pkgExtX, "fs/promises", KindExternal, ""},
		{"package extends: an unknown built-in subpath is unknown", pkgExtX, "fs/nope", KindUnknown, ""},
		{"package extends: node: is external", pkgExtX, "node:fs", KindExternal, ""},
		{"package extends: a workspace package resolves", pkgExtX, "@acme/core/plain", KindFile, corePlain},
		{"package extends: a relative path resolves", pkgExtX, "./y", KindFile, "pkgext/src/y.ts"},
		{"extends chain: the paths read still resolve", chainX, "@chain/y", KindFile, "chain/src/y.ts"},
		{"extends chain: an unread link makes a bare name unknown", chainX, pkgLodash, KindUnknown, ""},
		{"a complete tsconfig nearer the importer decides", "chain/complete/x.ts", pkgLodash, KindExternal, ""},

		{".js with a .ts beside it names the .ts", extMain, "./model.js", KindFile, "ext/model.ts"},
		{".js alone names the .js", extMain, "./only.js", KindFile, "ext/only.js"},
		{".mjs with a .mts beside it names the .mts", extMain, "./mod.mjs", KindFile, "ext/mod.mts"},
		{".mjs with a .d.mts beside it names the .d.mts", extMain, "./mdecl.mjs", KindFile, extMdecl},
		{".cjs with a .cts beside it names the .cts", extMain, "./com.cjs", KindFile, "ext/com.cts"},
		{".jsx with a .tsx beside it names the .tsx", extMain, "./view.jsx", KindFile, "ext/view.tsx"},
		{".js with a .d.ts beside it names the .d.ts", extMain, "./decl.js", KindFile, "ext/decl.d.ts"},
		{".js names a .jsx file when no other is there", extMain, "./jsxonly.js", KindFile, "ext/jsxonly.jsx"},
		{".ts with a .d.ts beside it names the .ts", extMain, "./both.ts", KindFile, extBoth},
		{".ts names a .tsx file when no .ts is there", extMain, "./tsxonly.ts", KindFile, "ext/tsxonly.tsx"},
		{".tsx names a .ts file when no .tsx is there", extMain, "./tsonly.tsx", KindFile, "ext/tsonly.ts"},
		{".ts names a .tsx before a .d.ts", extMain, "./tx.ts", KindFile, "ext/tx.tsx"},
		// TypeScript tries both.ts first for an import of a declaration, but not for a
		// reference path or a package.json field, which reach this resolver the same way.
		{".d.ts names itself", extMain, "./both.d.ts", KindFile, "ext/both.d.ts"},
		{"types field names its declaration as written", srcX, "decl", KindFile, "packages/decl/index.d.ts"},
		{".mts names its .d.mts", extMain, "./mdecl.mts", KindFile, extMdecl},
		{".cts names its .d.cts", extMain, "./cdecl.cts", KindFile, "ext/cdecl.d.cts"},
		{"paths target names its file as written", jsAliasX, "foo", KindFile, "jsalias/src/foo.js"},
		{"paths target without its file names the source", jsAliasX, "bar", KindFile, "jsalias/src/bar.ts"},
		{"a relative .js beside that alias names the source", jsAliasX, "./foo.js", KindFile, "jsalias/src/foo.ts"},
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

func TestTsconfigUnread(t *testing.T) {
	t.Parallel()
	const (
		base     = "tsconfig.base.json"
		extBase  = `{"extends": "./tsconfig.base.json"}`
		extLoopA = `{"extends": "./a.json"}`
	)
	tests := []struct {
		name  string
		files map[string]string
		want  []UnknownItem
	}{
		{"no tsconfig", map[string]string{}, nil},
		{"relative extends is read", map[string]string{tsconfigJSON: extBase, base: "{}"}, nil},
		{"empty tsconfig is read", map[string]string{tsconfigJSON: ""}, nil},
		{"tsconfig of comments only is read", map[string]string{tsconfigJSON: "// none\n/* yet */\n"}, nil},
		{"byte order mark is dropped", map[string]string{tsconfigJSON: "\uFEFF" + extBase, base: "\uFEFF{}"}, nil},
		{"package", map[string]string{tsconfigJSON: extendsCompany},
			[]UnknownItem{{tsconfigJSON, companyConfig, reasonPackage}}},
		{"package subpath", map[string]string{tsconfigJSON: `{"extends": "@tsconfig/svelte/tsconfig.json"}`},
			[]UnknownItem{{tsconfigJSON, "@tsconfig/svelte/tsconfig.json", reasonPackage}}},
		{"file in node_modules", map[string]string{
			tsconfigJSON: `{"extends": "./node_modules/@tsconfig/base/tsconfig.json"}`,
			"node_modules/@tsconfig/base/tsconfig.json": "{}",
		}, []UnknownItem{{tsconfigJSON, "./node_modules/@tsconfig/base/tsconfig.json", reasonNodeModules}}},
		{"path above the root", map[string]string{tsconfigJSON: `{"extends": "../shared/tsconfig.json"}`},
			[]UnknownItem{{tsconfigJSON, "../shared/tsconfig.json", reasonOutside}}},
		{"absolute path", map[string]string{tsconfigJSON: `{"extends": "/etc/tsconfig.json"}`},
			[]UnknownItem{{tsconfigJSON, "/etc/tsconfig.json", reasonOutside}}},
		{"missing file", map[string]string{tsconfigJSON: `{"extends": "./gone"}`},
			[]UnknownItem{{tsconfigJSON, specGone, reasonMissing}}},
		{"loop stops at the depth limit", map[string]string{
			tsconfigJSON: extLoopA, "a.json": `{"extends": "./b.json"}`, "b.json": extLoopA,
		}, []UnknownItem{{"b.json", "./a.json", reasonDepth}}},
		{"extended file does not parse", map[string]string{tsconfigJSON: extBase, base: "{"},
			[]UnknownItem{{base, "", reasonNoParse}}},
		{"tsconfig does not parse", map[string]string{tsconfigJSON: `["not an object"]`},
			[]UnknownItem{{tsconfigJSON, "", reasonNoParse}}},
		{"an unread link deep in the chain", map[string]string{tsconfigJSON: extBase, base: extendsCompany},
			[]UnknownItem{{base, companyConfig, reasonPackage}}},
		{"array extends keeps the read ones", map[string]string{
			tsconfigJSON: `{"extends": ["./tsconfig.base.json", "@company/tsconfig", "./gone.json"]}`, base: "{}",
		}, []UnknownItem{{tsconfigJSON, companyConfig, reasonPackage}, {tsconfigJSON, "./gone.json", reasonMissing}}},
	}
	for _, tt := range tests {
		got := newResolver(tree(tt.files)).tsconfig("src").unread
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: unread = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestReadJSONDropsByteOrderMark(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{pkgJSON: "\uFEFF{\"name\": \"x\"}"})
	if got, _ := asString(readJSON(fsys, pkgJSON).get("name")); got != "x" {
		t.Errorf("name = %q, want x", got)
	}
}

func TestPackageName(t *testing.T) {
	t.Parallel()
	const scoped = "@s/a"
	for spec, want := range map[string]string{
		pkgReact: pkgReact, "react/jsx-runtime": pkgReact, scoped: scoped, "@s/a/b/c": scoped, "@s": "@s",
	} {
		if got := packageName(spec); got != want {
			t.Errorf("packageName(%q) = %q, want %q", spec, got, want)
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
		{"byte order mark", "\uFEFF{\"a\": \"x\"}", "x"},
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

func TestRootBaseConfigFallback(t *testing.T) {
	t.Parallel()
	const base = "tsconfig.base.json"
	for _, tc := range []struct {
		name    string
		configs map[string]string
		kind    Kind
		target  string
		unread  int
	}{
		{"base aliases", map[string]string{base: `{"compilerOptions":{"paths":{"@app/*":["src/*"]}}}`}, KindFile, srcA, 0},
		{"root config wins", map[string]string{base: `{"compilerOptions":{"paths":{"@app/*":["missing/*"]}}}`, tsconfigJSON: `{"compilerOptions":{"paths":{"@app/*":["src/*"]}}}`}, KindFile, srcA, 0},
		{"nearest config wins", map[string]string{base: `{"compilerOptions":{"paths":{"@app/*":["missing/*"]}}}`, "src/tsconfig.json": `{"compilerOptions":{"paths":{"@app/*":["./*"]}}}`}, KindFile, srcA, 0},
		{"unread base keeps imports unknown", map[string]string{base: `{`}, KindUnknown, "", 1},
		{"no config keeps external imports external", nil, KindExternal, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{srcX: "", srcA: ""}
			maps.Copy(files, tc.configs)
			r := newResolver(tree(files))
			kind, target := r.resolve(srcX, "@app/a")
			if kind != tc.kind || target != tc.target || len(r.tsconfig("src").unread) != tc.unread {
				t.Fatalf("resolved=(%s, %q), unread=%+v", kind, target, r.tsconfig("src").unread)
			}
		})
	}
}
