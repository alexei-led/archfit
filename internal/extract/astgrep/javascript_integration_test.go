package astgrep_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/extract/astgrep"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/pattern"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestSyntaxIntegration_JavaScript(t *testing.T) {
	const (
		jsFile    = "fixture.js"
		mjsFile   = "fixture.mjs"
		method    = "greet"
		framework = "express"
	)
	if testing.Short() {
		t.Skip("subprocess integration test — skipped with -short")
	}
	sgSkipGuard(t)
	root, err := filepath.Abs(filepath.Join("testdata", "javascript"))
	if err != nil {
		t.Fatal(err)
	}
	a := astgrep.New(toolrun.New())
	a.Cache = factcache.NewStore(t.TempDir())
	var firstJSON []byte
	// Cold, warm, and uncached results must be byte-identical, including ordering.
	for pass := range 3 {
		if pass == 2 {
			a.Cache = nil
		}
		facts, cov, err := a.Syntax(t.Context(), scope.Scope{Root: root}, []string{langTypeScriptStr})
		if err != nil || cov.Status != "ok" || cov.Tool != toolAstGrepSyntax || cov.FilesSeen != 3 {
			t.Fatalf("Syntax: coverage=%+v err=%v", cov, err)
		}
		type expectedFact struct {
			file, kind, name string
			start, end       int
		}
		want := []expectedFact{
			{"fixture.cjs", kindFunctionStr, "cjsGreet", 3, 5},
			{"fixture.cjs", kindRouteStr, "/cjs", 8, 10},
			{jsFile, kindFunctionStr, "jsGreet", 3, 5},
			{jsFile, kindClassStr, "JsGreeter", 7, 11},
			{jsFile, kindMethodStr, method, 8, 10},
			{jsFile, kindRouteStr, "/js", 14, 16},
			{mjsFile, kindFunctionStr, "mjsGreet", 3, 5},
			{mjsFile, kindClassStr, "MjsGreeter", 7, 11},
			{mjsFile, kindMethodStr, method, 8, 10},
			{mjsFile, kindRouteStr, "/mjs", 14, 16},
		}
		if len(facts) != len(want) {
			t.Fatalf("got %d facts, want %d: %+v", len(facts), len(want), facts)
		}
		for i, f := range facts {
			w := want[i]
			if f.File != w.file || f.Kind != w.kind || f.Name != w.name || f.StartLine != w.start || f.EndLine != w.end || f.Language != langTypeScriptStr {
				t.Errorf("fact[%d]=%+v, want %+v with language=typescript", i, f, w)
			}
			route := f.Kind == kindRouteStr
			if f.Exported == route || f.FrameworkConfirmed != route || (route && f.Framework != framework) || (!route && f.Framework != "") {
				t.Errorf("incorrect declaration/export/route semantics: %+v", f)
			}
		}
		encoded, err := json.Marshal(facts)
		if err != nil {
			t.Fatal(err)
		}
		if pass == 0 {
			firstJSON = encoded
		} else if !bytes.Equal(firstJSON, encoded) {
			t.Fatalf("pass %d changed output bytes:\n%s\n%s", pass, firstJSON, encoded)
		}
	}
}

func TestSyntaxIntegration_JavaScriptEmptyAndUnsupported(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration test — skipped with -short")
	}
	sgSkipGuard(t)
	for _, control := range []string{"empty", "unsupported.js", "unsupported.cjs"} {
		t.Run(control, func(t *testing.T) {
			root := t.TempDir()
			if control != "empty" {
				data, err := os.ReadFile(filepath.Join("testdata", "javascript", control)) //nolint:gosec // control is a fixed fixture name from the table above
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, control), data, 0o600); err != nil { //nolint:gosec // fixed fixture name under t.TempDir
					t.Fatal(err)
				}
			}
			facts, cov, err := astgrep.New(toolrun.New()).Syntax(t.Context(), scope.Scope{Root: root}, []string{langTypeScriptStr})
			if err != nil || cov.Status != "ok" || cov.FilesSeen != 0 || len(facts) != 0 {
				t.Fatalf("negative control: facts=%+v coverage=%+v err=%v", facts, cov, err)
			}
		})
	}
}

func TestAdapterIntegration_CanonicalBinaryIgnoresDecoySG(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration test — skipped with -short")
	}
	canonical, err := exec.LookPath(toolAstGrep)
	if err != nil {
		t.Skip("ast-grep not found on PATH")
	}
	// npm's ast-grep launcher needs node; native installations do not.
	node, _ := exec.LookPath("node")
	for _, present := range []bool{true, false} {
		t.Run(fmt.Sprintf("canonical-present=%t", present), func(t *testing.T) {
			bin := t.TempDir()
			marker := filepath.Join(bin, "sg-was-invoked")
			decoy := fmt.Sprintf("#!/bin/sh\nprintf invoked > %q\nprintf 'util-linux sg decoy' >&2\nexit 99\n", marker)
			if err := os.WriteFile(filepath.Join(bin, "sg"), []byte(decoy), 0o700); err != nil { //nolint:gosec // executable decoy in t.TempDir
				t.Fatal(err)
			}
			if present {
				binaries := map[string]string{toolAstGrep: canonical}
				if node != "" {
					binaries["node"] = node
				}
				for name, target := range binaries {
					if err := os.Symlink(target, filepath.Join(bin, name)); err != nil {
						t.Fatal(err)
					}
				}
			}
			t.Setenv("PATH", bin)
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "fixture.js"), []byte("export function canonicalGreet() {\n    return 1;\n}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			a := astgrep.New(toolrun.New())
			facts, syntaxCov, err := a.Syntax(t.Context(), scope.Scope{Root: root}, []string{langTypeScriptStr})
			if err != nil {
				t.Fatal(err)
			}
			matches, patternCov, err := a.Find(context.Background(), scope.Scope{Root: root}, pattern.Config{{ID: "named-js", Lang: "javascript", Rule: "function $NAME($$$) { $$$ }"}})
			if err != nil {
				t.Fatal(err)
			}
			if present {
				if syntaxCov.Status != "ok" || patternCov.Status != "ok" || len(facts) != 1 || len(matches) != 1 {
					t.Errorf("canonical binary: facts=%+v matches=%+v syntax=%+v patterns=%+v", facts, matches, syntaxCov, patternCov)
				}
				if !strings.HasPrefix(syntaxCov.Version, "ast-grep ") || syntaxCov.Version != patternCov.Version {
					t.Errorf("unexpected producer versions: syntax=%q patterns=%q", syntaxCov.Version, patternCov.Version)
				}
			} else if syntaxCov.Status != statusAbsentStr || patternCov.Status != statusAbsentStr || len(facts) != 0 || len(matches) != 0 {
				t.Errorf("sg alone is not ast-grep: syntax=%+v patterns=%+v", syntaxCov, patternCov)
			}
			if syntaxCov.Tool != toolAstGrepSyntax || patternCov.Tool != toolAstGrep {
				t.Errorf("public coverage identity changed: syntax=%+v patterns=%+v", syntaxCov, patternCov)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("decoy sg was invoked: %v", err)
			}
		})
	}
}

func TestSyntaxIntegration_JavaScriptRouteQuotes(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration test — skipped with -short")
	}
	sgSkipGuard(t)
	for _, tc := range []struct {
		name, literal, want string
	}{
		{"double", `"/users"`, pathUsers},
		{"single-quoted", `'/users'`, pathUsers},
		{"backtick", "`/users`", pathUsers},
		{"embedded-apostrophe", `"/users/o'neill"`, "/users/o'neill"},
		{"boundary-apostrophes", `"'/users'"`, "'/users'"},
		{"boundary-double-quotes", `'"/users"'`, `"/users"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			source := "import express from 'express';\nconst app = express();\napp.get(" + tc.literal + ", handler);\n"
			if err := os.WriteFile(filepath.Join(root, "route.js"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			facts, cov, err := astgrep.New(toolrun.New()).Syntax(t.Context(), scope.Scope{Root: root}, []string{langTypeScriptStr})
			if err != nil || cov.Status != "ok" || len(facts) != 1 {
				t.Fatalf("route quote: facts=%+v coverage=%+v err=%v", facts, cov, err)
			}
			f := facts[0]
			if f.Name != tc.want || f.Kind != kindRouteStr || f.Exported || !f.FrameworkConfirmed || f.StartLine != 3 || f.EndLine != 3 {
				t.Errorf("got %+v, want route name=%q at line 3", f, tc.want)
			}
		})
	}
}

func TestSyntaxIntegration_JavaScriptPrivateAndComputedMethods(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess integration test — skipped with -short")
	}
	sgSkipGuard(t)
	root := t.TempDir()
	source := `export class PublicClass {
    #privateMethod() {}
    [dynamicName]() {}
    "stringMethod"() {}
}
`
	if err := os.WriteFile(filepath.Join(root, "methods.js"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	facts, cov, err := astgrep.New(toolrun.New()).Syntax(t.Context(), scope.Scope{Root: root}, []string{langTypeScriptStr})
	if err != nil || cov.Status != "ok" || len(facts) != 1 || facts[0].Kind != kindClassStr || facts[0].Name != "PublicClass" {
		t.Fatalf("private/computed methods are not named public declarations: facts=%+v coverage=%+v err=%v", facts, cov, err)
	}
}
