package rust_test

import (
	"context"
	"strings"
	"testing"

	"github.com/alexei-led/archfit/internal/extract/rust"
	"github.com/alexei-led/archfit/internal/model/graph"
	"github.com/alexei-led/archfit/internal/scope"
	"github.com/alexei-led/archfit/internal/toolrun"
	"github.com/alexei-led/archfit/internal/view"
)

const toolModules = "cargo-modules"

// canned DOT output for a library crate with two modules.
const libCrateDOT = `digraph {
    graph [label="mylib"];
    node [shape="record"];
    edge [fontname="monospace"];

    "mylib" [label="crate|mylib", fillcolor="#5397c8"]; // "crate" node
    "mylib::core" [label="pub mod|core", fillcolor="#f8c04c"]; // "mod" node
    "mylib::util" [label="pub mod|util", fillcolor="#f8c04c"]; // "mod" node
    "mylib::core::SomeStruct" [label="pub struct|SomeStruct", fillcolor="#81c169"]; // "struct" node

    "mylib" -> "mylib::core" [label="owns", color="#000000"]; // "owns" edge
    "mylib" -> "mylib::util" [label="owns", color="#000000"]; // "owns" edge
    "mylib::util" -> "mylib::core" [label="uses", color="#7f7f7f"]; // "uses" edge
    "mylib::util::helper" -> "mylib::core::SomeStruct" [label="uses", color="#7f7f7f"]; // item-level "uses" edge
}
`

// canned DOT output for a binary-only crate.
const binCrateDOT = `digraph {
    graph [label="mybin"];
    node [shape="record"];

    "mybin" [label="crate|mybin", fillcolor="#5397c8"]; // "crate" node
    "mybin::cli" [label="pub(crate) mod|cli", fillcolor="#f8c04c"]; // "mod" node

    "mybin" -> "mybin::cli" [label="owns", color="#000000"]; // "owns" edge
}
`

// canned cargo metadata for a lib crate.
const libCargoMeta = `{
  "packages": [
    {
      "id": "mylib 0.1.0 (path+file:///tmp/mylib)",
      "name": "mylib",
      "manifest_path": "/tmp/mylib/Cargo.toml",
      "source": null,
      "dependencies": [],
      "targets": [{"name": "mylib", "kind": ["lib"]}]
    }
  ],
  "workspace_members": ["mylib 0.1.0 (path+file:///tmp/mylib)"],
  "workspace_root": "/tmp/mylib"
}`

// canned cargo metadata for a binary-only crate.
const binCargoMeta = `{
  "packages": [
    {
      "id": "mybin 0.1.0 (path+file:///tmp/mybin)",
      "name": "mybin",
      "manifest_path": "/tmp/mybin/Cargo.toml",
      "source": null,
      "dependencies": [],
      "targets": [{"name": "mybin", "kind": ["bin"]}]
    }
  ],
  "workspace_members": ["mybin 0.1.0 (path+file:///tmp/mybin)"],
  "workspace_root": "/tmp/mybin"
}`

// moduleGraphRunner returns a RunnerMock that:
//   - reports both "cargo" and toolModules as present
//   - returns metaJSON for cargo metadata calls
//   - returns dotOutput for cargo-modules calls
//   - returns a version string for --version calls
func moduleGraphRunner(metaJSON, dotOutput string) *toolrun.RunnerMock {
	return &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == testTool || tool == toolModules
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			switch {
			case cmd.Name == testTool && len(cmd.Args) > 0 && cmd.Args[0] == "--version":
				return toolrun.Output{Stdout: []byte(cargoVersion)}, nil
			case cmd.Name == testTool:
				return toolrun.Output{Stdout: []byte(metaJSON)}, nil
			case cmd.Name == toolModules:
				return toolrun.Output{Stdout: []byte(dotOutput)}, nil
			default:
				return toolrun.Output{ExitCode: 1}, nil
			}
		},
	}
}

func TestModuleGraph_LibCrate(t *testing.T) {
	runner := moduleGraphRunner(libCargoMeta, libCrateDOT)
	e := rust.New(runner, view.ExtractConfig{
		Mode:        view.ModeAuto,
		ModuleGraph: true,
	})

	facts, cov, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if cov.Status != "ok" {
		t.Errorf("cargo coverage status = %q, want %q", cov.Status, "ok")
	}

	// Expect: crate-level node "mylib" + module nodes "mylib", "mylib::core", "mylib::util"
	// (struct nodes are filtered out).
	nodeIDs := make(map[string]bool)
	for _, n := range facts.Nodes {
		nodeIDs[n.ID()] = true
	}
	for _, want := range []string{
		"package:mylib",
		"package:mylib::core",
		"package:mylib::util",
	} {
		if !nodeIDs[want] {
			t.Errorf("missing node %q; got %v", want, nodeKeys(facts.Nodes))
		}
	}
	if nodeIDs["package:mylib::core::SomeStruct"] {
		t.Error("struct node should be filtered out, but was present")
	}

	// Edges are module->module dependencies aggregated from "uses" edges. "owns"
	// (hierarchy) edges are NOT graph edges. The module-level uses (util->core) and
	// the item-level uses (util::helper->core::SomeStruct) both aggregate to the same
	// module edge and dedup to exactly one depends_on; no belongs_to.
	edgeKinds := make(map[graph.EdgeKind]int)
	var depEdges []string
	for _, ed := range facts.Edges {
		edgeKinds[ed.Kind]++
		if ed.Kind == graph.EdgeKindDependsOn {
			depEdges = append(depEdges, ed.From+" -> "+ed.To)
		}
	}
	if edgeKinds[graph.EdgeKindBelongsTo] != 0 {
		t.Errorf("owns/hierarchy must not be emitted as graph edges; got %d belongs_to", edgeKinds[graph.EdgeKindBelongsTo])
	}
	if got := edgeKinds[graph.EdgeKindDependsOn]; got != 1 {
		t.Errorf("expected exactly one aggregated depends_on edge, got %d: %v", got, depEdges)
	}
	if len(depEdges) != 1 || depEdges[0] != "package:mylib::util -> package:mylib::core" {
		t.Errorf("expected aggregated edge package:mylib::util -> package:mylib::core, got %v", depEdges)
	}

	// Cargo-modules coverage must be reported.
	modCov := e.LastModuleGraphCoverage()
	if modCov.Tool != toolModules {
		t.Errorf("module graph coverage tool = %q, want cargo-modules", modCov.Tool)
	}
	if modCov.Status != "ok" {
		t.Errorf("module graph coverage status = %q, want ok", modCov.Status)
	}
}

func TestModuleGraph_BinaryOnlyCrate(t *testing.T) {
	runner := moduleGraphRunner(binCargoMeta, binCrateDOT)
	e := rust.New(runner, view.ExtractConfig{
		Mode:        view.ModeAuto,
		ModuleGraph: true,
	})

	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Verify --bin was used: the runner must have called cargo-modules with --bin mybin.
	binFlagSeen := false
	for _, call := range runner.RunCalls() {
		if call.Cmd.Name == toolModules {
			args := strings.Join(call.Cmd.Args, " ")
			if strings.Contains(args, "--bin") {
				binFlagSeen = true
			}
		}
	}
	if !binFlagSeen {
		t.Error("expected cargo-modules to be called with --bin for binary-only crate")
	}

	// Both the crate root and cli module should be present.
	nodeIDs := make(map[string]bool)
	for _, n := range facts.Nodes {
		nodeIDs[n.ID()] = true
	}
	if !nodeIDs["package:mybin"] {
		t.Errorf("missing package:mybin node; got %v", nodeKeys(facts.Nodes))
	}
	if !nodeIDs["package:mybin::cli"] {
		t.Errorf("missing package:mybin::cli node; got %v", nodeKeys(facts.Nodes))
	}
}

func TestModuleGraph_Disabled(t *testing.T) {
	runner := moduleGraphRunner(libCargoMeta, libCrateDOT)
	e := rust.New(runner, view.ExtractConfig{
		Mode:        view.ModeAuto,
		ModuleGraph: false,
	})

	_, _, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// cargo-modules must not have been invoked.
	for _, call := range runner.RunCalls() {
		if call.Cmd.Name == toolModules {
			t.Error("cargo-modules was invoked but ModuleGraph is disabled")
		}
	}

	cov := e.LastModuleGraphCoverage()
	if cov.Status != statusAbsent {
		t.Errorf("module graph coverage status = %q, want absent when disabled", cov.Status)
	}
}

func TestModuleGraph_ToolAbsent(t *testing.T) {
	// cargo-modules not on PATH.
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
			return toolrun.ToolInfo{Name: tool}, tool == "cargo" // cargo-modules absent
		},
		RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			if cmd.Name == "cargo" && len(cmd.Args) > 0 && cmd.Args[0] == "--version" {
				return toolrun.Output{Stdout: []byte(cargoVersion)}, nil
			}
			return toolrun.Output{Stdout: []byte(libCargoMeta)}, nil
		},
	}
	e := rust.New(runner, view.ExtractConfig{
		Mode:        view.ModeAuto,
		ModuleGraph: true,
	})

	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// No module nodes should be present (only crate-level from cargo metadata).
	for _, n := range facts.Nodes {
		if strings.Contains(n.Path, "::") {
			t.Errorf("unexpected module node %q when cargo-modules is absent", n.Path)
		}
	}

	cov := e.LastModuleGraphCoverage()
	if cov.Tool != toolModules {
		t.Errorf("module graph coverage tool = %q, want %q", cov.Tool, toolModules)
	}
	if cov.Status != statusAbsent {
		t.Errorf("module graph coverage status = %q, want absent", cov.Status)
	}
}

func TestModuleGraph_MalformedDOTLinesSkipped(t *testing.T) {
	malformedDOT := `digraph {
    "mylib" [label="crate|mylib"]; // "crate" node
    "mylib::util" [label="pub mod|util"]; // "mod" node
    this is not valid DOT at all !!!
    -> broken edge ->
    "mylib" -> "mylib::util" [label="owns"]; // "owns" edge
}
`
	runner := moduleGraphRunner(libCargoMeta, malformedDOT)
	e := rust.New(runner, view.ExtractConfig{
		Mode:        view.ModeAuto,
		ModuleGraph: true,
	})

	facts, _, err := e.Extract(context.Background(), scope.Scope{Root: fixtureDir})
	if err != nil {
		t.Fatalf("Extract should not error on malformed DOT lines: %v", err)
	}

	// Valid lines still parsed.
	nodeIDs := make(map[string]bool)
	for _, n := range facts.Nodes {
		nodeIDs[n.ID()] = true
	}
	if !nodeIDs["package:mylib"] {
		t.Error("missing package:mylib despite valid node line")
	}
	if !nodeIDs["package:mylib::util"] {
		t.Error("missing package:mylib::util despite valid node line")
	}
}

// nodeKeys returns a sorted list of node IDs for error messages.
func nodeKeys(nodes []graph.Node) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID()
	}
	return ids
}
