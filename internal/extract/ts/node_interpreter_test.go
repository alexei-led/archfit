package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	evidenceports "github.com/alexei-led/archfit/v3/internal/evidence/ports"
	"github.com/alexei-led/archfit/v3/internal/extract/ts"
	"github.com/alexei-led/archfit/v3/internal/model/evidence"
	"github.com/alexei-led/archfit/v3/internal/model/graph"
	"github.com/alexei-led/archfit/v3/internal/scope"
	"github.com/alexei-led/archfit/v3/internal/toolrun"
)

func TestExtract_TargetNodeInterpreterNeverExecutes(t *testing.T) {
	producer := ownedDepcruise(t)
	root := writeTSFixture(t, "export const value = 1", nil)
	localBin := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(localBin, 0o750); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "target-node-executed")
	decoy := filepath.Join(localBin, nodeExecutable)
	if err := os.WriteFile(decoy, []byte("#!/bin/sh\nprintf executed > '"+marker+"'\nexit 99\n"), 0o700); err != nil { //nolint:gosec // This executable fixture reproduces the inherited-PATH interpreter attack.
		t.Fatal(err)
	}
	ownedBin := t.TempDir()
	if err := os.Symlink(producer, filepath.Join(ownedBin, depcruiseExecutable)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", strings.Join([]string{localBin, ownedBin, os.Getenv("PATH")}, string(os.PathListSeparator)))
	for _, mode := range []evidenceports.ToolMode{evidenceports.ModeAuto, evidenceports.ModeOn} {
		_, cov, err := ts.New(toolrun.New(), evidenceports.ExtractConfig{Mode: mode, Src: "."}).Extract(t.Context(), scope.Scope{Root: root})
		if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
			t.Fatalf("target node executed through owned producer shebang: %v", statErr)
		}
		if mode == evidenceports.ModeOn {
			if err == nil {
				t.Fatal("required target-owned interpreter must error")
			}
		} else if err != nil || cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing == 0 {
			t.Fatalf("target-owned interpreter must be incomplete: coverage=%+v error=%v", cov, err)
		}
	}
}

func TestExtract_RejectsUnsafeNodeInterpreter(t *testing.T) {
	for _, kind := range []string{"not-installed", "relative", "nonexistent", "target", "target-symlink", "symlink-destination", "git-root"} {
		t.Run(kind, func(t *testing.T) {
			root := writeTSFixture(t, "export const value = 1", nil)
			repo := filepath.Dir(root)
			path := filepath.Join(root, nodeExecutable)
			if err := os.WriteFile(path, []byte("target code"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "not-installed":
				path = ""
			case "relative":
				path = nodeExecutable
			case "nonexistent":
				path = filepath.Join(t.TempDir(), "missing-node")
			case "target-symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				owned, _ := mockNodeInfo()
				if err := os.Symlink(owned.Path, path); err != nil {
					t.Fatal(err)
				}
			case "symlink-destination":
				link := filepath.Join(t.TempDir(), nodeExecutable)
				if err := os.Symlink(path, link); err != nil {
					t.Fatal(err)
				}
				path = link
			case "git-root":
				path = filepath.Join(repo, nodeExecutable)
				if err := os.WriteFile(path, []byte("target code"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runner := &toolrun.RunnerMock{
				DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
					if name == nodeExecutable {
						return toolrun.ToolInfo{Name: name, Path: path}, path != ""
					}
					return toolrun.ToolInfo{Name: name, Path: ownedDepcruisePath}, name == depcruiseExecutable
				},
				RunFunc: func(_ context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
					t.Fatalf("unsafe interpreter executed: %+v", cmd)
					return toolrun.Output{}, nil
				},
			}
			for _, mode := range []evidenceports.ToolMode{evidenceports.ModeAuto, evidenceports.ModeOn} {
				_, cov, err := ts.New(runner, evidenceports.ExtractConfig{Mode: mode, Src: "."}).Extract(t.Context(), scope.Scope{Root: root, GitRoot: repo})
				if mode == evidenceports.ModeOn {
					if err == nil {
						t.Fatal("required unsafe interpreter must error")
					}
				} else if err != nil || cov.Status != evidence.StatusPartial || cov.UnresolvedInputsMissing != 1 {
					t.Fatalf("unsafe interpreter must be incomplete: coverage=%+v error=%v", cov, err)
				}
			}
		})
	}
}

func TestExtract_OwnedNodeBypassesInheritedPATH(t *testing.T) {
	producer := ownedDepcruise(t)
	realRunner := toolrun.New()
	node, ok := realRunner.Detect(t.Context(), nodeExecutable)
	if !ok {
		t.Fatal("owned Node interpreter not available")
	}
	canonical, err := filepath.EvalSymlinks(node.Path)
	if err != nil {
		t.Fatal(err)
	}
	root := writeTSFixture(t, `import { secret } from "./forbidden"; export const value = secret;`, map[string]string{forbiddenTSFile: "export const secret = 1"})
	localBin := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(localBin, 0o750); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "target-node-executed")
	if err := os.WriteFile(filepath.Join(localBin, nodeExecutable), []byte("#!/bin/sh\nprintf executed > '"+marker+"'\nexit 99\n"), 0o700); err != nil { //nolint:gosec // The decoy must be executable to exercise /usr/bin/env node selection.
		t.Fatal(err)
	}
	t.Setenv("PATH", localBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	calls := 0
	runner := &toolrun.RunnerMock{
		DetectFunc: func(_ context.Context, name string) (toolrun.ToolInfo, bool) {
			if name == nodeExecutable {
				return node, true
			}
			return toolrun.ToolInfo{Name: name, Path: producer}, name == depcruiseExecutable
		},
		RunFunc: func(ctx context.Context, cmd toolrun.ToolCmd) (toolrun.Output, error) {
			calls++
			if cmd.Name != canonical || len(cmd.Args) == 0 || cmd.Args[0] != producer {
				t.Fatalf("producer shebang was not bypassed: %+v", cmd)
			}
			return realRunner.Run(ctx, cmd)
		},
	}
	from := graph.Node{Kind: graph.NodeKindFile, Path: fixtureTSName}.ID()
	facts, cov, err := ts.New(runner, evidenceports.ExtractConfig{Mode: evidenceports.ModeOn, Src: ".", Internal: []string{forbiddenTSFile}}).Extract(t.Context(), scope.Scope{Root: root})
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("target node executed despite owned interpreter: %v", statErr)
	}
	if err != nil || cov.Status != evidence.StatusOK || calls != 2 || !slices.ContainsFunc(facts.Edges, func(edge graph.Edge) bool {
		return edge.From == from && edge.To == forbiddenTSNodeID && edge.Kind == graph.EdgeKindUsesInternal
	}) {
		t.Fatalf("owned runtime lost forbidden edge: facts=%+v coverage=%+v calls=%d error=%v", facts, cov, calls, err)
	}
}
