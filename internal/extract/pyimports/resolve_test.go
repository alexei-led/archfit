package pyimports

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"testing/fstest"
)

// Names the tests share.
const (
	modApp     = "app"
	modA       = "app.a"
	modSub     = "app.sub"
	modSubB    = "app.sub.b"
	modSubC    = "app.sub.c"
	modGone    = "app.gone"
	fileSubB   = "src/app/sub/b.py"
	fileSubC   = "src/app/sub/c.py"
	fileInnerM = "src/app/ns/inner/m.py"
	flatInit   = "app/__init__.py"
	pkgLib     = "lib"
)

// tree builds an in-memory file tree from file names and contents.
func tree(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

// names lists imported names, all on line 1.
func names(texts ...string) []name {
	out := make([]name, 0, len(texts))
	for _, t := range texts {
		out = append(out, name{t, 1})
	}
	return out
}

// TestResolve ports test_rules of scripts/eval/extractors/py_imports_test.py.
func TestResolve(t *testing.T) {
	t.Parallel()
	modules := map[string]string{modApp: "a", modA: "a", modSub: "a", modSubB: "a", modSubC: "a"}
	tests := []struct {
		name      string
		importer  string
		isPackage bool
		imp       pyImport
		want      []string
	}{
		{"absolute import", modA, false, pyImport{module: modSubB}, []string{modSubB}},
		{"from module import name", modA, false, pyImport{module: modSubB, names: names("f"), isFrom: true}, []string{modSubB}},
		{"from package import submodule", modA, false, pyImport{module: modSub, names: names("c"), isFrom: true}, []string{modSubC}},
		{"from package import name and submodule", modA, false, pyImport{module: modSub, names: names("c", "f"), isFrom: true}, []string{modSubC, modSub}},
		{"relative depth 1 in a module", modSubB, false, pyImport{module: ".", names: names("c"), isFrom: true}, []string{modSubC}},
		{"relative depth 1 in a package", modSub, true, pyImport{module: ".", names: names("b"), isFrom: true}, []string{modSubB}},
		{"relative depth 2", modSubB, false, pyImport{module: "..a", names: names("x"), isFrom: true}, []string{modA}},
		{"wildcard", modA, false, pyImport{module: modSub, isFrom: true}, []string{modSub}},
		{"missing module stays named", modA, false, pyImport{module: modGone, names: names("x"), isFrom: true}, []string{modGone}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolve(tt.importer, tt.isPackage, tt.imp, modules)
			if err != nil {
				t.Fatal(err)
			}
			mods := make([]string, 0, len(got))
			for _, g := range got {
				mods = append(mods, g.module)
			}
			if !slices.Equal(mods, tt.want) {
				t.Errorf("resolve = %q, want %q", mods, tt.want)
			}
		})
	}
}

func TestResolveRelativeAboveThePackage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, importer string
		isPackage      bool
		module         string
	}{
		{"three levels from the top package", modApp, true, "...x"},
		{"two levels from a top-level module", modA, false, "..."},
		{"a module with no package", "solo", false, ".x"},
	}
	for _, tt := range tests {
		_, err := resolve(tt.importer, tt.isPackage, pyImport{module: tt.module, isFrom: true}, nil)
		if !errors.Is(err, errAboveTopLevel) {
			t.Errorf("%s: err = %v, want errAboveTopLevel", tt.name, err)
		}
	}
}

func TestResolveKeepsTheLineOfEachName(t *testing.T) {
	t.Parallel()
	imp := pyImport{module: modSub, isFrom: true, line: 4, names: []name{{"b", 4}, {"c", 5}}}
	got, err := resolve(modA, false, imp, map[string]string{modSubC: "f"})
	want := []target{{modSub, 4}, {modSubC, 5}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("resolve = %v, %v, want %v", got, err, want)
	}
}

func TestLayout(t *testing.T) {
	t.Parallel()

	t.Run("src layout wins over top-level packages", func(t *testing.T) {
		t.Parallel()
		fsys := tree(map[string]string{"src/app/__init__.py": "", "tests/__init__.py": ""})
		if got, err := discoverPackages(fsys); err != nil || !slices.Equal(got, []string{modApp}) {
			t.Errorf("discoverPackages = %q, %v, want [app]", got, err)
		}
		if got := packageDir(fsys, modApp); got != "src/app" {
			t.Errorf("packageDir = %q, want src/app", got)
		}
	})

	t.Run("flat layout", func(t *testing.T) {
		t.Parallel()
		fsys := tree(map[string]string{flatInit: "", "docs/x.py": ""})
		if got, err := discoverPackages(fsys); err != nil || !slices.Equal(got, []string{modApp}) {
			t.Errorf("discoverPackages = %q, %v, want [app]", got, err)
		}
		if got := packageDir(fsys, modApp); got != modApp {
			t.Errorf("packageDir = %q, want app", got)
		}
	})

	t.Run("namespace directory inside a package holds modules", func(t *testing.T) {
		t.Parallel()
		fsys := tree(map[string]string{
			flatInit:                   "",
			"app/ns/inner/__init__.py": "",
			"app/ns/inner/m.py":        "",
			"app/not-id/x.py":          "",
			"app/.hidden.py":           "",
			"app/a.b.py":               "",
			"app/notes.txt":            "",
		})
		got, err := sourceFiles(fsys, modApp)
		want := map[string]string{
			modApp:           flatInit,
			"app.ns.inner":   "app/ns/inner/__init__.py",
			"app.ns.inner.m": "app/ns/inner/m.py",
		}
		if err != nil || !maps.Equal(got, want) {
			t.Errorf("sourceFiles = %v, %v, want %v", got, err, want)
		}
	})
}

func TestPackages(t *testing.T) {
	t.Parallel()
	fsys := tree(map[string]string{"src/app/__init__.py": "", "src/lib/__init__.py": "", "src/other/x.py": ""})
	tests := []struct {
		name, configured string
		read, discovered []string
		wantErr          bool
	}{
		{"discovered", "", []string{modApp, pkgLib}, []string{modApp, pkgLib}, false},
		{"configured", modApp, []string{modApp}, []string{modApp, pkgLib}, false},
		{"configured namespace package", "other", []string{"other"}, []string{modApp, pkgLib}, false},
		{"configured without a directory", "gone", nil, nil, true},
	}
	for _, tt := range tests {
		read, discovered, err := packages(fsys, tt.configured)
		if (err != nil) != tt.wantErr || !slices.Equal(read, tt.read) || !slices.Equal(discovered, tt.discovered) {
			t.Errorf("%s: packages = %q, %q, %v; want %q, %q, error %v", tt.name, read, discovered, err, tt.read, tt.discovered, tt.wantErr)
		}
	}
	t.Run("no package and no root-name fallback", func(t *testing.T) {
		read, discovered, err := packages(tree(map[string]string{"scripts/x.py": "", "x.py": ""}), "")
		if err != nil || read != nil || discovered != nil {
			t.Errorf("packages = %q, %q, %v, want none", read, discovered, err)
		}
	})
}

func TestExcludedSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		file string
		want bool
	}{
		{"src/app/legacy/__init__.py", true},
		{"src/app/legacy/deep/m.py", true},
		{"src/app/legacy.py", false},
		{"src/app/a.py", false},
	}
	for _, tt := range tests {
		if got := excludedSource([]string{"src/app/legacy"}, tt.file); got != tt.want {
			t.Errorf("excludedSource(%s) = %v, want %v", tt.file, got, tt.want)
		}
	}
}

func TestIsIdentifier(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		modApp: true, "_private": true, "v2": true, "__pycache__": true, "café": true,
		"": false, "2fa": false, "not-id": false, ".git": false, "a.b": false,
	}
	for s, want := range tests {
		if got := isIdentifier(s); got != want {
			t.Errorf("isIdentifier(%q) = %v, want %v", s, got, want)
		}
	}
}
