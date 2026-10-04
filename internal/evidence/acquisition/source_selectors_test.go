package acquisition

import (
	"context"
	"slices"
	"testing"

	evidencecontract "github.com/alexei-led/archfit/internal/evidence"
	"github.com/alexei-led/archfit/internal/model/fileclass"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/toolrun"
)

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

// stdlibRunner answers `go list std` with canned output and counts the calls.
type stdlibRunner struct {
	toolrun.Runner
	stdout string
	calls  *int
}

func (r stdlibRunner) Run(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	if cmd.Name == "go" && slices.Equal(cmd.Args, []string{"list", "std"}) {
		*r.calls++
		return toolrun.Output{Stdout: []byte(r.stdout)}, nil
	}
	return toolrun.Output{ExitCode: 1}, nil
}

func TestGoStdlibPackagesKeepsOnlyImportablePaths(t *testing.T) {
	calls := 0
	runner := stdlibRunner{stdout: "bufio\ncrypto/internal/boring\ndatabase/sql\ninternal/abi\nnet/http\nvendor/golang.org/x/net/idna\n\n", calls: &calls}
	got := goStdlibPackages(context.Background(), runner, t.TempDir(), []string{"example.com/shop"})
	if want := []string{"bufio", "database/sql", "net/http"}; !slices.Equal(got, want) {
		t.Fatalf("stdlib packages = %v, want %v (internal and vendor packages dropped)", got, want)
	}
	if got := goStdlibPackages(context.Background(), runner, t.TempDir(), nil); got != nil || calls != 1 {
		t.Fatalf("without a Go module: packages = %v after %d probe(s), want nil and no probe", got, calls)
	}
	if got := goStdlibPackages(context.Background(), exitRunner{}, t.TempDir(), []string{"example.com/shop"}); got != nil {
		t.Fatalf("failed probe: packages = %v, want nil", got)
	}
}

// exitRunner fails every command with a non-zero exit.
type exitRunner struct{ toolrun.Runner }

func (exitRunner) Run(context.Context, toolrun.ToolCmd) (toolrun.Output, error) {
	return toolrun.Output{ExitCode: 1, Stderr: []byte("go: not found")}, nil
}
