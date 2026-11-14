package main

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestIsVersion3(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		data string
		want bool
	}{
		{"version: 3\nowners: [a]\n", true},
		{"version: 3.0\n", true},
		{"version: 3\nnot: [valid\n", false},
		{"version: 3\nmodules: {}\n", true},
		{"version: 2\n", false},
		{version1, false},
		{"version: 4\n", false},
		{"version: \"3\"\n", false},
		{"owners: [a]\n", false},
		{"", false},
	} {
		if got := isVersion3([]byte(tc.data)); got != tc.want {
			t.Errorf("isVersion3(%q) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

func TestPolicyArg(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "repo")
	co := checkout{root: root, start: filepath.Join(root, "services", "api")}
	outside := filepath.Join(filepath.Dir(root), "candidate.yaml")
	for _, tc := range []struct{ arg, want string }{
		{defaultConfigPath, apiPolicy},
		{"./.archfit.yaml", apiPolicy},
		{"../../.archfit.yaml", defaultConfigPath},
		{filepath.Join(root, "x.yaml"), "x.yaml"},
		{outside, outside},
		{"../../../candidate.yaml", outside},
	} {
		if got := co.policyArg(tc.arg); got != tc.want {
			t.Errorf("policyArg(%q) = %q, want %q", tc.arg, got, tc.want)
		}
	}
}

// TestProtectionNeedsAStepThatUsesTheAction (D6): only a uses: key with the
// Action at a ref protects the policy.
func TestProtectionNeedsAStepThatUsesTheAction(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, workflow string
		want           bool
	}{
		{"a step", "steps:\n  - uses: alexei-led/archfit-action@v4\n", true},
		{"quoted", "steps:\n  - name: check\n    uses: \"alexei-led/archfit-action@v4\"\n", true},
		{"commented out", "steps:\n  # - uses: alexei-led/archfit-action@v4\n", false},
		{"a mention", "steps:\n  - run: echo alexei-led/archfit-action@v4\n", false},
		{"another Action", "steps:\n  - uses: alexei-led/archfit-action-foo@v1\n", false},
		{"no workflow", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if tc.workflow != "" {
				writeFiles(t, root, map[string]string{".github/workflows/archfit.yml": tc.workflow, ".github/workflows/notes.txt": protectingWorkflow})
			}
			got, err := protection{checkout{root: root}}.Protected(context.Background())
			if err != nil || got != tc.want {
				t.Errorf("Protected = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

// TestGoModulesPreferTheModuleCheckReads: a fixture go.mod under testdata, or
// a deeper copy of the module, does not take the import path from the root.
func TestGoModulesPreferTheModuleCheckReads(t *testing.T) {
	t.Parallel()
	dir := fixtureRepo(t, with(fixtureFiles(), map[string]string{
		"testdata/fixture/go.mod":        "module example.com/pumba\n",
		"tools/copy/go.mod":              "module example.com/pumba\n",
		"tools/gen/go.mod":               "module example.com/gen\n",
		"pkg/util/_old/go.mod":           "module example.com/old\n",
		"testdata/fixture/pkg/util/a.go": "package util\n",
	}), nil)
	g := &goModules{co: checkout{root: dir, runner: toolrun.New()}}
	for _, tc := range []struct {
		importPath, want string
		ok               bool
	}{
		{"example.com/pumba/pkg/util", pkgUtil, true},
		{"example.com/gen/x", "tools/gen/x", true},
		{"example.com/old", "", false},
	} {
		got, ok, err := g.Dir(context.Background(), tc.importPath)
		if err != nil || got != tc.want || ok != tc.ok {
			t.Errorf("Dir(%s) = %q, %v, %v; want %q, %v", tc.importPath, got, ok, err, tc.want, tc.ok)
		}
	}
}

// TestDirFilesNeedCompleteSources: a missing tool or base leaves a directory's
// files unknown, so `can` gives no verdict instead of judging it empty.
func TestDirFilesNeedCompleteSources(t *testing.T) {
	t.Parallel()
	const srcA = "src/a.ts"
	sources := []string{srcA, "src/b.ts", "src/sub/c.ts", "dest/d.ts"}
	files, err := dirFiles(evidence.CheckFacts{Sources: sources, Unknown: []evidence.UnknownItem{
		{File: srcA, Cause: evidence.CauseImport, Reason: "cannot resolve ./x"},
	}}, "src")
	if err != nil || !slices.Equal(files, []string{srcA, "src/b.ts"}) {
		t.Errorf("with an unresolved import: %q, %v; want the two files of src", files, err)
	}
	if _, err := dirFiles(evidence.CheckFacts{Sources: sources, Unknown: []evidence.UnknownItem{
		{File: "src/cgo.go", Cause: evidence.CauseImport, Reason: "cgo file", Fix: "set CGO_ENABLED"},
	}}, "src"); err == nil {
		t.Error("a file of src left out of the sources: no error")
	}
	if files, err := dirFiles(evidence.CheckFacts{Sources: sources, Unknown: []evidence.UnknownItem{
		{File: "other/cgo.go", Cause: evidence.CauseImport, Reason: "cgo file"},
	}}, "src"); err != nil || len(files) != 2 {
		t.Errorf("a file of another directory left out: %q, %v; want the two files of src", files, err)
	}
	for _, cause := range []evidence.UnknownCause{evidence.CauseTool, evidence.CauseBase} {
		_, err := dirFiles(evidence.CheckFacts{Sources: sources, Unknown: []evidence.UnknownItem{
			{Cause: cause, Reason: "ast-grep is not installed", Fix: "install ast-grep"},
		}}, "src")
		if err == nil || !strings.Contains(err.Error(), "the files of src are not all known: ast-grep is not installed: install ast-grep") {
			t.Errorf("cause %s: err = %v, want the files of src not known", cause, err)
		}
	}
}
