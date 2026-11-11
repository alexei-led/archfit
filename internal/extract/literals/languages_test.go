package literals

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

// realAstGrep returns the real runner, or skips when ast-grep is not
// installed. CI provisions it.
func realAstGrep(t *testing.T) toolrun.Runner {
	t.Helper()
	runner := toolrun.New()
	if _, ok := runner.Detect(context.Background(), astGrep); !ok {
		t.Skip("ast-grep is not installed: install it to read literals with the real parser")
	}
	return runner
}

// The pumba code the trap agents copied (design appendix D.4): a regular
// expression and an error text, both found by the copy-noise scan.
const (
	pumbaRegex = "[a-zA-Z][a-zA-Z0-9.:_-]*"
	pumbaError = "bad network interface name: must match '%s'"
)

// languageCases are one file per language, and the literals of each, as
// "line value". Each file has a comment, an import or the language's
// non-literal strings, and a multi-line string, none of which is read.
var languageCases = []struct {
	name, file, source string
	want               []string
}{
	{
		name: "Go",
		file: "pkg/util/util.go",
		source: "package util\n" +
			"\n" +
			"import (\n" +
			"\t\"fmt\"\n" +
			"\tre \"regexp\" // \"comment in import\"\n" +
			")\n" +
			"\n" +
			"// Parse reads \"quoted words in a comment\".\n" +
			"var reInterface = re.MustCompile(`" + pumbaRegex + "`)\n" +
			"\n" +
			"type Rule struct {\n" +
			"\tName string `json:\"name\" yaml:\"name\"`\n" +
			"}\n" +
			"\n" +
			"/* \"block comment\" */\n" +
			"func Validate(name string) error {\n" +
			"\tif name == \"\" {\n" +
			"\t\treturn fmt.Errorf(\"" + pumbaError + "\", reInterface.String())\n" +
			"\t}\n" +
			"\treturn fmt.Errorf(\"a\\\"b\" + `raw\\d`)\n" +
			"}\n" +
			"\n" +
			"var table = []string{\n" +
			"\t\"first element of a table\",\n" +
			"}\n" +
			"\n" +
			"var multi = `line one\n" +
			"line two`\n",
		want: []string{"9 " + pumbaRegex, "17 ", "18 " + pumbaError, "20 a\\\"b", "20 raw\\d", "24 first element of a table"},
	},
	{
		name:   "Go with a line directive",
		file:   "gen/parser.go",
		source: "package gen\n\n//line grammar.y:1000\nconst s = \"physical line three\"\n",
		want:   []string{"4 physical line three"},
	},
	{
		name: "Python",
		file: "pkg/util.py",
		source: "\"\"\"Module docstring is prose.\"\"\"\n" +
			"import importlib\n" +
			"from os import path\n" +
			"# a comment with \"quoted words\"\n" +
			"x = r'\\d+' + f\"{path}\" + b'z' + u\"w\"\n" +
			"doc = \"\"\"one line\"\"\"\n" +
			"\n" +
			"def validate(name):\n" +
			"    \"\"\"Function docstring is prose.\"\"\"\n" +
			"    raise ValueError(\"" + pumbaError + "\")\n" +
			"\n" +
			"class Rule:\n" +
			"    'class docstring is prose'\n" +
			"    ('parenthesised docstring')\n" +
			"    'split' ' docstring'\n" +
			"    kept = ('parenthesised value', 'split' ' value')\n" +
			"    pattern = '" + pumbaRegex + "'\n" +
			"\n" +
			"m = importlib.import_module(\"a.b\")\n" +
			"multi = \"\"\"line one\n" +
			"line two\"\"\"\n",
		want: []string{"5 \\d+", "5 w", "6 one line", "10 " + pumbaError, "16  value", "16 parenthesised value", "16 split", "17 " + pumbaRegex, "19 a.b"},
	},
	{
		name: "TypeScript",
		file: "src/util.ts",
		source: "/// <reference path=\"../types.d.ts\" />\n" +
			"import a from './a';\n" +
			"import type { T } from './t';\n" +
			"export * from './re';\n" +
			"import eq = require('./eq');\n" +
			"const d = import('./dyn');\n" +
			"const r = require('./req');\n" +
			"const dt = import(`./dyn/template`), rt = require(`./req/template`);\n" +
			"// const c = \"commented\";\n" +
			"const re = /" + pumbaRegex + "/;\n" +
			"const e = \"" + pumbaError + "\";\n" +
			"const t = `plain template`, u = `with ${e}`, q = 'q';\n" +
			"/* \"block\" */ const multi = `line one\n" +
			"line two`;\n",
		want: []string{"11 " + pumbaError, "12 plain template", "12 q"},
	},
	{
		name:   "TSX",
		file:   "src/view.tsx",
		source: "import React from 'react';\nconst v = <div title=\"a title\">{'text'}</div>;\n",
		want:   []string{"2 a title", "2 text"},
	},
	{
		name:   "JavaScript",
		file:   "lib/util.mjs",
		source: "import a from './a.js';\nconst r = require('./r');\nconst s = 'kept', u = `x${s}`; // 'comment'\n",
		want:   []string{"3 kept"},
	},
	{
		name: "Rust",
		file: "src/lib.rs",
		source: "use std::fmt;\n" +
			"// a comment \"not read\"\n" +
			"/// doc comment \"not read\"\n" +
			"pub fn f() -> &'static str {\n" +
			"    let b = b\"bytes\"; let c = 'x'; let r = r#\"a\"b\"#;\n" +
			"    println!(\"" + pumbaError + "\", 1);\n" +
			"    let multi = \"line one\n" +
			"line two\";\n" +
			"    \"text\"\n" +
			"}\n",
		want: []string{"5 a\"b", "6 " + pumbaError, "9 text"},
	},
	{
		name: "C#",
		file: "src/Rule.cs",
		source: "using System.Text;\n" +
			"// \"comment\"\n" +
			"class Rule {\n" +
			"    string s = \"" + pumbaError + "\";\n" +
			"    string v = @\"a \"\"b\"\"\";\n" +
			"    string i = $\"x {s}\", j = $@\"y {s}\";\n" +
			"    byte[] u = \"bytes\"u8;\n" +
			"    string r = \"\"\"raw \"quoted\" text\"\"\", ir = $\"\"\"x {s}\"\"\";\n" +
			"    char c = 'x';\n" +
			"}\n",
		want: []string{"4 " + pumbaError, "5 a \"\"b\"\"", "8 raw \"quoted\" text"},
	},
}

func TestExtractPerLanguage(t *testing.T) {
	t.Parallel()
	runner := realAstGrep(t)
	for _, tt := range languageCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, map[string]string{tt.file: tt.source})
			got, err := Extract(context.Background(), runner, root, Config{})
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, l := range got {
				if l.File != tt.file {
					t.Errorf("literal of %s, want only %s", l.File, tt.file)
				}
				lines = append(lines, valueLine(l))
			}
			if !slices.Equal(lines, tt.want) {
				t.Errorf("literals =\n%q\nwant\n%q", lines, tt.want)
			}
		})
	}
}

func valueLine(l Literal) string { return fmt.Sprintf("%d %s", l.Line, l.Value) }

// TestIndexOfFixtureTreeIsStable reads every language case as one tree: the
// pumba literals each language repeats are one index entry, and two runs give
// the same index.
func TestIndexOfFixtureTreeIsStable(t *testing.T) {
	t.Parallel()
	runner := realAstGrep(t)
	files := map[string]string{}
	for _, tt := range languageCases {
		files[tt.file] = tt.source
	}
	root := writeTree(t, files)
	first, err := Extract(context.Background(), runner, root, Config{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Extract(context.Background(), runner, root, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first, second) {
		t.Fatalf("two runs differ:\n%v\n%v", first, second)
	}
	idx := BuildIndex(first)
	sites := func(value string) []string {
		out := make([]string, 0, len(idx[value]))
		for _, l := range idx[value] {
			out = append(out, fmt.Sprintf("%s:%d", l.File, l.Line))
		}
		return out
	}
	wantError := []string{"pkg/util.py:10", "pkg/util/util.go:18", "src/Rule.cs:4", "src/lib.rs:6", "src/util.ts:11"}
	if got := sites(pumbaError); !slices.Equal(got, wantError) {
		t.Errorf("index[%q] = %q, want %q", pumbaError, got, wantError)
	}
	if got := sites(pumbaRegex); !slices.Equal(got, []string{"pkg/util.py:17", "pkg/util/util.go:9"}) {
		t.Errorf("index[%q] = %q, want the Go and Python sites", pumbaRegex, got)
	}
	keys := slices.Sorted(maps.Keys(idx))
	wantKeys := []string{pumbaRegex, pumbaError, "first element of a table", "parenthesised value", "physical line three", "plain template", `raw "quoted" text`}
	slices.Sort(wantKeys)
	if !slices.Equal(keys, wantKeys) {
		t.Errorf("index keys = %q, want %q", keys, wantKeys)
	}
}
