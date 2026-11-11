package registry_test

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
	"github.com/alexei-led/archfit/v3/internal/extract/registry"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	fileGoMod  = "go.mod"
	goModule   = "module example.com/m\n\ngo 1.22\n"
	fileMainGo = "main.go"
	filePkgA   = "pkg/a/a.go"
	fileAppX   = "src/app/x.py"
	fileAppY   = "src/app/y.py"
	fileWebA   = "web/a.ts"
	fileWebB   = "web/b.ts"
	fileCargo  = "Cargo.toml"
	keyName    = "name"
	keyKind    = "kind"
	fixImport  = "fix the import"
	fixSyntax  = "fix the syntax error"
	fixAstGrep = "install ast-grep: https://github.com/alexei-led/archfit/blob/main/docs/languages.md#install-analyzers"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func auto(ids ...string) registry.Configs {
	c := registry.Configs{}
	for _, id := range ids {
		c[id] = evidenceports.ExtractConfig{Src: ".", Mode: evidenceports.ModeAuto}
	}
	return c
}

// withoutTool hides tools from Detect and Run, as when they are not on PATH.
type withoutTool struct {
	toolrun.Runner
	hidden []string
}

func (w withoutTool) Detect(ctx context.Context, tool string) (toolrun.ToolInfo, bool) {
	if slices.Contains(w.hidden, tool) {
		return toolrun.ToolInfo{}, false
	}
	return w.Runner.Detect(ctx, tool)
}

func requireAstGrep(t *testing.T) {
	t.Helper()
	if _, ok := toolrun.New().Detect(context.Background(), registry.ToolAstGrep); !ok {
		t.Skip("ast-grep not installed")
	}
}

// noPackageName is a Go file whose header does not parse.
const noPackageName = "package\n"

// goTree is a Go module with a first-party, a standard library and an
// unresolved import, a test file and testdata (imports not read, literals
// read), a file whose header does not parse, and excluded code.
var goTree = map[string]string{
	fileGoMod:            goModule,
	fileMainGo:           "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/m/gone\"\n\t\"example.com/m/pkg/a\"\n)\n\nconst s = \"a long literal value\"\n\nfunc main() { fmt.Println(a.X, gone.Y, s) }\n",
	filePkgA:             "package a\n\nconst X = \"word\"\n",
	"pkg/a/a_test.go":    "package a\n\nimport \"testing\"\n\nfunc TestX(*testing.T) { _ = `json:\"x\"` }\n",
	"testdata/t.go":      "package t\n\nimport \"example.com/m/pkg/a\"\n\nvar _ = \"from testdata\"\n",
	"bad.go":             noPackageName,
	"testdata/broken.go": noPackageName,
	"_tools/broken.go":   noPackageName,
	"gen/g.go":           "package gen\n\nimport \"example.com/m/pkg/a\"\n\nvar _ = \"generated text\" + a.X\n",
	"vendor/v/v.go":      "package v\n\nvar _ = \"vendored text\"\n",
	"web/vendor/x.go":    "package x\n",
	"node_modules/n.go":  "package n\n",
}

func TestReadCheckGo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, goTree)
	got, err := registry.ReadCheck(context.Background(), &toolrun.RunnerMock{}, root, registry.CheckRequest{
		Configs: auto("go"), Exclude: []string{"gen/**"}, Literals: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := evidence.CheckFacts{
		Sources: []string{fileMainGo, filePkgA},
		Imports: []evidence.ImportFact{
			{File: fileMainGo, Line: 4, Target: "fmt", External: true},
			{File: fileMainGo, Line: 7, Target: "pkg/a"},
		},
		Literals: []evidence.LiteralOccurrence{
			{File: fileMainGo, Line: 10, Value: "a long literal value", Shape: evidence.ShapeOther},
			{File: filePkgA, Line: 3, Value: "word", Shape: evidence.ShapeWord},
			{File: "pkg/a/a_test.go", Line: 5, Value: `json:"x"`, Shape: evidence.ShapeTag},
			{File: "testdata/t.go", Line: 5, Value: "from testdata", Shape: evidence.ShapeOther},
		},
	}
	wantUnknown := []string{
		"bad.go:0 import " + fixSyntax,
		"main.go:6 example.com/m/gone import " + fixImport,
	}
	if gotUnknown := unknownLines(got.Unknown); !slices.Equal(gotUnknown, wantUnknown) {
		t.Errorf("unknown =\n%q\nwant\n%q", gotUnknown, wantUnknown)
	}
	got.Unknown = nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("facts =\n%s\nwant\n%s", jsonOf(got), jsonOf(want))
	}
}

func unknownLines(items []evidence.UnknownItem) []string {
	out := make([]string, 0, len(items))
	for _, u := range items {
		spec := ""
		if u.Specifier != "" {
			spec = " " + u.Specifier
		}
		out = append(out, u.File+":"+itoa(u.Line)+spec+" "+string(u.Cause)+" "+u.Fix)
	}
	return out
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func jsonOf(v any) string {
	b, _ := json.MarshalIndent(v, "", " ")
	return string(b)
}

// scriptTree holds TypeScript, Python and Go: a first-party, an external and
// an unresolved import each, a tsconfig extends of a package, and code the
// policy excludes or node_modules holds.
var scriptTree = map[string]string{
	fileGoMod:             goModule,
	fileMainGo:            "package main\n\nfunc main() {}\n",
	"web/tsconfig.json":   `{"extends": "@tsconfig/node20/tsconfig.json"}`,
	fileWebA:              "import { b } from './b';\nimport _ from 'lodash';\nimport { c } from './nope';\nexport const s = 'a typescript literal';\n",
	fileWebB:              "export const b = 1;\n",
	"web/package.json":    `{"dependencies": {"lodash": "4"}}`,
	"gen/g.ts":            "import { b } from '../web/b';\n",
	"node_modules/x/i.ts": "import y from 'y';\n",
	"src/app/__init__.py": "",
	fileAppX:              "import os\nfrom app import y\nimport app.nope\nS = 'a python literal'\n",
	fileAppY:              "",
}

func TestReadCheckTypeScriptAndPython(t *testing.T) {
	t.Parallel()
	requireAstGrep(t)
	root := t.TempDir()
	writeFiles(t, root, scriptTree)
	got, err := registry.ReadCheck(context.Background(), toolrun.New(), root, registry.CheckRequest{
		Configs: auto("go", "typescript", "python"), Exclude: []string{"gen/**"}, Literals: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSources := []string{fileMainGo, "src/app/__init__.py", fileAppX, fileAppY, fileWebA, fileWebB}
	if !slices.Equal(got.Sources, wantSources) {
		t.Errorf("sources = %q, want %q", got.Sources, wantSources)
	}
	wantImports := []evidence.ImportFact{
		{File: fileAppX, Line: 1, Target: "os", External: true},
		{File: fileAppX, Line: 2, Target: fileAppY},
		{File: fileWebA, Line: 1, Target: fileWebB},
		{File: fileWebA, Line: 2, Target: "lodash", External: true},
	}
	if !reflect.DeepEqual(got.Imports, wantImports) {
		t.Errorf("imports =\n%s\nwant\n%s", jsonOf(got.Imports), jsonOf(wantImports))
	}
	wantUnknown := []string{
		"src/app/x.py:3 app.nope import " + fixImport,
		"web/a.ts:3 ./nope import " + fixImport,
		"web/tsconfig.json:0 @tsconfig/node20/tsconfig.json import make the extends a relative path inside the repository",
	}
	if gotUnknown := unknownLines(got.Unknown); !slices.Equal(gotUnknown, wantUnknown) {
		t.Errorf("unknown =\n%q\nwant\n%q", gotUnknown, wantUnknown)
	}
	values := make([]string, 0, len(got.Literals))
	for _, l := range got.Literals {
		values = append(values, l.File+":"+l.Value)
	}
	if want := []string{"src/app/x.py:a python literal", "web/a.ts:a typescript literal"}; !slices.Equal(values, want) {
		t.Errorf("literals = %q, want %q", values, want)
	}
}

// TestReadCheckWithoutAstGrep: TypeScript, Python and their literals are
// unknown items of cause tool, about the whole tree; Go is read in full.
func TestReadCheckWithoutAstGrep(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, scriptTree)
	writeFiles(t, root, map[string]string{fileMainGo: "package main\n\nconst s = \"a go literal\"\n"})
	runner := withoutTool{Runner: toolrun.New(), hidden: []string{registry.ToolAstGrep}}
	got, err := registry.ReadCheck(context.Background(), runner, root, registry.CheckRequest{
		Configs: auto("go", "typescript", "python"), Literals: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	reasons := make([]string, 0, len(got.Unknown))
	for _, u := range got.Unknown {
		if u.File != "" || u.Cause != evidence.CauseTool || u.Fix != fixAstGrep {
			t.Errorf("unknown %+v, want a whole-tree tool item that names ast-grep", u)
		}
		reasons = append(reasons, u.Reason)
	}
	want := []string{
		"ast-grep is not installed; it reads TypeScript and JavaScript imports",
		"ast-grep is not installed; it reads Python imports",
		"ast-grep is not installed; it reads the literals of typescript, python",
	}
	slices.Sort(want)
	slices.Sort(reasons)
	if !slices.Equal(reasons, want) {
		t.Errorf("reasons = %q, want %q", reasons, want)
	}
	if !slices.Equal(got.Sources, []string{fileMainGo}) || len(got.Literals) != 1 || got.Literals[0].Value != "a go literal" {
		t.Errorf("Go facts = %q, %+v; want main.go and its literal", got.Sources, got.Literals)
	}
}

// TestReadCheckExcludedScriptsNeedNoAstGrep: TypeScript and JavaScript only
// under vendor/ or a policy exclude are not read, so a Go repository needs no
// ast-grep and has no unknown item.
func TestReadCheckExcludedScriptsNeedNoAstGrep(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		fileGoMod:             goModule,
		fileMainGo:            "package main\n",
		"vendor/lib/x.min.js": "var a='b';\n",
		"web/app.ts":          "import x from './y';\n",
	})
	runner := withoutTool{Runner: toolrun.New(), hidden: []string{registry.ToolAstGrep}}
	got, err := registry.ReadCheck(context.Background(), runner, root, registry.CheckRequest{
		Configs: auto("go", "typescript", "python"), Exclude: []string{"web/**"}, Literals: true,
	})
	want := evidence.CheckFacts{Sources: []string{fileMainGo}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("facts = %s, %v; want main.go only and no unknown", jsonOf(got), err)
	}
}

// TestReadCheckLooseScripts: Python files outside any package are not a
// Python project, so a Go repository with scripts needs no ast-grep and has
// no unknown item.
func TestReadCheckLooseScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		fileGoMod:         goModule,
		fileMainGo:        "package main\n",
		"scripts/tool.py": "import os\nS = 'a script literal'\n",
	})
	runner := withoutTool{Runner: toolrun.New(), hidden: []string{registry.ToolAstGrep}}
	got, err := registry.ReadCheck(context.Background(), runner, root, registry.CheckRequest{
		Configs: auto("go", "typescript", "python"), Literals: true,
	})
	want := evidence.CheckFacts{Sources: []string{fileMainGo}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("facts = %s, %v; want main.go only", jsonOf(got), err)
	}
}

// TestReadCheckSkipsWhatIsOff: a language that is off, or absent from the
// configs, is not read; without Literals no literal is read.
func TestReadCheckSkipsWhatIsOff(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{fileGoMod: goModule, fileMainGo: "package main\n\nvar _ = \"a literal\"\n"})
	off := auto("go")
	off["go"] = evidenceports.ExtractConfig{Mode: evidenceports.ModeOff}
	for name, req := range map[string]registry.CheckRequest{
		"off":    {Configs: off, Literals: true},
		"absent": {Configs: registry.Configs{}, Literals: true},
	} {
		got, err := registry.ReadCheck(context.Background(), &toolrun.RunnerMock{}, root, req)
		if err != nil || !reflect.DeepEqual(got, evidence.CheckFacts{}) {
			t.Errorf("%s: facts = %s, %v; want none", name, jsonOf(got), err)
		}
	}
	got, err := registry.ReadCheck(context.Background(), &toolrun.RunnerMock{}, root, registry.CheckRequest{Configs: auto("go")})
	if err != nil || len(got.Literals) != 0 || !slices.Equal(got.Sources, []string{fileMainGo}) {
		t.Errorf("without literals: facts = %s, %v; want main.go and no literal", jsonOf(got), err)
	}
}

// rustTree is a workspace: the root crate app depends on the workspace crate
// core and on serde.
var rustTree = map[string]string{
	fileCargo:                "[package]\nname = \"app\"\n\n[dependencies]\ncore = { path = \"crates/core\" }\nserde = \"1\"\n",
	"src/main.rs":            "fn main() {}\n",
	"crates/core/Cargo.toml": "[package]\nname = \"core\"\n",
	"crates/core/src/lib.rs": "pub fn f() {}\n",
	"vendor/x/lib.rs":        "",
}

func cargoMetadata(t *testing.T, root string) []byte {
	t.Helper()
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	registrySource := "registry+https://github.com/rust-lang/crates.io-index"
	pkg := func(name, dir string, deps ...map[string]any) map[string]any {
		return map[string]any{
			"id": "path+file://" + dir + "#" + name, keyName: name, "manifest_path": filepath.Join(dir, fileCargo),
			"dependencies": deps, "targets": []map[string]any{{keyName: name, keyKind: []string{"lib"}}},
		}
	}
	core := filepath.Join(resolvedRoot, "crates", "core")
	data, err := json.Marshal(map[string]any{
		"packages": []map[string]any{
			pkg("app", resolvedRoot, map[string]any{keyName: "core", "source": nil, keyKind: nil}, map[string]any{keyName: "serde", "source": registrySource, keyKind: nil}),
			pkg("core", core),
		},
		"workspace_members": []string{"path+file://" + resolvedRoot + "#app", "path+file://" + core + "#core"},
		"workspace_root":    resolvedRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func fakeCargo(present bool, metadata []byte) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, present && tool == registry.ToolCargo
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if slices.Contains(cmd.Args, "--version") {
				return toolrun.Output{Stdout: []byte("cargo 1.90.0\n")}, nil
			}
			if slices.Contains(cmd.Args, "metadata") {
				return toolrun.Output{Stdout: metadata}, nil
			}
			return toolrun.Output{ExitCode: 1, Stderr: []byte("unexpected " + strings.Join(cmd.Args, " "))}, nil
		},
	}
}

// TestReadCheckRust: a dependency on a workspace crate targets its directory,
// one on a registry crate is external, both sited on their Cargo.toml line;
// the sources are the .rs files outside the exclusions. cargo-modules never
// runs.
func TestReadCheckRust(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, rustTree)
	runner := fakeCargo(true, cargoMetadata(t, root))
	got, err := registry.ReadCheck(context.Background(), runner, root, registry.CheckRequest{Configs: auto("rust")})
	if err != nil {
		t.Fatal(err)
	}
	want := evidence.CheckFacts{
		Sources: []string{"crates/core/src/lib.rs", "src/main.rs"},
		Imports: []evidence.ImportFact{
			{File: fileCargo, Line: 5, Target: "crates/core"},
			{File: fileCargo, Line: 6, Target: "serde", External: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("facts =\n%s\nwant\n%s", jsonOf(got), jsonOf(want))
	}
	for _, call := range runner.RunCalls() {
		if call.Cmd.Name != registry.ToolCargo || slices.Contains(call.Cmd.Args, "modules") {
			t.Errorf("ran %s %q, want cargo metadata only", call.Cmd.Name, call.Cmd.Args)
		}
	}
}

func TestReadCheckRustWithoutCargo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, rustTree)
	got, err := registry.ReadCheck(context.Background(), fakeCargo(false, nil), root, registry.CheckRequest{Configs: auto("rust")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Imports) != 0 || len(got.Unknown) != 1 || got.Unknown[0].Cause != evidence.CauseTool ||
		got.Unknown[0].Fix != "install Rust: https://rustup.rs" {
		t.Errorf("facts = %s; want one tool item that names Rust", jsonOf(got))
	}
}

func TestReadCheckDotNetWithoutDotnet(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"App/App.csproj": "<Project Sdk=\"Microsoft.NET.Sdk\" />\n", "App/A.cs": "class A {}\n"})
	runner := &toolrun.RunnerMock{DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false }}
	got, err := registry.ReadCheck(context.Background(), runner, root, registry.CheckRequest{Configs: auto("dotnet")})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Sources, []string{"App/A.cs"}) || len(got.Unknown) != 1 ||
		got.Unknown[0].Fix != "install the .NET SDK: https://dotnet.microsoft.com/download" {
		t.Errorf("facts = %s; want App/A.cs and one tool item that names the .NET SDK", jsonOf(got))
	}
}

// TestReadCheckDotNetOnFixture: project references through the real SDK,
// with no Roslyn helper: every import is a project file line that targets a
// referenced project's directory.
func TestReadCheckDotNetOnFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("evaluating the fixture takes seconds")
	}
	runner := toolrun.New()
	if _, ok := runner.Detect(context.Background(), "dotnet"); !ok {
		t.Skip("dotnet is not on PATH")
	}
	t.Setenv("DOTNET_CLI_HOME", t.TempDir())
	t.Setenv("DOTNET_NOLOGO", "1")
	t.Setenv("DOTNET_CLI_TELEMETRY_OPTOUT", "1")
	t.Setenv("ARCHFIT_DOTNET_ROSLYN", "")
	root := copyTree(t, filepath.Join("..", "..", "testdata", "dotnet"))
	got, err := registry.ReadCheck(context.Background(), runner, root, registry.CheckRequest{Configs: auto("dotnet")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Imports) == 0 {
		t.Fatalf("no project reference read: %s", jsonOf(got))
	}
	for _, imp := range got.Imports {
		if !strings.HasSuffix(imp.File, ".csproj") || imp.External || imp.Line == 0 {
			t.Errorf("import %+v, want a line of a project file", imp)
		}
		if matches, _ := filepath.Glob(filepath.Join(root, imp.Target, "*.csproj")); len(matches) == 0 {
			t.Errorf("import %+v targets no project directory", imp)
		}
	}
	if !slices.ContainsFunc(got.Sources, func(s string) bool { return strings.HasSuffix(s, ".cs") }) {
		t.Errorf("sources = %q, want the .cs files", got.Sources)
	}
}
