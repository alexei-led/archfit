package acquisition

import (
	"context"
	"maps"
	"slices"
	"testing"

	evidencecontract "github.com/alexei-led/archfit/v3/internal/evidence"
	"github.com/alexei-led/archfit/v3/internal/factcache"
	"github.com/alexei-led/archfit/v3/internal/model/fileclass"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/policy"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

const (
	crateShared = "yazi-shared"
	crateFM     = "yazi-fm"
	crateYazi   = "yazi"
	crateApp    = "app"
	modShared   = "shared"
	crateLinter = "ruff_linter"
	idShared    = "yazi_shared"
)

func TestCrateRootDirsKeyEveryCrateSpelling(t *testing.T) {
	got := crateRootDirsOf([]graph.CrateRoot{
		{Dir: crateShared, Name: crateShared, Crate: idShared},
		{Dir: crateFM, Name: crateFM, Crate: crateYazi},
		{Dir: "", Name: crateApp, Crate: crateApp},
	})
	want := map[string]string{
		crateShared: crateShared, idShared: crateShared,
		crateFM: crateFM, crateYazi: crateFM,
		crateApp: "",
	}
	if !maps.Equal(got, want) {
		t.Errorf("crateRootDirsOf = %v, want %v", got, want)
	}
}

func TestCrateRootDirsPackageNameWinsACollision(t *testing.T) {
	roots := []graph.CrateRoot{
		{Dir: "tools/a", Name: "a-cli", Crate: modShared},
		{Dir: "crates/shared", Name: modShared, Crate: modShared},
	}
	for _, order := range [][]graph.CrateRoot{roots, {roots[1], roots[0]}} {
		if got := crateRootDirsOf(order)[modShared]; got != "crates/shared" {
			t.Errorf(`crateRootDirsOf["shared"] = %q, want the package named shared`, got)
		}
	}
}

func TestCrateOwnersResolveCratesToTheirDeclaredModules(t *testing.T) {
	mm := policy.BuildModuleMap(map[string]policy.ModuleDef{
		modShared: {Paths: []string{crateShared}},
		"linter":  {Paths: []string{"crates/ruff_linter/**"}},
	})
	f := evidencecontract.Facts{Graph: graph.Build([]graph.Facts{{CrateRoots: []graph.CrateRoot{
		{Dir: crateShared, Name: crateShared, Crate: idShared},
		{Dir: "crates/ruff_linter", Name: crateLinter, Crate: crateLinter},
		{Dir: "tools/undeclared", Name: "undeclared", Crate: "undeclared"},
	}}})}
	want := map[string]string{crateShared: modShared, idShared: modShared, crateLinter: "linter"}
	if got := crateOwnersOf(f, mm); !maps.Equal(got, want) {
		t.Errorf("crateOwnersOf = %v, want %v", got, want)
	}
	if got := crateOwnersOf(evidencecontract.Facts{}, mm); got != nil {
		t.Errorf("crateOwnersOf without a graph = %v, want nil", got)
	}
}

func TestSourceSelectorsUseProducerRootsWithoutEdges(t *testing.T) {
	f := evidencecontract.Facts{
		Graph:          graph.Build([]graph.Facts{{CrateRoots: []graph.CrateRoot{{Dir: "core", Name: "smoke-core"}, {Dir: "adapter", Name: "smoke-adapter"}}}}),
		FileLOC:        map[string]int{"core/src/lib.rs": 5},
		FileClassIndex: map[string]fileclass.FileClass{"adapter/src/lib.rs": fileclass.Production},
	}
	got := sourceSelectorsOf(f)
	if got["core/src/lib.rs"] != "smoke-core" || got["adapter/src/lib.rs"] != "smoke-adapter" {
		t.Fatalf("selectors = %+v", got)
	}
	f.Graph = nil
	got = sourceSelectorsOf(f)
	if got["core/src/lib.rs"] != "" || got["adapter/src/lib.rs"] != "" {
		t.Fatalf("guessed crates without producer: %+v", got)
	}
}

const stdlibTestModule = "example.com/shop"

// stdlibRunner answers `go list std` with canned output and counts the calls.
type stdlibRunner struct {
	toolrun.Runner
	stdout string
	calls  *int
}

// isGoListStd reports whether cmd is the stdlib listing `go list std`.
func isGoListStd(cmd toolrun.ToolCmd) bool {
	return cmd.Name == "go" && slices.Equal(cmd.Args, []string{"list", "std"})
}

func (r stdlibRunner) Run(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	if isGoListStd(cmd) {
		*r.calls++
		return toolrun.Output{Stdout: []byte(r.stdout)}, nil
	}
	return toolrun.Output{ExitCode: 1}, nil
}

func TestGoStdlibPackagesKeepsOnlyImportablePaths(t *testing.T) {
	calls := 0
	runner := stdlibRunner{stdout: "bufio\ncrypto/internal/boring\ndatabase/sql\ninternal/abi\nnet/http\nvendor/golang.org/x/net/idna\n\n", calls: &calls}
	got := goStdlibPackages(context.Background(), runner, nil, t.TempDir(), []string{stdlibTestModule})
	if want := []string{"bufio", "database/sql", "net/http"}; !slices.Equal(got, want) {
		t.Fatalf("stdlib packages = %v, want %v (internal and vendor packages dropped)", got, want)
	}
	if got := goStdlibPackages(context.Background(), runner, nil, t.TempDir(), nil); got != nil || calls != 1 {
		t.Fatalf("without a Go module: packages = %v after %d probe(s), want nil and no probe", got, calls)
	}
	if got := goStdlibPackages(context.Background(), exitRunner{}, nil, t.TempDir(), []string{stdlibTestModule}); got != nil {
		t.Fatalf("failed probe: packages = %v, want nil", got)
	}
}

// exitRunner fails every command with a non-zero exit.
type exitRunner struct{ toolrun.Runner }

func (exitRunner) Run(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
	return toolrun.Output{ExitCode: 1, Stderr: []byte("go: not found")}, nil
}

// toolchainRunner reports a toolchain identity through `go env -json` and
// counts the `go list std` runs, which fail while listExit is non-zero.
type toolchainRunner struct {
	toolrun.Runner
	identity  string
	listExit  int
	listCalls *int
}

func (r toolchainRunner) Run(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	switch {
	case isGoListStd(cmd):
		*r.listCalls++
		if r.listExit != 0 {
			return toolrun.Output{ExitCode: r.listExit, Stderr: []byte("go: toolchain not available")}, nil
		}
		return toolrun.Output{Stdout: []byte("bufio\nnet/http\n")}, nil
	case cmd.Name == "go" && len(cmd.Args) > 1 && cmd.Args[0] == "env" && cmd.Args[1] == "-json":
		if r.identity == "" {
			return toolrun.Output{ExitCode: 1}, nil
		}
		if cmd.WorkDir == "" {
			return toolrun.Output{ExitCode: 2, Stderr: []byte("identity probed outside the analysed root")}, nil
		}
		return toolrun.Output{Stdout: []byte(r.identity)}, nil
	}
	return toolrun.Output{ExitCode: 1}, nil
}

// TestGoStdlibPackagesCachesPerToolchain pins the warm path: the standard
// library list is a fact of the toolchain, so a second run with the same
// toolchain reads it from the fact cache instead of running `go list std`.
func TestGoStdlibPackagesCachesPerToolchain(t *testing.T) {
	const go126 = `{"GOARCH":"arm64","GOOS":"darwin","GOROOT":"/go","GOVERSION":"go1.26.1"}`
	const go127 = `{"GOARCH":"arm64","GOOS":"darwin","GOROOT":"/go","GOVERSION":"go1.27.0"}`
	modules := []string{stdlibTestModule}
	want := []string{"bufio", "net/http"}
	type step struct {
		identity string
		listExit int
		refresh  bool
		runs     int // cumulative go list std runs after this step
	}
	cases := []struct {
		name  string
		store bool
		steps []step
	}{
		{"same toolchain reads the cache", true, []step{{identity: go126, runs: 1}, {identity: go126, runs: 1}}},
		{"another toolchain misses", true, []step{{identity: go126, runs: 1}, {identity: go127, runs: 2}, {identity: go126, runs: 2}}},
		{"refresh re-runs and records", true, []step{{identity: go126, runs: 1}, {identity: go126, refresh: true, runs: 2}, {identity: go126, runs: 2}}},
		{"a failed list is never cached", true, []step{{identity: go126, listExit: 1, runs: 1}, {identity: go126, runs: 2}, {identity: go126, runs: 2}}},
		{"no toolchain identity runs uncached", true, []step{{runs: 1}, {runs: 2}}},
		{"no store runs uncached", false, []step{{identity: go126, runs: 1}, {identity: go126, runs: 2}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var store *factcache.Store
			if tc.store {
				store = factcache.NewStore(t.TempDir())
			}
			runs := 0
			for i, s := range tc.steps {
				if store != nil {
					store.RefreshMode = s.refresh
				}
				runner := toolchainRunner{identity: s.identity, listExit: s.listExit, listCalls: &runs}
				got := goStdlibPackages(context.Background(), runner, store, t.TempDir(), modules)
				if s.listExit == 0 && !slices.Equal(got, want) {
					t.Fatalf("step %d: packages = %v, want %v", i, got, want)
				}
				if runs != s.runs {
					t.Fatalf("step %d: go list std ran %d time(s), want %d", i, runs, s.runs)
				}
			}
		})
	}
}
