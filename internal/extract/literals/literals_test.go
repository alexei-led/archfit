package literals

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

// valuesOf returns "file:line value" for each literal, in order.
func valuesOf(lits []Literal) []string {
	out := make([]string, 0, len(lits))
	for _, l := range lits {
		out = append(out, fmt.Sprintf("%s:%d %s", l.File, l.Line, l.Value))
	}
	return out
}

func TestShape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value string
		want  Shape
	}{
		{`json:"name" yaml:"name"`, ShapeTag},
		{`json:"to"`, ShapeTag},
		{"container_id", ShapeWord},
		{"ast-grep.syntax:v2@x", ShapeWord},
		{"schlüsselwörter", ShapeWord}, // Python's \w is Unicode
		{"https://example.com/x", ShapeOther},
		{"^[a-z]+$", ShapeOther},
		{"bad network interface name", ShapeOther},
		{"/var/run/docker.sock", ShapeOther},
		{"%s=%d", ShapeOther},
		{"json:name", ShapeWord},
		{"", ShapeOther},
	}
	for _, tt := range tests {
		if got := (Literal{Value: tt.value}).Shape(); got != tt.want {
			t.Errorf("Shape(%q) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

func TestLengthCountsRunes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value string
		want  int
	}{
		{"", 0},
		{"abc", 3},
		{"été", 3},
		{"\\u274c", 6}, // escapes are as written
	}
	for _, tt := range tests {
		if got := (Literal{Value: tt.value}).Length(); got != tt.want {
			t.Errorf("Length(%q) = %d, want %d", tt.value, got, tt.want)
		}
	}
}

func TestBuildIndex(t *testing.T) {
	t.Parallel()
	const fileA = "a.go"
	const long = "twelve runes"    // 12
	const accented = "ééééééééééé" // 11 runes, 22 bytes
	lits := []Literal{
		{File: fileA, Line: 3, Value: long},
		{File: fileA, Line: 3, Value: long},
		{File: "b.go", Line: 1, Value: "eleven rune"},
		{File: "b.go", Line: 2, Value: accented},
		{File: "c.py", Line: 9, Value: long},
	}
	want := Index{long: {lits[0], lits[1], lits[4]}}
	if got := BuildIndex(lits); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("BuildIndex = %v, want %v", got, want)
	}
	if got := BuildIndex(nil); len(got) != 0 {
		t.Errorf("BuildIndex(nil) = %v, want empty", got)
	}
}

func TestExtractFailures(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{"src/x.ts": ""})

	t.Run("ast-grep absent", func(t *testing.T) {
		t.Parallel()
		runner := &toolrun.RunnerMock{
			DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false },
		}
		if _, err := Extract(context.Background(), runner, root, Config{}); !errors.Is(err, ErrNoAstGrep) {
			t.Errorf("err = %v, want ErrNoAstGrep", err)
		}
	})

	t.Run("Go and skipped files need no ast-grep", func(t *testing.T) {
		t.Parallel()
		runner := &toolrun.RunnerMock{} // any call panics
		tree := writeTree(t, map[string]string{
			"main.go":             "package main\n\nconst s = \"a go literal\"\n",
			"node_modules/x/i.js": "const a = 'x';\n",
			"vendor/v/v.py":       "a = 'x'\n",
			"gen/g.rs":            "fn f() {}\n",
		})
		got, err := Extract(context.Background(), runner, tree, Config{Exclude: []string{"gen"}})
		if err != nil || !slices.Equal(valuesOf(got), []string{"main.go:3 a go literal"}) {
			t.Errorf("Extract = %q, %v, want the Go literal only", valuesOf(got), err)
		}
	})

	t.Run("failed run names the exit and stderr", func(t *testing.T) {
		t.Parallel()
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: 2, Stderr: []byte("bad rule\n")}, nil
		})
		_, err := Extract(context.Background(), runner, root, Config{})
		if err == nil || !strings.Contains(err.Error(), "exited 2: bad rule") {
			t.Errorf("err = %v, want the exit code and stderr", err)
		}
	})

	t.Run("a run a signal ended is an error, whatever it printed", func(t *testing.T) {
		t.Parallel()
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) {
			return toolrun.Output{ExitCode: -1, Stdout: []byte(`{"text":"'abc'","file":"src/x.ts","ruleId":"js-quoted"}` + "\n")}, nil
		})
		if _, err := Extract(context.Background(), runner, root, Config{}); err == nil || !strings.Contains(err.Error(), "exited -1") {
			t.Errorf("err = %v, want exited -1", err)
		}
	})

	t.Run("runner error is passed on", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) { return toolrun.Output{}, boom })
		if _, err := Extract(context.Background(), runner, root, Config{}); !errors.Is(err, boom) {
			t.Errorf("err = %v, want boom", err)
		}
	})

	t.Run("bad output is an error", func(t *testing.T) {
		t.Parallel()
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte("{not json\n")}, nil
		})
		if _, err := Extract(context.Background(), runner, root, Config{}); err == nil {
			t.Error("err = nil, want a read error")
		}
	})

	t.Run("a match the rule cannot read is an error", func(t *testing.T) {
		t.Parallel()
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) {
			return toolrun.Output{Stdout: []byte(`{"text":"abc","file":"src/x.ts","ruleId":"js-quoted"}` + "\n")}, nil
		})
		_, err := Extract(context.Background(), runner, root, Config{})
		if err == nil || !strings.Contains(err.Error(), "src/x.ts:1") {
			t.Errorf("err = %v, want an error naming src/x.ts:1", err)
		}
	})

	t.Run("a Go syntax error names the file", func(t *testing.T) {
		t.Parallel()
		tree := writeTree(t, map[string]string{"pkg/bad.go": "package bad\n\nfunc {\n"})
		_, err := Extract(context.Background(), &toolrun.RunnerMock{}, tree, Config{})
		if err == nil || !strings.Contains(err.Error(), "pkg/bad.go") {
			t.Errorf("err = %v, want a parse error naming pkg/bad.go", err)
		}
	})

	t.Run("a bad exclude glob is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := Extract(context.Background(), &toolrun.RunnerMock{}, root, Config{Exclude: []string{"src/["}}); err == nil {
			t.Error("err = nil, want a bad glob error")
		}
	})

	t.Run("missing root is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := Extract(context.Background(), &toolrun.RunnerMock{}, filepath.Join(root, "gone"), Config{}); err == nil {
			t.Error("err = nil, want a walk error")
		}
	})
}

func TestExtractReadsOnlyTheTree(t *testing.T) {
	t.Parallel()
	outside := writeTree(t, map[string]string{"secret.go": "package secret\n\nconst s = \"outside the tree\"\n"})
	root := writeTree(t, map[string]string{
		"a.go":            "package a\n\nconst s = \"inside the tree\"\n",
		"skip/x/b.go":     "package b\n\nconst s = \"excluded by glob\"\n",
		"keep/c_test.go":  "package c\n\nconst s = \"tests are read\"\n",
		"testdata/d.go":   "package d\n\nconst s = \"testdata is read\"\n",
		".git/hooks/e.go": "package e\n\nconst s = \"git dir\"\n",
	})
	if err := os.Symlink(filepath.Join(outside, "secret.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	got, err := Extract(context.Background(), &toolrun.RunnerMock{}, root, Config{Exclude: []string{"skip/**"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.go:3 inside the tree", "keep/c_test.go:3 tests are read", "testdata/d.go:3 testdata is read"}
	if !slices.Equal(valuesOf(got), want) {
		t.Errorf("Extract = %q, want %q", valuesOf(got), want)
	}
}

func TestExtractBatches(t *testing.T) {
	t.Parallel()
	files := map[string]string{}
	for i := range batchSize + 1 {
		files[fmt.Sprintf("src/f%03d.py", i)] = ""
	}
	runner, calls := fakeAstGrep(noMatches)
	if _, err := Extract(context.Background(), runner, writeTree(t, files), Config{}); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || len((*calls)[0]) != batchSize || len((*calls)[1]) != 1 {
		t.Errorf("%d calls, want 2 calls with %d and 1 paths", len(*calls), batchSize)
	}
}

func TestValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rule, text, want string
		ok, err          bool
	}{
		{ruleJSQuoted, `"a\"b"`, `a\"b`, true, false},
		{ruleJSQuoted, `'q'`, "q", true, false},
		{ruleJSQuoted, "`plain`", "plain", true, false},
		{ruleJSQuoted, `""`, "", true, false},
		{ruleJSQuoted, `"open`, "", false, true},
		{rulePyString, `r'\d+'`, `\d+`, true, false},
		{rulePyString, `u"w"`, "w", true, false},
		{rulePyString, `"""one line"""`, "one line", true, false},
		{rulePyString, `''''''`, "", true, false},
		{rulePyString, `''`, "", true, false},
		{rulePyString, `'it\'s'`, `it\'s`, true, false},
		{rulePyString, `f"{y}"`, "", false, false},
		{rulePyString, `Rb'z'`, "", false, false},
		{rulePyString, `t"{y}"`, "", false, false},
		{rulePyString, `"""open"`, "", false, true},
		{rulePyString, `abc`, "", false, true},
		{ruleRsString, `"text"`, "text", true, false},
		{ruleRsString, `r"\d"`, `\d`, true, false},
		{ruleRsString, `r#"a"b"#`, `a"b`, true, false},
		{ruleRsString, `r##"a"#b"##`, `a"#b`, true, false},
		{ruleRsString, `b"bytes"`, "", false, false},
		{ruleRsString, `br"bytes"`, "", false, false},
		{ruleRsString, `c"cstr"`, "", false, false},
		{ruleRsString, `cr#"cstr"#`, "", false, false},
		{ruleRsString, `r#"open"`, "", false, true},
		{ruleCSQuoted, `"cs"`, "cs", true, false},
		{ruleCSQuoted, `"utf8"u8`, "", false, false},
		{ruleCSVerbatim, `@"a ""b"""`, `a ""b""`, true, false},
		{ruleCSVerbatim, `"not verbatim"`, "", false, true},
		{ruleCSRaw, `"""raw "q" text"""`, `raw "q" text`, true, false},
		{ruleCSRaw, `""""four """ quotes""""`, `four """ quotes`, true, false},
		{ruleCSRaw, `""x""`, "", false, true},
		{ruleCSRaw, `"""open""`, "", false, true},
		{"other-rule", `"x"`, "", false, true},
	}
	for _, tt := range tests {
		got, ok, err := value(tt.rule, tt.text)
		if got != tt.want || ok != tt.ok || (err != nil) != tt.err {
			t.Errorf("value(%s, %s) = %q, %v, %v; want %q, %v, error %v", tt.rule, tt.text, got, ok, err, tt.want, tt.ok, tt.err)
		}
	}
}
