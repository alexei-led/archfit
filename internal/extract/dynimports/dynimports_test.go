package dynimports_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/dynimports"
	"github.com/alexei-led/archfit/internal/model/diagnostic"
)

// writeFile writes content to root/rel, creating parent dirs.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// langPy is the python language tag, factored out to satisfy goconst.
const langPy = "python"

func TestDetect_Python(t *testing.T) {
	const pyFile = "pkg/mod.py"
	root := t.TempDir()
	// Top-level imports must NOT be flagged; in-function imports + importlib must be.
	writeFile(t, root, pyFile, `import os
from typing import List

def loader():
    import json          # in-function: lazy_import
    from .other import X  # in-function: lazy_import
    return json, X

class C:
    import collections    # class body, not in a function: NOT flagged

    def m(self):
        importlib.import_module("plugins.a")  # importlib anywhere
        return __import__("dynmod")           # __import__ anywhere
`)

	sites := dynimports.Detect(root)

	want := []diagnostic.DynamicImportSite{
		{File: pyFile, Line: 5, Kind: "lazy_import", Language: langPy},
		{File: pyFile, Line: 6, Kind: "lazy_import", Language: langPy},
		{File: pyFile, Line: 13, Kind: "importlib", Language: langPy},
		{File: pyFile, Line: 14, Kind: "importlib", Language: langPy},
	}
	if !reflect.DeepEqual(sites, want) {
		t.Errorf("python sites mismatch:\n got = %#v\nwant = %#v", sites, want)
	}
}

func TestDetect_TypeScript(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/a.ts", `import { Foo } from "./foo";       // static: NOT flagged
import type { Bar } from "./bar";  // type-only static: NOT flagged
const m = require("./legacy");     // require
async function load() {
  const x = await import("./lazy"); // dynamic import
  return x;
}
// require("commented") should be ignored
`)

	sites := dynimports.Detect(root)

	want := []diagnostic.DynamicImportSite{
		{File: "src/a.ts", Line: 3, Kind: "require", Language: "typescript"},
		{File: "src/a.ts", Line: 5, Kind: "dynamic_import", Language: "typescript"},
	}
	if !reflect.DeepEqual(sites, want) {
		t.Errorf("ts sites mismatch:\n got = %#v\nwant = %#v", sites, want)
	}
}

func TestDetect_NoDynamicImports(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clean.py", "import os\nfrom sys import argv\n")
	writeFile(t, root, "clean.ts", "import { A } from './a';\nexport const x = 1;\n")

	if sites := dynimports.Detect(root); len(sites) != 0 {
		t.Errorf("expected no sites, got %#v", sites)
	}
}

func TestDetect_SkipsVendorAndNodeModules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "node_modules/dep/index.js", `const x = require("x");`)
	writeFile(t, root, "vendor/lib.py", "def f():\n    import json\n")
	writeFile(t, root, ".git/hooks/h.ts", `const y = require("y");`)
	writeFile(t, root, "app.py", "def f():\n    import json\n")

	sites := dynimports.Detect(root)
	if len(sites) != 1 || sites[0].File != "app.py" {
		t.Errorf("expected only app.py site, got %#v", sites)
	}
}

func TestDetect_Deterministic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "b/two.py", "def g():\n    import json\n")
	writeFile(t, root, "a/one.ts", `const m = require("./m");`)
	writeFile(t, root, "a/two.py", "def f():\n    from x import y\n")

	first := dynimports.Detect(root)
	second := dynimports.Detect(root)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("non-deterministic: %#v vs %#v", first, second)
	}
	// Sorted by (file, line, kind): a/one.ts, a/two.py, b/two.py.
	wantFiles := []string{"a/one.ts", "a/two.py", "b/two.py"}
	for i, f := range wantFiles {
		if first[i].File != f {
			t.Errorf("site[%d].File = %q, want %q (sites=%#v)", i, first[i].File, f, first)
		}
	}
}
