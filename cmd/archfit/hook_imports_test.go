package main

import (
	"maps"
	"slices"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/config"
)

// TestImportTargets pins what post-tool-use reads from the added lines of a
// source file: the import forms per language, and nothing from a line the
// change did not add.
func TestImportTargets(t *testing.T) {
	t.Parallel()
	const (
		tsApp   = "web/src/app.ts"
		tsIndex = "web/src/ui/index.tsx"
	)
	files := map[string]bool{
		"web/src/db/client.ts":    true,
		tsIndex:                   true,
		"web/src/legacy.js":       true,
		"web/src/util/fmt.ts":     true,
		tsApp:                     true,
		"web/src/shared/index.js": true,
	}
	isFile := func(p string) bool { return files[p] }
	const goBlock = "package a\n\nimport (\n\t\"fmt\"\n\tx \"example.com/m/pkg/b\"\n\t_ \"example.com/m/pkg/c\" // side effect (init)\n\t. \"example.com/m/pkg/d\"\n)\n\nvar s = \"example.com/m/not/an/import\"\n"
	for _, tc := range []struct {
		name    string
		lang    string
		file    string
		content string
		added   []int // nil: every line is new
		want    []string
	}{
		{name: "go single import with an alias", lang: config.LangGo, file: filePkgAA,
			content: "package a\n\nimport impl \"example.com/m/pkg/b/impl\"\n",
			want:    []string{"example.com/m/pkg/b/impl"}},
		{name: "go block, every line new", lang: config.LangGo, file: filePkgAA, content: goBlock,
			want: []string{"example.com/m/pkg/b", "example.com/m/pkg/c", "example.com/m/pkg/d", "fmt"}},
		{name: "go block, one added spec", lang: config.LangGo, file: filePkgAA, content: goBlock, added: []int{5},
			want: []string{"example.com/m/pkg/b"}},
		{name: "go block, an added line outside the block", lang: config.LangGo, file: filePkgAA, content: goBlock, added: []int{10},
			want: nil},
		{name: "go one-line block", lang: config.LangGo, file: filePkgAA,
			content: "package a\n\nimport (\"os\")\n\nvar x = \"y\"\n", want: []string{"os"}},
		{name: "ts relative specifiers resolve to files", lang: config.LangTypeScript, file: tsApp,
			content: "import { db } from './db/client';\nimport './ui';\nconst l = require(\"./legacy\");\nconst f = await import('./util/fmt.ts');\nexport * from './shared';\n",
			want:    []string{"web/src/db/client.ts", "web/src/legacy.js", "web/src/shared/index.js", tsIndex, "web/src/util/fmt.ts"}},
		{name: "ts bare, unresolved, and escaping specifiers are skipped", lang: config.LangTypeScript, file: tsApp,
			content: "import React from 'react';\nimport { x } from './missing';\nimport y from '../../../outside';\n", want: nil},
		{name: "ts only added lines", lang: config.LangTypeScript, file: tsApp,
			content: "import { db } from './db/client';\nimport './ui';\n", added: []int{2},
			want: []string{tsIndex}},
		{name: "python import and from", lang: config.LangPython, file: "src/app/views.py",
			content: "import app.domain.model, os.path as p\nfrom app.infra.db import Session  # storage\nfrom . import sibling\nfrom .local import thing\n",
			want:    []string{"app.domain.model", "app.infra.db", "os.path"}},
		{name: "python only added lines", lang: config.LangPython, file: "src/app/views.py",
			content: "import app.domain\nimport app.infra\n", added: []int{1},
			want: []string{"app.domain"}},
		{name: "rust is not read", lang: config.LangRust, file: "src/lib.rs",
			content: "use crate::infra::db;\n", want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var added map[int]bool
			if tc.added != nil {
				added = map[int]bool{}
				for _, n := range tc.added {
					added[n] = true
				}
			}
			got := importTargets(tc.lang, tc.file, []byte(tc.content), added, isFile)
			if !slices.Equal(got, tc.want) {
				t.Errorf("importTargets = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseAddedLines reads the new-file line numbers of a git diff -U0.
func TestParseAddedLines(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		diff string
		want []int
	}{
		{name: "a hunk of three added lines", diff: "@@ -1,0 +2,3 @@\n+a\n+b\n+c\n", want: []int{2, 3, 4}},
		{name: "a one-line change", diff: "@@ -5 +5 @@ func x()\n-old\n+new\n", want: []int{5}},
		{name: "a deletion adds nothing", diff: "@@ -3,2 +2,0 @@\n-a\n-b\n", want: []int{}},
		{name: "a new file", diff: "diff --git a/x.go b/x.go\nnew file mode 100644\n--- /dev/null\n+++ b/x.go\n@@ -0,0 +1,2 @@\n+package x\n+\n", want: []int{1, 2}},
		{name: "an added line that looks like a header is a body line", diff: "@@ -1,0 +1 @@\n+@@ -9 +9 @@\n", want: []int{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := slices.Sorted(maps.Keys(parseAddedLines(tc.diff)))
			if !slices.Equal(got, tc.want) {
				t.Errorf("parseAddedLines = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSessionFileName keeps a harness session id to a safe file name.
func TestSessionFileName(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]string{
		"0b6f1c4e-9d2a-4c1b-8f3e-7a5d2c9e1b40": "0b6f1c4e-9d2a-4c1b-8f3e-7a5d2c9e1b40.json",
		"../../etc/passwd":                     "______etc_passwd.json",
		"":                                     "default.json",
	} {
		if got := sessionFileName(id); got != want {
			t.Errorf("sessionFileName(%q) = %q, want %q", id, got, want)
		}
	}
}
