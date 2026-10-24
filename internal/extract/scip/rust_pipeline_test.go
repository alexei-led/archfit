package scip

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const rustMappedFacts = `"edges":[{"from":"custom_lib::core","to":"custom_lib::adapter","strength":"functional","data_strength":"model","connascence":["name","type","algorithm"]}],"symbols":[{"symbol":"root","path":"src/lib.rs","module":"custom_lib","fan_in":0}]`
const rustAttributionOK = `{"status":"ok","unresolved_symbols":0,"diagnostics":[]}`
const rustAttributionPartial = `{"status":"partial","unresolved_symbols":2,"diagnostics":[{"reason":"ambiguous_target","count":2}]}`

// rustPipelineHarness mocks only tool boundaries, including an explicit owned Python.
// Metadata is mutable between new adapters to exercise cold/warm acquisition parity.
type rustPipelineHarness struct {
	root, python, metadata, readerJSON  string
	metadataErr                         error
	metadataExit                        int
	metadataRuns, indexRuns, readerRuns int
}

func newRustPipelineHarness(t *testing.T) *rustPipelineHarness {
	t.Helper()
	python, err := os.Executable() // a trusted executable outside the target; execution is mocked
	if err != nil {
		t.Fatal(err)
	}
	h := &rustPipelineHarness{root: t.TempDir(), python: python}
	for path, data := range map[string]string{
		filepath.Join(h.root, "Cargo.toml"): "[package]\nname='z-root'\nversion='0.1.0'\n[workspace]\nmembers=['member']\n",
		filepath.Join(h.root, "lib.rs"):     "pub fn run() {}\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("ARCHFIT_TOOL_PYTHON", h.python)
	h.metadata = `{"workspace_members":["root-id","member-id"],"packages":[{"id":"root-id","name":"z-root","version":"0.1.0","targets":[{"name":"custom_lib","src_path":"/workspace/src/lib.rs"}]},{"id":"external-id","name":"not-a-member","version":"1.0.0","targets":[]},{"id":"member-id","name":"a-member","version":"2.0.0","targets":[{"name":"custom-bin","src_path":"/workspace/member/src/main.rs"},{"name":"an-example","src_path":"/workspace/member/examples/demo.rs"}]}]}`
	h.readerJSON = `{` + rustMappedFacts + `,"rust_attribution":` + rustAttributionOK + `}`
	return h
}

func (h *rustPipelineHarness) adapter(t *testing.T, store *factcache.Store) *Adapter {
	t.Helper()
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == indexerRust
		},
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			switch {
			case cmd.Name == toolCargo:
				h.metadataRuns++
				if !reflect.DeepEqual(cmd.Args, []string{"metadata", "--no-deps", "--format-version", "1"}) || cmd.WorkDir != h.root || cmd.Timeout != 60*time.Second {
					t.Fatalf("cargo invocation = %+v", cmd)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("metadata is outside watchdog")
				}
				return toolrun.Output{Stdout: []byte(h.metadata), ExitCode: h.metadataExit}, h.metadataErr
			case slices.Contains(cmd.Args, "--version"):
				return toolrun.Output{Stdout: []byte("rust-analyzer 1.99.1")}, nil
			case cmd.Name == indexerRust:
				h.indexRuns++
				for i, arg := range cmd.Args {
					if arg == flagOutput {
						return toolrun.Output{}, os.WriteFile(cmd.Args[i+1], []byte("index"), 0o600)
					}
				}
				t.Fatal("index output missing")
			case cmd.Name == h.python && slices.Contains(cmd.Args, "--proto"):
				h.readerRuns++
				if cmd.Args[0] != "-I" || cmd.WorkDir == h.root {
					t.Fatalf("reader isolation lost: %+v", cmd)
				}
				for flag, want := range map[string]string{"--package": "a-member,z-root", "--lang": "rust"} {
					i := slices.Index(cmd.Args, flag)
					if i < 0 || i+1 >= len(cmd.Args) || cmd.Args[i+1] != want {
						t.Fatalf("reader %s: %+v", flag, cmd)
					}
				}
				i := slices.Index(cmd.Args, "--rust-metadata")
				if i < 0 || i+1 >= len(cmd.Args) {
					t.Fatalf("mapped Rust metadata argument missing: %+v", cmd)
				}
				path := cmd.Args[i+1]
				if filepath.Dir(path) != cmd.WorkDir {
					t.Fatalf("metadata not in private reader directory: %s", path)
				}
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("metadata mode: %v %v", info, err)
				}
				dir, err := os.Stat(cmd.WorkDir)
				if err != nil || dir.Mode().Perm() != 0o700 {
					t.Fatalf("temp directory mode: %v %v", dir, err)
				}
				privateDir, err := os.OpenRoot(cmd.WorkDir)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = privateDir.Close() }()
				data, err := privateDir.ReadFile(filepath.Base(path))
				if err != nil || string(data) != h.metadata {
					t.Fatalf("raw metadata changed: %s %v", data, err)
				}
				return toolrun.Output{Stdout: []byte(h.readerJSON)}, nil
			case cmd.Name == h.python: // installed dependency probe
				return toolrun.Output{}, nil
			default:
				t.Fatalf("unexpected tool invocation: %+v", cmd)
			}
			return toolrun.Output{}, nil
		},
	}
	a := New(runner, 0)
	a.Cache = store
	return a
}

func rustPipelineCoverage(t *testing.T, a *Adapter, root string, wantFacts bool) []evidence.Coverage {
	t.Helper()
	s := scope.Scope{Root: root}
	strengths, sc, err := a.Strengths(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	symbols, sy, err := a.Symbols(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	data, dc, err := a.DataStrengths(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	conn, cc, err := a.Connascence(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	key := "custom_lib::core\x00custom_lib::adapter"
	if wantFacts {
		if strengths[key] != "functional" || data[key] != "model" || len(conn[key]) != 3 || symbols.Module["root"] != "custom_lib" {
			t.Fatalf("mapped facts lost: %v %v %v %+v", strengths, data, conn, symbols)
		}
	} else if len(strengths)+len(data)+len(conn)+len(symbols.Module) != 0 {
		t.Fatalf("uncertain output fabricated facts: %v %v %v %+v", strengths, data, conn, symbols)
	}
	if sc.Tool != toolName || sy.Tool != toolNameSymbols || dc.Tool != toolName || cc.Tool != toolName {
		t.Fatal("coverage tool names lost")
	}
	return []evidence.Coverage{sc, sy, dc, cc}
}

func TestRustPipelineMappedInvocationAndCacheCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, attribution string
		empty             bool
		status            string
	}{
		{"mapped", rustAttributionOK, false, evidence.StatusOK},
		{"partial root symbols", rustAttributionPartial, false, evidence.StatusPartial},
		{"empty ambiguous", rustAttributionPartial, true, evidence.StatusPartial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRustPipelineHarness(t)
			facts := rustMappedFacts
			if tc.empty {
				facts = `"edges":[],"symbols":[],"symbol_refs":[]`
			}
			h.readerJSON = `{` + facts + `,"rust_attribution":` + tc.attribution + `}`
			store := factcache.NewStore(t.TempDir())
			cold := rustPipelineCoverage(t, h.adapter(t, store), h.root, !tc.empty)
			warm := rustPipelineCoverage(t, h.adapter(t, store), h.root, !tc.empty)
			if !reflect.DeepEqual(cold, warm) {
				t.Fatalf("cold/warm coverage differs: %+v / %+v", cold, warm)
			}
			for _, cov := range cold {
				if cov.Status != tc.status {
					t.Fatalf("coverage = %+v", cov)
				}
				if tc.status == evidence.StatusPartial && (!strings.Contains(cov.Reason, "ambiguous_target=2") || strings.Contains(cov.Reason, "Python") || strings.Contains(cov.Reason, "empty index")) {
					t.Fatalf("Rust reason masked: %+v", cov)
				}
			}
			wantRuns := 1
			if tc.empty {
				wantRuns = 2
			} // empty indexes are deliberately not durable facts
			if h.metadataRuns != 2 || h.indexRuns != wantRuns || h.readerRuns != wantRuns {
				t.Fatalf("phase runs metadata/index/reader = %d/%d/%d", h.metadataRuns, h.indexRuns, h.readerRuns)
			}
		})
	}
}

func TestRustPipelineMandatoryAttribution(t *testing.T) {
	for _, attribution := range []string{"", "null", `{}`, `{"status":"ok","diagnostics":[]}`, `{"status":"ok","unresolved_symbols":0}`, `{"status":"ok","unresolved_symbols":1,"diagnostics":[]}`, `{"status":"partial","unresolved_symbols":0,"diagnostics":[]}`, `{"status":"partial","unresolved_symbols":2,"diagnostics":[{"reason":"guess","count":2}]}`, `{"status":"partial","unresolved_symbols":2,"diagnostics":[{"reason":"missing_target","count":1}]}`} {
		t.Run(attribution, func(t *testing.T) {
			h := newRustPipelineHarness(t)
			h.readerJSON = `{` + rustMappedFacts
			if attribution != "" {
				h.readerJSON += `,"rust_attribution":` + attribution
			}
			h.readerJSON += `}`
			store := factcache.NewStore(t.TempDir())
			for range 2 {
				for _, cov := range rustPipelineCoverage(t, h.adapter(t, store), h.root, false) {
					if cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, "rust attribution") {
						t.Fatalf("missing/invalid attribution accepted: %+v", cov)
					}
				}
			}
			if h.readerRuns != 2 {
				t.Fatal("invalid attribution was cached")
			}
		})
	}
}

func TestRustPipelineMetadataErrorsColdAndWarm(t *testing.T) {
	for _, warm := range []bool{false, true} {
		for _, failure := range []string{"execution", "exit", "invalid json", "missing members", "missing package fields", readerTimeoutCase} {
			t.Run(failure+map[bool]string{false: " cold", true: " warm"}[warm], func(t *testing.T) {
				h := newRustPipelineHarness(t)
				store := factcache.NewStore(t.TempDir())
				if warm {
					rustPipelineCoverage(t, h.adapter(t, store), h.root, true)
				}
				switch failure {
				case "execution":
					h.metadataErr = errors.New("cargo unavailable")
				case "exit":
					h.metadataExit = 1
				case "invalid json":
					h.metadata = "{"
				case "missing members":
					h.metadata = `{"packages":[]}`
				case "missing package fields":
					h.metadata = `{"workspace_members":["id"],"packages":[{"id":"id","name":"pkg"}]}`
				case readerTimeoutCase:
					h.metadataErr = context.DeadlineExceeded
				}
				for _, cov := range rustPipelineCoverage(t, h.adapter(t, store), h.root, false) {
					want := evidence.StatusPartial
					if failure == readerTimeoutCase {
						want = evidence.StatusTimedOut
					}
					if cov.Status != want || !strings.Contains(strings.ToLower(cov.Reason), "rust") {
						t.Fatalf("metadata failure masked: %+v", cov)
					}
				}
				wantRuns := 0
				if warm {
					wantRuns = 1
				}
				if h.indexRuns != wantRuns || h.readerRuns != wantRuns {
					t.Fatal("failed metadata reached reader/index/cache")
				}
			})
		}
	}
}

func TestRustPipelineMetadataInvalidatesCache(t *testing.T) {
	h := newRustPipelineHarness(t)
	store := factcache.NewStore(t.TempDir())
	rustPipelineCoverage(t, h.adapter(t, store), h.root, true)
	// Same source tree and package names, changed target metadata/invocation identity.
	var meta map[string]any
	if err := json.Unmarshal([]byte(h.metadata), &meta); err != nil {
		t.Fatal(err)
	}
	meta["target_directory"] = "/new/target"
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	h.metadata = string(data)
	rustPipelineCoverage(t, h.adapter(t, store), h.root, true)
	if h.indexRuns != 2 || h.readerRuns != 2 {
		t.Fatal("metadata identity missing from cache key")
	}
}

func TestRustPipelineNonemptyRootSymbolsRemainPartial(t *testing.T) {
	h := newRustPipelineHarness(t)
	h.readerJSON = `{"edges":[],"symbols":[{"symbol":"root","module":"custom_lib","path":"src/lib.rs"}],"rust_attribution":` + rustAttributionPartial + `}`
	store := factcache.NewStore(t.TempDir())
	var cold []evidence.Coverage
	for run := range 2 {
		a := h.adapter(t, store)
		s := scope.Scope{Root: h.root}
		strengths, sc, _ := a.Strengths(t.Context(), s)
		symbols, sy, _ := a.Symbols(t.Context(), s)
		data, dc, _ := a.DataStrengths(t.Context(), s)
		conn, cc, _ := a.Connascence(t.Context(), s)
		if len(strengths)+len(data)+len(conn) != 0 || symbols.Module["root"] != "custom_lib" {
			t.Fatalf("root-only subset changed: %v %+v %v %v", strengths, symbols, data, conn)
		}
		coverage := []evidence.Coverage{sc, sy, dc, cc}
		for _, cov := range coverage {
			if cov.Status != evidence.StatusPartial || !strings.Contains(cov.Reason, "ambiguous_target=2") {
				t.Fatalf("root symbols promoted uncertainty: %+v", cov)
			}
		}
		if run == 0 {
			cold = coverage
		} else if !reflect.DeepEqual(cold, coverage) {
			t.Fatal("root-only cache status differs")
		}
	}
	if h.metadataRuns != 2 || h.indexRuns != 1 || h.readerRuns != 1 {
		t.Fatal("root-only subset did not share mapped cache")
	}
}

func TestRustPipelineRejectsLegacyCachedAttribution(t *testing.T) {
	h := newRustPipelineHarness(t)
	store := factcache.NewStore(t.TempDir())
	a := h.adapter(t, store)
	if key := a.cacheKey(h.root, indexerRust, "a-member,z-root", langRust, "rust-analyzer 1.99.1", nil); key != "" {
		t.Fatal("legacy invocation can derive a mapped Rust cache key")
	}
	key := a.cacheKey(h.root, indexerRust, "a-member,z-root", langRust, "rust-analyzer 1.99.1", []byte(h.metadata))
	blob, err := json.Marshal(scipCacheEntry{Indexer: indexerRust, Raw: []byte(`{` + rustMappedFacts + `}`)})
	if err != nil {
		t.Fatal(err)
	}
	store.Put(toolName, key, blob)
	rustPipelineCoverage(t, a, h.root, true)
	if h.indexRuns != 1 || h.readerRuns != 1 {
		t.Fatal("legacy cached output was accepted without attribution")
	}
}
