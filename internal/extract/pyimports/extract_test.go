package pyimports

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	srcInit = "src/app/__init__.py"
	srcA    = "src/app/a.py"
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

func TestExtractFailures(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{srcInit: "", srcA: ""})

	t.Run("ast-grep absent", func(t *testing.T) {
		t.Parallel()
		runner := &toolrun.RunnerMock{
			DetectFunc: func(context.Context, string) (toolrun.ToolInfo, bool) { return toolrun.ToolInfo{}, false },
		}
		if _, err := Extract(context.Background(), runner, root, Config{}); !errors.Is(err, ErrNoAstGrep) {
			t.Errorf("err = %v, want ErrNoAstGrep", err)
		}
	})

	t.Run("no package needs no ast-grep", func(t *testing.T) {
		t.Parallel()
		runner := &toolrun.RunnerMock{} // any call panics
		got, err := Extract(context.Background(), runner, writeTree(t, map[string]string{"scripts/x.py": "import os\n", "x.py": ""}), Config{})
		if err != nil || len(got.Sources) != 0 || len(got.Imports) != 0 {
			t.Errorf("Extract = %+v, %v, want empty facts", got, err)
		}
	})

	t.Run("every module excluded needs no ast-grep", func(t *testing.T) {
		t.Parallel()
		got, err := Extract(context.Background(), &toolrun.RunnerMock{}, root, Config{Exclude: []string{"src/**"}})
		if err != nil || len(got.Sources) != 0 {
			t.Errorf("Extract = %+v, %v, want empty facts", got, err)
		}
	})

	t.Run("configured package without a directory", func(t *testing.T) {
		t.Parallel()
		_, err := Extract(context.Background(), &toolrun.RunnerMock{}, root, Config{Package: "gone"})
		if err == nil || !strings.Contains(err.Error(), `package "gone"`) {
			t.Errorf("err = %v, want the missing package named", err)
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
			return toolrun.Output{ExitCode: -1, Stdout: []byte(`{"text":"os","file":"src/app/a.py","ruleId":"name"}` + "\n")}, nil
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

	t.Run("a from-import without a module is an error", func(t *testing.T) {
		t.Parallel()
		runner, _ := fakeAstGrep(func([]string) (toolrun.Output, error) {
			out := `{"text":"from","file":"src/app/a.py","ruleId":"from-stmt","range":{"byteOffset":{"start":0,"end":4},"start":{"line":2}}}`
			return toolrun.Output{Stdout: []byte(out + "\n")}, nil
		})
		_, err := Extract(context.Background(), runner, root, Config{})
		if err == nil || !strings.Contains(err.Error(), "src/app/a.py: from-import on line 3 names no module") {
			t.Errorf("err = %v, want the file and line named", err)
		}
	})

	t.Run("missing root is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := Extract(context.Background(), &toolrun.RunnerMock{}, filepath.Join(root, "gone"), Config{}); err == nil {
			t.Error("err = nil, want a read error")
		}
	})
}

func TestExtractBatches(t *testing.T) {
	t.Parallel()
	files := map[string]string{srcInit: ""}
	for i := range batchSize {
		files[fmt.Sprintf("src/app/m%03d.py", i)] = ""
	}
	runner, calls := fakeAstGrep(noMatches)
	got, err := Extract(context.Background(), runner, writeTree(t, files), Config{})
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || len((*calls)[0]) != batchSize || len((*calls)[1]) != 1 {
		t.Errorf("calls with %d paths, want 2 calls with %d and 1", len(*calls), batchSize)
	}
	if len(got.Sources) != batchSize+1 {
		t.Errorf("sources = %d, want %d", len(got.Sources), batchSize+1)
	}
}

// TestExtractWithAstGrep runs the real ast-grep on the prototype's ExtractTest
// fixture (scripts/eval/extractors/py_imports_test.py).
func TestExtractWithAstGrep(t *testing.T) {
	t.Parallel()
	runner := toolrun.New()
	if _, ok := runner.Detect(context.Background(), astGrep); !ok {
		t.Skip("ast-grep is not installed: install it to read Python imports with the real parser")
	}
	root := writeTree(t, map[string]string{
		"pyproject.toml": "",
		srcInit:          "from . import a\n",
		srcA: "import os\n" +
			"import requests\n" +
			"from typing import TYPE_CHECKING\n" +
			"from app.sub import (b,  # comment\n    c)\n" +
			"from .gone import x\n" +
			"if TYPE_CHECKING:\n" +
			"    from app import typed_only\n" +
			"elif typing.TYPE_CHECKING:\n" +
			"    import app.typed_too\n" +
			"else:\n" +
			"    import app.sub.c\n" +
			"def f():\n" +
			"    from app.ns.inner import m\n" +
			"from ... import above\n",
		"src/app/sub/__init__.py":      "",
		fileSubB:                       "from ..a import f\n",
		fileSubC:                       "import app.legacy\n",
		"src/app/ns/inner/__init__.py": "",
		fileInnerM:                     "",
		legacyInit:                     "import app.a\n",
	})
	got, err := Extract(context.Background(), runner, root, Config{Exclude: []string{"src/app/legacy/**"}})
	if err != nil {
		t.Fatal(err)
	}
	wantSources := []string{srcInit, srcA, "src/app/ns/inner/__init__.py", fileInnerM,
		"src/app/sub/__init__.py", fileSubB, fileSubC}
	if !slices.Equal(got.Sources, wantSources) {
		t.Errorf("sources = %q, want %q (an excluded file is no source)", got.Sources, wantSources)
	}
	want := []Import{
		{File: srcInit, Line: 1, Module: modA, Kind: KindFile, Target: srcA},
		{File: srcA, Line: 1, Module: "os", Kind: KindExternal},
		{File: srcA, Line: 2, Module: "requests", Kind: KindExternal},
		{File: srcA, Line: 3, Module: "typing", Kind: KindExternal},
		{File: srcA, Line: 4, Module: modSubB, Kind: KindFile, Target: fileSubB},
		{File: srcA, Line: 5, Module: modSubC, Kind: KindFile, Target: fileSubC},
		{File: srcA, Line: 6, Module: modGone, Kind: KindUnknown},
		{File: srcA, Line: 12, Module: modSubC, Kind: KindFile, Target: fileSubC},
		{File: srcA, Line: 14, Module: "app.ns.inner.m", Kind: KindFile, Target: fileInnerM},
		{File: srcA, Line: 15, Module: "...", Kind: KindUnknown},
		{File: fileSubB, Line: 1, Module: modA, Kind: KindFile, Target: srcA},
		{File: fileSubC, Line: 1, Module: "app.legacy", Kind: KindFile, Target: legacyInit},
	}
	if !slices.Equal(got.Imports, want) {
		t.Errorf("imports =\n%+v\nwant\n%+v", got.Imports, want)
	}
}
