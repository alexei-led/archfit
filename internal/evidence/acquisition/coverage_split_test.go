// Regression test for the coverage split at the acquisition seam: which rows
// rule and metric evaluation may read, and which are report evidence only.
package acquisition_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexei-led/archfit/internal/application"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/evidence/acquisition"
	evidenceports "github.com/alexei-led/archfit/internal/evidence/ports"
	"github.com/alexei-led/archfit/internal/model/evidence"
	"github.com/alexei-led/archfit/internal/toolrun"
)

const (
	cargoModulesTool = "cargo-modules"
	cargoTool        = "cargo"
	gitTool          = "git"
)

// One workspace member with two modules — enough for cargo-modules to report
// "ok" coverage, which is the only status the coverage metric counts.
func cargoMetaFor(manifest, root string) string {
	return fmt.Sprintf(oneCrateCargoMeta, filepath.ToSlash(manifest), filepath.ToSlash(root))
}

const oneCrateCargoMeta = `{
  "packages": [
    {
      "id": "mylib 0.1.0 (path+file:///tmp/mylib)",
      "name": "mylib",
      "manifest_path": "%s",
      "source": null,
      "dependencies": [],
      "targets": [{"name": "mylib", "kind": ["lib"]}]
    }
  ],
  "workspace_members": ["mylib 0.1.0 (path+file:///tmp/mylib)"],
  "workspace_root": "%s"
}`

const oneCrateDOT = `digraph {
    graph [label="mylib"];

    "mylib" [label="crate|mylib", fillcolor="#5397c8"]; // "crate" node
    "mylib::core" [label="pub mod|core", fillcolor="#f8c04c"]; // "mod" node
    "mylib::util" [label="pub mod|util", fillcolor="#f8c04c"]; // "mod" node

    "mylib" -> "mylib::core" [label="owns", color="#000000"]; // "owns" edge
    "mylib" -> "mylib::util" [label="owns", color="#000000"]; // "owns" edge
    "mylib::util" -> "mylib::core" [label="uses", color="#7f7f7f"]; // "uses" edge
}
`

// cargoRunner answers the git probes scope resolution makes plus the cargo and
// cargo-modules calls the Rust extractor issues; every other tool is absent.
type cargoRunner struct {
	root string
	meta string
}

func (r *cargoRunner) Detect(_ context.Context, tool string) (toolrun.ToolInfo, bool) {
	return toolrun.ToolInfo{Name: tool}, tool == cargoTool || tool == cargoModulesTool
}

func (r *cargoRunner) Run(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
	switch {
	case cmd.Name == gitTool && len(cmd.Args) >= 2 && cmd.Args[0] == "rev-parse" && cmd.Args[1] == "--show-toplevel":
		return toolrun.Output{Stdout: []byte(r.root + "\n")}, nil
	case cmd.Name == gitTool:
		return toolrun.Output{}, nil
	case cmd.Name == cargoModulesTool:
		return toolrun.Output{Stdout: []byte(oneCrateDOT)}, nil
	case cmd.Name == cargoTool && len(cmd.Args) > 0 && cmd.Args[0] == "--version":
		return toolrun.Output{Stdout: []byte("cargo 1.79.0\n")}, nil
	case cmd.Name == cargoTool:
		return toolrun.Output{Stdout: []byte(r.meta)}, nil
	default:
		return toolrun.Output{ExitCode: 1}, nil
	}
}

func (r *cargoRunner) Stream(ctx context.Context, cmd toolrun.ToolCmd, consume func(io.Reader) error) (toolrun.Output, error) {
	out, err := r.Run(ctx, cmd)
	if err != nil {
		return out, err
	}
	return out, consume(bytes.NewReader(out.Stdout))
}

func hasCoverageTool(rows []evidence.Coverage, tool string) bool {
	for _, c := range rows {
		if c.Tool == tool {
			return true
		}
	}
	return false
}

// TestAcquireKeepsModuleGraphCoverageOutOfTheRawRows pins the split the
// coverage metric depends on. cargo-modules is a module-graph analyzer, not a
// file extractor: its row counts CRATES in FilesSeen/FilesApplicable. Rule and
// metric evaluation read Facts.Coverage, and the coverage metric divides
// FilesSeen by FilesApplicable across every non-absent row — so admitting the
// crate row there mixes crate counts into a file ratio, and a partial run
// (FilesApplicable < FilesSeen) can push it past the 1.0 ceiling the metric
// calls definitionally impossible. The row is still report evidence: it must
// reach MarkedCoverage, which feeds ToolCoverage, the coverage-gap block, the
// tool gate, and the partial-module-graph confidence cap.
func TestAcquireKeepsModuleGraphCoverageOutOfTheRawRows(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	manifest := filepath.Join(root, "Cargo.toml")
	if err := os.WriteFile(manifest, []byte("[package]\nname = \"mylib\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "lib.rs"), []byte("pub mod core;\npub mod util;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{Version: 1}
	cfg.Languages.Rust.Enabled = evidenceports.ModeOn
	cfg.Analyzers.CargoModules.Enabled = evidenceports.ModeOn
	svc := &acquisition.Service{
		ConfigPath: filepath.Join(root, ".archfit.yaml"), Root: root,
		Options: cfg.RunOptions(), Policy: cfg.PolicySnapshot(),
		Runner: &cargoRunner{root: root, meta: cargoMetaFor(manifest, root)}, Stderr: &bytes.Buffer{},
	}

	acquired, err := svc.Acquire(context.Background(), application.AnalysisRequest{EvaluatedAt: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCoverageTool(acquired.Context.MarkedCoverage, cargoModulesTool) {
		t.Fatalf("cargo-modules missing from MarkedCoverage (%v): the module-graph row is report evidence and must be disclosed",
			coverageTools(acquired.Context.MarkedCoverage))
	}
	if hasCoverageTool(acquired.Facts.Coverage, cargoModulesTool) {
		t.Errorf("cargo-modules present in the raw rows (%v): rule and metric evaluation read these, and the crate counts corrupt the file-coverage ratio",
			coverageTools(acquired.Facts.Coverage))
	}
}

func coverageTools(rows []evidence.Coverage) []string {
	out := make([]string, 0, len(rows))
	for _, c := range rows {
		out = append(out, c.Tool+"="+c.Status)
	}
	return out
}
