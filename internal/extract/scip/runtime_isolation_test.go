package scip

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	readerTimeoutCase = "timeout"
	readerTargetVenv  = "target venv"
	readerNoConfig    = "--no-config"
	readerPythonFlag  = "--python"
)

func provisionReaderPython(t *testing.T) string {
	t.Helper()
	if testing.Short() && os.Getenv("ARCHFIT_REQUIRE_TOOLS") != "1" {
		t.Skip("real Python reader skipped under -short")
	}
	if _, ok := toolrun.New().Detect(t.Context(), "uv"); !ok {
		if os.Getenv("ARCHFIT_REQUIRE_TOOLS") == "1" {
			t.Fatal("uv is missing and ARCHFIT_REQUIRE_TOOLS=1")
		}
		t.Skip("uv is not installed for test provisioning")
	}
	python := filepath.Join(t.TempDir(), "env", "bin", "python")
	for _, args := range [][]string{
		{"venv", readerNoConfig, readerPythonFlag, "3.14", filepath.Dir(filepath.Dir(python))},
		{"pip", "install", readerNoConfig, readerPythonFlag, python, "protobuf==7.36.2", "grpcio-tools==1.84.1"},
	} {
		out, err := toolrun.New().Run(t.Context(), toolrun.ToolCmd{Name: "uv", Args: args, WorkDir: t.TempDir()})
		if err != nil || out.ExitCode != 0 {
			t.Fatalf("provision real reader dependencies: %v %s", err, out.Stderr)
		}
	}
	return python
}

func readerIndexFixture(t *testing.T, python string) []byte {
	t.Helper()
	tmp := t.TempDir()
	proto := filepath.Join(tmp, "scip.proto")
	index := filepath.Join(tmp, "index.scip")
	if err := os.WriteFile(proto, scipProtoSrc, 0o600); err != nil {
		t.Fatal(err)
	}
	// Generate a real index using the exact installed protoc/protobuf pair.
	const generate = `
import importlib.util, pathlib, sys
from grpc_tools import protoc
proto, index = sys.argv[1:]
assert protoc.main(['protoc', '-I' + str(pathlib.Path(proto).parent), '--python_out=' + str(pathlib.Path(proto).parent), proto]) == 0
spec = importlib.util.spec_from_file_location('scip_pb2', str(pathlib.Path(proto).with_name('scip_pb2.py')))
pb = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pb)
idx = pb.Index()
sym = 'scip-python python app 0.0.0 ' + chr(96) + 'app.b' + chr(96) + '/Thing#'
a = idx.documents.add(relative_path='app/a.py')
a.occurrences.add(symbol=sym)
b = idx.documents.add(relative_path='app/b.py')
b.symbols.add(symbol=sym, kind=7)
b.occurrences.add(symbol=sym, symbol_roles=1)
pathlib.Path(index).write_bytes(idx.SerializeToString())
`
	realRunner := toolrun.New()
	out, err := realRunner.Run(t.Context(), toolrun.ToolCmd{Name: python, Args: []string{"-I", "-c", generate, proto, index}, WorkDir: tmp})
	if err != nil || out.ExitCode != 0 {
		t.Fatalf("real protobuf binding generation failed: %v %s", err, out.Stderr)
	}
	dir, err := os.OpenRoot(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	data, err := dir.ReadFile("index.scip")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSCIPProvisionedPythonOfflineReader(t *testing.T) {
	python := provisionReaderPython(t)
	data := readerIndexFixture(t, python)
	realRunner := toolrun.New()
	root := t.TempDir()
	marker := filepath.Join(root, "executed")
	poison := "import pathlib; pathlib.Path(" + strconv.Quote(marker) + ").write_text('executed')\n"
	for file, contents := range map[string]string{
		"pyproject.toml":  "[build-system]\nrequires=[]\nbuild-backend='evil'\nbackend-path=['.']\n[project]\nname='app'\nversion='0.0.0'\n",
		"app/__init__.py": poison, "app/a.py": "from app.b import Thing\n", "app/b.py": "class Thing: pass\n",
		"evil.py": poison, "setup.py": poison, "sitecustomize.py": poison,
		"grpc_tools/__init__.py": poison, "google/protobuf/__init__.py": poison,
		"uv.toml": "not valid config\n", ".python-version": "0.0.0\n",
		".venv/lib/python3.14/site-packages/target.pth": poison,
	} {
		path := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ARCHFIT_TOOL_PYTHON", python)
	t.Setenv("PYTHONPATH", root)
	t.Setenv("PYTHONHOME", root)
	t.Setenv("VIRTUAL_ENV", filepath.Join(root, ".venv"))
	t.Setenv("PYTHONUSERBASE", root)
	t.Setenv("UV_CONFIG_FILE", filepath.Join(root, "uv.toml"))
	t.Setenv("UV_CACHE_DIR", t.TempDir())
	t.Setenv("UV_OFFLINE", "1")
	t.Setenv("UV_PYTHON_DOWNLOADS", "never")
	t.Setenv("PATH", t.TempDir())
	readerCalls := 0
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: name}, name == indexerPython
		},
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == indexerPython {
				if slices.Contains(cmd.Args, "--version") {
					return toolrun.Output{Stdout: []byte("0.6.8")}, nil
				}
				for i, arg := range cmd.Args {
					if arg == flagOutput {
						output, err := os.OpenRoot(filepath.Dir(cmd.Args[i+1]))
						if err != nil {
							return toolrun.Output{}, err
						}
						defer func() { _ = output.Close() }()
						return toolrun.Output{}, output.WriteFile(filepath.Base(cmd.Args[i+1]), data, 0o600)
					}
				}
				t.Fatal("index output path missing")
			}
			if cmd.Name != python || len(cmd.Args) == 0 || cmd.Args[0] != "-I" || cmd.WorkDir == root || cmd.WorkDir == "" {
				t.Fatalf("reader not explicitly isolated: %+v", cmd)
			}
			if slices.Contains(cmd.Args, "--proto") {
				readerCalls++
			}
			return realRunner.Run(ctx, cmd)
		},
	}
	a := New(runner, 0)
	s := scope.Scope{Root: root}
	strengths, cov, err := a.Strengths(t.Context(), s)
	if err != nil || cov.Status != evidence.StatusOK || strengths["app.a\x00app.b"] != strengthModel {
		t.Fatalf("offline real reader lost semantic edge: %+v %+v %v", strengths, cov, err)
	}
	if symbols, cov, err := a.Symbols(t.Context(), s); err != nil || cov.Status != evidence.StatusOK || len(symbols.Module) == 0 {
		t.Fatalf("shared reader lost symbols: %+v %+v %v", symbols, cov, err)
	}
	if _, cov, err := a.DataStrengths(t.Context(), s); err != nil || cov.Status != evidence.StatusOK {
		t.Fatalf("shared reader lost data strengths: %+v %v", cov, err)
	}
	if _, cov, err := a.Connascence(t.Context(), s); err != nil || cov.Status != evidence.StatusOK {
		t.Fatalf("shared reader lost connascence: %+v %v", cov, err)
	}
	if readerCalls != 1 {
		t.Fatalf("reader calls=%d, want shared single pass", readerCalls)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("target startup/config/build code executed: %v", err)
	}
}

func TestSCIPInvalidProvisionedPythonDoesNotFallback(t *testing.T) {
	for _, value := range []string{"", "python", filepath.Join(t.TempDir(), "missing")} {
		t.Run(value, func(t *testing.T) {
			root := makeGoRoot(t)
			t.Setenv("ARCHFIT_TOOL_PYTHON", value)
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
					if name == "uv" {
						t.Fatal("invalid configured interpreter must not fall back to uv")
					}
					return toolrun.ToolInfo{Name: name}, name == indexerGo
				},
				RunFunc: func(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
					t.Fatal("invalid configured interpreter must not execute")
					return toolrun.Output{}, nil
				},
			}
			_, cov, err := New(runner, 0).Strengths(t.Context(), scope.Scope{Root: root})
			if err != nil || cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, "ARCHFIT_TOOL_PYTHON") {
				t.Fatalf("invalid runtime was hidden: %+v %v", cov, err)
			}
		})
	}
}

func TestSCIPUVReaderUsesOwnedIsolatedInvocation(t *testing.T) {
	root := makeGoRoot(t)
	indexRuns, readerRuns := 0, 0
	runner := scipCacheRunner(readerJSONSuccess, &indexRuns, &readerRuns)
	run := runner.RunFunc
	runner.RunFunc = func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
		if cmd.Name == "uv" {
			prefix := []string{"run", readerNoConfig, "--config-file", os.DevNull, "--no-env-file", "--no-project", "--isolated", readerPythonFlag, "3.14", "--with", "protobuf==7.36.2", "--with", "grpcio-tools==1.84.1", "--directory", cmd.WorkDir, "python", "-I"}
			if !slices.Equal(cmd.Args[:min(len(cmd.Args), len(prefix))], prefix) || cmd.WorkDir == root {
				t.Fatalf("reader can resolve target/PEP723 configuration: %+v", cmd)
			}
		}
		return run(ctx, cmd)
	}
	_, cov, err := New(runner, 0).Strengths(t.Context(), scope.Scope{Root: root})
	if err != nil || cov.Status != evidence.StatusOK {
		t.Fatalf("isolated uv fallback failed: %+v %v", cov, err)
	}
}

func TestSCIPProvisionedPythonReportsRuntimeFailures(t *testing.T) {
	for _, name := range []string{readerTargetVenv, "dependency failure", readerTimeoutCase} {
		t.Run(name, func(t *testing.T) {
			root := makeGoRoot(t)
			// An owned executable stands in for Python; only the mock runs it.
			python, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if name == readerTargetVenv {
				base := python
				python = filepath.Join(root, "python")
				if err := os.Symlink(base, python); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("ARCHFIT_TOOL_PYTHON", python)
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
					if tool == "uv" {
						t.Fatal("explicit interpreter must not fall back to uv")
					}
					return toolrun.ToolInfo{Name: tool}, tool == indexerGo
				},
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					if name == readerTargetVenv || cmd.Name != python {
						t.Fatalf("invalid runtime/indexer executed: %+v", cmd)
					}
					if name == readerTimeoutCase {
						return toolrun.Output{}, context.DeadlineExceeded
					}
					return toolrun.Output{ExitCode: 1, Stderr: []byte("protobuf==7.36.2 required")}, nil
				},
			}
			_, cov, err := New(runner, 0).Strengths(t.Context(), scope.Scope{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if name == readerTimeoutCase {
				if cov.Status != evidence.StatusTimedOut || cov.Reason != reasonTimedOut {
					t.Fatalf("runtime timeout hidden: %+v", cov)
				}
			} else if cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, "ARCHFIT_TOOL_PYTHON") {
				t.Fatalf("runtime failure hidden: %+v", cov)
			}
		})
	}
}
